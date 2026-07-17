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

type MessageProcessor interface {
	Process(ctx context.Context, message domain.Message) error
}

type Webhook struct {
	secret  [sha256.Size]byte
	maxBody int64
	process MessageProcessor
}

func NewWebhook(secret string, maxBody int64, processor MessageProcessor) (*Webhook, error) {
	if secret == "" || maxBody <= 0 || processor == nil {
		return nil, errors.New("secret, positive max body and processor are required")
	}
	return &Webhook{secret: sha256.Sum256([]byte(secret)), maxBody: maxBody, process: processor}, nil
}

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
