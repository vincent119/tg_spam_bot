package application

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/vincent119/tg_spam_bot/internal/command/domain"
	detectionapp "github.com/vincent119/tg_spam_bot/internal/detection/application"
	detectiondomain "github.com/vincent119/tg_spam_bot/internal/detection/domain"
)

const maxPreviewReplyRunes = 1200

func (h *Handler) handlePreview(ctx context.Context, command domain.Command, now time.Time) error {
	if h.preview == nil {
		return h.finishWithReply(ctx, command, "failed", "判定預覽功能尚未啟用。", string(domain.ErrorTemporary))
	}
	if command.Target == nil || command.TargetMessage <= 0 || strings.TrimSpace(command.TargetText) == "" {
		return h.finishWithReply(ctx, command, "invalid", "請回覆有文字或媒體說明的訊息。", string(domain.ErrorInvalidInput))
	}
	result, err := h.preview.Preview(ctx, detectiondomain.NewMessage(detectiondomain.Message{
		UpdateID: command.UpdateID, ChatID: command.ChatID, MessageID: command.TargetMessage,
		UserID: command.Target.ID, Text: command.TargetText, ReceivedAt: now,
	}))
	if err != nil {
		return h.fail(ctx, command, "判定預覽失敗", err)
	}
	return h.finishWithReply(ctx, command, "completed", formatPreview(result), "")
}

func formatPreview(result detectionapp.PreviewResult) string {
	rule := result.Rule
	lines := []string{
		"判定預覽（依目前規則重新計算；不執行處置）",
		fmt.Sprintf("規則：版本 %s；分類 %s；分數 %d／門檻 %d；垃圾判定 %s", previewField(rule.RuleVersion), previewField(rule.CategoryID), rule.Score, rule.Threshold, previewBool(rule.Spam)),
	}
	if len(rule.Matches) > 0 {
		ids := make([]string, 0, len(rule.Matches))
		for _, match := range rule.Matches {
			ids = append(ids, previewField(match.RuleID))
			if len(ids) == 5 {
				break
			}
		}
		lines = append(lines, "命中規則："+strings.Join(ids, "、"))
	} else {
		lines = append(lines, "命中規則：無")
	}
	if result.Repeat.Available {
		lines = append(lines, fmt.Sprintf("重複：已知 %d 則；假設納入後 %d 則／門檻 %d；是否觸發 %s", result.Repeat.ExistingCount, result.Repeat.ProjectedCount, result.Repeat.Threshold, previewBool(result.Repeat.WouldTrigger)))
		if result.Repeat.Truncated {
			lines = append(lines, "重複候選已截斷；可能無法列出全部可清理訊息")
		}
	} else {
		lines = append(lines, "重複：歷史計數不可得，不能當作零次")
	}
	lines = append(lines, "AI："+previewField(result.AIStatus)+"（"+previewField(result.AIReason)+"）；本次預覽未呼叫 AI")
	lines = append(lines, "歷史紀錄："+previewField(result.Historical.Status))
	if result.ExemptionKnown {
		lines = append(lines, "目標豁免："+previewBool(result.Exempt))
	} else {
		lines = append(lines, "目標豁免：無法確認")
	}
	if result.ActionKnown {
		actions := make([]string, 0, len(result.PlannedActions))
		for _, action := range result.PlannedActions {
			actions = append(actions, previewField(string(action)))
		}
		if len(actions) == 0 {
			actions = []string{"無"}
		}
		lines = append(lines, "預估動作："+strings.Join(actions, "、")+"（"+previewField(result.ActionReason)+"）")
	} else {
		lines = append(lines, "預估動作：無法確認（"+previewField(result.ActionReason)+"）")
	}
	lines = append(lines, "僅以回覆文字重算；若原訊息含 Telegram 實體連結，結構化訊號可能不完整。")
	message := []rune(strings.Join(lines, "\n"))
	if len(message) > maxPreviewReplyRunes {
		return string(message[:maxPreviewReplyRunes-1]) + "…"
	}
	return string(message)
}

func previewField(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "無"
	}
	clean := strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || r == '\u2028' || r == '\u2029' {
			return ' '
		}
		return r
	}, value)
	runes := []rune(clean)
	if len(runes) > 100 {
		return string(runes[:100]) + "…"
	}
	return clean
}

func previewBool(value bool) string {
	if value {
		return "是"
	}
	return "否"
}
