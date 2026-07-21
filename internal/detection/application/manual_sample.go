package application

import (
	"time"

	"github.com/vincent119/tg_spam_bot/internal/detection/domain"
)

const (
	ManualSampleStatusPendingEmbedding   = "pending_embedding"
	ManualSampleStatusEmbeddingCompleted = "embedding_completed"
	ManualSampleStatusEmbeddingFailed    = "embedding_failed"
)

// ManualSample 是管理員提交的漏網樣本摘要，不包含完整原文。
type ManualSample struct {
	ID                 uint64
	ChatID             int64
	MessageID          int64
	TargetUserID       int64
	OperatorID         int64
	ContentFingerprint string
	Label              domain.AILabel
	Category           string
	Source             string
	Status             string
	CreatedAt          time.Time
	EmbeddedAt         *time.Time
	ErrorCode          string
	ErrorText          string
	Retryable          bool
}
