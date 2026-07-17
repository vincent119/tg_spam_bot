package application

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type TrustedMembers interface {
	IsExempt(ctx context.Context, chatID, userID int64) (bool, string, error)
}

type AdminProvider interface {
	AdminIDs(ctx context.Context, chatID int64) ([]int64, error)
}

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

func NewCachedExemptions(trusted TrustedMembers, admins AdminProvider, ttl time.Duration) (*CachedExemptions, error) {
	if trusted == nil || admins == nil || ttl <= 0 {
		return nil, fmt.Errorf("trusted store, admin provider and positive ttl are required")
	}
	return &CachedExemptions{trusted: trusted, admins: admins, ttl: ttl, now: time.Now, cache: make(map[int64]adminEntry)}, nil
}

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
