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

	"github.com/vincent119/tg_spam_bot/internal/detection/domain"
)

const secretHeader = "X-Telegram-Bot-Api-Secret-Token"

// MessageProcessor 定義 Webhook 解析完成後的 application 邊界。
type MessageProcessor interface {
	Process(ctx context.Context, message domain.Message) error
}

// Webhook 驗證 Telegram secret、限制 body 並轉換 Update。
type Webhook struct {
	secret         [sha256.Size]byte
	maxBody        int64
	process        MessageProcessor
	allowedChatIDs map[int64]struct{}
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
	message, ok := update.DomainMessage()
	if !ok {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if len(h.allowedChatIDs) > 0 {
		if _, allowed := h.allowedChatIDs[message.ChatID]; !allowed {
			w.WriteHeader(http.StatusNoContent)
			return
		}
	}
	if err := h.process.Process(r.Context(), message); err != nil {
		http.Error(w, "temporary processing failure", http.StatusServiceUnavailable)
		return
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
