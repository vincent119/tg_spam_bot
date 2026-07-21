package postgres

import (
	"context"
	"encoding/json"
	"time"

	"github.com/vincent119/tg_spam_bot/internal/detection/application"
	"github.com/vincent119/tg_spam_bot/internal/detection/domain"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type aiDetectionEvent struct {
	ID                 uint64     `gorm:"primaryKey;comment:AI 判定流水號"`
	ChatID             int64      `gorm:"uniqueIndex:idx_ai_detection_update;index;not null;comment:Telegram 聊天識別碼"`
	UpdateID           int64      `gorm:"uniqueIndex:idx_ai_detection_update;not null;comment:Telegram 更新識別碼"`
	MessageID          int64      `gorm:"not null;comment:Telegram 訊息識別碼"`
	UserID             int64      `gorm:"index;not null;comment:Telegram 成員識別碼"`
	ContentFingerprint string     `gorm:"index:idx_ai_detection_cache,priority:1;not null;comment:有金鑰的內容指紋"`
	Provider           string     `gorm:"size:64;index:idx_ai_detection_cache,priority:2;not null;comment:AI provider 名稱"`
	Model              string     `gorm:"size:200;index:idx_ai_detection_cache,priority:3;not null;comment:AI 模型名稱"`
	PromptVersion      string     `gorm:"size:64;index:idx_ai_detection_cache,priority:4;not null;comment:Prompt 版本"`
	SchemaVersion      string     `gorm:"size:64;comment:AI 回應 schema 版本"`
	RuleVersion        string     `gorm:"size:64;index:idx_ai_detection_cache,priority:5;comment:規則快照版本"`
	Status             string     `gorm:"size:32;index;not null;comment:AI 判定狀態"`
	Label              string     `gorm:"size:32;comment:AI 判定標籤"`
	Category           string     `gorm:"size:100;comment:AI 判定分類"`
	Confidence         float64    `gorm:"comment:AI 信心分數"`
	ConfidenceSource   string     `gorm:"size:32;comment:AI 信心分數來源"`
	ReasonCode         string     `gorm:"size:100;comment:AI 判定原因代碼"`
	Evidence           []byte     `gorm:"type:jsonb;comment:AI 證據摘要，不含完整原文"`
	SafeAction         string     `gorm:"size:32;comment:AI 建議最高安全動作"`
	ErrorCode          string     `gorm:"size:64;comment:穩定錯誤類型"`
	ErrorText          string     `gorm:"size:500;comment:遮罩後錯誤摘要"`
	Retryable          bool       `gorm:"comment:失敗是否可重試"`
	CreatedAt          time.Time  `gorm:"index;index:idx_ai_detection_cache,priority:6;not null;comment:建立 UTC 時間"`
	CompletedAt        *time.Time `gorm:"comment:完成 UTC 時間"`
}

// ClaimAIDetection 以群組與 update 唯一鍵原子占用 AI 判定事件。
func (s *Store) ClaimAIDetection(ctx context.Context, event application.AIDetectionEvent) (application.AIDetectionClaim, error) {
	row := aiDetectionEvent{
		ChatID: event.ChatID, UpdateID: event.UpdateID, MessageID: event.MessageID, UserID: event.UserID,
		ContentFingerprint: event.ContentFingerprint, Provider: truncateRunes(event.Provider, 64), Model: truncateRunes(event.Model, 200),
		PromptVersion: truncateRunes(event.PromptVersion, 64), SchemaVersion: truncateRunes(event.SchemaVersion, 64),
		RuleVersion: truncateRunes(event.RuleVersion, 64), Status: "processing", CreatedAt: event.CreatedAt,
	}
	result := s.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&row)
	if result.Error != nil {
		return application.AIDetectionClaim{}, result.Error
	}
	if result.RowsAffected == 1 {
		return application.AIDetectionClaim{Acquired: true}, nil
	}
	var existing aiDetectionEvent
	if err := s.db.WithContext(ctx).Where("chat_id=? AND update_id=?", event.ChatID, event.UpdateID).Take(&existing).Error; err != nil {
		return application.AIDetectionClaim{}, err
	}
	if existing.Status == "failed" && existing.Retryable {
		update := s.db.WithContext(ctx).Model(&aiDetectionEvent{}).
			Where("chat_id=? AND update_id=? AND status=? AND retryable", event.ChatID, event.UpdateID, "failed").
			Updates(map[string]any{"status": "processing", "error_code": "", "error_text": "", "retryable": false})
		if update.Error != nil {
			return application.AIDetectionClaim{}, update.Error
		}
		if update.RowsAffected == 1 {
			return application.AIDetectionClaim{Acquired: true}, nil
		}
	}
	return application.AIDetectionClaim{Existing: aiDetectionResult(existing)}, nil
}

// CompleteAIDetection 保存 AI 判定成功結果，不保存 provider 原始完整 response。
func (s *Store) CompleteAIDetection(ctx context.Context, event application.AIDetectionEvent, result domain.AIClassifyResult) error {
	now := time.Now().UTC()
	evidence, _ := json.Marshal(result.EvidenceCopy())
	return s.db.WithContext(ctx).Model(&aiDetectionEvent{}).
		Where("chat_id=? AND update_id=?", event.ChatID, event.UpdateID).
		Updates(map[string]any{
			"status": "completed", "label": string(result.Label), "category": truncateRunes(result.Category, 100),
			"confidence": result.Confidence, "confidence_source": string(result.ConfidenceSource),
			"reason_code": truncateRunes(result.ReasonCode, 100), "evidence": evidence,
			"safe_action": string(result.SafeAction), "completed_at": now,
		}).Error
}

// FailAIDetection 保存 AI 判定失敗摘要與是否可重試。
func (s *Store) FailAIDetection(ctx context.Context, event application.AIDetectionEvent, result application.AIDetectionResult) error {
	now := time.Now().UTC()
	return s.db.WithContext(ctx).Model(&aiDetectionEvent{}).
		Where("chat_id=? AND update_id=?", event.ChatID, event.UpdateID).
		Updates(map[string]any{
			"status": "failed", "error_code": truncateRunes(result.ErrorCode, 64),
			"error_text": truncateRunes(result.ErrorText, 500), "retryable": result.Retryable, "completed_at": now,
		}).Error
}

// FindCachedAIDetection 依內容、provider、模型、prompt 與規則版本查詢未過期完成判定。
func (s *Store) FindCachedAIDetection(ctx context.Context, key application.AIDetectionCacheKey) (application.AIDetectionResult, bool, error) {
	if key.CacheTTL <= 0 {
		return application.AIDetectionResult{}, false, nil
	}
	now := key.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	var row aiDetectionEvent
	err := s.db.WithContext(ctx).Where(
		"content_fingerprint=? AND provider=? AND model=? AND prompt_version=? AND rule_version=? AND status=? AND created_at>=?",
		key.ContentFingerprint, key.Provider, key.Model, key.PromptVersion, key.RuleVersion, "completed", now.UTC().Add(-key.CacheTTL),
	).Order("created_at DESC").Take(&row).Error
	if err == nil {
		return *aiDetectionResult(row), true, nil
	}
	if err == gorm.ErrRecordNotFound {
		return application.AIDetectionResult{}, false, nil
	}
	return application.AIDetectionResult{}, false, err
}

func aiDetectionResult(row aiDetectionEvent) *application.AIDetectionResult {
	result := domain.AIClassifyResult{
		Label:            domain.AILabel(row.Label),
		Category:         row.Category,
		Confidence:       row.Confidence,
		ConfidenceSource: domain.AIConfidenceSource(row.ConfidenceSource),
		ReasonCode:       row.ReasonCode,
		SafeAction:       domain.AISafeAction(row.SafeAction),
		PromptVersion:    row.PromptVersion,
	}
	if len(row.Evidence) > 0 {
		_ = json.Unmarshal(row.Evidence, &result.Evidence)
	}
	return &application.AIDetectionResult{
		Status: row.Status, Result: result, ErrorCode: row.ErrorCode, ErrorText: row.ErrorText,
		Retryable: row.Retryable, CreatedAt: row.CreatedAt, CompletedAt: row.CompletedAt,
	}
}
