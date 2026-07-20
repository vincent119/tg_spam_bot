package telegram

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/vincent119/zlogger"

	commanddomain "github.com/vincent119/tg_spam_bot/internal/command/domain"
	detectionapp "github.com/vincent119/tg_spam_bot/internal/detection/application"
	"github.com/vincent119/tg_spam_bot/internal/detection/domain"
)

const secretHeader = "X-Telegram-Bot-Api-Secret-Token" //nolint:gosec // 這是 Telegram 公開 Header 名稱，不是秘密值。

// MessageProcessor 定義 Webhook 解析完成後的 application 邊界。
type MessageProcessor interface {
	Process(ctx context.Context, message domain.Message) (detectionapp.ProcessResult, error)
}

// AutoReplyProcessor 定義非垃圾訊息後可選的固定問答回覆邊界。
type AutoReplyProcessor interface {
	Process(ctx context.Context, message domain.Message) error
}

// CommandProcessor 定義指令解析完成後的 application 邊界。
type CommandProcessor interface {
	Handle(ctx context.Context, command commanddomain.Command) error
}

// Webhook 驗證 Telegram secret、限制 body 並轉換 Update。
type Webhook struct {
	secret         [sha256.Size]byte
	maxBody        int64
	process        MessageProcessor
	autoReplies    AutoReplyProcessor
	commands       CommandProcessor
	botUsername    string
	allowedChatIDs map[int64]struct{}
}

// WithAutoReplyProcessor 在非垃圾的一般訊息後執行固定問答自動回覆。
func WithAutoReplyProcessor(processor AutoReplyProcessor) Option {
	return func(webhook *Webhook) error {
		if processor == nil {
			return errors.New("auto reply processor is required")
		}
		webhook.autoReplies = processor
		return nil
	}
}

// WithCommandProcessor 在一般垃圾訊息偵測前分流 Telegram bot command。
func WithCommandProcessor(processor CommandProcessor, botUsername string) Option {
	return func(webhook *Webhook) error {
		if processor == nil || strings.TrimSpace(botUsername) == "" {
			return errors.New("command processor and bot username are required")
		}
		webhook.commands = processor
		webhook.botUsername = strings.TrimPrefix(strings.TrimSpace(botUsername), "@")
		return nil
	}
}

// Option 調整 Webhook 的安全接收範圍。
type Option func(*Webhook) error

// WithAllowedChatIDs 建立不可變的群組允許清單，避免呼叫端後續修改原始 slice 影響執行中服務。
func WithAllowedChatIDs(chatIDs []int64) Option {
	return func(webhook *Webhook) error {
		if len(chatIDs) == 0 {
			return errors.New("at least one allowed chat id is required")
		}
		webhook.allowedChatIDs = make(map[int64]struct{}, len(chatIDs))
		for _, chatID := range chatIDs {
			if chatID == 0 {
				return errors.New("allowed chat id must not be zero")
			}
			if _, exists := webhook.allowedChatIDs[chatID]; exists {
				return fmt.Errorf("duplicate allowed chat id %d", chatID)
			}
			webhook.allowedChatIDs[chatID] = struct{}{}
		}
		return nil
	}
}

// NewWebhook 只保存 secret 雜湊，避免原始秘密值長期留在結構中。
func NewWebhook(secret string, maxBody int64, processor MessageProcessor, opts ...Option) (*Webhook, error) {
	if secret == "" || maxBody <= 0 || processor == nil {
		return nil, errors.New("secret, positive max body and processor are required")
	}
	webhook := &Webhook{secret: sha256.Sum256([]byte(secret)), maxBody: maxBody, process: processor}
	for _, opt := range opts {
		if opt == nil {
			return nil, errors.New("webhook option must not be nil")
		}
		if err := opt(webhook); err != nil {
			return nil, fmt.Errorf("configure webhook: %w", err)
		}
	}
	return webhook, nil
}

// ServeHTTP 以固定時間比較驗證來源，並拒絕超大或尾隨 JSON 請求。
func (h *Webhook) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	provided := sha256.Sum256([]byte(r.Header.Get(secretHeader)))
	if subtle.ConstantTimeCompare(h.secret[:], provided[:]) != 1 {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, h.maxBody)
	defer func() { _ = r.Body.Close() }()
	var update Update
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&update); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			http.Error(w, "request too large", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "invalid update", http.StatusBadRequest)
		return
	}
	if err := ensureEOF(decoder); err != nil {
		http.Error(w, "invalid update", http.StatusBadRequest)
		return
	}
	if update.Message == nil || !update.Message.Chat.isModeratedGroup() {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if len(h.allowedChatIDs) > 0 {
		if _, allowed := h.allowedChatIDs[update.Message.Chat.ID]; !allowed {
			w.WriteHeader(http.StatusNoContent)
			return
		}
	}
	ctx := zlogger.WithRequestID(r.Context(), fmt.Sprintf("tg:%d", update.UpdateID))
	zlogger.DebugContext(ctx, "收到 Telegram Webhook 更新",
		zlogger.String("subsystem", "webhook"),
		zlogger.Int64("update_id", update.UpdateID),
		zlogger.Int64("chat_id", update.Message.Chat.ID),
	)
	if h.commands != nil {
		command, disposition := update.Command(h.botUsername)
		switch disposition {
		case CommandHandle:
			if err := h.commands.Handle(ctx, command); err != nil {
				zlogger.ErrorContext(ctx, "處理 Telegram 管理指令失敗",
					zlogger.String("subsystem", "webhook"),
					zlogger.Int64("update_id", update.UpdateID),
					zlogger.Int64("chat_id", update.Message.Chat.ID),
					zlogger.String("command", string(command.Name)),
					zlogger.Err(err),
				)
				http.Error(w, "temporary command failure", http.StatusServiceUnavailable)
				return
			}
			w.WriteHeader(http.StatusNoContent)
			return
		case CommandIgnore:
			zlogger.DebugContext(ctx, "略過不屬於目前 Bot 的管理指令",
				zlogger.String("subsystem", "webhook"),
				zlogger.Int64("update_id", update.UpdateID),
				zlogger.Int64("chat_id", update.Message.Chat.ID),
			)
			w.WriteHeader(http.StatusNoContent)
			return
		}
	}
	message, ok := update.DomainMessage()
	if !ok {
		if h.autoReplies != nil {
			if message, autoReplyOK := update.AutoReplyMessage(); autoReplyOK {
				if err := h.autoReplies.Process(ctx, message); err != nil {
					zlogger.ErrorContext(ctx, "處理 Telegram 自動回覆失敗",
						zlogger.String("subsystem", "webhook"),
						zlogger.Int64("update_id", update.UpdateID),
						zlogger.Int64("chat_id", update.Message.Chat.ID),
						zlogger.Err(err),
					)
					http.Error(w, "temporary auto reply failure", http.StatusServiceUnavailable)
					return
				}
			}
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	result, err := h.process.Process(ctx, message)
	if err != nil {
		zlogger.ErrorContext(ctx, "處理 Telegram 訊息失敗",
			zlogger.String("subsystem", "webhook"),
			zlogger.Int64("update_id", update.UpdateID),
			zlogger.Int64("chat_id", update.Message.Chat.ID),
			zlogger.Err(err),
		)
		http.Error(w, "temporary processing failure", http.StatusServiceUnavailable)
		return
	}
	if h.autoReplies != nil && !result.Spam {
		if err := h.autoReplies.Process(ctx, message); err != nil {
			zlogger.ErrorContext(ctx, "處理 Telegram 自動回覆失敗",
				zlogger.String("subsystem", "webhook"),
				zlogger.Int64("update_id", update.UpdateID),
				zlogger.Int64("chat_id", update.Message.Chat.ID),
				zlogger.Err(err),
			)
			http.Error(w, "temporary auto reply failure", http.StatusServiceUnavailable)
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

func ensureEOF(decoder *json.Decoder) error {
	var extra any
	err := decoder.Decode(&extra)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err == nil {
		return fmt.Errorf("unexpected trailing JSON")
	}
	return err
}
