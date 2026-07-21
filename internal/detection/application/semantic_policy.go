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
	MaxTextRunes            int
	MaxNeighbors            int
	SpamSimilarityThreshold float64
	HamSimilarityThreshold  float64
}

// Observe 先產生 embedding，再查詢語意記憶庫並轉成輔助訊號。
func (p SemanticLookupPolicy) Observe(ctx context.Context, text string) (SemanticObservation, error) {
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
	return ObserveSemanticMatches(matches, p.SpamSimilarityThreshold, p.HamSimilarityThreshold), nil
}
