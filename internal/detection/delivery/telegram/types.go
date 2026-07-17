// Package telegram 將 Telegram Webhook DTO 安全轉換為領域訊息。
package telegram

import (
	"strings"
	"time"
	"unicode/utf8"

	commanddomain "github.com/vincent119/tg_spam_bot/internal/command/domain"
	"github.com/vincent119/tg_spam_bot/internal/detection/domain"
)

// Update 只描述本服務需要的 Telegram 更新欄位，以容忍未使用欄位擴充。
type Update struct {
	UpdateID int64    `json:"update_id"`
	Message  *Message `json:"message,omitempty"`
}

// Message 描述文字或媒體說明所需的 Telegram 訊息欄位。
type Message struct {
	MessageID       int64           `json:"message_id"`
	Date            int64           `json:"date"`
	Chat            Chat            `json:"chat"`
	From            *User           `json:"from,omitempty"`
	SenderChat      *Chat           `json:"sender_chat,omitempty"`
	ReplyToMessage  *Message        `json:"reply_to_message,omitempty"`
	Text            string          `json:"text,omitempty"`
	Caption         string          `json:"caption,omitempty"`
	Entities        []MessageEntity `json:"entities,omitempty"`
	CaptionEntities []MessageEntity `json:"caption_entities,omitempty"`
}

// Chat 保存 Telegram 群組識別資訊。
type Chat struct {
	ID   int64  `json:"id"`
	Type string `json:"type"`
}

// User 保存發送者識別資訊並排除 Bot 訊息。
type User struct {
	ID        int64  `json:"id"`
	IsBot     bool   `json:"is_bot"`
	FirstName string `json:"first_name"`
	Username  string `json:"username,omitempty"`
}

// CommandDisposition 說明 delivery 是否應分流或忽略開頭的 Bot 指令。
type CommandDisposition uint8

const (
	// CommandNone 表示訊息不是 Telegram bot command，應繼續垃圾訊息偵測。
	CommandNone CommandDisposition = iota
	// CommandHandle 表示指令屬於目前 Bot，應交由 command handler。
	CommandHandle
	// CommandIgnore 表示指令明確屬於其他 Bot，不得回覆或偵測。
	CommandIgnore
)

// Command 依 Telegram UTF-16 entity 邊界解析開頭指令，避免 Unicode 文字造成 offset 錯位。
func (u Update) Command(botUsername string) (commanddomain.Command, CommandDisposition) {
	if u.UpdateID == 0 || u.Message == nil || u.Message.From == nil || u.Message.From.IsBot || u.Message.Chat.ID == 0 || u.Message.MessageID == 0 || !u.Message.Chat.isModeratedGroup() {
		return commanddomain.Command{}, CommandNone
	}
	var entity *MessageEntity
	for i := range u.Message.Entities {
		candidate := &u.Message.Entities[i]
		if candidate.Type == "bot_command" && candidate.Offset == 0 {
			entity = candidate
			break
		}
	}
	if entity == nil {
		return commanddomain.Command{}, CommandNone
	}
	token, byteEnd, ok := utf16Segment(u.Message.Text, entity.Offset, entity.Length)
	if !ok || !strings.HasPrefix(token, "/") || len(token) < 2 {
		return commanddomain.Command{}, CommandIgnore
	}
	namePart := token[1:]
	if name, suffix, found := strings.Cut(namePart, "@"); found {
		if !strings.EqualFold(suffix, strings.TrimPrefix(botUsername, "@")) {
			return commanddomain.Command{}, CommandIgnore
		}
		namePart = name
	}
	command := commanddomain.Command{
		UpdateID:  u.UpdateID,
		ChatID:    u.Message.Chat.ID,
		MessageID: u.Message.MessageID,
		Actor:     commanddomain.User{ID: u.Message.From.ID, IsBot: u.Message.From.IsBot, Username: u.Message.From.Username},
		Name:      commanddomain.Name(strings.ToLower(namePart)),
		Args:      strings.TrimSpace(u.Message.Text[byteEnd:]),
	}
	if reply := u.Message.ReplyToMessage; reply != nil {
		command.TargetMessage = reply.MessageID
		if reply.From != nil {
			command.Target = &commanddomain.User{ID: reply.From.ID, IsBot: reply.From.IsBot, Username: reply.From.Username}
		}
	}
	validated, err := commanddomain.NewCommand(command)
	if err != nil {
		return commanddomain.Command{}, CommandIgnore
	}
	return validated, CommandHandle
}

// utf16Segment 將 Telegram UTF-16 code unit offset 安全轉換為 Go UTF-8 byte 邊界。
func utf16Segment(value string, offset, length int) (string, int, bool) {
	if offset < 0 || length <= 0 {
		return "", 0, false
	}
	if offset > int(^uint(0)>>1)-length {
		return "", 0, false
	}
	endUnit := offset + length
	startByte := -1
	endByte := -1
	units := 0
	for byteIndex, r := range value {
		if units == offset {
			startByte = byteIndex
		}
		width := 1
		if r > 0xFFFF {
			width = 2
		}
		if units < offset && units+width > offset || units < endUnit && units+width > endUnit {
			return "", 0, false
		}
		units += width
		if units == endUnit {
			_, runeWidth := utf8.DecodeRuneInString(value[byteIndex:])
			endByte = byteIndex + runeWidth
			break
		}
	}
	if offset == 0 {
		startByte = 0
	}
	if endUnit == 0 {
		endByte = 0
	}
	if startByte < 0 || endByte < startByte {
		return "", 0, false
	}
	return value[startByte:endByte], endByte, true
}

// MessageEntity 保存 Telegram 已解析的 URL 與 mention 範圍。
type MessageEntity struct {
	Type   string `json:"type"`
	Offset int    `json:"offset"`
	Length int    `json:"length"`
	URL    string `json:"url,omitempty"`
}

// DomainMessage 驗證必要識別欄位，並將文字或 caption 轉為領域訊息。
func (u Update) DomainMessage() (domain.Message, bool) {
	if u.UpdateID == 0 || u.Message == nil || u.Message.From == nil || u.Message.From.IsBot || u.Message.Chat.ID == 0 || u.Message.MessageID == 0 || !u.Message.Chat.isModeratedGroup() {
		return domain.Message{}, false
	}
	text := u.Message.Text
	entities := u.Message.Entities
	if strings.TrimSpace(text) == "" {
		text = u.Message.Caption
		entities = u.Message.CaptionEntities
	}
	if strings.TrimSpace(text) == "" {
		return domain.Message{}, false
	}
	domainEntities := make([]domain.Entity, 0, len(entities))
	for _, entity := range entities {
		domainEntities = append(domainEntities, domain.Entity{Type: entity.Type, URL: entity.URL})
	}
	return domain.NewMessage(domain.Message{
		UpdateID: u.UpdateID, ChatID: u.Message.Chat.ID, MessageID: u.Message.MessageID,
		UserID: u.Message.From.ID, Text: text, Entities: domainEntities,
		ReceivedAt: time.Unix(u.Message.Date, 0).UTC(),
	}), true
}

// isModeratedGroup 只接受具成員管理語意的群組，避免將私人聊天或頻道貼文送入處置流程。
func (c Chat) isModeratedGroup() bool {
	return c.Type == "group" || c.Type == "supergroup"
}
