package ai

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/vincent119/tg_spam_bot/internal/detection/domain"
)

func TestOpenAICompatibleClassifierClassify(t *testing.T) {
	t.Parallel()

	var authHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"label\":\"spam\",\"category\":\"ad\",\"confidence\":0.91,\"confidence_source\":\"model_reported\",\"reason_code\":\"commercial_solicitation\",\"evidence\":[\"導流\"],\"safe_action\":\"delete\"}"}}]}`))
	}))
	defer server.Close()

	classifier, err := NewOpenAICompatibleClassifier(server.URL, "chat-model", "secret-key", time.Second, server.Client())
	if err != nil {
		t.Fatalf("NewOpenAICompatibleClassifier() error = %v", err)
	}
	result, err := classifier.Classify(context.Background(), domain.AIClassifyInput{
		Text: "抖音禮物項目 @x", RuleScore: 20, RuleThreshold: 80, Signals: []string{"telegram_mention"},
	})
	if err != nil {
		t.Fatalf("Classify() error = %v", err)
	}
	if authHeader != "Bearer secret-key" {
		t.Fatalf("Authorization = %q", authHeader)
	}
	if result.Label != domain.AILabelSpam || result.PromptVersion != PromptVersion || result.SafeAction != domain.AISafeActionDelete {
		t.Fatalf("result = %+v", result)
	}
}

func TestOpenAICompatibleClassifierErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		status    int
		body      string
		wantCode  string
		retryable bool
	}{
		{name: "權限錯誤", status: http.StatusUnauthorized, body: `secret-key https://user:pass@example.com`, wantCode: codePermissionDenied},
		{name: "限流", status: http.StatusTooManyRequests, body: `rate limited`, wantCode: codeRateLimited, retryable: true},
		{name: "server error", status: http.StatusBadGateway, body: `bad gateway`, wantCode: codeServerError, retryable: true},
		{name: "invalid response", status: http.StatusOK, body: `{"choices":[{"message":{"content":"{"}}]}`, wantCode: codeInvalidResponse},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()

			classifier, err := NewOpenAICompatibleClassifier(server.URL, "chat-model", "secret-key", time.Second, server.Client())
			if err != nil {
				t.Fatalf("NewOpenAICompatibleClassifier() error = %v", err)
			}
			_, err = classifier.Classify(context.Background(), domain.AIClassifyInput{Text: "test"})
			assertProviderError(t, err, tt.wantCode, tt.retryable)
			if strings.Contains(err.Error(), "secret-key") || strings.Contains(err.Error(), "user:pass") {
				t.Fatalf("error 未遮蔽敏感資訊：%v", err)
			}
		})
	}
}

func TestOpenAICompatibleClassifierTimeout(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(50 * time.Millisecond)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	client := server.Client()
	client.Timeout = time.Millisecond
	classifier, err := NewOpenAICompatibleClassifier(server.URL, "chat-model", "secret-key", time.Second, client)
	if err != nil {
		t.Fatalf("NewOpenAICompatibleClassifier() error = %v", err)
	}
	_, err = classifier.Classify(context.Background(), domain.AIClassifyInput{Text: "test"})
	assertProviderError(t, err, codeTimeout, true)
}

func TestOpenAICompatibleEmbeddingEmbed(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"embedding":[0.1,0.2,0.3]}]}`))
	}))
	defer server.Close()

	embedding, err := NewOpenAICompatibleEmbedding(server.URL, "embedding-model", "secret-key", "v1", 3, time.Second, server.Client())
	if err != nil {
		t.Fatalf("NewOpenAICompatibleEmbedding() error = %v", err)
	}
	result, err := embedding.Embed(context.Background(), domain.EmbeddingInput{Text: "測試"})
	if err != nil {
		t.Fatalf("Embed() error = %v", err)
	}
	if result.Provider != "openai_compatible" || result.Model != "embedding-model" || result.Version != "v1" || result.Dimensions != 3 {
		t.Fatalf("result = %+v", result)
	}
}

func TestOpenAICompatibleEmbeddingErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		status   int
		body     string
		dims     int
		wantCode string
	}{
		{name: "空向量", status: http.StatusOK, body: `{"data":[{"embedding":[]}]}`, dims: 0, wantCode: codeInvalidResponse},
		{name: "維度不一致", status: http.StatusOK, body: `{"data":[{"embedding":[0.1]}]}`, dims: 2, wantCode: codeInvalidResponse},
		{name: "權限錯誤", status: http.StatusForbidden, body: `secret-key`, dims: 0, wantCode: codePermissionDenied},
		{name: "限流", status: http.StatusTooManyRequests, body: `rate limited`, dims: 0, wantCode: codeRateLimited},
		{name: "server error", status: http.StatusInternalServerError, body: `failed`, dims: 0, wantCode: codeServerError},
		{name: "invalid JSON", status: http.StatusOK, body: `{`, dims: 0, wantCode: codeInvalidResponse},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()

			embedding, err := NewOpenAICompatibleEmbedding(server.URL, "embedding-model", "secret-key", "v1", tt.dims, time.Second, server.Client())
			if err != nil {
				t.Fatalf("NewOpenAICompatibleEmbedding() error = %v", err)
			}
			_, err = embedding.Embed(context.Background(), domain.EmbeddingInput{Text: "測試"})
			assertProviderError(t, err, tt.wantCode, tt.status == http.StatusTooManyRequests || tt.status >= http.StatusInternalServerError)
			if strings.Contains(err.Error(), "secret-key") {
				t.Fatalf("error 未遮蔽敏感資訊：%v", err)
			}
		})
	}
}

func TestOpenAICompatibleEmbeddingTimeout(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(50 * time.Millisecond)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	client := server.Client()
	client.Timeout = time.Millisecond
	embedding, err := NewOpenAICompatibleEmbedding(server.URL, "embedding-model", "secret-key", "v1", 0, time.Second, client)
	if err != nil {
		t.Fatalf("NewOpenAICompatibleEmbedding() error = %v", err)
	}
	_, err = embedding.Embed(context.Background(), domain.EmbeddingInput{Text: "測試"})
	assertProviderError(t, err, codeTimeout, true)
}

func assertProviderError(t *testing.T, err error, wantCode string, wantRetryable bool) {
	t.Helper()

	var providerErr *ProviderError
	if !errors.As(err, &providerErr) {
		t.Fatalf("error = %v，預期 ProviderError", err)
	}
	if providerErr.ErrorCode() != wantCode {
		t.Fatalf("ErrorCode() = %q，預期 %q", providerErr.ErrorCode(), wantCode)
	}
	if providerErr.IsRetryable() != wantRetryable {
		t.Fatalf("IsRetryable() = %v，預期 %v", providerErr.IsRetryable(), wantRetryable)
	}
}
