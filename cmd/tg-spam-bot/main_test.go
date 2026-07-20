package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

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
