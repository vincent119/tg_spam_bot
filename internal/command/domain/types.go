// Package domain 定義 Telegram 管理指令的框架無關模型與驗證規則。
package domain

import (
	"errors"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// Name 是固定公開的管理指令名稱。
type Name string

const (
	// NameHelp 顯示指令說明。
	NameHelp Name = "help"
	// NamePing 檢查 Bot 存活。
	NamePing Name = "ping"
	// NameID 顯示群組與使用者 ID。
	NameID Name = "id"
	// NameWarnings 查詢警告摘要。
	NameWarnings Name = "warnings"
	// NameWarn 新增人工警告。
	NameWarn Name = "warn"
	// NameClearWarn 失效有效警告。
	NameClearWarn Name = "clearwarn"
	// NameDelete 刪除被回覆訊息。
	NameDelete Name = "del"
	// NameMute 限時禁言成員。
	NameMute Name = "mute"
	// NameUnmute 解除成員禁言。
	NameUnmute Name = "unmute"
	// NameBan 封鎖成員。
	NameBan Name = "ban"
	// NameUnban 解除成員封鎖。
	NameUnban Name = "unban"
)

// User 保存指令授權及目標保護需要的最小 Telegram 使用者資訊。
type User struct {
	ID       int64
	IsBot    bool
	Username string
}

// Command 是 delivery 驗證 Telegram entity 後交給 application 的不可變輸入。
type Command struct {
	UpdateID      int64
	ChatID        int64
	MessageID     int64
	Actor         User
	Target        *User
	TargetMessage int64
	Name          Name
	Args          string
}

// NewCommand 驗證必要識別資訊並複製文字邊界。
func NewCommand(command Command) (Command, error) {
	if command.UpdateID == 0 || command.ChatID == 0 || command.MessageID == 0 || command.Actor.ID == 0 || command.Name == "" {
		return Command{}, errors.New("command identifiers and name are required")
	}
	command.Actor.Username = strings.Clone(command.Actor.Username)
	command.Args = strings.Clone(strings.TrimSpace(command.Args))
	if command.Target != nil {
		target := *command.Target
		target.Username = strings.Clone(target.Username)
		command.Target = &target
	}
	return command, nil
}

// Definition 描述指令權限、目標及顯示用法。
type Definition struct {
	Name          Name
	AdminOnly     bool
	RequiresReply bool
	Usage         string
	Description   string
}

// Definitions 回傳固定且複製過的指令註冊表。
func Definitions() []Definition {
	return []Definition{
		{Name: NameHelp, Usage: "/help", Description: "查看指令說明"},
		{Name: NamePing, Usage: "/ping", Description: "檢查機器人是否在線"},
		{Name: NameID, Usage: "/id", Description: "查看群組與使用者 ID"},
		{Name: NameWarnings, AdminOnly: true, RequiresReply: true, Usage: "/warnings（回覆成員訊息）", Description: "查看最近 30 天警告"},
		{Name: NameWarn, AdminOnly: true, RequiresReply: true, Usage: "/warn [原因]（回覆成員訊息）", Description: "人工增加警告"},
		{Name: NameClearWarn, AdminOnly: true, RequiresReply: true, Usage: "/clearwarn [原因]（回覆成員訊息）", Description: "失效目前警告"},
		{Name: NameDelete, AdminOnly: true, RequiresReply: true, Usage: "/del（回覆訊息）", Description: "刪除訊息"},
		{Name: NameMute, AdminOnly: true, RequiresReply: true, Usage: "/mute <時間> [原因]（回覆成員訊息）", Description: "禁言成員"},
		{Name: NameUnmute, AdminOnly: true, RequiresReply: true, Usage: "/unmute（回覆成員訊息）", Description: "解除禁言"},
		{Name: NameBan, AdminOnly: true, RequiresReply: true, Usage: "/ban [原因]（回覆成員訊息）", Description: "封鎖成員"},
		{Name: NameUnban, AdminOnly: true, Usage: "/unban <user_id> 或回覆訊息", Description: "解除封鎖"},
	}
}

// LookupDefinition 查詢固定指令定義。
func LookupDefinition(name Name) (Definition, bool) {
	for _, definition := range Definitions() {
		if definition.Name == name {
			return definition, true
		}
	}
	return Definition{}, false
}

// ParseReason 限制人工原因，避免超長內容進入 Telegram 回應及稽核。
func ParseReason(value string) (string, error) {
	reason := strings.TrimSpace(value)
	if utf8.RuneCountInString(reason) > 200 {
		return "", errors.New("原因不得超過 200 個字元")
	}
	return reason, nil
}

// ParseDuration 支援管理員易讀的 m、h、d 單位，並限制一分鐘至七天。
func ParseDuration(value string) (time.Duration, error) {
	value = strings.TrimSpace(value)
	if len(value) < 2 {
		return 0, errors.New("禁言時間格式錯誤")
	}
	unit := value[len(value)-1]
	number, err := strconv.ParseInt(value[:len(value)-1], 10, 64)
	if err != nil || number <= 0 {
		return 0, errors.New("禁言時間格式錯誤")
	}
	var duration time.Duration
	switch unit {
	case 'm':
		duration = time.Duration(number) * time.Minute
	case 'h':
		duration = time.Duration(number) * time.Hour
	case 'd':
		duration = time.Duration(number) * 24 * time.Hour
	default:
		return 0, errors.New("禁言時間只支援 m、h、d")
	}
	if duration < time.Minute || duration > 7*24*time.Hour {
		return 0, errors.New("禁言時間必須介於一分鐘至七天")
	}
	return duration, nil
}

// ParseUserID 只接受正整數 Telegram user ID。
func ParseUserID(value string) (int64, error) {
	id, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil || id <= 0 {
		return 0, errors.New("user ID 格式錯誤")
	}
	return id, nil
}
