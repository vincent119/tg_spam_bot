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
