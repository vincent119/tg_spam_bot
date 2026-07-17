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

type Mode string

const (
	ModeObserve    Mode = "observe"
	ModeDeleteOnly Mode = "delete-only"
	ModeEnforce    Mode = "enforce"
)

type Config struct {
	App struct {
		Port             int           `mapstructure:"port"`
		Addr             string        `mapstructure:"addr"`
		Env              string        `mapstructure:"env"`
		Mode             Mode          `mapstructure:"mode"`
		BusinessTimezone string        `mapstructure:"business_timezone"`
		ReadTimeout      time.Duration `mapstructure:"read_timeout"`
		WriteTimeout     time.Duration `mapstructure:"write_timeout"`
		ShutdownTimeout  time.Duration `mapstructure:"shutdown_timeout"`
		MaxBodyBytes     int64         `mapstructure:"max_body_bytes"`
	} `mapstructure:"app"`
	Log struct {
		Level  string `mapstructure:"level"`
		Format string `mapstructure:"format"`
	} `mapstructure:"log"`
	DB struct {
		URL     string `mapstructure:"url"`
		Name    string `mapstructure:"name"`
		Primary struct {
			Host     string `mapstructure:"host"`
			Port     int    `mapstructure:"port"`
			User     string `mapstructure:"user"`
			Password string `mapstructure:"password"`
		} `mapstructure:"primary"`
		Replicas        []DBEndpoint  `mapstructure:"replicas"`
		MaxOpenConns    int           `mapstructure:"max_open_conns"`
		MaxIdleConns    int           `mapstructure:"max_idle_conns"`
		ConnMaxLifetime time.Duration `mapstructure:"conn_max_lifetime"`
		TLS             DBTLS         `mapstructure:"tls"`
	} `mapstructure:"db"`
	Telegram struct {
		BotToken      string `mapstructure:"bot_token"`
		WebhookSecret string `mapstructure:"webhook_secret"`
	} `mapstructure:"telegram"`
	Redis struct {
		Addr string `mapstructure:"addr"`
	} `mapstructure:"redis"`
	Security struct {
		ContentHashKey string `mapstructure:"content_hash_key"`
	} `mapstructure:"security"`
	Rules struct {
		Dir string `mapstructure:"dir"`
	} `mapstructure:"rules"`
}

type DBEndpoint struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
}

type DBTLS struct {
	Enabled    bool   `mapstructure:"enabled"`
	Mode       string `mapstructure:"mode"`
	CACert     string `mapstructure:"ca_cert"`
	ClientCert string `mapstructure:"client_cert"`
	ClientKey  string `mapstructure:"client_key"`
}

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
	v.SetDefault("rules.dir", "configs/rules")
}

func envBindings() map[string]string {
	return map[string]string{
		"app.addr": "HTTP_ADDR", "app.port": "APP_PORT", "app.env": "APP_ENV", "app.mode": "APP_MODE",
		"app.business_timezone": "BUSINESS_TIMEZONE", "app.read_timeout": "READ_TIMEOUT", "app.write_timeout": "WRITE_TIMEOUT",
		"app.shutdown_timeout": "SHUTDOWN_TIMEOUT", "log.level": "LOG_LEVEL", "log.format": "LOG_FORMAT",
		"db.url": "DATABASE_URL", "db.name": "DB_NAME", "db.primary.host": "DB_HOST", "db.primary.port": "DB_PORT",
		"db.primary.user": "DB_USER", "db.primary.password": "DB_PASSWORD", "telegram.bot_token": "TELEGRAM_BOT_TOKEN",
		"telegram.webhook_secret": "TELEGRAM_WEBHOOK_SECRET", "redis.addr": "REDIS_ADDR",
		"security.content_hash_key": "CONTENT_HASH_KEY", "rules.dir": "RULES_DIR",
	}
}

func (c Config) HTTPAddress() string {
	if c.App.Addr != "" {
		return c.App.Addr
	}
	return ":" + strconv.Itoa(c.App.Port)
}

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

func setQueryIfNotEmpty(values url.Values, key, value string) {
	if value != "" {
		values.Set(key, value)
	}
}

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
	if c.Redis.Addr == "" {
		errs = append(errs, errors.New("redis.addr: required"))
	}
	if len(c.Security.ContentHashKey) < 32 {
		errs = append(errs, errors.New("security.content_hash_key: must contain at least 32 characters"))
	}
	if c.Rules.Dir == "" {
		errs = append(errs, errors.New("rules.dir: required"))
	}
	return errors.Join(errs...)
}
