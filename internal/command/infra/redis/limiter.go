// Package redis 使用 Redis 保存跨副本一致的公開指令頻率狀態。
package redis

import (
	"context"
	"errors"
	"fmt"
	"time"

	redislib "github.com/redis/go-redis/v9"
)

// Limiter 以固定時間窗限制單一群組成員的公開指令次數。
type Limiter struct {
	client *redislib.Client
	limit  int64
	window time.Duration
}

// NewLimiter 建立公開指令頻率限制器。
func NewLimiter(client *redislib.Client, limit int64, window time.Duration) (*Limiter, error) {
	if client == nil || limit <= 0 || window <= 0 {
		return nil, errors.New("redis client、限制次數與時間窗不得為空")
	}
	return &Limiter{client: client, limit: limit, window: window}, nil
}

// Allow 原子增加時間窗計數，第一次寫入時設定 TTL，避免狀態無限成長。
func (l *Limiter) Allow(ctx context.Context, chatID, userID int64) (bool, error) {
	key := fmt.Sprintf("tgspam:command:%d:%d", chatID, userID)
	count, err := l.client.Incr(ctx, key).Result()
	if err != nil {
		return false, fmt.Errorf("增加公開指令計數：%w", err)
	}
	if count == 1 {
		if err := l.client.Expire(ctx, key, l.window).Err(); err != nil {
			return false, fmt.Errorf("設定公開指令計數期限：%w", err)
		}
	}
	return count <= l.limit, nil
}
