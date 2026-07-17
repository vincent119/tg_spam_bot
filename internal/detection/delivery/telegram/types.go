// Package telegram 將 Telegram Webhook DTO 安全轉換為領域訊息。
package telegram

import (
	"strings"
	"time"

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
