package postgres

import (
	"context"
	"errors"
	"fmt"
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
func (s *Store) PlanFeedbackActions(ctx context.Context, command commanddomain.Command, kinds []commandapp.FeedbackActionKind, messageIDs ...int64) error {
	if command.Target == nil || command.TargetMessage <= 0 || len(kinds) == 0 {
		return errors.New("manual feedback action target is required")
	}
	rows, err := feedbackActionPlan(command, kinds, messageIDs)
	if err != nil {
		return err
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var execution commandExecution
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("chat_id=? AND update_id=?", command.ChatID, command.UpdateID).Take(&execution).Error; err != nil {
			return err
		}
		var existing []manualFeedbackAction
		if err := tx.Where("chat_id=? AND command_update_id=?", command.ChatID, command.UpdateID).Find(&existing).Error; err != nil {
			return err
		}
		if len(existing) > 0 {
			if !sameFeedbackActionPlan(existing, rows) {
				return errors.New("人工標記處置計畫已固定，不允許變更目標")
			}
			return nil
		}
		return tx.Create(&rows).Error
	})
}

func feedbackActionPlan(command commanddomain.Command, kinds []commandapp.FeedbackActionKind, messageIDs []int64) ([]manualFeedbackAction, error) {
	if len(messageIDs) == 0 {
		messageIDs = []int64{command.TargetMessage}
	}
	if len(messageIDs) > 100 {
		return nil, errors.New("人工標記刪除上限為 100 則")
	}
	seen := make(map[string]struct{})
	var rows []manualFeedbackAction
	for _, kind := range kinds {
		if kind != commandapp.FeedbackActionDelete && kind != commandapp.FeedbackActionBan {
			return nil, errors.New("unsupported manual feedback action")
		}
		targets := []int64{command.TargetMessage}
		if kind == commandapp.FeedbackActionDelete {
			targets = messageIDs
		}
		for _, messageID := range targets {
			if messageID <= 0 {
				return nil, errors.New("人工標記刪除目標無效")
			}
			key := feedbackActionKey(kind, command.TargetMessage, messageID)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			rows = append(rows, manualFeedbackAction{
				ChatID: command.ChatID, CommandUpdateID: command.UpdateID, Kind: key,
				MessageID: messageID, TargetUserID: command.Target.ID, Status: "pending", CreatedAt: time.Now().UTC(),
			})
		}
	}
	return rows, nil
}

func feedbackActionKey(kind commandapp.FeedbackActionKind, targetMessageID, messageID int64) string {
	// 保留原目標的既有動作鍵，額外訊息沿用同一唯一索引逐項稽核。
	if kind == commandapp.FeedbackActionDelete && messageID != targetMessageID {
		return fmt.Sprintf("delete:%d", messageID)
	}
	return string(kind)
}

func sameFeedbackActionPlan(existing, planned []manualFeedbackAction) bool {
	if len(existing) != len(planned) {
		return false
	}
	byKey := make(map[string]manualFeedbackAction, len(existing))
	for _, row := range existing {
		byKey[row.Kind] = row
	}
	for _, row := range planned {
		previous, ok := byKey[row.Kind]
		if !ok || previous.MessageID != row.MessageID || previous.TargetUserID != row.TargetUserID {
			return false
		}
	}
	return true
}

// CompleteFeedbackAction 單獨保存 Telegram 動作結果，未回寫成功時仍保留 pending 供稽核。
func (s *Store) CompleteFeedbackAction(ctx context.Context, command commanddomain.Command, kind commandapp.FeedbackActionKind, succeeded, retryable bool, errorCode string, messageIDs ...int64) error {
	messageID := command.TargetMessage
	if len(messageIDs) > 1 || (len(messageIDs) == 1 && (messageIDs[0] <= 0 || kind != commandapp.FeedbackActionDelete)) {
		return errors.New("人工標記處置結果的目標無效")
	}
	if len(messageIDs) == 1 {
		messageID = messageIDs[0]
	}
	status := "failed"
	if succeeded {
		status = "completed"
	}
	result := s.db.WithContext(ctx).Model(&manualFeedbackAction{}).
		Where("chat_id=? AND command_update_id=? AND kind=? AND message_id=? AND status=?", command.ChatID, command.UpdateID, feedbackActionKey(kind, command.TargetMessage, messageID), messageID, "pending").
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
