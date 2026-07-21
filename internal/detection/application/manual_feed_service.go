package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/vincent119/tg_spam_bot/internal/detection/domain"
)

// ManualFeedInput 是 `/feedspam` 當下同步向量化所需資料；Text 不會被 store 保存。
type ManualFeedInput struct {
	Sample       ManualSample
	Text         string
	MaxTextRunes int
	EmbeddingTTL time.Duration
}

// ManualFeedService 保存人工樣本並在原文仍在記憶體時同步產生 embedding。
type ManualFeedService struct {
	Samples    ManualSampleStore
	Embeddings EmbeddingProvider
	Store      EmbeddingStore
	now        func() time.Time
}

// NewManualFeedService 建立 manual feed service。
func NewManualFeedService(samples ManualSampleStore, embeddings EmbeddingProvider, store EmbeddingStore) (*ManualFeedService, error) {
	if samples == nil || embeddings == nil || store == nil {
		return nil, errors.New("manual feed dependencies are required")
	}
	return &ManualFeedService{Samples: samples, Embeddings: embeddings, Store: store, now: time.Now}, nil
}

// SubmitSpam 保存 manual sample，並同步寫入 embedding，避免資料庫保存完整原文。
func (s *ManualFeedService) SubmitSpam(ctx context.Context, input ManualFeedInput) (ManualSample, bool, error) {
	sample := input.Sample
	if sample.CreatedAt.IsZero() {
		sample.CreatedAt = s.now().UTC()
	}
	if sample.Label == "" {
		sample.Label = domain.AILabelSpam
	}
	if sample.Status == "" {
		sample.Status = ManualSampleStatusPendingEmbedding
	}
	created, inserted, err := s.Samples.CreateManualSample(ctx, sample)
	if err != nil {
		return ManualSample{}, false, fmt.Errorf("create manual sample: %w", err)
	}
	if !inserted && created.Status == ManualSampleStatusEmbeddingCompleted {
		return created, false, nil
	}
	embedding, err := s.Embeddings.Embed(ctx, domain.NewEmbeddingInput(input.Text, input.MaxTextRunes))
	if err != nil {
		_ = s.Samples.MarkManualSampleEmbeddingFailed(context.WithoutCancel(ctx), created.ID, errorCode(err), err.Error(), isRetryable(err))
		return created, inserted, fmt.Errorf("embed manual sample: %w", err)
	}
	if err := embedding.Validate(); err != nil {
		_ = s.Samples.MarkManualSampleEmbeddingFailed(context.WithoutCancel(ctx), created.ID, "invalid_embedding", err.Error(), false)
		return created, inserted, fmt.Errorf("validate manual sample embedding: %w", err)
	}
	expiresAt := s.now().UTC().Add(input.EmbeddingTTL)
	if input.EmbeddingTTL <= 0 {
		expiresAt = s.now().UTC().Add(168 * time.Hour)
	}
	if err := s.Store.SaveEmbedding(ctx, EmbeddingRecord{
		ContentFingerprint: created.ContentFingerprint,
		Embedding:          embedding,
		Label:              created.Label,
		Category:           created.Category,
		ReasonCode:         "manual_feedspam",
		CreatedAt:          s.now().UTC(),
		ExpiresAt:          expiresAt,
	}); err != nil {
		_ = s.Samples.MarkManualSampleEmbeddingFailed(context.WithoutCancel(ctx), created.ID, "embedding_store_failed", err.Error(), true)
		return created, inserted, fmt.Errorf("save manual sample embedding: %w", err)
	}
	if err := s.Samples.MarkManualSampleEmbeddingCompleted(ctx, created.ID, s.now().UTC()); err != nil {
		return created, inserted, fmt.Errorf("mark manual sample embedding completed: %w", err)
	}
	created.Status = ManualSampleStatusEmbeddingCompleted
	now := s.now().UTC()
	created.EmbeddedAt = &now
	return created, inserted, nil
}

func errorCode(err error) string {
	type coded interface {
		ErrorCode() string
	}
	var c coded
	if errors.As(err, &c) {
		return c.ErrorCode()
	}
	return "temporary_failure"
}

func isRetryable(err error) bool {
	type retryable interface {
		IsRetryable() bool
	}
	var r retryable
	return errors.As(err, &r) && r.IsRetryable()
}
