package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	redislib "github.com/redis/go-redis/v9"
	"github.com/vincent119/tg_spam_bot/internal/config"
	"github.com/vincent119/tg_spam_bot/internal/detection/application"
	delivery "github.com/vincent119/tg_spam_bot/internal/detection/delivery/telegram"
	"github.com/vincent119/tg_spam_bot/internal/detection/domain"
	pgstore "github.com/vincent119/tg_spam_bot/internal/detection/infra/postgres"
	redisstore "github.com/vincent119/tg_spam_bot/internal/detection/infra/redis"
	tgclient "github.com/vincent119/tg_spam_bot/internal/detection/infra/telegram"
	"github.com/vincent119/tg_spam_bot/internal/detection/rules"
	"github.com/vincent119/zlogger"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func main() {
	cfg, err := config.Load(os.Getenv("CONFIG_FILE"))
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	zlogger.Init(&zlogger.Config{Level: cfg.Log.Level, Format: cfg.Log.Format, Outputs: []string{"console"}, AddCaller: true})
	defer func() { _ = zlogger.Sync() }()
	if err := run(cfg); err != nil {
		zlogger.Error("應用程式結束", zlogger.Err(err))
		os.Exit(1)
	}
}

func run(cfg config.Config) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	ruleSet, err := rules.LoadDir(cfg.Rules.Dir)
	if err != nil {
		return err
	}
	detector, err := domain.NewDetector(ruleSet, domain.NewNormalizer(domain.OpenCCConverter{}, 8192), nil, nil)
	if err != nil {
		return err
	}
	db, err := gorm.Open(postgres.Open(cfg.DatabaseURL()), &gorm.Config{})
	if err != nil {
		return err
	}
	if err := pgstore.AutoMigrate(ctx, db); err != nil {
		return err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	sqlDB.SetMaxOpenConns(cfg.DB.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.DB.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(cfg.DB.ConnMaxLifetime)
	defer func() {
		if closeErr := sqlDB.Close(); closeErr != nil {
			zlogger.Error("關閉資料庫失敗", zlogger.Err(closeErr))
		}
	}()
	postgresStore, err := pgstore.NewStore(db)
	if err != nil {
		return err
	}
	redisClient := redislib.NewClient(&redislib.Options{Addr: cfg.Redis.Addr})
	defer func() {
		if closeErr := redisClient.Close(); closeErr != nil {
			zlogger.Error("關閉 Redis Client 失敗", zlogger.Err(closeErr))
		}
	}()
	if err := redisClient.Ping(ctx).Err(); err != nil {
		return err
	}
	behaviors, err := redisstore.NewBehaviorStore(redisClient, time.Minute)
	if err != nil {
		return err
	}
	telegram, err := tgclient.NewClient("https://api.telegram.org", cfg.Telegram.BotToken, nil)
	if err != nil {
		return err
	}
	exemptions, err := application.NewCachedExemptions(postgresStore, telegram, 5*time.Minute)
	if err != nil {
		return err
	}
	processor := application.NewProcessor(detector, postgresStore, exemptions, behaviors, postgresStore, telegram, application.Mode(cfg.App.Mode), []byte(cfg.Security.ContentHashKey))
	webhook, err := delivery.NewWebhook(cfg.Telegram.WebhookSecret, cfg.App.MaxBodyBytes, processor)
	if err != nil {
		return err
	}

	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery())
	router.POST("/telegram/webhook", func(c *gin.Context) { webhook.ServeHTTP(c.Writer, c.Request) })
	router.GET("/health/live", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	router.GET("/health/ready", func(c *gin.Context) {
		checkCtx, cancel := context.WithTimeout(c.Request.Context(), time.Second)
		defer cancel()
		if err := sqlDB.PingContext(checkCtx); err != nil || redisClient.Ping(checkCtx).Err() != nil {
			c.String(http.StatusServiceUnavailable, "not ready")
			return
		}
		c.Status(http.StatusNoContent)
	})
	server := &http.Server{Addr: cfg.HTTPAddress(), Handler: router, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: cfg.App.ReadTimeout, WriteTimeout: cfg.App.WriteTimeout, IdleTimeout: 60 * time.Second}
	errCh := make(chan error, 1)
	go func() { errCh <- server.ListenAndServe() }()
	zlogger.InfoContext(ctx, "伺服器已啟動", zlogger.String("addr", cfg.HTTPAddress()), zlogger.String("mode", string(cfg.App.Mode)), zlogger.String("env", cfg.App.Env), zlogger.String("rule_version", ruleSet.Version))

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.App.ShutdownTimeout)
		defer cancel()
		return server.Shutdown(shutdownCtx)
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}
