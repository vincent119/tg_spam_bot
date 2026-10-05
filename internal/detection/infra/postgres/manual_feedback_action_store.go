package postgres

import (
	"context"
	"errors"
	"time"

	commandapp "github.com/vincent119/tg_spam_bot/internal/command/application"
	commanddomain "github.com/vincent119/tg_spam_bot/internal/command/domain"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type manualFeedbackAction struct {
	ID              uint64    `gorm:"primaryKey"`
	ChatID          int64     `gorm:"uniqueIndex:idx_manual_feedback_action;not null"`
	CommandUpdateID int64     `gorm:"uniqueIndex:idx_manual_feedback_action;not null"`
	Kind            string    `gorm:"size:32;uniqueIndex:idx_manual_feedback_action;not null"`
	MessageID       int64     `gorm:"not null"`
	TargetUserID    int64     `gorm:"not null"`
	Status          string    `gorm:"size:32;not null"`
	Retryable       bool      `gorm:"not null"`
	ErrorCode       string    `gorm:"size:64"`
	CreatedAt       time.Time `gorm:"not null"`
	EndedAt         *time.Time
}

// AutoMigrateManualFeedbackActions 建立人工回饋處置的逐項稽核表。
func AutoMigrateManualFeedbackActions(ctx context.Context, db *gorm.DB) error {
	if db == nil {
		return errors.New("gorm db is required")
	}
	return db.WithContext(ctx).AutoMigrate(&manualFeedbackAction{})
}

// FindFeedbackTarget 僅從同一群組的既存偵測事件確認目標。
func (s *Store) FindFeedbackTarget(ctx context.Context, chatID int64, eventID string) (commandapp.FeedbackTarget, bool, error) {
	var row detectionEvent
	query := s.db.WithContext(ctx).Where("chat_id=? AND event_id=?", chatID, eventID).Limit(1).Find(&row)
	if query.Error != nil {
		return commandapp.FeedbackTarget{}, false, query.Error
	}
	if query.RowsAffected == 0 {
		return commandapp.FeedbackTarget{}, false, nil
	}
	if row.MessageID <= 0 || row.UserID <= 0 || row.ContentFingerprint == "" {
		return commandapp.FeedbackTarget{}, false, nil
	}
	return commandapp.FeedbackTarget{MessageID: row.MessageID, UserID: row.UserID, ContentFingerprint: row.ContentFingerprint}, true, nil
}

// PlanFeedbackActions 在外部處置前持久化不可擴大的固定目標。
func (s *Store) PlanFeedbackActions(ctx context.Context, command commanddomain.Command, kinds []commandapp.FeedbackActionKind) error {
	if command.Target == nil || command.TargetMessage <= 0 || len(kinds) == 0 {
		return errors.New("manual feedback action target is required")
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var execution commandExecution
		if err := tx.Where("chat_id=? AND update_id=?", command.ChatID, command.UpdateID).Take(&execution).Error; err != nil {
			return err
		}
		for _, kind := range kinds {
			if kind != commandapp.FeedbackActionDelete && kind != commandapp.FeedbackActionBan {
				return errors.New("unsupported manual feedback action")
			}
			row := manualFeedbackAction{
				ChatID: command.ChatID, CommandUpdateID: command.UpdateID, Kind: string(kind),
				MessageID: command.TargetMessage, TargetUserID: command.Target.ID, Status: "pending", CreatedAt: time.Now().UTC(),
			}
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// CompleteFeedbackAction 單獨保存 Telegram 動作結果，未回寫成功時仍保留 pending 供稽核。
func (s *Store) CompleteFeedbackAction(ctx context.Context, command commanddomain.Command, kind commandapp.FeedbackActionKind, succeeded, retryable bool, errorCode string) error {
	status := "failed"
	if succeeded {
		status = "completed"
	}
	result := s.db.WithContext(ctx).Model(&manualFeedbackAction{}).
		Where("chat_id=? AND command_update_id=? AND kind=? AND status=?", command.ChatID, command.UpdateID, kind, "pending").
		Updates(map[string]any{
			"status": status, "retryable": retryable, "error_code": truncateRunes(errorCode, 64), "ended_at": time.Now().UTC(),
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return errors.New("manual feedback action plan is missing or already completed")
	}
	return nil
}
