// Package redis 提供跨程序共享的短期垃圾訊息行為狀態。
package redis

import (
	"context"
	"fmt"
	"strconv"
	"time"

	redislib "github.com/redis/go-redis/v9"
	"github.com/vincent119/tg_spam_bot/internal/detection/domain"
)

// BehaviorStore 以 Redis 共享多副本的短期頻率與內容協同狀態。
type BehaviorStore struct {
	client *redislib.Client
	window time.Duration
	now    func() time.Time
}

// NewBehaviorStore 建立具有固定時間窗的行為訊號儲存器。
func NewBehaviorStore(client *redislib.Client, window time.Duration) (*BehaviorStore, error) {
	if client == nil || window <= 0 {
		return nil, fmt.Errorf("redis client and positive window are required")
	}
	return &BehaviorStore{client: client, window: window, now: time.Now}, nil
}

// Observe 原子更新時間窗資料並回傳達到門檻的行為訊號。
func (s *BehaviorStore) Observe(ctx context.Context, message domain.Message, fingerprint string) ([]string, error) {
	now := s.now()
	start := now.Add(-s.window).UnixMilli()
	member := fmt.Sprintf("%d:%d", now.UnixNano(), message.MessageID)
	userKey := fmt.Sprintf("spam:frequency:%d:%d", message.ChatID, message.UserID)
	repeatKey := fmt.Sprintf("spam:repeat:%d:%d:%s", message.ChatID, message.UserID, fingerprint)
	usersKey := fmt.Sprintf("spam:content-users:%d:%s", message.ChatID, fingerprint)

	pipe := s.client.TxPipeline()
	pipe.ZRemRangeByScore(ctx, userKey, "-inf", strconv.FormatInt(start, 10))
	pipe.ZRemRangeByScore(ctx, repeatKey, "-inf", strconv.FormatInt(start, 10))
	frequency := pipe.ZCard(ctx, userKey)
	repeats := pipe.ZCard(ctx, repeatKey)
	pipe.ZAdd(ctx, userKey, redislib.Z{Score: float64(now.UnixMilli()), Member: member})
	pipe.ZAdd(ctx, repeatKey, redislib.Z{Score: float64(now.UnixMilli()), Member: member})
	pipe.SAdd(ctx, usersKey, message.UserID)
	distinct := pipe.SCard(ctx, usersKey)
	pipe.Expire(ctx, userKey, s.window)
	pipe.Expire(ctx, repeatKey, s.window)
	pipe.Expire(ctx, usersKey, s.window)
	if _, err := pipe.Exec(ctx); err != nil {
		return nil, fmt.Errorf("record redis behavior: %w", err)
	}
	var signals []string
	if frequency.Val() >= 4 {
		signals = append(signals, "high_frequency")
	}
	if repeats.Val() >= 1 {
		signals = append(signals, "repeated_content")
	}
	if distinct.Val() >= 3 {
		signals = append(signals, "coordinated_content")
	}
	return signals, nil
}
