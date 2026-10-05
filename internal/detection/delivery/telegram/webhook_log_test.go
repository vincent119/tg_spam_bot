package telegram

import (
	"testing"
	"time"

	"github.com/vincent119/zlogger"
)

func TestWebhookUpdateLogFields(t *testing.T) {
	t.Parallel()
	sentAt := time.Date(2026, time.October, 5, 2, 0, 0, 0, time.UTC)
	receivedAt := sentAt.Add(2 * time.Hour)
	tests := []struct {
		name         string
		from         *User
		wantUserID   bool
		wantUsername bool
	}{
		{name: "有 username", from: &User{ID: 42, Username: "example_user", FirstName: "測試"}, wantUserID: true, wantUsername: true},
		{name: "沒有 username", from: &User{ID: 42, FirstName: "測試"}, wantUserID: true},
		{name: "沒有個人發送者", from: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			update := Update{UpdateID: 1, Message: &Message{MessageID: 3, Date: sentAt.Unix(), Chat: Chat{ID: -1001}, From: tt.from, Text: "不應寫入日誌的原文"}}
			fields := webhookUpdateLogFields(update, receivedAt)
			byKey := make(map[string]zlogger.Field, len(fields))
			for _, field := range fields {
				byKey[field.Key] = field
			}
			_, hasUserID := byKey["user_id"]
			_, hasUsername := byKey["username"]
			if hasUserID != tt.wantUserID || hasUsername != tt.wantUsername {
				t.Fatalf("發送者欄位不符：%v", byKey)
			}
			if tt.wantUserID && byKey["user_id"].Integer != 42 || tt.wantUsername && byKey["username"].String != "example_user" {
				t.Fatalf("發送者識別不符：%v", byKey)
			}
			if byKey["message_sent_at"].String != sentAt.Format(time.RFC3339) || byKey["delivery_lag_seconds"].Integer != 7200 {
				t.Fatalf("原始時間或延遲不符：%v", byKey)
			}
			if _, exists := byKey["first_name"]; exists {
				t.Fatal("日誌不應記錄 first_name")
			}
			if _, exists := byKey["text"]; exists {
				t.Fatal("日誌不應記錄原文")
			}
		})
	}
}
