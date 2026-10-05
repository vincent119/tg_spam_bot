package postgres

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/vincent119/tg_spam_bot/internal/detection/application"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	repeatActionPolicyVersion = "repeat-v1"
	repeatStepLeaseDuration   = 2 * time.Minute
	maxRepeatActionSteps      = 101
)

type repeatActionExecution struct {
	EventID            string    `gorm:"primaryKey;size:128;not null"`
	ChatID             int64     `gorm:"index:idx_repeat_execution_scope;not null"`
	UserID             int64     `gorm:"index:idx_repeat_execution_scope;not null"`
	MessageID          int64     `gorm:"not null"`
	ContentFingerprint string    `gorm:"not null"`
	WindowMillis       int64     `gorm:"not null"`
	Count              int       `gorm:"not null"`
	Truncated          bool      `gorm:"not null"`
	Action             string    `gorm:"size:16;not null"`
	Mode               string    `gorm:"size:16;not null"`
	PolicyVersion      string    `gorm:"size:32;not null"`
	Steps              []byte    `gorm:"type:jsonb;not null"`
	CreatedAt          time.Time `gorm:"not null"`
}

type repeatActionStep struct {
	ActionKey         string `gorm:"primaryKey;size:200;not null"`
	FirstEventID      string `gorm:"size:128;not null"`
	ChatID            int64  `gorm:"index:idx_repeat_delete_target,unique,where:kind = 'delete';not null"`
	UserID            int64  `gorm:"not null"`
	MessageID         int64  `gorm:"index:idx_repeat_delete_target,unique,where:kind = 'delete';not null"`
	Kind              string `gorm:"size:16;not null"`
	Status            string `gorm:"size:16;not null"`
	Retryable         bool   `gorm:"not null"`
	ErrorCode         string `gorm:"size:64;not null"`
	ErrorText         string `gorm:"size:500;not null"`
	AttemptCount      int    `gorm:"not null"`
	HadUnknownOutcome bool   `gorm:"not null"`
	LeaseToken        string `gorm:"size:32;not null"`
	ClaimedAt         *time.Time
	LeaseExpiresAt    *time.Time
	EndedAt           *time.Time
	CreatedAt         time.Time `gorm:"not null"`
}

// AutoMigrateRepeatActions 建立精確重複處置的固定計畫與逐項狀態。
func AutoMigrateRepeatActions(ctx context.Context, db *gorm.DB) error {
	if db == nil {
		return errors.New("缺少資料庫連線")
	}
	if err := db.WithContext(ctx).AutoMigrate(&repeatActionExecution{}, &repeatActionStep{}); err != nil {
		return fmt.Errorf("建立重複處置資料表: %w", err)
	}
	for _, table := range []struct{ name, comment string }{
		{name: "repeat_action_executions", comment: "精確重複處置的不可變事件快照"},
		{name: "repeat_action_steps", comment: "精確重複處置的逐項 Telegram 動作"},
	} {
		if err := db.WithContext(ctx).Exec("COMMENT ON TABLE " + table.name + " IS '" + table.comment + "'").Error; err != nil {
			return fmt.Errorf("註記重複處置資料表 %s: %w", table.name, err)
		}
	}
	return nil
}

// CreateOrLoadRepeatExecution 首次保存候選及動作，重送只讀取原計畫。
func (s *Store) CreateOrLoadRepeatExecution(ctx context.Context, event application.Event, plan application.RepeatPlan, skipCurrentDelete bool) (application.RepeatExecution, error) {
	var saved repeatActionExecution
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		lookup := tx.Where("event_id = ?", event.ID).Limit(1).Find(&saved)
		if lookup.Error != nil {
			return lookup.Error
		}
		if lookup.RowsAffected == 1 {
			return validateRepeatExecutionScope(saved, event)
		}
		if !plan.Triggered {
			return nil
		}
		steps, err := buildRepeatSteps(event, plan, skipCurrentDelete)
		if err != nil {
			return err
		}
		encoded, err := json.Marshal(steps)
		if err != nil {
			return fmt.Errorf("編碼重複處置目標: %w", err)
		}
		createdAt := event.CreatedAt.UTC()
		if createdAt.IsZero() {
			createdAt = time.Now().UTC()
		}
		row := repeatActionExecution{
			EventID: event.ID, ChatID: event.Message.ChatID, UserID: event.Message.UserID, MessageID: event.Message.MessageID,
			ContentFingerprint: event.Fingerprint, WindowMillis: plan.Window.Milliseconds(),
			Count: plan.Count, Truncated: plan.Truncated, Action: string(plan.Action), Mode: string(plan.Mode),
			PolicyVersion: repeatActionPolicyVersion, Steps: encoded, CreatedAt: createdAt,
		}
		insert := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row)
		if insert.Error != nil {
			return insert.Error
		}
		if insert.RowsAffected == 0 {
			lookup = tx.Where("event_id = ?", event.ID).Limit(1).Find(&saved)
			if lookup.Error != nil {
				return lookup.Error
			}
			if lookup.RowsAffected != 1 {
				return errors.New("並行重複處置計畫未能讀取已保存事件")
			}
			return validateRepeatExecutionScope(saved, event)
		}
		insertOrder := slices.Clone(steps)
		slices.SortFunc(insertOrder, func(a, b application.RepeatStep) int {
			if a.Key < b.Key {
				return -1
			}
			if a.Key > b.Key {
				return 1
			}
			return 0
		})
		for _, step := range insertOrder {
			if err := insertRepeatStep(tx, row, step); err != nil {
				return err
			}
		}
		saved = row
		return nil
	})
	if err != nil {
		return application.RepeatExecution{}, fmt.Errorf("保存重複處置計畫: %w", err)
	}
	if saved.EventID == "" {
		return application.RepeatExecution{}, nil
	}
	var steps []application.RepeatStep
	if err := json.Unmarshal(saved.Steps, &steps); err != nil {
		return application.RepeatExecution{}, fmt.Errorf("解碼重複處置目標: %w", err)
	}
	return application.RepeatExecution{
		EventID: saved.EventID, ChatID: saved.ChatID, UserID: saved.UserID,
		Count: saved.Count, Truncated: saved.Truncated, Steps: steps,
	}, nil
}

func validateRepeatExecutionScope(saved repeatActionExecution, event application.Event) error {
	if saved.ChatID != event.Message.ChatID || saved.UserID != event.Message.UserID || saved.MessageID != event.Message.MessageID || saved.ContentFingerprint != event.Fingerprint {
		return errors.New("重送事件與已保存的重複處置範圍不一致")
	}
	return nil
}

func buildRepeatSteps(event application.Event, plan application.RepeatPlan, skipCurrentDelete bool) ([]application.RepeatStep, error) {
	if event.ID == "" || event.Fingerprint == "" || event.Message.ChatID == 0 || event.Message.UserID <= 0 || event.Message.MessageID <= 0 {
		return nil, errors.New("重複處置事件識別資料不完整")
	}
	if plan.Window < time.Millisecond || plan.Window > 24*time.Hour || plan.Count < len(plan.MessageIDs) || len(plan.MessageIDs) == 0 || len(plan.MessageIDs) > 100 {
		return nil, errors.New("重複處置候選快照無效")
	}
	if plan.Action != application.RepeatActionDelete && plan.Action != application.RepeatActionBan {
		return nil, errors.New("重複處置動作無效")
	}
	if plan.Mode != application.ModeDeleteOnly && plan.Mode != application.ModeEnforce {
		return nil, errors.New("重複處置模式無效")
	}
	if plan.Ban != (plan.Action == application.RepeatActionBan && plan.Mode == application.ModeEnforce) {
		return nil, errors.New("重複處置封鎖旗標與政策不符")
	}
	if plan.Truncated != (plan.Count > len(plan.MessageIDs)) {
		return nil, errors.New("重複處置截斷狀態與候選數不符")
	}
	seen := make(map[int64]struct{}, len(plan.MessageIDs))
	currentIncluded := false
	steps := make([]application.RepeatStep, 0, min(len(plan.MessageIDs)+1, maxRepeatActionSteps))
	for _, id := range plan.MessageIDs {
		if id <= 0 {
			return nil, errors.New("重複處置訊息 ID 無效")
		}
		if _, duplicate := seen[id]; duplicate {
			return nil, errors.New("重複處置候選包含重複 ID")
		}
		seen[id] = struct{}{}
		if id == event.Message.MessageID {
			currentIncluded = true
			if skipCurrentDelete {
				continue
			}
		}
		steps = append(steps, application.RepeatStep{
			Key:  fmt.Sprintf("repeat:delete:%d:%d", event.Message.ChatID, id),
			Kind: application.ActionDelete, MessageID: id, UserID: event.Message.UserID,
		})
	}
	if !currentIncluded {
		return nil, errors.New("重複處置候選缺少目前訊息")
	}
	if plan.Ban {
		steps = append(steps, application.RepeatStep{
			Key:  fmt.Sprintf("repeat:ban:%s:%d:%d", event.ID, event.Message.ChatID, event.Message.UserID),
			Kind: application.ActionBan, UserID: event.Message.UserID,
		})
	}
	return steps, nil
}

func insertRepeatStep(tx *gorm.DB, execution repeatActionExecution, step application.RepeatStep) error {
	row := repeatActionStep{
		ActionKey: step.Key, FirstEventID: execution.EventID, ChatID: execution.ChatID,
		UserID: execution.UserID, MessageID: step.MessageID, Kind: string(step.Kind),
		Status: "pending", CreatedAt: execution.CreatedAt,
	}
	insert := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row)
	if insert.Error != nil {
		return insert.Error
	}
	if insert.RowsAffected == 1 {
		return nil
	}
	var existing repeatActionStep
	if err := tx.Where("action_key = ?", step.Key).Take(&existing).Error; err != nil {
		return err
	}
	if existing.ChatID != row.ChatID || existing.UserID != row.UserID || existing.MessageID != row.MessageID || existing.Kind != row.Kind {
		return errors.New("重複處置步驟與已保存目標衝突")
	}
	return nil
}

// ClaimRepeatStep 以短租約占用單一動作，避免並行副本同時呼叫 Telegram。
func (s *Store) ClaimRepeatStep(ctx context.Context, key string) (application.RepeatStepClaim, error) {
	if key == "" {
		return application.RepeatStepClaim{}, errors.New("缺少重複處置步驟鍵")
	}
	token, err := randomRepeatLeaseToken()
	if err != nil {
		return application.RepeatStepClaim{}, err
	}
	now := time.Now().UTC()
	until := now.Add(repeatStepLeaseDuration)
	update := s.db.WithContext(ctx).Model(&repeatActionStep{}).
		Where("action_key = ? AND (status = ? OR (status = ? AND retryable) OR (status = ? AND lease_expires_at <= ?))", key, "pending", "failed", "processing", now).
		Updates(map[string]any{
			"status": "processing", "lease_token": token, "claimed_at": now, "lease_expires_at": until,
			"attempt_count":       gorm.Expr("attempt_count + 1"),
			"had_unknown_outcome": gorm.Expr("had_unknown_outcome OR status = ?", "processing"),
			"retryable":           false, "error_code": "", "error_text": "", "ended_at": nil,
		})
	if update.Error != nil {
		return application.RepeatStepClaim{}, fmt.Errorf("占用重複處置步驟: %w", update.Error)
	}
	if update.RowsAffected == 1 {
		return application.RepeatStepClaim{Acquired: true, LeaseToken: token}, nil
	}
	var row repeatActionStep
	if err := s.db.WithContext(ctx).Where("action_key = ?", key).Take(&row).Error; err != nil {
		return application.RepeatStepClaim{}, fmt.Errorf("讀取重複處置步驟: %w", err)
	}
	if row.Status == "completed" {
		return application.RepeatStepClaim{Completed: true}, nil
	}
	if row.Status == "failed" && !row.Retryable {
		return application.RepeatStepClaim{}, errors.New("重複處置步驟已失敗且不可重試")
	}
	return application.RepeatStepClaim{}, nil
}

// CompleteRepeatStep 只接受目前租約的回寫，過期執行者不能覆蓋新結果。
func (s *Store) CompleteRepeatStep(ctx context.Context, key, leaseToken string, result application.ActionResult) error {
	if key == "" || leaseToken == "" {
		return errors.New("重複處置步驟鍵與租約令牌不可為空")
	}
	status := "failed"
	if result.Succeeded {
		status = "completed"
		result.Retryable = false
	}
	endedAt := result.EndedAt.UTC()
	if endedAt.IsZero() {
		endedAt = time.Now().UTC()
	}
	update := s.db.WithContext(ctx).Model(&repeatActionStep{}).
		Where("action_key = ? AND status = ? AND lease_token = ?", key, "processing", leaseToken).
		Updates(map[string]any{
			"status": status, "retryable": result.Retryable,
			"error_code": truncateRunes(result.ErrorCode, 64), "error_text": truncateRunes(result.ErrorText, 500),
			"ended_at": endedAt, "lease_token": "", "lease_expires_at": nil,
		})
	if update.Error != nil {
		return fmt.Errorf("回寫重複處置步驟: %w", update.Error)
	}
	if update.RowsAffected != 1 {
		return errors.New("重複處置租約已失效，外部結果須視為未知")
	}
	return nil
}

func randomRepeatLeaseToken() (string, error) {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", fmt.Errorf("產生重複處置租約令牌: %w", err)
	}
	return hex.EncodeToString(bytes[:]), nil
}
