// Package postgres 提供以 GORM 實作的偵測狀態與稽核儲存。
package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

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
	ID         uint64    `gorm:"primaryKey;comment:違規流水號"`
	EventID    string    `gorm:"uniqueIndex;not null;comment:偵測事件識別碼"`
	ChatID     int64     `gorm:"index;comment:Telegram 聊天識別碼"`
	UserID     int64     `gorm:"index;comment:Telegram 成員識別碼"`
	CategoryID string    `gorm:"comment:違規類型"`
	Severity   string    `gorm:"comment:違規嚴重度"`
	OccurredAt time.Time `gorm:"index;not null;comment:違規 UTC 時間"`
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
	if err := db.WithContext(ctx).AutoMigrate(&processedUpdate{}, &detectionEvent{}, &violation{}, &enforcementAction{}, &trustedMember{}); err != nil {
		return fmt.Errorf("auto migrate detection schema: %w", err)
	}
	comments := map[string]string{"processed_updates": "Telegram 更新冪等紀錄", "detection_events": "垃圾訊息偵測事件", "violations": "成員違規紀錄", "enforcement_actions": "Telegram 處置執行紀錄", "trusted_members": "可信任成員名單"}
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
	err := s.db.WithContext(ctx).Where("chat_id=? AND user_id=? AND enabled", chatID, userID).First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, "", nil
	}
	return err == nil, m.Reason, err
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
			v := violation{EventID: event.ID, ChatID: event.Message.ChatID, UserID: event.Message.UserID, CategoryID: event.Result.CategoryID, Severity: string(event.Result.Severity), OccurredAt: event.CreatedAt}
			if e := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&v).Error; e != nil {
				return e
			}
			var count64 int64
			if e := tx.Model(&violation{}).Where("chat_id=? AND user_id=? AND occurred_at>=?", v.ChatID, v.UserID, event.CreatedAt.Add(-30*24*time.Hour)).Count(&count64).Error; e != nil {
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
