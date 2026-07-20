// Package application 實作自動回覆比對與執行流程。
package application

import (
	"strings"

	autoreplydomain "github.com/vincent119/tg_spam_bot/internal/autoreply/domain"
	detectiondomain "github.com/vincent119/tg_spam_bot/internal/detection/domain"
)

// Matcher 以固定順序比對自動回覆規則。
type Matcher struct {
	normalizer detectiondomain.Normalizer
	rules      []preparedRule
}

type preparedRule struct {
	rule        autoreplydomain.Rule
	keywords    []string
	traditional []string
}

// NewMatcher 建立使用既有文字正規化策略的自動回覆比對器。
func NewMatcher(ruleSet autoreplydomain.RuleSet, normalizer detectiondomain.Normalizer) Matcher {
	enabled := ruleSet.EnabledRules()
	rules := make([]preparedRule, 0, len(enabled))
	for _, rule := range enabled {
		prepared := preparedRule{rule: rule}
		for _, keyword := range rule.Keywords {
			text := normalizer.Normalize(keyword)
			prepared.keywords = append(prepared.keywords, text.Normalized)
			if text.TraditionalVariant != text.Normalized {
				prepared.traditional = append(prepared.traditional, text.TraditionalVariant)
			}
		}
		rules = append(rules, prepared)
	}
	return Matcher{normalizer: normalizer, rules: rules}
}

// Match 回傳第一條命中的啟用規則。
func (m Matcher) Match(message detectiondomain.Message) (autoreplydomain.Match, bool) {
	text := m.normalizer.Normalize(message.Text)
	for _, rule := range m.rules {
		if containsAny(text.Normalized, rule.keywords) || containsAny(text.TraditionalVariant, rule.keywords) || containsAny(text.TraditionalVariant, rule.traditional) || containsAny(text.Normalized, rule.traditional) {
			return autoreplydomain.Match{RuleID: rule.rule.ID, Reply: rule.rule.Reply}, true
		}
	}
	return autoreplydomain.Match{}, false
}

func containsAny(text string, keywords []string) bool {
	for _, keyword := range keywords {
		if keyword != "" && strings.Contains(text, keyword) {
			return true
		}
	}
	return false
}
