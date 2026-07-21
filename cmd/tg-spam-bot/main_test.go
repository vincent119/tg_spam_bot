package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vincent119/tg_spam_bot/internal/config"
	"go.uber.org/zap"
)

func TestInitializeLoggerConsoleOnlyDoesNotRequireFileSettings(t *testing.T) {
	cfg := config.Config{}
	cfg.Log.Level = "info"
	cfg.Log.Format = "json"
	cfg.Log.Outputs = []string{"console"}
	cfg.Log.Rotate.Enabled = false
	cfg.Log.Path = filepath.Join(t.TempDir(), "missing")

	syncLogger, err := initializeLogger(cfg)
	if err != nil {
		t.Fatalf("initializeLogger() error = %v", err)
	}
	if syncLogger == nil {
		t.Fatal("initializeLogger() 應回傳 sync 函式")
	}
}

func TestBuildRotatingCoreWritesFile(t *testing.T) {
	dir := t.TempDir()
	cfg := config.LogConfig{
		Level:   "debug",
		Format:  "json",
		Outputs: []string{"file"},
		Path:    dir,
		File:    "app.log",
		Rotate: config.LogRotateConfig{
			Enabled:    true,
			MaxSizeMB:  1,
			MaxBackups: 1,
			MaxAgeDays: 1,
			Compress:   false,
		},
	}

	core, err := buildRotatingCore(cfg)
	if err != nil {
		t.Fatalf("buildRotatingCore() error = %v", err)
	}
	logger := zap.New(core)
	logger.Info("rotate test", zap.String("component", "logger"))
	_ = logger.Sync()

	content, err := os.ReadFile(filepath.Join(dir, "app.log"))
	if err != nil {
		t.Fatalf("讀取 rotate log 失敗：%v", err)
	}
	if !strings.Contains(string(content), "rotate test") {
		t.Fatalf("log 檔案未寫入預期內容：%s", string(content))
	}
}

func TestInitializeLoggerLegacyPruneWhenRotateDisabled(t *testing.T) {
	dir := t.TempDir()
	writeLogFile(t, dir, "old.log", time.Unix(1, 0))
	writeLogFile(t, dir, "new.log", time.Unix(2, 0))

	cfg := config.Config{}
	cfg.Log.Level = "info"
	cfg.Log.Format = "json"
	cfg.Log.Outputs = []string{"console"}
	cfg.Log.Path = dir
	cfg.Log.MaxFiles = 1
	cfg.Log.Rotate.Enabled = false

	if _, err := initializeLogger(cfg); err != nil {
		t.Fatalf("initializeLogger() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "old.log")); !os.IsNotExist(err) {
		t.Fatalf("rotate disabled 時 old.log 應被 legacy prune 移除，err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "new.log")); err != nil {
		t.Fatalf("new.log 應保留：%v", err)
	}
}

func TestInitializeLoggerSkipsLegacyPruneWhenRotateEnabled(t *testing.T) {
	dir := t.TempDir()
	writeLogFile(t, dir, "old.log", time.Unix(1, 0))
	writeLogFile(t, dir, "new.log", time.Unix(2, 0))

	cfg := config.Config{}
	cfg.Log.Level = "info"
	cfg.Log.Format = "json"
	cfg.Log.Outputs = []string{"file"}
	cfg.Log.Path = dir
	cfg.Log.File = "app.log"
	cfg.Log.MaxFiles = 1
	cfg.Log.Rotate.Enabled = true
	cfg.Log.Rotate.MaxSizeMB = 1
	cfg.Log.Rotate.MaxBackups = 1
	cfg.Log.Rotate.MaxAgeDays = 1

	if _, err := initializeLogger(cfg); err != nil {
		t.Fatalf("initializeLogger() error = %v", err)
	}
	for _, name := range []string{"old.log", "new.log"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("rotate enabled 時 %s 不應被 legacy prune 移除：%v", name, err)
		}
	}
}

func TestRotatingLogWriterEffectiveValues(t *testing.T) {
	dir := t.TempDir()
	cfg := config.LogConfig{
		Path: dir,
		File: "app.log",
		Rotate: config.LogRotateConfig{
			Enabled:    true,
			MaxSizeMB:  0,
			MaxBackups: 0,
			MaxAgeDays: 0,
			Compress:   true,
		},
	}
	writer, err := rotatingLogWriter(cfg)
	if err != nil {
		t.Fatalf("rotatingLogWriter() error = %v", err)
	}
	if writer.MaxSize != 100 || writer.MaxBackups != 0 || writer.MaxAge != 0 || !writer.Compress {
		t.Fatalf("writer 設定未套用有效值：%+v", writer)
	}
}

func TestPruneLogFiles(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	files := []string{"old.log", "middle.log", "new.log", "keep.txt"}
	for i, name := range files {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(name), 0o600); err != nil {
			t.Fatalf("建立測試檔案失敗：%v", err)
		}
		modTime := time.Unix(int64(i+1), 0)
		if err := os.Chtimes(path, modTime, modTime); err != nil {
			t.Fatalf("設定測試檔案時間失敗：%v", err)
		}
	}

	if err := pruneLogFiles(dir, 2); err != nil {
		t.Fatalf("pruneLogFiles() error = %v", err)
	}
	for _, name := range []string{"middle.log", "new.log", "keep.txt"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("檔案 %s 應保留：%v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "old.log")); !os.IsNotExist(err) {
		t.Fatalf("old.log 應被移除，err=%v", err)
	}
}

func TestPruneLogFilesDisabled(t *testing.T) {
	t.Parallel()

	if err := pruneLogFiles(filepath.Join(t.TempDir(), "missing"), 0); err != nil {
		t.Fatalf("max_files=0 不應讀取目錄：%v", err)
	}
}

func writeLogFile(t *testing.T, dir, name string, modTime time.Time) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(name), 0o600); err != nil {
		t.Fatalf("建立測試檔案失敗：%v", err)
	}
	if err := os.Chtimes(path, modTime, modTime); err != nil {
		t.Fatalf("設定測試檔案時間失敗：%v", err)
	}
}
