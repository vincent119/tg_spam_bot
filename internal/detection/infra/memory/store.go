// Package memory 提供單程序開發與測試用的有界狀態儲存器。
package memory

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/vincent119/tg_spam_bot/internal/detection/application"
	"github.com/vincent119/tg_spam_bot/internal/detection/domain"
)

// Store 提供單程序開發與測試使用的有界狀態儲存器。
type Store struct {
	mu         sync.Mutex
	updates    map[int64]bool
	trusted    map[string]string
	events     []application.Event
	violations map[string][]time.Time
	actions    map[string]application.ActionResult
	windows    map[string][]observation
	joined     map[string]time.Time
	now        func() time.Time
	window     time.Duration
	maxEntries int
}

type observation struct {
	at          time.Time
	userID      int64
	fingerprint string
}

// NewStore 建立具有時間窗與容量上限的記憶體儲存器。
func NewStore(window time.Duration, maxEntries int) *Store {
	return &Store{
		updates: make(map[int64]bool), trusted: make(map[string]string),
		violations: make(map[string][]time.Time), actions: make(map[string]application.ActionResult),
		windows: make(map[string][]observation), joined: make(map[string]time.Time),
		now: time.Now, window: window, maxEntries: maxEntries,
	}
}

// Claim 原子占用 update_id，防止同程序重複處理。
func (s *Store) Claim(_ context.Context, updateID int64) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.updates[updateID]; ok {
		return false, nil
	}
	s.updates[updateID] = false
	return true, nil
}

// Complete 將更新標記為完整處理完成。
func (s *Store) Complete(_ context.Context, updateID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.updates[updateID]; !ok {
		return fmt.Errorf("update %d was not claimed", updateID)
	}
	s.updates[updateID] = true
	return nil
}

// Release 釋放失敗處理的占用，允許後續安全重試。
func (s *Store) Release(_ context.Context, updateID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if done := s.updates[updateID]; !done {
		delete(s.updates, updateID)
	}
	return nil
}

// IsExempt 查詢群組範圍的可信任成員設定。
func (s *Store) IsExempt(_ context.Context, chatID, userID int64) (bool, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	reason, ok := s.trusted[key(chatID, userID)]
	return ok, reason, nil
}

// Trust 新增僅供目前程序使用的可信任成員。
func (s *Store) Trust(chatID, userID int64, reason string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.trusted[key(chatID, userID)] = reason
}

// Observe 計算有界時間窗內的頻率、重複及跨帳號訊號。
func (s *Store) Observe(_ context.Context, message domain.Message, fingerprint string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	chatKey := fmt.Sprintf("%d", message.ChatID)
	cutoff := now.Add(-s.window)
	items := s.windows[chatKey][:0]
	for _, item := range s.windows[chatKey] {
		if !item.at.Before(cutoff) {
			items = append(items, item)
		}
	}
	var sameUser, sameContent, distinctUsers int
	users := make(map[int64]struct{})
	for _, item := range items {
		if item.userID == message.UserID {
			sameUser++
			if item.fingerprint == fingerprint {
				sameContent++
			}
		}
		if item.fingerprint == fingerprint {
			users[item.userID] = struct{}{}
		}
	}
	distinctUsers = len(users)
	items = append(items, observation{at: now, userID: message.UserID, fingerprint: fingerprint})
	if s.maxEntries > 0 && len(items) > s.maxEntries {
		items = items[len(items)-s.maxEntries:]
	}
	s.windows[chatKey] = items
	var signals []string
	if sameUser >= 4 {
		signals = append(signals, "high_frequency")
	}
	if sameContent >= 1 {
		signals = append(signals, "repeated_content")
	}
	if distinctUsers >= 2 {
		signals = append(signals, "coordinated_content")
	}
	joinedAt := message.JoinedAt
	if joinedAt.IsZero() {
		joinedAt = s.joined[key(message.ChatID, message.UserID)]
	}
	if !joinedAt.IsZero() && now.Sub(joinedAt) <= 10*time.Minute {
		signals = append(signals, "new_member_link")
	}
	return signals, nil
}

// RecordJoin 保存 Bot 實際觀測到的入群時間。
func (s *Store) RecordJoin(chatID, userID int64, at time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.joined[key(chatID, userID)] = at
}

// RecordDetection 保存不會推進違規階梯的觀測結果。
func (s *Store) RecordDetection(_ context.Context, event application.Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, event)
	return nil
}

// Create 原子建立有效違規並依次數產生冪等處置計畫。
func (s *Store) Create(_ context.Context, event application.Event) (int, []application.EnforcementAction, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, event)
	if event.Mode == application.ModeDeleteOnly {
		kinds := application.PlanActions(event.Result, event.Mode, 0)
		actions := make([]application.EnforcementAction, 0, len(kinds))
		for _, kind := range kinds {
			actions = append(actions, application.EnforcementAction{Key: event.ID + ":" + string(kind), Kind: kind})
		}
		return 0, actions, nil
	}
	violationKey := key(event.Message.ChatID, event.Message.UserID)
	cutoff := event.CreatedAt.Add(-30 * 24 * time.Hour)
	valid := s.violations[violationKey][:0]
	for _, at := range s.violations[violationKey] {
		if !at.Before(cutoff) {
			valid = append(valid, at)
		}
	}
	valid = append(valid, event.CreatedAt)
	s.violations[violationKey] = valid
	kinds := application.PlanActions(event.Result, event.Mode, len(valid))
	actions := make([]application.EnforcementAction, 0, len(kinds))
	for _, kind := range kinds {
		actions = append(actions, application.EnforcementAction{Key: event.ID + ":" + string(kind), Kind: kind})
	}
	return len(valid), actions, nil
}

// CompleteAction 保存單項處置結果，避免部分失敗無法追蹤。
func (s *Store) CompleteAction(_ context.Context, actionKey string, result application.ActionResult) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.actions[actionKey] = result
	return nil
}

func key(chatID, userID int64) string { return fmt.Sprintf("%d:%d", chatID, userID) }
