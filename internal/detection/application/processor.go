package application

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/vincent119/tg_spam_bot/internal/detection/domain"
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

// NewProcessor 組裝用例並複製雜湊金鑰，避免呼叫端修改敏感資料。
func NewProcessor(detector Detector, updates UpdateStore, exemptions ExemptionStore, behaviors BehaviorStore, violations ViolationStore, telegram Telegram, mode Mode, hashKey []byte) *Processor {
	return &Processor{detector: detector, updates: updates, exemptions: exemptions, behaviors: behaviors, violations: violations, telegram: telegram, mode: mode, hashKey: append([]byte(nil), hashKey...), now: time.Now}
}

// Process 以 update_id 收斂重送，且只在完整成功後標記更新完成。
func (p *Processor) Process(ctx context.Context, message domain.Message) error {
	claimed, err := p.updates.Claim(ctx, message.UpdateID)
	if err != nil {
		return fmt.Errorf("claim update: %w", err)
	}
	if !claimed {
		return nil
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
		return fmt.Errorf("check exemption: %w", err)
	}
	if exempt {
		completed = true
		return p.updates.Complete(ctx, message.UpdateID)
	}
	fingerprint := p.fingerprint(message.Text)
	signals, err := p.behaviors.Observe(ctx, message, fingerprint)
	if err != nil {
		return fmt.Errorf("observe behavior: %w", err)
	}
	result := p.detector.Detect(message, signals...)
	event := Event{ID: fmt.Sprintf("tg:%d", message.UpdateID), Message: domain.NewMessage(message), Fingerprint: fingerprint, Result: result, Mode: p.mode, CreatedAt: p.now().UTC()}

	if !result.Spam || p.mode == ModeObserve {
		if err := p.violations.RecordDetection(ctx, event); err != nil {
			return fmt.Errorf("record detection: %w", err)
		}
	} else {
		_, actions, err := p.violations.Create(ctx, event)
		if err != nil {
			return fmt.Errorf("create violation: %w", err)
		}
		if err := p.execute(ctx, event, actions); err != nil {
			return err
		}
	}
	if err := p.updates.Complete(ctx, message.UpdateID); err != nil {
		return fmt.Errorf("complete update: %w", err)
	}
	completed = true
	return nil
}

func (p *Processor) execute(ctx context.Context, event Event, actions []EnforcementAction) error {
	for _, action := range actions {
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
		if err != nil {
			return fmt.Errorf("execute action %s: %w", action.Kind, err)
		}
	}
	return nil
}

func (p *Processor) fingerprint(text string) string {
	h := hmac.New(sha256.New, p.hashKey)
	_, _ = h.Write([]byte(text))
	return hex.EncodeToString(h.Sum(nil))
}
