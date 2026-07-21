package postgres

import (
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/vincent119/tg_spam_bot/internal/detection/application"
	"github.com/vincent119/tg_spam_bot/internal/detection/domain"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type pgVector []float32

func (v pgVector) Value() (driver.Value, error) {
	if len(v) == 0 {
		return nil, errors.New("pgvector must not be empty")
	}
	parts := make([]string, 0, len(v))
	for _, value := range v {
		parts = append(parts, strconv.FormatFloat(float64(value), 'f', -1, 32))
	}
	return "[" + strings.Join(parts, ",") + "]", nil
}

func (v *pgVector) Scan(value any) error {
	switch raw := value.(type) {
	case []byte:
		return v.scanString(string(raw))
	case string:
		return v.scanString(raw)
	default:
		return fmt.Errorf("unsupported pgvector scan type %T", value)
	}
}

func (v *pgVector) scanString(value string) error {
	value = strings.Trim(value, "[] ")
	if value == "" {
		*v = nil
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]float32, 0, len(parts))
	for _, part := range parts {
		parsed, err := strconv.ParseFloat(strings.TrimSpace(part), 32)
		if err != nil {
			return fmt.Errorf("parse pgvector value: %w", err)
		}
		out = append(out, float32(parsed))
	}
	*v = out
	return nil
}

type messageEmbedding struct {
	ID                 uint64    `gorm:"primaryKey;comment:訊息向量流水號"`
	ContentFingerprint string    `gorm:"uniqueIndex:idx_message_embedding_unique;index;not null;comment:有金鑰的內容指紋"`
	EmbeddingProvider  string    `gorm:"size:64;uniqueIndex:idx_message_embedding_unique;index:idx_message_embedding_scope,priority:1;not null;comment:Embedding provider 名稱"`
	EmbeddingModel     string    `gorm:"size:200;uniqueIndex:idx_message_embedding_unique;index:idx_message_embedding_scope,priority:2;not null;comment:Embedding 模型名稱"`
	EmbeddingVersion   string    `gorm:"size:64;uniqueIndex:idx_message_embedding_unique;index:idx_message_embedding_scope,priority:3;not null;comment:Embedding 版本"`
	Dimensions         int       `gorm:"uniqueIndex:idx_message_embedding_unique;index:idx_message_embedding_scope,priority:4;not null;comment:向量維度"`
	Vector             pgVector  `gorm:"type:vector;not null;comment:訊息 embedding 向量"`
	Label              string    `gorm:"size:32;index;not null;comment:語意標籤"`
	Category           string    `gorm:"size:100;index;comment:語意分類"`
	ReasonCode         string    `gorm:"size:100;comment:語意原因代碼"`
	CreatedAt          time.Time `gorm:"index;not null;comment:建立 UTC 時間"`
	ExpiresAt          time.Time `gorm:"index;not null;comment:過期 UTC 時間"`
}

// AutoMigrateSemanticMemory 檢查 pgvector extension 並建立語意記憶表。
func AutoMigrateSemanticMemory(ctx context.Context, db *gorm.DB) error {
	if db == nil {
		return errors.New("gorm db is required")
	}
	if err := ensurePGVector(ctx, db); err != nil {
		return err
	}
	if err := db.WithContext(ctx).AutoMigrate(&messageEmbedding{}, &semanticBlacklistCategory{}, &semanticBlacklistExample{}); err != nil {
		return fmt.Errorf("auto migrate semantic memory schema: %w", err)
	}
	comments := map[string]string{
		"message_embeddings":            "訊息 embedding 與語意垃圾記憶紀錄",
		"semantic_blacklist_categories": "語意黑名單分類",
		"semantic_blacklist_examples":   "語意黑名單範例向量",
	}
	for table, comment := range comments {
		query := fmt.Sprintf("COMMENT ON TABLE %s IS '%s'", table, strings.ReplaceAll(comment, "'", "''"))
		if err := db.WithContext(ctx).Exec(query).Error; err != nil {
			return fmt.Errorf("comment table %s: %w", table, err)
		}
	}
	return nil
}

func ensurePGVector(ctx context.Context, db *gorm.DB) error {
	var exists bool
	err := db.WithContext(ctx).Raw("SELECT EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'vector')").Scan(&exists).Error
	if err != nil {
		return fmt.Errorf("check pgvector extension: %w", err)
	}
	if !exists {
		return errors.New("pgvector extension is required")
	}
	return nil
}

// SaveEmbedding 冪等保存訊息 embedding，不保存完整原文。
func (s *Store) SaveEmbedding(ctx context.Context, record application.EmbeddingRecord) error {
	if err := record.Embedding.Validate(); err != nil {
		return err
	}
	row := messageEmbedding{
		ContentFingerprint: record.ContentFingerprint,
		EmbeddingProvider:  truncateRunes(record.Embedding.Provider, 64),
		EmbeddingModel:     truncateRunes(record.Embedding.Model, 200),
		EmbeddingVersion:   truncateRunes(record.Embedding.Version, 64),
		Dimensions:         record.Embedding.Dimensions,
		Vector:             pgVector(record.Embedding.VectorCopy()),
		Label:              string(record.Label),
		Category:           truncateRunes(record.Category, 100),
		ReasonCode:         truncateRunes(record.ReasonCode, 100),
		CreatedAt:          record.CreatedAt,
		ExpiresAt:          record.ExpiresAt,
	}
	return s.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error
}

// FindEmbeddingByFingerprint 依 fingerprint 與模型範圍查詢未過期 embedding。
func (s *Store) FindEmbeddingByFingerprint(ctx context.Context, fingerprint, provider, model, version string, dimensions int, now time.Time) (application.EmbeddingRecord, bool, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	var row messageEmbedding
	err := s.db.WithContext(ctx).Where(
		"content_fingerprint=? AND embedding_provider=? AND embedding_model=? AND embedding_version=? AND dimensions=? AND expires_at>?",
		fingerprint, provider, model, version, dimensions, now.UTC(),
	).Take(&row).Error
	if err == nil {
		return embeddingRecord(row), true, nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return application.EmbeddingRecord{}, false, nil
	}
	return application.EmbeddingRecord{}, false, err
}

// SearchSimilar 依相同 provider/model/version/dimensions 查詢未過期相似樣本。
func (s *Store) SearchSimilar(ctx context.Context, embedding domain.EmbeddingResult, maxNeighbors int) ([]domain.SemanticMatch, error) {
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
		SourceEventID string
		Label         string
		Category      string
		ReasonCode    string
		Similarity    float64
	}
	err = s.db.WithContext(ctx).Raw(`
		SELECT content_fingerprint AS source_event_id, label, category, reason_code,
		       1 - (vector <=> ?::vector) AS similarity
		FROM message_embeddings
		WHERE embedding_provider = ?
		  AND embedding_model = ?
		  AND embedding_version = ?
		  AND dimensions = ?
		  AND expires_at > ?
		ORDER BY vector <=> ?::vector
		LIMIT ?
	`, vectorValue, embedding.Provider, embedding.Model, embedding.Version, embedding.Dimensions, time.Now().UTC(), vectorValue, maxNeighbors).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	matches := make([]domain.SemanticMatch, 0, len(rows))
	for _, row := range rows {
		matches = append(matches, domain.SemanticMatch{
			SourceEventID: row.SourceEventID,
			Label:         domain.AILabel(row.Label),
			Category:      row.Category,
			ReasonCode:    row.ReasonCode,
			Similarity:    row.Similarity,
		})
	}
	return matches, nil
}

func embeddingRecord(row messageEmbedding) application.EmbeddingRecord {
	return application.EmbeddingRecord{
		ContentFingerprint: row.ContentFingerprint,
		Embedding: domain.EmbeddingResult{
			Provider: row.EmbeddingProvider, Model: row.EmbeddingModel, Version: row.EmbeddingVersion,
			Dimensions: row.Dimensions, Vector: append([]float32(nil), row.Vector...),
		},
		Label: domain.AILabel(row.Label), Category: row.Category, ReasonCode: row.ReasonCode,
		CreatedAt: row.CreatedAt, ExpiresAt: row.ExpiresAt,
	}
}
