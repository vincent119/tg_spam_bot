// Package ai 提供外部 AI provider adapter。
package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/vincent119/tg_spam_bot/internal/detection/domain"
)

const (
	// PromptVersion 固定 classifier prompt 與 JSON schema 版本，供快取與稽核隔離。
	PromptVersion = "ai-spam-v1"

	codePermissionDenied = "permission_denied"
	codeRateLimited      = "rate_limited"
	codeServerError      = "provider_server_error"
	codeTimeout          = "provider_timeout"
	codeInvalidResponse  = "invalid_response"
	codeRequestFailed    = "request_failed"
)

var credentialURLPattern = regexp.MustCompile(`(?i)(https?://)[^\s/@:]+:[^\s/@]+@`)

// ProviderError 是已遮蔽敏感資訊的 provider 穩定錯誤分類。
type ProviderError struct {
	code      string
	retryable bool
	message   string
}

func (e *ProviderError) Error() string {
	return fmt.Sprintf("ai provider 失敗：code=%s message=%s", e.code, e.message)
}

// ErrorCode 回傳可寫入稽核的穩定錯誤碼。
func (e *ProviderError) ErrorCode() string { return e.code }

// IsRetryable 標示是否適合稍後重試。
func (e *ProviderError) IsRetryable() bool { return e.retryable }

// OpenAICompatibleClassifier 呼叫 OpenAI-compatible chat completions API。
type OpenAICompatibleClassifier struct {
	endpoint string
	model    string
	apiKey   string
	http     *http.Client
}

// NewOpenAICompatibleClassifier 建立 classifier adapter。
func NewOpenAICompatibleClassifier(endpoint, model, apiKey string, timeout time.Duration, client *http.Client) (*OpenAICompatibleClassifier, error) {
	endpoint = strings.TrimSpace(endpoint)
	model = strings.TrimSpace(model)
	if endpoint == "" || model == "" || apiKey == "" {
		return nil, errors.New("openai compatible endpoint, model and api key are required")
	}
	if err := validateEndpoint(endpoint); err != nil {
		return nil, err
	}
	if client == nil {
		if timeout <= 0 {
			timeout = 3 * time.Second
		}
		client = &http.Client{Timeout: timeout}
	}
	return &OpenAICompatibleClassifier{endpoint: endpoint, model: model, apiKey: apiKey, http: client}, nil
}

// Classify 送出最小化訊息摘要並驗證 provider JSON-only 回應。
func (c *OpenAICompatibleClassifier) Classify(ctx context.Context, input domain.AIClassifyInput) (domain.AIClassifyResult, error) {
	userPayload := struct {
		Text          string   `json:"text"`
		RuleScore     int      `json:"rule_score"`
		RuleThreshold int      `json:"rule_threshold"`
		Signals       []string `json:"signals"`
	}{
		Text:          input.Text,
		RuleScore:     input.RuleScore,
		RuleThreshold: input.RuleThreshold,
		Signals:       input.SignalsCopy(),
	}
	userContent, err := json.Marshal(userPayload)
	if err != nil {
		return domain.AIClassifyResult{}, fmt.Errorf("encode ai classify input: %w", err)
	}
	requestBody := map[string]any{
		"model": c.model,
		"messages": []map[string]string{
			{"role": "system", "content": classifierSystemPrompt()},
			{"role": "user", "content": string(userContent)},
		},
		"response_format": map[string]string{"type": "json_object"},
		"temperature":     0,
	}
	var response chatCompletionResponse
	if err := c.doJSON(ctx, requestBody, &response); err != nil {
		return domain.AIClassifyResult{}, err
	}
	if len(response.Choices) == 0 || strings.TrimSpace(response.Choices[0].Message.Content) == "" {
		return domain.AIClassifyResult{}, newProviderError(codeInvalidResponse, false, "missing chat completion content", c.apiKey)
	}
	result, err := domain.ParseAIClassifyResultJSON([]byte(response.Choices[0].Message.Content))
	if err != nil {
		return domain.AIClassifyResult{}, newProviderError(codeInvalidResponse, false, err.Error(), c.apiKey)
	}
	result.PromptVersion = PromptVersion
	return result, nil
}

// OpenAICompatibleEmbedding 呼叫 OpenAI-compatible embeddings API。
type OpenAICompatibleEmbedding struct {
	endpoint           string
	model              string
	apiKey             string
	version            string
	expectedDimensions int
	http               *http.Client
}

// NewOpenAICompatibleEmbedding 建立 embedding adapter。
func NewOpenAICompatibleEmbedding(endpoint, model, apiKey, version string, expectedDimensions int, timeout time.Duration, client *http.Client) (*OpenAICompatibleEmbedding, error) {
	endpoint = strings.TrimSpace(endpoint)
	model = strings.TrimSpace(model)
	version = strings.TrimSpace(version)
	if endpoint == "" || model == "" || apiKey == "" || version == "" {
		return nil, errors.New("openai compatible embedding endpoint, model, api key and version are required")
	}
	if expectedDimensions < 0 {
		return nil, errors.New("embedding expected dimensions must not be negative")
	}
	if err := validateEndpoint(endpoint); err != nil {
		return nil, err
	}
	if client == nil {
		if timeout <= 0 {
			timeout = 3 * time.Second
		}
		client = &http.Client{Timeout: timeout}
	}
	return &OpenAICompatibleEmbedding{endpoint: endpoint, model: model, apiKey: apiKey, version: version, expectedDimensions: expectedDimensions, http: client}, nil
}

// Embed 產生文字 embedding，並依設定驗證向量維度。
func (e *OpenAICompatibleEmbedding) Embed(ctx context.Context, input domain.EmbeddingInput) (domain.EmbeddingResult, error) {
	requestBody := map[string]any{"model": e.model, "input": input.Text}
	var response embeddingResponse
	if err := e.doJSON(ctx, requestBody, &response); err != nil {
		return domain.EmbeddingResult{}, err
	}
	if len(response.Data) == 0 || len(response.Data[0].Embedding) == 0 {
		return domain.EmbeddingResult{}, newProviderError(codeInvalidResponse, false, "missing embedding vector", e.apiKey)
	}
	dimensions := len(response.Data[0].Embedding)
	if e.expectedDimensions > 0 && dimensions != e.expectedDimensions {
		return domain.EmbeddingResult{}, newProviderError(codeInvalidResponse, false, fmt.Sprintf("embedding dimensions=%d, want %d", dimensions, e.expectedDimensions), e.apiKey)
	}
	result := domain.EmbeddingResult{
		Provider: "openai_compatible", Model: e.model, Version: e.version,
		Dimensions: dimensions, Vector: append([]float32(nil), response.Data[0].Embedding...),
	}
	if err := result.Validate(); err != nil {
		return domain.EmbeddingResult{}, newProviderError(codeInvalidResponse, false, err.Error(), e.apiKey)
	}
	return result, nil
}

type chatCompletionResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

type embeddingResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
}

func (c *OpenAICompatibleClassifier) doJSON(ctx context.Context, payload, target any) error {
	return doOpenAICompatibleJSON(ctx, c.http, c.endpoint, c.apiKey, payload, target)
}

func (e *OpenAICompatibleEmbedding) doJSON(ctx context.Context, payload, target any) error {
	return doOpenAICompatibleJSON(ctx, e.http, e.endpoint, e.apiKey, payload, target)
}

func doOpenAICompatibleJSON(ctx context.Context, client *http.Client, endpoint, apiKey string, payload, target any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode openai compatible request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create openai compatible request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		code := codeRequestFailed
		if errors.Is(ctx.Err(), context.DeadlineExceeded) || strings.Contains(strings.ToLower(err.Error()), "timeout") || strings.Contains(strings.ToLower(err.Error()), "deadline exceeded") {
			code = codeTimeout
		}
		return newProviderError(code, true, err.Error(), apiKey)
	}
	defer func() { _ = resp.Body.Close() }()
	limited, err := io.ReadAll(io.LimitReader(resp.Body, 8192))
	if err != nil {
		return fmt.Errorf("read openai compatible response: %w", err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		code, retryable := classifyStatus(resp.StatusCode)
		return newProviderError(code, retryable, string(limited), apiKey)
	}
	if err := json.Unmarshal(limited, target); err != nil {
		return newProviderError(codeInvalidResponse, false, err.Error(), apiKey)
	}
	return nil
}

func validateEndpoint(endpoint string) error {
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return fmt.Errorf("parse openai compatible endpoint: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" || parsed.Host == "" {
		return errors.New("openai compatible endpoint must be absolute http or https url")
	}
	return nil
}

func classifyStatus(status int) (string, bool) {
	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return codePermissionDenied, false
	case status == http.StatusTooManyRequests:
		return codeRateLimited, true
	case status >= http.StatusInternalServerError:
		return codeServerError, true
	default:
		return codeInvalidResponse, false
	}
}

func newProviderError(code string, retryable bool, message string, secrets ...string) *ProviderError {
	return &ProviderError{code: code, retryable: retryable, message: maskSecrets(message, secrets...)}
}

func maskSecrets(value string, secrets ...string) string {
	for _, secret := range secrets {
		if secret == "" {
			continue
		}
		value = strings.ReplaceAll(value, secret, "[已遮蔽]")
	}
	return credentialURLPattern.ReplaceAllString(value, `${1}[已遮蔽]@`)
}

func classifierSystemPrompt() string {
	return strings.Join([]string{
		"你是 Telegram 群組垃圾訊息分類器，只能輸出 JSON。",
		"允許 label：spam、ham、uncertain。",
		"允許 confidence_source：model_reported、heuristic、unavailable。",
		"允許 safe_action：none、delete、restrict_candidate。",
		"必填欄位：label、category、confidence、confidence_source、reason_code、evidence、safe_action。",
		"不要輸出完整使用者個資、秘密值或額外自然語言。",
	}, "\n")
}
