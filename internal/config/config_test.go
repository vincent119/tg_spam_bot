package config

import (
	"strings"
	"testing"
	"time"
)

func TestValidate(t *testing.T) {
	t.Parallel()

	valid := Config{}
	valid.App.Mode = ModeObserve
	valid.App.Port = 8080
	valid.App.Env = "test"
	valid.App.BusinessTimezone = "Asia/Taipei"
	valid.App.ReadTimeout = 10 * time.Second
	valid.App.WriteTimeout = 30 * time.Second
	valid.App.ShutdownTimeout = 30 * time.Second
	valid.App.MaxBodyBytes = 1024
	valid.Log.Level = "info"
	valid.Log.Format = "json"
	valid.Telegram.BotToken = "token"
	valid.Telegram.WebhookSecret = "secret"
	valid.DB.URL = "postgres://db"
	valid.DB.MaxOpenConns = 25
	valid.DB.MaxIdleConns = 5
	valid.DB.ConnMaxLifetime = 5 * time.Minute
	valid.Redis.Addr = "redis:6379"
	valid.Security.ContentHashKey = "01234567890123456789012345678901"
	valid.Rules.Dir = "rules"

	tests := []struct {
		name    string
		mutate  func(*Config)
		wantErr bool
	}{
		{name: "valid", mutate: func(*Config) {}},
		{name: "invalid mode", mutate: func(c *Config) { c.App.Mode = "bad" }, wantErr: true},
		{name: "missing token", mutate: func(c *Config) { c.Telegram.BotToken = "" }, wantErr: true},
		{name: "short hash key", mutate: func(c *Config) { c.Security.ContentHashKey = "short" }, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := valid
			tt.mutate(&cfg)
			if got := cfg.Validate(); (got != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", got, tt.wantErr)
			}
		})
	}
}

func TestLoadStructuredSample(t *testing.T) {
	t.Setenv("DB_USER", "tg_spam")
	t.Setenv("DB_PASSWORD", "secret")
	t.Setenv("TELEGRAM_BOT_TOKEN", "token")
	t.Setenv("TELEGRAM_WEBHOOK_SECRET", "webhook-secret")
	t.Setenv("CONTENT_HASH_KEY", "01234567890123456789012345678901")

	cfg, err := Load("../../configs/config.sample.yaml")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.HTTPAddress() != ":8080" || cfg.App.WriteTimeout != 30*time.Second {
		t.Fatalf("未正確載入 app 設定：%+v", cfg.App)
	}
	if got := cfg.DatabaseURL(); !strings.Contains(got, "tg_spam:secret@localhost:5432/tg_spam") {
		t.Fatalf("DatabaseURL() = %q", got)
	}
}
