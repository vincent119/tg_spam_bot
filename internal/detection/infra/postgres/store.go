// Package postgres 提供以 GORM 實作的偵測狀態與稽核儲存。
package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	commandapp "github.com/vincent119/tg_spam_bot/internal/command/application"
	commanddomain "github.com/vincent119/tg_spam_bot/internal/command/domain"
	"github.com/vincent119/tg_spam_bot/internal/detection/application"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type processedUpdate struct {
	UpdateID    int64      `gorm:"primaryKey;comment:Telegram 更新唯一識別碼"`
	Status      string     `gorm:"not null;comment:處理狀態"`
	ClaimedAt   time.Time  `gorm:"not null;comment:開始處理 UTC 時間"`
	CompletedAt *time.Time `gorm:"comment:完成處理 UTC 時間"`
}
type detectionEvent struct {
	EventID            string    `gorm:"primaryKey;comment:事件唯一識別碼"`
	UpdateID           int64     `gorm:"uniqueIndex;not null;comment:Telegram 更新識別碼"`
	ChatID             int64     `gorm:"index;not null;comment:Telegram 聊天識別碼"`
	MessageID          int64     `gorm:"not null;comment:Telegram 訊息識別碼"`
	UserID             int64     `gorm:"index;not null;comment:Telegram 成員識別碼"`
	ContentFingerprint string    `gorm:"not null;comment:有金鑰的內容指紋"`
	CategoryID         string    `gorm:"comment:命中的違規類型"`
	Severity           string    `gorm:"comment:違規嚴重度"`
	RuleVersion        string    `gorm:"comment:規則快照版本"`
	Mode               string    `gorm:"comment:執行模式"`
	Score              int       `gorm:"comment:偵測總分"`
	Threshold          int       `gorm:"comment:判定門檻"`
	IsSpam             bool      `gorm:"comment:是否判定為垃圾訊息"`
	Matches            []byte    `gorm:"type:jsonb;comment:命中規則摘要"`
	Signals            []byte    `gorm:"type:jsonb;comment:命中行為訊號摘要"`
	CreatedAt          time.Time `gorm:"index;not null;comment:事件 UTC 時間"`
}
type violation struct {
	ID                 uint64     `gorm:"primaryKey;comment:違規流水號"`
	EventID            string     `gorm:"uniqueIndex;not null;comment:偵測或人工事件識別碼"`
	ChatID             int64      `gorm:"index:idx_violation_member_time;comment:Telegram 聊天識別碼"`
	UserID             int64      `gorm:"index:idx_violation_member_time;comment:Telegram 成員識別碼"`
	CategoryID         string     `gorm:"comment:違規類型"`
	Severity           string     `gorm:"comment:違規嚴重度"`
	Source             string     `gorm:"not null;default:auto;index;comment:自動或人工來源"`
	OperatorID         int64      `gorm:"comment:人工操作管理員識別碼"`
	Reason             string     `gorm:"size:200;comment:人工操作原因"`
	OccurredAt         time.Time  `gorm:"index:idx_violation_member_time;not null;comment:違規 UTC 時間"`
	InvalidatedAt      *time.Time `gorm:"index;comment:違規失效 UTC 時間"`
	InvalidatedBy      int64      `gorm:"comment:執行失效的管理員識別碼"`
	InvalidationReason string     `gorm:"size:200;comment:違規失效原因"`
}
type enforcementAction struct {
	ActionKey string     `gorm:"primaryKey;comment:冪等處置鍵"`
	EventID   string     `gorm:"index;not null;comment:偵測事件識別碼"`
	Kind      string     `gorm:"comment:處置種類"`
	Status    string     `gorm:"comment:處置狀態"`
	Retryable bool       `gorm:"comment:是否允許重試"`
	ErrorCode string     `gorm:"comment:外部錯誤代碼"`
	ErrorText string     `gorm:"comment:遮罩後錯誤摘要"`
	EndedAt   *time.Time `gorm:"comment:處置結束 UTC 時間"`
}
type trustedMember struct {
	ChatID    int64     `gorm:"primaryKey;comment:Telegram 聊天識別碼"`
	UserID    int64     `gorm:"primaryKey;comment:可信任成員識別碼"`
	Reason    string    `gorm:"comment:信任原因"`
	Enabled   bool      `gorm:"comment:是否啟用豁免"`
	CreatedAt time.Time `gorm:"comment:建立 UTC 時間"`
}

type commandExecution struct {
	ID              uint64     `gorm:"primaryKey;comment:管理指令流水號"`
	ChatID          int64      `gorm:"uniqueIndex:idx_command_update;not null;comment:Telegram 聊天識別碼"`
	UpdateID        int64      `gorm:"uniqueIndex:idx_command_update;not null;comment:Telegram 更新識別碼"`
	MessageID       int64      `gorm:"not null;comment:Telegram 指令訊息識別碼"`
	Command         string     `gorm:"size:32;not null;comment:管理指令名稱"`
	OperatorID      int64      `gorm:"index;not null;comment:指令操作者識別碼"`
	TargetUserID    int64      `gorm:"index;comment:目標成員識別碼"`
	TargetMessageID int64      `gorm:"comment:目標訊息識別碼"`
	ArgumentSummary string     `gorm:"size:200;comment:不含秘密值的參數摘要"`
	Source          string     `gorm:"size:32;not null;comment:操作來源"`
	Status          string     `gorm:"index;not null;comment:執行狀態"`
	Result          string     `gorm:"size:500;comment:安全結果摘要"`
	ErrorText       string     `gorm:"size:500;comment:安全錯誤摘要"`
	CreatedAt       time.Time  `gorm:"not null;comment:建立 UTC 時間"`
	CompletedAt     *time.Time `gorm:"comment:完成 UTC 時間"`
}

// Store 保存偵測、違規、處置及可信任成員資料。
type Store struct{ db *gorm.DB }

// NewStore 建立 GORM repository。
func NewStore(db *gorm.DB) (*Store, error) {
	if db == nil {
		return nil, errors.New("gorm db is required")
	}
	return &Store{db: db}, nil
}

// AutoMigrate 建立或向前更新本服務需要的資料表。
func AutoMigrate(ctx context.Context, db *gorm.DB) error {
	if db == nil {
		return errors.New("gorm db is required")
	}
	if err := db.WithContext(ctx).AutoMigrate(&processedUpdate{}, &detectionEvent{}, &violation{}, &enforcementAction{}, &trustedMember{}, &commandExecution{}); err != nil {
		return fmt.Errorf("auto migrate detection schema: %w", err)
	}
	comments := map[string]string{"processed_updates": "Telegram 更新冪等紀錄", "detection_events": "垃圾訊息偵測事件", "violations": "成員違規紀錄", "enforcement_actions": "Telegram 處置執行紀錄", "trusted_members": "可信任成員名單", "command_executions": "Telegram 人工管理指令與稽核紀錄"}
	for table, comment := range comments {
		query := fmt.Sprintf("COMMENT ON TABLE %s IS '%s'", table, strings.ReplaceAll(comment, "'", "''"))
		if err := db.WithContext(ctx).Exec(query).Error; err != nil {
			return fmt.Errorf("comment table %s: %w", table, err)
		}
	}
	return nil
}

// Claim 以資料庫唯一鍵原子占用 update_id，收斂跨程序重送。
func (s *Store) Claim(ctx context.Context, id int64) (bool, error) {
	result := s.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&processedUpdate{UpdateID: id, Status: "processing", ClaimedAt: time.Now().UTC()})
	return result.RowsAffected == 1, result.Error
}

// Complete 將已完整處理的 Telegram 更新標記完成。
func (s *Store) Complete(ctx context.Context, id int64) error {
	now := time.Now().UTC()
	return s.db.WithContext(ctx).Model(&processedUpdate{}).Where("update_id = ?", id).Updates(map[string]any{"status": "completed", "completed_at": now}).Error
}

// Release 只釋放仍在 processing 的更新，讓失敗請求可重試。
func (s *Store) Release(ctx context.Context, id int64) error {
	return s.db.WithContext(ctx).Where("update_id = ? AND status = ?", id, "processing").Delete(&processedUpdate{}).Error
}

// IsExempt 查詢群組範圍且仍啟用的可信任成員。
func (s *Store) IsExempt(ctx context.Context, chatID, userID int64) (bool, string, error) {
	var m trustedMember
	result := s.db.WithContext(ctx).Where("chat_id=? AND user_id=? AND enabled", chatID, userID).Limit(1).Find(&m)
	if result.Error != nil {
		return false, "", result.Error
	}
	if result.RowsAffected == 0 {
		return false, "", nil
	}
	return true, m.Reason, nil
}

// RecordDetection 保存觀測或非垃圾訊息的可稽核判定。
func (s *Store) RecordDetection(ctx context.Context, event application.Event) error {
	return s.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(toEvent(event)).Error
}

// Create 在單一 transaction 內建立違規並計算 30 天處置階梯。
func (s *Store) Create(ctx context.Context, event application.Event) (count int, actions []application.EnforcementAction, err error) {
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(toEvent(event)).Error; e != nil {
			return e
		}
		if event.Mode != application.ModeDeleteOnly {
			v := violation{EventID: event.ID, ChatID: event.Message.ChatID, UserID: event.Message.UserID, CategoryID: event.Result.CategoryID, Severity: string(event.Result.Severity), Source: "auto", OccurredAt: event.CreatedAt}
			if e := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&v).Error; e != nil {
				return e
			}
			var count64 int64
			if e := tx.Model(&violation{}).Where("chat_id=? AND user_id=? AND occurred_at>=? AND invalidated_at IS NULL", v.ChatID, v.UserID, event.CreatedAt.Add(-30*24*time.Hour)).Count(&count64).Error; e != nil {
				return e
			}
			count = int(count64)
		}
		for _, kind := range application.PlanActions(event.Result, event.Mode, count) {
			a := application.EnforcementAction{Key: event.ID + ":" + string(kind), Kind: kind}
			row := enforcementAction{ActionKey: a.Key, EventID: event.ID, Kind: string(kind), Status: "pending"}
			if e := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; e != nil {
				return e
			}
			actions = append(actions, a)
		}
		return nil
	})
	return
}

// ClaimCommand 以群組與 update 唯一鍵原子占用管理指令。
func (s *Store) ClaimCommand(ctx context.Context, command commanddomain.Command) (bool, error) {
	targetID := int64(0)
	if command.Target != nil {
		targetID = command.Target.ID
	}
	row := commandExecution{
		ChatID: command.ChatID, UpdateID: command.UpdateID, MessageID: command.MessageID,
		Command: truncateRunes(string(command.Name), 32), OperatorID: command.Actor.ID, TargetUserID: targetID,
		TargetMessageID: command.TargetMessage, ArgumentSummary: truncateRunes(command.Args, 200), Source: "manual_command",
		Status: "processing", CreatedAt: time.Now().UTC(),
	}
	result := s.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&row)
	return result.RowsAffected == 1, result.Error
}

func truncateRunes(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}

// CompleteCommand 保存指令穩定結果，不保存 Telegram 原始 response。
func (s *Store) CompleteCommand(ctx context.Context, command commanddomain.Command, status, resultText, errorText string) error {
	now := time.Now().UTC()
	return s.db.WithContext(ctx).Model(&commandExecution{}).
		Where("chat_id=? AND update_id=?", command.ChatID, command.UpdateID).
		Updates(map[string]any{"status": status, "result": resultText, "error_text": errorText, "completed_at": now}).Error
}

// Warnings 回傳指定期間仍有效的人工及自動違規摘要。
func (s *Store) Warnings(ctx context.Context, chatID, userID int64, since time.Time) (commandapp.WarningSummary, error) {
	type sourceCount struct {
		Source string
		Count  int64
	}
	var rows []sourceCount
	err := s.db.WithContext(ctx).Model(&violation{}).
		Select("source, count(*) AS count").
		Where("chat_id=? AND user_id=? AND occurred_at>=? AND invalidated_at IS NULL", chatID, userID, since).
		Group("source").Scan(&rows).Error
	if err != nil {
		return commandapp.WarningSummary{}, err
	}
	var summary commandapp.WarningSummary
	for _, row := range rows {
		summary.Total += int(row.Count)
		if row.Source == "manual" {
			summary.Manual += int(row.Count)
		} else {
			summary.Automatic += int(row.Count)
		}
	}
	return summary, nil
}

// AddManualWarning 在 transaction 內建立人工違規並回傳最新 30 天摘要。
func (s *Store) AddManualWarning(ctx context.Context, command commanddomain.Command, reason string, occurredAt time.Time) (summary commandapp.WarningSummary, err error) {
	if command.Target == nil {
		return commandapp.WarningSummary{}, errors.New("人工警告缺少目標")
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		row := violation{
			EventID: fmt.Sprintf("manual:%d:%d", command.ChatID, command.UpdateID), ChatID: command.ChatID,
			UserID: command.Target.ID, CategoryID: "manual_warning", Severity: "warning", Source: "manual",
			OperatorID: command.Actor.ID, Reason: reason, OccurredAt: occurredAt,
		}
		if createErr := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; createErr != nil {
			return createErr
		}
		store := &Store{db: tx}
		var queryErr error
		summary, queryErr = store.Warnings(ctx, command.ChatID, command.Target.ID, occurredAt.Add(-30*24*time.Hour))
		return queryErr
	})
	return summary, err
}

// ClearWarnings 以失效標記保留原始違規及完整稽核鏈。
func (s *Store) ClearWarnings(ctx context.Context, command commanddomain.Command, reason string, invalidatedAt time.Time) (int64, error) {
	if command.Target == nil {
		return 0, errors.New("清除警告缺少目標")
	}
	result := s.db.WithContext(ctx).Model(&violation{}).
		Where("chat_id=? AND user_id=? AND invalidated_at IS NULL", command.ChatID, command.Target.ID).
		Updates(map[string]any{"invalidated_at": invalidatedAt, "invalidated_by": command.Actor.ID, "invalidation_reason": reason})
	return result.RowsAffected, result.Error
}

// CompleteAction 保存每項 Telegram API 呼叫的獨立結果。
func (s *Store) CompleteAction(ctx context.Context, key string, result application.ActionResult) error {
	status := "failed"
	if result.Succeeded {
		status = "completed"
	}
	return s.db.WithContext(ctx).Model(&enforcementAction{}).Where("action_key=?", key).Updates(map[string]any{"status": status, "retryable": result.Retryable, "error_code": result.ErrorCode, "error_text": result.ErrorText, "ended_at": result.EndedAt}).Error
}

func toEvent(e application.Event) *detectionEvent {
	matches, _ := json.Marshal(e.Result.MatchesCopy())
	signals, _ := json.Marshal(e.Result.SignalsCopy())
	return &detectionEvent{EventID: e.ID, UpdateID: e.Message.UpdateID, ChatID: e.Message.ChatID, MessageID: e.Message.MessageID, UserID: e.Message.UserID, ContentFingerprint: e.Fingerprint, CategoryID: e.Result.CategoryID, Severity: string(e.Result.Severity), Score: e.Result.Score, Threshold: e.Result.Threshold, RuleVersion: e.Result.RuleVersion, Mode: string(e.Mode), IsSpam: e.Result.Spam, Matches: matches, Signals: signals, CreatedAt: e.CreatedAt}
}
