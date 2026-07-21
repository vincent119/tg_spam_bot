package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	"github.com/aws/smithy-go"
	appconfig "github.com/vincent119/tg_spam_bot/internal/config"
	"github.com/vincent119/tg_spam_bot/internal/detection/domain"
)

type bedrockConverseAPI interface {
	Converse(ctx context.Context, params *bedrockruntime.ConverseInput, optFns ...func(*bedrockruntime.Options)) (*bedrockruntime.ConverseOutput, error)
}

type bedrockInvokeAPI interface {
	InvokeModel(ctx context.Context, params *bedrockruntime.InvokeModelInput, optFns ...func(*bedrockruntime.Options)) (*bedrockruntime.InvokeModelOutput, error)
}

// BedrockCredentials 保存已驗證的 credential 模式，不輸出秘密值。
type BedrockCredentials struct {
	AuthMode appconfig.BedrockAuthMode
}

// ResolveBedrockCredentials 驗證 Bedrock credential 設定。
func ResolveBedrockCredentials(cfg appconfig.BedrockConfig) (BedrockCredentials, error) {
	switch cfg.AuthMode {
	case appconfig.BedrockAuthModeIAMRole:
		return BedrockCredentials{AuthMode: cfg.AuthMode}, nil
	case appconfig.BedrockAuthModeStaticKeys:
		if strings.TrimSpace(cfg.AccessKeyID) == "" || strings.TrimSpace(cfg.SecretAccessKey) == "" {
			return BedrockCredentials{}, errors.New("bedrock static keys require access key id and secret access key")
		}
		return BedrockCredentials{AuthMode: cfg.AuthMode}, nil
	default:
		return BedrockCredentials{}, fmt.Errorf("unsupported bedrock auth mode %q", cfg.AuthMode)
	}
}

// BedrockClassifier 透過 Bedrock Converse API 呼叫支援訊息介面的模型。
type BedrockClassifier struct {
	modelID string
	timeout time.Duration
	client  bedrockConverseAPI
	secrets []string
}

// NewBedrockClassifier 建立 Bedrock classifier adapter，credential 由 AWS SDK 管理。
func NewBedrockClassifier(ctx context.Context, cfg appconfig.BedrockConfig, timeout time.Duration) (*BedrockClassifier, error) {
	awsCfg, secrets, err := loadBedrockAWSConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return newBedrockClassifierWithClient(cfg.ModelID, timeout, bedrockruntime.NewFromConfig(awsCfg), secrets)
}

func newBedrockClassifierWithClient(modelID string, timeout time.Duration, client bedrockConverseAPI, secrets []string) (*BedrockClassifier, error) {
	modelID = strings.TrimSpace(modelID)
	if modelID == "" || client == nil {
		return nil, errors.New("bedrock classifier model id and client are required")
	}
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	return &BedrockClassifier{modelID: modelID, timeout: timeout, client: client, secrets: append([]string(nil), secrets...)}, nil
}

// Classify 送出最小化摘要並解析 Bedrock 模型 JSON 回應。
func (c *BedrockClassifier) Classify(ctx context.Context, input domain.AIClassifyInput) (domain.AIClassifyResult, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	userContent, err := json.Marshal(struct {
		Text          string   `json:"text"`
		RuleScore     int      `json:"rule_score"`
		RuleThreshold int      `json:"rule_threshold"`
		Signals       []string `json:"signals"`
	}{
		Text: input.Text, RuleScore: input.RuleScore, RuleThreshold: input.RuleThreshold, Signals: input.SignalsCopy(),
	})
	if err != nil {
		return domain.AIClassifyResult{}, fmt.Errorf("encode bedrock classify input: %w", err)
	}
	temperature := float32(0)
	maxTokens := int32(512)
	output, err := c.client.Converse(ctx, &bedrockruntime.ConverseInput{
		ModelId: aws.String(c.modelID),
		System:  []types.SystemContentBlock{&types.SystemContentBlockMemberText{Value: classifierSystemPrompt()}},
		Messages: []types.Message{{
			Role:    types.ConversationRoleUser,
			Content: []types.ContentBlock{&types.ContentBlockMemberText{Value: string(userContent)}},
		}},
		InferenceConfig: &types.InferenceConfiguration{Temperature: &temperature, MaxTokens: &maxTokens},
	})
	if err != nil {
		return domain.AIClassifyResult{}, bedrockProviderError(ctx, err, c.secrets...)
	}
	content := bedrockTextOutput(output)
	if strings.TrimSpace(content) == "" {
		return domain.AIClassifyResult{}, newProviderError(codeInvalidResponse, false, "missing bedrock converse text", c.secrets...)
	}
	result, err := domain.ParseAIClassifyResultJSON([]byte(content))
	if err != nil {
		return domain.AIClassifyResult{}, newProviderError(codeInvalidResponse, false, err.Error(), c.secrets...)
	}
	result.PromptVersion = PromptVersion
	return result, nil
}

// BedrockEmbedding 透過 Bedrock InvokeModel 呼叫 embedding 模型。
type BedrockEmbedding struct {
	modelID            string
	version            string
	expectedDimensions int
	timeout            time.Duration
	client             bedrockInvokeAPI
	secrets            []string
}

// NewBedrockEmbedding 建立 Bedrock embedding adapter，credential 由 AWS SDK 管理。
func NewBedrockEmbedding(ctx context.Context, cfg appconfig.BedrockConfig, version string, expectedDimensions int, timeout time.Duration) (*BedrockEmbedding, error) {
	awsCfg, secrets, err := loadBedrockAWSConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return newBedrockEmbeddingWithClient(cfg.ModelID, version, expectedDimensions, timeout, bedrockruntime.NewFromConfig(awsCfg), secrets)
}

func newBedrockEmbeddingWithClient(modelID, version string, expectedDimensions int, timeout time.Duration, client bedrockInvokeAPI, secrets []string) (*BedrockEmbedding, error) {
	modelID = strings.TrimSpace(modelID)
	version = strings.TrimSpace(version)
	if modelID == "" || version == "" || client == nil {
		return nil, errors.New("bedrock embedding model id, version and client are required")
	}
	if expectedDimensions < 0 {
		return nil, errors.New("bedrock embedding expected dimensions must not be negative")
	}
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	return &BedrockEmbedding{modelID: modelID, version: version, expectedDimensions: expectedDimensions, timeout: timeout, client: client, secrets: append([]string(nil), secrets...)}, nil
}

// Embed 產生 Bedrock embedding，並驗證回傳向量維度。
func (e *BedrockEmbedding) Embed(ctx context.Context, input domain.EmbeddingInput) (domain.EmbeddingResult, error) {
	ctx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()

	body, err := json.Marshal(map[string]string{"inputText": input.Text})
	if err != nil {
		return domain.EmbeddingResult{}, fmt.Errorf("encode bedrock embedding input: %w", err)
	}
	output, err := e.client.InvokeModel(ctx, &bedrockruntime.InvokeModelInput{
		ModelId: aws.String(e.modelID), ContentType: aws.String("application/json"), Accept: aws.String("application/json"), Body: body,
	})
	if err != nil {
		return domain.EmbeddingResult{}, bedrockProviderError(ctx, err, e.secrets...)
	}
	vector, err := parseBedrockEmbedding(output.Body)
	if err != nil {
		return domain.EmbeddingResult{}, newProviderError(codeInvalidResponse, false, err.Error(), e.secrets...)
	}
	dimensions := len(vector)
	if e.expectedDimensions > 0 && dimensions != e.expectedDimensions {
		return domain.EmbeddingResult{}, newProviderError(codeInvalidResponse, false, fmt.Sprintf("embedding dimensions=%d, want %d", dimensions, e.expectedDimensions), e.secrets...)
	}
	result := domain.EmbeddingResult{
		Provider: "bedrock", Model: e.modelID, Version: e.version, Dimensions: dimensions, Vector: append([]float32(nil), vector...),
	}
	if err := result.Validate(); err != nil {
		return domain.EmbeddingResult{}, newProviderError(codeInvalidResponse, false, err.Error(), e.secrets...)
	}
	return result, nil
}

func loadBedrockAWSConfig(ctx context.Context, cfg appconfig.BedrockConfig) (aws.Config, []string, error) {
	if strings.TrimSpace(cfg.Region) == "" || strings.TrimSpace(cfg.ModelID) == "" {
		return aws.Config{}, nil, errors.New("bedrock region and model id are required")
	}
	if _, err := ResolveBedrockCredentials(cfg); err != nil {
		return aws.Config{}, nil, err
	}
	secrets := []string{cfg.AccessKeyID, cfg.SecretAccessKey, cfg.SessionToken}
	opts := []func(*awsconfig.LoadOptions) error{awsconfig.WithRegion(cfg.Region)}
	if cfg.AuthMode == appconfig.BedrockAuthModeStaticKeys {
		opts = append(opts, awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, cfg.SessionToken)))
	}
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return aws.Config{}, nil, fmt.Errorf("load bedrock aws config: %w", err)
	}
	return awsCfg, secrets, nil
}

func bedrockTextOutput(output *bedrockruntime.ConverseOutput) string {
	if output == nil {
		return ""
	}
	message, ok := output.Output.(*types.ConverseOutputMemberMessage)
	if !ok {
		return ""
	}
	for _, block := range message.Value.Content {
		text, ok := block.(*types.ContentBlockMemberText)
		if ok {
			return text.Value
		}
	}
	return ""
}

func parseBedrockEmbedding(data []byte) ([]float32, error) {
	var payload struct {
		Embedding  []float32   `json:"embedding"`
		Embeddings [][]float32 `json:"embeddings"`
		Data       []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("decode bedrock embedding response: %w", err)
	}
	switch {
	case len(payload.Embedding) > 0:
		return append([]float32(nil), payload.Embedding...), nil
	case len(payload.Embeddings) > 0 && len(payload.Embeddings[0]) > 0:
		return append([]float32(nil), payload.Embeddings[0]...), nil
	case len(payload.Data) > 0 && len(payload.Data[0].Embedding) > 0:
		return append([]float32(nil), payload.Data[0].Embedding...), nil
	default:
		return nil, errors.New("missing bedrock embedding vector")
	}
}

func bedrockProviderError(ctx context.Context, err error, secrets ...string) *ProviderError {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return newProviderError(codeTimeout, true, err.Error(), secrets...)
	}
	var apiErr smithy.APIError
	if !errors.As(err, &apiErr) {
		return newProviderError(codeRequestFailed, true, err.Error(), secrets...)
	}
	switch strings.ToLower(apiErr.ErrorCode()) {
	case "accessdeniedexception", "unrecognizedclientexception", "unauthorizedexception", "forbiddenexception":
		return newProviderError(codePermissionDenied, false, apiErr.ErrorMessage(), secrets...)
	case "throttlingexception", "toomanyrequestsexception", "servicequotaexceededexception":
		return newProviderError(codeRateLimited, true, apiErr.ErrorMessage(), secrets...)
	case "modeltimeoutexception":
		return newProviderError(codeTimeout, true, apiErr.ErrorMessage(), secrets...)
	case "internalserverexception", "serviceunavailableexception", "modelnotreadyexception":
		return newProviderError(codeServerError, true, apiErr.ErrorMessage(), secrets...)
	case "validationexception":
		return newProviderError(codeInvalidResponse, false, apiErr.ErrorMessage(), secrets...)
	default:
		return newProviderError(codeRequestFailed, false, apiErr.ErrorMessage(), secrets...)
	}
}
