package redis

import (
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	redislib "github.com/redis/go-redis/v9"
)

func TestLimiterScopesByChatAndUserAndExpires(t *testing.T) {
	t.Parallel()

	server := miniredis.RunT(t)
	client := redislib.NewClient(&redislib.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	limiter, err := NewLimiter(client, 2, 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}

	for attempt := 1; attempt <= 3; attempt++ {
		allowed, err := limiter.Allow(t.Context(), -1001, 7)
		if err != nil {
			t.Fatal(err)
		}
		if allowed != (attempt <= 2) {
			t.Fatalf("第 %d 次 allowed=%v", attempt, allowed)
		}
	}
	allowed, err := limiter.Allow(t.Context(), -1001, 8)
	if err != nil || !allowed {
		t.Fatalf("不同成員應有獨立額度：allowed=%v err=%v", allowed, err)
	}
	allowed, err = limiter.Allow(t.Context(), -1002, 7)
	if err != nil || !allowed {
		t.Fatalf("不同群組應有獨立額度：allowed=%v err=%v", allowed, err)
	}

	server.FastForward(31 * time.Second)
	allowed, err = limiter.Allow(t.Context(), -1001, 7)
	if err != nil || !allowed {
		t.Fatalf("時間窗過期後應恢復額度：allowed=%v err=%v", allowed, err)
	}
}

func TestNewLimiterRejectsInvalidDependencies(t *testing.T) {
	t.Parallel()

	if _, err := NewLimiter(nil, 1, time.Second); err == nil {
		t.Fatal("缺少 Redis client 應失敗")
	}
}
