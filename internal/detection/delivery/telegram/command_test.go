package telegram

import (
	"testing"

	commanddomain "github.com/vincent119/tg_spam_bot/internal/command/domain"
)

func TestUpdateCommand(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		text        string
		entity      MessageEntity
		botUsername string
		wantName    commanddomain.Name
		wantArgs    string
		want        CommandDisposition
	}{
		{name: "基本指令", text: "/ping", entity: MessageEntity{Type: "bot_command", Length: 5}, botUsername: "liyu_spam_bot", wantName: commanddomain.NamePing, want: CommandHandle},
		{name: "群組 suffix", text: "/warn@liyu_spam_bot 廣告", entity: MessageEntity{Type: "bot_command", Length: 19}, botUsername: "liyu_spam_bot", wantName: commanddomain.NameWarn, wantArgs: "廣告", want: CommandHandle},
		{name: "其他 bot", text: "/ping@other_bot", entity: MessageEntity{Type: "bot_command", Length: 15}, botUsername: "liyu_spam_bot", want: CommandIgnore},
		{name: "不是開頭", text: "測試 /ping", entity: MessageEntity{Type: "bot_command", Offset: 3, Length: 5}, botUsername: "liyu_spam_bot", want: CommandNone},
		{name: "Unicode 邊界無效", text: "😀/ping", entity: MessageEntity{Type: "bot_command", Offset: 1, Length: 5}, botUsername: "liyu_spam_bot", want: CommandNone},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			update := Update{UpdateID: 1, Message: &Message{MessageID: 2, Chat: Chat{ID: -1001, Type: "supergroup"}, From: &User{ID: 3}, Text: tt.text, Entities: []MessageEntity{tt.entity}}}
			got, disposition := update.Command(tt.botUsername)
			if disposition != tt.want || got.Name != tt.wantName || got.Args != tt.wantArgs {
				t.Fatalf("Command() = %+v, %v", got, disposition)
			}
		})
	}
}

func FuzzUTF16Segment(f *testing.F) {
	f.Add("/ping 測試", 0, 5)
	f.Add("😀/ping", 2, 5)
	f.Fuzz(func(_ *testing.T, value string, offset, length int) {
		_, _, _ = utf16Segment(value, offset, length)
	})
}

func BenchmarkUpdateCommand(b *testing.B) {
	update := Update{UpdateID: 1, Message: &Message{MessageID: 2, Chat: Chat{ID: -1001, Type: "supergroup"}, From: &User{ID: 3}, Text: "/mute@liyu_spam_bot 10m 洗版", Entities: []MessageEntity{{Type: "bot_command", Length: 19}}}}
	for b.Loop() {
		_, _ = update.Command("liyu_spam_bot")
	}
}
