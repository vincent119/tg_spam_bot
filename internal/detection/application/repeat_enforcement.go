package application

import (
	"context"
	"errors"
	"fmt"
)

// RepeatStep 保存單一 Telegram 目標，避免重送時重新擴張清理範圍。
type RepeatStep struct {
	Key       string
	Kind      ActionKind
	MessageID int64
	UserID    int64
}

// RepeatExecution 是已保存的完整重複處置快照。
type RepeatExecution struct {
	EventID   string
	ChatID    int64
	UserID    int64
	Count     int
	Truncated bool
	Steps     []RepeatStep
}

// RepeatStepClaim 區分已成功、可執行與別的程序正在執行的項目。
type RepeatStepClaim struct {
	Acquired   bool
	Completed  bool
	LeaseToken string
}

// RepeatActionStore 先保存固定快照，再逐項記錄 Telegram 副作用。
type RepeatActionStore interface {
	CreateOrLoadRepeatExecution(ctx context.Context, event Event, plan RepeatPlan, skipCurrentDelete bool) (RepeatExecution, error)
	ClaimRepeatStep(ctx context.Context, key string) (RepeatStepClaim, error)
	CompleteRepeatStep(ctx context.Context, key, leaseToken string, result ActionResult) error
}

// RepeatAdminChecker 重新向 Telegram 查詢目標管理員狀態。
type RepeatAdminChecker interface {
	IsAdmin(ctx context.Context, chatID, userID int64) (bool, error)
}

// WithRepeatEnforcement 注入明示的重複政策與處置保護依賴。
func WithRepeatEnforcement(action RepeatAction, threshold int, store RepeatActionStore, trusted TrustedMembers, admins RepeatAdminChecker) ProcessorOption {
	return func(p *Processor) {
		p.repeatAction = action
		p.repeatThreshold = threshold
		p.repeatStore = store
		p.repeatTrusted = trusted
		p.repeatAdmins = admins
	}
}

func (p *Processor) executeRepeat(ctx context.Context, event Event, plan RepeatPlan, skipCurrentDelete bool) (bool, error) {
	if p.repeatStore == nil {
		return false, errors.New("repeat action store is required")
	}
	execution, err := p.repeatStore.CreateOrLoadRepeatExecution(ctx, event, plan, skipCurrentDelete)
	if err != nil {
		return false, fmt.Errorf("create repeat execution: %w", err)
	}
	if len(execution.Steps) == 0 {
		return false, nil
	}
	if p.repeatTrusted == nil || p.repeatAdmins == nil {
		return false, errors.New("repeat target guards are required")
	}
	trusted, _, err := p.repeatTrusted.IsExempt(ctx, execution.ChatID, execution.UserID)
	if err != nil {
		return false, fmt.Errorf("check repeat trusted member: %w", err)
	}
	if trusted {
		return false, nil
	}
	admin, err := p.repeatAdmins.IsAdmin(ctx, execution.ChatID, execution.UserID)
	if err != nil {
		return false, fmt.Errorf("check repeat admin: %w", err)
	}
	if admin {
		return false, nil
	}
	for _, step := range execution.Steps {
		claim, err := p.repeatStore.ClaimRepeatStep(ctx, step.Key)
		if err != nil {
			return true, fmt.Errorf("claim repeat action: %w", err)
		}
		if claim.Completed {
			continue
		}
		if !claim.Acquired {
			return true, errors.New("repeat action is in progress")
		}
		var actionErr error
		switch step.Kind {
		case ActionDelete:
			actionErr = p.telegram.DeleteMessage(ctx, execution.ChatID, step.MessageID)
		case ActionBan:
			actionErr = p.telegram.BanMember(ctx, execution.ChatID, step.UserID)
		default:
			actionErr = fmt.Errorf("unsupported repeat action %q", step.Kind)
		}
		result := ActionResult{Succeeded: actionErr == nil, Retryable: actionErr != nil, EndedAt: p.now().UTC()}
		if actionErr != nil {
			result.ErrorText = actionErr.Error()
		}
		if err := p.repeatStore.CompleteRepeatStep(ctx, step.Key, claim.LeaseToken, result); err != nil {
			return true, fmt.Errorf("record repeat action: %w", err)
		}
		if actionErr != nil {
			return true, fmt.Errorf("execute repeat action %s: %w", step.Key, actionErr)
		}
	}
	return true, nil
}
