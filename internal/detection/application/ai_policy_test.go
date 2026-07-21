package application_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/vincent119/tg_spam_bot/internal/detection/application"
	"github.com/vincent119/tg_spam_bot/internal/detection/domain"
)

func TestAITriggerPolicyEvaluate(t *testing.T) {
	t.Parallel()

	eligible := application.AIEligibility{ChatAuthorized: true, MessageSupported: true}
	policy := application.AITriggerPolicy{OnlyWhenAmbiguous: true}
	tests := []struct {
		name        string
		result      domain.Result
		eligibility application.AIEligibility
		extra       []string
		wantCall    bool
		wantReason  string
		wantSignal  string
	}{
		{
			name:        "明確垃圾不呼叫 AI",
			result:      domain.Result{Spam: true, Score: 100, Threshold: 80, Signals: []string{"telegram_mention"}},
			eligibility: eligible,
			wantReason:  "clear_rule_spam",
		},
		{
			name:        "明確正常不呼叫 AI",
			result:      domain.Result{Score: 0, Threshold: 0},
			eligibility: eligible,
			wantReason:  "clear_normal",
		},
		{
			name:        "低規則分數呼叫 AI",
			result:      domain.Result{Score: 40, Threshold: 80},
			eligibility: eligible,
			wantCall:    true,
			wantReason:  "ambiguous_rule_score",
			wantSignal:  application.SignalLowRuleScore,
		},
		{
			name:        "Telegram mention 弱訊號呼叫 AI",
			result:      domain.Result{Signals: []string{"telegram_mention"}},
			eligibility: eligible,
			wantCall:    true,
			wantReason:  "weak_suspicious_signal",
			wantSignal:  "telegram_mention",
		},
		{
			name:        "交易導流弱訊號呼叫 AI",
			result:      domain.Result{Signals: []string{"transaction_signal"}},
			eligibility: eligible,
			wantCall:    true,
			wantReason:  "weak_suspicious_signal",
			wantSignal:  "transaction_signal",
		},
		{
			name:        "語意相似 spam 輔助訊號呼叫 AI",
			result:      domain.Result{},
			eligibility: eligible,
			extra:       []string{application.SignalSemanticSimilarSpam},
			wantCall:    true,
			wantReason:  "weak_suspicious_signal",
			wantSignal:  application.SignalSemanticSimilarSpam,
		},
		{
			name:        "豁免對象不呼叫 AI",
			result:      domain.Result{Score: 40, Threshold: 80, Signals: []string{"telegram_mention"}},
			eligibility: application.AIEligibility{ChatAuthorized: true, MessageSupported: true, Exempt: true},
			wantReason:  "not_eligible",
		},
		{
			name:        "Bot 訊息不呼叫 AI",
			result:      domain.Result{Score: 40, Threshold: 80, Signals: []string{"telegram_mention"}},
			eligibility: application.AIEligibility{ChatAuthorized: true, MessageSupported: true, FromBot: true},
			wantReason:  "not_eligible",
		},
		{
			name:        "未授權聊天不呼叫 AI",
			result:      domain.Result{Score: 40, Threshold: 80, Signals: []string{"telegram_mention"}},
			eligibility: application.AIEligibility{MessageSupported: true},
			wantReason:  "not_eligible",
		},
		{
			name:        "不支援訊息不呼叫 AI",
			result:      domain.Result{Score: 40, Threshold: 80, Signals: []string{"telegram_mention"}},
			eligibility: application.AIEligibility{ChatAuthorized: true},
			wantReason:  "not_eligible",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision := policy.Evaluate(tt.result, tt.eligibility, tt.extra...)
			if decision.ShouldClassify != tt.wantCall {
				t.Fatalf("ShouldClassify = %v，預期 %v，decision=%+v", decision.ShouldClassify, tt.wantCall, decision)
			}
			if decision.Reason != tt.wantReason {
				t.Fatalf("Reason = %q，預期 %q", decision.Reason, tt.wantReason)
			}
			if tt.wantSignal != "" && !slices.Contains(decision.Signals, tt.wantSignal) {
				t.Fatalf("Signals = %v，預期包含 %q", decision.Signals, tt.wantSignal)
			}
		})
	}
}

type embeddingProviderSpy struct {
	input domain.EmbeddingInput
	err   error
}

func (s *embeddingProviderSpy) Embed(_ context.Context, input domain.EmbeddingInput) (domain.EmbeddingResult, error) {
	s.input = input
	if s.err != nil {
		return domain.EmbeddingResult{}, s.err
	}
	return domain.EmbeddingResult{Provider: "test", Model: "embedding", Version: "v1", Dimensions: 2, Vector: []float32{0.1, 0.2}}, nil
}

type semanticMemorySpy struct {
	embedding domain.EmbeddingResult
	neighbors int
	err       error
}

func (s *semanticMemorySpy) SearchSimilar(_ context.Context, embedding domain.EmbeddingResult, maxNeighbors int) ([]domain.SemanticMatch, error) {
	s.embedding = embedding
	s.neighbors = maxNeighbors
	if s.err != nil {
		return nil, s.err
	}
	return []domain.SemanticMatch{{SourceEventID: "event-1", Label: domain.AILabelSpam, Category: "ad", ReasonCode: "similar_pitch", Similarity: 0.93}}, nil
}

func TestSemanticLookupPolicyObserve(t *testing.T) {
	t.Parallel()

	embeddings := &embeddingProviderSpy{}
	memory := &semanticMemorySpy{}
	policy := application.SemanticLookupPolicy{
		Embeddings: embeddings, Memory: memory, MaxTextRunes: 3, MaxNeighbors: 5,
		SpamSimilarityThreshold: 0.90, HamSimilarityThreshold: 0.95,
	}
	observation, err := policy.Observe(context.Background(), "一二三四五")
	if err != nil {
		t.Fatalf("Observe() error = %v", err)
	}
	if embeddings.input.Text != "一二三" {
		t.Fatalf("embedding input = %q，預期截斷", embeddings.input.Text)
	}
	if memory.neighbors != 5 {
		t.Fatalf("max neighbors = %d，預期 5", memory.neighbors)
	}
	if memory.embedding.Model != "embedding" {
		t.Fatalf("embedding = %+v", memory.embedding)
	}
	if !slices.Contains(observation.Signals, application.SignalSemanticSimilarSpam) {
		t.Fatalf("Signals = %v，預期包含相似 spam 訊號", observation.Signals)
	}
	if observation.CanEnforce {
		t.Fatal("相似 spam 不得單獨提升為可處置來源")
	}
}

func TestSemanticLookupPolicyObserveReturnsErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		embeddings application.EmbeddingProvider
		memory     application.SemanticMemory
	}{
		{name: "缺依賴"},
		{name: "embedding 失敗", embeddings: &embeddingProviderSpy{err: errors.New("timeout")}, memory: &semanticMemorySpy{}},
		{name: "語意記憶查詢失敗", embeddings: &embeddingProviderSpy{}, memory: &semanticMemorySpy{err: errors.New("pgvector unavailable")}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy := application.SemanticLookupPolicy{
				Embeddings: tt.embeddings, Memory: tt.memory, MaxTextRunes: 3, MaxNeighbors: 5,
				SpamSimilarityThreshold: 0.90, HamSimilarityThreshold: 0.95,
			}
			if _, err := policy.Observe(context.Background(), "測試"); err == nil {
				t.Fatal("Observe() error = nil，預期錯誤")
			}
		})
	}
}

func TestObserveSemanticMatches(t *testing.T) {
	t.Parallel()

	matches := []domain.SemanticMatch{
		{SourceEventID: "event-1", Label: domain.AILabelSpam, Category: "ad", ReasonCode: "similar_pitch", Similarity: 0.93},
		{SourceEventID: "event-2", Label: domain.AILabelHam, Category: "normal", ReasonCode: "support_reply", Similarity: 0.96},
		{SourceEventID: "event-3", Label: domain.AILabelSpam, Category: "ad", ReasonCode: "weak", Similarity: 0.40},
	}
	observation := application.ObserveSemanticMatches(matches, 0.90, 0.95)
	matches[0].SourceEventID = "mutated"

	if !slices.Contains(observation.Signals, application.SignalSemanticSimilarSpam) {
		t.Fatalf("Signals = %v，預期包含相似 spam 訊號", observation.Signals)
	}
	if !slices.Contains(observation.Signals, application.SignalSemanticSimilarHam) {
		t.Fatalf("Signals = %v，預期包含相似 ham 訊號", observation.Signals)
	}
	if observation.CanEnforce {
		t.Fatal("語意相似案例不得直接提升為可處置來源")
	}
	if observation.Matches[0].SourceEventID != "event-1" {
		t.Fatalf("Matches 未複製邊界：%+v", observation.Matches[0])
	}
}

func TestObserveSemanticBlacklistMatches(t *testing.T) {
	t.Parallel()

	observation := application.ObserveSemanticBlacklistMatches([]application.SemanticBlacklistMatch{
		{CategoryID: "agent_recruiting", CategoryName: "代理招募", Similarity: 0.93},
		{CategoryID: "weak", CategoryName: "低相似", Similarity: 0.20},
	}, 0.90)

	if !slices.Contains(observation.Signals, "semantic_blacklist_match") {
		t.Fatalf("Signals = %v，預期包含黑名單輔助訊號", observation.Signals)
	}
	if observation.CanEnforce {
		t.Fatal("語意黑名單不得單獨提升為可處置來源")
	}
}
