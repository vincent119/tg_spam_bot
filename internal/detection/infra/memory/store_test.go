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
	for range 5 {
		_, err := store.Observe(context.Background(), message, "hash")
		if err != nil {
			t.Fatal(err)
		}
	}
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
