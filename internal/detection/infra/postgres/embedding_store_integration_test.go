package postgres

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/vincent119/tg_spam_bot/internal/detection/application"
	"github.com/vincent119/tg_spam_bot/internal/detection/domain"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestSemanticMemoryIntegration(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("未設定 TEST_DATABASE_URL")
	}
	ctx := t.Context()
	db, err := gorm.Open(postgres.Open(databaseURL), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := AutoMigrateSemanticMemory(ctx, db); err != nil {
		t.Skipf("pgvector extension 未啟用或不可用：%v", err)
	}
	store, err := NewStore(db)
	if err != nil {
		t.Fatal(err)
	}
	seed := time.Now().UnixNano()
	prefix := fmt.Sprintf("embedding-it-%d", seed)
	t.Cleanup(func() {
		_ = db.WithContext(context.Background()).Where("content_fingerprint LIKE ?", prefix+"%").Delete(&messageEmbedding{}).Error
		_ = db.WithContext(context.Background()).Where("category_id LIKE ?", prefix+"%").Delete(&semanticBlacklistExample{}).Error
		_ = db.WithContext(context.Background()).Where("id LIKE ?", prefix+"%").Delete(&semanticBlacklistCategory{}).Error
	})

	now := time.Now().UTC()
	base := application.EmbeddingRecord{
		ContentFingerprint: prefix + "-spam",
		Embedding: domain.EmbeddingResult{
			Provider: "openai_compatible", Model: "embedding", Version: "v1",
			Dimensions: 3, Vector: []float32{0.1, 0.2, 0.3},
		},
		Label: domain.AILabelSpam, Category: "ad", ReasonCode: "similar_pitch",
		CreatedAt: now, ExpiresAt: now.Add(time.Hour),
	}
	if err := store.SaveEmbedding(ctx, base); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveEmbedding(ctx, base); err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := db.Model(&messageEmbedding{}).Where("content_fingerprint=?", base.ContentFingerprint).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("embedding count=%d err=%v", count, err)
	}
	found, ok, err := store.FindEmbeddingByFingerprint(ctx, base.ContentFingerprint, "openai_compatible", "embedding", "v1", 3, now)
	if err != nil || !ok || found.Label != domain.AILabelSpam || found.Embedding.Dimensions != 3 {
		t.Fatalf("FindEmbeddingByFingerprint()=%+v ok=%v err=%v", found, ok, err)
	}
	if _, ok, err := store.FindEmbeddingByFingerprint(ctx, base.ContentFingerprint, "bedrock", "embedding", "v1", 3, now); err != nil || ok {
		t.Fatalf("不同 provider 不應命中：ok=%v err=%v", ok, err)
	}
	if err := store.SaveEmbedding(ctx, application.EmbeddingRecord{
		ContentFingerprint: prefix + "-ham",
		Embedding: domain.EmbeddingResult{
			Provider: "openai_compatible", Model: "embedding", Version: "v1",
			Dimensions: 3, Vector: []float32{0.9, 0.9, 0.9},
		},
		Label: domain.AILabelHam, Category: "normal", ReasonCode: "known_ham",
		CreatedAt: now, ExpiresAt: now.Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveEmbedding(ctx, application.EmbeddingRecord{
		ContentFingerprint: prefix + "-other-model",
		Embedding: domain.EmbeddingResult{
			Provider: "openai_compatible", Model: "other-embedding", Version: "v1",
			Dimensions: 3, Vector: []float32{0.1, 0.2, 0.3},
		},
		Label: domain.AILabelSpam, Category: "ad", ReasonCode: "wrong_model",
		CreatedAt: now, ExpiresAt: now.Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	matches, err := store.SearchSimilar(ctx, domain.EmbeddingResult{
		Provider: "openai_compatible", Model: "embedding", Version: "v1",
		Dimensions: 3, Vector: []float32{0.1, 0.2, 0.31},
	}, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) == 0 {
		t.Fatal("SearchSimilar() 未回傳相似結果")
	}
	for _, match := range matches {
		if match.ReasonCode == "wrong_model" {
			t.Fatalf("SearchSimilar() 混入不同模型：%+v", matches)
		}
	}

	enabledCategory := application.SemanticBlacklistCategory{ID: prefix + "-enabled", Name: "代理招募", Enabled: true, CreatedAt: now}
	if err := store.SaveSemanticBlacklistCategory(ctx, enabledCategory); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveSemanticBlacklistExample(ctx, application.SemanticBlacklistExample{
		Category: enabledCategory,
		Embedding: domain.EmbeddingResult{
			Provider: "openai_compatible", Model: "embedding", Version: "v1",
			Dimensions: 3, Vector: []float32{0.1, 0.2, 0.3},
		},
		Source: "manual", CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	disabledCategory := application.SemanticBlacklistCategory{ID: prefix + "-disabled", Name: "停用分類", Enabled: false, CreatedAt: now}
	if err := store.SaveSemanticBlacklistCategory(ctx, disabledCategory); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveSemanticBlacklistExample(ctx, application.SemanticBlacklistExample{
		Category: disabledCategory,
		Embedding: domain.EmbeddingResult{
			Provider: "openai_compatible", Model: "embedding", Version: "v1",
			Dimensions: 3, Vector: []float32{0.1, 0.2, 0.3},
		},
		Source: "manual", CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	blacklistMatches, err := store.SearchSemanticBlacklist(ctx, domain.EmbeddingResult{
		Provider: "openai_compatible", Model: "embedding", Version: "v1",
		Dimensions: 3, Vector: []float32{0.1, 0.2, 0.31},
	}, 0.80, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(blacklistMatches) == 0 {
		t.Fatal("SearchSemanticBlacklist() 未回傳啟用分類命中")
	}
	for _, match := range blacklistMatches {
		if match.CategoryID == disabledCategory.ID {
			t.Fatalf("停用分類不應命中：%+v", blacklistMatches)
		}
	}
	blacklistMatches, err = store.SearchSemanticBlacklist(ctx, domain.EmbeddingResult{
		Provider: "openai_compatible", Model: "other-embedding", Version: "v1",
		Dimensions: 3, Vector: []float32{0.1, 0.2, 0.31},
	}, 0.80, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(blacklistMatches) != 0 {
		t.Fatalf("不同模型不應命中黑名單：%+v", blacklistMatches)
	}
}
