package telegram

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/vincent119/tg_spam_bot/internal/detection/domain"
)

type processorFunc func(context.Context, domain.Message) error

func (f processorFunc) Process(ctx context.Context, message domain.Message) error {
	return f(ctx, message)
}

func TestWebhook(t *testing.T) {
	t.Parallel()
	called := false
	h, err := NewWebhook("secret", 1024, processorFunc(func(_ context.Context, m domain.Message) error {
		called = m.Text == "hello"
		return nil
	}))
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		secret string
		body   string
		status int
	}{
		{name: "valid", secret: "secret", body: `{"update_id":1,"message":{"message_id":2,"date":1,"chat":{"id":3,"type":"supergroup"},"from":{"id":4,"is_bot":false,"first_name":"u"},"text":"hello"}}`, status: http.StatusNoContent},
		{name: "bad secret", secret: "bad", body: `{}`, status: http.StatusUnauthorized},
		{name: "invalid json", secret: "secret", body: `{`, status: http.StatusBadRequest},
		{name: "too large", secret: "secret", body: `{"padding":"` + strings.Repeat("x", 2000) + `"}`, status: http.StatusRequestEntityTooLarge},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/telegram/webhook", strings.NewReader(tt.body))
			req.Header.Set(secretHeader, tt.secret)
			res := httptest.NewRecorder()
			h.ServeHTTP(res, req)
			if res.Code != tt.status {
				t.Fatalf("status = %d, want %d", res.Code, tt.status)
			}
		})
	}
	if !called {
		t.Fatal("processor was not called")
	}
}
