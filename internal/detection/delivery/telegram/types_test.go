package telegram

import "testing"

func TestDomainMessageExtractsReferenceText(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		message Message
		want    string
	}{
		{
			name:    "優先使用明確引用",
			message: Message{Text: "聯絡 @seller", Quote: &TextQuote{Text: "抖音代刷礼物"}, ReplyToMessage: &Message{Text: "完整原文"}},
			want:    "抖音代刷礼物",
		},
		{
			name:    "無引用時使用回覆內容",
			message: Message{Text: "聯絡 @seller", ReplyToMessage: &Message{Caption: "抖音代刷礼物"}},
			want:    "抖音代刷礼物",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.message.MessageID = 2
			tt.message.Date = 1
			tt.message.Chat = Chat{ID: -1001, Type: "supergroup"}
			tt.message.From = &User{ID: 4}
			message, ok := (Update{UpdateID: 1, Message: &tt.message}).DomainMessage()
			if !ok || message.ReferenceText != tt.want {
				t.Fatalf("DomainMessage() ok=%v reference=%q, want %q", ok, message.ReferenceText, tt.want)
			}
		})
	}
}

func TestDomainMessageCaptionAndInvalidUpdates(t *testing.T) {
	t.Parallel()

	update := Update{UpdateID: 1, Message: &Message{
		MessageID: 2, Date: 1, Chat: Chat{ID: -1001, Type: "supergroup"}, From: &User{ID: 4},
		Caption: "媒體說明", CaptionEntities: []MessageEntity{{Type: "mention"}},
	}}
	message, ok := update.DomainMessage()
	if !ok || message.Text != "媒體說明" || len(message.Entities) != 1 {
		t.Fatalf("message=%+v ok=%v", message, ok)
	}

	invalid := []Update{
		{},
		{UpdateID: 1, Message: &Message{MessageID: 2, Chat: Chat{ID: -1001, Type: "supergroup"}, From: &User{ID: 4, IsBot: true}, Text: "hello"}},
		{UpdateID: 1, Message: &Message{MessageID: 2, Chat: Chat{ID: -1001, Type: "supergroup"}, From: &User{ID: 4}}},
	}
	for _, candidate := range invalid {
		if _, ok := candidate.DomainMessage(); ok {
			t.Fatalf("無效 update 不應轉換：%+v", candidate)
		}
	}
}
