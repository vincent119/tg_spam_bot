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

type manualSampleStoreSpy struct {
	sample      application.ManualSample
	inserted    bool
	completedID uint64
	failedID    uint64
	errorCode   string
}

func (s *manualSampleStoreSpy) CreateManualSample(_ context.Context, sample application.ManualSample) (application.ManualSample, bool, error) {
	if s.sample.ID == 0 {
		s.sample = sample
		s.sample.ID = 1
		s.inserted = true
	}
	return s.sample, s.inserted, nil
}

func (s *manualSampleStoreSpy) PendingManualSamples(context.Context, int) ([]application.ManualSample, error) {
	return nil, nil
}

func (s *manualSampleStoreSpy) MarkManualSampleEmbeddingCompleted(_ context.Context, id uint64, _ time.Time) error {
	s.completedID = id
	return nil
}

func (s *manualSampleStoreSpy) MarkManualSampleEmbeddingFailed(_ context.Context, id uint64, errorCode, _ string, _ bool) error {
	s.failedID = id
	s.errorCode = errorCode
	return nil
}

type embeddingStoreSpy struct {
	record application.EmbeddingRecord
}

func (s *embeddingStoreSpy) SaveEmbedding(_ context.Context, record application.EmbeddingRecord) error {
	s.record = record
	return nil
}

func (s *embeddingStoreSpy) FindEmbeddingByFingerprint(context.Context, string, string, string, string, int, time.Time) (application.EmbeddingRecord, bool, error) {
	return application.EmbeddingRecord{}, false, nil
}

func TestManualFeedServiceSubmitSpamEmbedsSynchronously(t *testing.T) {
	t.Parallel()

	samples := &manualSampleStoreSpy{}
	embeddings := &embeddingProviderSpy{}
	store := &embeddingStoreSpy{}
	service, err := application.NewManualFeedService(samples, embeddings, store)
	if err != nil {
		t.Fatalf("NewManualFeedService() error = %v", err)
	}
	sample, inserted, err := service.SubmitSpam(context.Background(), application.ManualFeedInput{
		Sample: application.ManualSample{
			ChatID: 1, MessageID: 2, TargetUserID: 3, OperatorID: 4,
			ContentFingerprint: "fingerprint", Category: "agent_recruiting", Source: "feedspam",
		},
		Text: "一二三四五", MaxTextRunes: 3, EmbeddingTTL: time.Hour,
	})
	if err != nil {
		t.Fatalf("SubmitSpam() error = %v", err)
	}
	if !inserted || sample.Status != application.ManualSampleStatusEmbeddingCompleted || samples.completedID != 1 {
		t.Fatalf("sample=%+v inserted=%v completed=%d", sample, inserted, samples.completedID)
	}
	if embeddings.input.Text != "一二三" {
		t.Fatalf("embedding input = %q，預期截斷", embeddings.input.Text)
	}
	if store.record.ContentFingerprint != "fingerprint" || store.record.Category != "agent_recruiting" || store.record.ReasonCode != "manual_feedspam" {
		t.Fatalf("embedding record = %+v", store.record)
	}
}

func TestManualFeedServiceSubmitSpamMarksFailure(t *testing.T) {
	t.Parallel()

	samples := &manualSampleStoreSpy{}
	embeddings := &embeddingProviderSpy{err: errors.New("timeout")}
	store := &embeddingStoreSpy{}
	service, err := application.NewManualFeedService(samples, embeddings, store)
	if err != nil {
		t.Fatalf("NewManualFeedService() error = %v", err)
	}
	_, _, err = service.SubmitSpam(context.Background(), application.ManualFeedInput{
		Sample: application.ManualSample{ChatID: 1, MessageID: 2, TargetUserID: 3, OperatorID: 4, ContentFingerprint: "fingerprint", Category: "ad", Source: "feedspam"},
		Text:   "廣告",
	})
	if err == nil {
		t.Fatal("SubmitSpam() error = nil，預期錯誤")
	}
	if samples.failedID != 1 || samples.errorCode == "" {
		t.Fatalf("failedID=%d errorCode=%q", samples.failedID, samples.errorCode)
	}
}

func TestClusterServiceReport(t *testing.T) {
	t.Parallel()

	report := application.ClusterService{}.Report([]application.ClusterSample{
		{ID: "1", Category: "agent_recruiting", ReasonCode: "commercial_solicitation", Signals: []string{"telegram_mention"}, Evidence: []string{"代理招募"}},
		{ID: "2", Category: "agent_recruiting", ReasonCode: "commercial_solicitation", Signals: []string{"telegram_mention", "external_url"}, Evidence: []string{"代理招募"}},
	})
	if len(report.Clusters) != 1 {
		t.Fatalf("clusters = %+v", report.Clusters)
	}
	cluster := report.Clusters[0]
	if cluster.SampleCount != 2 || cluster.SuggestedCategory != "agent_recruiting" {
		t.Fatalf("cluster = %+v", cluster)
	}
	if !slices.Contains(cluster.Signals, "telegram_mention") || !slices.Contains(cluster.CandidateYAMLSummary, "代理招募") {
		t.Fatalf("cluster = %+v", cluster)
	}
	if slices.Contains(cluster.CandidateYAMLSummary, "完整原文") {
		t.Fatalf("聚類摘要不應包含完整原文：%+v", cluster)
	}
}

var _ application.EmbeddingProvider = (*embeddingProviderSpy)(nil)
var _ application.EmbeddingStore = (*embeddingStoreSpy)(nil)
var _ application.ManualSampleStore = (*manualSampleStoreSpy)(nil)
var _ = domain.AILabelSpam
