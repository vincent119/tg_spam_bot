package health

import (
	"context"
	"strings"
	"testing"

	tgclient "github.com/vincent119/tg_spam_bot/internal/detection/infra/telegram"
)

func TestTelegramMonitorCheck(t *testing.T) {
	t.Parallel()

	client := &telegramClientStub{
		identity: tgclient.BotIdentity{ID: 99, Username: "liyu_spam_bot"},
		webhook:  tgclient.WebhookInfo{URL: "https://example.com/telegram/webhook"},
		permissions: map[int64]tgclient.BotPermissions{
			-1001: {Status: "administrator", CanDeleteMessages: true, CanRestrictMembers: true},
		},
	}
	monitor, err := NewTelegramMonitor(client, []int64{-1001}, "https://example.com/telegram/webhook")
	if err != nil {
		t.Fatalf("NewTelegramMonitor() error = %v", err)
	}

	if err := monitor.Check(t.Context()); err != nil {
		t.Fatalf("Check() error = %v", err)
	}
}

func TestTelegramMonitorReportsMissingPermissions(t *testing.T) {
	t.Parallel()

	client := &telegramClientStub{
		identity: tgclient.BotIdentity{ID: 99, Username: "liyu_spam_bot"},
		webhook:  tgclient.WebhookInfo{URL: "https://example.com/telegram/webhook"},
		permissions: map[int64]tgclient.BotPermissions{
			-1001: {Status: "administrator", CanDeleteMessages: true},
		},
	}
	monitor, err := NewTelegramMonitor(client, []int64{-1001}, "")
	if err != nil {
		t.Fatalf("NewTelegramMonitor() error = %v", err)
	}

	err = monitor.Check(t.Context())
	if err == nil || !strings.Contains(err.Error(), "can_restrict_members") {
		t.Fatalf("Check() error = %v，預期缺少 can_restrict_members", err)
	}
}

func TestTelegramMonitorReportsWebhookURLMismatch(t *testing.T) {
	t.Parallel()

	client := &telegramClientStub{
		identity: tgclient.BotIdentity{ID: 99, Username: "liyu_spam_bot"},
		webhook:  tgclient.WebhookInfo{URL: "https://example.com/wrong"},
		permissions: map[int64]tgclient.BotPermissions{
			-1001: {Status: "administrator", CanDeleteMessages: true, CanRestrictMembers: true},
		},
	}
	monitor, err := NewTelegramMonitor(client, []int64{-1001}, "https://example.com/telegram/webhook")
	if err != nil {
		t.Fatalf("NewTelegramMonitor() error = %v", err)
	}

	err = monitor.Check(t.Context())
	if err == nil || !strings.Contains(err.Error(), "Webhook") {
		t.Fatalf("Check() error = %v，預期 Webhook URL 不一致", err)
	}
}

type telegramClientStub struct {
	identity    tgclient.BotIdentity
	webhook     tgclient.WebhookInfo
	permissions map[int64]tgclient.BotPermissions
}

func (s *telegramClientStub) GetMe(context.Context) (tgclient.BotIdentity, error) {
	return s.identity, nil
}

func (s *telegramClientStub) GetWebhookInfo(context.Context) (tgclient.WebhookInfo, error) {
	return s.webhook, nil
}

func (s *telegramClientStub) BotPermissions(_ context.Context, chatID, _ int64) (tgclient.BotPermissions, error) {
	return s.permissions[chatID], nil
}
