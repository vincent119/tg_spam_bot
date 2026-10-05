// Package application 協調 Telegram 管理指令的授權、冪等、資料與外部處置。
package application

import (
	"context"
	"time"

	"github.com/vincent119/tg_spam_bot/internal/command/domain"
	detectionapp "github.com/vincent119/tg_spam_bot/internal/detection/application"
	detectiondomain "github.com/vincent119/tg_spam_bot/internal/detection/domain"
)

// Telegram 提供管理指令所需的最小 Bot API 集合。
type Telegram interface {
	IsAdmin(ctx context.Context, chatID, userID int64) (bool, error)
	SendMessage(ctx context.Context, chatID, replyToMessageID int64, text string) error
	DeleteMessage(ctx context.Context, chatID, messageID int64) error
	DeleteMessages(ctx context.Context, chatID int64, messageIDs []int64) error
	RestrictMember(ctx context.Context, chatID, userID int64, until time.Time) error
	UnrestrictMember(ctx context.Context, chatID, userID int64) error
	BanMember(ctx context.Context, chatID, userID int64) error
	UnbanMember(ctx context.Context, chatID, userID int64) error
}

// DuplicateMessageFinder 只查詢同一群組、同一成員與同一內容指紋的近期訊息，供人工批次清除使用。
type DuplicateMessageFinder interface {
	FindDuplicateMessageIDs(ctx context.Context, chatID, userID, targetMessageID int64, since time.Time, limit int) ([]int64, error)
}

// TrustedMembers 只查詢資料庫可信任名單，不使用管理員快取。
type TrustedMembers interface {
	IsExempt(ctx context.Context, chatID, userID int64) (bool, string, error)
}

// WarningSummary 保存最近 30 天有效違規的來源摘要。
type WarningSummary struct {
	Total     int
	Manual    int
	Automatic int
}

// ExecutionStore 保存指令冪等、警告調整及稽核狀態。
type ExecutionStore interface {
	ClaimCommand(ctx context.Context, command domain.Command) (domain.Claim, error)
	CompleteCommand(ctx context.Context, command domain.Command, result domain.Result) error
	Warnings(ctx context.Context, chatID, userID int64, since time.Time) (WarningSummary, error)
	AddManualWarning(ctx context.Context, command domain.Command, reason domain.Reason, occurredAt time.Time) (WarningSummary, error)
	ClearWarnings(ctx context.Context, command domain.Command, reason domain.Reason, invalidatedAt time.Time) (int64, error)
}

// FeedSpamSubmitter 同步保存 `/feedspam` 樣本與 embedding，不保存完整原文。
type FeedSpamSubmitter interface {
	SubmitSpam(ctx context.Context, input detectionapp.ManualFeedInput) (detectionapp.ManualSample, bool, error)
}

// ManualFeedbackSubmitter 先保存可修訂標記，並將向量化狀態獨立回傳。
type ManualFeedbackSubmitter interface {
	Submit(ctx context.Context, input detectionapp.ManualFeedbackInput) (detectionapp.ManualFeedbackResult, error)
}

// FeedbackTarget 是可從本群偵測稽核確認的已刪訊息摘要。
type FeedbackTarget struct {
	MessageID          int64
	UserID             int64
	ContentFingerprint string
}

// FeedbackTargetFinder 只依同群的既存事件解析已刪訊息回饋對象。
type FeedbackTargetFinder interface {
	FindFeedbackTarget(ctx context.Context, chatID int64, eventID string) (FeedbackTarget, bool, error)
}

// FeedbackActionKind 是人工垃圾標記可能產生的個別外部動作。
type FeedbackActionKind string

const (
	// FeedbackActionDelete 表示刪除人工標記的目標訊息。
	FeedbackActionDelete FeedbackActionKind = "delete"
	// FeedbackActionBan 表示封鎖人工標記的目標成員。
	FeedbackActionBan FeedbackActionKind = "ban"
)

// FeedbackActionStore 在呼叫 Telegram 前保存動作意圖，並逐項記錄結果。
type FeedbackActionStore interface {
	PlanFeedbackActions(ctx context.Context, command domain.Command, kinds []FeedbackActionKind) error
	CompleteFeedbackAction(ctx context.Context, command domain.Command, kind FeedbackActionKind, succeeded, retryable bool, errorCode string) error
}

// DetectionPreviewer 不提供任何正式寫入與處置能力。
type DetectionPreviewer interface {
	Preview(ctx context.Context, message detectiondomain.Message) (detectionapp.PreviewResult, error)
}

// Clock 讓指令的 UTC 到期時間與 30 天視窗可決定性測試。
type Clock interface {
	Now() time.Time
}

// Limiter 限制公開指令在單一群組及成員的短期使用次數。
type Limiter interface {
	Allow(ctx context.Context, chatID, userID int64) (bool, error)
}
