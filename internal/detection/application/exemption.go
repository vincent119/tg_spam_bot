// Package application 協調偵測、豁免、違規與 Telegram 處置用例。
package application

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// TrustedMembers 查詢每個群組明確設定的可信任成員。
type TrustedMembers interface {
	IsExempt(ctx context.Context, chatID, userID int64) (bool, string, error)
}

// AdminProvider 從 Telegram 取得群組管理員清單。
type AdminProvider interface {
	AdminIDs(ctx context.Context, chatID int64) ([]int64, error)
}

// CachedExemptions 合併可信任名單與短期快取的 Telegram 管理員資料。
type CachedExemptions struct {
	trusted TrustedMembers
	admins  AdminProvider
	ttl     time.Duration
	now     func() time.Time
	mu      sync.Mutex
	cache   map[int64]adminEntry
}

type adminEntry struct {
	expires time.Time
	ids     map[int64]struct{}
}

// NewCachedExemptions 建立具有有限快取期限的豁免查詢器。
func NewCachedExemptions(trusted TrustedMembers, admins AdminProvider, ttl time.Duration) (*CachedExemptions, error) {
	if trusted == nil || admins == nil || ttl <= 0 {
		return nil, fmt.Errorf("trusted store, admin provider and positive ttl are required")
	}
	return &CachedExemptions{trusted: trusted, admins: admins, ttl: ttl, now: time.Now, cache: make(map[int64]adminEntry)}, nil
}

// IsExempt 優先查可信任名單，再查 Telegram 管理員身分。
func (c *CachedExemptions) IsExempt(ctx context.Context, chatID, userID int64) (bool, string, error) {
	trusted, reason, err := c.trusted.IsExempt(ctx, chatID, userID)
	if err != nil || trusted {
		return trusted, reason, err
	}
	ids, err := c.adminsFor(ctx, chatID)
	if err != nil {
		return false, "", err
	}
	_, admin := ids[userID]
	if admin {
		return true, "telegram_admin", nil
	}
	return false, "", nil
}

func (c *CachedExemptions) adminsFor(ctx context.Context, chatID int64) (map[int64]struct{}, error) {
	c.mu.Lock()
	entry, ok := c.cache[chatID]
	if ok && c.now().Before(entry.expires) {
		c.mu.Unlock()
		return entry.ids, nil
	}
	c.mu.Unlock()
	ids, err := c.admins.AdminIDs(ctx, chatID)
	if err != nil {
		return nil, fmt.Errorf("get telegram admins: %w", err)
	}
	set := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		set[id] = struct{}{}
	}
	c.mu.Lock()
	c.cache[chatID] = adminEntry{expires: c.now().Add(c.ttl), ids: set}
	c.mu.Unlock()
	return set, nil
}
