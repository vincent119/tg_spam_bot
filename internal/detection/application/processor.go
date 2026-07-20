package application

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/vincent119/tg_spam_bot/internal/detection/domain"
	"github.com/vincent119/zlogger"
)

// Processor 協調冪等占用、豁免、偵測、違規保存及 Telegram 處置。
type Processor struct {
	detector   Detector
	updates    UpdateStore
	exemptions ExemptionStore
	behaviors  BehaviorStore
	violations ViolationStore
	telegram   Telegram
	mode       Mode
	hashKey    []byte
	now        func() time.Time
}

// ProcessResult 描述一般訊息經垃圾偵測流程後是否仍可接續其他非垃圾流程。
type ProcessResult struct {
	Duplicate bool
	Exempt    bool
	Spam      bool
}

// NewProcessor 組裝用例並複製雜湊金鑰，避免呼叫端修改敏感資料。
func NewProcessor(detector Detector, updates UpdateStore, exemptions ExemptionStore, behaviors BehaviorStore, violations ViolationStore, telegram Telegram, mode Mode, hashKey []byte) *Processor {
	return &Processor{detector: detector, updates: updates, exemptions: exemptions, behaviors: behaviors, violations: violations, telegram: telegram, mode: mode, hashKey: append([]byte(nil), hashKey...), now: time.Now}
}

// Process 以 update_id 收斂重送，且只在完整成功後標記更新完成。
func (p *Processor) Process(ctx context.Context, message domain.Message) (ProcessResult, error) {
	claimed, err := p.updates.Claim(ctx, message.UpdateID)
	if err != nil {
		return ProcessResult{}, fmt.Errorf("claim update: %w", err)
	}
	if !claimed {
		zlogger.DebugContext(ctx, "Telegram 更新已處理，略過重複請求",
			zlogger.String("subsystem", "detection"),
			zlogger.Int64("update_id", message.UpdateID),
			zlogger.Int64("chat_id", message.ChatID),
		)
		return ProcessResult{Duplicate: true}, nil
	}
	completed := false
	// 失敗時釋放 processing 占用，讓 Telegram 重送可安全接續處理。
	defer func() {
		if !completed {
			_ = p.updates.Release(context.WithoutCancel(ctx), message.UpdateID)
		}
	}()

	exempt, _, err := p.exemptions.IsExempt(ctx, message.ChatID, message.UserID)
	if err != nil {
		return ProcessResult{}, fmt.Errorf("check exemption: %w", err)
	}
	if exempt {
		zlogger.DebugContext(ctx, "成員命中豁免規則，略過垃圾訊息偵測",
			zlogger.String("subsystem", "detection"),
			zlogger.Int64("update_id", message.UpdateID),
			zlogger.Int64("chat_id", message.ChatID),
			zlogger.Int64("user_id", message.UserID),
		)
		completed = true
		return ProcessResult{Exempt: true}, p.updates.Complete(ctx, message.UpdateID)
	}
	fingerprint := p.fingerprint(message.Text)
	signals, err := p.behaviors.Observe(ctx, message, fingerprint)
	if err != nil {
		return ProcessResult{}, fmt.Errorf("observe behavior: %w", err)
	}
	result := p.detector.Detect(message, signals...)
	event := Event{ID: fmt.Sprintf("tg:%d", message.UpdateID), Message: domain.NewMessage(message), Fingerprint: fingerprint, Result: result, Mode: p.mode, CreatedAt: p.now().UTC()}
	logDetectionResult(ctx, event)

	if !result.Spam || p.mode == ModeObserve {
		if err := p.violations.RecordDetection(ctx, event); err != nil {
			return ProcessResult{}, fmt.Errorf("record detection: %w", err)
		}
	} else {
		_, actions, err := p.violations.Create(ctx, event)
		if err != nil {
			return ProcessResult{}, fmt.Errorf("create violation: %w", err)
		}
		if err := p.execute(ctx, event, actions); err != nil {
			return ProcessResult{}, err
		}
	}
	if err := p.updates.Complete(ctx, message.UpdateID); err != nil {
		return ProcessResult{}, fmt.Errorf("complete update: %w", err)
	}
	completed = true
	return ProcessResult{Spam: result.Spam}, nil
}

func (p *Processor) execute(ctx context.Context, event Event, actions []EnforcementAction) error {
	for _, action := range actions {
		zlogger.DebugContext(ctx, "開始執行 Telegram 處置",
			zlogger.String("subsystem", "enforcement"),
			zlogger.String("event_id", event.ID),
			zlogger.Int64("chat_id", event.Message.ChatID),
			zlogger.Int64("message_id", event.Message.MessageID),
			zlogger.String("action", string(action.Kind)),
		)
		var err error
		switch action.Kind {
		case ActionDelete:
			err = p.telegram.DeleteMessage(ctx, event.Message.ChatID, event.Message.MessageID)
		case ActionWarn:
			err = p.telegram.SendWarning(ctx, event.Message.ChatID, event.Message.UserID, "請勿發送廣告或垃圾訊息")
		case ActionMute10m:
			err = p.telegram.RestrictMember(ctx, event.Message.ChatID, event.Message.UserID, p.now().Add(10*time.Minute))
		case ActionMute24h:
			err = p.telegram.RestrictMember(ctx, event.Message.ChatID, event.Message.UserID, p.now().Add(24*time.Hour))
		case ActionBan:
			err = p.telegram.BanMember(ctx, event.Message.ChatID, event.Message.UserID)
		default:
			err = fmt.Errorf("unsupported action %q", action.Kind)
		}
		result := ActionResult{Succeeded: err == nil, Retryable: err != nil, EndedAt: p.now().UTC()}
		if err != nil {
			result.ErrorText = err.Error()
		}
		if recordErr := p.violations.CompleteAction(ctx, action.Key, result); recordErr != nil {
			return fmt.Errorf("record action %s: %w", action.Kind, recordErr)
		}
		zlogger.DebugContext(ctx, "完成 Telegram 處置",
			zlogger.String("subsystem", "enforcement"),
			zlogger.String("event_id", event.ID),
			zlogger.String("action", string(action.Kind)),
			zlogger.Bool("succeeded", err == nil),
			zlogger.Bool("retryable", result.Retryable),
		)
		if err != nil {
			return fmt.Errorf("execute action %s: %w", action.Kind, err)
		}
	}
	return nil
}

// logDetectionResult 只記錄判定摘要，避免將訊息原文或敏感內容寫入日誌。
func logDetectionResult(ctx context.Context, event Event) {
	zlogger.DebugContext(ctx, "完成垃圾訊息判定",
		zlogger.String("subsystem", "detection"),
		zlogger.String("event_id", event.ID),
		zlogger.Int64("update_id", event.Message.UpdateID),
		zlogger.Int64("chat_id", event.Message.ChatID),
		zlogger.Int64("message_id", event.Message.MessageID),
		zlogger.String("category_id", event.Result.CategoryID),
		zlogger.String("severity", string(event.Result.Severity)),
		zlogger.Int("score", event.Result.Score),
		zlogger.Int("threshold", event.Result.Threshold),
		zlogger.Bool("is_spam", event.Result.Spam),
		zlogger.String("action", string(event.Result.Action)),
		zlogger.String("mode", string(event.Mode)),
		zlogger.String("rule_version", event.Result.RuleVersion),
		zlogger.Int("match_count", len(event.Result.Matches)),
		zlogger.Strings("signals", event.Result.Signals),
	)
}

func (p *Processor) fingerprint(text string) string {
	h := hmac.New(sha256.New, p.hashKey)
	_, _ = h.Write([]byte(text))
	return hex.EncodeToString(h.Sum(nil))
}
