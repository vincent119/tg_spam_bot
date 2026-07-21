// Package main 組裝 Telegram 垃圾訊息偵測服務及其完整生命週期。
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	redislib "github.com/redis/go-redis/v9"
	autoreplyapp "github.com/vincent119/tg_spam_bot/internal/autoreply/application"
	autoreplyrules "github.com/vincent119/tg_spam_bot/internal/autoreply/rules"
	commandapp "github.com/vincent119/tg_spam_bot/internal/command/application"
	commandredis "github.com/vincent119/tg_spam_bot/internal/command/infra/redis"
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
	os.Exit(execute())
}

// execute 回傳程序退出碼，讓所有已註冊 defer 在 main 呼叫 os.Exit 前完成。
func execute() int {
	// 先驗證完整設定，避免 logger 初始化後無法套用 YAML 的等級與格式。
	cfg, err := config.Load(os.Getenv("CONFIG_FILE"))
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		return 1
	}
	syncLogger, err := initializeLogger(cfg)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer func() { _ = syncLogger() }()
	if err := run(cfg); err != nil {
		zlogger.Error("應用程式結束", zlogger.Err(err))
		return 1
	}
	return 0
}

func pruneLogFiles(dir string, maxFiles int) error {
	if maxFiles <= 0 {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("read log directory: %w", err)
	}
	type logFile struct {
		path    string
		modTime time.Time
		name    string
	}
	files := make([]logFile, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".log" {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("stat log file %s: %w", entry.Name(), err)
		}
		if !info.Mode().IsRegular() {
			continue
		}
		files = append(files, logFile{path: filepath.Join(dir, entry.Name()), modTime: info.ModTime(), name: entry.Name()})
	}
	slices.SortFunc(files, func(a, b logFile) int {
		if !a.modTime.Equal(b.modTime) {
			if a.modTime.After(b.modTime) {
				return -1
			}
			return 1
		}
		return strings.Compare(a.name, b.name)
	})
	if len(files) <= maxFiles {
		return nil
	}
	for _, file := range files[maxFiles:] {
		if err := os.Remove(file.path); err != nil {
			return fmt.Errorf("remove old log file %s: %w", file.path, err)
		}
	}
	return nil
}

func run(cfg config.Config) error {
	// 根 context 統一傳遞終止訊號，確保外部 I/O 與 HTTP Server 能依序收斂。
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	ruleSet, err := rules.LoadDir(cfg.Rules.Dir)
	if err != nil {
		return err
	}
	normalizer := domain.NewNormalizer(domain.OpenCCConverter{}, 8192)
	detector, err := domain.NewDetector(ruleSet, normalizer, nil, nil)
	if err != nil {
		return err
	}
	db, err := gorm.Open(postgres.Open(cfg.DatabaseURL()), &gorm.Config{})
	if err != nil {
		return err
	}
	// 新環境只需預先建立 database；資料表、索引與註解由模型統一建立。
	if err := pgstore.AutoMigrate(ctx, db); err != nil {
		return err
	}
	zlogger.InfoContext(ctx, "資料庫結構同步完成",
		zlogger.String("subsystem", "database"),
		zlogger.String("operation", "auto_migrate"),
	)
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
	redisClient := redislib.NewClient(&redislib.Options{
		Addr:     cfg.Redis.Addr,
		Username: cfg.Redis.Username,
		Password: cfg.RedisPassword(),
		DB:       cfg.Redis.DB,
	})
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
	identity, err := telegram.GetMe(ctx)
	if err != nil {
		return fmt.Errorf("驗證 Telegram Bot 身分：%w", err)
	}
	telegramHealth, err := newTelegramHealthMonitor(telegram, cfg.Telegram.AllowedChatIDs, cfg.Telegram.WebhookURL)
	if err != nil {
		return err
	}
	if err := telegramHealth.checkWithTimeout(ctx); err != nil {
		return err
	}
	go telegramHealth.start(ctx)
	exemptions, err := application.NewCachedExemptions(postgresStore, telegram, 5*time.Minute)
	if err != nil {
		return err
	}
	processor := application.NewProcessor(detector, postgresStore, exemptions, behaviors, postgresStore, telegram, application.Mode(cfg.App.Mode), []byte(cfg.Security.ContentHashKey))
	commandLimiter, err := commandredis.NewLimiter(redisClient, 5, 30*time.Second)
	if err != nil {
		return err
	}
	commandHandler, err := commandapp.NewHandler(telegram, postgresStore, postgresStore, commandLimiter, identity.ID)
	if err != nil {
		return err
	}
	var autoReplyProcessor *autoreplyapp.Processor
	if cfg.AutoReplies.Enabled {
		autoReplyRules, err := autoreplyrules.LoadFile(cfg.AutoReplies.RulesFile)
		if err != nil {
			return err
		}
		autoReplyProcessor, err = autoreplyapp.NewProcessor(autoreplyapp.NewMatcher(autoReplyRules, normalizer), postgresStore, telegram)
		if err != nil {
			return err
		}
	}
	webhookOptions := []delivery.Option{
		delivery.WithAllowedChatIDs(cfg.Telegram.AllowedChatIDs),
		delivery.WithCommandProcessor(commandHandler, identity.Username),
	}
	if autoReplyProcessor != nil {
		webhookOptions = append(webhookOptions, delivery.WithAutoReplyProcessor(autoReplyProcessor))
	}
	webhook, err := delivery.NewWebhook(cfg.Telegram.WebhookSecret, cfg.App.MaxBodyBytes, processor, webhookOptions...)
	if err != nil {
		return err
	}

	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery())
	router.POST("/telegram/webhook", func(c *gin.Context) { webhook.ServeHTTP(c.Writer, c.Request) })
	router.GET("/health/live", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	router.GET("/health/ready", func(c *gin.Context) {
		// 就緒狀態必須同時反映永久資料與短期行為狀態的可用性。
		checkCtx, cancel := context.WithTimeout(c.Request.Context(), time.Second)
		defer cancel()
		if err := sqlDB.PingContext(checkCtx); err != nil || redisClient.Ping(checkCtx).Err() != nil {
			c.String(http.StatusServiceUnavailable, "not ready")
			return
		}
		if err := telegramHealth.lastErr(); err != nil {
			c.String(http.StatusServiceUnavailable, "telegram not ready")
			return
		}
		c.Status(http.StatusNoContent)
	})
	server := &http.Server{Addr: cfg.HTTPAddress(), Handler: router, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: cfg.App.ReadTimeout, WriteTimeout: cfg.App.WriteTimeout, IdleTimeout: 60 * time.Second}
	errCh := make(chan error, 1)
	go func() { errCh <- server.ListenAndServe() }()
	zlogger.InfoContext(ctx, "伺服器已啟動", zlogger.String("subsystem", "application"), zlogger.String("addr", cfg.HTTPAddress()), zlogger.String("mode", string(cfg.App.Mode)), zlogger.String("env", cfg.App.Env), zlogger.String("rule_version", ruleSet.Version))

	select {
	case <-ctx.Done():
		// 停止接受新請求後，允許進行中的 Webhook 在設定期限內完成。
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
