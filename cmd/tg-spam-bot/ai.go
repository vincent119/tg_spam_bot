package main

import (
	"context"
	"errors"
	"fmt"

	appconfig "github.com/vincent119/tg_spam_bot/internal/config"
	detectionapp "github.com/vincent119/tg_spam_bot/internal/detection/application"
	infraai "github.com/vincent119/tg_spam_bot/internal/detection/infra/ai"
	"github.com/vincent119/tg_spam_bot/internal/detection/infra/postgres"
)

const aiSchemaVersion = "ai-result-v1"

type aiComponents struct {
	processor       *detectionapp.AIDetectionProcessor
	feedSpamService *detectionapp.ManualFeedService
}

func buildAIComponents(ctx context.Context, cfg appconfig.Config, store *postgres.Store) (aiComponents, error) {
	var components aiComponents
	var semantic *detectionapp.SemanticLookupPolicy
	var embeddings detectionapp.EmbeddingProvider

	if cfg.SemanticMemory.Enabled {
		provider, err := buildEmbeddingProvider(ctx, cfg.SemanticMemory)
		if err != nil {
			return aiComponents{}, err
		}
		embeddings = provider
		semantic = &detectionapp.SemanticLookupPolicy{
			Embeddings:              embeddings,
			Memory:                  store,
			MaxTextRunes:            cfg.AIDetection.MaxTextChars,
			MaxNeighbors:            cfg.SemanticMemory.MaxNeighbors,
			SpamSimilarityThreshold: cfg.SemanticMemory.SpamSimilarityThreshold,
			HamSimilarityThreshold:  cfg.SemanticMemory.HamSimilarityThreshold,
		}
		feedSpamService, err := detectionapp.NewManualFeedService(store, embeddings, store)
		if err != nil {
			return aiComponents{}, fmt.Errorf("建立 feedspam service：%w", err)
		}
		components.feedSpamService = feedSpamService
	}

	if !cfg.AIDetection.Enabled {
		return components, nil
	}
	classifier, err := buildAIClassifier(ctx, cfg.AIDetection)
	if err != nil {
		return aiComponents{}, err
	}
	processor, err := detectionapp.NewAIDetectionProcessor(detectionapp.AIDetectionProcessorPolicy{
		Enabled:       cfg.AIDetection.Enabled,
		Mode:          detectionapp.Mode(cfg.AIDetection.Mode),
		Provider:      string(cfg.AIDetection.Provider),
		Model:         aiDetectionModel(cfg.AIDetection),
		PromptVersion: infraai.PromptVersion,
		SchemaVersion: aiSchemaVersion,
		MaxTextRunes:  cfg.AIDetection.MaxTextChars,
		MinConfidence: cfg.AIDetection.MinConfidence,
		CacheTTL:      cfg.AIDetection.CacheTTL,
	}, detectionapp.AITriggerPolicy{OnlyWhenAmbiguous: cfg.AIDetection.OnlyWhenAmbiguous}, store, classifier, semantic)
	if err != nil {
		return aiComponents{}, fmt.Errorf("建立 AI 偵測 processor：%w", err)
	}
	components.processor = processor
	return components, nil
}

func buildAIClassifier(ctx context.Context, cfg appconfig.AIDetectionConfig) (detectionapp.AIClassifier, error) {
	switch cfg.Provider {
	case appconfig.AIProviderOpenAICompatible:
		classifier, err := infraai.NewOpenAICompatibleClassifier(cfg.OpenAICompatible.Endpoint, cfg.OpenAICompatible.Model, cfg.OpenAICompatible.APIKey, cfg.Timeout, nil)
		if err != nil {
			return nil, fmt.Errorf("建立 OpenAI-compatible classifier：%w", err)
		}
		return classifier, nil
	case appconfig.AIProviderBedrock:
		classifier, err := infraai.NewBedrockClassifier(ctx, cfg.Bedrock, cfg.Timeout)
		if err != nil {
			return nil, fmt.Errorf("建立 Bedrock classifier：%w", err)
		}
		return classifier, nil
	default:
		return nil, fmt.Errorf("不支援的 AI provider：%s", cfg.Provider)
	}
}

func buildEmbeddingProvider(ctx context.Context, cfg appconfig.SemanticMemoryConfig) (detectionapp.EmbeddingProvider, error) {
	switch cfg.EmbeddingProvider {
	case appconfig.AIProviderOpenAICompatible:
		provider, err := infraai.NewOpenAICompatibleEmbedding(cfg.OpenAICompatible.Endpoint, cfg.OpenAICompatible.Model, cfg.OpenAICompatible.APIKey, cfg.EmbeddingVersion, cfg.EmbeddingDimensions, 0, nil)
		if err != nil {
			return nil, fmt.Errorf("建立 OpenAI-compatible embedding provider：%w", err)
		}
		return provider, nil
	case appconfig.AIProviderBedrock:
		provider, err := infraai.NewBedrockEmbedding(ctx, cfg.Bedrock, cfg.EmbeddingVersion, cfg.EmbeddingDimensions, 0)
		if err != nil {
			return nil, fmt.Errorf("建立 Bedrock embedding provider：%w", err)
		}
		return provider, nil
	default:
		return nil, fmt.Errorf("不支援的 embedding provider：%s", cfg.EmbeddingProvider)
	}
}

func aiDetectionModel(cfg appconfig.AIDetectionConfig) string {
	switch cfg.Provider {
	case appconfig.AIProviderOpenAICompatible:
		return cfg.OpenAICompatible.Model
	case appconfig.AIProviderBedrock:
		return cfg.Bedrock.ModelID
	default:
		return ""
	}
}

func validateAIComponents(cfg appconfig.Config, components aiComponents) error {
	if cfg.AIDetection.Enabled && components.processor == nil {
		return errors.New("AI 偵測已啟用但 processor 未建立")
	}
	if cfg.SemanticMemory.Enabled && components.feedSpamService == nil {
		return errors.New("語意記憶已啟用但 feedspam service 未建立")
	}
	return nil
}
