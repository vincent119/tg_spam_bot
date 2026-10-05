package application_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/vincent119/tg_spam_bot/internal/detection/application"
	"github.com/vincent119/tg_spam_bot/internal/detection/domain"
	"github.com/vincent119/tg_spam_bot/internal/detection/infra/memory"
)

type detailedRepeatStub struct{ ids []int64 }

func (s *detailedRepeatStub) Observe(ctx context.Context, message domain.Message, fingerprint string) ([]string, error) {
	observation, err := s.ObserveDetailed(ctx, message, fingerprint)
	return observation.Signals, err
}

func (s *detailedRepeatStub) ObserveDetailed(_ context.Context, message domain.Message, _ string) (application.BehaviorObservation, error) {
	if !slices.Contains(s.ids, message.MessageID) {
		s.ids = append(s.ids, message.MessageID)
	}
	snapshot := application.RepeatSnapshot{
		Count: len(s.ids), MessageIDs: slices.Clone(s.ids), Window: 30 * time.Minute,
		ObservedAt: time.Now().UTC(), Available: true, CurrentObserved: true,
	}
	observation := application.BehaviorObservation{Repeat: snapshot}
	if len(s.ids) >= 3 {
		observation.Signals = []string{domain.SignalRepeatedContent}
	}
	return observation, nil
}

type repeatActionStub struct {
	executions map[string]application.RepeatExecution
	completed  map[string]bool
}

func (s *repeatActionStub) CreateOrLoadRepeatExecution(_ context.Context, event application.Event, plan application.RepeatPlan, skipCurrentDelete bool) (application.RepeatExecution, error) {
	if execution, ok := s.executions[event.ID]; ok {
		return execution, nil
	}
	if len(plan.MessageIDs) == 0 {
		return application.RepeatExecution{}, nil
	}
	execution := application.RepeatExecution{EventID: event.ID, ChatID: event.Message.ChatID, UserID: event.Message.UserID, Count: plan.Count}
	for _, id := range plan.MessageIDs {
		if id == event.Message.MessageID && skipCurrentDelete {
			continue
		}
		execution.Steps = append(execution.Steps, application.RepeatStep{Key: fmt.Sprintf("delete:%d:%d", event.Message.ChatID, id), Kind: application.ActionDelete, MessageID: id})
	}
	if plan.Ban {
		execution.Steps = append(execution.Steps, application.RepeatStep{Key: event.ID + ":ban", Kind: application.ActionBan, UserID: event.Message.UserID})
	}
	s.executions[event.ID] = execution
	return execution, nil
}

func (s *repeatActionStub) ClaimRepeatStep(_ context.Context, key string) (application.RepeatStepClaim, error) {
	if s.completed[key] {
		return application.RepeatStepClaim{Completed: true}, nil
	}
	return application.RepeatStepClaim{Acquired: true, LeaseToken: "test-lease"}, nil
}

func (s *repeatActionStub) CompleteRepeatStep(_ context.Context, key, leaseToken string, result application.ActionResult) error {
	if leaseToken != "test-lease" {
		return errors.New("unexpected lease")
	}
	if result.Succeeded {
		s.completed[key] = true
	}
	return nil
}

type repeatTelegramSpy struct {
	telegramSpy
	deleted   []int64
	bans      int
	admin     bool
	adminErr  error
	failID    int64
	failCount int
}

func (s *repeatTelegramSpy) DeleteMessage(_ context.Context, _, messageID int64) error {
	s.deleted = append(s.deleted, messageID)
	if s.failID == messageID && s.failCount > 0 {
		s.failCount--
		return errors.New("temporary delete error")
	}
	return nil
}

func (s *repeatTelegramSpy) BanMember(context.Context, int64, int64) error {
	s.bans++
	return nil
}

func (s *repeatTelegramSpy) IsAdmin(context.Context, int64, int64) (bool, error) {
	return s.admin, s.adminErr
}

func newRepeatEnforcementProcessor(t *testing.T, mode application.Mode, action application.RepeatAction, tg *repeatTelegramSpy, actions *repeatActionStub) *application.Processor {
	t.Helper()
	store := memory.NewStore(time.Minute, 100)
	behaviors := &detailedRepeatStub{}
	return application.NewProcessor(detectorStub{result: domain.Result{RuleVersion: "test", Threshold: 1}}, store, store, behaviors, store, tg,
		mode, []byte("01234567890123456789012345678901"),
		application.WithRepeatEnforcement(action, 3, actions, store, tg))
}

func TestRepeatEnforcementThresholdAndCleanup(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name       string
		mode       application.Mode
		action     application.RepeatAction
		wantDelete []int64
		wantBan    int
	}{
		{name: "預設只觀察", mode: application.ModeEnforce, action: application.RepeatActionObserve},
		{name: "明示刪除", mode: application.ModeEnforce, action: application.RepeatActionDelete, wantDelete: []int64{1, 2, 3}},
		{name: "明示封鎖", mode: application.ModeEnforce, action: application.RepeatActionBan, wantDelete: []int64{1, 2, 3}, wantBan: 1},
		{name: "全域觀察限制", mode: application.ModeObserve, action: application.RepeatActionBan},
		{name: "全域僅刪除限制", mode: application.ModeDeleteOnly, action: application.RepeatActionBan, wantDelete: []int64{1, 2, 3}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tg := &repeatTelegramSpy{}
			actions := &repeatActionStub{executions: map[string]application.RepeatExecution{}, completed: map[string]bool{}}
			processor := newRepeatEnforcementProcessor(t, tt.mode, tt.action, tg, actions)
			for id := int64(1); id <= 3; id++ {
				result, err := processor.Process(t.Context(), repeatMessage(id))
				if err != nil {
					t.Fatal(err)
				}
				if id < 3 && (result.Spam || len(tg.deleted) > 0 || tg.bans > 0) {
					t.Fatalf("第 %d 則不得提前處置：%+v", id, result)
				}
			}
			if !slices.Equal(tg.deleted, tt.wantDelete) || tg.bans != tt.wantBan {
				t.Fatalf("deleted=%v ban=%d；預期 deleted=%v ban=%d", tg.deleted, tg.bans, tt.wantDelete, tt.wantBan)
			}
		})
	}
}

func TestRepeatCleanupRetry(t *testing.T) {
	t.Parallel()
	tg := &repeatTelegramSpy{failID: 2, failCount: 1}
	actions := &repeatActionStub{executions: map[string]application.RepeatExecution{}, completed: map[string]bool{}}
	processor := newRepeatEnforcementProcessor(t, application.ModeEnforce, application.RepeatActionBan, tg, actions)
	for id := int64(1); id <= 2; id++ {
		if _, err := processor.Process(t.Context(), repeatMessage(id)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := processor.Process(t.Context(), repeatMessage(3)); err == nil {
		t.Fatal("第二則刪除失敗應保留重試")
	}
	if !slices.Equal(tg.deleted, []int64{1, 2}) || tg.bans != 0 {
		t.Fatalf("第一次部分失敗不得提前封鎖：%v，ban=%d", tg.deleted, tg.bans)
	}
	if result, err := processor.Process(t.Context(), repeatMessage(3)); err != nil || !result.Spam {
		t.Fatalf("重送應接續固定計畫：%+v，%v", result, err)
	}
	if !slices.Equal(tg.deleted, []int64{1, 2, 2, 3}) || tg.bans != 1 {
		t.Fatalf("已完成第一則不可重刪：%v，ban=%d", tg.deleted, tg.bans)
	}
}

func TestRepeatCleanupScopeAndExemptions(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name     string
		admin    bool
		adminErr error
		wantErr  bool
	}{
		{name: "管理員受保護", admin: true},
		{name: "查詢管理員失敗時安全停止", adminErr: errors.New("查詢失敗"), wantErr: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tg := &repeatTelegramSpy{admin: tt.admin, adminErr: tt.adminErr}
			actions := &repeatActionStub{executions: map[string]application.RepeatExecution{}, completed: map[string]bool{}}
			processor := newRepeatEnforcementProcessor(t, application.ModeEnforce, application.RepeatActionBan, tg, actions)
			for id := int64(1); id <= 3; id++ {
				_, err := processor.Process(t.Context(), repeatMessage(id))
				if (err != nil) != (tt.wantErr && id == 3) {
					t.Fatalf("第 %d 則錯誤=%v", id, err)
				}
			}
			if len(tg.deleted) != 0 || tg.bans != 0 {
				t.Fatalf("受保護或狀態未知不得處置：deleted=%v，ban=%d", tg.deleted, tg.bans)
			}
		})
	}
}
