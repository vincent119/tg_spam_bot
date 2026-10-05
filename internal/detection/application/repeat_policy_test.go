package application

import (
	"slices"
	"testing"
)

func TestRepeatEnforcementModeMatrix(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		action     RepeatAction
		mode       Mode
		available  bool
		current    bool
		count      int
		wantIDs    []int64
		wantBan    bool
		wantSignal bool
	}{
		{"預設只觀察", RepeatActionObserve, ModeEnforce, true, true, 3, nil, false, true},
		{"全域只觀察", RepeatActionBan, ModeObserve, true, true, 3, nil, false, true},
		{"僅刪除限制封鎖", RepeatActionBan, ModeDeleteOnly, true, true, 3, []int64{1, 2, 3}, false, true},
		{"明示刪除", RepeatActionDelete, ModeEnforce, true, true, 3, []int64{1, 2, 3}, false, true},
		{"明示封鎖", RepeatActionBan, ModeEnforce, true, true, 3, []int64{1, 2, 3}, true, true},
		{"未達門檻", RepeatActionBan, ModeEnforce, true, true, 2, nil, false, false},
		{"容量降級", RepeatActionBan, ModeEnforce, false, true, 3, nil, false, false},
		{"未觀測當前訊息", RepeatActionBan, ModeEnforce, true, false, 3, nil, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ids := []int64{1, 2, 3}
			snapshot := RepeatSnapshot{Available: tt.available, CurrentObserved: tt.current, Count: tt.count, MessageIDs: ids}
			plan := PlanRepeat(snapshot, 3, tt.action, tt.mode)
			if plan.Triggered != tt.wantSignal || plan.Ban != tt.wantBan || !slices.Equal(plan.MessageIDs, tt.wantIDs) {
				t.Fatalf("計畫=%+v，預期訊號=%v、訊息=%v、封鎖=%v", plan, tt.wantSignal, tt.wantIDs, tt.wantBan)
			}
			if len(plan.MessageIDs) > 0 {
				plan.MessageIDs[0] = 99
				if ids[0] != 1 {
					t.Fatal("處置計畫不得共享輸入切片")
				}
			}
		})
	}
}
