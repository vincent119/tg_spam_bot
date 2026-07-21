package application

import (
	"context"
	"time"

	"github.com/vincent119/tg_spam_bot/internal/detection/domain"
)

// Detector 定義 application 層需要的垃圾訊息判定能力。
type Detector interface {
	Detect(message domain.Message, extraSignals ...string) domain.Result
}

// AIClassifier 定義可替換的 AI provider 邊界，application 不處理 credential 型態。
type AIClassifier interface {
	Classify(ctx context.Context, input domain.AIClassifyInput) (domain.AIClassifyResult, error)
}

// EmbeddingProvider 定義可替換的 embedding provider 邊界，避免語意記憶耦合特定廠商。
type EmbeddingProvider interface {
	Embed(ctx context.Context, input domain.EmbeddingInput) (domain.EmbeddingResult, error)
}

// SemanticMemory 定義語意相似歷史案例查詢邊界，application 不耦合 pgvector 實作。
type SemanticMemory interface {
	SearchSimilar(ctx context.Context, embedding domain.EmbeddingResult, maxNeighbors int) ([]domain.SemanticMatch, error)
}

// EmbeddingStore 保存與查詢訊息 embedding。
type EmbeddingStore interface {
	SaveEmbedding(ctx context.Context, record EmbeddingRecord) error
	FindEmbeddingByFingerprint(ctx context.Context, fingerprint, provider, model, version string, dimensions int, now time.Time) (EmbeddingRecord, bool, error)
}

// AIDetectionStore 保存 AI 判定稽核、失敗摘要與內容快取。
type AIDetectionStore interface {
	ClaimAIDetection(ctx context.Context, event AIDetectionEvent) (AIDetectionClaim, error)
	CompleteAIDetection(ctx context.Context, event AIDetectionEvent, result domain.AIClassifyResult) error
	FailAIDetection(ctx context.Context, event AIDetectionEvent, result AIDetectionResult) error
	FindCachedAIDetection(ctx context.Context, key AIDetectionCacheKey) (AIDetectionResult, bool, error)
}

// ManualSampleStore 保存管理員提交的漏網垃圾樣本，供後續向量化流程使用。
type ManualSampleStore interface {
	CreateManualSample(ctx context.Context, sample ManualSample) (ManualSample, bool, error)
	PendingManualSamples(ctx context.Context, limit int) ([]ManualSample, error)
	MarkManualSampleEmbeddingCompleted(ctx context.Context, id uint64, embeddedAt time.Time) error
	MarkManualSampleEmbeddingFailed(ctx context.Context, id uint64, errorCode, errorText string, retryable bool) error
}

// SemanticBlacklistStore 保存與查詢語意黑名單分類與範例。
type SemanticBlacklistStore interface {
	SaveSemanticBlacklistCategory(ctx context.Context, category SemanticBlacklistCategory) error
	SaveSemanticBlacklistExample(ctx context.Context, example SemanticBlacklistExample) error
	SearchSemanticBlacklist(ctx context.Context, embedding domain.EmbeddingResult, threshold float64, maxNeighbors int) ([]SemanticBlacklistMatch, error)
}

// UpdateStore 以 update_id 保證 Telegram Webhook 重送不會重複處置。
type UpdateStore interface {
	Claim(ctx context.Context, updateID int64) (bool, error)
	Complete(ctx context.Context, updateID int64) error
	Release(ctx context.Context, updateID int64) error
}

// ExemptionStore 在偵測前辨識管理員與可信任成員。
type ExemptionStore interface {
	IsExempt(ctx context.Context, chatID, userID int64) (bool, string, error)
}

// BehaviorStore 計算頻率、重複內容與跨帳號協同等短期訊號。
type BehaviorStore interface {
	Observe(ctx context.Context, message domain.Message, fingerprint string) ([]string, error)
}

// ViolationStore 原子保存偵測、違規及冪等處置計畫。
type ViolationStore interface {
	Create(ctx context.Context, event Event) (violationCount int, actions []EnforcementAction, err error)
	RecordDetection(ctx context.Context, event Event) error
	CompleteAction(ctx context.Context, key string, result ActionResult) error
}

// Telegram 定義 application 層允許呼叫的最小 Bot API 集合。
type Telegram interface {
	DeleteMessage(ctx context.Context, chatID, messageID int64) error
	SendWarning(ctx context.Context, chatID, userID int64, text string) error
	RestrictMember(ctx context.Context, chatID, userID int64, until time.Time) error
	BanMember(ctx context.Context, chatID, userID int64) error
}

// Event 保存不含完整原文的偵測與處置稽核資料。
type Event struct {
	ID          string
	Message     domain.Message
	Fingerprint string
	Result      domain.Result
	Mode        Mode
	CreatedAt   time.Time
}

// ActionKind 定義可獨立記錄與重試的 Telegram 處置。
type ActionKind string

// 支援的 Telegram 處置種類。
const (
	ActionDelete  ActionKind = "delete"
	ActionWarn    ActionKind = "warn"
	ActionMute10m ActionKind = "mute_10m"
	ActionMute24h ActionKind = "mute_24h"
	ActionBan     ActionKind = "ban"
)

// EnforcementAction 使用唯一鍵收斂部分失敗後的重試。
type EnforcementAction struct {
	Key  string
	Kind ActionKind
}

// ActionResult 保存單項 Telegram API 呼叫結果。
type ActionResult struct {
	Succeeded bool
	Retryable bool
	ErrorCode string
	ErrorText string
	EndedAt   time.Time
}

// Mode 控制判定結果能否推進成實際處置。
type Mode string

// 支援的應用程式執行模式。
const (
	ModeObserve    Mode = "observe"
	ModeDeleteOnly Mode = "delete-only"
	ModeEnforce    Mode = "enforce"
)
