// Package config 負責合併預設值、YAML 與環境變數並驗證啟動設定。
package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Mode 定義偵測命中後允許執行的處置範圍。
type Mode string

const (
	// ModeObserve 只留下判定紀錄，用於上線前校準規則。
	ModeObserve Mode = "observe"
	// ModeDeleteOnly 只刪除訊息，不累計違規或限制成員。
	ModeDeleteOnly Mode = "delete-only"
	// ModeEnforce 啟用違規累計及完整階梯處置。
	ModeEnforce Mode = "enforce"
)

// AIProvider 定義 AI classifier 或 embedding adapter 的供應商類型。
type AIProvider string

const (
	// AIProviderOpenAICompatible 使用 OpenAI-compatible HTTP API 與 API key。
	AIProviderOpenAICompatible AIProvider = "openai_compatible"
	// AIProviderBedrock 使用 AWS Bedrock 與 AWS credential chain。
	AIProviderBedrock AIProvider = "bedrock"
)

// BedrockAuthMode 定義 Bedrock adapter 的 credential 來源。
type BedrockAuthMode string

const (
	// BedrockAuthModeIAMRole 透過 AWS SDK credential chain 使用 IAM Role。
	BedrockAuthModeIAMRole BedrockAuthMode = "iam_role"
	// BedrockAuthModeStaticKeys 透過明確 access key pair 呼叫 Bedrock。
	BedrockAuthModeStaticKeys BedrockAuthMode = "static_keys"
)

// Config 集中描述應用程式所有可由 YAML 或環境變數提供的設定。
type Config struct {
	// App 控制執行環境、HTTP 生命週期及垃圾訊息處置模式。
	App struct {
		// Port 是未提供完整監聽位址時使用的 HTTP 連接埠。
		Port int `mapstructure:"port"`
		// Addr 保留給 HTTP_ADDR 相容使用；設定後優先於 Port。
		Addr string `mapstructure:"addr"`
		// Env 標示 dev、staging 或 production 等部署環境。
		Env string `mapstructure:"env"`
		// Mode 決定命中垃圾訊息後可執行的處置範圍。
		Mode Mode `mapstructure:"mode"`
		// BusinessTimezone 僅用於業務顯示，資料儲存仍統一使用 UTC。
		BusinessTimezone string `mapstructure:"business_timezone"`
		// ReadTimeout 限制 HTTP Request 的完整讀取時間。
		ReadTimeout time.Duration `mapstructure:"read_timeout"`
		// WriteTimeout 限制 HTTP Response 的完整寫入時間。
		WriteTimeout time.Duration `mapstructure:"write_timeout"`
		// ShutdownTimeout 限制優雅關機等待進行中請求的時間。
		ShutdownTimeout time.Duration `mapstructure:"shutdown_timeout"`
		// MaxBodyBytes 限制 Webhook body，避免超大請求耗盡記憶體。
		MaxBodyBytes int64 `mapstructure:"max_body_bytes"`
	} `mapstructure:"app"`
	// Log 控制 zlogger 的最低等級、輸出目的地與檔案輪轉設定。
	Log LogConfig `mapstructure:"log"`
	// DB 控制 PostgreSQL 連線、連線池及 TLS；URL 可整體覆寫分項設定。
	DB struct {
		// URL 對應 DATABASE_URL，設定後不再組合 Primary 與 TLS 欄位。
		URL string `mapstructure:"url"`
		// Name 是 PostgreSQL 資料庫名稱。
		Name string `mapstructure:"name"`
		// Primary 是目前供讀寫使用的主要資料庫節點。
		Primary struct {
			// Host 是資料庫主機名稱或 IP。
			Host string `mapstructure:"host"`
			// Port 是 PostgreSQL TCP 連接埠。
			Port int `mapstructure:"port"`
			// User 必須透過環境變數或 Secret Manager 提供。
			User string `mapstructure:"user"`
			// Password 必須透過環境變數或 Secret Manager 提供。
			Password string `mapstructure:"password"`
		} `mapstructure:"primary"`
		// Replicas 預留唯讀節點設定；目前查詢仍使用 Primary。
		Replicas []DBEndpoint `mapstructure:"replicas"`
		// MaxOpenConns 限制資料庫開啟中及閒置連線總數。
		MaxOpenConns int `mapstructure:"max_open_conns"`
		// MaxIdleConns 限制連線池保留的閒置連線數。
		MaxIdleConns int `mapstructure:"max_idle_conns"`
		// ConnMaxLifetime 避免長期重用已失效或已輪替的連線。
		ConnMaxLifetime time.Duration `mapstructure:"conn_max_lifetime"`
		// TLS 控制 PostgreSQL 傳輸驗證及用戶端憑證。
		TLS DBTLS `mapstructure:"tls"`
	} `mapstructure:"db"`
	// Telegram 保存 Bot API 與 Webhook 驗證所需秘密值。
	Telegram struct {
		// BotToken 必須透過環境變數或 Secret Manager 提供。
		BotToken string `mapstructure:"bot_token"`
		// WebhookSecret 用於固定時間比較 Telegram Secret Header。
		WebhookSecret string `mapstructure:"webhook_secret"`
		// WebhookURL 是健康檢查用的公開 Webhook URL；留空時只確認 Telegram 已設定 Webhook。
		WebhookURL string `mapstructure:"webhook_url"`
		// AllowedChatIDs 限制可進入偵測流程的 Telegram 群組，避免 Bot 被加入未授權群組後誤執行處置。
		AllowedChatIDs []int64 `mapstructure:"allowed_chat_ids"`
	} `mapstructure:"telegram"`
	// Redis 控制短期頻率與重複內容狀態的儲存位置。
	Redis struct {
		// Addr 是 Redis 的 host:port 連線位址。
		Addr string `mapstructure:"addr"`
		// Username 是 Redis 6+ ACL 帳號；未啟用 ACL 時留空。
		Username string `mapstructure:"username"`
		// Password 是 Redis Client 驗證密碼，必須由環境變數或 Secret Manager 提供。
		Password string `mapstructure:"password"`
		// RequirePass 相容 Redis Server 的 requirepass 命名，Password 未設定時才使用。
		RequirePass string `mapstructure:"requirepass"`
		// DB 是 Redis logical database 編號。
		DB int `mapstructure:"db"`
	} `mapstructure:"redis"`
	// Security 保存不可寫入版本控制的安全設定。
	Security struct {
		// ContentHashKey 用於產生不可逆、有金鑰的訊息內容指紋。
		ContentHashKey string `mapstructure:"content_hash_key"`
	} `mapstructure:"security"`
	// Rules 控制垃圾訊息 YAML 規則的載入位置。
	Rules struct {
		// Dir 是啟動時編譯為不可變規則快照的目錄。
		Dir string `mapstructure:"dir"`
	} `mapstructure:"rules"`
	// AutoReplies 控制固定問答自動回覆規則。
	AutoReplies struct {
		// Enabled 決定是否載入並執行自動回覆規則。
		Enabled bool `mapstructure:"enabled"`
		// RulesFile 是獨立自動回覆 YAML 規則檔路徑。
		RulesFile string `mapstructure:"rules_file"`
	} `mapstructure:"auto_replies"`
	// AIDetection 控制外部 AI classifier 輔助偵測；預設停用。
	AIDetection AIDetectionConfig `mapstructure:"ai_detection"`
	// SemanticMemory 控制 pgvector 語意記憶與 embedding 查詢；預設停用。
	SemanticMemory SemanticMemoryConfig `mapstructure:"semantic_memory"`
}

// LogConfig 描述日誌輸出格式、目的地及檔案輸出相容設定。
type LogConfig struct {
	// Level 支援 debug、info、warn、error 及 fatal。
	Level string `mapstructure:"level"`
	// Format 支援 json 或 console。
	Format string `mapstructure:"format"`
	// Outputs 支援 console、file，可同時輸出。
	Outputs []string `mapstructure:"outputs"`
	// Path 是 file output 使用的 log 目錄。
	Path string `mapstructure:"path"`
	// File 是 file output 使用的檔名；留空時由 logger 使用日期檔名。
	File string `mapstructure:"file"`
	// MaxFiles 是 deprecated 欄位；rotate 未啟用時才保留啟動清理相容行為。
	MaxFiles int `mapstructure:"max_files"`
	// Rotate 控制應用層檔案日誌輪轉。
	Rotate LogRotateConfig `mapstructure:"rotate"`
}

// LogRotateConfig 描述檔案日誌輪轉行為；零值語意需由 EffectiveLogRotate 套用。
type LogRotateConfig struct {
	// Enabled 決定是否啟用應用層檔案日誌輪轉。
	Enabled bool `mapstructure:"enabled"`
	// MaxSizeMB 是單一日誌檔案大小上限；0 表示使用預設 100 MB。
	MaxSizeMB int `mapstructure:"max_size_mb"`
	// MaxBackups 是保留備份數量；0 表示不限制備份數。
	MaxBackups int `mapstructure:"max_backups"`
	// MaxAgeDays 是保留天數；0 表示不依天數刪除。
	MaxAgeDays int `mapstructure:"max_age_days"`
	// Compress 決定是否壓縮輪轉後的舊日誌。
	Compress bool `mapstructure:"compress"`
}

// AIDetectionConfig 描述 AI classifier 輔助判定設定；provider credential 需分開驗證。
type AIDetectionConfig struct {
	// Enabled 決定是否啟用 AI classifier。
	Enabled bool `mapstructure:"enabled"`
	// Mode 限制 AI 結果可影響的處置範圍。
	Mode Mode `mapstructure:"mode"`
	// Provider 決定使用哪個 classifier adapter。
	Provider AIProvider `mapstructure:"provider"`
	// Timeout 限制單次 provider 呼叫時間。
	Timeout time.Duration `mapstructure:"timeout"`
	// MaxTextChars 限制送給 provider 的文字長度。
	MaxTextChars int `mapstructure:"max_text_chars"`
	// MinConfidence 是 spam 結果可被採用的最低信心分數。
	MinConfidence float64 `mapstructure:"min_confidence"`
	// OnlyWhenAmbiguous 限制 AI 只處理規則模糊區間。
	OnlyWhenAmbiguous bool `mapstructure:"only_when_ambiguous"`
	// CacheTTL 控制 AI 判定快取保存時間。
	CacheTTL time.Duration `mapstructure:"cache_ttl"`
	// OpenAICompatible 保存 OpenAI-compatible provider 設定。
	OpenAICompatible OpenAICompatibleConfig `mapstructure:"openai_compatible"`
	// Bedrock 保存 AWS Bedrock provider 設定。
	Bedrock BedrockConfig `mapstructure:"bedrock"`
}

// SemanticMemoryConfig 描述 pgvector 語意記憶與 embedding provider 設定。
type SemanticMemoryConfig struct {
	// Enabled 決定是否啟用語意記憶查詢。
	Enabled bool `mapstructure:"enabled"`
	// EmbeddingProvider 決定使用哪個 embedding adapter。
	EmbeddingProvider AIProvider `mapstructure:"embedding_provider"`
	// EmbeddingVersion 區分同一模型的向量版本或 prompt/schema 版本。
	EmbeddingVersion string `mapstructure:"embedding_version"`
	// EmbeddingDimensions 是 provider 回傳 vector 的預期維度；0 表示 adapter 首次結果決定。
	EmbeddingDimensions int `mapstructure:"embedding_dimensions"`
	// SimilarityThreshold 是一般相似案例門檻。
	SimilarityThreshold float64 `mapstructure:"similarity_threshold"`
	// SpamSimilarityThreshold 是 spam 樣本相似門檻。
	SpamSimilarityThreshold float64 `mapstructure:"spam_similarity_threshold"`
	// HamSimilarityThreshold 是 ham 樣本相似門檻。
	HamSimilarityThreshold float64 `mapstructure:"ham_similarity_threshold"`
	// MaxNeighbors 限制單次相似查詢回傳筆數。
	MaxNeighbors int `mapstructure:"max_neighbors"`
	// CacheTTL 控制 embedding 或相似查詢快取保存時間。
	CacheTTL time.Duration `mapstructure:"cache_ttl"`
	// OpenAICompatible 保存 OpenAI-compatible embedding provider 設定。
	OpenAICompatible OpenAICompatibleConfig `mapstructure:"openai_compatible"`
	// Bedrock 保存 AWS Bedrock embedding provider 設定。
	Bedrock BedrockConfig `mapstructure:"bedrock"`
}

// OpenAICompatibleConfig 描述 OpenAI-compatible HTTP provider 的最小設定。
type OpenAICompatibleConfig struct {
	// Endpoint 是 provider HTTP API base URL。
	Endpoint string `mapstructure:"endpoint"`
	// Model 是 classifier 或 embedding 模型名稱。
	Model string `mapstructure:"model"`
	// APIKey 必須由環境變數或 Secret Manager 注入。
	APIKey string `mapstructure:"api_key"`
}

// BedrockConfig 描述 AWS Bedrock provider 的區域、模型與 credential 模式。
type BedrockConfig struct {
	// Region 是 AWS Bedrock 呼叫區域。
	Region string `mapstructure:"region"`
	// ModelID 是 Bedrock model id。
	ModelID string `mapstructure:"model_id"`
	// AuthMode 決定使用 IAM Role 或 static keys。
	AuthMode BedrockAuthMode `mapstructure:"auth_mode"`
	// AccessKeyID 僅 static_keys 模式需要，必須由環境變數或 Secret Manager 注入。
	AccessKeyID string `mapstructure:"access_key_id"`
	// SecretAccessKey 僅 static_keys 模式需要，必須由環境變數或 Secret Manager 注入。
	SecretAccessKey string `mapstructure:"secret_access_key"`
	// SessionToken 是 AWS temporary credential 的可選 token。
	SessionToken string `mapstructure:"session_token"`
}

// DBEndpoint 描述一個可供資料庫連線使用的節點。
type DBEndpoint struct {
	// Host 是資料庫主機名稱或 IP。
	Host string `mapstructure:"host"`
	// Port 是 PostgreSQL TCP 連接埠。
	Port int `mapstructure:"port"`
	// User 是節點登入帳號。
	User string `mapstructure:"user"`
	// Password 是節點登入密碼，不得寫入版本控制。
	Password string `mapstructure:"password"`
}

// DBTLS 描述 PostgreSQL 伺服器驗證與雙向 TLS 憑證設定。
type DBTLS struct {
	// Enabled 決定是否啟用 PostgreSQL TLS。
	Enabled bool `mapstructure:"enabled"`
	// Mode 對應 PostgreSQL sslmode，例如 verify-full。
	Mode string `mapstructure:"mode"`
	// CACert 是驗證資料庫伺服器憑證的 CA 檔案路徑。
	CACert string `mapstructure:"ca_cert"`
	// ClientCert 是雙向 TLS 用戶端憑證路徑。
	ClientCert string `mapstructure:"client_cert"`
	// ClientKey 是雙向 TLS 私鑰路徑，必須由 Secret 掛載。
	ClientKey string `mapstructure:"client_key"`
}

// Load 依序套用預設值、YAML 與環境變數，最後驗證完整設定。
func Load(path string) (Config, error) {
	v := viper.New()
	setDefaults(v)
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()
	for key, env := range envBindings() {
		if err := v.BindEnv(key, env); err != nil {
			return Config{}, fmt.Errorf("bind %s: %w", key, err)
		}
	}
	if path != "" {
		v.SetConfigFile(path)
		if err := v.ReadInConfig(); err != nil {
			return Config{}, fmt.Errorf("read config: %w", err)
		}
	}
	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return Config{}, fmt.Errorf("decode config: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func setDefaults(v *viper.Viper) {
	v.SetDefault("app.port", 8080)
	v.SetDefault("app.env", "dev")
	v.SetDefault("app.mode", ModeObserve)
	v.SetDefault("app.business_timezone", "Asia/Taipei")
	v.SetDefault("app.read_timeout", 10*time.Second)
	v.SetDefault("app.write_timeout", 30*time.Second)
	v.SetDefault("app.shutdown_timeout", 30*time.Second)
	v.SetDefault("app.max_body_bytes", int64(1<<20))
	v.SetDefault("log.level", "info")
	v.SetDefault("log.format", "json")
	v.SetDefault("log.outputs", []string{"console"})
	v.SetDefault("log.path", "./logs")
	v.SetDefault("log.max_files", 0)
	v.SetDefault("log.rotate.enabled", false)
	v.SetDefault("log.rotate.max_size_mb", 100)
	v.SetDefault("log.rotate.max_backups", 14)
	v.SetDefault("log.rotate.max_age_days", 30)
	v.SetDefault("log.rotate.compress", true)
	v.SetDefault("db.primary.host", "localhost")
	v.SetDefault("db.primary.port", 5432)
	v.SetDefault("db.max_open_conns", 25)
	v.SetDefault("db.max_idle_conns", 5)
	v.SetDefault("db.conn_max_lifetime", 5*time.Minute)
	v.SetDefault("db.tls.mode", "verify-full")
	v.SetDefault("redis.db", 0)
	v.SetDefault("rules.dir", "configs/rules")
	v.SetDefault("auto_replies.enabled", false)
	v.SetDefault("ai_detection.enabled", false)
	v.SetDefault("ai_detection.mode", ModeObserve)
	v.SetDefault("ai_detection.timeout", 3*time.Second)
	v.SetDefault("ai_detection.max_text_chars", 800)
	v.SetDefault("ai_detection.min_confidence", 0.85)
	v.SetDefault("ai_detection.only_when_ambiguous", true)
	v.SetDefault("ai_detection.cache_ttl", 24*time.Hour)
	v.SetDefault("ai_detection.bedrock.auth_mode", BedrockAuthModeIAMRole)
	v.SetDefault("semantic_memory.enabled", false)
	v.SetDefault("semantic_memory.embedding_version", "v1")
	v.SetDefault("semantic_memory.embedding_dimensions", 0)
	v.SetDefault("semantic_memory.similarity_threshold", 0.88)
	v.SetDefault("semantic_memory.spam_similarity_threshold", 0.90)
	v.SetDefault("semantic_memory.ham_similarity_threshold", 0.95)
	v.SetDefault("semantic_memory.max_neighbors", 5)
	v.SetDefault("semantic_memory.cache_ttl", 168*time.Hour)
	v.SetDefault("semantic_memory.bedrock.auth_mode", BedrockAuthModeIAMRole)
}

func envBindings() map[string]string {
	// 這些是環境變數名稱而非憑證內容，實際秘密值只在執行階段注入。
	//nolint:gosec
	return map[string]string{
		"app.addr": "HTTP_ADDR", "app.port": "APP_PORT", "app.env": "APP_ENV", "app.mode": "APP_MODE",
		"app.business_timezone": "BUSINESS_TIMEZONE", "app.read_timeout": "READ_TIMEOUT", "app.write_timeout": "WRITE_TIMEOUT",
		"app.shutdown_timeout": "SHUTDOWN_TIMEOUT", "log.level": "LOG_LEVEL", "log.format": "LOG_FORMAT",
		"log.outputs": "LOG_OUTPUTS", "log.path": "LOG_PATH", "log.file": "LOG_FILE", "log.max_files": "LOG_MAX_FILES",
		"log.rotate.enabled": "LOG_ROTATE_ENABLED", "log.rotate.max_size_mb": "LOG_ROTATE_MAX_SIZE_MB",
		"log.rotate.max_backups": "LOG_ROTATE_MAX_BACKUPS", "log.rotate.max_age_days": "LOG_ROTATE_MAX_AGE_DAYS",
		"log.rotate.compress": "LOG_ROTATE_COMPRESS",
		"db.url":              "DATABASE_URL", "db.name": "DB_NAME", "db.primary.host": "DB_HOST", "db.primary.port": "DB_PORT",
		"db.primary.user": "DB_USER", "db.primary.password": "DB_PASSWORD", "telegram.bot_token": "TELEGRAM_BOT_TOKEN",
		"telegram.webhook_secret": "TELEGRAM_WEBHOOK_SECRET", "telegram.webhook_url": "TELEGRAM_WEBHOOK_URL",
		"telegram.allowed_chat_ids": "TELEGRAM_ALLOWED_CHAT_IDS",
		"redis.addr":                "REDIS_ADDR",
		"redis.username":            "REDIS_USERNAME", "redis.password": "REDIS_PASSWORD", "redis.requirepass": "REDIS_REQUIREPASS", "redis.db": "REDIS_DB",
		"security.content_hash_key": "CONTENT_HASH_KEY", "rules.dir": "RULES_DIR",
		"auto_replies.enabled": "AUTO_REPLIES_ENABLED", "auto_replies.rules_file": "AUTO_REPLIES_RULES_FILE",
		"ai_detection.enabled": "AI_DETECTION_ENABLED", "ai_detection.mode": "AI_DETECTION_MODE", "ai_detection.provider": "AI_DETECTION_PROVIDER",
		"ai_detection.timeout": "AI_DETECTION_TIMEOUT", "ai_detection.max_text_chars": "AI_DETECTION_MAX_TEXT_CHARS",
		"ai_detection.min_confidence": "AI_DETECTION_MIN_CONFIDENCE", "ai_detection.only_when_ambiguous": "AI_DETECTION_ONLY_WHEN_AMBIGUOUS",
		"ai_detection.cache_ttl": "AI_DETECTION_CACHE_TTL", "ai_detection.openai_compatible.endpoint": "AI_DETECTION_OPENAI_COMPATIBLE_ENDPOINT",
		"ai_detection.openai_compatible.model": "AI_DETECTION_OPENAI_COMPATIBLE_MODEL", "ai_detection.openai_compatible.api_key": "AI_DETECTION_OPENAI_COMPATIBLE_API_KEY",
		"ai_detection.bedrock.region": "AI_DETECTION_BEDROCK_REGION", "ai_detection.bedrock.model_id": "AI_DETECTION_BEDROCK_MODEL_ID",
		"ai_detection.bedrock.auth_mode": "AI_DETECTION_BEDROCK_AUTH_MODE", "ai_detection.bedrock.access_key_id": "AI_DETECTION_BEDROCK_ACCESS_KEY_ID",
		"ai_detection.bedrock.secret_access_key": "AI_DETECTION_BEDROCK_SECRET_ACCESS_KEY", "ai_detection.bedrock.session_token": "AI_DETECTION_BEDROCK_SESSION_TOKEN",
		"semantic_memory.enabled": "SEMANTIC_MEMORY_ENABLED", "semantic_memory.embedding_provider": "SEMANTIC_MEMORY_EMBEDDING_PROVIDER",
		"semantic_memory.embedding_version": "SEMANTIC_MEMORY_EMBEDDING_VERSION", "semantic_memory.embedding_dimensions": "SEMANTIC_MEMORY_EMBEDDING_DIMENSIONS",
		"semantic_memory.similarity_threshold":      "SEMANTIC_MEMORY_SIMILARITY_THRESHOLD",
		"semantic_memory.spam_similarity_threshold": "SEMANTIC_MEMORY_SPAM_SIMILARITY_THRESHOLD",
		"semantic_memory.ham_similarity_threshold":  "SEMANTIC_MEMORY_HAM_SIMILARITY_THRESHOLD", "semantic_memory.max_neighbors": "SEMANTIC_MEMORY_MAX_NEIGHBORS",
		"semantic_memory.cache_ttl": "SEMANTIC_MEMORY_CACHE_TTL", "semantic_memory.openai_compatible.endpoint": "SEMANTIC_MEMORY_OPENAI_COMPATIBLE_ENDPOINT",
		"semantic_memory.openai_compatible.model": "SEMANTIC_MEMORY_OPENAI_COMPATIBLE_MODEL", "semantic_memory.openai_compatible.api_key": "SEMANTIC_MEMORY_OPENAI_COMPATIBLE_API_KEY",
		"semantic_memory.bedrock.region": "SEMANTIC_MEMORY_BEDROCK_REGION", "semantic_memory.bedrock.model_id": "SEMANTIC_MEMORY_BEDROCK_MODEL_ID",
		"semantic_memory.bedrock.auth_mode": "SEMANTIC_MEMORY_BEDROCK_AUTH_MODE", "semantic_memory.bedrock.access_key_id": "SEMANTIC_MEMORY_BEDROCK_ACCESS_KEY_ID",
		"semantic_memory.bedrock.secret_access_key": "SEMANTIC_MEMORY_BEDROCK_SECRET_ACCESS_KEY", "semantic_memory.bedrock.session_token": "SEMANTIC_MEMORY_BEDROCK_SESSION_TOKEN",
	}
}

// HTTPAddress 回傳 HTTP Server 實際監聽位址，並維持 HTTP_ADDR 相容性。
func (c Config) HTTPAddress() string {
	if c.App.Addr != "" {
		return c.App.Addr
	}
	return ":" + strconv.Itoa(c.App.Port)
}

// DatabaseURL 回傳 GORM 使用的 PostgreSQL DSN，並安全編碼帳號密碼。
func (c Config) DatabaseURL() string {
	if c.DB.URL != "" {
		return c.DB.URL
	}
	dsn := &url.URL{Scheme: "postgres", Host: net.JoinHostPort(c.DB.Primary.Host, strconv.Itoa(c.DB.Primary.Port)), Path: c.DB.Name}
	dsn.User = url.UserPassword(c.DB.Primary.User, c.DB.Primary.Password)
	query := dsn.Query()
	if c.DB.TLS.Enabled {
		query.Set("sslmode", c.DB.TLS.Mode)
		setQueryIfNotEmpty(query, "sslrootcert", c.DB.TLS.CACert)
		setQueryIfNotEmpty(query, "sslcert", c.DB.TLS.ClientCert)
		setQueryIfNotEmpty(query, "sslkey", c.DB.TLS.ClientKey)
	} else {
		query.Set("sslmode", "disable")
	}
	dsn.RawQuery = query.Encode()
	return dsn.String()
}

// RedisPassword 回傳 Client 使用的密碼，並相容 Redis Server 的 requirepass 命名。
func (c Config) RedisPassword() string {
	if c.Redis.Password != "" {
		return c.Redis.Password
	}
	return c.Redis.RequirePass
}

// EffectiveLogRotate 回傳套用零值語意後的檔案輪轉設定。
func (c Config) EffectiveLogRotate() LogRotateConfig {
	rotate := c.Log.Rotate
	if rotate.MaxSizeMB == 0 {
		rotate.MaxSizeMB = 100
	}
	return rotate
}

func setQueryIfNotEmpty(values url.Values, key, value string) {
	if value != "" {
		values.Set(key, value)
	}
}

// Validate 聚合所有設定錯誤，讓啟動失敗時可一次修正完整缺漏。
func (c Config) Validate() error {
	var errs []error
	if c.App.Mode != ModeObserve && c.App.Mode != ModeDeleteOnly && c.App.Mode != ModeEnforce {
		errs = append(errs, fmt.Errorf("app.mode: unsupported value %q", c.App.Mode))
	}
	if c.App.Addr == "" && (c.App.Port < 1 || c.App.Port > 65535) {
		errs = append(errs, errors.New("app.port: must be between 1 and 65535"))
	}
	if c.App.ReadTimeout <= 0 || c.App.WriteTimeout <= 0 || c.App.ShutdownTimeout <= 0 {
		errs = append(errs, errors.New("app timeouts: must be positive"))
	}
	if c.App.MaxBodyBytes <= 0 {
		errs = append(errs, errors.New("app.max_body_bytes: must be positive"))
	}
	if _, err := time.LoadLocation(c.App.BusinessTimezone); err != nil {
		errs = append(errs, fmt.Errorf("app.business_timezone: %w", err))
	}
	if c.Log.Level == "" || (c.Log.Format != "json" && c.Log.Format != "console") {
		errs = append(errs, errors.New("log: level is required and format must be json or console"))
	}
	if len(c.Log.Outputs) == 0 {
		errs = append(errs, errors.New("log.outputs: at least one output is required"))
	}
	for _, output := range c.Log.Outputs {
		if output != "console" && output != "file" {
			errs = append(errs, fmt.Errorf("log.outputs: unsupported output %q", output))
		}
	}
	if c.Log.MaxFiles < 0 {
		errs = append(errs, errors.New("log.max_files: must not be negative"))
	}
	if c.Log.Rotate.MaxSizeMB < 0 {
		errs = append(errs, errors.New("log.rotate.max_size_mb: must not be negative"))
	}
	if c.Log.Rotate.MaxBackups < 0 {
		errs = append(errs, errors.New("log.rotate.max_backups: must not be negative"))
	}
	if c.Log.Rotate.MaxAgeDays < 0 {
		errs = append(errs, errors.New("log.rotate.max_age_days: must not be negative"))
	}
	if c.DB.URL == "" && (c.DB.Name == "" || c.DB.Primary.Host == "" || c.DB.Primary.User == "" || c.DB.Primary.Password == "") {
		errs = append(errs, errors.New("db: name, primary host, user and password are required when DATABASE_URL is empty"))
	}
	if c.DB.MaxOpenConns <= 0 || c.DB.MaxIdleConns < 0 || c.DB.MaxIdleConns > c.DB.MaxOpenConns || c.DB.ConnMaxLifetime <= 0 {
		errs = append(errs, errors.New("db connection pool: invalid limits"))
	}
	if c.Telegram.BotToken == "" || c.Telegram.WebhookSecret == "" {
		errs = append(errs, errors.New("telegram: bot_token and webhook_secret are required"))
	}
	if len(c.Telegram.AllowedChatIDs) == 0 {
		errs = append(errs, errors.New("telegram.allowed_chat_ids: at least one chat id is required"))
	}
	seenChatIDs := make(map[int64]struct{}, len(c.Telegram.AllowedChatIDs))
	for _, chatID := range c.Telegram.AllowedChatIDs {
		if chatID == 0 {
			errs = append(errs, errors.New("telegram.allowed_chat_ids: chat id must not be zero"))
			continue
		}
		if _, exists := seenChatIDs[chatID]; exists {
			errs = append(errs, fmt.Errorf("telegram.allowed_chat_ids: duplicate chat id %d", chatID))
			continue
		}
		seenChatIDs[chatID] = struct{}{}
	}
	if c.Redis.Addr == "" {
		errs = append(errs, errors.New("redis.addr: required"))
	}
	if c.Redis.DB < 0 {
		errs = append(errs, errors.New("redis.db: must not be negative"))
	}
	if len(c.Security.ContentHashKey) < 32 {
		errs = append(errs, errors.New("security.content_hash_key: must contain at least 32 characters"))
	}
	if c.Rules.Dir == "" {
		errs = append(errs, errors.New("rules.dir: required"))
	}
	if c.AutoReplies.Enabled && strings.TrimSpace(c.AutoReplies.RulesFile) == "" {
		errs = append(errs, errors.New("auto_replies.rules_file: required when auto replies are enabled"))
	}
	errs = append(errs, validateAIDetection(c.AIDetection)...)
	errs = append(errs, validateSemanticMemory(c.SemanticMemory)...)
	return errors.Join(errs...)
}

func validateAIDetection(cfg AIDetectionConfig) []error {
	var errs []error
	if cfg.Mode != ModeObserve && cfg.Mode != ModeDeleteOnly && cfg.Mode != ModeEnforce {
		errs = append(errs, fmt.Errorf("ai_detection.mode: unsupported value %q", cfg.Mode))
	}
	if cfg.Timeout <= 0 {
		errs = append(errs, errors.New("ai_detection.timeout: must be positive"))
	}
	if cfg.MaxTextChars <= 0 {
		errs = append(errs, errors.New("ai_detection.max_text_chars: must be positive"))
	}
	if cfg.MinConfidence < 0 || cfg.MinConfidence > 1 {
		errs = append(errs, errors.New("ai_detection.min_confidence: must be between 0 and 1"))
	}
	if cfg.CacheTTL <= 0 {
		errs = append(errs, errors.New("ai_detection.cache_ttl: must be positive"))
	}
	if !cfg.Enabled {
		return errs
	}
	if cfg.Provider == "" {
		errs = append(errs, errors.New("ai_detection.provider: required when AI detection is enabled"))
		return errs
	}
	return append(errs, validateProviderConfig("ai_detection", cfg.Provider, cfg.OpenAICompatible, cfg.Bedrock)...)
}

func validateSemanticMemory(cfg SemanticMemoryConfig) []error {
	var errs []error
	if cfg.EmbeddingDimensions < 0 {
		errs = append(errs, errors.New("semantic_memory.embedding_dimensions: must not be negative"))
	}
	for name, threshold := range map[string]float64{
		"similarity_threshold":      cfg.SimilarityThreshold,
		"spam_similarity_threshold": cfg.SpamSimilarityThreshold,
		"ham_similarity_threshold":  cfg.HamSimilarityThreshold,
	} {
		if threshold < 0 || threshold > 1 {
			errs = append(errs, fmt.Errorf("semantic_memory.%s: must be between 0 and 1", name))
		}
	}
	if cfg.MaxNeighbors <= 0 {
		errs = append(errs, errors.New("semantic_memory.max_neighbors: must be positive"))
	}
	if cfg.CacheTTL <= 0 {
		errs = append(errs, errors.New("semantic_memory.cache_ttl: must be positive"))
	}
	if !cfg.Enabled {
		return errs
	}
	if cfg.EmbeddingProvider == "" {
		errs = append(errs, errors.New("semantic_memory.embedding_provider: required when semantic memory is enabled"))
		return errs
	}
	return append(errs, validateProviderConfig("semantic_memory", cfg.EmbeddingProvider, cfg.OpenAICompatible, cfg.Bedrock)...)
}

func validateProviderConfig(prefix string, provider AIProvider, openAI OpenAICompatibleConfig, bedrock BedrockConfig) []error {
	switch provider {
	case AIProviderOpenAICompatible:
		return validateOpenAICompatibleConfig(prefix+".openai_compatible", openAI)
	case AIProviderBedrock:
		return validateBedrockConfig(prefix+".bedrock", bedrock)
	default:
		return []error{fmt.Errorf("%s.provider: unsupported value %q", prefix, provider)}
	}
}

func validateOpenAICompatibleConfig(prefix string, cfg OpenAICompatibleConfig) []error {
	var errs []error
	if strings.TrimSpace(cfg.Endpoint) == "" {
		errs = append(errs, fmt.Errorf("%s.endpoint: required", prefix))
	}
	if strings.TrimSpace(cfg.Model) == "" {
		errs = append(errs, fmt.Errorf("%s.model: required", prefix))
	}
	if strings.TrimSpace(cfg.APIKey) == "" {
		errs = append(errs, fmt.Errorf("%s.api_key: required", prefix))
	}
	return errs
}

func validateBedrockConfig(prefix string, cfg BedrockConfig) []error {
	var errs []error
	if strings.TrimSpace(cfg.Region) == "" {
		errs = append(errs, fmt.Errorf("%s.region: required", prefix))
	}
	if strings.TrimSpace(cfg.ModelID) == "" {
		errs = append(errs, fmt.Errorf("%s.model_id: required", prefix))
	}
	switch cfg.AuthMode {
	case BedrockAuthModeIAMRole:
	case BedrockAuthModeStaticKeys:
		if strings.TrimSpace(cfg.AccessKeyID) == "" {
			errs = append(errs, fmt.Errorf("%s.access_key_id: required for static_keys", prefix))
		}
		if strings.TrimSpace(cfg.SecretAccessKey) == "" {
			errs = append(errs, fmt.Errorf("%s.secret_access_key: required for static_keys", prefix))
		}
	default:
		errs = append(errs, fmt.Errorf("%s.auth_mode: unsupported value %q", prefix, cfg.AuthMode))
	}
	return errs
}
