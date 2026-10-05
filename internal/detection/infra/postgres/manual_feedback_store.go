package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/vincent119/tg_spam_bot/internal/detection/application"
	"github.com/vincent119/tg_spam_bot/internal/detection/domain"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type manualFeedbackCurrent struct {
	ID                 uint64    `gorm:"primaryKey"`
	ChatID             int64     `gorm:"uniqueIndex:idx_manual_feedback_message;index:idx_manual_feedback_fingerprint,priority:1;not null"`
	MessageID          int64     `gorm:"uniqueIndex:idx_manual_feedback_message;not null"`
	TargetUserID       int64     `gorm:"not null"`
	OperatorID         int64     `gorm:"not null"`
	CommandUpdateID    int64     `gorm:"not null"`
	ContentFingerprint string    `gorm:"index:idx_manual_feedback_fingerprint,priority:2;not null"`
	Label              string    `gorm:"size:32;not null"`
	Category           string    `gorm:"size:100;not null"`
	Reason             string    `gorm:"size:200"`
	Source             string    `gorm:"size:32;not null"`
	Revision           uint64    `gorm:"not null"`
	EmbeddingStatus    string    `gorm:"size:32;not null"`
	EmbeddingErrorCode string    `gorm:"size:64"`
	CreatedAt          time.Time `gorm:"not null"`
	UpdatedAt          time.Time `gorm:"not null"`
}

type manualFeedbackRevision struct {
	ID                 uint64    `gorm:"primaryKey"`
	ChatID             int64     `gorm:"uniqueIndex:idx_manual_feedback_command;uniqueIndex:idx_manual_feedback_revision,priority:1;not null"`
	CommandUpdateID    int64     `gorm:"uniqueIndex:idx_manual_feedback_command;not null"`
	MessageID          int64     `gorm:"uniqueIndex:idx_manual_feedback_revision,priority:2;not null"`
	Revision           uint64    `gorm:"uniqueIndex:idx_manual_feedback_revision,priority:3;not null"`
	TargetUserID       int64     `gorm:"not null"`
	OperatorID         int64     `gorm:"not null"`
	ContentFingerprint string    `gorm:"not null"`
	PreviousLabel      string    `gorm:"size:32"`
	Label              string    `gorm:"size:32;not null"`
	Category           string    `gorm:"size:100;not null"`
	Reason             string    `gorm:"size:200"`
	Source             string    `gorm:"size:32;not null"`
	CreatedAt          time.Time `gorm:"not null"`
}

type manualFeedbackEpochRow struct {
	ChatID  int64  `gorm:"primaryKey"`
	Version uint64 `gorm:"not null"`
}

type scopedManualFeedbackEmbedding struct {
	ID                 uint64    `gorm:"primaryKey"`
	ChatID             int64     `gorm:"uniqueIndex:idx_scoped_feedback_embedding;index:idx_scoped_feedback_chat_fingerprint,priority:1;not null"`
	MessageID          int64     `gorm:"uniqueIndex:idx_scoped_feedback_embedding;not null"`
	Revision           uint64    `gorm:"uniqueIndex:idx_scoped_feedback_embedding;not null"`
	ContentFingerprint string    `gorm:"index:idx_scoped_feedback_chat_fingerprint,priority:2;not null"`
	EmbeddingProvider  string    `gorm:"size:64;uniqueIndex:idx_scoped_feedback_embedding;not null"`
	EmbeddingModel     string    `gorm:"size:200;uniqueIndex:idx_scoped_feedback_embedding;not null"`
	EmbeddingVersion   string    `gorm:"size:64;uniqueIndex:idx_scoped_feedback_embedding;not null"`
	Dimensions         int       `gorm:"uniqueIndex:idx_scoped_feedback_embedding;not null"`
	Vector             pgVector  `gorm:"type:vector;not null"`
	CreatedAt          time.Time `gorm:"not null"`
	ExpiresAt          time.Time `gorm:"index;not null"`
}

// AutoMigrateManualFeedback 建立不依賴 pgvector 的人工標記與修訂結構。
func AutoMigrateManualFeedback(ctx context.Context, db *gorm.DB) error {
	if db == nil {
		return errors.New("gorm db is required")
	}
	return db.WithContext(ctx).AutoMigrate(&manualFeedbackCurrent{}, &manualFeedbackRevision{}, &manualFeedbackEpochRow{})
}

// AutoMigrateScopedManualEmbeddings 只在既有 pgvector 可用時建立群組向量表。
func AutoMigrateScopedManualEmbeddings(ctx context.Context, db *gorm.DB) error {
	if db == nil {
		return errors.New("gorm db is required")
	}
	if err := ensurePGVector(ctx, db); err != nil {
		return err
	}
	return db.WithContext(ctx).AutoMigrate(&scopedManualFeedbackEmbedding{})
}

// SaveFeedback 在同一交易保存有效標記、歷史修訂及群組快取版本。
func (s *Store) SaveFeedback(ctx context.Context, feedback application.ManualFeedback) (application.ManualFeedback, bool, error) {
	if feedback.ChatID == 0 || feedback.MessageID <= 0 || feedback.CommandUpdateID == 0 || feedback.ContentFingerprint == "" {
		return application.ManualFeedback{}, false, errors.New("manual feedback identifiers are required")
	}
	var saved application.ManualFeedback
	var changed bool
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var previousCommand manualFeedbackRevision
		commandQuery := tx.Where("chat_id=? AND command_update_id=?", feedback.ChatID, feedback.CommandUpdateID).Limit(1).Find(&previousCommand)
		if commandQuery.Error != nil {
			return commandQuery.Error
		}
		if commandQuery.RowsAffected > 0 {
			var existing manualFeedbackCurrent
			if err := tx.Where("chat_id=? AND message_id=?", feedback.ChatID, previousCommand.MessageID).Take(&existing).Error; err != nil {
				return err
			}
			saved = feedbackFromRow(existing)
			return nil
		}
		seed := manualFeedbackCurrent{
			ChatID: feedback.ChatID, MessageID: feedback.MessageID, TargetUserID: feedback.TargetUserID,
			OperatorID: feedback.OperatorID, CommandUpdateID: feedback.CommandUpdateID,
			ContentFingerprint: feedback.ContentFingerprint, Label: string(feedback.Label),
			Category: truncateRunes(feedback.Category, 100), Reason: truncateRunes(feedback.Reason, 200),
			Source: truncateRunes(feedback.Source, 32), EmbeddingStatus: feedback.EmbeddingStatus,
			CreatedAt: feedback.CreatedAt.UTC(), UpdatedAt: feedback.CreatedAt.UTC(),
		}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&seed).Error; err != nil {
			return err
		}
		var current manualFeedbackCurrent
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("chat_id=? AND message_id=?", feedback.ChatID, feedback.MessageID).Take(&current).Error; err != nil {
			return err
		}
		var existing manualFeedbackRevision
		commandQuery = tx.Where("chat_id=? AND command_update_id=?", feedback.ChatID, feedback.CommandUpdateID).Limit(1).Find(&existing)
		if commandQuery.Error != nil {
			return commandQuery.Error
		}
		if commandQuery.RowsAffected > 0 {
			saved = feedbackFromRow(current)
			return nil
		}
		previous := current.Label
		if current.Revision == 0 {
			previous = ""
		}
		revision := current.Revision + 1
		history := manualFeedbackRevision{
			ChatID: feedback.ChatID, CommandUpdateID: feedback.CommandUpdateID, MessageID: feedback.MessageID,
			Revision: revision, TargetUserID: feedback.TargetUserID, OperatorID: feedback.OperatorID,
			ContentFingerprint: feedback.ContentFingerprint, PreviousLabel: previous, Label: string(feedback.Label),
			Category: truncateRunes(feedback.Category, 100), Reason: truncateRunes(feedback.Reason, 200),
			Source: truncateRunes(feedback.Source, 32), CreatedAt: feedback.CreatedAt.UTC(),
		}
		if err := tx.Create(&history).Error; err != nil {
			return err
		}
		updates := map[string]any{
			"target_user_id": feedback.TargetUserID, "operator_id": feedback.OperatorID,
			"command_update_id": feedback.CommandUpdateID, "content_fingerprint": feedback.ContentFingerprint,
			"label": string(feedback.Label), "category": history.Category, "reason": history.Reason,
			"source": history.Source, "revision": revision, "embedding_status": feedback.EmbeddingStatus,
			"embedding_error_code": "", "updated_at": feedback.CreatedAt.UTC(),
		}
		if err := tx.Model(&manualFeedbackCurrent{}).Where("id=?", current.ID).Updates(updates).Error; err != nil {
			return err
		}
		if err := tx.Exec(`INSERT INTO manual_feedback_epoch_rows (chat_id, version) VALUES (?, 1)
			ON CONFLICT (chat_id) DO UPDATE SET version = manual_feedback_epoch_rows.version + 1`, feedback.ChatID).Error; err != nil {
			return err
		}
		current.TargetUserID = feedback.TargetUserID
		current.OperatorID = feedback.OperatorID
		current.CommandUpdateID = feedback.CommandUpdateID
		current.ContentFingerprint = feedback.ContentFingerprint
		current.Label = string(feedback.Label)
		current.Category = history.Category
		current.Reason = history.Reason
		current.Source = history.Source
		current.Revision = revision
		current.EmbeddingStatus = feedback.EmbeddingStatus
		current.EmbeddingErrorCode = ""
		current.UpdatedAt = feedback.CreatedAt.UTC()
		saved, changed = feedbackFromRow(current), true
		return nil
	})
	return saved, changed, err
}

// MarkFeedbackEmbeddingFailed 只改寫仍有效的 revision，避免舊工作污染新標記。
func (s *Store) MarkFeedbackEmbeddingFailed(ctx context.Context, chatID, messageID int64, revision uint64, code string) error {
	result := s.db.WithContext(ctx).Model(&manualFeedbackCurrent{}).
		Where("chat_id=? AND message_id=? AND revision=?", chatID, messageID, revision).
		Updates(map[string]any{"embedding_status": application.ManualFeedbackEmbeddingFailed, "embedding_error_code": truncateRunes(code, 64)})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return application.ErrStaleManualFeedback
	}
	return nil
}

// SaveScopedFeedbackEmbedding 在標記仍有效時寫入群組向量並完成狀態。
func (s *Store) SaveScopedFeedbackEmbedding(ctx context.Context, record application.ScopedFeedbackEmbedding) error {
	if err := record.Embedding.Validate(); err != nil {
		return err
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current manualFeedbackCurrent
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("chat_id=? AND message_id=?", record.Feedback.ChatID, record.Feedback.MessageID).Take(&current).Error; err != nil {
			return err
		}
		if current.Revision != record.Feedback.Revision || current.ContentFingerprint != record.Feedback.ContentFingerprint || current.Label != string(record.Feedback.Label) {
			return application.ErrStaleManualFeedback
		}
		row := scopedManualFeedbackEmbedding{
			ChatID: current.ChatID, MessageID: current.MessageID, Revision: current.Revision,
			ContentFingerprint: current.ContentFingerprint,
			EmbeddingProvider:  truncateRunes(record.Embedding.Provider, 64),
			EmbeddingModel:     truncateRunes(record.Embedding.Model, 200),
			EmbeddingVersion:   truncateRunes(record.Embedding.Version, 64),
			Dimensions:         record.Embedding.Dimensions, Vector: pgVector(record.Embedding.VectorCopy()),
			CreatedAt: record.CreatedAt.UTC(), ExpiresAt: record.ExpiresAt.UTC(),
		}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
			return err
		}
		return tx.Model(&manualFeedbackCurrent{}).Where("id=? AND revision=?", current.ID, current.Revision).
			Updates(map[string]any{"embedding_status": application.ManualFeedbackEmbeddingCompleted, "embedding_error_code": ""}).Error
	})
}

// FindEffectiveFeedback 僅讀本群目前標記；同指紋若有相反標籤則標記衝突。
func (s *Store) FindEffectiveFeedback(ctx context.Context, chatID int64, fingerprint string) (application.ManualFeedbackEvidence, error) {
	var rows []manualFeedbackCurrent
	if err := s.db.WithContext(ctx).Where("chat_id=? AND content_fingerprint=?", chatID, fingerprint).Find(&rows).Error; err != nil {
		return application.ManualFeedbackEvidence{}, err
	}
	if len(rows) == 0 {
		return application.ManualFeedbackEvidence{}, nil
	}
	evidence := application.ManualFeedbackEvidence{Present: true, Label: domain.AILabel(rows[0].Label), Category: rows[0].Category}
	for _, row := range rows[1:] {
		if row.Label != string(evidence.Label) {
			return application.ManualFeedbackEvidence{Present: true, Conflict: true}, nil
		}
		if row.Category != evidence.Category {
			evidence.Category = ""
		}
	}
	return evidence, nil
}

// SearchScopedManualSimilar 僅查目前有效且同群不衝突的向量版本。
func (s *Store) SearchScopedManualSimilar(ctx context.Context, chatID int64, embedding domain.EmbeddingResult, maxNeighbors int) ([]domain.SemanticMatch, error) {
	if err := embedding.Validate(); err != nil {
		return nil, err
	}
	if maxNeighbors <= 0 {
		maxNeighbors = 5
	}
	value, err := pgVector(embedding.VectorCopy()).Value()
	if err != nil {
		return nil, err
	}
	var rows []struct {
		Fingerprint string
		Label       string
		Category    string
		Similarity  float64
	}
	err = s.db.WithContext(ctx).Raw(`
		SELECT e.content_fingerprint AS fingerprint, f.label, f.category,
		       1 - (e.vector <=> ?::vector) AS similarity
		FROM scoped_manual_feedback_embeddings e
		JOIN manual_feedback_currents f
		  ON f.chat_id=e.chat_id AND f.message_id=e.message_id AND f.revision=e.revision
		WHERE e.chat_id=? AND e.embedding_provider=? AND e.embedding_model=?
		  AND e.embedding_version=? AND e.dimensions=? AND e.expires_at>?
		  AND f.embedding_status=?
		  AND NOT EXISTS (
		    SELECT 1 FROM manual_feedback_currents conflict
		    WHERE conflict.chat_id=e.chat_id AND conflict.content_fingerprint=e.content_fingerprint
		      AND conflict.label<>f.label
		  )
		ORDER BY e.vector <=> ?::vector
		LIMIT ?`, value, chatID, embedding.Provider, embedding.Model, embedding.Version,
		embedding.Dimensions, time.Now().UTC(), application.ManualFeedbackEmbeddingCompleted, value, maxNeighbors).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	matches := make([]domain.SemanticMatch, 0, len(rows))
	for _, row := range rows {
		matches = append(matches, domain.SemanticMatch{
			SourceEventID: row.Fingerprint, Label: domain.AILabel(row.Label), Category: row.Category,
			ReasonCode: "manual_feedback", Similarity: row.Similarity,
		})
	}
	return matches, nil
}

// ManualFeedbackEpoch 回傳本群標記版本，未曾提交時為零。
func (s *Store) ManualFeedbackEpoch(ctx context.Context, chatID int64) (uint64, error) {
	var row manualFeedbackEpochRow
	query := s.db.WithContext(ctx).Where("chat_id=?", chatID).Limit(1).Find(&row)
	if query.Error != nil {
		return 0, fmt.Errorf("find manual feedback epoch: %w", query.Error)
	}
	if query.RowsAffected == 0 {
		return 0, nil
	}
	return row.Version, nil
}

func feedbackFromRow(row manualFeedbackCurrent) application.ManualFeedback {
	return application.ManualFeedback{
		ID: row.ID, ChatID: row.ChatID, MessageID: row.MessageID, TargetUserID: row.TargetUserID,
		OperatorID: row.OperatorID, CommandUpdateID: row.CommandUpdateID, ContentFingerprint: strings.Clone(row.ContentFingerprint),
		Label: domain.AILabel(row.Label), Category: row.Category, Reason: row.Reason, Source: row.Source,
		Revision: row.Revision, EmbeddingStatus: row.EmbeddingStatus, EmbeddingErrorCode: row.EmbeddingErrorCode,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
}
