package telegram

import (
	"strings"
	"time"

	"github.com/vincent119/tg_spam_bot/internal/detection/domain"
)

type Update struct {
	UpdateID int64    `json:"update_id"`
	Message  *Message `json:"message,omitempty"`
}

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

type Chat struct {
	ID   int64  `json:"id"`
	Type string `json:"type"`
}

type User struct {
	ID        int64  `json:"id"`
	IsBot     bool   `json:"is_bot"`
	FirstName string `json:"first_name"`
}

type MessageEntity struct {
	Type   string `json:"type"`
	Offset int    `json:"offset"`
	Length int    `json:"length"`
	URL    string `json:"url,omitempty"`
}

func (u Update) DomainMessage() (domain.Message, bool) {
	if u.UpdateID == 0 || u.Message == nil || u.Message.From == nil || u.Message.From.IsBot || u.Message.Chat.ID == 0 || u.Message.MessageID == 0 {
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
