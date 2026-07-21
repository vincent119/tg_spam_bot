package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/vincent119/tg_spam_bot/internal/config"
	"github.com/vincent119/zlogger"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

type loggerSync func() error

func initializeLogger(cfg config.Config) (loggerSync, error) {
	if !cfg.Log.Rotate.Enabled && cfg.Log.MaxFiles > 0 {
		if err := pruneLogFiles(cfg.Log.Path, cfg.Log.MaxFiles); err != nil {
			return nil, err
		}
	}
	if cfg.Log.Rotate.Enabled && containsOutput(cfg.Log.Outputs, "file") {
		return initializeRotatingLogger(cfg.Log)
	}
	zlogger.Init(&zlogger.Config{Level: cfg.Log.Level, Format: cfg.Log.Format, Outputs: cfg.Log.Outputs, LogPath: cfg.Log.Path, FileName: cfg.Log.File, AddCaller: true})
	return zlogger.Sync, nil
}

func initializeRotatingLogger(cfg config.LogConfig) (loggerSync, error) {
	core, err := buildRotatingCore(cfg)
	if err != nil {
		return nil, err
	}
	// zlogger 的 facade 使用套件內私有 globalLogger；先初始化再替換同一指標的 core。
	zlogger.Init(&zlogger.Config{Level: cfg.Level, Format: cfg.Format, Outputs: []string{"console"}, AddCaller: true})
	base := zlogger.GetLogger()
	if base == nil {
		return nil, fmt.Errorf("initialize zlogger facade")
	}
	wrapped := base.WithOptions(zap.WrapCore(func(zapcore.Core) zapcore.Core {
		return core
	}))
	*base = *wrapped
	zap.ReplaceGlobals(base)
	zlogger.Info("logger initialized",
		zlogger.String("level", cfg.Level),
		zlogger.String("format", cfg.Format),
		zlogger.Strings("outputs", cfg.Outputs),
		zlogger.String("path", cfg.Path),
		zlogger.String("file", cfg.File),
		zlogger.Bool("rotate", true),
	)
	return base.Sync, nil
}

func buildRotatingCore(cfg config.LogConfig) (zapcore.Core, error) {
	encoderConfig := zapcore.EncoderConfig{
		TimeKey:          "ts",
		LevelKey:         "level",
		NameKey:          "logger",
		CallerKey:        "caller",
		FunctionKey:      zapcore.OmitKey,
		MessageKey:       "msg",
		StacktraceKey:    "stacktrace",
		LineEnding:       zapcore.DefaultLineEnding,
		EncodeLevel:      zapcore.CapitalLevelEncoder,
		EncodeTime:       zapcore.ISO8601TimeEncoder,
		EncodeDuration:   zapcore.StringDurationEncoder,
		EncodeCaller:     zapcore.ShortCallerEncoder,
		ConsoleSeparator: " ",
	}
	level := zap.NewAtomicLevelAt(parseLogLevel(cfg.Level))
	cores := make([]zapcore.Core, 0, len(cfg.Outputs))
	for _, output := range cfg.Outputs {
		switch strings.ToLower(output) {
		case "console":
			cores = append(cores, zapcore.NewCore(newLogEncoder(cfg.Format, encoderConfig), zapcore.Lock(os.Stdout), level))
		case "file":
			writer, err := rotatingLogWriter(cfg)
			if err != nil {
				return nil, err
			}
			cores = append(cores, zapcore.NewCore(newLogEncoder(cfg.Format, encoderConfig), zapcore.AddSync(writer), level))
		}
	}
	if len(cores) == 0 {
		cores = append(cores, zapcore.NewCore(newLogEncoder(cfg.Format, encoderConfig), zapcore.Lock(os.Stdout), level))
	}
	return zapcore.NewTee(cores...), nil
}

func rotatingLogWriter(cfg config.LogConfig) (*lumberjack.Logger, error) {
	logDir := cfg.Path
	if logDir == "" {
		logDir = "./logs"
	}
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return nil, fmt.Errorf("create log directory: %w", err)
	}
	logFile := cfg.File
	if logFile == "" {
		logFile = time.Now().Format("2006-01-02") + ".log"
	}
	rotate := (config.Config{Log: cfg}).EffectiveLogRotate()
	return &lumberjack.Logger{
		Filename:   filepath.Join(logDir, logFile),
		MaxSize:    rotate.MaxSizeMB,
		MaxBackups: rotate.MaxBackups,
		MaxAge:     rotate.MaxAgeDays,
		Compress:   rotate.Compress,
	}, nil
}

func newLogEncoder(format string, cfg zapcore.EncoderConfig) zapcore.Encoder {
	if strings.ToLower(format) == "json" {
		jsonConfig := cfg
		jsonConfig.EncodeTime = func(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
			enc.AppendString(t.Format(time.RFC3339))
		}
		return zapcore.NewJSONEncoder(jsonConfig)
	}
	return zapcore.NewConsoleEncoder(cfg)
}

func parseLogLevel(level string) zapcore.Level {
	switch strings.ToLower(level) {
	case "debug":
		return zapcore.DebugLevel
	case "warn":
		return zapcore.WarnLevel
	case "error", "fatal":
		return zapcore.ErrorLevel
	default:
		return zapcore.InfoLevel
	}
}

func containsOutput(outputs []string, target string) bool {
	for _, output := range outputs {
		if strings.EqualFold(output, target) {
			return true
		}
	}
	return false
}
