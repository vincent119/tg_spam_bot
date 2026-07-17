package application

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/vincent119/tg_spam_bot/internal/command/domain"
)

type telegramSpy struct {
	admins     map[int64]bool
	messages   []string
	deleted    int64
	restricted int64
	unmuted    int64
	banned     int64
	unbanned   int64
}

func (s *telegramSpy) IsAdmin(_ context.Context, _ int64, userID int64) (bool, error) {
	return s.admins[userID], nil
}

func (s *telegramSpy) SendMessage(_ context.Context, _ int64, _ int64, text string) error {
	s.messages = append(s.messages, text)
	return nil
}

func (s *telegramSpy) DeleteMessage(_ context.Context, _ int64, messageID int64) error {
	s.deleted = messageID
	return nil
}

func (s *telegramSpy) RestrictMember(_ context.Context, _ int64, userID int64, _ time.Time) error {
	s.restricted = userID
	return nil
}

func (s *telegramSpy) UnrestrictMember(_ context.Context, _ int64, userID int64) error {
	s.unmuted = userID
	return nil
}

func (s *telegramSpy) BanMember(_ context.Context, _ int64, userID int64) error {
	s.banned = userID
	return nil
}

func (s *telegramSpy) UnbanMember(_ context.Context, _ int64, userID int64) error {
	s.unbanned = userID
	return nil
}

type trustedStub struct{ ids map[int64]bool }

func (s trustedStub) IsExempt(_ context.Context, _ int64, userID int64) (bool, string, error) {
	return s.ids[userID], "trusted", nil
}

type storeStub struct {
	claimed     bool
	completed   string
	warnings    WarningSummary
	added       int
	clearCount  int64
	completeErr error
}

func (s *storeStub) ClaimCommand(context.Context, domain.Command) (bool, error) {
	return s.claimed, nil
}

func (s *storeStub) CompleteCommand(_ context.Context, _ domain.Command, status, _, _ string) error {
	s.completed = status
	return s.completeErr
}

func (s *storeStub) Warnings(context.Context, int64, int64, time.Time) (WarningSummary, error) {
	return s.warnings, nil
}

func (s *storeStub) AddManualWarning(context.Context, domain.Command, string, time.Time) (WarningSummary, error) {
	s.added++
	return WarningSummary{Total: 2, Manual: 1, Automatic: 1}, nil
}

func (s *storeStub) ClearWarnings(context.Context, domain.Command, string, time.Time) (int64, error) {
	return s.clearCount, nil
}

type limiterStub struct{ allowed bool }

func (s limiterStub) Allow(context.Context, int64, int64) (bool, error) { return s.allowed, nil }

func TestHandlerPublicAndIdempotence(t *testing.T) {
	t.Parallel()
	tg := &telegramSpy{admins: map[int64]bool{1: true}}
	store := &storeStub{claimed: true}
	handler, err := NewHandler(tg, trustedStub{ids: map[int64]bool{}}, store, limiterStub{allowed: true}, 99)
	if err != nil {
		t.Fatal(err)
	}
	command, _ := domain.NewCommand(domain.Command{UpdateID: 1, ChatID: -1001, MessageID: 2, Actor: domain.User{ID: 1}, Name: domain.NameHelp})
	if err := handler.Handle(t.Context(), command); err != nil {
		t.Fatal(err)
	}
	if store.completed != "completed" || len(tg.messages) != 1 || !strings.Contains(tg.messages[0], "/ban") {
		t.Fatalf("未回傳管理員 help：%+v", tg.messages)
	}
	store.claimed = false
	if err := handler.Handle(t.Context(), command); err != nil || len(tg.messages) != 1 {
		t.Fatal("重送不應再次回覆")
	}
}

func TestHandlerPublicRateLimit(t *testing.T) {
	t.Parallel()
	tg := &telegramSpy{admins: map[int64]bool{}}
	store := &storeStub{claimed: true}
	handler, err := NewHandler(tg, trustedStub{ids: map[int64]bool{}}, store, limiterStub{allowed: false}, 99)
	if err != nil {
		t.Fatal(err)
	}
	command, _ := domain.NewCommand(domain.Command{UpdateID: 2, ChatID: -1001, MessageID: 2, Actor: domain.User{ID: 1}, Name: domain.NamePing})
	if err := handler.Handle(t.Context(), command); err != nil {
		t.Fatal(err)
	}
	if store.completed != "rate_limited" || len(tg.messages) != 0 {
		t.Fatalf("公開指令超額未靜默忽略：status=%s messages=%v", store.completed, tg.messages)
	}
}

func TestHandlerAdminCommands(t *testing.T) {
	t.Parallel()
	target := domain.User{ID: 2}
	tests := []struct {
		name       string
		command    domain.Name
		args       string
		admin      bool
		trusted    bool
		wantStatus string
		verify     func(*testing.T, *telegramSpy, *storeStub)
	}{
		{name: "非管理員拒絕", command: domain.NameBan, wantStatus: "denied"},
		{name: "可信任成員拒絕", command: domain.NameBan, admin: true, trusted: true, wantStatus: "denied"},
		{name: "人工警告", command: domain.NameWarn, args: "廣告", admin: true, wantStatus: "completed", verify: func(t *testing.T, _ *telegramSpy, store *storeStub) {
			if store.added != 1 {
				t.Fatal("應新增一次人工警告")
			}
		}},
		{name: "禁言", command: domain.NameMute, args: "10m 洗版", admin: true, wantStatus: "completed", verify: func(t *testing.T, tg *telegramSpy, _ *storeStub) {
			if tg.restricted != 2 {
				t.Fatal("應禁言目標")
			}
		}},
		{name: "錯誤禁言時間", command: domain.NameMute, args: "8d", admin: true, wantStatus: "invalid"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tg := &telegramSpy{admins: map[int64]bool{1: tt.admin}}
			store := &storeStub{claimed: true}
			handler, err := NewHandler(tg, trustedStub{ids: map[int64]bool{2: tt.trusted}}, store, limiterStub{allowed: true}, 99)
			if err != nil {
				t.Fatal(err)
			}
			command, _ := domain.NewCommand(domain.Command{UpdateID: 1, ChatID: -1001, MessageID: 3, Actor: domain.User{ID: 1}, Target: &target, TargetMessage: 2, Name: tt.command, Args: tt.args})
			if err := handler.Handle(t.Context(), command); err != nil {
				t.Fatal(err)
			}
			if store.completed != tt.wantStatus {
				t.Fatalf("狀態 = %q，預期 %q", store.completed, tt.wantStatus)
			}
			if tt.verify != nil {
				tt.verify(t, tg, store)
			}
		})
	}
}

func TestHandlerDependencyFailure(t *testing.T) {
	t.Parallel()
	_, err := NewHandler(nil, trustedStub{}, &storeStub{}, limiterStub{}, 1)
	if err == nil {
		t.Fatal("缺少依賴應失敗")
	}
}
