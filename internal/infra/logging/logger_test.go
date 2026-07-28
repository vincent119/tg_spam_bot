package logging

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vincent119/tg_spam_bot/internal/config"
	"go.uber.org/zap"
)

func TestInitializeConsoleOnlyDoesNotRequireFileSettings(t *testing.T) {
	cfg := config.Config{}
	cfg.Log.Level = "info"
	cfg.Log.Format = "json"
	cfg.Log.Outputs = []string{"console"}
	cfg.Log.Rotate.Enabled = false
	cfg.Log.Path = filepath.Join(t.TempDir(), "missing")

	syncLogger, err := Initialize(cfg)
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	if syncLogger == nil {
		t.Fatal("Initialize() 應回傳 sync 函式")
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

	content, err := os.ReadFile(logFilePath(dir, "app.log", logDate(time.Now())))
	if err != nil {
		t.Fatalf("讀取 rotate log 失敗：%v", err)
	}
	if !strings.Contains(string(content), "rotate test") {
		t.Fatalf("log 檔案未寫入預期內容：%s", string(content))
	}
}

func TestInitializeLegacyPruneWhenRotateDisabled(t *testing.T) {
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

	if _, err := Initialize(cfg); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "old.log")); !os.IsNotExist(err) {
		t.Fatalf("rotate disabled 時 old.log 應被 legacy prune 移除，err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "new.log")); err != nil {
		t.Fatalf("new.log 應保留：%v", err)
	}
}

func TestInitializeSkipsLegacyPruneWhenRotateEnabled(t *testing.T) {
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

	if _, err := Initialize(cfg); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	for _, name := range []string{"old.log", "new.log"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("rotate enabled 時 %s 不應被 legacy prune 移除：%v", name, err)
		}
	}
}

func TestDailyRotatingLogWriterEffectiveValues(t *testing.T) {
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
	writer, err := newDailyRotatingLogWriter(cfg, func() time.Time {
		return time.Date(2026, 7, 27, 10, 0, 0, 0, time.Local)
	})
	if err != nil {
		t.Fatalf("newDailyRotatingLogWriter() error = %v", err)
	}
	if writer.logger.MaxSize != 100 || writer.logger.MaxBackups != 0 || writer.logger.MaxAge != 0 || !writer.logger.Compress {
		t.Fatalf("writer 設定未套用有效值：%+v", writer.logger)
	}
	if writer.logger.Filename != filepath.Join(dir, "2026-07-27.app.log") {
		t.Fatalf("writer 檔名未套用日期前綴：%s", writer.logger.Filename)
	}
}

func TestDailyRotatingLogWriterSwitchesConfiguredFileByDate(t *testing.T) {
	dir := t.TempDir()
	current := time.Date(2026, 7, 27, 23, 59, 0, 0, time.Local)
	writer, err := newDailyRotatingLogWriter(config.LogConfig{
		Path: dir,
		File: "app.log",
		Rotate: config.LogRotateConfig{
			Enabled:    true,
			MaxSizeMB:  1,
			MaxBackups: 14,
			MaxAgeDays: 30,
		},
	}, func() time.Time {
		return current
	})
	if err != nil {
		t.Fatalf("newDailyRotatingLogWriter() error = %v", err)
	}
	if _, err := writer.Write([]byte("before midnight\n")); err != nil {
		t.Fatalf("寫入跨日前日誌失敗：%v", err)
	}
	current = time.Date(2026, 7, 28, 0, 1, 0, 0, time.Local)
	if _, err := writer.Write([]byte("after midnight\n")); err != nil {
		t.Fatalf("寫入跨日後日誌失敗：%v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("關閉 writer 失敗：%v", err)
	}

	assertFileContains(t, filepath.Join(dir, "2026-07-27.app.log"), "before midnight")
	assertFileContains(t, filepath.Join(dir, "2026-07-28.app.log"), "after midnight")
}

func TestDailyRotatingLogWriterMigratesLegacyActiveFile(t *testing.T) {
	dir := t.TempDir()
	legacyPath := filepath.Join(dir, "app.log")
	if err := os.WriteFile(legacyPath, []byte("legacy yesterday\n"), 0o600); err != nil {
		t.Fatalf("建立 legacy active log 失敗：%v", err)
	}
	legacyTime := time.Date(2026, 7, 27, 23, 59, 0, 0, time.Local)
	if err := os.Chtimes(legacyPath, legacyTime, legacyTime); err != nil {
		t.Fatalf("設定 legacy active log 時間失敗：%v", err)
	}

	writer, err := newDailyRotatingLogWriter(config.LogConfig{
		Path: dir,
		File: "app.log",
		Rotate: config.LogRotateConfig{
			Enabled:   true,
			MaxSizeMB: 1,
		},
	}, func() time.Time {
		return time.Date(2026, 7, 28, 10, 0, 0, 0, time.Local)
	})
	if err != nil {
		t.Fatalf("newDailyRotatingLogWriter() error = %v", err)
	}
	if _, err := writer.Write([]byte("today\n")); err != nil {
		t.Fatalf("寫入今日日誌失敗：%v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("關閉 writer 失敗：%v", err)
	}

	if _, err := os.Stat(legacyPath); !os.IsNotExist(err) {
		t.Fatalf("legacy active log 應被搬移，err=%v", err)
	}
	assertFileContains(t, filepath.Join(dir, "2026-07-27.app.log"), "legacy yesterday")
	assertFileContains(t, filepath.Join(dir, "2026-07-28.app.log"), "today")
}

func TestDailyRotatingLogWriterSwitchesDefaultFileByDate(t *testing.T) {
	dir := t.TempDir()
	current := time.Date(2026, 7, 27, 23, 59, 0, 0, time.Local)
	writer, err := newDailyRotatingLogWriter(config.LogConfig{
		Path: dir,
		Rotate: config.LogRotateConfig{
			Enabled:   true,
			MaxSizeMB: 1,
		},
	}, func() time.Time {
		return current
	})
	if err != nil {
		t.Fatalf("newDailyRotatingLogWriter() error = %v", err)
	}
	if _, err := writer.Write([]byte("first day\n")); err != nil {
		t.Fatalf("寫入第一天日誌失敗：%v", err)
	}
	current = time.Date(2026, 7, 28, 0, 1, 0, 0, time.Local)
	if _, err := writer.Write([]byte("second day\n")); err != nil {
		t.Fatalf("寫入第二天日誌失敗：%v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("關閉 writer 失敗：%v", err)
	}

	assertFileContains(t, filepath.Join(dir, "2026-07-27.log"), "first day")
	assertFileContains(t, filepath.Join(dir, "2026-07-28.log"), "second day")
}

func TestPruneDailyLogFilesRemovesFilesByMaxAge(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeLogFile(t, dir, "2026-07-25.app.log", time.Date(2026, 7, 25, 12, 0, 0, 0, time.Local))
	writeLogFile(t, dir, "2026-07-26.app.log", time.Date(2026, 7, 26, 12, 0, 0, 0, time.Local))
	writeLogFile(t, dir, "other.log", time.Date(2026, 7, 25, 12, 0, 0, 0, time.Local))

	err := pruneDailyLogFiles(dir, "app.log", config.LogRotateConfig{MaxAgeDays: 1}, time.Date(2026, 7, 27, 9, 0, 0, 0, time.Local))
	if err != nil {
		t.Fatalf("pruneDailyLogFiles() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "2026-07-25.app.log")); !os.IsNotExist(err) {
		t.Fatalf("超過 max_age_days 的日誌應被移除，err=%v", err)
	}
	for _, name := range []string{"2026-07-26.app.log", "other.log"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("檔案 %s 應保留：%v", name, err)
		}
	}
}

func TestPruneLegacyLogFiles(t *testing.T) {
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

	if err := PruneLegacyLogFiles(dir, 2); err != nil {
		t.Fatalf("PruneLegacyLogFiles() error = %v", err)
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

func TestPruneLegacyLogFilesDisabled(t *testing.T) {
	t.Parallel()

	if err := PruneLegacyLogFiles(filepath.Join(t.TempDir(), "missing"), 0); err != nil {
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

func assertFileContains(t *testing.T, path, want string) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("讀取檔案 %s 失敗：%v", path, err)
	}
	if !strings.Contains(string(content), want) {
		t.Fatalf("檔案 %s 未包含 %q：%s", path, want, string(content))
	}
}
