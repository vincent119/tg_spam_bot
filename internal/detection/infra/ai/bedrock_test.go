package ai

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	"github.com/aws/smithy-go"
	appconfig "github.com/vincent119/tg_spam_bot/internal/config"
	"github.com/vincent119/tg_spam_bot/internal/detection/domain"
)

func TestResolveBedrockCredentials(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cfg     appconfig.BedrockConfig
		wantErr bool
	}{
		{name: "IAM role", cfg: appconfig.BedrockConfig{AuthMode: appconfig.BedrockAuthModeIAMRole}},
		{name: "static keys", cfg: appconfig.BedrockConfig{AuthMode: appconfig.BedrockAuthModeStaticKeys, AccessKeyID: "access", SecretAccessKey: "secret"}},
		{name: "static keys 缺 secret", cfg: appconfig.BedrockConfig{AuthMode: appconfig.BedrockAuthModeStaticKeys, AccessKeyID: "access"}, wantErr: true},
		{name: "未知 auth mode", cfg: appconfig.BedrockConfig{AuthMode: "bad"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ResolveBedrockCredentials(tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ResolveBedrockCredentials() error = %v，wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNewBedrockFactoriesValidateConfig(t *testing.T) {
	t.Parallel()

	_, err := NewBedrockClassifier(context.Background(), appconfig.BedrockConfig{Region: "us-east-1", ModelID: "model", AuthMode: appconfig.BedrockAuthModeIAMRole}, time.Second)
	if err != nil {
		t.Fatalf("NewBedrockClassifier() error = %v", err)
	}
	_, err = NewBedrockEmbedding(context.Background(), appconfig.BedrockConfig{
		Region: "us-east-1", ModelID: "model", AuthMode: appconfig.BedrockAuthModeStaticKeys, AccessKeyID: "access", SecretAccessKey: "secret",
	}, "v1", 3, time.Second)
	if err != nil {
		t.Fatalf("NewBedrockEmbedding() error = %v", err)
	}
	if _, err := NewBedrockClassifier(context.Background(), appconfig.BedrockConfig{ModelID: "model", AuthMode: appconfig.BedrockAuthModeIAMRole}, time.Second); err == nil {
		t.Fatal("缺 region 應失敗")
	}
	if _, err := NewBedrockEmbedding(context.Background(), appconfig.BedrockConfig{Region: "us-east-1", AuthMode: appconfig.BedrockAuthModeIAMRole}, "v1", 3, time.Second); err == nil {
		t.Fatal("缺 model id 應失敗")
	}
}

type fakeConverseClient struct {
	output *bedrockruntime.ConverseOutput
	err    error
}

func (c fakeConverseClient) Converse(context.Context, *bedrockruntime.ConverseInput, ...func(*bedrockruntime.Options)) (*bedrockruntime.ConverseOutput, error) {
	if c.err != nil {
		return nil, c.err
	}
	return c.output, nil
}

type fakeInvokeClient struct {
	output *bedrockruntime.InvokeModelOutput
	err    error
}

func (c fakeInvokeClient) InvokeModel(context.Context, *bedrockruntime.InvokeModelInput, ...func(*bedrockruntime.Options)) (*bedrockruntime.InvokeModelOutput, error) {
	if c.err != nil {
		return nil, c.err
	}
	return c.output, nil
}

func TestBedrockClassifierClassify(t *testing.T) {
	t.Parallel()

	client := fakeConverseClient{output: &bedrockruntime.ConverseOutput{
		Output: &types.ConverseOutputMemberMessage{Value: types.Message{
			Role:    types.ConversationRoleAssistant,
			Content: []types.ContentBlock{&types.ContentBlockMemberText{Value: `{"label":"spam","category":"ad","confidence":0.91,"confidence_source":"model_reported","reason_code":"commercial_solicitation","evidence":["導流"],"safe_action":"delete"}`}},
		}},
	}}
	classifier, err := newBedrockClassifierWithClient("model", time.Second, client, nil)
	if err != nil {
		t.Fatalf("newBedrockClassifierWithClient() error = %v", err)
	}
	result, err := classifier.Classify(context.Background(), domain.AIClassifyInput{Text: "廣告", Signals: []string{"telegram_mention"}})
	if err != nil {
		t.Fatalf("Classify() error = %v", err)
	}
	if result.Label != domain.AILabelSpam || result.PromptVersion != PromptVersion {
		t.Fatalf("result = %+v", result)
	}
}

func TestBedrockEmbeddingEmbed(t *testing.T) {
	t.Parallel()

	client := fakeInvokeClient{output: &bedrockruntime.InvokeModelOutput{Body: []byte(`{"embedding":[0.1,0.2,0.3]}`)}}
	embedding, err := newBedrockEmbeddingWithClient("model", "v1", 3, time.Second, client, nil)
	if err != nil {
		t.Fatalf("newBedrockEmbeddingWithClient() error = %v", err)
	}
	result, err := embedding.Embed(context.Background(), domain.EmbeddingInput{Text: "廣告"})
	if err != nil {
		t.Fatalf("Embed() error = %v", err)
	}
	if result.Provider != "bedrock" || result.Model != "model" || result.Dimensions != 3 {
		t.Fatalf("result = %+v", result)
	}
}

func TestBedrockProviderErrorMappingAndMasking(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		err       error
		wantCode  string
		retryable bool
	}{
		{name: "權限錯誤", err: &smithy.GenericAPIError{Code: "AccessDeniedException", Message: "denied secret access session"}, wantCode: codePermissionDenied},
		{name: "限流", err: &smithy.GenericAPIError{Code: "ThrottlingException", Message: "slow down"}, wantCode: codeRateLimited, retryable: true},
		{name: "server error", err: &smithy.GenericAPIError{Code: "InternalServerException", Message: "failed"}, wantCode: codeServerError, retryable: true},
		{name: "timeout", err: &smithy.GenericAPIError{Code: "ModelTimeoutException", Message: "timeout"}, wantCode: codeTimeout, retryable: true},
		{name: "validation", err: &smithy.GenericAPIError{Code: "ValidationException", Message: "bad request"}, wantCode: codeInvalidResponse},
		{name: "unknown", err: &smithy.GenericAPIError{Code: "Unknown", Message: "unknown"}, wantCode: codeRequestFailed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := bedrockProviderError(context.Background(), tt.err, "secret", "access", "session")
			assertProviderError(t, err, tt.wantCode, tt.retryable)
			if containsAny(err.Error(), []string{"secret", "access", "session"}) {
				t.Fatalf("error 未遮蔽敏感資訊：%v", err)
			}
		})
	}
}

func TestBedrockEmbeddingErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		output   *bedrockruntime.InvokeModelOutput
		err      error
		wantCode string
	}{
		{name: "權限錯誤", err: &smithy.GenericAPIError{Code: "AccessDeniedException", Message: "denied"}, wantCode: codePermissionDenied},
		{name: "空向量", output: &bedrockruntime.InvokeModelOutput{Body: []byte(`{"embedding":[]}`)}, wantCode: codeInvalidResponse},
		{name: "維度不一致", output: &bedrockruntime.InvokeModelOutput{Body: []byte(`{"embedding":[0.1]}`)}, wantCode: codeInvalidResponse},
		{name: "invalid JSON", output: &bedrockruntime.InvokeModelOutput{Body: []byte(`{`)}, wantCode: codeInvalidResponse},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			embedding, err := newBedrockEmbeddingWithClient("model", "v1", 2, time.Second, fakeInvokeClient{output: tt.output, err: tt.err}, nil)
			if err != nil {
				t.Fatalf("newBedrockEmbeddingWithClient() error = %v", err)
			}
			_, err = embedding.Embed(context.Background(), domain.EmbeddingInput{Text: "廣告"})
			assertProviderError(t, err, tt.wantCode, tt.wantCode == codeRateLimited || tt.wantCode == codeServerError || tt.wantCode == codeTimeout)
		})
	}
}

func TestBedrockContextTimeout(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	err := bedrockProviderError(ctx, errors.New("context deadline exceeded"), "secret")
	assertProviderError(t, err, codeTimeout, true)
}

func TestParseBedrockEmbeddingVariants(t *testing.T) {
	t.Parallel()

	for _, body := range [][]byte{
		[]byte(`{"embedding":[0.1,0.2]}`),
		[]byte(`{"embeddings":[[0.1,0.2]]}`),
		[]byte(`{"data":[{"embedding":[0.1,0.2]}]}`),
	} {
		vector, err := parseBedrockEmbedding(body)
		if err != nil {
			t.Fatalf("parseBedrockEmbedding() error = %v", err)
		}
		if len(vector) != 2 {
			t.Fatalf("vector = %v", vector)
		}
	}
}

func containsAny(value string, needles []string) bool {
	for _, needle := range needles {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}
