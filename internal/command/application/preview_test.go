package application

import (
	"context"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/vincent119/tg_spam_bot/internal/command/domain"
	detectionapp "github.com/vincent119/tg_spam_bot/internal/detection/application"
	detectiondomain "github.com/vincent119/tg_spam_bot/internal/detection/domain"
)

type previewerSpy struct {
	message detectiondomain.Message
	result  detectionapp.PreviewResult
	calls   int
}

func (s *previewerSpy) Preview(_ context.Context, message detectiondomain.Message) (detectionapp.PreviewResult, error) {
	s.calls++
	s.message = message
	return s.result, nil
}

func TestCheckCommandUsesReadOnlyPreview(t *testing.T) {
	t.Parallel()
	telegram := &telegramSpy{admins: map[int64]bool{1: true, 2: true}}
	store := &storeStub{claim: domain.Claim{Acquired: true}}
	preview := &previewerSpy{result: detectionapp.PreviewResult{
		Rule:     detectiondomain.Result{RuleVersion: "v1\n偽造回覆", CategoryID: strings.Repeat("廣", 1300), Score: 3, Threshold: 5},
		AIStatus: "not_run", AIReason: "preview_does_not_call_ai", Historical: detectionapp.PreviewHistory{Status: "not_observed"},
	}}
	handler, err := NewHandler(telegram, trustedStub{ids: map[int64]bool{}}, store, limiterStub{allowed: true}, 99, WithDetectionPreview(preview))
	if err != nil {
		t.Fatal(err)
	}
	command, err := domain.NewCommand(domain.Command{
		UpdateID: 30, ChatID: -1001, MessageID: 15, Actor: domain.Actor{ID: 1},
		Target: &domain.Target{ID: 2}, TargetMessage: 12, TargetText: "來收米 日掙1W", Name: domain.NameCheck,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := handler.Handle(t.Context(), command); err != nil {
		t.Fatal(err)
	}
	if preview.calls != 1 || preview.message.ChatID != -1001 || preview.message.MessageID != 12 || preview.message.UserID != 2 || telegram.deleted != 0 || telegram.banned != 0 || store.completed != "completed" {
		t.Fatalf("preview=%+v Telegram=%+v status=%s", preview, telegram, store.completed)
	}
	if len(telegram.messages) != 1 || utf8.RuneCountInString(telegram.messages[0]) > maxPreviewReplyRunes || !strings.Contains(telegram.messages[0], "本次預覽未呼叫 AI") || strings.Contains(telegram.messages[0], "v1\n偽造回覆") {
		t.Fatalf("不安全或過長的預覽回覆：%q", telegram.messages)
	}
}

func TestCheckCommandRequiresAdminReplyAndNoArgs(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		admin  bool
		target *domain.Target
		text   string
		args   string
	}{
		{name: "一般成員", target: &domain.Target{ID: 2}, text: "正常訊息"},
		{name: "缺少回覆", admin: true, text: "正常訊息"},
		{name: "回覆缺少文字", admin: true, target: &domain.Target{ID: 2}},
		{name: "多餘參數", admin: true, target: &domain.Target{ID: 2}, text: "正常訊息", args: "ai"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			telegram := &telegramSpy{admins: map[int64]bool{1: tt.admin}}
			store := &storeStub{claim: domain.Claim{Acquired: true}}
			preview := &previewerSpy{}
			handler, err := NewHandler(telegram, trustedStub{ids: map[int64]bool{}}, store, limiterStub{allowed: true}, 99, WithDetectionPreview(preview))
			if err != nil {
				t.Fatal(err)
			}
			command, _ := domain.NewCommand(domain.Command{
				UpdateID: 31, ChatID: -1001, MessageID: 16, Actor: domain.Actor{ID: 1},
				Target: tt.target, TargetMessage: 12, TargetText: tt.text, Name: domain.NameCheck, Args: tt.args,
			})
			_ = handler.Handle(t.Context(), command)
			if preview.calls != 0 || telegram.deleted != 0 || telegram.banned != 0 {
				t.Fatalf("不得預覽或處置：preview=%d Telegram=%+v", preview.calls, telegram)
			}
		})
	}
}
