package memory

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/vincent119/tg_spam_bot/internal/detection/domain"
)

func TestStoreObserve(t *testing.T) {
	t.Parallel()
	store := NewStore(time.Minute, 100)
	now := time.Date(2026, 7, 17, 0, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }
	message := domain.Message{ChatID: 1, UserID: 2}
	for id := int64(1); id <= 5; id++ {
		message.MessageID = id
		_, err := store.Observe(context.Background(), message, "hash")
		if err != nil {
			t.Fatal(err)
		}
	}
	message.MessageID = 6
	signals, err := store.Observe(context.Background(), message, "hash")
	if err != nil {
		t.Fatal(err)
	}
	if len(signals) < 2 {
		t.Fatalf("signals = %v, want frequency and repeat", signals)
	}
}

func TestStoreCoordinatedContentAndExpiry(t *testing.T) {
	t.Parallel()
	store := NewStore(time.Minute, 100)
	now := time.Date(2026, 7, 17, 0, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }
	for userID := int64(1); userID <= 3; userID++ {
		_, err := store.Observe(context.Background(), domain.Message{ChatID: 1, UserID: userID, MessageID: userID}, "same")
		if err != nil {
			t.Fatal(err)
		}
	}
	signals, _ := store.Observe(context.Background(), domain.Message{ChatID: 1, UserID: 4, MessageID: 4}, "same")
	if !slices.Contains(signals, "coordinated_content") {
		t.Fatalf("signals = %v", signals)
	}
	now = now.Add(2 * time.Minute)
	signals, _ = store.Observe(context.Background(), domain.Message{ChatID: 1, UserID: 4, MessageID: 5}, "same")
	if slices.Contains(signals, "coordinated_content") || slices.Contains(signals, "repeated_content") {
		t.Fatalf("expired signals = %v", signals)
	}
}

func TestTrustedMember(t *testing.T) {
	t.Parallel()
	store := NewStore(time.Minute, 100)
	store.Trust(1, 2, "moderator")
	exempt, reason, err := store.IsExempt(context.Background(), 1, 2)
	if err != nil || !exempt || reason != "moderator" {
		t.Fatalf("exempt = %v reason = %q err = %v", exempt, reason, err)
	}
}

func TestClaimIsAtomic(t *testing.T) {
	t.Parallel()
	store := NewStore(time.Minute, 100)
	claimed, _ := store.Claim(context.Background(), 1)
	duplicate, _ := store.Claim(context.Background(), 1)
	if !claimed || duplicate {
		t.Fatalf("claimed = %v duplicate = %v", claimed, duplicate)
	}
}

func newRepeatTestStore(t *testing.T, capacity int) (*Store, *time.Time) {
	t.Helper()
	now := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	store := NewStore(time.Minute, capacity)
	store.now = func() time.Time { return now }
	return store, &now
}

func observeMemory(t *testing.T, store *Store, message domain.Message, fingerprint string) []string {
	t.Helper()
	signals, err := store.Observe(t.Context(), message, fingerprint)
	if err != nil {
		t.Fatal(err)
	}
	return signals
}

func TestStoreRepeatWindow(t *testing.T) {
	t.Parallel()
	store, now := newRepeatTestStore(t, 100)
	for id := int64(1); id <= 4; id++ {
		signals := observeMemory(t, store, domain.Message{ChatID: 1, UserID: 2, MessageID: id}, "same")
		if slices.Contains(signals, domain.SignalRepeatedContent) != (id >= 3) || slices.Contains(signals, "high_frequency") {
			t.Fatalf("第 %d 則訊號=%v", id, signals)
		}
		*now = now.Add(9 * time.Minute)
	}
}

func TestStoreRepeatIsolation(t *testing.T) {
	t.Parallel()
	for _, field := range []string{"群組", "成員", "內容"} {
		t.Run(field, func(t *testing.T) {
			store, _ := newRepeatTestStore(t, 100)
			for id := int64(1); id <= 2; id++ {
				observeMemory(t, store, domain.Message{ChatID: 1, UserID: 2, MessageID: id}, "same")
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
			if signals := observeMemory(t, store, message, fingerprint); slices.Contains(signals, domain.SignalRepeatedContent) {
				t.Fatalf("不同%s不應合併：%v", field, signals)
			}
		})
	}
}

func TestStoreRepeatBoundaryAndExpiry(t *testing.T) {
	t.Parallel()
	store, now := newRepeatTestStore(t, 100)
	for id := int64(1); id <= 3; id++ {
		if signals := observeMemory(t, store, domain.Message{ChatID: 1, UserID: 2, MessageID: id}, "same"); slices.Contains(signals, domain.SignalRepeatedContent) {
			t.Fatalf("第 %d 則不應包含已過左界的紀錄：%v", id, signals)
		}
		*now = now.Add(15 * time.Minute)
	}
	if len(store.repeats[1].items) != 2 {
		t.Fatal("左界應已移除第一筆")
	}
	*now = now.Add(31 * time.Minute)
	observeMemory(t, store, domain.Message{ChatID: 1, UserID: 2, MessageID: 4}, "same")
	if len(store.repeats[1].items) != 1 {
		t.Fatal("過期後應只保留本次觀測")
	}
}

func TestStoreRepeatRetryDoesNotRecount(t *testing.T) {
	t.Parallel()
	store, now := newRepeatTestStore(t, 100)
	start := *now
	message := domain.Message{ChatID: 1, UserID: 2, MessageID: 1}
	observeMemory(t, store, message, "same")
	*now = now.Add(9 * time.Minute)
	for range 3 {
		if signals := observeMemory(t, store, message, "same"); slices.Contains(signals, domain.SignalRepeatedContent) {
			t.Fatalf("重送不應累計：%v", signals)
		}
	}
	if state := store.repeats[1]; len(state.items) != 1 || !state.items[0].at.Equal(start) {
		t.Fatalf("重送不應刷新首次觀測：%+v", state)
	}
	message.MessageID = 2
	observeMemory(t, store, message, "same")
	message.MessageID = 3
	for range 2 {
		if signals := observeMemory(t, store, message, "same"); !slices.Contains(signals, domain.SignalRepeatedContent) {
			t.Fatalf("第三則重試應保留線索：%v", signals)
		}
	}
	if len(store.repeats[1].items) != 3 {
		t.Fatal("重送後應只有三則")
	}
}

func TestStoreRepeatLateRetry(t *testing.T) {
	t.Parallel()
	store, now := newRepeatTestStore(t, 100)
	message := domain.Message{ChatID: 1, UserID: 2, MessageID: 1}
	observeMemory(t, store, message, "same")
	*now = now.Add(31 * time.Minute)
	if signals := observeMemory(t, store, message, "same"); slices.Contains(signals, domain.SignalRepeatedContent) {
		t.Fatalf("過期後單則重試不應達門檻：%v", signals)
	}
	if state := store.repeats[1]; len(state.items) != 1 || !state.items[0].at.Equal(*now) {
		t.Fatalf("過期後是新的觀測，非永久去重：%+v", state)
	}
}

func TestStoreRepeatCapacityDegradesSafely(t *testing.T) {
	t.Parallel()
	store, now := newRepeatTestStore(t, 5)
	for id := int64(1); id <= 6; id++ {
		signals := observeMemory(t, store, domain.Message{ChatID: 1, UserID: id, MessageID: id}, "same")
		if slices.Contains(signals, domain.SignalRepeatedContent) {
			t.Fatalf("不同成員及容量壓力不應觸發重複：%v", signals)
		}
	}
	deadline := now.Add(30 * time.Minute)
	for minute := range 30 {
		for attempt := range 5 {
			signals := observeMemory(t, store, domain.Message{ChatID: 1, UserID: 6, MessageID: 6}, "same")
			if slices.Contains(signals, domain.SignalRepeatedContent) {
				t.Fatalf("冷卻第 %d 分鐘不應產生重複訊號", minute)
			}
			if attempt == 4 && !slices.Contains(signals, "high_frequency") {
				t.Fatal("重複冷卻不應停用既有短窗口頻率")
			}
		}
		if state := store.repeats[1]; !state.cooldownUntil.Equal(deadline) || len(state.items) != 0 {
			t.Fatalf("持續流量不應續冷卻或保存歷史：%+v", state)
		}
		*now = now.Add(time.Minute)
	}
	for id := int64(6); id <= 8; id++ {
		signals := observeMemory(t, store, domain.Message{ChatID: 1, UserID: 6, MessageID: id}, "same")
		if slices.Contains(signals, domain.SignalRepeatedContent) != (id == 8) {
			t.Fatalf("冷卻截止後應從第一則重新累計：%v", signals)
		}
	}
	if state := store.repeats[1]; !state.cooldownUntil.IsZero() || len(state.items) != 3 {
		t.Fatalf("冷卻後狀態=%+v", state)
	}
}

func TestStoreDetailedRepeatSnapshot(t *testing.T) {
	t.Parallel()
	store, now := newRepeatTestStore(t, 120)
	for i, id := range []int64{11, 12, 13} {
		observation, err := store.ObserveDetailed(t.Context(), domain.Message{ChatID: 1, UserID: 2, MessageID: id}, "same")
		if err != nil {
			t.Fatal(err)
		}
		snapshot := observation.Repeat
		if !snapshot.Available || !snapshot.CurrentObserved || snapshot.Count != i+1 ||
			len(snapshot.MessageIDs) != i+1 || snapshot.Truncated || !snapshot.ObservedAt.Equal(*now) {
			t.Fatalf("第 %d 則快照=%+v", i+1, snapshot)
		}
		if slices.Contains(observation.Signals, domain.SignalRepeatedContent) != (i == 2) {
			t.Fatalf("第 %d 則訊號=%v", i+1, observation.Signals)
		}
		*now = now.Add(9 * time.Minute)
	}
	peek, err := store.PeekRepeat(t.Context(), domain.Message{ChatID: 1, UserID: 2, MessageID: 12}, "same")
	if err != nil || peek.Count != 3 || !peek.CurrentObserved || len(peek.MessageIDs) != 3 {
		t.Fatalf("唯讀快照=%+v %v", peek, err)
	}
	peek.MessageIDs[0] = 999
	again, err := store.PeekRepeat(t.Context(), domain.Message{ChatID: 1, UserID: 2, MessageID: 12}, "same")
	if err != nil || slices.Contains(again.MessageIDs, 999) {
		t.Fatalf("回傳 ID 不得別名共享：%+v %v", again, err)
	}
	missing, err := store.PeekRepeat(t.Context(), domain.Message{ChatID: 1, UserID: 2, MessageID: 99}, "same")
	if err != nil || missing.Count != 3 || missing.CurrentObserved {
		t.Fatalf("未觀測目標不得重複計數：%+v %v", missing, err)
	}
}

func TestStorePeekRepeatReadOnlyWindowAndIsolation(t *testing.T) {
	t.Parallel()
	store, now := newRepeatTestStore(t, 100)
	message := domain.Message{ChatID: 1, UserID: 2, MessageID: 11}
	observeMemory(t, store, message, "same")
	for _, tt := range []struct {
		name        string
		message     domain.Message
		fingerprint string
	}{
		{name: "其他群組", message: domain.Message{ChatID: 2, UserID: 2, MessageID: 11}, fingerprint: "same"},
		{name: "其他成員", message: domain.Message{ChatID: 1, UserID: 3, MessageID: 11}, fingerprint: "same"},
		{name: "其他指紋", message: message, fingerprint: "other"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			snapshot, err := store.PeekRepeat(t.Context(), tt.message, tt.fingerprint)
			if err != nil || !snapshot.Available || snapshot.Count != 0 || snapshot.CurrentObserved {
				t.Fatalf("隔離快照=%+v %v", snapshot, err)
			}
		})
	}
	*now = now.Add(30 * time.Minute)
	boundary, err := store.PeekRepeat(t.Context(), message, "same")
	if err != nil || boundary.Count != 0 || boundary.CurrentObserved || len(store.repeats[1].items) != 1 {
		t.Fatalf("左界應排除且唯讀不得清理狀態：%+v %v", boundary, err)
	}
}

func TestStoreDetailedRepeatSnapshotTruncationAndCooldown(t *testing.T) {
	t.Parallel()
	t.Run("截斷仍包含當前", func(t *testing.T) {
		store, _ := newRepeatTestStore(t, 120)
		for id := int64(1); id <= 100; id++ {
			observeMemory(t, store, domain.Message{ChatID: 1, UserID: 2, MessageID: id}, "same")
		}
		observation, err := store.ObserveDetailed(t.Context(), domain.Message{ChatID: 1, UserID: 2, MessageID: 101}, "same")
		if err != nil {
			t.Fatal(err)
		}
		snapshot := observation.Repeat
		if snapshot.Count != 101 || len(snapshot.MessageIDs) != maxRepeatCandidates || !snapshot.Truncated || !snapshot.CurrentObserved || !slices.Contains(snapshot.MessageIDs, 101) {
			t.Fatalf("截斷快照=%+v", snapshot)
		}
	})
	t.Run("容量冷卻資料不可用", func(t *testing.T) {
		store, _ := newRepeatTestStore(t, 2)
		for id := int64(1); id <= 2; id++ {
			observeMemory(t, store, domain.Message{ChatID: 1, UserID: 2, MessageID: id}, "same")
		}
		observation, err := store.ObserveDetailed(t.Context(), domain.Message{ChatID: 1, UserID: 2, MessageID: 3}, "same")
		if err != nil {
			t.Fatal(err)
		}
		if observation.Repeat.Available || observation.Repeat.CurrentObserved || slices.Contains(observation.Signals, domain.SignalRepeatedContent) {
			t.Fatalf("冷卻不可偽裝為可信觀測：%+v", observation)
		}
		peek, err := store.PeekRepeat(t.Context(), domain.Message{ChatID: 1, UserID: 2, MessageID: 3}, "same")
		if err != nil || peek.Available || peek.Count != 0 || len(peek.MessageIDs) != 0 {
			t.Fatalf("冷卻預覽資料應未知：%+v %v", peek, err)
		}
	})
}
