package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/vincent119/tg_spam_bot/internal/detection/application"
	"github.com/vincent119/tg_spam_bot/internal/detection/domain"
	"github.com/vincent119/tg_spam_bot/internal/detection/infra/memory"
)

type aiClassifierStub struct {
	result domain.AIClassifyResult
	err    error
	calls  int
}

func (s *aiClassifierStub) Classify(context.Context, domain.AIClassifyInput) (domain.AIClassifyResult, error) {
	s.calls++
	if s.err != nil {
		return domain.AIClassifyResult{}, s.err
	}
	return s.result, nil
}

type aiStoreStub struct {
	claimAcquired bool
	completed     int
	failed        int
	cache         application.AIDetectionResult
	cacheFound    bool
}

func (s *aiStoreStub) ClaimAIDetection(context.Context, application.AIDetectionEvent) (application.AIDetectionClaim, error) {
	if s.claimAcquired {
		return application.AIDetectionClaim{Acquired: true}, nil
	}
	return application.AIDetectionClaim{Existing: &s.cache}, nil
}

func (s *aiStoreStub) CompleteAIDetection(context.Context, application.AIDetectionEvent, domain.AIClassifyResult) error {
	s.completed++
	return nil
}

func (s *aiStoreStub) FailAIDetection(context.Context, application.AIDetectionEvent, application.AIDetectionResult) error {
	s.failed++
	return nil
}

func (s *aiStoreStub) FindCachedAIDetection(context.Context, application.AIDetectionCacheKey) (application.AIDetectionResult, bool, error) {
	return s.cache, s.cacheFound, nil
}

func TestAIDetectionProcessorModes(t *testing.T) {
	t.Parallel()

	aiSpam := domain.AIClassifyResult{
		Label: domain.AILabelSpam, Category: "ai_ad", Confidence: 0.91,
		ConfidenceSource: domain.AIConfidenceModelReported, ReasonCode: "commercial_solicitation", SafeAction: domain.AISafeActionDelete,
	}
	tests := []struct {
		name       string
		aiMode     application.Mode
		appMode    application.Mode
		wantSpam   bool
		wantAction domain.Action
		wantMode   application.Mode
	}{
		{name: "observe 只記錄", aiMode: application.ModeObserve, appMode: application.ModeEnforce, wantMode: application.ModeEnforce},
		{name: "delete only 只刪除候選", aiMode: application.ModeDeleteOnly, appMode: application.ModeEnforce, wantSpam: true, wantAction: domain.ActionDelete, wantMode: application.ModeDeleteOnly},
		{name: "enforce 一般違規輔助", aiMode: application.ModeEnforce, appMode: application.ModeEnforce, wantSpam: true, wantAction: domain.ActionProgressive, wantMode: application.ModeEnforce},
		{name: "app observe 不升級", aiMode: application.ModeEnforce, appMode: application.ModeObserve, wantMode: application.ModeObserve},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			processor := newTestAIProcessor(t, tt.aiMode, &aiClassifierStub{result: aiSpam}, &aiStoreStub{claimAcquired: true}, nil)
			result, mode := processor.Evaluate(context.Background(), testMessage(), "fingerprint", ambiguousResult(), tt.appMode)
			if result.Spam != tt.wantSpam || result.Action != tt.wantAction || mode != tt.wantMode {
				t.Fatalf("result=%+v mode=%s", result, mode)
			}
		})
	}
}

func TestAIDetectionProcessorSafetyPolicies(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		rule     domain.Result
		aiResult domain.AIClassifyResult
		wantSpam bool
	}{
		{
			name: "低 confidence 不處置",
			rule: ambiguousResult(),
			aiResult: domain.AIClassifyResult{
				Label: domain.AILabelSpam, Category: "ai_ad", Confidence: 0.10,
				ConfidenceSource: domain.AIConfidenceModelReported, ReasonCode: "low", SafeAction: domain.AISafeActionDelete,
			},
		},
		{
			name: "unavailable confidence 不處置",
			rule: ambiguousResult(),
			aiResult: domain.AIClassifyResult{
				Label: domain.AILabelSpam, Category: "ai_ad", Confidence: 1,
				ConfidenceSource: domain.AIConfidenceUnavailable, ReasonCode: "none", SafeAction: domain.AISafeActionDelete,
			},
		},
		{
			name: "AI ham 不升級",
			rule: ambiguousResult(),
			aiResult: domain.AIClassifyResult{
				Label: domain.AILabelHam, Category: "normal", Confidence: 0.95,
				ConfidenceSource: domain.AIConfidenceModelReported, ReasonCode: "normal", SafeAction: domain.AISafeActionNone,
			},
		},
		{
			name: "規則 spam 不呼叫 AI 且不覆寫",
			rule: domain.Result{Spam: true, CategoryID: "yaml", Severity: domain.SeverityCritical, Action: domain.ActionBan, Score: 100, Threshold: 80},
			aiResult: domain.AIClassifyResult{
				Label: domain.AILabelHam, Category: "normal", Confidence: 0.95,
				ConfidenceSource: domain.AIConfidenceModelReported, ReasonCode: "normal", SafeAction: domain.AISafeActionNone,
			},
			wantSpam: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			classifier := &aiClassifierStub{result: tt.aiResult}
			processor := newTestAIProcessor(t, application.ModeEnforce, classifier, &aiStoreStub{claimAcquired: true}, nil)
			result, _ := processor.Evaluate(context.Background(), testMessage(), "fingerprint", tt.rule, application.ModeEnforce)
			if result.Spam != tt.wantSpam {
				t.Fatalf("result=%+v", result)
			}
			if tt.rule.Spam && classifier.calls != 0 {
				t.Fatal("明確規則 spam 不應呼叫 AI")
			}
		})
	}
}

func TestAIDetectionProcessorSafeDegrade(t *testing.T) {
	t.Parallel()

	processor := newTestAIProcessor(t, application.ModeEnforce, &aiClassifierStub{err: errors.New("provider timeout")}, &aiStoreStub{claimAcquired: true}, nil)
	result, mode := processor.Evaluate(context.Background(), testMessage(), "fingerprint", ambiguousResult(), application.ModeEnforce)
	if result.Spam || mode != application.ModeEnforce {
		t.Fatalf("provider 失敗應維持規則結果：result=%+v mode=%s", result, mode)
	}
}

func TestAIDetectionProcessorSemanticFailureDoesNotBlockAI(t *testing.T) {
	t.Parallel()

	classifier := &aiClassifierStub{result: domain.AIClassifyResult{
		Label: domain.AILabelSpam, Category: "ai_ad", Confidence: 0.91,
		ConfidenceSource: domain.AIConfidenceModelReported, ReasonCode: "commercial_solicitation", SafeAction: domain.AISafeActionDelete,
	}}
	semantic := &application.SemanticLookupPolicy{
		Embeddings:   &embeddingProviderSpy{},
		Memory:       &semanticMemorySpy{err: errors.New("pgvector unavailable")},
		MaxTextRunes: 800, MaxNeighbors: 5, SpamSimilarityThreshold: 0.90, HamSimilarityThreshold: 0.95,
	}
	processor := newTestAIProcessor(t, application.ModeDeleteOnly, classifier, &aiStoreStub{claimAcquired: true}, semantic)
	result, mode := processor.Evaluate(context.Background(), testMessage(), "fingerprint", ambiguousResult(), application.ModeEnforce)
	if !result.Spam || result.Action != domain.ActionDelete || mode != application.ModeDeleteOnly || classifier.calls != 1 {
		t.Fatalf("semantic 失敗仍應允許 AI：result=%+v mode=%s calls=%d", result, mode, classifier.calls)
	}
}

func TestProcessorIntegratesAIDetection(t *testing.T) {
	t.Parallel()

	store := memory.NewStore(time.Minute, 100)
	telegram := &telegramSpy{}
	aiProcessor := newTestAIProcessor(t, application.ModeDeleteOnly, &aiClassifierStub{result: domain.AIClassifyResult{
		Label: domain.AILabelSpam, Category: "ai_ad", Confidence: 0.91,
		ConfidenceSource: domain.AIConfidenceModelReported, ReasonCode: "commercial_solicitation", SafeAction: domain.AISafeActionDelete,
	}}, &aiStoreStub{claimAcquired: true}, nil)
	processor := application.NewProcessor(
		detectorStub{ambiguousResult()}, store, store, store, store, telegram,
		application.ModeEnforce, []byte("01234567890123456789012345678901"),
		application.WithAIDetectionProcessor(aiProcessor),
	)
	result, err := processor.Process(context.Background(), domain.Message{UpdateID: 40, ChatID: 2, MessageID: 3, UserID: 4, Text: "app 下載 @x"})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Spam || len(telegram.actions) != 1 || telegram.actions[0] != "delete" {
		t.Fatalf("result=%+v actions=%v", result, telegram.actions)
	}
}

func newTestAIProcessor(t *testing.T, mode application.Mode, classifier application.AIClassifier, store application.AIDetectionStore, semantic *application.SemanticLookupPolicy) *application.AIDetectionProcessor {
	t.Helper()

	processor, err := application.NewAIDetectionProcessor(application.AIDetectionProcessorPolicy{
		Enabled: true, Mode: mode, Provider: "test", Model: "classifier", PromptVersion: "ai-spam-v1",
		SchemaVersion: "v1", MaxTextRunes: 800, MinConfidence: 0.85, CacheTTL: time.Hour,
	}, application.AITriggerPolicy{OnlyWhenAmbiguous: true}, store, classifier, semantic)
	if err != nil {
		t.Fatal(err)
	}
	return processor
}

func ambiguousResult() domain.Result {
	return domain.Result{Score: 20, Threshold: 80, RuleVersion: "rules-v1", Signals: []string{"telegram_mention"}}
}

func testMessage() domain.Message {
	return domain.Message{UpdateID: 1, ChatID: 2, MessageID: 3, UserID: 4, Text: "app 下載 @x"}
}
