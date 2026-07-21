package application

import (
	"strings"

	"github.com/vincent119/tg_spam_bot/internal/detection/domain"
)

const (
	SignalLowRuleScore        = "low_rule_score"
	SignalSemanticSimilarSpam = "semantic_similar_spam"
	SignalSemanticSimilarHam  = "semantic_similar_ham"
)

// AIEligibility 彙整 delivery 與 application 已完成的安全門檻。
type AIEligibility struct {
	ChatAuthorized   bool
	MessageSupported bool
	Exempt           bool
	FromBot          bool
}

// EligibleForAI 回傳一般訊息是否允許進入 AI 判定候選流程。
func (e AIEligibility) EligibleForAI() bool {
	return e.ChatAuthorized && e.MessageSupported && !e.Exempt && !e.FromBot
}

// AITriggerPolicy 控制規則判定後是否需要進一步呼叫 AI classifier。
type AITriggerPolicy struct {
	OnlyWhenAmbiguous bool
}

// AITriggerDecision 說明本次訊息是否應呼叫 AI，以及使用哪些最小化訊號摘要。
type AITriggerDecision struct {
	ShouldClassify bool
	Reason         string
	Signals        []string
}

// Evaluate 判定 AI classifier 呼叫條件；明確垃圾與明確正常都不呼叫 AI。
func (p AITriggerPolicy) Evaluate(result domain.Result, eligibility AIEligibility, extraSignals ...string) AITriggerDecision {
	signals := uniqueAISignals(append(result.SignalsCopy(), extraSignals...))
	if !eligibility.EligibleForAI() {
		return AITriggerDecision{Reason: "not_eligible", Signals: signals}
	}
	if result.Spam {
		return AITriggerDecision{Reason: "clear_rule_spam", Signals: signals}
	}
	if result.Score > 0 {
		signals = uniqueAISignals(append(signals, SignalLowRuleScore))
		return AITriggerDecision{ShouldClassify: true, Reason: "ambiguous_rule_score", Signals: signals}
	}
	if hasWeakSuspiciousSignal(signals) {
		return AITriggerDecision{ShouldClassify: true, Reason: "weak_suspicious_signal", Signals: signals}
	}
	return AITriggerDecision{Reason: "clear_normal", Signals: signals}
}

// SemanticObservation 是語意相似查詢的輔助判定摘要；不得直接轉成處置。
type SemanticObservation struct {
	Signals    []string
	Matches    []domain.SemanticMatch
	CanEnforce bool
}

// ObserveSemanticMatches 將高相似 spam/ham 案例轉成輔助訊號。
func ObserveSemanticMatches(matches []domain.SemanticMatch, spamThreshold, hamThreshold float64) SemanticObservation {
	observation := SemanticObservation{Matches: domain.SemanticMatchesCopy(matches)}
	for _, match := range matches {
		switch match.Label {
		case domain.AILabelSpam:
			if match.Similarity >= spamThreshold {
				observation.Signals = append(observation.Signals, SignalSemanticSimilarSpam)
			}
		case domain.AILabelHam:
			if match.Similarity >= hamThreshold {
				observation.Signals = append(observation.Signals, SignalSemanticSimilarHam)
			}
		}
	}
	observation.Signals = uniqueAISignals(observation.Signals)
	// 語意相似案例只提供 observe 輔助，避免歷史樣本誤差被放大成封鎖。
	observation.CanEnforce = false
	return observation
}

func hasWeakSuspiciousSignal(signals []string) bool {
	for _, signal := range signals {
		switch strings.TrimSpace(signal) {
		case "external_url", "deny_domain", "telegram_mention", "telegram_invite",
			"transaction_signal", "profit_claim", "download_register_signal",
			"high_frequency", "repeated_content", "coordinated_content", "new_member_link",
			SignalSemanticSimilarSpam, "semantic_blacklist_match":
			return true
		}
	}
	return false
}

func uniqueAISignals(signals []string) []string {
	seen := make(map[string]struct{}, len(signals))
	out := make([]string, 0, len(signals))
	for _, signal := range signals {
		signal = strings.TrimSpace(signal)
		if signal == "" {
			continue
		}
		if _, exists := seen[signal]; exists {
			continue
		}
		seen[signal] = struct{}{}
		out = append(out, signal)
	}
	return out
}
