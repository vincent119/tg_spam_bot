package application

import (
	"context"
	"errors"
	"testing"

	autoreplydomain "github.com/vincent119/tg_spam_bot/internal/autoreply/domain"
	detectiondomain "github.com/vincent119/tg_spam_bot/internal/detection/domain"
)

func TestMatcherMatch(t *testing.T) {
	t.Parallel()

	matcher := NewMatcher(autoreplydomain.RuleSet{Rules: []autoreplydomain.Rule{
		{ID: "download_page", Enabled: true, Keywords: []string{"下載頁", "app download"}, Reply: "下載頁"},
		{ID: "support", Enabled: true, Keywords: []string{"客服"}, Reply: "客服"},
		{ID: "disabled", Enabled: false, Keywords: []string{"停用"}, Reply: "停用"},
	}}, detectiondomain.NewNormalizer(detectiondomain.OpenCCConverter{}, 8192))

	tests := []struct {
		name string
		text string
		want string
		ok   bool
	}{
		{name: "繁簡變體", text: "下载页在哪", want: "download_page", ok: true},
		{name: "英文大小寫", text: "APP Download please", want: "download_page", ok: true},
		{name: "第一條命中", text: "下載頁與客服", want: "download_page", ok: true},
		{name: "停用規則", text: "停用", ok: false},
		{name: "未命中", text: "今天天氣", ok: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, ok := matcher.Match(detectiondomain.Message{Text: tt.text})
			if ok != tt.ok || got.RuleID != tt.want {
				t.Fatalf("Match() = %+v, %v，預期 rule=%q ok=%v", got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestProcessorProcess(t *testing.T) {
	t.Parallel()

	matcher := NewMatcher(autoreplydomain.RuleSet{Rules: []autoreplydomain.Rule{{ID: "download_page", Enabled: true, Keywords: []string{"下載頁"}, Reply: "下載頁"}}}, detectiondomain.NewNormalizer(detectiondomain.OpenCCConverter{}, 8192))
	store := &storeSpy{}
	telegram := &telegramSpy{}
	processor, err := NewProcessor(matcher, store, telegram)
	if err != nil {
		t.Fatalf("NewProcessor() error = %v", err)
	}
	if err := processor.Process(t.Context(), detectiondomain.Message{UpdateID: 1, ChatID: -1001, MessageID: 2, UserID: 3, Text: "下載頁在哪"}); err != nil {
		t.Fatalf("Process() error = %v", err)
	}
	if telegram.sent != "下載頁" || store.completed != 1 {
		t.Fatalf("sent=%q completed=%d", telegram.sent, store.completed)
	}
	if err := processor.Process(t.Context(), detectiondomain.Message{UpdateID: 2, ChatID: -1001, MessageID: 3, UserID: 3, Text: "不相關"}); err != nil {
		t.Fatalf("Process() no match error = %v", err)
	}
	if store.claims != 1 {
		t.Fatalf("未命中不應 claim，claims=%d", store.claims)
	}
}

func TestProcessorProcessFailure(t *testing.T) {
	t.Parallel()

	matcher := NewMatcher(autoreplydomain.RuleSet{Rules: []autoreplydomain.Rule{{ID: "download_page", Enabled: true, Keywords: []string{"下載頁"}, Reply: "下載頁"}}}, detectiondomain.NewNormalizer(detectiondomain.OpenCCConverter{}, 8192))
	store := &storeSpy{}
	processor, err := NewProcessor(matcher, store, &telegramSpy{err: errors.New("telegram failed")})
	if err != nil {
		t.Fatalf("NewProcessor() error = %v", err)
	}
	if err := processor.Process(t.Context(), detectiondomain.Message{UpdateID: 1, ChatID: -1001, MessageID: 2, UserID: 3, Text: "下載頁在哪"}); err == nil {
		t.Fatal("Process() 應回傳 Telegram 錯誤")
	}
	if store.failed != 1 {
		t.Fatalf("failed=%d", store.failed)
	}
}

type storeSpy struct {
	claims    int
	completed int
	failed    int
}

func (s *storeSpy) ClaimAutoReply(context.Context, Event) (Claim, error) {
	s.claims++
	return Claim{Acquired: true}, nil
}

func (s *storeSpy) CompleteAutoReply(context.Context, Event) error {
	s.completed++
	return nil
}

func (s *storeSpy) FailAutoReply(context.Context, Event, Result) error {
	s.failed++
	return nil
}

type telegramSpy struct {
	sent string
	err  error
}

func (s *telegramSpy) SendMessage(_ context.Context, _ int64, _ int64, text string) error {
	s.sent = text
	return s.err
}
