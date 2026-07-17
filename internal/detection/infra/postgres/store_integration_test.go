package postgres

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/vincent119/tg_spam_bot/internal/detection/application"
	"github.com/vincent119/tg_spam_bot/internal/detection/domain"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestStoreIntegration(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("未設定 TEST_DATABASE_URL")
	}
	ctx := t.Context()
	db, err := gorm.Open(postgres.Open(databaseURL), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := AutoMigrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	store, _ := NewStore(db)
	seed := time.Now().UnixNano()
	chatID, userID := seed, seed+1
	t.Cleanup(func() {
		cleanup := db.WithContext(context.Background())
		_ = cleanup.Where("event_id LIKE ?", fmt.Sprintf("it-%d-%%", seed)).Delete(&enforcementAction{}).Error
		_ = cleanup.Where("chat_id = ?", chatID).Delete(&violation{}).Error
		_ = cleanup.Where("chat_id = ?", chatID).Delete(&detectionEvent{}).Error
	})

	for i := 1; i <= 4; i++ {
		event := application.Event{
			ID:          fmt.Sprintf("it-%d-%d", seed, i),
			Message:     domain.Message{UpdateID: seed + int64(i), ChatID: chatID, MessageID: int64(i), UserID: userID},
			Fingerprint: "fingerprint", Mode: application.ModeEnforce,
			Result:    domain.Result{Spam: true, CategoryID: "generic", Severity: domain.SeverityNormal, Action: domain.ActionProgressive, RuleVersion: "it"},
			CreatedAt: time.Now().UTC(),
		}
		count, actions, err := store.Create(ctx, event)
		if err != nil {
			t.Fatal(err)
		}
		if count != i || len(actions) != 2 {
			t.Fatalf("count = %d actions = %v", count, actions)
		}
	}
}
