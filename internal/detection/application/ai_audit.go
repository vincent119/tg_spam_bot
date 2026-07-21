package application

import (
	"time"

	"github.com/vincent119/tg_spam_bot/internal/detection/domain"
)

// AIDetectionEvent 保存 AI 判定必要稽核欄位，不包含完整原文或 provider 原始 response。
type AIDetectionEvent struct {
	ChatID             int64
	UpdateID           int64
	MessageID          int64
	UserID             int64
	ContentFingerprint string
	Provider           string
	Model              string
	PromptVersion      string
	SchemaVersion      string
	RuleVersion        string
	CreatedAt          time.Time
}

// AIDetectionClaim 區分新取得的 AI 判定事件與既有處理結果。
type AIDetectionClaim struct {
	Acquired bool
	Existing *AIDetectionResult
}

// AIDetectionResult 是可安全保存與重送回讀的 AI 判定摘要。
type AIDetectionResult struct {
	Status      string
	Result      domain.AIClassifyResult
	ErrorCode   string
	ErrorText   string
	Retryable   bool
	CreatedAt   time.Time
	CompletedAt *time.Time
}

// AIDetectionCacheKey 是 AI 判定快取查詢鍵。
type AIDetectionCacheKey struct {
	ContentFingerprint string
	Provider           string
	Model              string
	PromptVersion      string
	RuleVersion        string
	CacheTTL           time.Duration
	Now                time.Time
}
