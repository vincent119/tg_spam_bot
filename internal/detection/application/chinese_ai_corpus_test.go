package application_test

import (
	"path/filepath"
	"testing"

	"github.com/vincent119/tg_spam_bot/internal/detection/application"
	"github.com/vincent119/tg_spam_bot/internal/detection/domain"
	"github.com/vincent119/tg_spam_bot/internal/detection/rules"
)

func TestChineseModerationMockedAIPolicy(t *testing.T) {
	t.Parallel()
	ruleSet, err := rules.LoadDir(filepath.Join("..", "..", "..", "configs", "rules"))
	if err != nil {
		t.Fatal(err)
	}
	detector, err := domain.NewDetector(ruleSet, domain.NewNormalizer(domain.OpenCCConverter{}, 4096), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name       string
		text       string
		signals    []string
		aiResult   domain.AIClassifyResult
		wantCalls  int
		wantSpam   bool
		wantAction domain.Action
	}{
		{
			name: "已命中中文規則不由 AI 正常標籤推翻", text: "带码来干 收米一天1W",
			aiResult: domain.AIClassifyResult{Label: domain.AILabelHam, Confidence: 0.95, ConfidenceSource: domain.AIConfidenceModelReported, SafeAction: domain.AISafeActionNone},
			wantSpam: true, wantAction: domain.ActionProgressive,
		},
		{
			name: "正常重複短句的 AI 模擬垃圾只允許刪除", text: "收到", signals: []string{domain.SignalRepeatedContent},
			aiResult:  domain.AIClassifyResult{Label: domain.AILabelSpam, Category: "simulated_spam", Confidence: 0.95, ConfidenceSource: domain.AIConfidenceModelReported, SafeAction: domain.AISafeActionDelete},
			wantCalls: 1, wantSpam: true, wantAction: domain.ActionDelete,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			message := domain.Message{ChatID: 1, UserID: 2, MessageID: 3, Text: tt.text}
			rule := detector.Detect(message, tt.signals...)
			classifier := &aiClassifierStub{result: tt.aiResult}
			processor := newTestAIProcessor(t, application.ModeDeleteOnly, classifier, &aiStoreStub{claimAcquired: true}, nil)
			result, mode := processor.Evaluate(t.Context(), message, "fixture-fingerprint", rule, application.ModeEnforce)
			if classifier.calls != tt.wantCalls || result.Spam != tt.wantSpam || result.Action != tt.wantAction {
				t.Fatalf("AI 呼叫=%d，結果=%+v，模式=%q", classifier.calls, result, mode)
			}
			if tt.wantCalls > 0 && mode != application.ModeDeleteOnly {
				t.Fatalf("AI 模擬垃圾不可提升封鎖權限：模式=%q", mode)
			}
		})
	}
}
