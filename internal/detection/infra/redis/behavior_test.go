package redis

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	redislib "github.com/redis/go-redis/v9"

	"github.com/vincent119/tg_spam_bot/internal/detection/domain"
)

func newBehaviorTestStore(t *testing.T, opts ...BehaviorOption) (*BehaviorStore, *miniredis.Miniredis) {
	t.Helper()
	server := miniredis.RunT(t)
	client := redislib.NewClient(&redislib.Options{Addr: server.Addr()})
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Error(err)
		}
	})
	store, err := NewBehaviorStore(client, time.Minute, opts...)
	if err != nil {
		t.Fatal(err)
	}
	store.now = func() time.Time { return time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC) }
	return store, server
}

func observeBehavior(t *testing.T, store *BehaviorStore, message domain.Message, fingerprint string) []string {
	t.Helper()
	signals, err := store.Observe(t.Context(), message, fingerprint)
	if err != nil {
		t.Fatal(err)
	}
	return signals
}

func TestBehaviorStoreRepeatWindow(t *testing.T) {
	t.Parallel()
	for _, content := range []string{"带码来干 收米一天1W", "国庆风口来收米 赚1W", "帮我洗洗米 赚九千", "有码的来帮我洗米稳定秒结日挣1W", "有码来洗米 日挣1W", "能帮我洗米的来 日挣1w"} {
		t.Run(content, func(t *testing.T) {
			t.Parallel()
			store, _ := newBehaviorTestStore(t)
			start := store.now()
			for i := range 4 {
				now := start.Add(time.Duration(i) * 9 * time.Minute)
				store.now = func() time.Time { return now }
				signals := observeBehavior(t, store, domain.Message{ChatID: 1, UserID: 2, MessageID: int64(i + 1), Text: content}, "same")
				if slices.Contains(signals, domain.SignalRepeatedContent) != (i >= 2) || slices.Contains(signals, "high_frequency") {
					t.Fatalf("第 %d 則訊號=%v", i+1, signals)
				}
			}
		})
	}
}

func TestBehaviorStoreRepeatIsolation(t *testing.T) {
	t.Parallel()
	for _, field := range []string{"群組", "成員", "內容"} {
		t.Run(field, func(t *testing.T) {
			t.Parallel()
			store, _ := newBehaviorTestStore(t)
			for id := int64(1); id <= 2; id++ {
				observeBehavior(t, store, domain.Message{ChatID: 1, UserID: 2, MessageID: id}, "same")
			}
			message := domain.Message{ChatID: 1, UserID: 2, MessageID: 3}
			fingerprint := "same"
			switch field {
			case "群組":
				message.ChatID = 9
			case "成員":
				message.UserID = 9
			case "內容":
				fingerprint = "different"
			}
			if signals := observeBehavior(t, store, message, fingerprint); slices.Contains(signals, domain.SignalRepeatedContent) {
				t.Fatalf("不同%s不應合併：%v", field, signals)
			}
			if signals := observeBehavior(t, store, domain.Message{ChatID: 1, UserID: 2, MessageID: 4}, "same"); !slices.Contains(signals, domain.SignalRepeatedContent) {
				t.Fatalf("原範圍應達門檻：%v", signals)
			}
		})
	}
}

func TestBehaviorStoreRepeatBoundaryAndExpiry(t *testing.T) {
	t.Parallel()
	store, server := newBehaviorTestStore(t)
	start := store.now()
	for i := range 3 {
		now := start.Add(time.Duration(i) * 15 * time.Minute)
		store.now = func() time.Time { return now }
		if signals := observeBehavior(t, store, domain.Message{ChatID: 1, UserID: 2, MessageID: int64(i + 1)}, "same"); slices.Contains(signals, domain.SignalRepeatedContent) {
			t.Fatalf("左界紀錄應排除：%v", signals)
		}
	}
	key := "spam:repeat:v2:1:2:same"
	if count := store.client.ZCard(t.Context(), key).Val(); count != 2 {
		t.Fatalf("窗口內筆數=%d，預期 2", count)
	}
	if ttl := server.TTL(key); ttl != 30*time.Minute {
		t.Fatalf("重複 TTL=%v", ttl)
	}
	server.FastForward(30 * time.Minute)
	if server.Exists(key) {
		t.Fatal("停止觀測後應回收 key")
	}
}

func TestBehaviorStoreRepeatRetryDoesNotRecount(t *testing.T) {
	t.Parallel()
	store, _ := newBehaviorTestStore(t)
	start := store.now()
	message := domain.Message{ChatID: 1, UserID: 2, MessageID: 1}
	observeBehavior(t, store, message, "same")
	store.now = func() time.Time { return start.Add(9 * time.Minute) }
	for range 3 {
		if signals := observeBehavior(t, store, message, "same"); slices.Contains(signals, domain.SignalRepeatedContent) {
			t.Fatalf("重送不應累計：%v", signals)
		}
	}
	key := "spam:repeat:v2:1:2:same"
	if score := store.client.ZScore(t.Context(), key, "1").Val(); score != float64(start.UnixMilli()) {
		t.Fatalf("重送改變首次 score：%v", score)
	}
	message.MessageID = 2
	observeBehavior(t, store, message, "same")
	message.MessageID = 3
	for range 2 {
		if signals := observeBehavior(t, store, message, "same"); !slices.Contains(signals, domain.SignalRepeatedContent) {
			t.Fatalf("達門檻的訊息重試應保留線索：%v", signals)
		}
	}
	if count := store.client.ZCard(t.Context(), key).Val(); count != 3 {
		t.Fatalf("重送後筆數=%d，預期 3", count)
	}
}

func TestBehaviorStoreRepeatConcurrent(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name       string
		attempts   int
		duplicates bool
		wantHits   int
		wantCount  int64
	}{
		{name: "不同訊息", attempts: 3, wantHits: 1, wantCount: 3},
		{name: "相同訊息重送", attempts: 12, duplicates: true, wantCount: 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			first, server := newBehaviorTestStore(t)
			client := redislib.NewClient(&redislib.Options{Addr: server.Addr()})
			t.Cleanup(func() {
				if err := client.Close(); err != nil {
					t.Error(err)
				}
			})
			second, err := NewBehaviorStore(client, time.Minute)
			if err != nil {
				t.Fatal(err)
			}
			second.now = first.now
			stores := []*BehaviorStore{first, second}
			var wg sync.WaitGroup
			outcomes := make([]bool, tt.attempts)
			failures := make([]error, tt.attempts)
			for i := range outcomes {
				wg.Go(func() {
					id := int64(i + 1)
					if tt.duplicates {
						id = 1
					}
					signals, observeErr := stores[i%2].Observe(t.Context(), domain.Message{ChatID: 1, UserID: 2, MessageID: id}, "same")
					outcomes[i], failures[i] = slices.Contains(signals, domain.SignalRepeatedContent), observeErr
				})
			}
			wg.Wait()
			hits := 0
			for i, hit := range outcomes {
				if failures[i] != nil {
					t.Fatal(failures[i])
				}
				if hit {
					hits++
				}
			}
			if hits != tt.wantHits || first.client.ZCard(t.Context(), "spam:repeat:v2:1:2:same").Val() != tt.wantCount {
				t.Fatalf("並行結果不符：命中=%v，預期=%d，筆數預期=%d", outcomes, tt.wantHits, tt.wantCount)
			}
		})
	}
}

func TestBehaviorStoreRepeatLateRetryAndRequestOrdering(t *testing.T) {
	t.Parallel()
	for _, expiredBy := range []string{"窗口清除", "TTL"} {
		t.Run(expiredBy, func(t *testing.T) {
			store, server := newBehaviorTestStore(t)
			start := store.now()
			message := domain.Message{ChatID: 1, UserID: 2, MessageID: 1}
			observeBehavior(t, store, message, "same")
			if expiredBy == "TTL" {
				server.FastForward(31 * time.Minute)
			}
			late := start.Add(31 * time.Minute)
			store.now = func() time.Time { return late }
			if signals := observeBehavior(t, store, message, "same"); slices.Contains(signals, domain.SignalRepeatedContent) {
				t.Fatalf("過期後應從單則重新觀測：%v", signals)
			}
			if score := store.client.ZScore(t.Context(), "spam:repeat:v2:1:2:same", "1").Val(); score != float64(late.UnixMilli()) {
				t.Fatalf("過期後不應宣稱永久保留首次時間：%v", score)
			}
		})
	}
	t.Run("較晚取樣先提交", func(t *testing.T) {
		store, _ := newBehaviorTestStore(t)
		start := store.now()
		store.now = func() time.Time { return start.Add(time.Second) }
		for id := int64(1); id <= 2; id++ {
			observeBehavior(t, store, domain.Message{ChatID: 1, UserID: 2, MessageID: id}, "same")
		}
		store.now = func() time.Time { return start }
		if signals := observeBehavior(t, store, domain.Message{ChatID: 1, UserID: 2, MessageID: 3}, "same"); !slices.Contains(signals, domain.SignalRepeatedContent) {
			t.Fatalf("應依交易內已提交紀錄計數：%v", signals)
		}
	})
}

func TestBehaviorStoreShortWindowRegression(t *testing.T) {
	t.Parallel()
	store, server := newBehaviorTestStore(t)
	start := store.now()
	for id := int64(1); id <= 5; id++ {
		signals := observeBehavior(t, store, domain.Message{ChatID: 1, UserID: 2, MessageID: id}, strconv.FormatInt(id, 10))
		if slices.Contains(signals, "high_frequency") != (id == 5) {
			t.Fatalf("第 %d 則頻率訊號=%v", id, signals)
		}
	}
	store.now = func() time.Time { return start.Add(2 * time.Minute) }
	if signals := observeBehavior(t, store, domain.Message{ChatID: 1, UserID: 2, MessageID: 6}, "6"); slices.Contains(signals, "high_frequency") {
		t.Fatalf("長重複窗口不應延長頻率：%v", signals)
	}
	for userID := int64(1); userID <= 3; userID++ {
		signals := observeBehavior(t, store, domain.Message{ChatID: 2, UserID: userID, MessageID: userID}, "shared")
		if slices.Contains(signals, "coordinated_content") != (userID == 3) {
			t.Fatalf("協同門檻回歸：%v", signals)
		}
	}
	server.FastForward(2 * time.Minute)
	store.now = func() time.Time { return start.Add(4 * time.Minute) }
	if signals := observeBehavior(t, store, domain.Message{ChatID: 2, UserID: 4, MessageID: 4}, "shared"); slices.Contains(signals, "coordinated_content") {
		t.Fatalf("長重複窗口不應延長協同 TTL：%v", signals)
	}
}

func TestNewBehaviorStoreRepeatPolicy(t *testing.T) {
	t.Parallel()
	store, _ := newBehaviorTestStore(t)
	tests := []struct {
		name   string
		window time.Duration
		count  int
		valid  bool
	}{
		{name: "下界", window: time.Millisecond, count: 2, valid: true},
		{name: "上界", window: 24 * time.Hour, count: 100, valid: true},
		{name: "零窗口", count: 3},
		{name: "負窗口", window: -time.Second, count: 3},
		{name: "精度不足", window: time.Microsecond, count: 3},
		{name: "窗口過長", window: 25 * time.Hour, count: 3},
		{name: "門檻過低", window: time.Minute, count: 1},
		{name: "門檻過高", window: time.Minute, count: 101},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewBehaviorStore(store.client, time.Minute, WithRepeatPolicy(tt.window, tt.count))
			if (err == nil) != tt.valid {
				t.Fatalf("合法=%v，錯誤=%v", tt.valid, err)
			}
		})
	}
	if _, err := NewBehaviorStore(nil, time.Minute); err == nil {
		t.Fatal("應拒絕缺少 client")
	}
	if _, err := NewBehaviorStore(store.client, 0); err == nil {
		t.Fatal("應拒絕零短窗口")
	}
	if _, err := NewBehaviorStore(store.client, time.Minute, nil); err == nil {
		t.Fatal("應拒絕空 option")
	}
	t.Run("自訂政策實際生效", func(t *testing.T) {
		custom, server := newBehaviorTestStore(t, WithRepeatPolicy(1500*time.Millisecond, 2))
		for id := int64(1); id <= 2; id++ {
			signals := observeBehavior(t, custom, domain.Message{ChatID: 1, UserID: 2, MessageID: id}, "same")
			if slices.Contains(signals, domain.SignalRepeatedContent) != (id == 2) {
				t.Fatalf("自訂門檻無效：%v", signals)
			}
		}
		if ttl := server.TTL("spam:repeat:v2:1:2:same"); ttl != 1500*time.Millisecond {
			t.Fatalf("TTL 不應截斷毫秒：%v", ttl)
		}
	})
}

func TestBehaviorStoreObserveError(t *testing.T) {
	t.Parallel()
	t.Run("context 取消", func(t *testing.T) {
		store, _ := newBehaviorTestStore(t)
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		if signals, err := store.Observe(ctx, domain.Message{}, "same"); err == nil || len(signals) != 0 {
			t.Fatalf("取消應回傳錯誤與空訊號：%v %v", signals, err)
		}
	})
	t.Run("資料型別錯誤", func(t *testing.T) {
		store, server := newBehaviorTestStore(t)
		if err := server.Set("spam:repeat:v2:1:2:same", "invalid"); err != nil {
			t.Fatal(err)
		}
		if signals, err := store.Observe(t.Context(), domain.Message{ChatID: 1, UserID: 2, MessageID: 1}, "same"); err == nil || len(signals) != 0 {
			t.Fatalf("Redis 失敗不可假造正常結果：%v %v", signals, err)
		}
	})
}

func TestBehaviorStoreRepeatRetention(t *testing.T) {
	t.Parallel()
	store, server := newBehaviorTestStore(t)
	start := store.now()
	legacyKey := "spam:repeat:1:2:same"
	if _, err := server.ZAdd(legacyKey, float64(start.UnixMilli()), "legacy:1"); err != nil {
		t.Fatal(err)
	}
	const total = 120
	for i := range total {
		now := start.Add(time.Duration(i) * time.Minute)
		store.now = func() time.Time { return now }
		observeBehavior(t, store, domain.Message{ChatID: 1, UserID: 2, MessageID: int64(i + 1), Text: "不可儲存的原文"}, "same")
	}
	if count := store.client.ZCard(t.Context(), "spam:repeat:v2:1:2:same").Val(); count != 30 {
		t.Fatalf("120 則合成流量後應只保留 30 則：%d", count)
	}
	if !server.Exists(legacyKey) {
		t.Fatal("不得刪除舊 namespace")
	}
	members, err := store.client.ZRange(t.Context(), "spam:repeat:v2:1:2:same", 0, -1).Result()
	if err != nil {
		t.Fatal(err)
	}
	for i, member := range members {
		if member != fmt.Sprint(total-29+i) {
			t.Fatalf("member 應僅包含穩定訊息 ID：%q", member)
		}
	}
	t.Logf("合成觀測 %d 則，30 分鐘窗口保留 %d 個穩定 ID，TTL=%v", total, len(members), server.TTL("spam:repeat:v2:1:2:same"))
}
