package config

import (
	"os"
	"path/filepath"
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
	valid.Log.Outputs = []string{"console"}
	valid.Log.Path = "./logs"
	valid.Telegram.BotToken = "token"
	valid.Telegram.WebhookSecret = "secret"
	valid.Telegram.AllowedChatIDs = []int64{-1001234567890}
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
		{name: "missing allowed chat", mutate: func(c *Config) { c.Telegram.AllowedChatIDs = nil }, wantErr: true},
		{name: "zero allowed chat", mutate: func(c *Config) { c.Telegram.AllowedChatIDs = []int64{0} }, wantErr: true},
		{name: "duplicate allowed chat", mutate: func(c *Config) { c.Telegram.AllowedChatIDs = []int64{-1001, -1001} }, wantErr: true},
		{name: "short hash key", mutate: func(c *Config) { c.Security.ContentHashKey = "short" }, wantErr: true},
		{name: "missing log outputs", mutate: func(c *Config) { c.Log.Outputs = nil }, wantErr: true},
		{name: "invalid log output", mutate: func(c *Config) { c.Log.Outputs = []string{"console", "bad"} }, wantErr: true},
		{name: "negative log max files", mutate: func(c *Config) { c.Log.MaxFiles = -1 }, wantErr: true},
		{name: "enabled auto replies missing rules file", mutate: func(c *Config) { c.AutoReplies.Enabled = true }, wantErr: true},
		{name: "enabled auto replies with rules file", mutate: func(c *Config) {
			c.AutoReplies.Enabled = true
			c.AutoReplies.RulesFile = "configs/auto_replies.yaml"
		}},
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
	t.Setenv("TELEGRAM_ALLOWED_CHAT_IDS", "-1001234567890,-1009876543210")
	t.Setenv("CONTENT_HASH_KEY", "01234567890123456789012345678901")
	t.Setenv("REDIS_USERNAME", "app")
	t.Setenv("REDIS_PASSWORD", "redis-secret")
	t.Setenv("REDIS_DB", "2")

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	content := []byte(`
app:
  port: 8080
  mode: observe
  business_timezone: Asia/Taipei
  read_timeout: 10s
  write_timeout: 30s
  shutdown_timeout: 30s
log:
  level: info
  format: json
  outputs:
    - console
    - file
  path: ./logs
  file: app.log
  max_files: 14
db:
  name: tg_spam
  primary:
    host: localhost
    port: 5432
redis:
  addr: localhost:6379
rules:
  dir: configs/rules
auto_replies:
  enabled: true
  rules_file: configs/auto_replies.yaml
`)
	if err := os.WriteFile(configPath, content, 0o600); err != nil {
		t.Fatalf("建立測試設定檔失敗：%v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.HTTPAddress() != ":8080" || cfg.App.WriteTimeout != 30*time.Second {
		t.Fatalf("未正確載入 app 設定：%+v", cfg.App)
	}
	if got := cfg.DatabaseURL(); !strings.Contains(got, "tg_spam:secret@localhost:5432/tg_spam") {
		t.Fatalf("DatabaseURL() = %q", got)
	}
	if cfg.Redis.Username != "app" || cfg.RedisPassword() != "redis-secret" || cfg.Redis.DB != 2 {
		t.Fatalf("未正確載入 Redis 設定：%+v", cfg.Redis)
	}
	if len(cfg.Log.Outputs) != 2 || cfg.Log.Outputs[1] != "file" || cfg.Log.Path != "./logs" || cfg.Log.File != "app.log" || cfg.Log.MaxFiles != 14 {
		t.Fatalf("未正確載入 log 設定：%+v", cfg.Log)
	}
	if len(cfg.Telegram.AllowedChatIDs) != 2 || cfg.Telegram.AllowedChatIDs[0] != -1001234567890 {
		t.Fatalf("未正確載入 Telegram 允許群組：%+v", cfg.Telegram.AllowedChatIDs)
	}
	if !cfg.AutoReplies.Enabled || cfg.AutoReplies.RulesFile != "configs/auto_replies.yaml" {
		t.Fatalf("未正確載入自動回覆設定：%+v", cfg.AutoReplies)
	}
}
