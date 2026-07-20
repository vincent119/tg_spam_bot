package telegram_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	commandapp "github.com/vincent119/tg_spam_bot/internal/command/application"
	commanddomain "github.com/vincent119/tg_spam_bot/internal/command/domain"
	detectionapp "github.com/vincent119/tg_spam_bot/internal/detection/application"
	delivery "github.com/vincent119/tg_spam_bot/internal/detection/delivery/telegram"
	"github.com/vincent119/tg_spam_bot/internal/detection/domain"
)

type commandTelegramRecorder struct {
	admins   map[int64]bool
	messages []string
	bans     int
}

func (r *commandTelegramRecorder) IsAdmin(_ context.Context, _ int64, userID int64) (bool, error) {
	return r.admins[userID], nil
}

func (r *commandTelegramRecorder) SendMessage(_ context.Context, _ int64, _ int64, text string) error {
	r.messages = append(r.messages, text)
	return nil
}
func (*commandTelegramRecorder) DeleteMessage(context.Context, int64, int64) error { return nil }
func (*commandTelegramRecorder) RestrictMember(context.Context, int64, int64, time.Time) error {
	return nil
}
func (*commandTelegramRecorder) UnrestrictMember(context.Context, int64, int64) error { return nil }
func (r *commandTelegramRecorder) BanMember(context.Context, int64, int64) error {
	r.bans++
	return nil
}
func (*commandTelegramRecorder) UnbanMember(context.Context, int64, int64) error { return nil }

type commandStore struct {
	mu      sync.Mutex
	results map[int64]commanddomain.Result
}

func (s *commandStore) ClaimCommand(_ context.Context, command commanddomain.Command) (commanddomain.Claim, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if result, exists := s.results[command.UpdateID]; exists {
		existingResult := result
		return commanddomain.Claim{Existing: &existingResult}, nil
	}
	s.results[command.UpdateID] = commanddomain.Result{Status: "processing"}
	return commanddomain.Claim{Acquired: true}, nil
}

func (s *commandStore) CompleteCommand(_ context.Context, command commanddomain.Command, result commanddomain.Result) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.results[command.UpdateID] = result
	return nil
}

func (*commandStore) Warnings(context.Context, int64, int64, time.Time) (commandapp.WarningSummary, error) {
	return commandapp.WarningSummary{}, nil
}

func (*commandStore) AddManualWarning(context.Context, commanddomain.Command, commanddomain.Reason, time.Time) (commandapp.WarningSummary, error) {
	return commandapp.WarningSummary{Total: 1, Manual: 1}, nil
}

func (*commandStore) ClearWarnings(context.Context, commanddomain.Command, commanddomain.Reason, time.Time) (int64, error) {
	return 1, nil
}

func (*commandStore) IsExempt(context.Context, int64, int64) (bool, string, error) {
	return false, "", nil
}

type allowAllLimiter struct{}

func (allowAllLimiter) Allow(context.Context, int64, int64) (bool, error) { return true, nil }

type detectionCounter struct{ calls int }

func (p *detectionCounter) Process(context.Context, domain.Message) (detectionapp.ProcessResult, error) {
	p.calls++
	return detectionapp.ProcessResult{}, nil
}

func TestWebhookCommandEndToEnd(t *testing.T) {
	t.Parallel()

	telegram := &commandTelegramRecorder{admins: map[int64]bool{1: true}}
	store := &commandStore{results: make(map[int64]commanddomain.Result)}
	commands, err := commandapp.NewHandler(telegram, store, store, allowAllLimiter{}, 99)
	if err != nil {
		t.Fatal(err)
	}
	detection := &detectionCounter{}
	webhook, err := delivery.NewWebhook(
		"secret", 4096, detection,
		delivery.WithAllowedChatIDs([]int64{-1001}),
		delivery.WithCommandProcessor(commands, "liyu_spam_bot"),
	)
	if err != nil {
		t.Fatal(err)
	}

	requests := []struct {
		name string
		body string
	}{
		{name: "公開指令", body: `{"update_id":1,"message":{"message_id":10,"date":1,"chat":{"id":-1001,"type":"supergroup"},"from":{"id":2},"text":"/ping","entities":[{"type":"bot_command","offset":0,"length":5}]}}`},
		{name: "管理指令", body: `{"update_id":2,"message":{"message_id":11,"date":1,"chat":{"id":-1001,"type":"supergroup"},"from":{"id":1},"text":"/ban","entities":[{"type":"bot_command","offset":0,"length":4}],"reply_to_message":{"message_id":9,"date":1,"chat":{"id":-1001,"type":"supergroup"},"from":{"id":3},"text":"spam"}}}`},
		{name: "管理指令重送", body: `{"update_id":2,"message":{"message_id":11,"date":1,"chat":{"id":-1001,"type":"supergroup"},"from":{"id":1},"text":"/ban","entities":[{"type":"bot_command","offset":0,"length":4}],"reply_to_message":{"message_id":9,"date":1,"chat":{"id":-1001,"type":"supergroup"},"from":{"id":3},"text":"spam"}}}`},
		{name: "其他 Bot", body: `{"update_id":3,"message":{"message_id":12,"date":1,"chat":{"id":-1001,"type":"supergroup"},"from":{"id":2},"text":"/ping@other_bot","entities":[{"type":"bot_command","offset":0,"length":15}]}}`},
		{name: "未授權群組", body: `{"update_id":4,"message":{"message_id":13,"date":1,"chat":{"id":-1002,"type":"supergroup"},"from":{"id":2},"text":"/ping","entities":[{"type":"bot_command","offset":0,"length":5}]}}`},
		{name: "一般訊息", body: `{"update_id":5,"message":{"message_id":14,"date":1,"chat":{"id":-1001,"type":"supergroup"},"from":{"id":2},"text":"今天吃什麼"}}`},
	}
	for _, request := range requests {
		t.Run(request.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/telegram/webhook", strings.NewReader(request.body))
			req.Header.Set("X-Telegram-Bot-Api-Secret-Token", "secret")
			response := httptest.NewRecorder()
			webhook.ServeHTTP(response, req)
			if response.Code != http.StatusNoContent {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
	if telegram.bans != 1 {
		t.Fatalf("ban 次數=%d，預期 1", telegram.bans)
	}
	if detection.calls != 1 {
		t.Fatalf("一般訊息處理次數=%d，預期 1", detection.calls)
	}
}
