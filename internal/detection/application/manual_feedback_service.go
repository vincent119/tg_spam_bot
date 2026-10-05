package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/vincent119/tg_spam_bot/internal/detection/domain"
)

// ManualFeedbackInput 的 Text 僅在本次請求中使用，不會保存至資料庫。
type ManualFeedbackInput struct {
	Feedback     ManualFeedback
	Text         string
	MaxTextRunes int
	EmbeddingTTL time.Duration
}

// ManualFeedbackResult 分別描述標記保存與向量化結果。
type ManualFeedbackResult struct {
	Feedback           ManualFeedback
	Saved              bool
	EmbeddingStatus    string
	EmbeddingErrorCode string
}

// ManualFeedbackService 先保存標記，再選擇性同步產生群組範圍向量。
type ManualFeedbackService struct {
	store      ManualFeedbackStore
	embeddings EmbeddingProvider
	now        func() time.Time
}

// NewManualFeedbackService 允許沒有向量 provider，以便先保存人工標記。
func NewManualFeedbackService(store ManualFeedbackStore, embeddings EmbeddingProvider) (*ManualFeedbackService, error) {
	if store == nil {
		return nil, errors.New("manual feedback store is required")
	}
	return &ManualFeedbackService{store: store, embeddings: embeddings, now: time.Now}, nil
}

// Submit 保證標記先持久化；向量化失敗會保留標記並回報階段狀態。
func (s *ManualFeedbackService) Submit(ctx context.Context, input ManualFeedbackInput) (ManualFeedbackResult, error) {
	feedback := input.Feedback
	if feedback.ChatID == 0 || feedback.MessageID <= 0 || feedback.TargetUserID <= 0 || feedback.OperatorID <= 0 || feedback.CommandUpdateID == 0 || feedback.ContentFingerprint == "" {
		return ManualFeedbackResult{}, errors.New("manual feedback identifiers are required")
	}
	if feedback.Label != domain.AILabelSpam && feedback.Label != domain.AILabelHam {
		return ManualFeedbackResult{}, errors.New("manual feedback label is invalid")
	}
	if feedback.CreatedAt.IsZero() {
		feedback.CreatedAt = s.now().UTC()
	}
	feedback.EmbeddingStatus = ManualFeedbackEmbeddingPending
	if strings.TrimSpace(input.Text) == "" {
		feedback.EmbeddingStatus = ManualFeedbackEmbeddingUnavailable
	} else if s.embeddings == nil {
		feedback.EmbeddingStatus = ManualFeedbackEmbeddingDisabled
	}
	current, saved, err := s.store.SaveFeedback(ctx, feedback)
	if err != nil {
		return ManualFeedbackResult{}, fmt.Errorf("save manual feedback: %w", err)
	}
	result := ManualFeedbackResult{Feedback: current, Saved: saved, EmbeddingStatus: current.EmbeddingStatus, EmbeddingErrorCode: current.EmbeddingErrorCode}
	if !saved || current.EmbeddingStatus != ManualFeedbackEmbeddingPending {
		return result, nil
	}
	if input.MaxTextRunes <= 0 {
		return s.embeddingFailed(ctx, result, "invalid_text_limit")
	}
	embedding, err := s.embeddings.Embed(ctx, domain.NewEmbeddingInput(input.Text, input.MaxTextRunes))
	if err != nil {
		return s.embeddingFailed(ctx, result, errorCode(err))
	}
	if err := embedding.Validate(); err != nil {
		return s.embeddingFailed(ctx, result, "invalid_embedding")
	}
	ttl := input.EmbeddingTTL
	if ttl <= 0 {
		ttl = 168 * time.Hour
	}
	err = s.store.SaveScopedFeedbackEmbedding(ctx, ScopedFeedbackEmbedding{
		Feedback: current, Embedding: embedding, CreatedAt: s.now().UTC(), ExpiresAt: s.now().UTC().Add(ttl),
	})
	if errors.Is(err, ErrStaleManualFeedback) {
		result.EmbeddingStatus = ManualFeedbackEmbeddingSuperseded
		return result, nil
	}
	if err != nil {
		return s.embeddingFailed(ctx, result, "embedding_store_failed")
	}
	result.EmbeddingStatus = ManualFeedbackEmbeddingCompleted
	result.Feedback.EmbeddingStatus = result.EmbeddingStatus
	return result, nil
}

func (s *ManualFeedbackService) embeddingFailed(ctx context.Context, result ManualFeedbackResult, code string) (ManualFeedbackResult, error) {
	err := s.store.MarkFeedbackEmbeddingFailed(ctx, result.Feedback.ChatID, result.Feedback.MessageID, result.Feedback.Revision, code)
	if errors.Is(err, ErrStaleManualFeedback) {
		result.EmbeddingStatus = ManualFeedbackEmbeddingSuperseded
		return result, nil
	}
	if err != nil {
		return result, fmt.Errorf("record manual feedback embedding failure: %w", err)
	}
	result.EmbeddingStatus = ManualFeedbackEmbeddingFailed
	result.EmbeddingErrorCode = code
	result.Feedback.EmbeddingStatus = result.EmbeddingStatus
	result.Feedback.EmbeddingErrorCode = code
	return result, nil
}
