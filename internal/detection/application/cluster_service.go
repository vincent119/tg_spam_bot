package application

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// ClusterSample 是已脫敏的聚類輸入，不包含完整訊息原文。
type ClusterSample struct {
	ID         string
	Category   string
	ReasonCode string
	Signals    []string
	Evidence   []string
	OccurredAt time.Time
}

// ClusterReport 是人工審查用的聚類摘要。
type ClusterReport struct {
	Clusters []Cluster
}

// Cluster 是一組語意或原因相近的可疑訊息摘要。
type Cluster struct {
	ID                   string
	SampleCount          int
	SuggestedCategory    string
	ReasonCodes          []string
	Signals              []string
	CandidateYAMLSummary []string
}

// ClusterService 產生不含完整原文且不修改 YAML 的聚類報表。
type ClusterService struct{}

// Report 依 category 與 reason code 分群，輸出人工審查摘要。
func (ClusterService) Report(samples []ClusterSample) ClusterReport {
	groups := make(map[string][]ClusterSample)
	for _, sample := range samples {
		category := strings.TrimSpace(sample.Category)
		if category == "" {
			category = "uncategorized_spam"
		}
		reason := strings.TrimSpace(sample.ReasonCode)
		if reason == "" {
			reason = "unknown"
		}
		key := category + "\x00" + reason
		groups[key] = append(groups[key], sample)
	}
	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	report := ClusterReport{Clusters: make([]Cluster, 0, len(keys))}
	for i, key := range keys {
		items := groups[key]
		category, reason, _ := strings.Cut(key, "\x00")
		cluster := Cluster{
			ID:                fmt.Sprintf("cluster-%03d", i+1),
			SampleCount:       len(items),
			SuggestedCategory: category,
			ReasonCodes:       []string{reason},
			Signals:           uniqueClusterValues(flattenSignals(items)),
		}
		cluster.CandidateYAMLSummary = candidateSummary(category, reason, cluster.Signals, items)
		report.Clusters = append(report.Clusters, cluster)
	}
	return report
}

func flattenSignals(samples []ClusterSample) []string {
	var signals []string
	for _, sample := range samples {
		signals = append(signals, sample.Signals...)
	}
	return signals
}

func candidateSummary(category, reason string, signals []string, samples []ClusterSample) []string {
	values := []string{category, reason}
	values = append(values, signals...)
	for _, sample := range samples {
		values = append(values, sample.Evidence...)
	}
	return uniqueClusterValues(values)
}

func uniqueClusterValues(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
