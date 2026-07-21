package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	tgclient "github.com/vincent119/tg_spam_bot/internal/detection/infra/telegram"
	"github.com/vincent119/zlogger"
)

const (
	telegramHealthCheckInterval = 5 * time.Minute
	telegramHealthCheckTimeout  = 10 * time.Second
)

type telegramHealthClient interface {
	GetMe(ctx context.Context) (tgclient.BotIdentity, error)
	GetWebhookInfo(ctx context.Context) (tgclient.WebhookInfo, error)
	BotPermissions(ctx context.Context, chatID, botID int64) (tgclient.BotPermissions, error)
}

type telegramHealthMonitor struct {
	client             telegramHealthClient
	allowedChatIDs     []int64
	expectedWebhookURL string
	interval           time.Duration
	timeout            time.Duration

	mu        sync.RWMutex
	lastError error
}

func newTelegramHealthMonitor(client telegramHealthClient, allowedChatIDs []int64, expectedWebhookURL string) (*telegramHealthMonitor, error) {
	if client == nil {
		return nil, errors.New("telegram health client 不得為空")
	}
	if len(allowedChatIDs) == 0 {
		return nil, errors.New("telegram health allowed chat ids 不得為空")
	}
	return &telegramHealthMonitor{
		client:             client,
		allowedChatIDs:     append([]int64(nil), allowedChatIDs...),
		expectedWebhookURL: strings.TrimSpace(expectedWebhookURL),
		interval:           telegramHealthCheckInterval,
		timeout:            telegramHealthCheckTimeout,
	}, nil
}

// check 驗證 Bot 身分、Webhook 已設定，以及每個允許群組具備最小管理權限。
func (m *telegramHealthMonitor) check(ctx context.Context) error {
	identity, err := m.client.GetMe(ctx)
	if err != nil {
		return fmt.Errorf("驗證 Telegram Bot 身分：%w", err)
	}
	if strings.TrimSpace(identity.Username) == "" || identity.ID <= 0 {
		return errors.New("驗證 Telegram Bot 身分：缺少 bot id 或 username")
	}

	webhook, err := m.client.GetWebhookInfo(ctx)
	if err != nil {
		return fmt.Errorf("驗證 Telegram Webhook 設定：%w", err)
	}
	if strings.TrimSpace(webhook.URL) == "" {
		return errors.New("驗證 Telegram Webhook 設定：尚未設定 webhook url")
	}
	if m.expectedWebhookURL != "" && webhook.URL != m.expectedWebhookURL {
		return fmt.Errorf("驗證 Telegram Webhook 設定：url=%q，預期 %q", webhook.URL, m.expectedWebhookURL)
	}

	var errs []error
	for _, chatID := range m.allowedChatIDs {
		permissions, err := m.client.BotPermissions(ctx, chatID, identity.ID)
		if err != nil {
			errs = append(errs, fmt.Errorf("驗證 Telegram 群組權限 chat_id=%d：%w", chatID, err))
			continue
		}
		if permissions.Status != "administrator" && permissions.Status != "creator" {
			errs = append(errs, fmt.Errorf("驗證 Telegram 群組權限 chat_id=%d：bot status=%s，不是 administrator 或 creator", chatID, permissions.Status))
			continue
		}
		if !permissions.CanDeleteMessages {
			errs = append(errs, fmt.Errorf("驗證 Telegram 群組權限 chat_id=%d：缺少 can_delete_messages", chatID))
		}
		if !permissions.CanRestrictMembers {
			errs = append(errs, fmt.Errorf("驗證 Telegram 群組權限 chat_id=%d：缺少 can_restrict_members", chatID))
		}
	}
	return errors.Join(errs...)
}

func (m *telegramHealthMonitor) checkWithTimeout(ctx context.Context) error {
	checkCtx, cancel := context.WithTimeout(ctx, m.timeout)
	defer cancel()
	err := m.check(checkCtx)
	m.setLastError(err)
	return err
}

func (m *telegramHealthMonitor) start(ctx context.Context) {
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := m.checkWithTimeout(ctx); err != nil {
				zlogger.ErrorContext(ctx, "Telegram 健康檢查失敗",
					zlogger.String("subsystem", "health"),
					zlogger.Err(err),
				)
				continue
			}
			zlogger.DebugContext(ctx, "Telegram 健康檢查完成",
				zlogger.String("subsystem", "health"),
			)
		}
	}
}

func (m *telegramHealthMonitor) setLastError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastError = err
}

func (m *telegramHealthMonitor) lastErr() error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.lastError
}
