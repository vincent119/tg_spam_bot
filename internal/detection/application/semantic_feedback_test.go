package application_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/vincent119/tg_spam_bot/internal/detection/application"
	"github.com/vincent119/tg_spam_bot/internal/detection/domain"
)

type scopedSemanticMemorySpy struct {
	chatIDs []int64
	matches map[int64][]domain.SemanticMatch
	err     error
}

type semanticFeedbackSpy struct {
	evidence     map[int64]application.ManualFeedbackEvidence
	queries      []int64
	fingerprints []string
	err          error
}

func (s *semanticFeedbackSpy) FindEffectiveFeedback(_ context.Context, chatID int64, fingerprint string) (application.ManualFeedbackEvidence, error) {
	s.queries = append(s.queries, chatID)
	s.fingerprints = append(s.fingerprints, fingerprint)
	if s.err != nil {
		return application.ManualFeedbackEvidence{}, s.err
	}
	return s.evidence[chatID], nil
}

func (s *scopedSemanticMemorySpy) SearchScopedManualSimilar(_ context.Context, chatID int64, _ domain.EmbeddingResult, _ int) ([]domain.SemanticMatch, error) {
	s.chatIDs = append(s.chatIDs, chatID)
	if s.err != nil {
		return nil, s.err
	}
	return domain.SemanticMatchesCopy(s.matches[chatID]), nil
}

func TestSemanticLookupPolicyChatScopedFeedback(t *testing.T) {
	t.Parallel()
	scoped := &scopedSemanticMemorySpy{matches: map[int64][]domain.SemanticMatch{
		1: {{SourceEventID: "manual-1", Label: domain.AILabelHam, Category: "normal", ReasonCode: "manual_feedback", Similarity: 0.97}},
	}}
	policy := application.SemanticLookupPolicy{
		Embeddings: &embeddingProviderSpy{}, Memory: &semanticMemorySpy{}, ScopedMemory: scoped,
		MaxTextRunes: 800, MaxNeighbors: 5, SpamSimilarityThreshold: 0.90, HamSimilarityThreshold: 0.95,
	}
	first, err := policy.ObserveForChat(t.Context(), 1, "查詢文字")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(first.Signals, application.SignalSemanticSimilarSpam) || !slices.Contains(first.Signals, application.SignalSemanticSimilarHam) || len(first.Matches) != 2 {
		t.Fatalf("本群有效向量未合併：%+v", first)
	}
	second, err := policy.ObserveForChat(t.Context(), 2, "查詢文字")
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(second.Signals, application.SignalSemanticSimilarHam) || !slices.Equal(scoped.chatIDs, []int64{1, 2}) {
		t.Fatalf("他群不可取得本群標記：%+v，查詢群=%v", second, scoped.chatIDs)
	}
	legacy, err := policy.Observe(t.Context(), "查詢文字")
	if err != nil || len(legacy.Matches) != 1 || !slices.Equal(scoped.chatIDs, []int64{1, 2}) {
		t.Fatalf("舊 Observe 契約不應讀取群組向量：%+v，查詢群=%v，錯誤=%v", legacy, scoped.chatIDs, err)
	}
}

func TestSemanticLookupPolicyScopedLookupFailure(t *testing.T) {
	t.Parallel()
	policy := application.SemanticLookupPolicy{
		Embeddings: &embeddingProviderSpy{}, Memory: &semanticMemorySpy{},
		ScopedMemory: &scopedSemanticMemorySpy{err: errors.New("群組向量查詢失敗")},
		MaxTextRunes: 800, MaxNeighbors: 5, SpamSimilarityThreshold: 0.90, HamSimilarityThreshold: 0.95,
	}
	if observation, err := policy.ObserveForChat(t.Context(), 1, "查詢文字"); err == nil || len(observation.Signals) != 0 {
		t.Fatalf("群組證據不明時不得沿用全域訊號：%+v，錯誤=%v", observation, err)
	}
}

func TestSemanticLookupPolicySuppressesCorrectedGlobalSpam(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		evidence application.ManualFeedbackEvidence
		wantSpam bool
	}{
		{name: "本群正常修正", evidence: application.ManualFeedbackEvidence{Present: true, Label: domain.AILabelHam}},
		{name: "本群矛盾修正", evidence: application.ManualFeedbackEvidence{Present: true, Conflict: true}},
		{name: "本群垃圾標記", evidence: application.ManualFeedbackEvidence{Present: true, Label: domain.AILabelSpam}, wantSpam: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			feedback := &semanticFeedbackSpy{evidence: map[int64]application.ManualFeedbackEvidence{1: tt.evidence}}
			policy := application.SemanticLookupPolicy{
				Embeddings: &embeddingProviderSpy{}, Memory: &semanticMemorySpy{}, Feedbacks: feedback,
				MaxTextRunes: 800, MaxNeighbors: 5, SpamSimilarityThreshold: 0.90, HamSimilarityThreshold: 0.95,
			}
			first, err := policy.ObserveForChat(t.Context(), 1, "相似文字")
			if err != nil {
				t.Fatal(err)
			}
			if slices.Contains(first.Signals, application.SignalSemanticSimilarSpam) != tt.wantSpam || !slices.Equal(feedback.queries, []int64{1}) || !slices.Equal(feedback.fingerprints, []string{"event-1"}) {
				t.Fatalf("同群修正未正確過濾全域 spam：%+v，查詢群=%v，來源指紋=%v", first, feedback.queries, feedback.fingerprints)
			}
			second, err := policy.ObserveForChat(t.Context(), 2, "相似文字")
			if err != nil || !slices.Contains(second.Signals, application.SignalSemanticSimilarSpam) {
				t.Fatalf("本群修正不得影響他群：%+v，錯誤=%v", second, err)
			}
		})
	}
}

func TestSemanticLookupPolicyFeedbackLookupFailure(t *testing.T) {
	t.Parallel()
	policy := application.SemanticLookupPolicy{
		Embeddings: &embeddingProviderSpy{}, Memory: &semanticMemorySpy{},
		Feedbacks:    &semanticFeedbackSpy{err: errors.New("標記查詢失敗")},
		MaxTextRunes: 800, MaxNeighbors: 5, SpamSimilarityThreshold: 0.90, HamSimilarityThreshold: 0.95,
	}
	if observation, err := policy.ObserveForChat(t.Context(), 1, "相似文字"); err == nil || len(observation.Signals) != 0 {
		t.Fatalf("標記查詢失敗不可沿用舊全域 spam：%+v，錯誤=%v", observation, err)
	}
}
