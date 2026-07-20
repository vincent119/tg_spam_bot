// Package domain 定義自動回覆規則與比對結果。
package domain

import (
	"errors"
	"fmt"
	"strings"
)

// RuleSet 是啟動時驗證後固定的自動回覆規則快照。
type RuleSet struct {
	Version string
	Rules   []Rule
}

// Rule 定義單一固定回覆規則。
type Rule struct {
	ID       string
	Enabled  bool
	Keywords []string
	Reply    string
}

// Match 保存單次命中的規則與回覆內容。
type Match struct {
	RuleID string
	Reply  string
}

// Validate 驗證規則快照完整性，避免啟動後才發現部份規則不可用。
func (s RuleSet) Validate() error {
	seen := make(map[string]struct{}, len(s.Rules))
	var errs []error
	for i, rule := range s.Rules {
		index := i + 1
		id := strings.TrimSpace(rule.ID)
		if id == "" {
			errs = append(errs, fmt.Errorf("rule %d: id is required", index))
		} else if _, exists := seen[id]; exists {
			errs = append(errs, fmt.Errorf("rule %d: duplicate id %q", index, id))
		} else {
			seen[id] = struct{}{}
		}
		if len(rule.Keywords) == 0 {
			errs = append(errs, fmt.Errorf("rule %d: keywords are required", index))
		}
		for keywordIndex, keyword := range rule.Keywords {
			if strings.TrimSpace(keyword) == "" {
				errs = append(errs, fmt.Errorf("rule %d keyword %d: must not be empty", index, keywordIndex+1))
			}
		}
		if strings.TrimSpace(rule.Reply) == "" {
			errs = append(errs, fmt.Errorf("rule %d: reply is required", index))
		}
	}
	return errors.Join(errs...)
}

// EnabledRules 回傳啟用規則副本，保持原始順序供命中優先權使用。
func (s RuleSet) EnabledRules() []Rule {
	rules := make([]Rule, 0, len(s.Rules))
	for _, rule := range s.Rules {
		if rule.Enabled {
			rule.Keywords = append([]string(nil), rule.Keywords...)
			rules = append(rules, rule)
		}
	}
	return rules
}
