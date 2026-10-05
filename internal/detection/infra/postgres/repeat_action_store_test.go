package postgres

import (
	"context"
	"fmt"
	"os"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/vincent119/tg_spam_bot/internal/detection/application"
	"github.com/vincent119/tg_spam_bot/internal/detection/domain"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestBuildRepeatSteps(t *testing.T) {
	t.Parallel()
	event := application.Event{ID: "tg:100", Message: domain.Message{ChatID: 1, UserID: 2, MessageID: 13}, Fingerprint: "same"}
	plan := application.RepeatPlan{
		Triggered: true, Count: 3, Window: 30 * time.Minute,
		Action: application.RepeatActionBan, Mode: application.ModeEnforce,
		MessageIDs: []int64{13, 12, 11}, Ban: true,
	}
	for _, tt := range []struct {
		name        string
		skipCurrent bool
		wantIDs     []int64
	}{
		{name: "目前與先前訊息", wantIDs: []int64{13, 12, 11}},
		{name: "已有規則清除目前訊息", skipCurrent: true, wantIDs: []int64{12, 11}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			steps, err := buildRepeatSteps(event, plan, tt.skipCurrent)
			if err != nil {
				t.Fatal(err)
			}
			if len(steps) != len(tt.wantIDs)+1 || steps[len(steps)-1].Kind != application.ActionBan {
				t.Fatalf("ban 必須排在全部刪文後：%+v", steps)
			}
			ids := make([]int64, 0, len(tt.wantIDs))
			for _, step := range steps[:len(steps)-1] {
				if step.Kind != application.ActionDelete || step.UserID != event.Message.UserID {
					t.Fatalf("刪文範圍不符：%+v", step)
				}
				ids = append(ids, step.MessageID)
			}
			if !slices.Equal(ids, tt.wantIDs) {
				t.Fatalf("候選 ID=%v，預期=%v", ids, tt.wantIDs)
			}
		})
	}
	invalid := plan
	invalid.MessageIDs = []int64{11, 12, 14}
	if _, err := buildRepeatSteps(event, invalid, false); err == nil {
		t.Fatal("缺少目前訊息不得建立處置")
	}
	invalid = plan
	invalid.MessageIDs = []int64{13, 13, 11}
	if _, err := buildRepeatSteps(event, invalid, false); err == nil {
		t.Fatal("重複 ID 不得建立處置")
	}
}

func TestRepeatActionStoreIntegration(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("未設定 TEST_DATABASE_URL")
	}
	ctx := t.Context()
	db, err := gorm.Open(postgres.Open(databaseURL), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := AutoMigrateRepeatActions(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := AutoMigrateRepeatActions(ctx, nil); err == nil {
		t.Fatal("缺少 DB 不得遷移")
	}
	seed := time.Now().UnixNano()
	chatID, userID := seed, seed+1
	t.Cleanup(func() {
		cleanup := db.WithContext(context.Background())
		_ = cleanup.Where("chat_id = ?", chatID).Delete(&repeatActionStep{}).Error
		_ = cleanup.Where("chat_id = ?", chatID).Delete(&repeatActionExecution{}).Error
	})
	store, err := NewStore(db)
	if err != nil {
		t.Fatal(err)
	}
	event := application.Event{
		ID:          fmt.Sprintf("it-repeat:%d:first", seed),
		Message:     domain.Message{ChatID: chatID, UserID: userID, MessageID: 103},
		Fingerprint: "hmac-fingerprint", CreatedAt: time.Now().UTC(),
	}
	plan := application.RepeatPlan{
		Triggered: true, Count: 3, Window: 30 * time.Minute,
		Action: application.RepeatActionBan, Mode: application.ModeEnforce,
		MessageIDs: []int64{103, 102, 101}, Ban: true,
	}
	first, err := store.CreateOrLoadRepeatExecution(ctx, event, plan, false)
	if err != nil || len(first.Steps) != 4 || first.Steps[3].Kind != application.ActionBan {
		t.Fatalf("初次保存及 ban 順序不符：%+v %v", first, err)
	}
	var saved repeatActionExecution
	if err := db.WithContext(ctx).Where("event_id = ?", event.ID).Take(&saved).Error; err != nil {
		t.Fatal(err)
	}
	if saved.Count != 3 || saved.WindowMillis != 30*time.Minute.Milliseconds() || saved.PolicyVersion != repeatActionPolicyVersion ||
		saved.Action != string(application.RepeatActionBan) || saved.Mode != string(application.ModeEnforce) {
		t.Fatalf("政策快照未固定：%+v", saved)
	}
	changed := plan
	changed.MessageIDs = []int64{103, 102, 101, 104}
	changed.Count = 4
	loaded, err := store.CreateOrLoadRepeatExecution(ctx, event, changed, false)
	if err != nil || !slices.Equal(loaded.Steps, first.Steps) || loaded.Count != 3 {
		t.Fatalf("重送不得擴張候選：%+v %v", loaded, err)
	}
	loaded, err = store.CreateOrLoadRepeatExecution(ctx, event, application.RepeatPlan{}, false)
	if err != nil || !slices.Equal(loaded.Steps, first.Steps) {
		t.Fatalf("過窗重送應沿用已保存計畫：%+v %v", loaded, err)
	}
	wrongScope := event
	wrongScope.Message.ChatID++
	if _, err := store.CreateOrLoadRepeatExecution(ctx, wrongScope, application.RepeatPlan{}, false); err == nil {
		t.Fatal("相同事件 ID 不得跨群讀取舊計畫")
	}
	wrongScope = event
	wrongScope.Message.MessageID++
	if _, err := store.CreateOrLoadRepeatExecution(ctx, wrongScope, application.RepeatPlan{}, false); err == nil {
		t.Fatal("相同事件 ID 不得指向其他訊息")
	}

	secondEvent := event
	secondEvent.ID = fmt.Sprintf("it-repeat:%d:second", seed)
	secondEvent.Message.MessageID = 104
	secondPlan := application.RepeatPlan{
		Triggered: true, Count: 3, Window: 30 * time.Minute,
		Action: application.RepeatActionDelete, Mode: application.ModeEnforce,
		MessageIDs: []int64{104, 103, 102},
	}
	second, err := store.CreateOrLoadRepeatExecution(ctx, secondEvent, secondPlan, false)
	if err != nil || len(second.Steps) != 3 || second.Steps[1].Key != first.Steps[0].Key {
		t.Fatalf("跨事件應共用同群同訊息刪除步驟：%+v %v", second, err)
	}
	var stepCount int64
	if err := db.WithContext(ctx).Model(&repeatActionStep{}).Where("chat_id = ?", chatID).Count(&stepCount).Error; err != nil || stepCount != 5 {
		t.Fatalf("去重後步驟數=%d，錯誤=%v", stepCount, err)
	}

	deleteStep := first.Steps[0]
	claim, err := store.ClaimRepeatStep(ctx, deleteStep.Key)
	if err != nil || !claim.Acquired || claim.LeaseToken == "" {
		t.Fatalf("第一次占用失敗：%+v %v", claim, err)
	}
	busy, err := store.ClaimRepeatStep(ctx, deleteStep.Key)
	if err != nil || busy.Acquired || busy.Completed {
		t.Fatalf("有效租約不得重複占用：%+v %v", busy, err)
	}
	if err := store.CompleteRepeatStep(ctx, deleteStep.Key, "wrong-token", application.ActionResult{Succeeded: true}); err == nil {
		t.Fatal("錯誤租約不得回寫成功")
	}
	if err := store.CompleteRepeatStep(ctx, deleteStep.Key, claim.LeaseToken, application.ActionResult{Succeeded: true}); err != nil {
		t.Fatal(err)
	}
	completed, err := store.ClaimRepeatStep(ctx, second.Steps[1].Key)
	if err != nil || !completed.Completed || completed.Acquired {
		t.Fatalf("跨事件已完成刪文不可重做：%+v %v", completed, err)
	}

	retryStep := first.Steps[1]
	retryClaim, err := store.ClaimRepeatStep(ctx, retryStep.Key)
	if err != nil || !retryClaim.Acquired {
		t.Fatalf("失敗步驟首次占用：%+v %v", retryClaim, err)
	}
	if err := store.CompleteRepeatStep(ctx, retryStep.Key, retryClaim.LeaseToken, application.ActionResult{Retryable: true, ErrorCode: "temporary"}); err != nil {
		t.Fatal(err)
	}
	nextClaim, err := store.ClaimRepeatStep(ctx, retryStep.Key)
	if err != nil || !nextClaim.Acquired || nextClaim.LeaseToken == retryClaim.LeaseToken {
		t.Fatalf("可重試失敗應取得新租約：%+v %v", nextClaim, err)
	}
	if err := store.CompleteRepeatStep(ctx, retryStep.Key, retryClaim.LeaseToken, application.ActionResult{Succeeded: true}); err == nil {
		t.Fatal("舊租約不得覆寫新租約結果")
	}
	if err := store.CompleteRepeatStep(ctx, retryStep.Key, nextClaim.LeaseToken, application.ActionResult{Succeeded: true}); err != nil {
		t.Fatal(err)
	}

	unknownStep := first.Steps[2]
	unknownClaim, err := store.ClaimRepeatStep(ctx, unknownStep.Key)
	if err != nil || !unknownClaim.Acquired {
		t.Fatalf("未知結果測試首次占用：%+v %v", unknownClaim, err)
	}
	if err := db.WithContext(ctx).Model(&repeatActionStep{}).Where("action_key = ?", unknownStep.Key).
		Update("lease_expires_at", time.Now().Add(-time.Minute)).Error; err != nil {
		t.Fatal(err)
	}
	reclaimed, err := store.ClaimRepeatStep(ctx, unknownStep.Key)
	if err != nil || !reclaimed.Acquired || reclaimed.LeaseToken == unknownClaim.LeaseToken {
		t.Fatalf("租約到期應可接續：%+v %v", reclaimed, err)
	}
	var unknownRow repeatActionStep
	if err := db.WithContext(ctx).Where("action_key = ?", unknownStep.Key).Take(&unknownRow).Error; err != nil || !unknownRow.HadUnknownOutcome || unknownRow.AttemptCount != 2 {
		t.Fatalf("外部結果未知必須留痕：%+v %v", unknownRow, err)
	}
	if err := store.CompleteRepeatStep(ctx, unknownStep.Key, unknownClaim.LeaseToken, application.ActionResult{Succeeded: true}); err == nil {
		t.Fatal("租約過期後舊執行者不得回寫")
	}
	if err := store.CompleteRepeatStep(ctx, unknownStep.Key, reclaimed.LeaseToken, application.ActionResult{Succeeded: true}); err != nil {
		t.Fatal(err)
	}

	concurrentEvent := event
	concurrentEvent.ID = fmt.Sprintf("it-repeat:%d:concurrent", seed)
	concurrentEvent.Message.MessageID = 106
	plans := []application.RepeatPlan{
		{
			Triggered: true, Count: 3, Window: 30 * time.Minute, Action: application.RepeatActionDelete,
			Mode: application.ModeEnforce, MessageIDs: []int64{106, 104, 102},
		},
		{
			Triggered: true, Count: 3, Window: 30 * time.Minute, Action: application.RepeatActionDelete,
			Mode: application.ModeEnforce, MessageIDs: []int64{106, 103, 101},
		},
	}
	outcomes := make([]application.RepeatExecution, len(plans))
	failures := make([]error, len(plans))
	var wg sync.WaitGroup
	for i := range plans {
		wg.Go(func() {
			outcomes[i], failures[i] = store.CreateOrLoadRepeatExecution(ctx, concurrentEvent, plans[i], false)
		})
	}
	wg.Wait()
	for i, failure := range failures {
		if failure != nil {
			t.Fatalf("並行計畫 %d：%v", i, failure)
		}
	}
	if !slices.Equal(outcomes[0].Steps, outcomes[1].Steps) || len(outcomes[0].Steps) != 3 {
		t.Fatalf("並行同事件必須回傳單一固定計畫：%+v %+v", outcomes[0], outcomes[1])
	}
}
