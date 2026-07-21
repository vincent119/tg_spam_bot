package application

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/vincent119/tg_spam_bot/internal/command/domain"
	detectionapp "github.com/vincent119/tg_spam_bot/internal/detection/application"
)

type telegramSpy struct {
	admins          map[int64]bool
	adminErr        error
	messages        []string
	deleted         int64
	restricted      int64
	restrictedUntil time.Time
	unmuted         int64
	banned          int64
	banErr          error
	unbanned        int64
}

func (s *telegramSpy) IsAdmin(_ context.Context, _ int64, userID int64) (bool, error) {
	return s.admins[userID], s.adminErr
}

func (s *telegramSpy) SendMessage(_ context.Context, _ int64, _ int64, text string) error {
	s.messages = append(s.messages, text)
	return nil
}

func (s *telegramSpy) DeleteMessage(_ context.Context, _ int64, messageID int64) error {
	s.deleted = messageID
	return nil
}

func (s *telegramSpy) RestrictMember(_ context.Context, _ int64, userID int64, until time.Time) error {
	s.restricted = userID
	s.restrictedUntil = until
	return nil
}

func (s *telegramSpy) UnrestrictMember(_ context.Context, _ int64, userID int64) error {
	s.unmuted = userID
	return nil
}

func (s *telegramSpy) BanMember(_ context.Context, _ int64, userID int64) error {
	s.banned = userID
	return s.banErr
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
	claim       domain.Claim
	completed   string
	warnings    WarningSummary
	added       int
	clearCount  int64
	completeErr error
	result      domain.Result
}

func (s *storeStub) ClaimCommand(context.Context, domain.Command) (domain.Claim, error) {
	return s.claim, nil
}

func (s *storeStub) CompleteCommand(_ context.Context, _ domain.Command, result domain.Result) error {
	s.completed = result.Status
	s.result = result
	s.result = result
	return s.completeErr
}

type fixedClock struct{ now time.Time }

func (c fixedClock) Now() time.Time { return c.now }

func TestCommandLogFieldsExcludeSensitiveContent(t *testing.T) {
	t.Parallel()
	command := domain.Command{
		UpdateID: 1,
		ChatID:   -1001,
		Actor:    domain.Actor{ID: 2},
		Target:   &domain.Target{ID: 3},
		Name:     domain.NameWarn,
		Args:     "不得寫入日誌的原因",
	}
	fields := commandLogFields(command, domain.Result{Status: "completed", Message: "不得寫入日誌的回覆"})
	wantKeys := map[string]bool{
		"subsystem": false, "update_id": false, "chat_id": false, "operator_id": false,
		"target_user_id": false, "command": false, "status": false, "error_code": false, "retryable": false,
	}
	for _, field := range fields {
		if field.Key == "args" || field.Key == "reason" || field.Key == "message" || strings.Contains(field.String, "不得寫入日誌") {
			t.Fatalf("敏感欄位進入 command 日誌：%s", field.Key)
		}
		if _, ok := wantKeys[field.Key]; ok {
			wantKeys[field.Key] = true
		}
	}
	for key, found := range wantKeys {
		if !found {
			t.Errorf("缺少 command 日誌欄位 %s", key)
		}
	}
}

func (s *storeStub) Warnings(context.Context, int64, int64, time.Time) (WarningSummary, error) {
	return s.warnings, nil
}

func (s *storeStub) AddManualWarning(context.Context, domain.Command, domain.Reason, time.Time) (WarningSummary, error) {
	s.added++
	return WarningSummary{Total: 2, Manual: 1, Automatic: 1}, nil
}

func (s *storeStub) ClearWarnings(context.Context, domain.Command, domain.Reason, time.Time) (int64, error) {
	return s.clearCount, nil
}

type limiterStub struct{ allowed bool }

func (s limiterStub) Allow(context.Context, int64, int64) (bool, error) { return s.allowed, nil }

type feedSpamSpy struct {
	input detectionapp.ManualFeedInput
	err   error
	calls int
}

func (s *feedSpamSpy) SubmitSpam(_ context.Context, input detectionapp.ManualFeedInput) (detectionapp.ManualSample, bool, error) {
	s.calls++
	s.input = input
	if s.err != nil {
		return detectionapp.ManualSample{}, false, s.err
	}
	return detectionapp.ManualSample{ID: 1, Status: detectionapp.ManualSampleStatusEmbeddingCompleted}, true, nil
}

func TestHandlerPublicAndIdempotence(t *testing.T) {
	t.Parallel()
	tg := &telegramSpy{admins: map[int64]bool{1: true}}
	store := &storeStub{claim: domain.Claim{Acquired: true}}
	handler, err := NewHandler(tg, trustedStub{ids: map[int64]bool{}}, store, limiterStub{allowed: true}, 99)
	if err != nil {
		t.Fatal(err)
	}
	command, _ := domain.NewCommand(domain.Command{UpdateID: 1, ChatID: -1001, MessageID: 2, Actor: domain.Actor{ID: 1}, Name: domain.NameHelp})
	if err := handler.Handle(t.Context(), command); err != nil {
		t.Fatal(err)
	}
	if store.completed != "completed" || len(tg.messages) != 1 || !strings.Contains(tg.messages[0], "/ban") {
		t.Fatalf("未回傳管理員 help：%+v", tg.messages)
	}
	store.claim = domain.Claim{Existing: &domain.Result{Status: "completed"}}
	if err := handler.Handle(t.Context(), command); err != nil || len(tg.messages) != 1 {
		t.Fatal("重送不應再次回覆")
	}
}

func TestHandlerPublicRateLimit(t *testing.T) {
	t.Parallel()
	tg := &telegramSpy{admins: map[int64]bool{}}
	store := &storeStub{claim: domain.Claim{Acquired: true}}
	handler, err := NewHandler(tg, trustedStub{ids: map[int64]bool{}}, store, limiterStub{allowed: false}, 99)
	if err != nil {
		t.Fatal(err)
	}
	command, _ := domain.NewCommand(domain.Command{UpdateID: 2, ChatID: -1001, MessageID: 2, Actor: domain.Actor{ID: 1}, Name: domain.NamePing})
	if err := handler.Handle(t.Context(), command); err != nil {
		t.Fatal(err)
	}
	if store.completed != "rate_limited" || len(tg.messages) != 0 {
		t.Fatalf("公開指令超額未靜默忽略：status=%s messages=%v", store.completed, tg.messages)
	}
}

func TestHandlerPublicCommandsAndUnknown(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		command    domain.Name
		target     *domain.Target
		want       string
		wantStatus string
	}{
		{name: "ping", command: domain.NamePing, want: "機器人運作正常", wantStatus: "completed"},
		{name: "id 操作者", command: domain.NameID, want: "使用者 ID：1", wantStatus: "completed"},
		{name: "id 目標", command: domain.NameID, target: &domain.Target{ID: 2}, want: "使用者 ID：2", wantStatus: "completed"},
		{name: "未知指令", command: domain.Name("unknown"), want: "未知指令", wantStatus: "ignored"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tg := &telegramSpy{admins: map[int64]bool{}}
			store := &storeStub{claim: domain.Claim{Acquired: true}}
			handler, err := NewHandler(tg, trustedStub{}, store, limiterStub{allowed: true}, 99)
			if err != nil {
				t.Fatal(err)
			}
			command, _ := domain.NewCommand(domain.Command{UpdateID: 20, ChatID: -1001, MessageID: 3, Actor: domain.Actor{ID: 1}, Target: tt.target, Name: tt.command})
			if err := handler.Handle(t.Context(), command); err != nil {
				t.Fatal(err)
			}
			if store.completed != tt.wantStatus || len(tg.messages) != 1 || !strings.Contains(tg.messages[0], tt.want) {
				t.Fatalf("status=%q messages=%v", store.completed, tg.messages)
			}
		})
	}
}

func TestHandlerReturnsExistingResultWithoutRepeatingEffect(t *testing.T) {
	t.Parallel()
	tg := &telegramSpy{admins: map[int64]bool{1: true}}
	store := &storeStub{claim: domain.Claim{Existing: &domain.Result{Status: "completed", Message: "已完成舊指令"}}}
	handler, _ := NewHandler(tg, trustedStub{}, store, limiterStub{allowed: true}, 99)
	target := domain.Target{ID: 2}
	command, _ := domain.NewCommand(domain.Command{UpdateID: 21, ChatID: -1001, MessageID: 3, Actor: domain.Actor{ID: 1}, Target: &target, TargetMessage: 2, Name: domain.NameBan})
	if err := handler.Handle(t.Context(), command); err != nil {
		t.Fatal(err)
	}
	if tg.banned != 0 || len(tg.messages) != 1 || tg.messages[0] != "已完成舊指令" {
		t.Fatalf("banned=%d messages=%v", tg.banned, tg.messages)
	}
}

func TestHandlerMuteUsesInjectedUTCClock(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.July, 20, 2, 0, 0, 0, time.UTC)
	tg := &telegramSpy{admins: map[int64]bool{1: true}}
	store := &storeStub{claim: domain.Claim{Acquired: true}}
	handler, err := NewHandler(tg, trustedStub{}, store, limiterStub{allowed: true}, 99, WithClock(fixedClock{now: now}))
	if err != nil {
		t.Fatal(err)
	}
	target := domain.Target{ID: 2}
	command, _ := domain.NewCommand(domain.Command{UpdateID: 22, ChatID: -1001, MessageID: 3, Actor: domain.Actor{ID: 1}, Target: &target, TargetMessage: 2, Name: domain.NameMute, Args: "10m 洗版"})
	if err := handler.Handle(t.Context(), command); err != nil {
		t.Fatal(err)
	}
	if want := now.Add(10 * time.Minute); !tg.restrictedUntil.Equal(want) {
		t.Fatalf("until=%v，預期 %v", tg.restrictedUntil, want)
	}
}

func TestHandlerAdminCommands(t *testing.T) {
	t.Parallel()
	target := domain.Target{ID: 2}
	tests := []struct {
		name        string
		command     domain.Name
		args        string
		admin       bool
		trusted     bool
		targetAdmin bool
		targetBot   bool
		wantStatus  string
		verify      func(*testing.T, *telegramSpy, *storeStub)
	}{
		{name: "非管理員拒絕", command: domain.NameBan, wantStatus: "denied"},
		{name: "權限已撤銷", command: domain.NameMute, args: "10m", wantStatus: "denied"},
		{name: "可信任成員拒絕", command: domain.NameBan, admin: true, trusted: true, wantStatus: "denied"},
		{name: "管理員目標拒絕", command: domain.NameBan, admin: true, targetAdmin: true, wantStatus: "denied"},
		{name: "Bot 目標拒絕", command: domain.NameBan, admin: true, targetBot: true, wantStatus: "denied"},
		{name: "查詢警告", command: domain.NameWarnings, admin: true, wantStatus: "completed", verify: func(t *testing.T, _ *telegramSpy, store *storeStub) {
			if store.completed != "completed" {
				t.Fatal("應完成警告查詢")
			}
		}},
		{name: "人工警告", command: domain.NameWarn, args: "廣告", admin: true, wantStatus: "completed", verify: func(t *testing.T, _ *telegramSpy, store *storeStub) {
			if store.added != 1 {
				t.Fatal("應新增一次人工警告")
			}
		}},
		{name: "清除警告", command: domain.NameClearWarn, args: "誤判", admin: true, wantStatus: "completed", verify: func(t *testing.T, _ *telegramSpy, store *storeStub) {
			if store.clearCount != 3 {
				t.Fatalf("clearCount=%d", store.clearCount)
			}
		}},
		{name: "刪除訊息", command: domain.NameDelete, admin: true, wantStatus: "completed", verify: func(t *testing.T, tg *telegramSpy, _ *storeStub) {
			if tg.deleted != 2 {
				t.Fatalf("deleted=%d", tg.deleted)
			}
		}},
		{name: "禁言", command: domain.NameMute, args: "10m 洗版", admin: true, wantStatus: "completed", verify: func(t *testing.T, tg *telegramSpy, _ *storeStub) {
			if tg.restricted != 2 {
				t.Fatal("應禁言目標")
			}
		}},
		{name: "錯誤禁言時間", command: domain.NameMute, args: "8d", admin: true, wantStatus: "invalid"},
		{name: "解除禁言", command: domain.NameUnmute, admin: true, wantStatus: "completed", verify: func(t *testing.T, tg *telegramSpy, _ *storeStub) {
			if tg.unmuted != 2 {
				t.Fatalf("unmuted=%d", tg.unmuted)
			}
		}},
		{name: "封鎖", command: domain.NameBan, args: "廣告", admin: true, wantStatus: "completed", verify: func(t *testing.T, tg *telegramSpy, _ *storeStub) {
			if tg.banned != 2 {
				t.Fatalf("banned=%d", tg.banned)
			}
		}},
		{name: "解除封鎖", command: domain.NameUnban, admin: true, wantStatus: "completed", verify: func(t *testing.T, tg *telegramSpy, _ *storeStub) {
			if tg.unbanned != 2 {
				t.Fatalf("unbanned=%d", tg.unbanned)
			}
		}},
		{name: "漏網樣本功能未啟用", command: domain.NameFeedSpam, args: "agent_recruiting", admin: true, wantStatus: "failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			admins := map[int64]bool{1: tt.admin, 2: tt.targetAdmin}
			tg := &telegramSpy{admins: admins}
			store := &storeStub{claim: domain.Claim{Acquired: true}, warnings: WarningSummary{Total: 2, Manual: 1, Automatic: 1}, clearCount: 3}
			handler, err := NewHandler(tg, trustedStub{ids: map[int64]bool{2: tt.trusted}}, store, limiterStub{allowed: true}, 99)
			if err != nil {
				t.Fatal(err)
			}
			caseTarget := target
			caseTarget.IsBot = tt.targetBot
			command, _ := domain.NewCommand(domain.Command{UpdateID: 1, ChatID: -1001, MessageID: 3, Actor: domain.Actor{ID: 1}, Target: &caseTarget, TargetMessage: 2, Name: tt.command, Args: tt.args})
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

func TestHandlerFeedSpam(t *testing.T) {
	t.Parallel()

	tg := &telegramSpy{admins: map[int64]bool{1: true}}
	store := &storeStub{claim: domain.Claim{Acquired: true}}
	feedSpam := &feedSpamSpy{}
	handler, err := NewHandler(
		tg, trustedStub{}, store, limiterStub{allowed: true}, 99,
		WithFeedSpamSubmitter(feedSpam, []byte("01234567890123456789012345678901"), 800, time.Hour),
	)
	if err != nil {
		t.Fatal(err)
	}
	target := domain.Target{ID: 2}
	command, _ := domain.NewCommand(domain.Command{
		UpdateID: 30, ChatID: -1001, MessageID: 3, Actor: domain.Actor{ID: 1},
		Target: &target, TargetMessage: 2, TargetText: "抖音禮物項目 @x",
		Name: domain.NameFeedSpam, Args: "agent_recruiting",
	})
	if err := handler.Handle(t.Context(), command); err != nil {
		t.Fatal(err)
	}
	if store.completed != "completed" || feedSpam.calls != 1 || len(tg.messages) != 1 {
		t.Fatalf("status=%s calls=%d messages=%v", store.completed, feedSpam.calls, tg.messages)
	}
	if feedSpam.input.Sample.Category != "agent_recruiting" || feedSpam.input.Text != "抖音禮物項目 @x" || feedSpam.input.Sample.ContentFingerprint == "" {
		t.Fatalf("feedspam input=%+v", feedSpam.input)
	}
	if tg.deleted != 0 || tg.banned != 0 || tg.restricted != 0 {
		t.Fatalf("feedspam 不應處置：deleted=%d banned=%d restricted=%d", tg.deleted, tg.banned, tg.restricted)
	}
}

func TestHandlerFeedSpamValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		args       string
		targetText string
		wantStatus string
	}{
		{name: "分類格式錯誤", args: "中文", targetText: "spam", wantStatus: "invalid"},
		{name: "目標沒有文字", args: "agent_recruiting", wantStatus: "invalid"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tg := &telegramSpy{admins: map[int64]bool{1: true}}
			store := &storeStub{claim: domain.Claim{Acquired: true}}
			handler, err := NewHandler(
				tg, trustedStub{}, store, limiterStub{allowed: true}, 99,
				WithFeedSpamSubmitter(&feedSpamSpy{}, []byte("01234567890123456789012345678901"), 800, time.Hour),
			)
			if err != nil {
				t.Fatal(err)
			}
			target := domain.Target{ID: 2}
			command, _ := domain.NewCommand(domain.Command{
				UpdateID: 31, ChatID: -1001, MessageID: 3, Actor: domain.Actor{ID: 1},
				Target: &target, TargetMessage: 2, TargetText: tt.targetText,
				Name: domain.NameFeedSpam, Args: tt.args,
			})
			if err := handler.Handle(t.Context(), command); err != nil {
				t.Fatal(err)
			}
			if store.completed != tt.wantStatus {
				t.Fatalf("status=%s，預期 %s", store.completed, tt.wantStatus)
			}
		})
	}
}

func TestHandlerUnbanByUserID(t *testing.T) {
	t.Parallel()
	tg := &telegramSpy{admins: map[int64]bool{1: true}}
	store := &storeStub{claim: domain.Claim{Acquired: true}}
	handler, _ := NewHandler(tg, trustedStub{}, store, limiterStub{allowed: true}, 99)
	command, _ := domain.NewCommand(domain.Command{UpdateID: 23, ChatID: -1001, MessageID: 3, Actor: domain.Actor{ID: 1}, Name: domain.NameUnban, Args: "12345"})
	if err := handler.Handle(t.Context(), command); err != nil {
		t.Fatal(err)
	}
	if tg.unbanned != 12345 {
		t.Fatalf("unbanned=%d", tg.unbanned)
	}
}

type classifiedTestError struct {
	code      string
	retryable bool
}

func (e classifiedTestError) Error() string     { return "telegram failed" }
func (e classifiedTestError) ErrorCode() string { return e.code }
func (e classifiedTestError) IsRetryable() bool { return e.retryable }

func TestHandlerPersistsClassifiedFailure(t *testing.T) {
	t.Parallel()
	tg := &telegramSpy{
		admins: map[int64]bool{1: true},
		banErr: classifiedTestError{code: "permission_denied", retryable: false},
	}
	store := &storeStub{claim: domain.Claim{Acquired: true}}
	handler, _ := NewHandler(tg, trustedStub{}, store, limiterStub{allowed: true}, 99)
	target := domain.Target{ID: 2}
	command, _ := domain.NewCommand(domain.Command{UpdateID: 24, ChatID: -1001, MessageID: 3, Actor: domain.Actor{ID: 1}, Target: &target, TargetMessage: 2, Name: domain.NameBan})
	if err := handler.Handle(t.Context(), command); err == nil {
		t.Fatal("處置失敗應回傳錯誤")
	}
	if store.result.Status != "failed" || store.result.ErrorCode != "permission_denied" || store.result.Retryable {
		t.Fatalf("result=%+v", store.result)
	}
}

func TestHandlerPersistsUnclassifiedTemporaryFailure(t *testing.T) {
	t.Parallel()
	tg := &telegramSpy{admins: map[int64]bool{}, adminErr: errors.New("network unavailable")}
	store := &storeStub{claim: domain.Claim{Acquired: true}}
	handler, _ := NewHandler(tg, trustedStub{}, store, limiterStub{allowed: true}, 99)
	target := domain.Target{ID: 2}
	command, _ := domain.NewCommand(domain.Command{UpdateID: 25, ChatID: -1001, MessageID: 3, Actor: domain.Actor{ID: 1}, Target: &target, TargetMessage: 2, Name: domain.NameBan})
	if err := handler.Handle(t.Context(), command); err == nil {
		t.Fatal("授權查詢失敗應回傳錯誤")
	}
	if store.result.ErrorCode != string(domain.ErrorTemporary) || !store.result.Retryable {
		t.Fatalf("result=%+v", store.result)
	}
}

func TestHandlerRejectsUnexpectedArguments(t *testing.T) {
	t.Parallel()
	tests := []domain.Command{
		{UpdateID: 26, ChatID: -1001, MessageID: 3, Actor: domain.Actor{ID: 1}, Target: &domain.Target{ID: 2}, TargetMessage: 2, Name: domain.NameWarnings, Args: "extra"},
		{UpdateID: 27, ChatID: -1001, MessageID: 3, Actor: domain.Actor{ID: 1}, Target: &domain.Target{ID: 2}, TargetMessage: 2, Name: domain.NameUnban, Args: "123"},
	}
	for _, raw := range tests {
		command, _ := domain.NewCommand(raw)
		tg := &telegramSpy{admins: map[int64]bool{1: true}}
		store := &storeStub{claim: domain.Claim{Acquired: true}}
		handler, _ := NewHandler(tg, trustedStub{}, store, limiterStub{allowed: true}, 99)
		if err := handler.Handle(t.Context(), command); err != nil {
			t.Fatal(err)
		}
		if store.completed != "invalid" {
			t.Fatalf("command=%s status=%s", command.Name, store.completed)
		}
	}
}

func TestHandlerDependencyFailure(t *testing.T) {
	t.Parallel()
	_, err := NewHandler(nil, trustedStub{}, &storeStub{}, limiterStub{}, 1)
	if err == nil {
		t.Fatal("缺少依賴應失敗")
	}
	if _, err := NewHandler(&telegramSpy{}, trustedStub{}, &storeStub{}, limiterStub{}, 1, nil); err == nil {
		t.Fatal("nil option 應失敗")
	}
	if _, err := NewHandler(&telegramSpy{}, trustedStub{}, &storeStub{}, limiterStub{}, 1, WithClock(nil)); err == nil {
		t.Fatal("nil clock 應失敗")
	}
}
