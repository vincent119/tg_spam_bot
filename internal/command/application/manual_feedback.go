package application

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/vincent119/tg_spam_bot/internal/command/domain"
	detectionapp "github.com/vincent119/tg_spam_bot/internal/detection/application"
	detectiondomain "github.com/vincent119/tg_spam_bot/internal/detection/domain"
)

func (h *Handler) handleManualFeedback(ctx context.Context, command domain.Command, now time.Time) error {
	if h.feedback == nil || h.feedbackActions == nil {
		return h.finishWithReply(ctx, command, "failed", "人工回饋功能尚未啟用。", string(domain.ErrorTemporary))
	}
	if command.Target == nil || command.TargetMessage <= 0 {
		return h.finishWithReply(ctx, command, "invalid", "缺少可查證的目標訊息。", string(domain.ErrorInvalidInput))
	}
	fingerprint := command.TargetFingerprint
	if fingerprint == "" && strings.TrimSpace(command.TargetText) != "" {
		fingerprint = h.fingerprint(command.TargetText)
	}
	if fingerprint == "" {
		return h.finishWithReply(ctx, command, "invalid", "目標沒有可提交的原文或稽核指紋。", string(domain.ErrorInvalidInput))
	}
	feedback := detectionapp.ManualFeedback{
		ChatID: command.ChatID, MessageID: command.TargetMessage, TargetUserID: command.Target.ID,
		OperatorID: command.Actor.ID, CommandUpdateID: command.UpdateID, ContentFingerprint: fingerprint,
		CreatedAt: now,
	}
	var spamArgs domain.SpamArgs
	if command.Name == domain.NameSpam {
		if strings.TrimSpace(command.TargetText) == "" {
			return h.finishWithReply(ctx, command, "invalid", "垃圾標記必須回覆有文字或說明的訊息。", string(domain.ErrorInvalidInput))
		}
		var err error
		spamArgs, err = domain.ParseSpamArgs(command.Args)
		if err != nil {
			return h.finishWithReply(ctx, command, "invalid", err.Error(), string(domain.ErrorInvalidInput))
		}
		feedback.Label = detectiondomain.AILabelSpam
		feedback.Category = spamArgs.Category
		feedback.Source = "spam_command"
	} else {
		parsed, err := domain.ParseHamArgs(command.Args)
		if err != nil {
			return h.finishWithReply(ctx, command, "invalid", err.Error(), string(domain.ErrorInvalidInput))
		}
		feedback.Label = detectiondomain.AILabelHam
		feedback.Category = "normal"
		feedback.Reason = string(parsed.Reason)
		feedback.Source = "ham_command"
	}
	result, err := h.feedback.Submit(ctx, detectionapp.ManualFeedbackInput{
		Feedback: feedback, Text: command.TargetText,
		MaxTextRunes: h.feedSpamMaxTextRunes, EmbeddingTTL: h.feedSpamEmbeddingTTL,
	})
	if err != nil {
		if result.Saved {
			return h.finishWithReply(ctx, command, "partial", "標記已保存，向量狀態未確認；尚未執行處置。", string(domain.ErrorTemporary))
		}
		return h.fail(ctx, command, "保存人工標記失敗，未執行處置", err)
	}
	if !result.Saved {
		return h.finishWithReply(ctx, command, "completed", "此筆人工標記已處理，未重複執行處置。", "")
	}
	parts := []string{"標記已保存", feedbackEmbeddingReply(result)}
	if command.Name == domain.NameHam {
		return h.finishWithReply(ctx, command, "completed", strings.Join(parts, "；")+"；不會自動解封或清除警告。", "")
	}
	if result.EmbeddingStatus == detectionapp.ManualFeedbackEmbeddingSuperseded {
		return h.finishWithReply(ctx, command, "partial", strings.Join(parts, "；")+"；較新標記已取代本次操作，未執行處置。", "")
	}
	if err := h.protectTarget(ctx, command); err != nil {
		return h.finishWithReply(ctx, command, "partial", strings.Join(parts, "；")+"；目標權限已變更，未執行處置。", string(domain.ErrorProtected))
	}
	kinds := []FeedbackActionKind{FeedbackActionDelete}
	if spamArgs.Action == domain.SpamActionBan {
		kinds = append(kinds, FeedbackActionBan)
	}
	if err := h.feedbackActions.PlanFeedbackActions(ctx, command, kinds); err != nil {
		return h.finishWithReply(ctx, command, "partial", strings.Join(parts, "；")+"；處置計畫未保存，未呼叫 Telegram。", string(domain.ErrorTemporary))
	}
	status := "completed"
	for _, kind := range kinds {
		message, uncertain := h.executeFeedbackAction(ctx, command, kind)
		parts = append(parts, message)
		if uncertain {
			status = "partial"
			break
		}
		if strings.Contains(message, "失敗") {
			status = "partial"
		}
	}
	return h.finishWithReply(ctx, command, status, strings.Join(parts, "；")+"。", "")
}

func feedbackEmbeddingReply(result detectionapp.ManualFeedbackResult) string {
	switch result.EmbeddingStatus {
	case detectionapp.ManualFeedbackEmbeddingCompleted:
		return "向量已建立"
	case detectionapp.ManualFeedbackEmbeddingFailed:
		return "向量化失敗（" + result.EmbeddingErrorCode + "）"
	case detectionapp.ManualFeedbackEmbeddingUnavailable:
		return "沒有原文，未建立向量"
	case detectionapp.ManualFeedbackEmbeddingDisabled:
		return "語意向量功能未啟用"
	case detectionapp.ManualFeedbackEmbeddingSuperseded:
		return "向量工作已被較新標記取代"
	default:
		return "向量狀態未確認"
	}
}

func (h *Handler) executeFeedbackAction(ctx context.Context, command domain.Command, kind FeedbackActionKind) (string, bool) {
	if kind == FeedbackActionBan {
		if err := h.protectTarget(ctx, command); err != nil {
			if saveErr := h.feedbackActions.CompleteFeedbackAction(ctx, command, kind, false, false, string(domain.ErrorProtected)); saveErr != nil {
				return "封鎖保護檢查結果未確認", true
			}
			return "封鎖已略過：目標保護狀態變更", true
		}
	}
	var actionErr error
	switch kind {
	case FeedbackActionDelete:
		actionErr = h.telegram.DeleteMessage(ctx, command.ChatID, command.TargetMessage)
	case FeedbackActionBan:
		actionErr = h.telegram.BanMember(ctx, command.ChatID, command.Target.ID)
	default:
		return "不支援的處置", true
	}
	code, retryable := "", false
	if actionErr != nil {
		code, retryable = classifyError(actionErr)
	}
	if err := h.feedbackActions.CompleteFeedbackAction(ctx, command, kind, actionErr == nil, retryable, code); err != nil {
		return fmt.Sprintf("%s 結果未確認", kind), true
	}
	if actionErr != nil {
		return fmt.Sprintf("%s 失敗（%s）", kind, code), false
	}
	if kind == FeedbackActionDelete {
		return "訊息已刪除", false
	}
	return "成員已封鎖", false
}
