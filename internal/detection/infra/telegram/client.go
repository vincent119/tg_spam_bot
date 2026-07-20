// Package telegram 提供最小權限管理所需的 Telegram Bot API Client。
package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

var credentialURLPattern = regexp.MustCompile(`(?i)(https?://)[^\s/@:]+:[^\s/@]+@`)

// APIError 是已遮蔽敏感資料的 Telegram 穩定錯誤分類。
type APIError struct {
	method      string
	code        int
	description string
	retryable   bool
	kind        string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("telegram %s 失敗：code=%d kind=%s description=%s", e.method, e.code, e.kind, e.description)
}

// ErrorCode 供 application 將穩定類型寫入稽核，不暴露原始 response。
func (e *APIError) ErrorCode() string { return e.kind }

// IsRetryable 只對限流、Server 錯誤與網路類暫時問題回傳 true。
func (e *APIError) IsRetryable() bool { return e.retryable }

// Client 封裝最小權限管理所需的 Telegram Bot API。
type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

// BotIdentity 保存 getMe 回傳且可安全用於指令比對的 Bot 身分。
type BotIdentity struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
}

// NewClient 建立重用 Transport 且具有整體逾時的 Telegram Client。
func NewClient(baseURL, token string, client *http.Client) (*Client, error) {
	if token == "" {
		return nil, errors.New("telegram token 不得為空")
	}
	if client == nil {
		client = &http.Client{Transport: &http.Transport{MaxIdleConns: 100, IdleConnTimeout: 90 * time.Second, TLSHandshakeTimeout: 10 * time.Second}, Timeout: 15 * time.Second}
	}
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), token: token, http: client}, nil
}

// DeleteMessage 刪除已判定為垃圾訊息的群組訊息。
func (c *Client) DeleteMessage(ctx context.Context, chatID, messageID int64) error {
	return c.call(ctx, "deleteMessage", map[string]any{"chat_id": chatID, "message_id": messageID})
}

// SendWarning 發送包含成員識別碼的群組警告。
func (c *Client) SendWarning(ctx context.Context, chatID, userID int64, text string) error {
	return c.call(ctx, "sendMessage", map[string]any{"chat_id": chatID, "text": fmt.Sprintf("使用者 %d：%s", userID, text)})
}

// SendMessage 回覆觸發指令的訊息，讓群組管理員能對應操作結果。
func (c *Client) SendMessage(ctx context.Context, chatID, replyToMessageID int64, text string) error {
	payload := map[string]any{"chat_id": chatID, "text": text}
	if replyToMessageID != 0 {
		payload["reply_parameters"] = map[string]any{"message_id": replyToMessageID, "allow_sending_without_reply": true}
	}
	return c.call(ctx, "sendMessage", payload)
}

// RestrictMember 將成員限制至指定 UTC 時間。
func (c *Client) RestrictMember(ctx context.Context, chatID, userID int64, until time.Time) error {
	permissions := map[string]bool{"can_send_messages": false}
	return c.call(ctx, "restrictChatMember", map[string]any{"chat_id": chatID, "user_id": userID, "permissions": permissions, "until_date": until.Unix()})
}

// UnrestrictMember 恢復 Telegram 一般成員常用權限。
func (c *Client) UnrestrictMember(ctx context.Context, chatID, userID int64) error {
	var chat struct {
		Permissions map[string]bool `json:"permissions"`
	}
	if err := c.callResult(ctx, "getChat", map[string]any{"chat_id": chatID}, &chat); err != nil {
		return err
	}
	if chat.Permissions == nil {
		chat.Permissions = map[string]bool{"can_send_messages": true}
	}
	return c.call(ctx, "restrictChatMember", map[string]any{"chat_id": chatID, "user_id": userID, "permissions": chat.Permissions})
}

// BanMember 封鎖符合嚴重規則或達到第四階梯的成員。
func (c *Client) BanMember(ctx context.Context, chatID, userID int64) error {
	return c.call(ctx, "banChatMember", map[string]any{"chat_id": chatID, "user_id": userID, "revoke_messages": true})
}

// UnbanMember 解除封鎖但不要求使用者立即重新加入群組。
func (c *Client) UnbanMember(ctx context.Context, chatID, userID int64) error {
	return c.call(ctx, "unbanChatMember", map[string]any{"chat_id": chatID, "user_id": userID, "only_if_banned": true})
}

// GetMe 取得 Bot ID 與 username，供啟動驗證及群組指令 suffix 比對。
func (c *Client) GetMe(ctx context.Context) (BotIdentity, error) {
	var identity BotIdentity
	if err := c.callResult(ctx, "getMe", map[string]any{}, &identity); err != nil {
		return BotIdentity{}, err
	}
	if identity.ID <= 0 || strings.TrimSpace(identity.Username) == "" {
		return BotIdentity{}, errors.New("telegram getMe 缺少 Bot ID 或 username")
	}
	return identity, nil
}

// IsAdmin 即時查詢成員狀態，避免沿用已撤銷權限的快取。
func (c *Client) IsAdmin(ctx context.Context, chatID, userID int64) (bool, error) {
	var result struct {
		Status string `json:"status"`
	}
	if err := c.callResult(ctx, "getChatMember", map[string]any{"chat_id": chatID, "user_id": userID}, &result); err != nil {
		return false, err
	}
	return result.Status == "creator" || result.Status == "administrator", nil
}

// AdminIDs 取得管理員識別碼供偵測前豁免使用。
func (c *Client) AdminIDs(ctx context.Context, chatID int64) ([]int64, error) {
	var result []struct {
		User struct {
			ID int64 `json:"id"`
		} `json:"user"`
	}
	if err := c.callResult(ctx, "getChatAdministrators", map[string]any{"chat_id": chatID}, &result); err != nil {
		return nil, err
	}
	ids := make([]int64, 0, len(result))
	for _, admin := range result {
		ids = append(ids, admin.User.ID)
	}
	return ids, nil
}

func (c *Client) call(ctx context.Context, method string, payload any) error {
	return c.callResult(ctx, method, payload, nil)
}

func (c *Client) callResult(ctx context.Context, method string, payload, target any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode telegram request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/bot"+c.token+"/"+method, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create telegram request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return &APIError{method: method, kind: "temporary", description: c.mask(err.Error()), retryable: true}
	}
	defer func() { _ = resp.Body.Close() }()
	limited, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return fmt.Errorf("read telegram %s response: %w", method, err)
	}
	var result struct {
		OK          bool            `json:"ok"`
		ErrorCode   int             `json:"error_code"`
		Description string          `json:"description"`
		Result      json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(limited, &result); err != nil {
		return fmt.Errorf("decode telegram %s response with status %d", method, resp.StatusCode)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 || !result.OK {
		kind, retryable := classifyAPIError(resp.StatusCode, result.ErrorCode)
		return &APIError{method: method, code: result.ErrorCode, description: c.mask(result.Description), retryable: retryable, kind: kind}
	}
	if target != nil {
		if err := json.Unmarshal(result.Result, target); err != nil {
			return fmt.Errorf("decode telegram %s result: %w", method, err)
		}
	}
	return nil
}

func (c *Client) mask(value string) string {
	if c.token != "" {
		value = strings.ReplaceAll(value, c.token, "[已遮蔽]")
	}
	return credentialURLPattern.ReplaceAllString(value, `${1}[已遮蔽]@`)
}

func classifyAPIError(httpStatus, telegramCode int) (string, bool) {
	code := telegramCode
	if code == 0 {
		code = httpStatus
	}
	switch {
	case code == http.StatusUnauthorized || code == http.StatusForbidden:
		return "permission_denied", false
	case code == http.StatusTooManyRequests || code >= http.StatusInternalServerError:
		return "temporary", true
	default:
		return "telegram_rejected", false
	}
}
