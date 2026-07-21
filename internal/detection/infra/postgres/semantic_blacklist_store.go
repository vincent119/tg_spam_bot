package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/vincent119/tg_spam_bot/internal/detection/application"
	"github.com/vincent119/tg_spam_bot/internal/detection/domain"
	"gorm.io/gorm/clause"
)

type semanticBlacklistCategory struct {
	ID          string    `gorm:"primaryKey;size:100;comment:語意黑名單分類識別碼"`
	Name        string    `gorm:"size:100;not null;comment:分類名稱"`
	Description string    `gorm:"size:500;comment:分類說明"`
	Enabled     bool      `gorm:"index;not null;comment:是否啟用"`
	CreatedAt   time.Time `gorm:"not null;comment:建立 UTC 時間"`
}

type semanticBlacklistExample struct {
	ID                uint64    `gorm:"primaryKey;comment:語意黑名單範例流水號"`
	CategoryID        string    `gorm:"size:100;index;not null;comment:語意黑名單分類識別碼"`
	EmbeddingProvider string    `gorm:"size:64;index:idx_blacklist_example_scope,priority:1;not null;comment:Embedding provider 名稱"`
	EmbeddingModel    string    `gorm:"size:200;index:idx_blacklist_example_scope,priority:2;not null;comment:Embedding 模型名稱"`
	EmbeddingVersion  string    `gorm:"size:64;index:idx_blacklist_example_scope,priority:3;not null;comment:Embedding 版本"`
	Dimensions        int       `gorm:"index:idx_blacklist_example_scope,priority:4;not null;comment:向量維度"`
	Vector            pgVector  `gorm:"type:vector;not null;comment:黑名單範例 embedding 向量"`
	Source            string    `gorm:"size:32;not null;comment:建立來源"`
	CreatedAt         time.Time `gorm:"not null;comment:建立 UTC 時間"`
}

// SaveSemanticBlacklistCategory 冪等保存語意黑名單分類。
func (s *Store) SaveSemanticBlacklistCategory(ctx context.Context, category application.SemanticBlacklistCategory) error {
	row := semanticBlacklistCategory{
		ID: truncateRunes(category.ID, 100), Name: truncateRunes(category.Name, 100),
		Description: truncateRunes(category.Description, 500), Enabled: category.Enabled, CreatedAt: category.CreatedAt,
	}
	return s.db.WithContext(ctx).Clauses(clause.OnConflict{UpdateAll: true}).Create(&row).Error
}

// SaveSemanticBlacklistExample 保存語意黑名單範例向量。
func (s *Store) SaveSemanticBlacklistExample(ctx context.Context, example application.SemanticBlacklistExample) error {
	if err := example.Embedding.Validate(); err != nil {
		return err
	}
	if example.Category.ID == "" {
		return errors.New("semantic blacklist category id is required")
	}
	row := semanticBlacklistExample{
		CategoryID:        truncateRunes(example.Category.ID, 100),
		EmbeddingProvider: truncateRunes(example.Embedding.Provider, 64),
		EmbeddingModel:    truncateRunes(example.Embedding.Model, 200),
		EmbeddingVersion:  truncateRunes(example.Embedding.Version, 64),
		Dimensions:        example.Embedding.Dimensions,
		Vector:            pgVector(example.Embedding.VectorCopy()),
		Source:            truncateRunes(example.Source, 32),
		CreatedAt:         example.CreatedAt,
	}
	return s.db.WithContext(ctx).Create(&row).Error
}

// SearchSemanticBlacklist 查詢啟用分類下的相似黑名單範例。
func (s *Store) SearchSemanticBlacklist(ctx context.Context, embedding domain.EmbeddingResult, threshold float64, maxNeighbors int) ([]application.SemanticBlacklistMatch, error) {
	if err := embedding.Validate(); err != nil {
		return nil, err
	}
	if maxNeighbors <= 0 {
		maxNeighbors = 5
	}
	vectorValue, err := pgVector(embedding.VectorCopy()).Value()
	if err != nil {
		return nil, err
	}
	var rows []struct {
		CategoryID   string
		CategoryName string
		Similarity   float64
	}
	err = s.db.WithContext(ctx).Raw(`
		SELECT c.id AS category_id, c.name AS category_name,
		       1 - (e.vector <=> ?::vector) AS similarity
		FROM semantic_blacklist_examples e
		JOIN semantic_blacklist_categories c ON c.id = e.category_id
		WHERE c.enabled
		  AND e.embedding_provider = ?
		  AND e.embedding_model = ?
		  AND e.embedding_version = ?
		  AND e.dimensions = ?
		  AND 1 - (e.vector <=> ?::vector) >= ?
		ORDER BY e.vector <=> ?::vector
		LIMIT ?
	`, vectorValue, embedding.Provider, embedding.Model, embedding.Version, embedding.Dimensions, vectorValue, threshold, vectorValue, maxNeighbors).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	matches := make([]application.SemanticBlacklistMatch, 0, len(rows))
	for _, row := range rows {
		matches = append(matches, application.SemanticBlacklistMatch{
			CategoryID: row.CategoryID, CategoryName: row.CategoryName,
			Similarity: row.Similarity, ReasonCode: "semantic_blacklist_match",
		})
	}
	return matches, nil
}
