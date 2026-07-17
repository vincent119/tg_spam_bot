package application

import "github.com/vincent119/tg_spam_bot/internal/detection/domain"

// PlanActions 將偵測結果及 30 天違規次數轉為可冪等執行的處置順序。
func PlanActions(result domain.Result, mode Mode, count int) []ActionKind {
	if !result.Spam || mode == ModeObserve {
		return nil
	}
	if mode == ModeDeleteOnly {
		return []ActionKind{ActionDelete}
	}
	if result.Severity == domain.SeverityCritical && result.Action == domain.ActionBan {
		return []ActionKind{ActionDelete, ActionBan}
	}
	switch count {
	case 1:
		return []ActionKind{ActionDelete, ActionWarn}
	case 2:
		return []ActionKind{ActionDelete, ActionMute10m}
	case 3:
		return []ActionKind{ActionDelete, ActionMute24h}
	default:
		return []ActionKind{ActionDelete, ActionBan}
	}
}
