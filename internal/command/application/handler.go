package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/vincent119/tg_spam_bot/internal/command/domain"
)

// Handler 執行固定管理指令，並將人工操作與自動偵測模式分離。
type Handler struct {
	telegram Telegram
	trusted  TrustedMembers
	store    ExecutionStore
	limiter  Limiter
	botID    int64
	now      func() time.Time
}

// NewHandler 驗證管理指令的所有必要依賴。
func NewHandler(telegram Telegram, trusted TrustedMembers, store ExecutionStore, limiter Limiter, botID int64) (*Handler, error) {
	if telegram == nil || trusted == nil || store == nil || limiter == nil || botID <= 0 {
		return nil, errors.New("管理指令依賴與 bot ID 不得為空")
	}
	return &Handler{telegram: telegram, trusted: trusted, store: store, limiter: limiter, botID: botID, now: time.Now}, nil
}

// Handle 先以 update ID 收斂重送，再依固定權限矩陣執行一次指令。
func (h *Handler) Handle(ctx context.Context, command domain.Command) error {
	claimed, err := h.store.ClaimCommand(ctx, command)
	if err != nil {
		return fmt.Errorf("占用管理指令：%w", err)
	}
	if !claimed {
		return nil
	}
	definition, known := domain.LookupDefinition(command.Name)
	if !known {
		return h.finishWithReply(ctx, command, "ignored", "未知指令，請使用 /help 查看說明。", "")
	}
	if !definition.AdminOnly {
		if strings.TrimSpace(command.Args) != "" {
			return h.finishWithReply(ctx, command, "invalid", fmt.Sprintf("用法：%s。", definition.Usage), "")
		}
		allowed, err := h.limiter.Allow(ctx, command.ChatID, command.Actor.ID)
		if err != nil {
			return h.fail(ctx, command, "暫時無法檢查指令頻率", err)
		}
		if !allowed {
			return h.store.CompleteCommand(ctx, command, "rate_limited", "", "")
		}
		return h.handlePublic(ctx, command)
	}
	admin, err := h.telegram.IsAdmin(ctx, command.ChatID, command.Actor.ID)
	if err != nil {
		return h.fail(ctx, command, "暫時無法確認管理員權限", err)
	}
	if !admin {
		return h.finishWithReply(ctx, command, "denied", "此指令僅限群組管理員使用。", "")
	}
	if err := validateAdminArgs(command); err != nil {
		return h.finishWithReply(ctx, command, "invalid", err.Error(), err.Error())
	}
	if err := h.resolveTarget(&command, definition); err != nil {
		return h.finishWithReply(ctx, command, "invalid", err.Error(), "")
	}
	if command.Target != nil {
		if err := h.protectTarget(ctx, command); err != nil {
			return h.finishWithReply(ctx, command, "denied", err.Error(), "")
		}
	}
	return h.handleAdmin(ctx, command)
}

func validateAdminArgs(command domain.Command) error {
	args := strings.TrimSpace(command.Args)
	switch command.Name {
	case domain.NameWarnings, domain.NameDelete, domain.NameUnmute:
		if args != "" {
			definition, _ := domain.LookupDefinition(command.Name)
			return fmt.Errorf("用法：%s。", definition.Usage)
		}
	case domain.NameUnban:
		if command.Target != nil && args != "" {
			return errors.New("回覆成員訊息時不得再提供 user ID")
		}
	}
	return nil
}

func (h *Handler) handlePublic(ctx context.Context, command domain.Command) error {
	switch command.Name {
	case domain.NamePing:
		return h.finishWithReply(ctx, command, "completed", "機器人運作正常。", "")
	case domain.NameID:
		userID := command.Actor.ID
		if command.Target != nil && command.Target.ID != 0 {
			userID = command.Target.ID
		}
		return h.finishWithReply(ctx, command, "completed", fmt.Sprintf("群組 ID：%d\n使用者 ID：%d", command.ChatID, userID), "")
	case domain.NameHelp:
		admin, _ := h.telegram.IsAdmin(ctx, command.ChatID, command.Actor.ID)
		lines := []string{"可用指令："}
		for _, definition := range domain.Definitions() {
			if definition.AdminOnly && !admin {
				continue
			}
			lines = append(lines, definition.Usage+"："+definition.Description)
		}
		return h.finishWithReply(ctx, command, "completed", strings.Join(lines, "\n"), "")
	default:
		return h.finishWithReply(ctx, command, "ignored", "未知指令，請使用 /help 查看說明。", "")
	}
}

func (h *Handler) handleAdmin(ctx context.Context, command domain.Command) error {
	now := h.now().UTC()
	targetID := command.Target.ID
	switch command.Name {
	case domain.NameWarnings:
		summary, err := h.store.Warnings(ctx, command.ChatID, targetID, now.Add(-30*24*time.Hour))
		if err != nil {
			return h.fail(ctx, command, "查詢警告失敗", err)
		}
		text := fmt.Sprintf("使用者 %d 最近 30 天警告：%d 次（人工 %d、自動 %d）", targetID, summary.Total, summary.Manual, summary.Automatic)
		return h.finishWithReply(ctx, command, "completed", text, "")
	case domain.NameWarn:
		reason, err := domain.ParseReason(command.Args)
		if err != nil {
			return h.finishWithReply(ctx, command, "invalid", err.Error(), "")
		}
		summary, err := h.store.AddManualWarning(ctx, command, reason, now)
		if err != nil {
			return h.fail(ctx, command, "新增警告失敗", err)
		}
		return h.finishWithReply(ctx, command, "completed", fmt.Sprintf("已警告使用者 %d，目前最近 30 天共 %d 次。", targetID, summary.Total), "")
	case domain.NameClearWarn:
		reason, err := domain.ParseReason(command.Args)
		if err != nil {
			return h.finishWithReply(ctx, command, "invalid", err.Error(), "")
		}
		count, err := h.store.ClearWarnings(ctx, command, reason, now)
		if err != nil {
			return h.fail(ctx, command, "清除警告失敗", err)
		}
		return h.finishWithReply(ctx, command, "completed", fmt.Sprintf("已失效使用者 %d 的 %d 筆警告紀錄。", targetID, count), "")
	case domain.NameDelete:
		if err := h.telegram.DeleteMessage(ctx, command.ChatID, command.TargetMessage); err != nil {
			return h.fail(ctx, command, "刪除訊息失敗", err)
		}
		return h.finishWithReply(ctx, command, "completed", "已刪除訊息。", "")
	case domain.NameMute:
		durationText, reasonText, ok := strings.Cut(command.Args, " ")
		if !ok {
			durationText = command.Args
		}
		duration, err := domain.ParseDuration(durationText)
		if err != nil {
			return h.finishWithReply(ctx, command, "invalid", err.Error()+"，用法：/mute 10m [原因]。", "")
		}
		if _, err := domain.ParseReason(reasonText); err != nil {
			return h.finishWithReply(ctx, command, "invalid", err.Error(), "")
		}
		if err := h.telegram.RestrictMember(ctx, command.ChatID, targetID, now.Add(duration)); err != nil {
			return h.fail(ctx, command, "禁言失敗", err)
		}
		return h.finishWithReply(ctx, command, "completed", fmt.Sprintf("已禁言使用者 %d，時間 %s。", targetID, durationText), "")
	case domain.NameUnmute:
		if strings.TrimSpace(command.Args) != "" {
			return h.finishWithReply(ctx, command, "invalid", "用法：/unmute（回覆成員訊息）。", "")
		}
		if err := h.telegram.UnrestrictMember(ctx, command.ChatID, targetID); err != nil {
			return h.fail(ctx, command, "解除禁言失敗", err)
		}
		return h.finishWithReply(ctx, command, "completed", fmt.Sprintf("已解除使用者 %d 的禁言。", targetID), "")
	case domain.NameBan:
		if _, err := domain.ParseReason(command.Args); err != nil {
			return h.finishWithReply(ctx, command, "invalid", err.Error(), "")
		}
		if err := h.telegram.BanMember(ctx, command.ChatID, targetID); err != nil {
			return h.fail(ctx, command, "封鎖失敗", err)
		}
		return h.finishWithReply(ctx, command, "completed", fmt.Sprintf("已封鎖使用者 %d。", targetID), "")
	case domain.NameUnban:
		if err := h.telegram.UnbanMember(ctx, command.ChatID, targetID); err != nil {
			return h.fail(ctx, command, "解除封鎖失敗", err)
		}
		return h.finishWithReply(ctx, command, "completed", fmt.Sprintf("已解除使用者 %d 的封鎖。", targetID), "")
	default:
		return h.finishWithReply(ctx, command, "ignored", "未知指令，請使用 /help 查看說明。", "")
	}
}

func (h *Handler) resolveTarget(command *domain.Command, definition domain.Definition) error {
	if command.Name == domain.NameUnban && command.Target == nil {
		id, err := domain.ParseUserID(command.Args)
		if err != nil {
			return errors.New("用法：/unban <user_id> 或回覆成員訊息")
		}
		command.Target = &domain.User{ID: id}
		command.Args = ""
	}
	if definition.RequiresReply && (command.Target == nil || command.Target.ID == 0) {
		return fmt.Errorf("此指令必須回覆目標成員的訊息，用法：%s", definition.Usage)
	}
	return nil
}

func (h *Handler) protectTarget(ctx context.Context, command domain.Command) error {
	target := command.Target
	if target.ID == h.botID || target.IsBot {
		return errors.New("不得處置 Bot 帳號")
	}
	admin, err := h.telegram.IsAdmin(ctx, command.ChatID, target.ID)
	if err != nil {
		return errors.New("暫時無法確認目標權限")
	}
	if admin {
		return errors.New("不得處置群組管理員")
	}
	trusted, _, err := h.trusted.IsExempt(ctx, command.ChatID, target.ID)
	if err != nil {
		return errors.New("暫時無法確認可信任成員")
	}
	if trusted {
		return errors.New("不得處置可信任成員")
	}
	return nil
}

func (h *Handler) finishWithReply(ctx context.Context, command domain.Command, status, text, errorText string) error {
	if text != "" {
		if err := h.telegram.SendMessage(ctx, command.ChatID, command.MessageID, text); err != nil {
			return h.fail(ctx, command, "傳送指令回應失敗", err)
		}
	}
	if err := h.store.CompleteCommand(ctx, command, status, text, errorText); err != nil {
		return fmt.Errorf("完成管理指令：%w", err)
	}
	return nil
}

func (h *Handler) fail(ctx context.Context, command domain.Command, public string, cause error) error {
	_ = h.store.CompleteCommand(ctx, command, "failed", "", public)
	_ = h.telegram.SendMessage(ctx, command.ChatID, command.MessageID, public+"，請稍後再試。")
	return fmt.Errorf("%s：%w", public, cause)
}
