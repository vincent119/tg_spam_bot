package application

import (
	"time"

	"github.com/vincent119/tg_spam_bot/internal/detection/domain"
)

// SemanticBlacklistCategory 是可人工維護的語意黑名單分類。
type SemanticBlacklistCategory struct {
	ID          string
	Name        string
	Description string
	Enabled     bool
	CreatedAt   time.Time
}

// SemanticBlacklistExample 是語意黑名單分類下的範例向量。
type SemanticBlacklistExample struct {
	ID        uint64
	Category  SemanticBlacklistCategory
	Embedding domain.EmbeddingResult
	Source    string
	CreatedAt time.Time
}

// SemanticBlacklistMatch 是語意黑名單命中摘要，不包含完整原文。
type SemanticBlacklistMatch struct {
	CategoryID   string
	CategoryName string
	Similarity   float64
	ReasonCode   string
}

// ObserveSemanticBlacklistMatches 將語意黑名單命中轉成輔助訊號。
func ObserveSemanticBlacklistMatches(matches []SemanticBlacklistMatch, threshold float64) SemanticObservation {
	observation := SemanticObservation{}
	for _, match := range matches {
		if match.Similarity >= threshold {
			observation.Signals = append(observation.Signals, "semantic_blacklist_match")
		}
	}
	observation.Signals = uniqueAISignals(observation.Signals)
	observation.CanEnforce = false
	return observation
}
