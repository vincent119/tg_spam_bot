package main

import (
	"context"
	"strings"
	"testing"

	tgclient "github.com/vincent119/tg_spam_bot/internal/detection/infra/telegram"
)

func TestTelegramHealthMonitorCheck(t *testing.T) {
	t.Parallel()

	client := &telegramHealthClientStub{
		identity: tgclient.BotIdentity{ID: 99, Username: "liyu_spam_bot"},
		webhook:  tgclient.WebhookInfo{URL: "https://example.com/telegram/webhook"},
		permissions: map[int64]tgclient.BotPermissions{
			-1001: {Status: "administrator", CanDeleteMessages: true, CanRestrictMembers: true},
		},
	}
	monitor, err := newTelegramHealthMonitor(client, []int64{-1001}, "https://example.com/telegram/webhook")
	if err != nil {
		t.Fatalf("newTelegramHealthMonitor() error = %v", err)
	}

	if err := monitor.check(t.Context()); err != nil {
		t.Fatalf("check() error = %v", err)
	}
}

func TestTelegramHealthMonitorReportsMissingPermissions(t *testing.T) {
	t.Parallel()

	client := &telegramHealthClientStub{
		identity: tgclient.BotIdentity{ID: 99, Username: "liyu_spam_bot"},
		webhook:  tgclient.WebhookInfo{URL: "https://example.com/telegram/webhook"},
		permissions: map[int64]tgclient.BotPermissions{
			-1001: {Status: "administrator", CanDeleteMessages: true},
		},
	}
	monitor, err := newTelegramHealthMonitor(client, []int64{-1001}, "")
	if err != nil {
		t.Fatalf("newTelegramHealthMonitor() error = %v", err)
	}

	err = monitor.check(t.Context())
	if err == nil || !strings.Contains(err.Error(), "can_restrict_members") {
		t.Fatalf("check() error = %v，預期缺少 can_restrict_members", err)
	}
}

func TestTelegramHealthMonitorReportsWebhookURLMismatch(t *testing.T) {
	t.Parallel()

	client := &telegramHealthClientStub{
		identity: tgclient.BotIdentity{ID: 99, Username: "liyu_spam_bot"},
		webhook:  tgclient.WebhookInfo{URL: "https://example.com/wrong"},
		permissions: map[int64]tgclient.BotPermissions{
			-1001: {Status: "administrator", CanDeleteMessages: true, CanRestrictMembers: true},
		},
	}
	monitor, err := newTelegramHealthMonitor(client, []int64{-1001}, "https://example.com/telegram/webhook")
	if err != nil {
		t.Fatalf("newTelegramHealthMonitor() error = %v", err)
	}

	err = monitor.check(t.Context())
	if err == nil || !strings.Contains(err.Error(), "Webhook") {
		t.Fatalf("check() error = %v，預期 Webhook URL 不一致", err)
	}
}

type telegramHealthClientStub struct {
	identity    tgclient.BotIdentity
	webhook     tgclient.WebhookInfo
	permissions map[int64]tgclient.BotPermissions
}

func (s *telegramHealthClientStub) GetMe(context.Context) (tgclient.BotIdentity, error) {
	return s.identity, nil
}

func (s *telegramHealthClientStub) GetWebhookInfo(context.Context) (tgclient.WebhookInfo, error) {
	return s.webhook, nil
}

func (s *telegramHealthClientStub) BotPermissions(_ context.Context, chatID, _ int64) (tgclient.BotPermissions, error) {
	return s.permissions[chatID], nil
}
