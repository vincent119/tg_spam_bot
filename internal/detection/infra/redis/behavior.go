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
	client          *redislib.Client
	window          time.Duration
	repeatWindow    time.Duration
	repeatThreshold int
	now             func() time.Time
}

type behaviorOptions struct {
	repeatWindow    time.Duration
	repeatThreshold int
}

// BehaviorOption 調整獨立重複政策，不影響既有短窗口。
type BehaviorOption func(*behaviorOptions)

// WithRepeatPolicy 設定包含當前唯一訊息的重複門檻與保留時間。
func WithRepeatPolicy(window time.Duration, threshold int) BehaviorOption {
	return func(options *behaviorOptions) {
		options.repeatWindow = window
		options.repeatThreshold = threshold
	}
}

// NewBehaviorStore 建立具有固定時間窗的行為訊號儲存器。
func NewBehaviorStore(client *redislib.Client, window time.Duration, opts ...BehaviorOption) (*BehaviorStore, error) {
	if client == nil || window <= 0 {
		return nil, fmt.Errorf("redis client and positive window are required")
	}
	options := behaviorOptions{repeatWindow: 30 * time.Minute, repeatThreshold: 3}
	for _, opt := range opts {
		if opt == nil {
			return nil, fmt.Errorf("behavior option: 不得為 nil")
		}
		opt(&options)
	}
	if options.repeatWindow < time.Millisecond || options.repeatWindow > 24*time.Hour {
		return nil, fmt.Errorf("repeat window: 必須介於 1ms 與 24h")
	}
	if options.repeatThreshold < 2 || options.repeatThreshold > 100 {
		return nil, fmt.Errorf("repeat threshold: 必須介於 2 與 100")
	}
	return &BehaviorStore{
		client: client, window: window, now: time.Now,
		repeatWindow: options.repeatWindow, repeatThreshold: options.repeatThreshold,
	}, nil
}

// Observe 原子更新時間窗資料並回傳達到門檻的行為訊號。
func (s *BehaviorStore) Observe(ctx context.Context, message domain.Message, fingerprint string) ([]string, error) {
	now := s.now()
	start := now.Add(-s.window).UnixMilli()
	repeatStart := now.Add(-s.repeatWindow).UnixMilli()
	member := fmt.Sprintf("%d:%d", now.UnixNano(), message.MessageID)
	userKey := fmt.Sprintf("spam:frequency:%d:%d", message.ChatID, message.UserID)
	repeatKey := fmt.Sprintf("spam:repeat:v2:%d:%d:%s", message.ChatID, message.UserID, fingerprint)
	usersKey := fmt.Sprintf("spam:content-users:%d:%s", message.ChatID, fingerprint)

	pipe := s.client.TxPipeline()
	pipe.ZRemRangeByScore(ctx, userKey, "-inf", strconv.FormatInt(start, 10))
	pipe.ZRemRangeByScore(ctx, repeatKey, "-inf", strconv.FormatInt(repeatStart, 10))
	frequency := pipe.ZCard(ctx, userKey)
	pipe.ZAdd(ctx, userKey, redislib.Z{Score: float64(now.UnixMilli()), Member: member})
	// NX 保留首次觀測時間，避免下游失敗重送被當成另一則文案。
	pipe.ZAddNX(ctx, repeatKey, redislib.Z{Score: float64(now.UnixMilli()), Member: strconv.FormatInt(message.MessageID, 10)})
	repeats := pipe.ZCard(ctx, repeatKey)
	pipe.SAdd(ctx, usersKey, message.UserID)
	distinct := pipe.SCard(ctx, usersKey)
	pipe.Expire(ctx, userKey, s.window)
	pipe.PExpire(ctx, repeatKey, s.repeatWindow)
	pipe.Expire(ctx, usersKey, s.window)
	if _, err := pipe.Exec(ctx); err != nil {
		return nil, fmt.Errorf("record redis behavior: %w", err)
	}
	var signals []string
	if frequency.Val() >= 4 {
		signals = append(signals, "high_frequency")
	}
	if repeats.Val() >= int64(s.repeatThreshold) {
		signals = append(signals, domain.SignalRepeatedContent)
	}
	if distinct.Val() >= 3 {
		signals = append(signals, "coordinated_content")
	}
	return signals, nil
}
