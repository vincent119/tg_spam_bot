// Package application 協調 Telegram 管理指令的授權、冪等、資料與外部處置。
package application

import (
	"context"
	"time"

	"github.com/vincent119/tg_spam_bot/internal/command/domain"
)

// Telegram 提供管理指令所需的最小 Bot API 集合。
type Telegram interface {
	IsAdmin(ctx context.Context, chatID, userID int64) (bool, error)
	SendMessage(ctx context.Context, chatID, replyToMessageID int64, text string) error
	DeleteMessage(ctx context.Context, chatID, messageID int64) error
	RestrictMember(ctx context.Context, chatID, userID int64, until time.Time) error
	UnrestrictMember(ctx context.Context, chatID, userID int64) error
	BanMember(ctx context.Context, chatID, userID int64) error
	UnbanMember(ctx context.Context, chatID, userID int64) error
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

// Clock 讓指令的 UTC 到期時間與 30 天視窗可決定性測試。
type Clock interface {
	Now() time.Time
}

// Limiter 限制公開指令在單一群組及成員的短期使用次數。
type Limiter interface {
	Allow(ctx context.Context, chatID, userID int64) (bool, error)
}
