package application

import (
	"context"
	"errors"
	"fmt"

	"github.com/vincent119/tg_spam_bot/internal/detection/domain"
)

// SemanticLookupPolicy 控制 AI classifier 前的語意相似查詢。
type SemanticLookupPolicy struct {
	Embeddings              EmbeddingProvider
	Memory                  SemanticMemory
	ScopedMemory            ScopedSemanticMemory
	Feedbacks               SemanticFeedbackReader
	MaxTextRunes            int
	MaxNeighbors            int
	SpamSimilarityThreshold float64
	HamSimilarityThreshold  float64
}

// ScopedSemanticMemory 僅提供指定群組仍有效的人工向量證據。
type ScopedSemanticMemory interface {
	SearchScopedManualSimilar(ctx context.Context, chatID int64, embedding domain.EmbeddingResult, maxNeighbors int) ([]domain.SemanticMatch, error)
}

// SemanticFeedbackReader 用本群目前有效標記排除已被修正的全域垃圾向量。
type SemanticFeedbackReader interface {
	FindEffectiveFeedback(ctx context.Context, chatID int64, fingerprint string) (ManualFeedbackEvidence, error)
}

// Observe 保留原有全域語意查詢契約，供不具群組資訊的呼叫者使用。
func (p SemanticLookupPolicy) Observe(ctx context.Context, text string) (SemanticObservation, error) {
	return p.ObserveForChat(ctx, 0, text)
}

// ObserveForChat 合併全域樣本與同群有效人工修訂，不讀取其他群組的標記。
func (p SemanticLookupPolicy) ObserveForChat(ctx context.Context, chatID int64, text string) (SemanticObservation, error) {
	if p.Embeddings == nil || p.Memory == nil {
		return SemanticObservation{}, errors.New("semantic lookup dependencies are required")
	}
	embedding, err := p.Embeddings.Embed(ctx, domain.NewEmbeddingInput(text, p.MaxTextRunes))
	if err != nil {
		return SemanticObservation{}, fmt.Errorf("embed semantic lookup input: %w", err)
	}
	if err := embedding.Validate(); err != nil {
		return SemanticObservation{}, fmt.Errorf("validate semantic lookup embedding: %w", err)
	}
	matches, err := p.Memory.SearchSimilar(ctx, embedding, p.MaxNeighbors)
	if err != nil {
		return SemanticObservation{}, fmt.Errorf("search semantic memory: %w", err)
	}
	if chatID != 0 && p.Feedbacks != nil {
		filtered := make([]domain.SemanticMatch, 0, len(matches))
		for _, match := range matches {
			if match.Label == domain.AILabelSpam {
				evidence, err := p.Feedbacks.FindEffectiveFeedback(ctx, chatID, match.SourceEventID)
				if err != nil {
					return SemanticObservation{}, fmt.Errorf("find scoped feedback for semantic match: %w", err)
				}
				if evidence.Present && (evidence.Conflict || evidence.Label == domain.AILabelHam) {
					continue
				}
			}
			filtered = append(filtered, match)
		}
		matches = filtered
	}
	if chatID != 0 && p.ScopedMemory != nil {
		scoped, err := p.ScopedMemory.SearchScopedManualSimilar(ctx, chatID, embedding, p.MaxNeighbors)
		if err != nil {
			return SemanticObservation{}, fmt.Errorf("search scoped manual semantic memory: %w", err)
		}
		matches = append(matches, scoped...)
	}
	return ObserveSemanticMatches(matches, p.SpamSimilarityThreshold, p.HamSimilarityThreshold), nil
}
