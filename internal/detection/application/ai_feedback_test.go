package application_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/vincent119/tg_spam_bot/internal/detection/application"
	"github.com/vincent119/tg_spam_bot/internal/detection/domain"
)

type feedbackReaderSpy struct {
	evidence    map[int64]application.ManualFeedbackEvidence
	evidenceErr error
	epochs      []uint64
	epochErr    error
	epochCalls  int
	chatIDs     []int64
}

func (s *feedbackReaderSpy) FindEffectiveFeedback(_ context.Context, chatID int64, _ string) (application.ManualFeedbackEvidence, error) {
	s.chatIDs = append(s.chatIDs, chatID)
	if s.evidenceErr != nil {
		return application.ManualFeedbackEvidence{}, s.evidenceErr
	}
	return s.evidence[chatID], nil
}

func (s *feedbackReaderSpy) ManualFeedbackEpoch(context.Context, int64) (uint64, error) {
	s.epochCalls++
	if s.epochErr != nil {
		return 0, s.epochErr
	}
	if len(s.epochs) == 0 {
		return 0, nil
	}
	index := min(s.epochCalls-1, len(s.epochs)-1)
	return s.epochs[index], nil
}

type feedbackAIStoreSpy struct {
	aiStoreStub
	event application.AIDetectionEvent
	key   application.AIDetectionCacheKey
}

func (s *feedbackAIStoreSpy) ClaimAIDetection(ctx context.Context, event application.AIDetectionEvent) (application.AIDetectionClaim, error) {
	s.event = event
	return s.aiStoreStub.ClaimAIDetection(ctx, event)
}

func (s *feedbackAIStoreSpy) FindCachedAIDetection(ctx context.Context, key application.AIDetectionCacheKey) (application.AIDetectionResult, bool, error) {
	s.key = key
	return s.aiStoreStub.FindCachedAIDetection(ctx, key)
}

func newFeedbackAIProcessor(t *testing.T, classifier application.AIClassifier, store application.AIDetectionStore, feedback application.ManualFeedbackReader) *application.AIDetectionProcessor {
	t.Helper()
	processor, err := application.NewAIDetectionProcessor(application.AIDetectionProcessorPolicy{
		Enabled: true, Mode: application.ModeDeleteOnly, Provider: "test", Model: "classifier", PromptVersion: "v1",
		SchemaVersion: "v1", MaxTextRunes: 800, MinConfidence: 0.85, CacheTTL: time.Hour,
	}, application.AITriggerPolicy{OnlyWhenAmbiguous: true}, store, classifier, nil, application.WithManualFeedbackReader(feedback))
	if err != nil {
		t.Fatal(err)
	}
	return processor
}

func feedbackSpamResult() domain.AIClassifyResult {
	return domain.AIClassifyResult{
		Label: domain.AILabelSpam, Category: "simulated_spam", Confidence: 0.95,
		ConfidenceSource: domain.AIConfidenceModelReported, ReasonCode: "test", SafeAction: domain.AISafeActionDelete,
	}
}

func TestAIDetectionProcessorManualFeedback(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name           string
		evidence       application.ManualFeedbackEvidence
		evidenceErr    error
		epochErr       error
		rule           domain.Result
		wantCalls      int
		wantSpam       bool
		wantSignal     string
		wantEpochCalls int
	}{
		{name: "本群正常標記阻止舊 AI 判定", evidence: application.ManualFeedbackEvidence{Present: true, Label: domain.AILabelHam}, rule: ambiguousResult(), wantSignal: application.SignalManualFeedbackHam},
		{name: "本群垃圾標記成為弱訊號", evidence: application.ManualFeedbackEvidence{Present: true, Label: domain.AILabelSpam}, wantCalls: 1, wantSpam: true, wantSignal: application.SignalManualFeedbackSpam, wantEpochCalls: 3},
		{name: "矛盾標記不採用", evidence: application.ManualFeedbackEvidence{Present: true, Conflict: true}},
		{name: "標記查詢失敗安全降級", evidenceErr: errors.New("查詢失敗"), rule: ambiguousResult(), wantSignal: application.SignalAIUncertain},
		{name: "版本查詢失敗不使用舊快取", epochErr: errors.New("查詢失敗"), rule: ambiguousResult(), wantSignal: application.SignalAIUncertain, wantEpochCalls: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			feedback := &feedbackReaderSpy{evidence: map[int64]application.ManualFeedbackEvidence{2: tt.evidence}, evidenceErr: tt.evidenceErr, epochErr: tt.epochErr, epochs: []uint64{5}}
			classifier := &aiClassifierStub{result: feedbackSpamResult()}
			store := &feedbackAIStoreSpy{aiStoreStub: aiStoreStub{claimAcquired: true}}
			processor := newFeedbackAIProcessor(t, classifier, store, feedback)
			result, _ := processor.Evaluate(t.Context(), testMessage(), "fingerprint", tt.rule, application.ModeEnforce)
			if classifier.calls != tt.wantCalls || result.Spam != tt.wantSpam || feedback.epochCalls != tt.wantEpochCalls {
				t.Fatalf("AI 呼叫=%d，結果=%+v，版本查詢=%d", classifier.calls, result, feedback.epochCalls)
			}
			if tt.wantSignal != "" && !slices.Contains(result.Signals, tt.wantSignal) {
				t.Fatalf("缺少標記或安全降級訊號 %q：%v", tt.wantSignal, result.Signals)
			}
			if !slices.Equal(feedback.chatIDs, []int64{2}) {
				t.Fatalf("標記查詢跨群或缺失：%v", feedback.chatIDs)
			}
			if tt.wantCalls > 0 && (store.event.FeedbackEpoch != 5 || store.key.ChatID != 2 || store.key.FeedbackEpoch != 5) {
				t.Fatalf("AI cache 未帶群組與版本：event=%+v key=%+v", store.event, store.key)
			}
		})
	}
}

func TestAIDetectionProcessorRejectsStaleFeedbackEpoch(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		claim       bool
		cacheFound  bool
		cachedEpoch uint64
		epochs      []uint64
		wantCalls   int
		wantFailed  int
		wantSpam    bool
	}{
		{name: "舊完成事件不重用", cachedEpoch: 4, epochs: []uint64{5}},
		{name: "分類中標記變更", claim: true, epochs: []uint64{5, 6}, wantCalls: 1, wantFailed: 1},
		{name: "分類落庫後標記變更", claim: true, epochs: []uint64{5, 5, 6}, wantCalls: 1, wantFailed: 1},
		{name: "快取讀取後標記變更", claim: true, cacheFound: true, cachedEpoch: 5, epochs: []uint64{5, 6}, wantFailed: 1},
		{name: "快取落庫後標記變更", claim: true, cacheFound: true, cachedEpoch: 5, epochs: []uint64{5, 5, 6}, wantFailed: 1},
		{name: "同版本快取可節省模型呼叫", claim: true, cacheFound: true, cachedEpoch: 5, epochs: []uint64{5, 5}, wantSpam: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			feedback := &feedbackReaderSpy{epochs: tt.epochs}
			classifier := &aiClassifierStub{result: feedbackSpamResult()}
			store := &feedbackAIStoreSpy{aiStoreStub: aiStoreStub{
				claimAcquired: tt.claim, cacheFound: tt.cacheFound,
				cache: application.AIDetectionResult{Status: "completed", FeedbackEpoch: tt.cachedEpoch, Result: feedbackSpamResult()},
			}}
			processor := newFeedbackAIProcessor(t, classifier, store, feedback)
			result, _ := processor.Evaluate(t.Context(), testMessage(), "fingerprint", ambiguousResult(), application.ModeEnforce)
			if result.Spam != tt.wantSpam || classifier.calls != tt.wantCalls || store.failed != tt.wantFailed {
				t.Fatalf("修訂後沿用舊判定或動作錯誤：result=%+v classifier=%d failed=%d", result, classifier.calls, store.failed)
			}
			if !tt.wantSpam && !slices.Contains(result.Signals, application.SignalAIUncertain) {
				t.Fatalf("舊版本不可當作安全放行：%+v", result)
			}
		})
	}
}

func TestAIDetectionProcessorManualHamDoesNotOverrideClearRule(t *testing.T) {
	t.Parallel()
	feedback := &feedbackReaderSpy{evidence: map[int64]application.ManualFeedbackEvidence{2: {Present: true, Label: domain.AILabelHam}}}
	classifier := &aiClassifierStub{result: feedbackSpamResult()}
	store := &feedbackAIStoreSpy{aiStoreStub: aiStoreStub{claimAcquired: true}}
	processor := newFeedbackAIProcessor(t, classifier, store, feedback)
	rule := domain.Result{Spam: true, CategoryID: "rule", Action: domain.ActionBan, Severity: domain.SeverityCritical, Score: 100, Threshold: 80}
	result, _ := processor.Evaluate(t.Context(), testMessage(), "fingerprint", rule, application.ModeEnforce)
	if !result.Spam || result.CategoryID != "rule" || classifier.calls != 0 || len(feedback.chatIDs) != 0 {
		t.Fatalf("正常標記不應推翻明確規則：%+v，分類呼叫=%d，標記查詢=%v", result, classifier.calls, feedback.chatIDs)
	}
}
