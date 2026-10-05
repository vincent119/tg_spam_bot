package application

import (
	"testing"

	"github.com/vincent119/tg_spam_bot/internal/detection/domain"
	"github.com/vincent119/zlogger"
)

func TestDetectionResultLogFields(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		username     string
		wantUsername bool
	}{
		{name: "有 username", username: "example_user", wantUsername: true},
		{name: "沒有 username"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := Event{ID: "tg:1", Message: domain.Message{UpdateID: 1, ChatID: -1001, MessageID: 3, UserID: 42, Username: tt.username, FirstName: "測試", Text: "不應寫入日誌的原文"}}
			fields := detectionResultLogFields(event)
			byKey := make(map[string]zlogger.Field, len(fields))
			for _, field := range fields {
				byKey[field.Key] = field
			}
			if byKey["user_id"].Integer != 42 {
				t.Fatalf("user_id 不符：%v", byKey)
			}
			username, exists := byKey["username"]
			if exists != tt.wantUsername || exists && username.String != tt.username {
				t.Fatalf("username 不符：%v", byKey)
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
