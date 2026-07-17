package application

import (
	"context"
	"time"

	"github.com/vincent119/tg_spam_bot/internal/detection/domain"
)

type Detector interface {
	Detect(message domain.Message, extraSignals ...string) domain.Result
}

type UpdateStore interface {
	Claim(ctx context.Context, updateID int64) (bool, error)
	Complete(ctx context.Context, updateID int64) error
	Release(ctx context.Context, updateID int64) error
}

type ExemptionStore interface {
	IsExempt(ctx context.Context, chatID, userID int64) (bool, string, error)
}

type BehaviorStore interface {
	Observe(ctx context.Context, message domain.Message, fingerprint string) ([]string, error)
}

type ViolationStore interface {
	Create(ctx context.Context, event Event) (violationCount int, actions []EnforcementAction, err error)
	RecordDetection(ctx context.Context, event Event) error
	CompleteAction(ctx context.Context, key string, result ActionResult) error
}

type Telegram interface {
	DeleteMessage(ctx context.Context, chatID, messageID int64) error
	SendWarning(ctx context.Context, chatID, userID int64, text string) error
	RestrictMember(ctx context.Context, chatID, userID int64, until time.Time) error
	BanMember(ctx context.Context, chatID, userID int64) error
}

type Event struct {
	ID          string
	Message     domain.Message
	Fingerprint string
	Result      domain.Result
	Mode        Mode
	CreatedAt   time.Time
}

type ActionKind string

const (
	ActionDelete  ActionKind = "delete"
	ActionWarn    ActionKind = "warn"
	ActionMute10m ActionKind = "mute_10m"
	ActionMute24h ActionKind = "mute_24h"
	ActionBan     ActionKind = "ban"
)

type EnforcementAction struct {
	Key  string
	Kind ActionKind
}

type ActionResult struct {
	Succeeded bool
	Retryable bool
	ErrorCode string
	ErrorText string
	EndedAt   time.Time
}

type Mode string

const (
	ModeObserve    Mode = "observe"
	ModeDeleteOnly Mode = "delete-only"
	ModeEnforce    Mode = "enforce"
)
