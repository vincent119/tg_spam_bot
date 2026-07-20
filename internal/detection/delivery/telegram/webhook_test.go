package telegram

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/vincent119/zlogger"

	commanddomain "github.com/vincent119/tg_spam_bot/internal/command/domain"
	detectionapp "github.com/vincent119/tg_spam_bot/internal/detection/application"
	"github.com/vincent119/tg_spam_bot/internal/detection/domain"
)

type processorFunc func(context.Context, domain.Message) (detectionapp.ProcessResult, error)

func (f processorFunc) Process(ctx context.Context, message domain.Message) (detectionapp.ProcessResult, error) {
	return f(ctx, message)
}

type autoReplyProcessorFunc func(context.Context, domain.Message) error

func (f autoReplyProcessorFunc) Process(ctx context.Context, message domain.Message) error {
	return f(ctx, message)
}

type commandProcessorFunc func(context.Context, commanddomain.Command) error

func (f commandProcessorFunc) Handle(ctx context.Context, command commanddomain.Command) error {
	return f(ctx, command)
}

func TestWebhook(t *testing.T) {
	t.Parallel()
	called := false
	h, err := NewWebhook("secret", 1024, processorFunc(func(_ context.Context, m domain.Message) (detectionapp.ProcessResult, error) {
		called = m.Text == "hello"
		return detectionapp.ProcessResult{}, nil
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

func TestWebhookConfigurationValidation(t *testing.T) {
	t.Parallel()
	processor := processorFunc(func(context.Context, domain.Message) (detectionapp.ProcessResult, error) {
		return detectionapp.ProcessResult{}, nil
	})
	tests := []struct {
		name string
		new  func() (*Webhook, error)
	}{
		{name: "空 secret", new: func() (*Webhook, error) { return NewWebhook("", 1, processor) }},
		{name: "空 processor", new: func() (*Webhook, error) { return NewWebhook("secret", 1, nil) }},
		{name: "nil option", new: func() (*Webhook, error) { return NewWebhook("secret", 1, processor, nil) }},
		{name: "空 allowlist", new: func() (*Webhook, error) { return NewWebhook("secret", 1, processor, WithAllowedChatIDs(nil)) }},
		{name: "重複 chat", new: func() (*Webhook, error) {
			return NewWebhook("secret", 1, processor, WithAllowedChatIDs([]int64{-1, -1}))
		}},
		{name: "空 command processor", new: func() (*Webhook, error) { return NewWebhook("secret", 1, processor, WithCommandProcessor(nil, "bot")) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := tt.new(); err == nil {
				t.Fatal("無效設定應失敗")
			}
		})
	}
}

func TestWebhookAllowedChatIDs(t *testing.T) {
	t.Parallel()

	processed := 0
	h, err := NewWebhook("secret", 1024, processorFunc(func(_ context.Context, _ domain.Message) (detectionapp.ProcessResult, error) {
		processed++
		return detectionapp.ProcessResult{}, nil
	}), WithAllowedChatIDs([]int64{-1001}))
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		body string
	}{
		{name: "允許的超級群組", body: `{"update_id":1,"message":{"message_id":2,"date":1,"chat":{"id":-1001,"type":"supergroup"},"from":{"id":4},"text":"hello"}}`},
		{name: "未允許的群組", body: `{"update_id":2,"message":{"message_id":2,"date":1,"chat":{"id":-1002,"type":"group"},"from":{"id":4},"text":"hello"}}`},
		{name: "頻道", body: `{"update_id":3,"message":{"message_id":2,"date":1,"chat":{"id":-1001,"type":"channel"},"from":{"id":4},"text":"hello"}}`},
		{name: "私人聊天", body: `{"update_id":4,"message":{"message_id":2,"date":1,"chat":{"id":4,"type":"private"},"from":{"id":4},"text":"hello"}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/telegram/webhook", strings.NewReader(tt.body))
			req.Header.Set(secretHeader, "secret")
			res := httptest.NewRecorder()
			h.ServeHTTP(res, req)
			if res.Code != http.StatusNoContent {
				t.Fatalf("status = %d, want %d", res.Code, http.StatusNoContent)
			}
		})
	}
	if processed != 1 {
		t.Fatalf("processor 呼叫次數 = %d，預期 1", processed)
	}
}

func TestWebhookRoutesCommandBeforeDetection(t *testing.T) {
	t.Parallel()
	commands := 0
	messages := 0
	h, err := NewWebhook(
		"secret",
		2048,
		processorFunc(func(context.Context, domain.Message) (detectionapp.ProcessResult, error) {
			messages++
			return detectionapp.ProcessResult{}, nil
		}),
		WithAllowedChatIDs([]int64{-1001}),
		WithCommandProcessor(commandProcessorFunc(func(ctx context.Context, command commanddomain.Command) error {
			commands++
			if command.Name != commanddomain.NamePing {
				t.Fatalf("command = %q", command.Name)
			}
			if requestID := contextStringField(ctx, "request_id"); requestID != "tg:11" {
				t.Fatalf("request_id = %q，預期 tg:11", requestID)
			}
			return nil
		}), "liyu_spam_bot"),
	)
	if err != nil {
		t.Fatal(err)
	}
	body := `{"update_id":11,"message":{"message_id":2,"date":1,"chat":{"id":-1001,"type":"supergroup"},"from":{"id":4},"text":"/ping","entities":[{"type":"bot_command","offset":0,"length":5}]}}`
	req := httptest.NewRequest(http.MethodPost, "/telegram/webhook", strings.NewReader(body))
	req.Header.Set(secretHeader, "secret")
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusNoContent || commands != 1 || messages != 0 {
		t.Fatalf("status=%d commands=%d messages=%d", res.Code, commands, messages)
	}
}

func TestWebhookAutoReplyAfterNonSpam(t *testing.T) {
	t.Parallel()
	autoReplies := 0
	h, err := NewWebhook(
		"secret",
		2048,
		processorFunc(func(context.Context, domain.Message) (detectionapp.ProcessResult, error) {
			return detectionapp.ProcessResult{Spam: false}, nil
		}),
		WithAllowedChatIDs([]int64{-1001}),
		WithAutoReplyProcessor(autoReplyProcessorFunc(func(_ context.Context, message domain.Message) error {
			autoReplies++
			if message.Text != "下載頁在哪" {
				t.Fatalf("auto reply message text = %q", message.Text)
			}
			return nil
		})),
	)
	if err != nil {
		t.Fatal(err)
	}
	body := `{"update_id":12,"message":{"message_id":2,"date":1,"chat":{"id":-1001,"type":"supergroup"},"from":{"id":4},"text":"下載頁在哪"}}`
	req := httptest.NewRequest(http.MethodPost, "/telegram/webhook", strings.NewReader(body))
	req.Header.Set(secretHeader, "secret")
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusNoContent || autoReplies != 1 {
		t.Fatalf("status=%d autoReplies=%d", res.Code, autoReplies)
	}
}

func TestWebhookSkipsAutoReplyForSpamAndCommands(t *testing.T) {
	t.Parallel()
	autoReplies := 0
	h, err := NewWebhook(
		"secret",
		2048,
		processorFunc(func(context.Context, domain.Message) (detectionapp.ProcessResult, error) {
			return detectionapp.ProcessResult{Spam: true}, nil
		}),
		WithAllowedChatIDs([]int64{-1001}),
		WithCommandProcessor(commandProcessorFunc(func(context.Context, commanddomain.Command) error { return nil }), "liyu_spam_bot"),
		WithAutoReplyProcessor(autoReplyProcessorFunc(func(context.Context, domain.Message) error {
			autoReplies++
			return nil
		})),
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{
		`{"update_id":13,"message":{"message_id":2,"date":1,"chat":{"id":-1001,"type":"supergroup"},"from":{"id":4},"text":"下載 app 賺錢"}}`,
		`{"update_id":14,"message":{"message_id":3,"date":1,"chat":{"id":-1001,"type":"supergroup"},"from":{"id":4},"text":"/ping","entities":[{"type":"bot_command","offset":0,"length":5}]}}`,
		`{"update_id":15,"message":{"message_id":4,"date":1,"chat":{"id":-1001,"type":"supergroup"},"from":{"id":4},"text":"/ping@other_bot","entities":[{"type":"bot_command","offset":0,"length":15}]}}`,
	} {
		req := httptest.NewRequest(http.MethodPost, "/telegram/webhook", strings.NewReader(body))
		req.Header.Set(secretHeader, "secret")
		res := httptest.NewRecorder()
		h.ServeHTTP(res, req)
		if res.Code != http.StatusNoContent {
			t.Fatalf("status=%d body=%s", res.Code, body)
		}
	}
	if autoReplies != 0 {
		t.Fatalf("autoReplies=%d，預期 0", autoReplies)
	}
}

func contextStringField(ctx context.Context, key string) string {
	for _, field := range zlogger.FromContext(ctx) {
		if field.Key == key {
			return field.String
		}
	}
	return ""
}
