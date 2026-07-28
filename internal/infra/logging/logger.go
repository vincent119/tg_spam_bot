// Package logging 提供應用程式啟動時使用的結構化日誌初始化與檔案輪轉。
package logging

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/vincent119/tg_spam_bot/internal/config"
	"github.com/vincent119/zlogger"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

// SyncFunc 封裝 logger flush 行為，呼叫端應在程序結束前執行。
type SyncFunc func() error

// Initialize 依設定初始化全域 zlogger，並回傳程序結束時應呼叫的 sync 函式。
func Initialize(cfg config.Config) (SyncFunc, error) {
	if !cfg.Log.Rotate.Enabled && cfg.Log.MaxFiles > 0 {
		if err := PruneLegacyLogFiles(cfg.Log.Path, cfg.Log.MaxFiles); err != nil {
			return nil, err
		}
	}
	if cfg.Log.Rotate.Enabled && containsOutput(cfg.Log.Outputs, "file") {
		return initializeRotatingLogger(cfg.Log)
	}
	zlogger.Init(&zlogger.Config{Level: cfg.Log.Level, Format: cfg.Log.Format, Outputs: cfg.Log.Outputs, LogPath: cfg.Log.Path, FileName: cfg.Log.File, AddCaller: true})
	return zlogger.Sync, nil
}

// PruneLegacyLogFiles 保留未啟用 rotate 時的舊版啟動清理行為。
func PruneLegacyLogFiles(dir string, maxFiles int) error {
	if maxFiles <= 0 {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
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

func initializeRotatingLogger(cfg config.LogConfig) (SyncFunc, error) {
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
			writer, err := newDailyRotatingLogWriter(cfg, time.Now)
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

type dailyRotatingLogWriter struct {
	mu             sync.Mutex
	logDir         string
	configuredFile string
	rotate         config.LogRotateConfig
	now            func() time.Time
	currentDate    string
	logger         *lumberjack.Logger
}

func newDailyRotatingLogWriter(cfg config.LogConfig, now func() time.Time) (*dailyRotatingLogWriter, error) {
	if now == nil {
		now = time.Now
	}
	logDir := cfg.Path
	if logDir == "" {
		logDir = "./logs"
	}
	if err := os.MkdirAll(filepath.Join(logDir, filepath.Dir(cfg.File)), 0o755); err != nil {
		return nil, fmt.Errorf("create log directory: %w", err)
	}
	rotate := (config.Config{Log: cfg}).EffectiveLogRotate()
	if err := migrateLegacyActiveLogFile(logDir, cfg.File); err != nil {
		return nil, err
	}
	date := logDate(now())
	writer := &dailyRotatingLogWriter{
		logDir:         logDir,
		configuredFile: cfg.File,
		rotate:         rotate,
		now:            now,
		currentDate:    date,
		logger:         lumberjackLogWriter(logFilePath(logDir, cfg.File, date), rotate),
	}
	if err := pruneDailyLogFiles(logDir, cfg.File, rotate, now()); err != nil {
		return nil, err
	}
	return writer, nil
}

func lumberjackLogWriter(filename string, rotate config.LogRotateConfig) *lumberjack.Logger {
	return &lumberjack.Logger{
		Filename:   filename,
		MaxSize:    rotate.MaxSizeMB,
		MaxBackups: rotate.MaxBackups,
		MaxAge:     rotate.MaxAgeDays,
		LocalTime:  true,
		Compress:   rotate.Compress,
	}
}

func (w *dailyRotatingLogWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.rotateIfDateChanged(); err != nil {
		return 0, err
	}
	return w.logger.Write(p)
}

func (w *dailyRotatingLogWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.logger.Close()
}

func (w *dailyRotatingLogWriter) rotateIfDateChanged() error {
	date := logDate(w.now())
	if date == w.currentDate {
		return nil
	}
	if err := w.switchDailyLogFile(date); err != nil {
		return err
	}
	return pruneDailyLogFiles(w.logDir, w.configuredFile, w.rotate, w.now())
}

func (w *dailyRotatingLogWriter) switchDailyLogFile(date string) error {
	if err := w.logger.Close(); err != nil {
		return fmt.Errorf("close dated log file: %w", err)
	}
	w.logger = lumberjackLogWriter(logFilePath(w.logDir, w.configuredFile, date), w.rotate)
	w.currentDate = date
	return nil
}

func logDate(t time.Time) string {
	return t.Format("2006-01-02")
}

func logFileName(configuredFile, date string) string {
	dir, base := filepath.Split(configuredFile)
	if base == "" {
		return filepath.Join(dir, date+".log")
	}
	return filepath.Join(dir, date+"."+base)
}

func logFilePath(logDir, configuredFile, date string) string {
	return filepath.Join(logDir, logFileName(configuredFile, date))
}

func migrateLegacyActiveLogFile(logDir, configuredFile string) error {
	if configuredFile == "" {
		return nil
	}
	legacyPath := filepath.Join(logDir, configuredFile)
	info, err := os.Stat(legacyPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("stat legacy active log file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil
	}
	targetPath := logFilePath(logDir, configuredFile, logDate(info.ModTime()))
	if samePath(legacyPath, targetPath) {
		return nil
	}
	if _, err := os.Stat(targetPath); err == nil {
		targetPath = collidingDailyLogFilePath(logDir, configuredFile, info.ModTime())
	} else if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("stat dated legacy log file: %w", err)
	}
	if err := os.Rename(legacyPath, targetPath); err != nil {
		return fmt.Errorf("migrate legacy active log file: %w", err)
	}
	return nil
}

func collidingDailyLogFilePath(logDir, configuredFile string, t time.Time) string {
	dir, base := filepath.Split(logFileName(configuredFile, logDate(t)))
	ext := filepath.Ext(base)
	prefix := strings.TrimSuffix(base, ext)
	return filepath.Join(logDir, dir, prefix+"-"+t.Format("15-04-05.000")+ext)
}

func samePath(a, b string) bool {
	absA, errA := filepath.Abs(a)
	absB, errB := filepath.Abs(b)
	if errA == nil && errB == nil {
		return absA == absB
	}
	return filepath.Clean(a) == filepath.Clean(b)
}

type dailyLogFile struct {
	path    string
	date    time.Time
	modTime time.Time
	name    string
}

func pruneDailyLogFiles(logDir, configuredFile string, rotate config.LogRotateConfig, now time.Time) error {
	if rotate.MaxAgeDays == 0 && rotate.MaxBackups == 0 {
		return nil
	}
	files, err := listDailyLogFiles(logDir, configuredFile)
	if err != nil {
		return err
	}
	remove := make(map[string]dailyLogFile)
	if rotate.MaxAgeDays > 0 {
		cutoff := startOfDay(now).AddDate(0, 0, -rotate.MaxAgeDays)
		for _, file := range files {
			if file.date.Before(cutoff) {
				remove[file.path] = file
			}
		}
	}
	if rotate.MaxBackups > 0 && len(files) > rotate.MaxBackups {
		slices.SortFunc(files, func(a, b dailyLogFile) int {
			if cmp := b.date.Compare(a.date); cmp != 0 {
				return cmp
			}
			return b.modTime.Compare(a.modTime)
		})
		for _, file := range files[rotate.MaxBackups:] {
			remove[file.path] = file
		}
	}
	for _, file := range remove {
		if err := os.Remove(file.path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove old daily log file %s: %w", file.name, err)
		}
	}
	return nil
}

func listDailyLogFiles(logDir, configuredFile string) ([]dailyLogFile, error) {
	targetDir := filepath.Join(logDir, filepath.Dir(configuredFile))
	entries, err := os.ReadDir(targetDir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read daily log directory: %w", err)
	}
	files := make([]dailyLogFile, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		date, ok := matchDailyLogFile(configuredFile, entry.Name())
		if !ok {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return nil, fmt.Errorf("stat daily log file %s: %w", entry.Name(), err)
		}
		files = append(files, dailyLogFile{
			path:    filepath.Join(targetDir, entry.Name()),
			date:    date,
			modTime: info.ModTime(),
			name:    entry.Name(),
		})
	}
	return files, nil
}

func matchDailyLogFile(configuredFile, name string) (time.Time, bool) {
	if len(name) < len("2006-01-02") {
		return time.Time{}, false
	}
	date, err := time.ParseInLocation("2006-01-02", name[:len("2006-01-02")], time.Local)
	if err != nil {
		return time.Time{}, false
	}
	if configuredFile == "" {
		if name == name[:len("2006-01-02")]+".log" ||
			strings.HasPrefix(name, name[:len("2006-01-02")]+"-") && strings.HasSuffix(strings.TrimSuffix(name, ".gz"), ".log") {
			return date, true
		}
		return time.Time{}, false
	}
	base := filepath.Base(configuredFile)
	ext := filepath.Ext(base)
	prefix := name[:len("2006-01-02")]
	prefix += "." + strings.TrimSuffix(base, ext)
	if name == prefix+ext || strings.HasPrefix(name, prefix+"-") && strings.HasSuffix(strings.TrimSuffix(name, ".gz"), ext) {
		return date, true
	}
	return time.Time{}, false
}

func startOfDay(t time.Time) time.Time {
	year, month, day := t.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, t.Location())
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
