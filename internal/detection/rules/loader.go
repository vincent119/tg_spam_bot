// Package rules 載入並驗證具版本的 YAML 垃圾訊息規則快照。
package rules

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/vincent119/tg_spam_bot/internal/detection/domain"
	"gopkg.in/yaml.v3"
)

// LoadDir 載入目錄內完整 YAML 集合，任一檔案無效時拒絕部分快照。
func LoadDir(dir string) (domain.RuleSet, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return domain.RuleSet{}, fmt.Errorf("read rules directory: %w", err)
	}
	var paths []string
	for _, entry := range entries {
		if !entry.IsDir() && (strings.HasSuffix(entry.Name(), ".yaml") || strings.HasSuffix(entry.Name(), ".yml")) {
			paths = append(paths, filepath.Join(dir, entry.Name()))
		}
	}
	slices.Sort(paths)
	if len(paths) == 0 {
		return domain.RuleSet{}, fs.ErrNotExist
	}

	// 規則快照版本由全部來源檔案內容決定，個別規則檔可獨立維護其版本。
	snapshot := sha256.New()
	var merged domain.RuleSet
	for _, path := range paths {
		data, err := os.ReadFile(path) //nolint:gosec // path 由已驗證的規則目錄與 ReadDir entry 組成。
		if err != nil {
			return domain.RuleSet{}, fmt.Errorf("read rule file %s: %w", path, err)
		}
		var part domain.RuleSet
		if err := yaml.Unmarshal(data, &part); err != nil {
			return domain.RuleSet{}, fmt.Errorf("decode rule file %s: %w", path, err)
		}
		if err := part.Validate(); err != nil {
			return domain.RuleSet{}, fmt.Errorf("validate rule file %s: %w", path, err)
		}
		_, _ = snapshot.Write([]byte(filepath.Base(path)))
		_, _ = snapshot.Write([]byte{0})
		_, _ = snapshot.Write(data)
		_, _ = snapshot.Write([]byte{0})
		merged.Categories = append(merged.Categories, part.Categories...)
	}
	merged.Version = hex.EncodeToString(snapshot.Sum(nil))
	if err := merged.Validate(); err != nil {
		return domain.RuleSet{}, fmt.Errorf("validate rules: %w", err)
	}
	return merged, nil
}
