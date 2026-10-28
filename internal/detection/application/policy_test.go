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

func TestYAMLFirstOffenseEnforcement(t *testing.T) {
	result := domain.Result{Spam: true, Severity: domain.SeverityNormal, Action: domain.ActionProgressive}
	for _, tt := range []struct {
		name  string
		event Event
		want  []ActionKind
	}{
		{"規則首次封鎖", Event{Result: result, YAMLSpam: true, Mode: ModeEnforce}, []ActionKind{ActionDelete, ActionBan}},
		{"僅刪除", Event{Result: result, YAMLSpam: true, Mode: ModeDeleteOnly}, []ActionKind{ActionDelete}},
		{"觀察", Event{Result: result, YAMLSpam: true, Mode: ModeObserve}, nil},
		{"非規則維持階梯", Event{Result: result, Mode: ModeEnforce}, []ActionKind{ActionDelete, ActionWarn}},
		{"未判垃圾", Event{Result: domain.Result{}, YAMLSpam: true, Mode: ModeEnforce}, nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := PlanEventActions(tt.event, 1); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("actions=%v，預期=%v", got, tt.want)
			}
		})
	}
}
