package domain

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// AILabel 是 AI provider 回傳的穩定垃圾訊息分類。
type AILabel string

const (
	AILabelSpam      AILabel = "spam"
	AILabelHam       AILabel = "ham"
	AILabelUncertain AILabel = "uncertain"
)

// AIConfidenceSource 標示信心分數是否來自模型、adapter 推估或不可用。
type AIConfidenceSource string

const (
	AIConfidenceModelReported AIConfidenceSource = "model_reported"
	AIConfidenceHeuristic     AIConfidenceSource = "heuristic"
	AIConfidenceUnavailable   AIConfidenceSource = "unavailable"
)

// AISafeAction 是 AI provider 建議的最高安全動作；實際處置仍由 application policy 決定。
type AISafeAction string

const (
	AISafeActionNone              AISafeAction = "none"
	AISafeActionDelete            AISafeAction = "delete"
	AISafeActionRestrictCandidate AISafeAction = "restrict_candidate"
)

// AIValidationError 保存可寫入稽核的穩定驗證錯誤碼。
type AIValidationError struct {
	Code string
	Err  error
}

func (e *AIValidationError) Error() string { return e.Err.Error() }

func (e *AIValidationError) Unwrap() error { return e.Err }

// AIClassifyInput 是傳給 AI classifier 的最小化輸入。
type AIClassifyInput struct {
	Text          string
	RuleScore     int
	RuleThreshold int
	Signals       []string
}

// NewAIClassifyInput 建立已截斷且複製 slice 邊界的 AI classifier 輸入。
func NewAIClassifyInput(text string, score, threshold int, signals []string, maxTextRunes int) AIClassifyInput {
	return AIClassifyInput{
		Text:          limitRunes(text, maxTextRunes),
		RuleScore:     score,
		RuleThreshold: threshold,
		Signals:       append([]string(nil), signals...),
	}
}

// SignalsCopy 回傳獨立副本，避免 AI 輸入外洩可變 slice。
func (i AIClassifyInput) SignalsCopy() []string {
	return append([]string(nil), i.Signals...)
}

// AIClassifyResult 是 provider-neutral 的 AI 判定結果。
type AIClassifyResult struct {
	Label            AILabel
	Category         string
	Confidence       float64
	ConfidenceSource AIConfidenceSource
	ReasonCode       string
	Evidence         []string
	SafeAction       AISafeAction
	PromptVersion    string
}

// EvidenceCopy 回傳獨立副本，避免呼叫端修改領域結果。
func (r AIClassifyResult) EvidenceCopy() []string {
	return append([]string(nil), r.Evidence...)
}

// ParseAIClassifyResultJSON 驗證 provider JSON 輸出並轉成穩定領域型別。
func ParseAIClassifyResultJSON(data []byte) (AIClassifyResult, error) {
	var payload struct {
		Label            AILabel            `json:"label"`
		Category         string             `json:"category"`
		Confidence       float64            `json:"confidence"`
		ConfidenceSource AIConfidenceSource `json:"confidence_source"`
		ReasonCode       string             `json:"reason_code"`
		Evidence         []string           `json:"evidence"`
		SafeAction       AISafeAction       `json:"safe_action"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return AIClassifyResult{}, newAIValidationError("invalid_json", "decode ai result: %w", err)
	}
	result := AIClassifyResult{
		Label:            payload.Label,
		Category:         strings.TrimSpace(payload.Category),
		Confidence:       payload.Confidence,
		ConfidenceSource: payload.ConfidenceSource,
		ReasonCode:       strings.TrimSpace(payload.ReasonCode),
		Evidence:         append([]string(nil), payload.Evidence...),
		SafeAction:       payload.SafeAction,
	}
	if err := result.Validate(); err != nil {
		return AIClassifyResult{}, err
	}
	return result, nil
}

// Validate 確認 AI 判定結果只包含允許列舉與合理分數。
func (r AIClassifyResult) Validate() error {
	if r.Label != AILabelSpam && r.Label != AILabelHam && r.Label != AILabelUncertain {
		return newAIValidationError("invalid_label", "invalid ai label %q", r.Label)
	}
	if r.Confidence < 0 || r.Confidence > 1 {
		return newAIValidationError("invalid_confidence", "invalid ai confidence %.4f", r.Confidence)
	}
	if r.ConfidenceSource != AIConfidenceModelReported && r.ConfidenceSource != AIConfidenceHeuristic && r.ConfidenceSource != AIConfidenceUnavailable {
		return newAIValidationError("invalid_confidence_source", "invalid ai confidence source %q", r.ConfidenceSource)
	}
	if r.SafeAction != AISafeActionNone && r.SafeAction != AISafeActionDelete && r.SafeAction != AISafeActionRestrictCandidate {
		return newAIValidationError("invalid_safe_action", "invalid ai safe action %q", r.SafeAction)
	}
	if r.Category == "" {
		return newAIValidationError("missing_category", "ai category is required")
	}
	if r.ReasonCode == "" {
		return newAIValidationError("missing_reason_code", "ai reason code is required")
	}
	return nil
}

// EmbeddingInput 是產生向量前的最小化文字輸入。
type EmbeddingInput struct {
	Text string
}

// NewEmbeddingInput 建立已截斷的 embedding 輸入。
func NewEmbeddingInput(text string, maxTextRunes int) EmbeddingInput {
	return EmbeddingInput{Text: limitRunes(text, maxTextRunes)}
}

// EmbeddingResult 保存 provider-neutral 的向量結果與模型隔離資訊。
type EmbeddingResult struct {
	Provider   string
	Model      string
	Version    string
	Dimensions int
	Vector     []float32
}

// VectorCopy 回傳獨立副本，避免呼叫端修改領域結果。
func (r EmbeddingResult) VectorCopy() []float32 {
	return append([]float32(nil), r.Vector...)
}

// Validate 確認 embedding 可安全寫入語意記憶庫。
func (r EmbeddingResult) Validate() error {
	if strings.TrimSpace(r.Provider) == "" {
		return newAIValidationError("missing_embedding_provider", "embedding provider is required")
	}
	if strings.TrimSpace(r.Model) == "" {
		return newAIValidationError("missing_embedding_model", "embedding model is required")
	}
	if strings.TrimSpace(r.Version) == "" {
		return newAIValidationError("missing_embedding_version", "embedding version is required")
	}
	if len(r.Vector) == 0 {
		return newAIValidationError("empty_embedding_vector", "embedding vector is empty")
	}
	if r.Dimensions <= 0 {
		return newAIValidationError("invalid_embedding_dimensions", "embedding dimensions must be positive")
	}
	if len(r.Vector) != r.Dimensions {
		return newAIValidationError("embedding_dimensions_mismatch", "embedding vector dimensions=%d, want %d", len(r.Vector), r.Dimensions)
	}
	return nil
}

// SemanticMatch 是語意記憶庫回傳的相似歷史案例摘要，不包含完整原文。
type SemanticMatch struct {
	SourceEventID string
	Label         AILabel
	Category      string
	ReasonCode    string
	Similarity    float64
}

// SemanticMatchesCopy 回傳相似案例的獨立副本，避免跨層共享可變 slice。
func SemanticMatchesCopy(matches []SemanticMatch) []SemanticMatch {
	return append([]SemanticMatch(nil), matches...)
}

func newAIValidationError(code, format string, args ...any) error {
	err := fmt.Errorf(format, args...)
	if len(args) == 0 {
		err = errors.New(format)
	}
	return &AIValidationError{Code: code, Err: err}
}
