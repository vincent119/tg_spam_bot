package postgres

import (
	"testing"
	"time"

	"github.com/vincent119/tg_spam_bot/internal/detection/application"
	"github.com/vincent119/tg_spam_bot/internal/detection/domain"
)

func TestToEventMessageMetadata(t *testing.T) {
	t.Parallel()
	sentAt := time.Date(2026, time.October, 5, 2, 15, 0, 0, time.FixedZone("測試時區", 8*60*60))
	tests := []struct {
		name          string
		message       domain.Message
		wantUsername  bool
		wantFirstName bool
		wantTime      bool
	}{
		{
			name:         "有名稱與發送時間",
			message:      domain.Message{Username: "example", FirstName: "測試", ReceivedAt: sentAt},
			wantUsername: true, wantFirstName: true, wantTime: true,
		},
		{name: "只有 first_name", message: domain.Message{FirstName: "測試", ReceivedAt: sentAt}, wantFirstName: true, wantTime: true},
		{name: "缺少名稱與時間", message: domain.Message{}},
		{name: "Unix 零值不是有效時間", message: domain.Message{ReceivedAt: time.Unix(0, 0)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			row := toEvent(application.Event{ID: "tg:1", Message: tt.message, CreatedAt: time.Now().UTC()})
			if (row.Username != nil) != tt.wantUsername || (row.FirstName != nil) != tt.wantFirstName || (row.MessageSentAt != nil) != tt.wantTime {
				t.Fatalf("toEvent() 名稱或時間欄位不符：%+v", row)
			}
			if tt.wantUsername && *row.Username != tt.message.Username || tt.wantFirstName && *row.FirstName != tt.message.FirstName {
				t.Fatalf("toEvent() 名稱快照不符：%+v", row)
			}
			if tt.wantTime && (!row.MessageSentAt.Equal(sentAt) || row.MessageSentAt.Location() != time.UTC) {
				t.Fatalf("toEvent() 發送時間應為 UTC：%v", row.MessageSentAt)
			}
		})
	}
}
