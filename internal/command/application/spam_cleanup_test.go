package application

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/vincent119/tg_spam_bot/internal/command/domain"
	detectionapp "github.com/vincent119/tg_spam_bot/internal/detection/application"
)

type spamDuplicateFinder struct {
	ids                    []int64
	err                    error
	chatID, userID, target int64
	since                  time.Time
	limit                  int
}

func (s *spamDuplicateFinder) FindDuplicateMessageIDs(_ context.Context, chatID, userID, target int64, since time.Time, limit int) ([]int64, error) {
	s.chatID, s.userID, s.target, s.since, s.limit = chatID, userID, target, since, limit
	return slices.Clone(s.ids), s.err
}

func TestSpamFeedbackDuplicateCleanup(t *testing.T) {
	t.Parallel()
	many := make([]int64, 120)
	for i := range many {
		many[i] = int64(i + 100)
	}
	tests := []struct {
		name string
		ids  []int64
		want []int64
	}{
		{name: "包含原目標並去重", ids: []int64{10, 12, 10, 14, 0, -1}, want: []int64{12, 10, 14}},
		{name: "無追蹤紀錄仍清除目標", want: []int64{12}},
		{name: "最多一百則且目標優先", ids: many, want: append([]int64{12}, many[:99]...)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			now := time.Date(2026, 10, 7, 2, 0, 0, 0, time.UTC)
			tg := &telegramSpy{admins: map[int64]bool{1: true}}
			store := &storeStub{claim: domain.Claim{Acquired: true}}
			feedback := &feedbackSubmitterSpy{result: detectionapp.ManualFeedbackResult{Saved: true, EmbeddingStatus: detectionapp.ManualFeedbackEmbeddingDisabled}}
			actions := &feedbackActionSpy{}
			finder := &spamDuplicateFinder{ids: tt.ids}
			handler := newFeedbackHandler(t, tg, store, feedback, &feedbackTargetSpy{}, actions)
			handler.duplicates, handler.clock = finder, fixedClock{now: now}
			if err := handler.Handle(t.Context(), feedbackCommand(domain.NameSpam, "ban")); err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(tg.deleteCalls, tt.want) || !slices.Equal(actions.messageIDs, tt.want) || !slices.Equal(actions.deleteIDs, tt.want) {
				t.Fatalf("刪除=%v 計畫=%v 完成=%v 預期=%v", tg.deleteCalls, actions.messageIDs, actions.deleteIDs, tt.want)
			}
			if finder.chatID != -1001 || finder.userID != 2 || finder.target != 12 || finder.limit != 100 || !finder.since.Equal(now.Add(-48*time.Hour)) {
				t.Fatalf("查詢範圍=%+v", finder)
			}
			if feedback.calls != 1 || tg.banned != 2 || store.completed != "completed" || !strings.Contains(store.result.Message, "語意向量功能未啟用") {
				t.Fatalf("標記=%d 封鎖=%d 回覆=%+v", feedback.calls, tg.banned, store.result)
			}
			if len(tt.want) > 1 && !strings.Contains(store.result.Message, fmt.Sprintf("已清除 %d 則相同訊息", len(tt.want))) {
				t.Fatalf("清除數量回覆=%s", store.result.Message)
			}
		})
	}
}

func TestSpamFeedbackDuplicateCleanupFailures(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		lookupErr   error
		completeErr error
		deleteErr   error
		wantDeletes []int64
		wantReply   string
		wantBan     int64
	}{
		{name: "查詢失敗保留標記不處置", lookupErr: errors.New("db unavailable"), wantReply: "查詢重複訊息失敗"},
		{name: "單筆失敗仍清其他目標", deleteErr: errors.New("刪除失敗"), wantDeletes: []int64{12, 10, 14}, wantReply: "已清除 2 則相同訊息，1 則刪除失敗", wantBan: 2},
		{name: "結果保存失敗停止後續動作", completeErr: errors.New("db unavailable"), wantDeletes: []int64{12}, wantReply: "結果未確認"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tg := &telegramSpy{admins: map[int64]bool{1: true}, deleteErrors: map[int64]error{10: tt.deleteErr}}
			store := &storeStub{claim: domain.Claim{Acquired: true}}
			feedback := &feedbackSubmitterSpy{}
			actions := &feedbackActionSpy{completeErr: tt.completeErr}
			handler := newFeedbackHandler(t, tg, store, feedback, &feedbackTargetSpy{}, actions)
			handler.duplicates = &spamDuplicateFinder{ids: []int64{10, 14}, err: tt.lookupErr}
			if err := handler.Handle(t.Context(), feedbackCommand(domain.NameSpam, "ban")); err != nil {
				t.Fatal(err)
			}
			if feedback.calls != 1 || !slices.Equal(tg.deleteCalls, tt.wantDeletes) || tg.banned != tt.wantBan || store.completed != "partial" || !strings.Contains(store.result.Message, tt.wantReply) {
				t.Fatalf("標記=%d 刪除=%v 封鎖=%d 回覆=%+v", feedback.calls, tg.deleteCalls, tg.banned, store.result)
			}
		})
	}
}
