package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

func NewClient(baseURL, token string, client *http.Client) (*Client, error) {
	if token == "" {
		return nil, fmt.Errorf("telegram token is required")
	}
	if client == nil {
		client = &http.Client{Transport: &http.Transport{MaxIdleConns: 100, IdleConnTimeout: 90 * time.Second, TLSHandshakeTimeout: 10 * time.Second}, Timeout: 15 * time.Second}
	}
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), token: token, http: client}, nil
}

func (c *Client) DeleteMessage(ctx context.Context, chatID, messageID int64) error {
	return c.call(ctx, "deleteMessage", map[string]any{"chat_id": chatID, "message_id": messageID})
}

func (c *Client) SendWarning(ctx context.Context, chatID, userID int64, text string) error {
	return c.call(ctx, "sendMessage", map[string]any{"chat_id": chatID, "text": fmt.Sprintf("使用者 %d：%s", userID, text)})
}

func (c *Client) RestrictMember(ctx context.Context, chatID, userID int64, until time.Time) error {
	permissions := map[string]bool{"can_send_messages": false}
	return c.call(ctx, "restrictChatMember", map[string]any{"chat_id": chatID, "user_id": userID, "permissions": permissions, "until_date": until.Unix()})
}

func (c *Client) BanMember(ctx context.Context, chatID, userID int64) error {
	return c.call(ctx, "banChatMember", map[string]any{"chat_id": chatID, "user_id": userID, "revoke_messages": true})
}

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
		return fmt.Errorf("telegram %s: %w", method, err)
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
		return fmt.Errorf("telegram %s failed: code=%d description=%s", method, result.ErrorCode, result.Description)
	}
	if target != nil {
		if err := json.Unmarshal(result.Result, target); err != nil {
			return fmt.Errorf("decode telegram %s result: %w", method, err)
		}
	}
	return nil
}
