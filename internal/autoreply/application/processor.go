package application

import (
	"context"
	"errors"
	"time"

	detectiondomain "github.com/vincent119/tg_spam_bot/internal/detection/domain"
	"github.com/vincent119/zlogger"
)

// Store 保存自動回覆副作用的冪等與稽核狀態。
type Store interface {
	ClaimAutoReply(ctx context.Context, event Event) (Claim, error)
	CompleteAutoReply(ctx context.Context, event Event) error
	FailAutoReply(ctx context.Context, event Event, result Result) error
}

// Telegram 定義自動回覆需要的最小 Telegram API。
type Telegram interface {
	SendMessage(ctx context.Context, chatID, replyToMessageID int64, text string) error
}

// Claim 區分新取得的回覆事件與既有處理結果。
type Claim struct {
	Acquired bool
	Existing *Result
}

// Event 保存不含完整原文的自動回覆執行資訊。
type Event struct {
	ChatID    int64
	UpdateID  int64
	MessageID int64
	UserID    int64
	RuleID    string
	CreatedAt time.Time
}

// Result 是可安全保存的自動回覆結果。
type Result struct {
	Status    string
	ErrorCode string
	ErrorText string
	Retryable bool
}

// Processor 協調自動回覆比對、冪等占用與 Telegram 回覆。
type Processor struct {
	matcher  Matcher
	store    Store
	telegram Telegram
	now      func() time.Time
}

// NewProcessor 建立自動回覆處理器。
func NewProcessor(matcher Matcher, store Store, telegram Telegram) (*Processor, error) {
	if store == nil || telegram == nil {
		return nil, errors.New("auto reply store and telegram are required")
	}
	return &Processor{matcher: matcher, store: store, telegram: telegram, now: time.Now}, nil
}

// Process 對非垃圾的一般訊息嘗試送出最多一則自動回覆。
func (p *Processor) Process(ctx context.Context, message detectiondomain.Message) error {
	match, ok := p.matcher.Match(message)
	if !ok {
		return nil
	}
	event := Event{ChatID: message.ChatID, UpdateID: message.UpdateID, MessageID: message.MessageID, UserID: message.UserID, RuleID: match.RuleID, CreatedAt: p.now().UTC()}
	claim, err := p.store.ClaimAutoReply(ctx, event)
	if err != nil {
		return err
	}
	if !claim.Acquired {
		logAutoReply(ctx, "自動回覆已處理，略過重複請求", event, Result{Status: "duplicate"})
		return nil
	}
	if err := p.telegram.SendMessage(ctx, message.ChatID, message.MessageID, match.Reply); err != nil {
		result := classifyError(err)
		_ = p.store.FailAutoReply(context.WithoutCancel(ctx), event, result)
		logAutoReply(ctx, "自動回覆失敗", event, result)
		return err
	}
	if err := p.store.CompleteAutoReply(ctx, event); err != nil {
		return err
	}
	logAutoReply(ctx, "完成自動回覆", event, Result{Status: "completed"})
	return nil
}

func classifyError(err error) Result {
	result := Result{Status: "failed", ErrorText: err.Error(), Retryable: false}
	type retryable interface {
		IsRetryable() bool
	}
	type coded interface {
		ErrorCode() string
	}
	var r retryable
	if errors.As(err, &r) {
		result.Retryable = r.IsRetryable()
	}
	var c coded
	if errors.As(err, &c) {
		result.ErrorCode = c.ErrorCode()
	}
	return result
}

func logAutoReply(ctx context.Context, message string, event Event, result Result) {
	zlogger.DebugContext(ctx, message,
		zlogger.String("subsystem", "auto_reply"),
		zlogger.Int64("update_id", event.UpdateID),
		zlogger.Int64("chat_id", event.ChatID),
		zlogger.Int64("message_id", event.MessageID),
		zlogger.Int64("user_id", event.UserID),
		zlogger.String("rule_id", event.RuleID),
		zlogger.String("status", result.Status),
		zlogger.String("error_code", result.ErrorCode),
		zlogger.Bool("retryable", result.Retryable),
	)
}
