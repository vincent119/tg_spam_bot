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
	valid.Log.Rotate.MaxSizeMB = 100
	valid.Log.Rotate.MaxBackups = 14
	valid.Log.Rotate.MaxAgeDays = 30
	valid.Log.Rotate.Compress = true
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
	valid.AIDetection.Mode = ModeObserve
	valid.AIDetection.Timeout = 3 * time.Second
	valid.AIDetection.MaxTextChars = 800
	valid.AIDetection.MinConfidence = 0.85
	valid.AIDetection.OnlyWhenAmbiguous = true
	valid.AIDetection.CacheTTL = 24 * time.Hour
	valid.AIDetection.Bedrock.AuthMode = BedrockAuthModeIAMRole
	valid.SemanticMemory.EmbeddingVersion = "v1"
	valid.SemanticMemory.SimilarityThreshold = 0.88
	valid.SemanticMemory.SpamSimilarityThreshold = 0.90
	valid.SemanticMemory.HamSimilarityThreshold = 0.95
	valid.SemanticMemory.MaxNeighbors = 5
	valid.SemanticMemory.CacheTTL = 168 * time.Hour
	valid.SemanticMemory.Bedrock.AuthMode = BedrockAuthModeIAMRole

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
		{name: "negative log rotate max size", mutate: func(c *Config) { c.Log.Rotate.MaxSizeMB = -1 }, wantErr: true},
		{name: "negative log rotate max backups", mutate: func(c *Config) { c.Log.Rotate.MaxBackups = -1 }, wantErr: true},
		{name: "negative log rotate max age", mutate: func(c *Config) { c.Log.Rotate.MaxAgeDays = -1 }, wantErr: true},
		{name: "zero log rotate max size allowed", mutate: func(c *Config) { c.Log.Rotate.MaxSizeMB = 0 }},
		{name: "zero log rotate max backups allowed", mutate: func(c *Config) { c.Log.Rotate.MaxBackups = 0 }},
		{name: "zero log rotate max age allowed", mutate: func(c *Config) { c.Log.Rotate.MaxAgeDays = 0 }},
		{name: "enabled auto replies missing rules file", mutate: func(c *Config) { c.AutoReplies.Enabled = true }, wantErr: true},
		{name: "enabled auto replies with rules file", mutate: func(c *Config) {
			c.AutoReplies.Enabled = true
			c.AutoReplies.RulesFile = "configs/auto_replies.yaml"
		}},
		{name: "ai disabled defaults valid", mutate: func(c *Config) { c.AIDetection.Enabled = false }},
		{name: "ai invalid confidence", mutate: func(c *Config) { c.AIDetection.MinConfidence = 1.1 }, wantErr: true},
		{name: "ai enabled missing provider", mutate: func(c *Config) { c.AIDetection.Enabled = true }, wantErr: true},
		{name: "ai openai compatible", mutate: func(c *Config) {
			c.AIDetection.Enabled = true
			c.AIDetection.Provider = AIProviderOpenAICompatible
			c.AIDetection.OpenAICompatible.Endpoint = "https://api.example.com/v1"
			c.AIDetection.OpenAICompatible.Model = "classifier"
			c.AIDetection.OpenAICompatible.APIKey = "secret"
		}},
		{name: "ai bedrock iam role", mutate: func(c *Config) {
			c.AIDetection.Enabled = true
			c.AIDetection.Provider = AIProviderBedrock
			c.AIDetection.Bedrock.Region = "us-east-1"
			c.AIDetection.Bedrock.ModelID = "amazon.nova-lite-v1:0"
			c.AIDetection.Bedrock.AuthMode = BedrockAuthModeIAMRole
		}},
		{name: "ai bedrock static keys missing secret", mutate: func(c *Config) {
			c.AIDetection.Enabled = true
			c.AIDetection.Provider = AIProviderBedrock
			c.AIDetection.Bedrock.Region = "us-east-1"
			c.AIDetection.Bedrock.ModelID = "amazon.nova-lite-v1:0"
			c.AIDetection.Bedrock.AuthMode = BedrockAuthModeStaticKeys
			c.AIDetection.Bedrock.AccessKeyID = "access"
		}, wantErr: true},
		{name: "semantic memory disabled defaults valid", mutate: func(c *Config) { c.SemanticMemory.Enabled = false }},
		{name: "semantic memory invalid threshold", mutate: func(c *Config) { c.SemanticMemory.SpamSimilarityThreshold = -0.1 }, wantErr: true},
		{name: "semantic memory enabled openai compatible", mutate: func(c *Config) {
			c.SemanticMemory.Enabled = true
			c.SemanticMemory.EmbeddingProvider = AIProviderOpenAICompatible
			c.SemanticMemory.OpenAICompatible.Endpoint = "https://api.example.com/v1"
			c.SemanticMemory.OpenAICompatible.Model = "embedding"
			c.SemanticMemory.OpenAICompatible.APIKey = "secret"
		}},
		{name: "semantic memory bedrock static keys", mutate: func(c *Config) {
			c.SemanticMemory.Enabled = true
			c.SemanticMemory.EmbeddingProvider = AIProviderBedrock
			c.SemanticMemory.Bedrock.Region = "us-east-1"
			c.SemanticMemory.Bedrock.ModelID = "amazon.titan-embed-text-v2:0"
			c.SemanticMemory.Bedrock.AuthMode = BedrockAuthModeStaticKeys
			c.SemanticMemory.Bedrock.AccessKeyID = "access"
			c.SemanticMemory.Bedrock.SecretAccessKey = "secret"
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
	t.Setenv("TELEGRAM_WEBHOOK_URL", "https://example.com/telegram/webhook")
	t.Setenv("TELEGRAM_ALLOWED_CHAT_IDS", "-1001234567890,-1009876543210")
	t.Setenv("CONTENT_HASH_KEY", "01234567890123456789012345678901")
	t.Setenv("REDIS_USERNAME", "app")
	t.Setenv("REDIS_PASSWORD", "redis-secret")
	t.Setenv("REDIS_DB", "2")
	t.Setenv("AI_DETECTION_OPENAI_COMPATIBLE_API_KEY", "ai-secret")
	t.Setenv("SEMANTIC_MEMORY_BEDROCK_ACCESS_KEY_ID", "semantic-access")
	t.Setenv("SEMANTIC_MEMORY_BEDROCK_SECRET_ACCESS_KEY", "semantic-secret")

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
  rotate:
    enabled: true
    max_size_mb: 0
    max_backups: 0
    max_age_days: 0
    compress: false
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
ai_detection:
  enabled: true
  mode: observe
  provider: openai_compatible
  timeout: 3s
  max_text_chars: 800
  min_confidence: 0.85
  only_when_ambiguous: true
  cache_ttl: 24h
  openai_compatible:
    endpoint: https://ai.example.com/v1
    model: spam-classifier
semantic_memory:
  enabled: true
  embedding_provider: bedrock
  embedding_version: v1
  embedding_dimensions: 1024
  similarity_threshold: 0.88
  spam_similarity_threshold: 0.90
  ham_similarity_threshold: 0.95
  max_neighbors: 5
  cache_ttl: 168h
  bedrock:
    region: us-east-1
    model_id: amazon.titan-embed-text-v2:0
    auth_mode: static_keys
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
	if !cfg.Log.Rotate.Enabled || cfg.Log.Rotate.MaxSizeMB != 0 || cfg.Log.Rotate.MaxBackups != 0 || cfg.Log.Rotate.MaxAgeDays != 0 || cfg.Log.Rotate.Compress {
		t.Fatalf("未正確載入 log rotate 設定：%+v", cfg.Log.Rotate)
	}
	if effective := cfg.EffectiveLogRotate(); effective.MaxSizeMB != 100 || effective.MaxBackups != 0 || effective.MaxAgeDays != 0 || effective.Compress {
		t.Fatalf("未正確套用 log rotate 有效值：%+v", effective)
	}
	if len(cfg.Telegram.AllowedChatIDs) != 2 || cfg.Telegram.AllowedChatIDs[0] != -1001234567890 {
		t.Fatalf("未正確載入 Telegram 允許群組：%+v", cfg.Telegram.AllowedChatIDs)
	}
	if cfg.Telegram.WebhookURL != "https://example.com/telegram/webhook" {
		t.Fatalf("未正確載入 Telegram Webhook URL：%q", cfg.Telegram.WebhookURL)
	}
	if !cfg.AutoReplies.Enabled || cfg.AutoReplies.RulesFile != "configs/auto_replies.yaml" {
		t.Fatalf("未正確載入自動回覆設定：%+v", cfg.AutoReplies)
	}
	if !cfg.AIDetection.Enabled || cfg.AIDetection.Provider != AIProviderOpenAICompatible || cfg.AIDetection.OpenAICompatible.APIKey != "ai-secret" {
		t.Fatalf("未正確載入 AI 設定：%+v", cfg.AIDetection)
	}
	if !cfg.SemanticMemory.Enabled || cfg.SemanticMemory.EmbeddingProvider != AIProviderBedrock || cfg.SemanticMemory.EmbeddingDimensions != 1024 {
		t.Fatalf("未正確載入語意記憶設定：%+v", cfg.SemanticMemory)
	}
	if cfg.SemanticMemory.Bedrock.AccessKeyID != "semantic-access" || cfg.SemanticMemory.Bedrock.SecretAccessKey != "semantic-secret" {
		t.Fatalf("未正確載入 Bedrock credential：%+v", cfg.SemanticMemory.Bedrock)
	}
}
