// Package domain 定義 Telegram 管理指令的框架無關模型與驗證規則。
package domain

import (
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
	// NameFeedSpam 提交漏網垃圾樣本供語意記憶使用。
	NameFeedSpam Name = "feedspam"
)

// User 保存指令授權及目標保護需要的最小 Telegram 使用者資訊。
type User struct {
	ID       int64
	IsBot    bool
	Username string
}

// Actor 是發起指令的群組成員，與處置目標分離可避免授權邏輯混用。
type Actor User

// Target 是指令要查詢或處置的一般成員。
type Target User

// Duration 是已通過一分鐘至七天邊界驗證的禁言時間。
type Duration time.Duration

// TimeDuration 轉為標準庫時間，僅在 Telegram API 邊界使用。
func (d Duration) TimeDuration() time.Duration { return time.Duration(d) }

// Reason 是已通過 200 Unicode code points 上限驗證的稽核原因。
type Reason string

// Result 是可安全落盤與在重送時回傳的穩定指令結果。
type Result struct {
	Status    string
	Message   string
	ErrorCode string
	Retryable bool
}

// Claim 區分新取得的指令與已有結果，避免重送再次執行副作用。
type Claim struct {
	Acquired bool
	Existing *Result
}

// ErrorCode 是可供 errors.As 後穩定判斷的指令錯誤類型。
type ErrorCode string

const (
	// ErrorInvalidInput 表示指令參數不符合固定語法。
	ErrorInvalidInput ErrorCode = "invalid_input"
	// ErrorUnauthorized 表示操作者沒有群組管理權限。
	ErrorUnauthorized ErrorCode = "unauthorized"
	// ErrorProtected 表示目標為管理員、Bot 或可信任成員。
	ErrorProtected ErrorCode = "protected_target"
	// ErrorTemporary 表示依賴服務的可重試暫時失敗。
	ErrorTemporary ErrorCode = "temporary_failure"
)

// CommandError 將穩定 code 與繁體中文公開訊息分離。
type CommandError struct {
	Code    ErrorCode
	Message string
}

func (e *CommandError) Error() string { return e.Message }

func invalidInput(message string) error {
	return &CommandError{Code: ErrorInvalidInput, Message: message}
}

// Command 是 delivery 驗證 Telegram entity 後交給 application 的不可變輸入。
type Command struct {
	UpdateID      int64
	ChatID        int64
	MessageID     int64
	Actor         Actor
	Target        *Target
	TargetMessage int64
	TargetText    string
	Name          Name
	Args          string
}

// NewCommand 驗證必要識別資訊並複製文字邊界。
func NewCommand(command Command) (Command, error) {
	if command.UpdateID == 0 || command.ChatID == 0 || command.MessageID == 0 || command.Actor.ID == 0 || command.Name == "" {
		return Command{}, invalidInput("指令識別資訊與名稱不得為空")
	}
	command.Actor.Username = strings.Clone(command.Actor.Username)
	command.Args = strings.Clone(strings.TrimSpace(command.Args))
	command.TargetText = strings.Clone(strings.TrimSpace(command.TargetText))
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
		{Name: NameFeedSpam, AdminOnly: true, RequiresReply: true, Usage: "/feedspam [分類]（回覆漏網垃圾訊息）", Description: "提交漏網垃圾樣本"},
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
func ParseReason(value string) (Reason, error) {
	reason := strings.TrimSpace(value)
	if utf8.RuneCountInString(reason) > 200 {
		return "", invalidInput("原因不得超過 200 個字元")
	}
	return Reason(reason), nil
}

// ParseFeedSpamCategory 驗證人工提交樣本分類，避免任意長文字進入稽核欄位。
func ParseFeedSpamCategory(value string) (string, error) {
	category := strings.TrimSpace(value)
	if category == "" {
		return "uncategorized_spam", nil
	}
	if utf8.RuneCountInString(category) > 64 {
		return "", invalidInput("分類不得超過 64 個字元")
	}
	for _, r := range category {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-' {
			continue
		}
		return "", invalidInput("分類只能包含英文字母、數字、底線或連字號")
	}
	return strings.ToLower(category), nil
}

// ParseDuration 支援管理員易讀的 m、h、d 單位，並限制一分鐘至七天。
func ParseDuration(value string) (Duration, error) {
	value = strings.TrimSpace(value)
	if len(value) < 2 {
		return 0, invalidInput("禁言時間格式錯誤")
	}
	unit := value[len(value)-1]
	number, err := strconv.ParseInt(value[:len(value)-1], 10, 64)
	if err != nil || number <= 0 {
		return 0, invalidInput("禁言時間格式錯誤")
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
		return 0, invalidInput("禁言時間只支援 m、h、d")
	}
	if duration < time.Minute || duration > 7*24*time.Hour {
		return 0, invalidInput("禁言時間必須介於一分鐘至七天")
	}
	return Duration(duration), nil
}

// ParseUserID 只接受正整數 Telegram user ID。
func ParseUserID(value string) (int64, error) {
	id, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil || id <= 0 {
		return 0, invalidInput("user ID 格式錯誤")
	}
	return id, nil
}
