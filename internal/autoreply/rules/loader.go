// Package rules 載入自動回覆獨立 YAML 規則檔。
package rules

import (
	"fmt"
	"os"
	"strings"

	"github.com/vincent119/tg_spam_bot/internal/autoreply/domain"
	"gopkg.in/yaml.v3"
)

type fileRuleSet struct {
	Version string     `yaml:"version"`
	Rules   []fileRule `yaml:"rules"`
}

type fileRule struct {
	ID       string   `yaml:"id"`
	Enabled  *bool    `yaml:"enabled"`
	Keywords []string `yaml:"keywords"`
	Reply    string   `yaml:"reply"`
}

// LoadFile 載入並驗證自動回覆規則檔，任一規則無效時拒絕整份快照。
func LoadFile(path string) (domain.RuleSet, error) {
	if strings.TrimSpace(path) == "" {
		return domain.RuleSet{}, fmt.Errorf("auto reply rules file is required")
	}
	data, err := os.ReadFile(path) //nolint:gosec // path 由部署設定指定，啟動時讀取固定規則檔。
	if err != nil {
		return domain.RuleSet{}, fmt.Errorf("read auto reply rules file %s: %w", path, err)
	}
	var raw fileRuleSet
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return domain.RuleSet{}, fmt.Errorf("decode auto reply rules file %s: %w", path, err)
	}
	ruleSet := domain.RuleSet{Version: strings.TrimSpace(raw.Version), Rules: make([]domain.Rule, 0, len(raw.Rules))}
	for _, rawRule := range raw.Rules {
		enabled := true
		if rawRule.Enabled != nil {
			enabled = *rawRule.Enabled
		}
		keywords := make([]string, 0, len(rawRule.Keywords))
		for _, keyword := range rawRule.Keywords {
			keywords = append(keywords, strings.TrimSpace(keyword))
		}
		ruleSet.Rules = append(ruleSet.Rules, domain.Rule{
			ID:       strings.TrimSpace(rawRule.ID),
			Enabled:  enabled,
			Keywords: keywords,
			Reply:    strings.TrimSpace(rawRule.Reply),
		})
	}
	if err := ruleSet.Validate(); err != nil {
		return domain.RuleSet{}, fmt.Errorf("validate auto reply rules: %w", err)
	}
	return ruleSet, nil
}
