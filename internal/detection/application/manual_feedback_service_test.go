package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/vincent119/tg_spam_bot/internal/detection/application"
	"github.com/vincent119/tg_spam_bot/internal/detection/domain"
)

type feedbackStoreSpy struct {
	saved        application.ManualFeedback
	embedding    application.ScopedFeedbackEmbedding
	markCode     string
	saveErr      error
	embeddingErr error
}

func (s *feedbackStoreSpy) SaveFeedback(_ context.Context, feedback application.ManualFeedback) (application.ManualFeedback, bool, error) {
	if s.saveErr != nil {
		return application.ManualFeedback{}, false, s.saveErr
	}
	feedback.ID = 1
	feedback.Revision = 1
	s.saved = feedback
	return feedback, true, nil
}

func (s *feedbackStoreSpy) MarkFeedbackEmbeddingFailed(_ context.Context, _, _ int64, _ uint64, code string) error {
	s.markCode = code
	return nil
}

func (s *feedbackStoreSpy) SaveScopedFeedbackEmbedding(_ context.Context, record application.ScopedFeedbackEmbedding) error {
	s.embedding = record
	return s.embeddingErr
}

func (*feedbackStoreSpy) FindEffectiveFeedback(context.Context, int64, string) (application.ManualFeedbackEvidence, error) {
	return application.ManualFeedbackEvidence{}, nil
}

func (*feedbackStoreSpy) SearchScopedManualSimilar(context.Context, int64, domain.EmbeddingResult, int) ([]domain.SemanticMatch, error) {
	return nil, nil
}

func (*feedbackStoreSpy) ManualFeedbackEpoch(context.Context, int64) (uint64, error) {
	return 0, nil
}

func feedbackInput(text string) application.ManualFeedbackInput {
	return application.ManualFeedbackInput{
		Feedback: application.ManualFeedback{
			ChatID: -1001, MessageID: 12, TargetUserID: 3, OperatorID: 4, CommandUpdateID: 25,
			ContentFingerprint: "fingerprint", Label: domain.AILabelSpam, Category: "ad", Source: "spam_command",
		},
		Text: text, MaxTextRunes: 800, EmbeddingTTL: time.Hour,
	}
}

func TestManualFeedbackServiceStages(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		text       string
		embeddings bool
		embedErr   error
		storeErr   error
		wantStatus string
		wantCode   string
	}{
		{name: "有原文與 provider", text: "來收米", embeddings: true, wantStatus: application.ManualFeedbackEmbeddingCompleted},
		{name: "已刪目標沒有原文", wantStatus: application.ManualFeedbackEmbeddingUnavailable},
		{name: "語意功能停用", text: "來收米", wantStatus: application.ManualFeedbackEmbeddingDisabled},
		{name: "provider 暫時失敗", text: "來收米", embeddings: true, embedErr: errors.New("timeout"), wantStatus: application.ManualFeedbackEmbeddingFailed, wantCode: "temporary_failure"},
		{name: "舊向量工作不覆寫修正", text: "來收米", embeddings: true, storeErr: application.ErrStaleManualFeedback, wantStatus: application.ManualFeedbackEmbeddingSuperseded},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &feedbackStoreSpy{embeddingErr: tt.storeErr}
			var provider application.EmbeddingProvider
			if tt.embeddings {
				provider = &embeddingProviderSpy{err: tt.embedErr}
			}
			service, err := application.NewManualFeedbackService(store, provider)
			if err != nil {
				t.Fatal(err)
			}
			result, err := service.Submit(t.Context(), feedbackInput(tt.text))
			if err != nil || !result.Saved || result.EmbeddingStatus != tt.wantStatus || result.EmbeddingErrorCode != tt.wantCode {
				t.Fatalf("Submit()=%+v err=%v", result, err)
			}
			if store.saved.ContentFingerprint != "fingerprint" || store.saved.ChatID != -1001 {
				t.Fatalf("save=%+v", store.saved)
			}
			if tt.wantStatus == application.ManualFeedbackEmbeddingCompleted && store.embedding.Feedback.Revision != 1 {
				t.Fatalf("embedding=%+v", store.embedding)
			}
			if tt.wantCode != "" && store.markCode != tt.wantCode {
				t.Fatalf("markCode=%q", store.markCode)
			}
		})
	}
}

func TestManualFeedbackServiceSaveFailurePreventsEmbedding(t *testing.T) {
	t.Parallel()
	store := &feedbackStoreSpy{saveErr: errors.New("db unavailable")}
	provider := &embeddingProviderSpy{}
	service, err := application.NewManualFeedbackService(store, provider)
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Submit(t.Context(), feedbackInput("來收米"))
	if err == nil || result.Saved || provider.input.Text != "" {
		t.Fatalf("Submit()=%+v err=%v provider=%+v", result, err, provider.input)
	}
}
