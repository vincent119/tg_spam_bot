package application

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/vincent119/tg_spam_bot/internal/command/domain"
	detectionapp "github.com/vincent119/tg_spam_bot/internal/detection/application"
	detectiondomain "github.com/vincent119/tg_spam_bot/internal/detection/domain"
)

type feedbackSubmitterSpy struct {
	input  detectionapp.ManualFeedbackInput
	result detectionapp.ManualFeedbackResult
	err    error
	calls  int
}

func (s *feedbackSubmitterSpy) Submit(_ context.Context, input detectionapp.ManualFeedbackInput) (detectionapp.ManualFeedbackResult, error) {
	s.calls++
	s.input = input
	if s.err != nil {
		return s.result, s.err
	}
	if s.result.EmbeddingStatus == "" {
		s.result = detectionapp.ManualFeedbackResult{Saved: true, EmbeddingStatus: detectionapp.ManualFeedbackEmbeddingCompleted}
	}
	return s.result, nil
}

type feedbackTargetSpy struct {
	target FeedbackTarget
	found  bool
	chatID int64
	event  string
}

func (s *feedbackTargetSpy) FindFeedbackTarget(_ context.Context, chatID int64, eventID string) (FeedbackTarget, bool, error) {
	s.chatID, s.event = chatID, eventID
	return s.target, s.found, nil
}

type feedbackActionSpy struct {
	kinds       []FeedbackActionKind
	completed   []FeedbackActionKind
	planErr     error
	completeErr error
	planned     int
}

func (s *feedbackActionSpy) PlanFeedbackActions(_ context.Context, _ domain.Command, kinds []FeedbackActionKind) error {
	s.planned++
	s.kinds = append([]FeedbackActionKind(nil), kinds...)
	return s.planErr
}

func (s *feedbackActionSpy) CompleteFeedbackAction(_ context.Context, _ domain.Command, kind FeedbackActionKind, _, _ bool, _ string) error {
	s.completed = append(s.completed, kind)
	return s.completeErr
}

func newFeedbackHandler(t *testing.T, telegram *telegramSpy, store *storeStub, feedback *feedbackSubmitterSpy, targets *feedbackTargetSpy, actions *feedbackActionSpy) *Handler {
	t.Helper()
	handler, err := NewHandler(telegram, trustedStub{ids: map[int64]bool{}}, store, limiterStub{allowed: true}, 99,
		WithManualFeedback(feedback, targets, actions, []byte("01234567890123456789012345678901"), 800, time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	return handler
}

func feedbackCommand(name domain.Name, args string) domain.Command {
	command, _ := domain.NewCommand(domain.Command{
		UpdateID: 25, ChatID: -1001, MessageID: 13, Actor: domain.Actor{ID: 1},
		Target: &domain.Target{ID: 2}, TargetMessage: 12, TargetText: "帶碼來收米 日掙1W",
		Name: name, Args: args,
	})
	return command
}

func TestSpamFeedbackActionsAndAuthorization(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		args       string
		admin      bool
		protected  bool
		saveErr    error
		planErr    error
		wantCalls  int
		wantDelete int64
		wantBan    int64
		wantStatus string
	}{
		{name: "預設刪文", admin: true, wantCalls: 1, wantDelete: 12, wantStatus: "completed"},
		{name: "明示封鎖", args: "agent_recruiting ban", admin: true, wantCalls: 1, wantDelete: 12, wantBan: 2, wantStatus: "completed"},
		{name: "非管理員不得標記", wantStatus: "denied"},
		{name: "受保護帳號不得標記", admin: true, protected: true, wantStatus: "denied"},
		{name: "錯誤參數", args: "ad mute", admin: true, wantStatus: "invalid"},
		{name: "樣本儲存失敗不處置", admin: true, saveErr: errors.New("db unavailable"), wantCalls: 1, wantStatus: "failed"},
		{name: "處置計畫失敗不呼叫 Telegram", admin: true, planErr: errors.New("db unavailable"), wantCalls: 1, wantStatus: "partial"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			telegram := &telegramSpy{admins: map[int64]bool{1: tt.admin, 2: tt.protected}}
			store := &storeStub{claim: domain.Claim{Acquired: true}}
			feedback := &feedbackSubmitterSpy{err: tt.saveErr}
			actions := &feedbackActionSpy{planErr: tt.planErr}
			handler := newFeedbackHandler(t, telegram, store, feedback, &feedbackTargetSpy{}, actions)
			_ = handler.Handle(t.Context(), feedbackCommand(domain.NameSpam, tt.args))
			if feedback.calls != tt.wantCalls || telegram.deleted != tt.wantDelete || telegram.banned != tt.wantBan || store.completed != tt.wantStatus {
				t.Fatalf("feedback=%d deleted=%d banned=%d status=%s", feedback.calls, telegram.deleted, telegram.banned, store.completed)
			}
			if tt.wantBan != 0 && (len(actions.kinds) != 2 || actions.kinds[1] != FeedbackActionBan) {
				t.Fatalf("actions=%v", actions.kinds)
			}
			if tt.wantCalls == 1 && tt.saveErr == nil && feedback.input.Feedback.Label != detectiondomain.AILabelSpam {
				t.Fatalf("feedback label=%q", feedback.input.Feedback.Label)
			}
		})
	}
}

func TestHamFeedbackAllowsCorrectionAfterTargetBecomesAdmin(t *testing.T) {
	t.Parallel()
	telegram := &telegramSpy{admins: map[int64]bool{1: true, 2: true}}
	store := &storeStub{claim: domain.Claim{Acquired: true}}
	feedback := &feedbackSubmitterSpy{}
	handler := newFeedbackHandler(t, telegram, store, feedback, &feedbackTargetSpy{}, &feedbackActionSpy{})
	if err := handler.Handle(t.Context(), feedbackCommand(domain.NameHam, "誤判")); err != nil {
		t.Fatal(err)
	}
	if feedback.calls != 1 || store.completed != "completed" || telegram.deleted != 0 || telegram.banned != 0 {
		t.Fatalf("標籤修正應成功且不處置：feedback=%d status=%s Telegram=%+v", feedback.calls, store.completed, telegram)
	}
}

func TestHamFeedbackRevisionAndAuditTarget(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name            string
		command         domain.Command
		auditFound      bool
		wantCalls       int
		wantStatus      string
		wantFingerprint string
	}{
		{name: "回覆正常訊息", command: feedbackCommand(domain.NameHam, "誤判"), wantCalls: 1, wantStatus: "completed"},
		{name: "已刪訊息使用本群稽核", command: domain.Command{UpdateID: 26, ChatID: -1001, MessageID: 14, Actor: domain.Actor{ID: 1}, Name: domain.NameHam, Args: "event:tg:10 誤判"}, auditFound: true, wantCalls: 1, wantStatus: "completed", wantFingerprint: "audited-fingerprint"},
		{name: "他群或未知事件拒絕", command: domain.Command{UpdateID: 27, ChatID: -1001, MessageID: 15, Actor: domain.Actor{ID: 1}, Name: domain.NameHam, Args: "event:tg:11"}, wantStatus: "invalid"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			telegram := &telegramSpy{admins: map[int64]bool{1: true}}
			store := &storeStub{claim: domain.Claim{Acquired: true}}
			feedback := &feedbackSubmitterSpy{}
			targets := &feedbackTargetSpy{target: FeedbackTarget{MessageID: 12, UserID: 2, ContentFingerprint: "audited-fingerprint"}, found: tt.auditFound}
			actions := &feedbackActionSpy{}
			handler := newFeedbackHandler(t, telegram, store, feedback, targets, actions)
			_ = handler.Handle(t.Context(), tt.command)
			if feedback.calls != tt.wantCalls || store.completed != tt.wantStatus || actions.planned != 0 || telegram.banned != 0 || telegram.deleted != 0 || telegram.unbanned != 0 {
				t.Fatalf("feedback=%d status=%s planned=%d Telegram=%+v", feedback.calls, store.completed, actions.planned, telegram)
			}
			if tt.wantCalls == 1 && (feedback.input.Feedback.Label != detectiondomain.AILabelHam || feedback.input.Feedback.Reason != "誤判") {
				t.Fatalf("feedback=%+v", feedback.input.Feedback)
			}
			if tt.wantFingerprint != "" && (feedback.input.Feedback.ContentFingerprint != tt.wantFingerprint || feedback.input.Text != "" || targets.chatID != -1001) {
				t.Fatalf("audit feedback=%+v target=%+v", feedback.input, targets)
			}
			if len(telegram.messages) > 0 && strings.Contains(telegram.messages[0], "已解封") {
				t.Fatalf("/ham 不得宣稱解封：%q", telegram.messages[0])
			}
		})
	}
}

func TestSpamFeedbackPartialEmbeddingStillDeletes(t *testing.T) {
	t.Parallel()
	telegram := &telegramSpy{admins: map[int64]bool{1: true}}
	store := &storeStub{claim: domain.Claim{Acquired: true}}
	feedback := &feedbackSubmitterSpy{result: detectionapp.ManualFeedbackResult{
		Saved: true, EmbeddingStatus: detectionapp.ManualFeedbackEmbeddingFailed, EmbeddingErrorCode: "timeout",
	}}
	actions := &feedbackActionSpy{}
	handler := newFeedbackHandler(t, telegram, store, feedback, &feedbackTargetSpy{}, actions)
	if err := handler.Handle(t.Context(), feedbackCommand(domain.NameSpam, "")); err != nil {
		t.Fatal(err)
	}
	if telegram.deleted != 12 || actions.planned != 1 || len(telegram.messages) != 1 || !strings.Contains(telegram.messages[0], "向量化失敗") {
		t.Fatalf("deleted=%d planned=%d messages=%v", telegram.deleted, actions.planned, telegram.messages)
	}
}

func TestSpamFeedbackRechecksProtectionBeforeBan(t *testing.T) {
	t.Parallel()
	telegram := &telegramSpy{admins: map[int64]bool{1: true}}
	telegram.onDelete = func() { telegram.admins[2] = true }
	store := &storeStub{claim: domain.Claim{Acquired: true}}
	feedback := &feedbackSubmitterSpy{}
	actions := &feedbackActionSpy{}
	handler := newFeedbackHandler(t, telegram, store, feedback, &feedbackTargetSpy{}, actions)
	if err := handler.Handle(t.Context(), feedbackCommand(domain.NameSpam, "ban")); err != nil {
		t.Fatal(err)
	}
	if telegram.deleted != 12 || telegram.banned != 0 || store.completed != "partial" || len(actions.completed) != 2 || actions.completed[1] != FeedbackActionBan {
		t.Fatalf("保護狀態變更後不得封鎖：deleted=%d banned=%d status=%s actions=%v", telegram.deleted, telegram.banned, store.completed, actions.completed)
	}
}
