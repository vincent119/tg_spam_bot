package postgres

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	commandapp "github.com/vincent119/tg_spam_bot/internal/command/application"
	commanddomain "github.com/vincent119/tg_spam_bot/internal/command/domain"
	"github.com/vincent119/tg_spam_bot/internal/detection/application"
	"github.com/vincent119/tg_spam_bot/internal/detection/domain"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestManualFeedbackRevisionAndIsolationIntegration(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("未設定 TEST_DATABASE_URL")
	}
	ctx := t.Context()
	db, err := gorm.Open(postgres.Open(databaseURL), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := AutoMigrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := AutoMigrateManualFeedback(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := AutoMigrateManualFeedbackActions(ctx, db); err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(db)
	if err != nil {
		t.Fatal(err)
	}
	seed := time.Now().UnixNano()
	chatA, chatB, fingerprint := -seed, -seed-1, "manual-feedback-integration"
	t.Cleanup(func() {
		cleanup := db.WithContext(context.Background())
		for _, chatID := range []int64{chatA, chatB} {
			_ = cleanup.Where("chat_id=?", chatID).Delete(&scopedManualFeedbackEmbedding{}).Error
			_ = cleanup.Where("chat_id=?", chatID).Delete(&manualFeedbackAction{}).Error
			_ = cleanup.Where("chat_id=?", chatID).Delete(&manualFeedbackRevision{}).Error
			_ = cleanup.Where("chat_id=?", chatID).Delete(&manualFeedbackCurrent{}).Error
			_ = cleanup.Where("chat_id=?", chatID).Delete(&manualFeedbackEpochRow{}).Error
			_ = cleanup.Where("chat_id=?", chatID).Delete(&detectionEvent{}).Error
			_ = cleanup.Where("chat_id=?", chatID).Delete(&commandExecution{}).Error
		}
	})
	now := time.Now().UTC()
	spam := application.ManualFeedback{
		ChatID: chatA, MessageID: 12, TargetUserID: 3, OperatorID: 4, CommandUpdateID: seed + 1,
		ContentFingerprint: fingerprint, Label: domain.AILabelSpam, Category: "ad", Source: "spam_command",
		EmbeddingStatus: application.ManualFeedbackEmbeddingPending, CreatedAt: now,
	}
	first, saved, err := store.SaveFeedback(ctx, spam)
	if err != nil || !saved || first.Revision != 1 {
		t.Fatalf("首次 SaveFeedback=%+v saved=%v err=%v", first, saved, err)
	}
	replayed, saved, err := store.SaveFeedback(ctx, spam)
	if err != nil || saved || replayed.Revision != 1 {
		t.Fatalf("重送 SaveFeedback=%+v saved=%v err=%v", replayed, saved, err)
	}
	ham := spam
	ham.CommandUpdateID = seed + 2
	ham.OperatorID = 5
	ham.Label = domain.AILabelHam
	ham.Category = "normal"
	ham.Reason = "誤判"
	ham.Source = "ham_command"
	ham.EmbeddingStatus = application.ManualFeedbackEmbeddingUnavailable
	corrected, saved, err := store.SaveFeedback(ctx, ham)
	if err != nil || !saved || corrected.Revision != 2 || corrected.Label != domain.AILabelHam {
		t.Fatalf("修正 SaveFeedback=%+v saved=%v err=%v", corrected, saved, err)
	}
	if err := store.MarkFeedbackEmbeddingFailed(ctx, chatA, 12, first.Revision, "timeout"); !errors.Is(err, application.ErrStaleManualFeedback) {
		t.Fatalf("舊向量工作應失效：%v", err)
	}
	epoch, err := store.ManualFeedbackEpoch(ctx, chatA)
	if err != nil || epoch != 2 {
		t.Fatalf("ManualFeedbackEpoch=%d err=%v", epoch, err)
	}
	otherChat := spam
	otherChat.ChatID = chatB
	otherChat.CommandUpdateID = seed + 3
	if _, saved, err := store.SaveFeedback(ctx, otherChat); err != nil || !saved {
		t.Fatalf("他群 SaveFeedback saved=%v err=%v", saved, err)
	}
	for _, tt := range []struct {
		chatID int64
		label  domain.AILabel
	}{{chatA, domain.AILabelHam}, {chatB, domain.AILabelSpam}} {
		evidence, err := store.FindEffectiveFeedback(ctx, tt.chatID, fingerprint)
		if err != nil || !evidence.Present || evidence.Conflict || evidence.Label != tt.label {
			t.Fatalf("chat=%d evidence=%+v err=%v", tt.chatID, evidence, err)
		}
	}
	conflicting := spam
	conflicting.MessageID = 13
	conflicting.CommandUpdateID = seed + 4
	if _, saved, err := store.SaveFeedback(ctx, conflicting); err != nil || !saved {
		t.Fatalf("矛盾標記 saved=%v err=%v", saved, err)
	}
	evidence, err := store.FindEffectiveFeedback(ctx, chatA, fingerprint)
	if err != nil || !evidence.Present || !evidence.Conflict || evidence.Label != "" {
		t.Fatalf("矛盾 evidence=%+v err=%v", evidence, err)
	}
	var historyCount int64
	if err := db.Model(&manualFeedbackRevision{}).Where("chat_id=?", chatA).Count(&historyCount).Error; err != nil || historyCount != 3 {
		t.Fatalf("修訂歷史=%d err=%v", historyCount, err)
	}
	var sensitiveColumns int64
	err = db.Raw(`SELECT count(*) FROM information_schema.columns
		WHERE table_name IN ('manual_feedback_currents', 'manual_feedback_revisions')
		  AND column_name IN ('text', 'message_text', 'raw_text')`).Scan(&sensitiveColumns).Error
	if err != nil || sensitiveColumns != 0 {
		t.Fatalf("原文欄位=%d err=%v", sensitiveColumns, err)
	}
}

func TestManualFeedbackScopedEmbeddingIntegration(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("未設定 TEST_DATABASE_URL")
	}
	ctx := t.Context()
	db, err := gorm.Open(postgres.Open(databaseURL), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := AutoMigrateManualFeedback(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := AutoMigrateScopedManualEmbeddings(ctx, db); err != nil {
		t.Skipf("pgvector 不可用：%v", err)
	}
	store, err := NewStore(db)
	if err != nil {
		t.Fatal(err)
	}
	seed := time.Now().UnixNano()
	chatID, fingerprint := -seed, "scoped-feedback-integration"
	t.Cleanup(func() {
		cleanup := db.WithContext(context.Background())
		_ = cleanup.Where("chat_id=?", chatID).Delete(&scopedManualFeedbackEmbedding{}).Error
		_ = cleanup.Where("chat_id=?", chatID).Delete(&manualFeedbackRevision{}).Error
		_ = cleanup.Where("chat_id=?", chatID).Delete(&manualFeedbackCurrent{}).Error
		_ = cleanup.Where("chat_id=?", chatID).Delete(&manualFeedbackEpochRow{}).Error
	})
	spam := application.ManualFeedback{
		ChatID: chatID, MessageID: 12, TargetUserID: 3, OperatorID: 4, CommandUpdateID: seed + 1,
		ContentFingerprint: fingerprint, Label: domain.AILabelSpam, Category: "ad", Source: "spam_command",
		EmbeddingStatus: application.ManualFeedbackEmbeddingPending, CreatedAt: time.Now().UTC(),
	}
	first, _, err := store.SaveFeedback(ctx, spam)
	if err != nil {
		t.Fatal(err)
	}
	vector := domain.EmbeddingResult{Provider: "test", Model: "manual", Version: "v1", Dimensions: 2, Vector: []float32{0.1, 0.2}}
	input := application.ScopedFeedbackEmbedding{Feedback: first, Embedding: vector, CreatedAt: time.Now().UTC(), ExpiresAt: time.Now().UTC().Add(time.Hour)}
	if err := store.SaveScopedFeedbackEmbedding(ctx, input); err != nil {
		t.Fatal(err)
	}
	matches, err := store.SearchScopedManualSimilar(ctx, chatID, vector, 5)
	if err != nil || len(matches) != 1 || matches[0].Label != domain.AILabelSpam {
		t.Fatalf("初始 scoped matches=%+v err=%v", matches, err)
	}
	ham := spam
	ham.CommandUpdateID = seed + 2
	ham.Label = domain.AILabelHam
	ham.Category = "normal"
	second, _, err := store.SaveFeedback(ctx, ham)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveScopedFeedbackEmbedding(ctx, input); !errors.Is(err, application.ErrStaleManualFeedback) {
		t.Fatalf("舊向量重寫應被拒：%v", err)
	}
	matches, err = store.SearchScopedManualSimilar(ctx, chatID, vector, 5)
	if err != nil || len(matches) != 0 {
		t.Fatalf("舊向量仍生效：%+v err=%v", matches, err)
	}
	input.Feedback = second
	if err := store.SaveScopedFeedbackEmbedding(ctx, input); err != nil {
		t.Fatal(err)
	}
	matches, err = store.SearchScopedManualSimilar(ctx, chatID, vector, 5)
	if err != nil || len(matches) != 1 || matches[0].Label != domain.AILabelHam {
		t.Fatalf("修正後 scoped matches=%+v err=%v", matches, err)
	}
	matches, err = store.SearchScopedManualSimilar(ctx, chatID-1, vector, 5)
	if err != nil || len(matches) != 0 {
		t.Fatalf("跨群 scoped matches=%+v err=%v", matches, err)
	}
}

func TestManualFeedbackActionAndAuditTargetIntegration(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("未設定 TEST_DATABASE_URL")
	}
	ctx := t.Context()
	db, err := gorm.Open(postgres.Open(databaseURL), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := AutoMigrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := AutoMigrateManualFeedbackActions(ctx, db); err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(db)
	if err != nil {
		t.Fatal(err)
	}
	seed := time.Now().UnixNano()
	chatID, eventID := -seed, fmt.Sprintf("tg:%d", seed+10)
	t.Cleanup(func() {
		cleanup := db.WithContext(context.Background())
		_ = cleanup.Where("chat_id=?", chatID).Delete(&manualFeedbackAction{}).Error
		_ = cleanup.Where("chat_id=?", chatID).Delete(&detectionEvent{}).Error
		_ = cleanup.Where("chat_id=?", chatID).Delete(&commandExecution{}).Error
	})
	if err := db.Create(&detectionEvent{EventID: eventID, UpdateID: seed + 10, ChatID: chatID, MessageID: 12, UserID: 3, ContentFingerprint: "fp", CreatedAt: time.Now().UTC()}).Error; err != nil {
		t.Fatal(err)
	}
	if _, found, err := store.FindFeedbackTarget(ctx, chatID-1, eventID); err != nil || found {
		t.Fatalf("他群稽核 found=%v err=%v", found, err)
	}
	target, found, err := store.FindFeedbackTarget(ctx, chatID, eventID)
	if err != nil || !found || target.MessageID != 12 || target.ContentFingerprint != "fp" {
		t.Fatalf("本群稽核 target=%+v found=%v err=%v", target, found, err)
	}
	command := commanddomain.Command{
		ChatID: chatID, UpdateID: seed + 20, MessageID: 13, Actor: commanddomain.Actor{ID: 4},
		Target: &commanddomain.Target{ID: 3}, TargetMessage: 12, Name: commanddomain.NameSpam,
	}
	if claim, err := store.ClaimCommand(ctx, command); err != nil || !claim.Acquired {
		t.Fatalf("ClaimCommand=%+v err=%v", claim, err)
	}
	if err := store.PlanFeedbackActions(ctx, command, []commandapp.FeedbackActionKind{commandapp.FeedbackActionDelete, commandapp.FeedbackActionBan}); err != nil {
		t.Fatal(err)
	}
	if err := store.CompleteFeedbackAction(ctx, command, commandapp.FeedbackActionDelete, true, false, ""); err != nil {
		t.Fatal(err)
	}
	if err := store.CompleteFeedbackAction(ctx, command, commandapp.FeedbackActionBan, false, true, "rate_limited"); err != nil {
		t.Fatal(err)
	}
	var actions []manualFeedbackAction
	if err := db.Where("chat_id=?", chatID).Order("kind ASC").Find(&actions).Error; err != nil || len(actions) != 2 {
		t.Fatalf("actions=%+v err=%v", actions, err)
	}
	statuses := map[string]string{}
	for _, action := range actions {
		statuses[action.Kind] = action.Status
	}
	if statuses["delete"] != "completed" || statuses["ban"] != "failed" {
		t.Fatalf("部分失敗紀錄錯誤：%+v", actions)
	}
}
