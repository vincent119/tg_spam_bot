// Package main 組裝 Telegram 垃圾訊息偵測服務及其完整生命週期。
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	redislib "github.com/redis/go-redis/v9"
	"github.com/vincent119/commons/graceful"
	autoreplyapp "github.com/vincent119/tg_spam_bot/internal/autoreply/application"
	autoreplyrules "github.com/vincent119/tg_spam_bot/internal/autoreply/rules"
	commandapp "github.com/vincent119/tg_spam_bot/internal/command/application"
	commandredis "github.com/vincent119/tg_spam_bot/internal/command/infra/redis"
	"github.com/vincent119/tg_spam_bot/internal/config"
	detectiondi "github.com/vincent119/tg_spam_bot/internal/detection"
	"github.com/vincent119/tg_spam_bot/internal/detection/application"
	delivery "github.com/vincent119/tg_spam_bot/internal/detection/delivery/telegram"
	"github.com/vincent119/tg_spam_bot/internal/detection/domain"
	pgstore "github.com/vincent119/tg_spam_bot/internal/detection/infra/postgres"
	redisstore "github.com/vincent119/tg_spam_bot/internal/detection/infra/redis"
	tgclient "github.com/vincent119/tg_spam_bot/internal/detection/infra/telegram"
	"github.com/vincent119/tg_spam_bot/internal/detection/rules"
	"github.com/vincent119/tg_spam_bot/internal/infra/health"
	"github.com/vincent119/tg_spam_bot/internal/infra/logging"
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
	syncLogger, err := logging.Initialize(cfg)
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

func run(cfg config.Config) error {
	startupCtx := context.Background()

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
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	sqlDB.SetMaxOpenConns(cfg.DB.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.DB.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(cfg.DB.ConnMaxLifetime)
	dbManagedByGraceful := false
	defer func() {
		if !dbManagedByGraceful {
			_ = sqlDB.Close()
		}
	}()
	// 新環境只需預先建立 database；資料表、索引與註解由模型統一建立。
	if err := pgstore.AutoMigrate(startupCtx, db); err != nil {
		return err
	}
	if cfg.SemanticMemory.Enabled {
		if err := pgstore.AutoMigrateSemanticMemory(startupCtx, db); err != nil {
			return err
		}
	}
	zlogger.InfoContext(startupCtx, "資料庫結構同步完成",
		zlogger.String("subsystem", "database"),
		zlogger.String("operation", "auto_migrate"),
	)
	postgresStore, err := pgstore.NewStore(db)
	if err != nil {
		return err
	}
	aiComponents, err := detectiondi.BuildAIComponents(startupCtx, cfg, postgresStore)
	if err != nil {
		return err
	}
	if err := detectiondi.ValidateAIComponents(cfg, aiComponents); err != nil {
		return err
	}
	redisClient := redislib.NewClient(&redislib.Options{
		Addr:     cfg.Redis.Addr,
		Username: cfg.Redis.Username,
		Password: cfg.RedisPassword(),
		DB:       cfg.Redis.DB,
	})
	redisManagedByGraceful := false
	defer func() {
		if !redisManagedByGraceful {
			_ = redisClient.Close()
		}
	}()
	if err := redisClient.Ping(startupCtx).Err(); err != nil {
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
	identity, err := telegram.GetMe(startupCtx)
	if err != nil {
		return fmt.Errorf("驗證 Telegram Bot 身分：%w", err)
	}
	telegramHealth, err := health.NewTelegramMonitor(telegram, cfg.Telegram.AllowedChatIDs, cfg.Telegram.WebhookURL)
	if err != nil {
		return err
	}
	if err := telegramHealth.CheckWithTimeout(startupCtx); err != nil {
		return err
	}
	exemptions, err := application.NewCachedExemptions(postgresStore, telegram, 5*time.Minute)
	if err != nil {
		return err
	}
	processorOptions := []application.ProcessorOption{}
	if aiComponents.Processor != nil {
		processorOptions = append(processorOptions, application.WithAIDetectionProcessor(aiComponents.Processor))
	}
	processor := application.NewProcessor(detector, postgresStore, exemptions, behaviors, postgresStore, telegram, application.Mode(cfg.App.Mode), []byte(cfg.Security.ContentHashKey), processorOptions...)
	commandLimiter, err := commandredis.NewLimiter(redisClient, 5, 30*time.Second)
	if err != nil {
		return err
	}
	commandOptions := []commandapp.Option{}
	if aiComponents.FeedSpamService != nil {
		commandOptions = append(commandOptions, commandapp.WithFeedSpamSubmitter(aiComponents.FeedSpamService, []byte(cfg.Security.ContentHashKey), cfg.AIDetection.MaxTextChars, cfg.SemanticMemory.CacheTTL))
	}
	commandHandler, err := commandapp.NewHandler(telegram, postgresStore, postgresStore, commandLimiter, identity.ID, commandOptions...)
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
		if err := telegramHealth.LastErr(); err != nil {
			c.String(http.StatusServiceUnavailable, "telegram not ready")
			return
		}
		c.Status(http.StatusNoContent)
	})
	server := &http.Server{Addr: cfg.HTTPAddress(), Handler: router, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: cfg.App.ReadTimeout, WriteTimeout: cfg.App.WriteTimeout, IdleTimeout: 60 * time.Second}
	task := func(ctx context.Context) error {
		go telegramHealth.Start(ctx)
		zlogger.InfoContext(ctx, "伺服器已啟動", zlogger.String("subsystem", "application"), zlogger.String("addr", cfg.HTTPAddress()), zlogger.String("mode", string(cfg.App.Mode)), zlogger.String("env", cfg.App.Env), zlogger.String("rule_version", ruleSet.Version))
		return graceful.HTTPTask(server)(ctx)
	}
	dbManagedByGraceful = true
	redisManagedByGraceful = true
	return graceful.Run(
		task,
		graceful.WithCloser(sqlDB),
		graceful.WithCloser(redisClient),
		graceful.WithCleanup(func(ctx context.Context) error {
			// 必須先停止接收新請求，再關閉資料庫與 Redis。
			return server.Shutdown(ctx)
		}),
		graceful.WithTimeout(cfg.App.ShutdownTimeout),
	)
}
