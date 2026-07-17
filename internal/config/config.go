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
	// Log 控制 zlogger 的最低等級與輸出編碼格式。
	Log struct {
		// Level 支援 debug、info、warn、error 及 fatal。
		Level string `mapstructure:"level"`
		// Format 支援 json 或 console。
		Format string `mapstructure:"format"`
	} `mapstructure:"log"`
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
	v.SetDefault("db.primary.host", "localhost")
	v.SetDefault("db.primary.port", 5432)
	v.SetDefault("db.max_open_conns", 25)
	v.SetDefault("db.max_idle_conns", 5)
	v.SetDefault("db.conn_max_lifetime", 5*time.Minute)
	v.SetDefault("db.tls.mode", "verify-full")
	v.SetDefault("redis.db", 0)
	v.SetDefault("rules.dir", "configs/rules")
}

func envBindings() map[string]string {
	// 這些是環境變數名稱而非憑證內容，實際秘密值只在執行階段注入。
	//nolint:gosec
	return map[string]string{
		"app.addr": "HTTP_ADDR", "app.port": "APP_PORT", "app.env": "APP_ENV", "app.mode": "APP_MODE",
		"app.business_timezone": "BUSINESS_TIMEZONE", "app.read_timeout": "READ_TIMEOUT", "app.write_timeout": "WRITE_TIMEOUT",
		"app.shutdown_timeout": "SHUTDOWN_TIMEOUT", "log.level": "LOG_LEVEL", "log.format": "LOG_FORMAT",
		"db.url": "DATABASE_URL", "db.name": "DB_NAME", "db.primary.host": "DB_HOST", "db.primary.port": "DB_PORT",
		"db.primary.user": "DB_USER", "db.primary.password": "DB_PASSWORD", "telegram.bot_token": "TELEGRAM_BOT_TOKEN",
		"telegram.webhook_secret": "TELEGRAM_WEBHOOK_SECRET", "telegram.allowed_chat_ids": "TELEGRAM_ALLOWED_CHAT_IDS",
		"redis.addr":     "REDIS_ADDR",
		"redis.username": "REDIS_USERNAME", "redis.password": "REDIS_PASSWORD", "redis.requirepass": "REDIS_REQUIREPASS", "redis.db": "REDIS_DB",
		"security.content_hash_key": "CONTENT_HASH_KEY", "rules.dir": "RULES_DIR",
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
	return errors.Join(errs...)
}
