package domain

import (
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
)

func (r RuleSet) Validate() error {
	var errs []error
	if strings.TrimSpace(r.Version) == "" {
		errs = append(errs, errors.New("version: required"))
	}
	seen := make(map[string]struct{}, len(r.Categories))
	for i, c := range r.Categories {
		prefix := fmt.Sprintf("categories[%d]", i)
		if c.ID == "" {
			errs = append(errs, fmt.Errorf("%s.id: required", prefix))
		} else if _, ok := seen[c.ID]; ok {
			errs = append(errs, fmt.Errorf("%s.id: duplicate %q", prefix, c.ID))
		}
		seen[c.ID] = struct{}{}
		if c.Severity != SeverityNormal && c.Severity != SeverityHigh && c.Severity != SeverityCritical {
			errs = append(errs, fmt.Errorf("%s.severity: invalid", prefix))
		}
		if c.Action != ActionObserve && c.Action != ActionDelete && c.Action != ActionProgressive && c.Action != ActionBan {
			errs = append(errs, fmt.Errorf("%s.action: invalid", prefix))
		}
		if c.Threshold <= 0 || c.Weight < 0 || len(c.Terms)+len(c.Aliases) == 0 {
			errs = append(errs, fmt.Errorf("%s: invalid threshold, weight, or terms", prefix))
		}
		if c.Action == ActionBan && (c.Severity != SeverityCritical || len(c.RequireAny) == 0) {
			errs = append(errs, fmt.Errorf("%s: ban requires critical severity and require_any", prefix))
		}
	}
	return errors.Join(errs...)
}

type Detector struct {
	rules      RuleSet
	normalizer Normalizer
	allow      []string
	deny       []string
}

func NewDetector(rules RuleSet, normalizer Normalizer, allowDomains, denyDomains []string) (*Detector, error) {
	if err := rules.Validate(); err != nil {
		return nil, err
	}
	return &Detector{
		rules:      rules,
		normalizer: normalizer,
		allow:      normalizeDomains(allowDomains),
		deny:       normalizeDomains(denyDomains),
	}, nil
}

func (d *Detector) Detect(message Message, extraSignals ...string) Result {
	text := d.normalizer.Normalize(message.Text)
	signals := detectSignals(text.Normalized, message.Entities, d.allow, d.deny)
	signals = unique(append(signals, extraSignals...))
	result := Result{RuleVersion: d.rules.Version, Signals: append([]string(nil), signals...)}

	for _, category := range d.rules.Categories {
		if !category.Enabled {
			continue
		}
		matches := matchCategory(category, text)
		if len(matches) == 0 {
			continue
		}
		score := len(matches) * category.Weight
		for _, signal := range signals {
			if slices.Contains(category.RequireAny, signal) {
				score += 20
			}
		}
		hasRequired := len(category.RequireAny) == 0 || intersects(category.RequireAny, signals)
		spam := score >= category.Threshold
		if category.Action == ActionBan && !hasRequired {
			spam = false
		}
		candidate := Result{
			Spam: spam, CategoryID: category.ID, Severity: category.Severity,
			Score: score, Threshold: category.Threshold, RuleVersion: d.rules.Version,
			Matches: matches, Signals: append([]string(nil), signals...),
		}
		if spam {
			candidate.Action = category.Action
		}
		if (candidate.Spam && !result.Spam) || (candidate.Spam == result.Spam && candidate.Score > result.Score) {
			result = candidate
		}
	}
	return result
}

func matchCategory(category Category, text NormalizedText) []Match {
	terms := append(append([]string(nil), category.Terms...), category.Aliases...)
	seen := make(map[string]struct{}, len(terms))
	var matches []Match
	for _, term := range terms {
		normalizedTerm := normalizeComparable(term)
		if normalizedTerm == "" {
			continue
		}
		source := SourceNormalized
		matched := strings.Contains(text.Normalized, normalizedTerm)
		if !matched && strings.Contains(text.TraditionalVariant, normalizedTerm) {
			matched = true
			source = SourceTraditional
		}
		if matched {
			key := category.ID + "\x00" + normalizedTerm
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			matches = append(matches, Match{RuleID: category.ID, Term: term, Source: source, Weight: category.Weight})
		}
	}
	return matches
}

func detectSignals(text string, entities []Entity, allow, deny []string) []string {
	var signals []string
	if strings.Contains(text, "t.me/") || strings.Contains(text, "telegram.me/") || strings.Contains(text, "telegram.dog/") {
		signals = append(signals, "telegram_invite")
	}
	if strings.Contains(text, "@") {
		signals = append(signals, "telegram_mention")
	}
	if strings.Contains(text, "出售") || strings.Contains(text, "出货") || strings.Contains(text, "供應") || strings.Contains(text, "供应") {
		signals = append(signals, "transaction_signal")
	}
	if strings.Contains(text, "日結") || strings.Contains(text, "日结") || strings.Contains(text, "profit") || strings.Contains(text, "盈利") {
		signals = append(signals, "profit_claim")
	}
	for _, e := range entities {
		if e.Type == "mention" || e.Type == "text_mention" {
			signals = append(signals, "telegram_mention")
		}
		if e.URL == "" {
			continue
		}
		u, err := url.Parse(e.URL)
		if err != nil {
			continue
		}
		host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
		if domainMatch(host, deny) {
			signals = append(signals, "deny_domain")
		} else if !domainMatch(host, allow) {
			signals = append(signals, "external_url")
		}
	}
	return unique(signals)
}

func normalizeDomains(in []string) []string {
	out := make([]string, 0, len(in))
	for _, domain := range in {
		domain = strings.ToLower(strings.Trim(strings.TrimSpace(domain), "."))
		if domain != "" {
			out = append(out, domain)
		}
	}
	return out
}

func domainMatch(host string, domains []string) bool {
	for _, domain := range domains {
		if host == domain || strings.HasSuffix(host, "."+domain) {
			return true
		}
	}
	return false
}

func intersects(a, b []string) bool {
	for _, item := range a {
		if slices.Contains(b, item) {
			return true
		}
	}
	return false
}

func unique(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, item := range in {
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}
