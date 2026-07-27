package detection

import (
	"strings"
	"testing"
	"time"

	appconfig "github.com/vincent119/tg_spam_bot/internal/config"
)

func TestBuildAIComponentsDisabled(t *testing.T) {
	t.Parallel()

	components, err := BuildAIComponents(t.Context(), configForDITest(), nil)
	if err != nil {
		t.Fatalf("BuildAIComponents() error = %v", err)
	}
	if components.Processor != nil || components.FeedSpamService != nil {
		t.Fatalf("AI 功能未啟用時不應建立元件：%+v", components)
	}
}

func TestBuildAIComponentsRejectsUnsupportedAIProvider(t *testing.T) {
	t.Parallel()

	cfg := configForDITest()
	cfg.AIDetection.Enabled = true
	cfg.AIDetection.Provider = "unknown"

	_, err := BuildAIComponents(t.Context(), cfg, nil)
	if err == nil || !strings.Contains(err.Error(), "不支援的 AI provider") {
		t.Fatalf("BuildAIComponents() error = %v，預期不支援 provider", err)
	}
}

func TestBuildAIComponentsRejectsUnsupportedEmbeddingProvider(t *testing.T) {
	t.Parallel()

	cfg := configForDITest()
	cfg.SemanticMemory.Enabled = true
	cfg.SemanticMemory.EmbeddingProvider = "unknown"

	_, err := BuildAIComponents(t.Context(), cfg, nil)
	if err == nil || !strings.Contains(err.Error(), "不支援的 embedding provider") {
		t.Fatalf("BuildAIComponents() error = %v，預期不支援 embedding provider", err)
	}
}

func TestAIDetectionModel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cfg  appconfig.AIDetectionConfig
		want string
	}{
		{
			name: "openai compatible",
			cfg: appconfig.AIDetectionConfig{
				Provider: appconfig.AIProviderOpenAICompatible,
				OpenAICompatible: appconfig.OpenAICompatibleConfig{
					Model: "spam-classifier",
				},
			},
			want: "spam-classifier",
		},
		{
			name: "bedrock",
			cfg: appconfig.AIDetectionConfig{
				Provider: appconfig.AIProviderBedrock,
				Bedrock: appconfig.BedrockConfig{
					ModelID: "amazon.nova-lite-v1:0",
				},
			},
			want: "amazon.nova-lite-v1:0",
		},
		{
			name: "unknown",
			cfg: appconfig.AIDetectionConfig{
				Provider: "unknown",
			},
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := aiDetectionModel(tt.cfg); got != tt.want {
				t.Fatalf("aiDetectionModel() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestValidateAIComponents(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		mutate     func(*appconfig.Config)
		components AIComponents
		wantErr    string
	}{
		{
			name:   "disabled",
			mutate: func(*appconfig.Config) {},
		},
		{
			name: "ai enabled missing processor",
			mutate: func(cfg *appconfig.Config) {
				cfg.AIDetection.Enabled = true
			},
			wantErr: "processor 未建立",
		},
		{
			name: "semantic memory enabled missing feedspam service",
			mutate: func(cfg *appconfig.Config) {
				cfg.SemanticMemory.Enabled = true
			},
			wantErr: "feedspam service 未建立",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := configForDITest()
			tt.mutate(&cfg)

			err := ValidateAIComponents(cfg, tt.components)
			if tt.wantErr == "" && err != nil {
				t.Fatalf("ValidateAIComponents() error = %v", err)
			}
			if tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)) {
				t.Fatalf("ValidateAIComponents() error = %v，預期包含 %q", err, tt.wantErr)
			}
		})
	}
}

func configForDITest() appconfig.Config {
	cfg := appconfig.Config{}
	cfg.AIDetection.Mode = appconfig.ModeObserve
	cfg.AIDetection.Timeout = time.Second
	cfg.AIDetection.MaxTextChars = 800
	cfg.AIDetection.MinConfidence = 0.85
	cfg.AIDetection.CacheTTL = time.Hour
	cfg.SemanticMemory.EmbeddingVersion = "v1"
	cfg.SemanticMemory.MaxNeighbors = 5
	cfg.SemanticMemory.SpamSimilarityThreshold = 0.9
	cfg.SemanticMemory.HamSimilarityThreshold = 0.95
	cfg.SemanticMemory.CacheTTL = time.Hour
	return cfg
}
