package application

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/vincent119/tg_spam_bot/internal/detection/domain"
	"github.com/vincent119/zlogger"
)

const (
	SignalAISpam      = "ai_spam"
	SignalAIUncertain = "ai_uncertain"
	SignalAIHam       = "ai_ham"
)

// AIDetectionProcessorPolicy 定義 AI 判定對既有偵測結果的影響範圍。
type AIDetectionProcessorPolicy struct {
	Enabled       bool
	Mode          Mode
	Provider      string
	Model         string
	PromptVersion string
	SchemaVersion string
	MaxTextRunes  int
	MinConfidence float64
	CacheTTL      time.Duration
}

// AIDetectionProcessor 協調語意查詢、AI 判定、稽核與模式政策。
type AIDetectionProcessor struct {
	policy     AIDetectionProcessorPolicy
	trigger    AITriggerPolicy
	semantic   *SemanticLookupPolicy
	store      AIDetectionStore
	classifier AIClassifier
	now        func() time.Time
}

// NewAIDetectionProcessor 建立 AI 判定流程。
func NewAIDetectionProcessor(policy AIDetectionProcessorPolicy, trigger AITriggerPolicy, store AIDetectionStore, classifier AIClassifier, semantic *SemanticLookupPolicy) (*AIDetectionProcessor, error) {
	if !policy.Enabled {
		return &AIDetectionProcessor{policy: policy, trigger: trigger, semantic: semantic, store: store, classifier: classifier, now: time.Now}, nil
	}
	if store == nil || classifier == nil {
		return nil, errors.New("ai detection store and classifier are required")
	}
	if strings.TrimSpace(policy.Provider) == "" || strings.TrimSpace(policy.Model) == "" || strings.TrimSpace(policy.PromptVersion) == "" {
		return nil, errors.New("ai detection provider, model and prompt version are required")
	}
	if policy.MinConfidence <= 0 || policy.MinConfidence > 1 {
		return nil, errors.New("ai detection min confidence must be within 0 and 1")
	}
	if policy.MaxTextRunes <= 0 {
		return nil, errors.New("ai detection max text runes must be positive")
	}
	return &AIDetectionProcessor{policy: policy, trigger: trigger, semantic: semantic, store: store, classifier: classifier, now: time.Now}, nil
}

// Evaluate 在規則判定後插入語意查詢與 AI 判定，並回傳可能被 AI 模式提升的結果。
func (p *AIDetectionProcessor) Evaluate(ctx context.Context, message domain.Message, fingerprint string, ruleResult domain.Result, appMode Mode) (domain.Result, Mode) {
	if p == nil || !p.policy.Enabled {
		return ruleResult, appMode
	}
	result := ruleResult
	if p.semantic != nil && !result.Spam {
		observation, err := p.semantic.Observe(ctx, message.Text)
		if err != nil {
			logAIDetectionError(ctx, "semantic_memory", message, "semantic_lookup_failed", err)
		} else {
			result.Signals = uniqueAISignals(append(result.SignalsCopy(), observation.Signals...))
			logSemanticObservation(ctx, message, observation, result.Signals)
		}
	}
	decision := p.trigger.Evaluate(result, AIEligibility{ChatAuthorized: true, MessageSupported: true})
	if !decision.ShouldClassify {
		return result, appMode
	}
	aiResult, ok := p.classify(ctx, message, fingerprint, result, decision.Signals)
	if !ok {
		return appendAISignal(result, SignalAIUncertain), appMode
	}
	return p.applyResult(result, aiResult, appMode), p.effectiveMode(result, aiResult, appMode)
}

func (p *AIDetectionProcessor) classify(ctx context.Context, message domain.Message, fingerprint string, result domain.Result, signals []string) (domain.AIClassifyResult, bool) {
	event := applicationEvent(message, fingerprint, p.policy, result.RuleVersion, p.now().UTC())
	claim, err := p.store.ClaimAIDetection(ctx, event)
	if err != nil {
		logAIDetectionError(ctx, "ai_detection", message, "claim_failed", err)
		return domain.AIClassifyResult{}, false
	}
	if !claim.Acquired {
		if claim.Existing != nil && claim.Existing.Status == "completed" {
			return claim.Existing.Result, true
		}
		return domain.AIClassifyResult{}, false
	}
	if cached, found, err := p.store.FindCachedAIDetection(ctx, applicationCacheKey(fingerprint, p.policy, result.RuleVersion, p.now().UTC())); err != nil {
		logAIDetectionError(ctx, "ai_detection", message, "cache_lookup_failed", err)
	} else if found {
		if err := p.store.CompleteAIDetection(ctx, event, cached.Result); err != nil {
			logAIDetectionError(ctx, "ai_detection", message, "cache_complete_failed", err)
			return domain.AIClassifyResult{}, false
		}
		return cached.Result, true
	}
	input := domain.NewAIClassifyInput(message.Text, result.Score, result.Threshold, signals, p.policy.MaxTextRunes)
	aiResult, err := p.classifier.Classify(ctx, input)
	if err != nil {
		fail := AIDetectionResult{Status: "failed", ErrorCode: errorCode(err), ErrorText: err.Error(), Retryable: isRetryable(err)}
		_ = p.store.FailAIDetection(context.WithoutCancel(ctx), event, fail)
		logAIDetectionError(ctx, "ai_detection", message, fail.ErrorCode, err)
		return domain.AIClassifyResult{}, false
	}
	if err := p.store.CompleteAIDetection(ctx, event, aiResult); err != nil {
		logAIDetectionError(ctx, "ai_detection", message, "complete_failed", err)
		return domain.AIClassifyResult{}, false
	}
	logAIDetectionResult(ctx, message, p.policy, aiResult, "completed")
	return aiResult, true
}

func (p *AIDetectionProcessor) applyResult(result domain.Result, aiResult domain.AIClassifyResult, appMode Mode) domain.Result {
	if result.Spam {
		return result
	}
	if aiResult.Label == domain.AILabelHam {
		return appendAISignal(result, SignalAIHam)
	}
	if !p.highConfidenceSpam(aiResult) {
		return appendAISignal(result, SignalAIUncertain)
	}
	switch p.policy.Mode {
	case ModeObserve:
		return appendAISignal(result, SignalAISpam)
	case ModeDeleteOnly:
		return aiSpamResult(result, aiResult, domain.ActionDelete)
	case ModeEnforce:
		if appMode == ModeObserve {
			return appendAISignal(result, SignalAISpam)
		}
		return aiSpamResult(result, aiResult, domain.ActionProgressive)
	default:
		return appendAISignal(result, SignalAIUncertain)
	}
}

func (p *AIDetectionProcessor) effectiveMode(result domain.Result, aiResult domain.AIClassifyResult, appMode Mode) Mode {
	if result.Spam || !p.highConfidenceSpam(aiResult) {
		return appMode
	}
	if p.policy.Mode == ModeDeleteOnly {
		return ModeDeleteOnly
	}
	return appMode
}

func (p *AIDetectionProcessor) highConfidenceSpam(result domain.AIClassifyResult) bool {
	return result.Label == domain.AILabelSpam &&
		result.Confidence >= p.policy.MinConfidence &&
		result.ConfidenceSource != domain.AIConfidenceUnavailable
}

func aiSpamResult(base domain.Result, aiResult domain.AIClassifyResult, action domain.Action) domain.Result {
	base.Spam = true
	base.CategoryID = aiResult.Category
	base.Severity = domain.SeverityNormal
	base.Action = action
	base.Score = 1
	base.Threshold = 1
	base.Signals = uniqueAISignals(append(base.SignalsCopy(), SignalAISpam))
	return base
}

func appendAISignal(result domain.Result, signal string) domain.Result {
	result.Signals = uniqueAISignals(append(result.SignalsCopy(), signal))
	return result
}

func applicationEvent(message domain.Message, fingerprint string, policy AIDetectionProcessorPolicy, ruleVersion string, createdAt time.Time) AIDetectionEvent {
	return AIDetectionEvent{
		ChatID: message.ChatID, UpdateID: message.UpdateID, MessageID: message.MessageID, UserID: message.UserID,
		ContentFingerprint: fingerprint, Provider: policy.Provider, Model: policy.Model,
		PromptVersion: policy.PromptVersion, SchemaVersion: policy.SchemaVersion,
		RuleVersion: ruleVersion, CreatedAt: createdAt,
	}
}

func applicationCacheKey(fingerprint string, policy AIDetectionProcessorPolicy, ruleVersion string, now time.Time) AIDetectionCacheKey {
	return AIDetectionCacheKey{
		ContentFingerprint: fingerprint, Provider: policy.Provider, Model: policy.Model,
		PromptVersion: policy.PromptVersion, RuleVersion: ruleVersion, CacheTTL: policy.CacheTTL, Now: now,
	}
}

func logAIDetectionError(ctx context.Context, subsystem string, message domain.Message, code string, err error) {
	zlogger.DebugContext(ctx, "AI 輔助偵測安全降級",
		zlogger.String("subsystem", subsystem),
		zlogger.Int64("update_id", message.UpdateID),
		zlogger.Int64("chat_id", message.ChatID),
		zlogger.Int64("message_id", message.MessageID),
		zlogger.String("error_code", code),
		zlogger.Err(err),
	)
}

func logAIDetectionResult(ctx context.Context, message domain.Message, policy AIDetectionProcessorPolicy, result domain.AIClassifyResult, status string) {
	zlogger.DebugContext(ctx, "完成 AI 輔助判定",
		zlogger.String("subsystem", "ai_detection"),
		zlogger.Int64("update_id", message.UpdateID),
		zlogger.Int64("chat_id", message.ChatID),
		zlogger.Int64("message_id", message.MessageID),
		zlogger.String("provider", policy.Provider),
		zlogger.String("model", policy.Model),
		zlogger.String("label", string(result.Label)),
		zlogger.String("confidence", strconv.FormatFloat(result.Confidence, 'f', 4, 64)),
		zlogger.String("confidence_source", string(result.ConfidenceSource)),
		zlogger.String("reason_code", result.ReasonCode),
		zlogger.String("safe_action", string(result.SafeAction)),
		zlogger.String("status", status),
	)
}

func logSemanticObservation(ctx context.Context, message domain.Message, observation SemanticObservation, signals []string) {
	best := 0.0
	for _, match := range observation.Matches {
		if match.Similarity > best {
			best = match.Similarity
		}
	}
	zlogger.DebugContext(ctx, "完成語意相似查詢",
		zlogger.String("subsystem", "semantic_memory"),
		zlogger.Int64("update_id", message.UpdateID),
		zlogger.Int64("chat_id", message.ChatID),
		zlogger.Int64("message_id", message.MessageID),
		zlogger.Int("match_count", len(observation.Matches)),
		zlogger.String("similarity", strconv.FormatFloat(best, 'f', 4, 64)),
		zlogger.Strings("signals", signals),
		zlogger.String("status", "completed"),
	)
}
