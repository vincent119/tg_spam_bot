package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/vincent119/tg_spam_bot/internal/detection/application"
	"gorm.io/gorm"
)

// FindHistoricalDetection 只讀取同群訊息的已保存摘要，不回傳原文或雜湊指紋。
func (s *Store) FindHistoricalDetection(ctx context.Context, chatID, messageID int64) (application.HistoricalDetection, bool, error) {
	if chatID == 0 || messageID <= 0 {
		return application.HistoricalDetection{}, false, fmt.Errorf("invalid historical detection target")
	}
	var row detectionEvent
	err := s.db.WithContext(ctx).Where("chat_id=? AND message_id=?", chatID, messageID).
		Order("created_at DESC").Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return application.HistoricalDetection{}, false, nil
	}
	if err != nil {
		return application.HistoricalDetection{}, false, fmt.Errorf("find historical detection: %w", err)
	}
	return application.HistoricalDetection{
		RuleVersion: row.RuleVersion,
		CategoryID:  row.CategoryID,
		Score:       row.Score,
		Threshold:   row.Threshold,
		Spam:        row.IsSpam,
		Mode:        application.Mode(row.Mode),
		CreatedAt:   row.CreatedAt,
	}, true, nil
}
