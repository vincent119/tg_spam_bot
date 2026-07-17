package application

import (
	"reflect"
	"testing"

	"github.com/vincent119/tg_spam_bot/internal/detection/domain"
)

func TestPlanActions(t *testing.T) {
	t.Parallel()
	normal := domain.Result{Spam: true, Severity: domain.SeverityNormal, Action: domain.ActionProgressive}
	critical := domain.Result{Spam: true, Severity: domain.SeverityCritical, Action: domain.ActionBan}
	tests := []struct {
		name   string
		result domain.Result
		mode   Mode
		count  int
		want   []ActionKind
	}{
		{name: "observe", result: normal, mode: ModeObserve},
		{name: "delete only", result: normal, mode: ModeDeleteOnly, want: []ActionKind{ActionDelete}},
		{name: "first", result: normal, mode: ModeEnforce, count: 1, want: []ActionKind{ActionDelete, ActionWarn}},
		{name: "second", result: normal, mode: ModeEnforce, count: 2, want: []ActionKind{ActionDelete, ActionMute10m}},
		{name: "third", result: normal, mode: ModeEnforce, count: 3, want: []ActionKind{ActionDelete, ActionMute24h}},
		{name: "fourth", result: normal, mode: ModeEnforce, count: 4, want: []ActionKind{ActionDelete, ActionBan}},
		{name: "critical", result: critical, mode: ModeEnforce, count: 1, want: []ActionKind{ActionDelete, ActionBan}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := PlanActions(tt.result, tt.mode, tt.count); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("PlanActions() = %v, want %v", got, tt.want)
			}
		})
	}
}
