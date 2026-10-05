package redis

import (
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/vincent119/tg_spam_bot/internal/detection/application"
	"github.com/vincent119/tg_spam_bot/internal/detection/domain"
	"github.com/vincent119/tg_spam_bot/internal/detection/rules"
)

func TestChineseModerationRepeatSequence(t *testing.T) {
	t.Parallel()
	ruleSet, err := rules.LoadDir(filepath.Join("..", "..", "..", "..", "configs", "rules"))
	if err != nil {
		t.Fatal(err)
	}
	detector, err := domain.NewDetector(ruleSet, domain.NewNormalizer(domain.OpenCCConverter{}, 4096), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name     string
		text     string
		ruleSpam bool
	}{
		{name: "短廣告", text: "带码来干 收米一天1W", ruleSpam: true},
		{name: "正常短句仍有重複封鎖風險", text: "收到", ruleSpam: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			store, _ := newBehaviorTestStore(t)
			start := store.now()
			for i, minute := range []int{0, 9, 18} {
				now := start.Add(time.Duration(minute) * time.Minute)
				store.now = func() time.Time { return now }
				message := domain.Message{ChatID: 1, UserID: 2, MessageID: int64(i + 1), Text: tt.text}
				observed, err := store.ObserveDetailed(t.Context(), message, "同一原文指紋")
				if err != nil {
					t.Fatal(err)
				}
				result := detector.Detect(message, observed.Signals...)
				plan := application.PlanRepeat(observed.Repeat, 3, application.RepeatActionBan, application.ModeEnforce)
				if result.Spam != tt.ruleSpam || observed.Repeat.Count != i+1 || plan.Triggered != (i == 2) || plan.Ban != (i == 2) {
					t.Fatalf("第 %d 則：rule=%v repeat=%+v plan=%+v", i+1, result.Spam, observed.Repeat, plan)
				}
				if i == 2 {
					if !slices.Equal(plan.MessageIDs, []int64{3, 2, 1}) || !slices.Contains(result.Signals, domain.SignalRepeatedContent) {
						t.Fatalf("第三則未包含完整候選或重複訊號：plan=%+v signals=%v", plan, result.Signals)
					}
				}
			}
		})
	}
}
