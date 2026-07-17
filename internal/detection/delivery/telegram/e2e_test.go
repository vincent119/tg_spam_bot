package telegram_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/vincent119/tg_spam_bot/internal/detection/application"
	delivery "github.com/vincent119/tg_spam_bot/internal/detection/delivery/telegram"
	"github.com/vincent119/tg_spam_bot/internal/detection/domain"
	"github.com/vincent119/tg_spam_bot/internal/detection/infra/memory"
)

type adminNone struct{}

func (adminNone) AdminIDs(context.Context, int64) ([]int64, error) { return nil, nil }

type telegramRecorder struct{ actions []string }

func (r *telegramRecorder) DeleteMessage(context.Context, int64, int64) error {
	r.actions = append(r.actions, "delete")
	return nil
}
func (r *telegramRecorder) SendWarning(context.Context, int64, int64, string) error {
	r.actions = append(r.actions, "warn")
	return nil
}
func (r *telegramRecorder) RestrictMember(context.Context, int64, int64, time.Time) error {
	r.actions = append(r.actions, "restrict")
	return nil
}
func (r *telegramRecorder) BanMember(context.Context, int64, int64) error {
	r.actions = append(r.actions, "ban")
	return nil
}

func TestWebhookToCriticalEnforcement(t *testing.T) {
	t.Parallel()
	ruleSet := domain.RuleSet{Version: "e2e", Categories: []domain.Category{{
		ID: "counterfeit", Name: "偽鈔", Severity: domain.SeverityCritical,
		Action: domain.ActionBan, Threshold: 80, Weight: 70, Enabled: true,
		Terms: []string{"假鈔"}, Aliases: []string{"假钞"}, RequireAny: []string{"telegram_mention", "transaction_signal"},
	}}}
	detector, err := domain.NewDetector(ruleSet, domain.NewNormalizer(domain.OpenCCConverter{}, 4096), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	store := memory.NewStore(time.Minute, 100)
	exemptions, _ := application.NewCachedExemptions(store, adminNone{}, time.Minute)
	recorder := &telegramRecorder{}
	processor := application.NewProcessor(detector, store, exemptions, store, store, recorder, application.ModeEnforce, []byte("01234567890123456789012345678901"))
	handler, err := delivery.NewWebhook("secret", 4096, processor)
	if err != nil {
		t.Fatal(err)
	}
	body := `{"update_id":7,"message":{"message_id":8,"date":1,"chat":{"id":9,"type":"supergroup"},"from":{"id":10,"is_bot":false,"first_name":"u"},"text":"假钞稳定出货 @seller"}}`
	for range 2 {
		req := httptest.NewRequest(http.MethodPost, "/telegram/webhook", strings.NewReader(body))
		req.Header.Set("X-Telegram-Bot-Api-Secret-Token", "secret")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		if response.Code != http.StatusNoContent {
			t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
		}
	}
	if len(recorder.actions) != 2 || recorder.actions[0] != "delete" || recorder.actions[1] != "ban" {
		t.Fatalf("actions = %v", recorder.actions)
	}
}
