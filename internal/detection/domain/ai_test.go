package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestParseAIClassifyResultJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		payload string
		wantErr string
	}{
		{
			name:    "有效輸出",
			payload: `{"label":"spam","category":"ad","confidence":0.92,"confidence_source":"model_reported","reason_code":"commercial_solicitation","evidence":["income"],"safe_action":"delete"}`,
		},
		{name: "無效 JSON", payload: `{`, wantErr: "invalid_json"},
		{
			name:    "未知 label",
			payload: `{"label":"bad","category":"ad","confidence":0.92,"confidence_source":"model_reported","reason_code":"commercial_solicitation","evidence":[],"safe_action":"delete"}`,
			wantErr: "invalid_label",
		},
		{
			name:    "未知 safe action",
			payload: `{"label":"spam","category":"ad","confidence":0.92,"confidence_source":"model_reported","reason_code":"commercial_solicitation","evidence":[],"safe_action":"ban"}`,
			wantErr: "invalid_safe_action",
		},
		{
			name:    "未知 confidence source",
			payload: `{"label":"spam","category":"ad","confidence":0.92,"confidence_source":"provider_magic","reason_code":"commercial_solicitation","evidence":[],"safe_action":"delete"}`,
			wantErr: "invalid_confidence_source",
		},
		{
			name:    "confidence 超出範圍",
			payload: `{"label":"spam","category":"ad","confidence":1.2,"confidence_source":"model_reported","reason_code":"commercial_solicitation","evidence":[],"safe_action":"delete"}`,
			wantErr: "invalid_confidence",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseAIClassifyResultJSON([]byte(tt.payload))
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("ParseAIClassifyResultJSON() error = %v", err)
				}
				if result.Label != AILabelSpam || result.Confidence != 0.92 || result.SafeAction != AISafeActionDelete {
					t.Fatalf("result = %+v", result)
				}
				return
			}
			var validationErr *AIValidationError
			if !errors.As(err, &validationErr) || validationErr.Code != tt.wantErr {
				t.Fatalf("error = %v，預期 code %q", err, tt.wantErr)
			}
		})
	}
}

func TestNewAIClassifyInputTruncatesAndCopiesSignals(t *testing.T) {
	t.Parallel()

	signals := []string{"telegram_mention"}
	input := NewAIClassifyInput("一二三四五", 1, 10, signals, 3)
	signals[0] = "mutated"

	if input.Text != "一二三" {
		t.Fatalf("Text = %q，預期截斷", input.Text)
	}
	if got := input.SignalsCopy(); len(got) != 1 || got[0] != "telegram_mention" {
		t.Fatalf("signals = %v", got)
	}
}

func TestAIClassifyResultTreatsUnavailableConfidenceAsValidButAuditable(t *testing.T) {
	t.Parallel()

	result := AIClassifyResult{
		Label:            AILabelUncertain,
		Category:         "unknown",
		Confidence:       0,
		ConfidenceSource: AIConfidenceUnavailable,
		ReasonCode:       "provider_without_confidence",
		SafeAction:       AISafeActionNone,
	}
	if err := result.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestEmbeddingResultValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		result  EmbeddingResult
		wantErr string
	}{
		{name: "有效向量", result: EmbeddingResult{Provider: "openai_compatible", Model: "embedding", Version: "v1", Dimensions: 3, Vector: []float32{0.1, 0.2, 0.3}}},
		{name: "缺 provider", result: EmbeddingResult{Model: "embedding", Version: "v1", Dimensions: 1, Vector: []float32{0.1}}, wantErr: "missing_embedding_provider"},
		{name: "空向量", result: EmbeddingResult{Provider: "openai_compatible", Model: "embedding", Version: "v1", Dimensions: 1}, wantErr: "empty_embedding_vector"},
		{name: "維度不一致", result: EmbeddingResult{Provider: "openai_compatible", Model: "embedding", Version: "v1", Dimensions: 2, Vector: []float32{0.1}}, wantErr: "embedding_dimensions_mismatch"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.result.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate() error = %v", err)
				}
				copy := tt.result.VectorCopy()
				copy[0] = 99
				if tt.result.Vector[0] == 99 {
					t.Fatal("VectorCopy() 不應回傳原 slice")
				}
				return
			}
			var validationErr *AIValidationError
			if !errors.As(err, &validationErr) || validationErr.Code != tt.wantErr {
				t.Fatalf("error = %v，預期 code %q", err, tt.wantErr)
			}
		})
	}
}

func TestNewEmbeddingInputTruncates(t *testing.T) {
	t.Parallel()

	input := NewEmbeddingInput(strings.Repeat("廣", 5), 2)
	if input.Text != "廣廣" {
		t.Fatalf("Text = %q，預期截斷", input.Text)
	}
}
