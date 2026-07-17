package rules

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/vincent119/tg_spam_bot/internal/detection/domain"
	"gopkg.in/yaml.v3"
)

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

	var merged domain.RuleSet
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return domain.RuleSet{}, fmt.Errorf("read rule file %s: %w", path, err)
		}
		var part domain.RuleSet
		if err := yaml.Unmarshal(data, &part); err != nil {
			return domain.RuleSet{}, fmt.Errorf("decode rule file %s: %w", path, err)
		}
		if merged.Version == "" {
			merged.Version = part.Version
		} else if part.Version != merged.Version {
			return domain.RuleSet{}, fmt.Errorf("rule version mismatch in %s", path)
		}
		merged.Categories = append(merged.Categories, part.Categories...)
	}
	if err := merged.Validate(); err != nil {
		return domain.RuleSet{}, fmt.Errorf("validate rules: %w", err)
	}
	return merged, nil
}
