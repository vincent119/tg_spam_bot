package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/vincent119/tg_spam_bot/internal/detection/application"
	"github.com/vincent119/tg_spam_bot/internal/detection/domain"
	"github.com/vincent119/tg_spam_bot/internal/detection/infra/memory"
)

type detectorStub struct{ result domain.Result }

func (d detectorStub) Detect(domain.Message, ...string) domain.Result { return d.result }

type telegramSpy struct {
	actions []string
	failOn  string
}

func (s *telegramSpy) DeleteMessage(context.Context, int64, int64) error {
	s.actions = append(s.actions, "delete")
	if s.failOn == "delete" {
		return errors.New("temporary delete failure")
	}
	return nil
}

func TestProcessorModes(t *testing.T) {
	t.Parallel()
	result := domain.Result{Spam: true, Severity: domain.SeverityNormal, Action: domain.ActionProgressive, CategoryID: "ad", RuleVersion: "v1"}
	for _, tt := range []struct {
		name string
		mode application.Mode
		want int
	}{
		{name: "observe", mode: application.ModeObserve, want: 0},
		{name: "delete only", mode: application.ModeDeleteOnly, want: 1},
		{name: "enforce", mode: application.ModeEnforce, want: 2},
	} {
		t.Run(tt.name, func(t *testing.T) {
			store := memory.NewStore(time.Minute, 100)
			telegram := &telegramSpy{}
			processor := application.NewProcessor(detectorStub{result}, store, store, store, store, telegram, tt.mode, []byte("01234567890123456789012345678901"))
			if err := processor.Process(context.Background(), domain.Message{UpdateID: 1, ChatID: 2, MessageID: 3, UserID: 4, Text: "ad"}); err != nil {
				t.Fatal(err)
			}
			if len(telegram.actions) != tt.want {
				t.Fatalf("actions = %v, want count %d", telegram.actions, tt.want)
			}
		})
	}
}

func TestProcessorReturnsPartialActionFailure(t *testing.T) {
	t.Parallel()
	store := memory.NewStore(time.Minute, 100)
	telegram := &telegramSpy{failOn: "delete"}
	result := domain.Result{Spam: true, Severity: domain.SeverityNormal, Action: domain.ActionProgressive, CategoryID: "ad", RuleVersion: "v1"}
	processor := application.NewProcessor(detectorStub{result}, store, store, store, store, telegram, application.ModeEnforce, []byte("01234567890123456789012345678901"))
	err := processor.Process(context.Background(), domain.Message{UpdateID: 1, ChatID: 2, MessageID: 3, UserID: 4, Text: "ad"})
	if err == nil {
		t.Fatal("expected action failure")
	}
}
func (s *telegramSpy) SendWarning(context.Context, int64, int64, string) error {
	s.actions = append(s.actions, "warn")
	return nil
}
func (s *telegramSpy) RestrictMember(context.Context, int64, int64, time.Time) error {
	s.actions = append(s.actions, "restrict")
	return nil
}
func (s *telegramSpy) BanMember(context.Context, int64, int64) error {
	s.actions = append(s.actions, "ban")
	return nil
}

func TestProcessorCriticalAndDuplicate(t *testing.T) {
	t.Parallel()
	store := memory.NewStore(time.Minute, 100)
	telegram := &telegramSpy{}
	result := domain.Result{Spam: true, Severity: domain.SeverityCritical, Action: domain.ActionBan, CategoryID: "counterfeit", RuleVersion: "v1"}
	processor := application.NewProcessor(detectorStub{result}, store, store, store, store, telegram, application.ModeEnforce, []byte("01234567890123456789012345678901"))
	message := domain.Message{UpdateID: 1, ChatID: 2, MessageID: 3, UserID: 4, Text: "假鈔出售"}
	if err := processor.Process(context.Background(), message); err != nil {
		t.Fatal(err)
	}
	if err := processor.Process(context.Background(), message); err != nil {
		t.Fatal(err)
	}
	if len(telegram.actions) != 2 || telegram.actions[0] != "delete" || telegram.actions[1] != "ban" {
		t.Fatalf("actions = %v", telegram.actions)
	}
}

func TestProcessorExemption(t *testing.T) {
	t.Parallel()
	store := memory.NewStore(time.Minute, 100)
	store.Trust(2, 4, "trusted")
	telegram := &telegramSpy{}
	result := domain.Result{Spam: true, Severity: domain.SeverityCritical, Action: domain.ActionBan}
	processor := application.NewProcessor(detectorStub{result}, store, store, store, store, telegram, application.ModeEnforce, []byte("01234567890123456789012345678901"))
	if err := processor.Process(context.Background(), domain.Message{UpdateID: 1, ChatID: 2, MessageID: 3, UserID: 4, Text: "spam"}); err != nil {
		t.Fatal(err)
	}
	if len(telegram.actions) != 0 {
		t.Fatalf("trusted member actions = %v", telegram.actions)
	}
}
