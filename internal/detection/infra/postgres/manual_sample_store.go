package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/vincent119/tg_spam_bot/internal/detection/application"
	"github.com/vincent119/tg_spam_bot/internal/detection/domain"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type semanticManualSample struct {
	ID                 uint64     `gorm:"primaryKey;comment:人工語意樣本流水號"`
	ChatID             int64      `gorm:"uniqueIndex:idx_manual_sample_message;index;not null;comment:Telegram 聊天識別碼"`
	MessageID          int64      `gorm:"uniqueIndex:idx_manual_sample_message;not null;comment:Telegram 目標訊息識別碼"`
	TargetUserID       int64      `gorm:"index;not null;comment:目標成員識別碼"`
	OperatorID         int64      `gorm:"index;not null;comment:提交樣本的管理員識別碼"`
	ContentFingerprint string     `gorm:"index;not null;comment:有金鑰的內容指紋"`
	Label              string     `gorm:"size:32;index;not null;comment:人工標籤"`
	Category           string     `gorm:"size:100;index;not null;comment:人工分類"`
	Source             string     `gorm:"size:32;not null;comment:樣本來源"`
	Status             string     `gorm:"size:32;index;not null;comment:向量化狀態"`
	ErrorCode          string     `gorm:"size:64;comment:穩定錯誤類型"`
	ErrorText          string     `gorm:"size:500;comment:遮罩後錯誤摘要"`
	Retryable          bool       `gorm:"comment:失敗是否可重試"`
	CreatedAt          time.Time  `gorm:"index;not null;comment:建立 UTC 時間"`
	EmbeddedAt         *time.Time `gorm:"comment:向量化完成 UTC 時間"`
}

// CreateManualSample 冪等保存管理員提交的漏網樣本摘要，不保存完整原文。
func (s *Store) CreateManualSample(ctx context.Context, sample application.ManualSample) (application.ManualSample, bool, error) {
	row := semanticManualSample{
		ChatID: sample.ChatID, MessageID: sample.MessageID, TargetUserID: sample.TargetUserID, OperatorID: sample.OperatorID,
		ContentFingerprint: sample.ContentFingerprint, Label: string(sample.Label), Category: truncateRunes(sample.Category, 100),
		Source: truncateRunes(sample.Source, 32), Status: application.ManualSampleStatusPendingEmbedding, CreatedAt: sample.CreatedAt,
	}
	result := s.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&row)
	if result.Error != nil {
		return application.ManualSample{}, false, result.Error
	}
	if result.RowsAffected == 1 {
		return manualSample(row), true, nil
	}
	var existing semanticManualSample
	if err := s.db.WithContext(ctx).Where("chat_id=? AND message_id=?", sample.ChatID, sample.MessageID).Take(&existing).Error; err != nil {
		return application.ManualSample{}, false, err
	}
	return manualSample(existing), false, nil
}

// PendingManualSamples 查詢等待向量化或可重試失敗的樣本。
func (s *Store) PendingManualSamples(ctx context.Context, limit int) ([]application.ManualSample, error) {
	if limit <= 0 {
		limit = 50
	}
	var rows []semanticManualSample
	err := s.db.WithContext(ctx).
		Where("status=? OR (status=? AND retryable)", application.ManualSampleStatusPendingEmbedding, application.ManualSampleStatusEmbeddingFailed).
		Order("created_at ASC").Limit(limit).Find(&rows).Error
	if err != nil {
		return nil, err
	}
	samples := make([]application.ManualSample, 0, len(rows))
	for _, row := range rows {
		samples = append(samples, manualSample(row))
	}
	return samples, nil
}

// MarkManualSampleEmbeddingCompleted 將樣本標示為已完成向量化。
func (s *Store) MarkManualSampleEmbeddingCompleted(ctx context.Context, id uint64, embeddedAt time.Time) error {
	result := s.db.WithContext(ctx).Model(&semanticManualSample{}).Where("id=?", id).Updates(map[string]any{
		"status": application.ManualSampleStatusEmbeddingCompleted, "embedded_at": embeddedAt.UTC(),
		"error_code": "", "error_text": "", "retryable": false,
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// MarkManualSampleEmbeddingFailed 保存向量化失敗摘要。
func (s *Store) MarkManualSampleEmbeddingFailed(ctx context.Context, id uint64, errorCode, errorText string, retryable bool) error {
	if id == 0 {
		return errors.New("manual sample id is required")
	}
	result := s.db.WithContext(ctx).Model(&semanticManualSample{}).Where("id=?", id).Updates(map[string]any{
		"status":     application.ManualSampleStatusEmbeddingFailed,
		"error_code": truncateRunes(errorCode, 64), "error_text": truncateRunes(errorText, 500), "retryable": retryable,
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func manualSample(row semanticManualSample) application.ManualSample {
	return application.ManualSample{
		ID: row.ID, ChatID: row.ChatID, MessageID: row.MessageID, TargetUserID: row.TargetUserID, OperatorID: row.OperatorID,
		ContentFingerprint: row.ContentFingerprint, Label: domain.AILabel(row.Label), Category: row.Category, Source: row.Source,
		Status: row.Status, CreatedAt: row.CreatedAt, EmbeddedAt: row.EmbeddedAt,
		ErrorCode: row.ErrorCode, ErrorText: row.ErrorText, Retryable: row.Retryable,
	}
}
