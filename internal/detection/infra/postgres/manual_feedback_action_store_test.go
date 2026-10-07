package postgres

import (
	"testing"

	commandapp "github.com/vincent119/tg_spam_bot/internal/command/application"
	commanddomain "github.com/vincent119/tg_spam_bot/internal/command/domain"
)

func TestFeedbackActionBatchPlan(t *testing.T) {
	t.Parallel()
	command := commanddomain.Command{ChatID: -1001, UpdateID: 25, Target: &commanddomain.Target{ID: 2}, TargetMessage: 12}
	kinds := []commandapp.FeedbackActionKind{commandapp.FeedbackActionDelete, commandapp.FeedbackActionBan}
	rows, err := feedbackActionPlan(command, kinds, []int64{12, 10, 10, 14})
	if err != nil || len(rows) != 4 {
		t.Fatalf("計畫=%+v 錯誤=%v", rows, err)
	}
	for i, key := range []string{"delete", "delete:10", "delete:14", "ban"} {
		if rows[i].Kind != key || rows[i].ChatID != command.ChatID || rows[i].TargetUserID != 2 || rows[i].Status != "pending" {
			t.Fatalf("目標=%+v 預期鍵=%s", rows[i], key)
		}
	}
	if !sameFeedbackActionPlan(rows, []manualFeedbackAction{rows[3], rows[2], rows[1], rows[0]}) {
		t.Fatal("相同目標不應受順序影響")
	}
	changed := append([]manualFeedbackAction(nil), rows...)
	changed[1].MessageID = 15
	if sameFeedbackActionPlan(rows, changed) || sameFeedbackActionPlan(rows, rows[:3]) {
		t.Fatal("不得變更或縮減既存目標")
	}
	if _, err := feedbackActionPlan(command, kinds, []int64{0}); err == nil {
		t.Fatal("不得保存無效目標")
	}
	if _, err := feedbackActionPlan(command, kinds, make([]int64, 101)); err == nil {
		t.Fatal("不得超過一百則")
	}
}
