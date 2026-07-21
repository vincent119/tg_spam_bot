package application

import (
	"time"

	"github.com/vincent119/tg_spam_bot/internal/detection/domain"
)

// EmbeddingRecord 是可安全保存的訊息向量摘要，不包含完整原文。
type EmbeddingRecord struct {
	ContentFingerprint string
	Embedding          domain.EmbeddingResult
	Label              domain.AILabel
	Category           string
	ReasonCode         string
	CreatedAt          time.Time
	ExpiresAt          time.Time
}
