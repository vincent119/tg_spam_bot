package postgres

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	autoreplyapp "github.com/vincent119/tg_spam_bot/internal/autoreply/application"
	commanddomain "github.com/vincent119/tg_spam_bot/internal/command/domain"
	"github.com/vincent119/tg_spam_bot/internal/detection/application"
	"github.com/vincent119/tg_spam_bot/internal/detection/domain"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestStoreIntegration(t *testing.T) {
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
	var tableComment string
	if err := db.Raw("SELECT obj_description('command_executions'::regclass)").Scan(&tableComment).Error; err != nil || tableComment == "" {
		t.Fatalf("command_executions table comment=%q err=%v", tableComment, err)
	}
	store, _ := NewStore(db)
	if _, err := NewStore(nil); err == nil {
		t.Fatal("nil DB 應失敗")
	}
	if err := AutoMigrate(ctx, nil); err == nil {
		t.Fatal("nil DB AutoMigrate 應失敗")
	}
	seed := time.Now().UnixNano()
	chatID, userID := seed, seed+1
	t.Cleanup(func() {
		cleanup := db.WithContext(context.Background())
		_ = cleanup.Where("event_id LIKE ?", fmt.Sprintf("it-%d-%%", seed)).Delete(&enforcementAction{}).Error
		_ = cleanup.Where("chat_id = ?", chatID).Delete(&violation{}).Error
		_ = cleanup.Where("chat_id = ?", chatID).Delete(&detectionEvent{}).Error
		_ = cleanup.Where("chat_id = ?", chatID).Delete(&aiDetectionEvent{}).Error
		_ = cleanup.Where("chat_id = ?", chatID).Delete(&commandExecution{}).Error
		_ = cleanup.Where("chat_id = ?", chatID).Delete(&autoReplyExecution{}).Error
		_ = cleanup.Where("chat_id = ?", chatID).Delete(&semanticManualSample{}).Error
		_ = cleanup.Where("chat_id = ?", chatID).Delete(&trustedMember{}).Error
	})

	claimedUpdate, err := store.Claim(ctx, seed+500)
	if err != nil || !claimedUpdate {
		t.Fatalf("Claim()=%v, %v", claimedUpdate, err)
	}
	claimedUpdate, err = store.Claim(ctx, seed+500)
	if err != nil || claimedUpdate {
		t.Fatalf("重複 Claim()=%v, %v", claimedUpdate, err)
	}
	if err := store.Complete(ctx, seed+500); err != nil {
		t.Fatal(err)
	}
	if claimed, err := store.Claim(ctx, seed+501); err != nil || !claimed {
		t.Fatalf("Claim()=%v, %v", claimed, err)
	}
	if err := store.Release(ctx, seed+501); err != nil {
		t.Fatal(err)
	}

	autoReplyEvent := autoreplyapp.Event{ChatID: chatID, UpdateID: seed + 700, MessageID: 700, UserID: userID, RuleID: "download_page", CreatedAt: time.Now().UTC()}
	autoClaim, err := store.ClaimAutoReply(ctx, autoReplyEvent)
	if err != nil || !autoClaim.Acquired {
		t.Fatalf("ClaimAutoReply()=%+v, %v", autoClaim, err)
	}
	if err := store.CompleteAutoReply(ctx, autoReplyEvent); err != nil {
		t.Fatal(err)
	}
	autoClaim, err = store.ClaimAutoReply(ctx, autoReplyEvent)
	if err != nil || autoClaim.Acquired || autoClaim.Existing == nil || autoClaim.Existing.Status != "completed" {
		t.Fatalf("重複 ClaimAutoReply()=%+v, %v", autoClaim, err)
	}
	retryEvent := autoreplyapp.Event{ChatID: chatID, UpdateID: seed + 701, MessageID: 701, UserID: userID, RuleID: "download_page", CreatedAt: time.Now().UTC()}
	retryClaim, err := store.ClaimAutoReply(ctx, retryEvent)
	if err != nil || !retryClaim.Acquired {
		t.Fatalf("retry ClaimAutoReply()=%+v, %v", retryClaim, err)
	}
	if err := store.FailAutoReply(ctx, retryEvent, autoreplyapp.Result{Status: "failed", ErrorText: "temporary", Retryable: true}); err != nil {
		t.Fatal(err)
	}
	retryClaim, err = store.ClaimAutoReply(ctx, retryEvent)
	if err != nil || !retryClaim.Acquired {
		t.Fatalf("retryable ClaimAutoReply()=%+v, %v", retryClaim, err)
	}

	if err := db.Create(&trustedMember{ChatID: chatID, UserID: userID, Reason: "integration", Enabled: true, CreatedAt: time.Now().UTC()}).Error; err != nil {
		t.Fatal(err)
	}
	trusted, reason, err := store.IsExempt(ctx, chatID, userID)
	if err != nil || !trusted || reason != "integration" {
		t.Fatalf("IsExempt()=%v, %q, %v", trusted, reason, err)
	}
	trusted, _, err = store.IsExempt(ctx, chatID, userID+999)
	if err != nil || trusted {
		t.Fatalf("未登錄 IsExempt()=%v, %v", trusted, err)
	}

	observeEvent := application.Event{
		ID: fmt.Sprintf("it-%d-observe", seed), Message: domain.Message{UpdateID: seed + 600, ChatID: chatID, MessageID: 600, UserID: userID},
		Fingerprint: "fingerprint", Mode: application.ModeObserve, Result: domain.Result{RuleVersion: "it"}, CreatedAt: time.Now().UTC(),
	}
	if err := store.RecordDetection(ctx, observeEvent); err != nil {
		t.Fatal(err)
	}

	manualSample, created, err := store.CreateManualSample(ctx, application.ManualSample{
		ChatID: chatID, MessageID: 900, TargetUserID: userID, OperatorID: seed + 3,
		ContentFingerprint: "manual-fingerprint", Label: domain.AILabelSpam,
		Category: "uncategorized_spam", Source: "feedspam", CreatedAt: time.Now().UTC(),
	})
	if err != nil || !created || manualSample.Status != application.ManualSampleStatusPendingEmbedding {
		t.Fatalf("CreateManualSample()=%+v created=%v err=%v", manualSample, created, err)
	}
	replayedSample, created, err := store.CreateManualSample(ctx, application.ManualSample{
		ChatID: chatID, MessageID: 900, TargetUserID: userID, OperatorID: seed + 4,
		ContentFingerprint: "manual-fingerprint", Label: domain.AILabelSpam,
		Category: "other", Source: "feedspam", CreatedAt: time.Now().UTC(),
	})
	if err != nil || created || replayedSample.ID != manualSample.ID {
		t.Fatalf("重複 CreateManualSample()=%+v created=%v err=%v", replayedSample, created, err)
	}
	pending, err := store.PendingManualSamples(ctx, 10)
	if err != nil || len(pending) == 0 {
		t.Fatalf("PendingManualSamples()=%+v err=%v", pending, err)
	}
	if err := store.MarkManualSampleEmbeddingFailed(ctx, manualSample.ID, "provider_timeout", "temporary", true); err != nil {
		t.Fatal(err)
	}
	pending, err = store.PendingManualSamples(ctx, 10)
	if err != nil || len(pending) == 0 {
		t.Fatalf("retryable PendingManualSamples()=%+v err=%v", pending, err)
	}
	if err := store.MarkManualSampleEmbeddingCompleted(ctx, manualSample.ID, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	pending, err = store.PendingManualSamples(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range pending {
		if item.ID == manualSample.ID {
			t.Fatalf("完成樣本不應仍在 pending：%+v", pending)
		}
	}
	var manualSensitiveColumnCount int64
	if err := db.Raw(`
		SELECT count(*)
		FROM information_schema.columns
		WHERE table_name = 'semantic_manual_samples'
		  AND column_name IN ('text', 'message_text', 'raw_text', 'raw_response', 'api_key', 'bot_token', 'webhook_secret')
	`).Scan(&manualSensitiveColumnCount).Error; err != nil || manualSensitiveColumnCount != 0 {
		t.Fatalf("semantic_manual_samples 敏感欄位數=%d err=%v", manualSensitiveColumnCount, err)
	}

	aiEvent := application.AIDetectionEvent{
		ChatID: chatID, UpdateID: seed + 800, MessageID: 800, UserID: userID,
		ContentFingerprint: "ai-fingerprint", Provider: "openai_compatible", Model: "classifier",
		PromptVersion: "ai-spam-v1", SchemaVersion: "v1", RuleVersion: "it", CreatedAt: time.Now().UTC(),
	}
	aiClaim, err := store.ClaimAIDetection(ctx, aiEvent)
	if err != nil || !aiClaim.Acquired {
		t.Fatalf("ClaimAIDetection()=%+v, %v", aiClaim, err)
	}
	aiResult := domain.AIClassifyResult{
		Label: domain.AILabelSpam, Category: "ad", Confidence: 0.91,
		ConfidenceSource: domain.AIConfidenceModelReported, ReasonCode: "commercial_solicitation",
		Evidence: []string{"導流摘要"}, SafeAction: domain.AISafeActionDelete, PromptVersion: "ai-spam-v1",
	}
	if err := store.CompleteAIDetection(ctx, aiEvent, aiResult); err != nil {
		t.Fatal(err)
	}
	aiClaim, err = store.ClaimAIDetection(ctx, aiEvent)
	if err != nil || aiClaim.Acquired || aiClaim.Existing == nil || aiClaim.Existing.Status != "completed" || aiClaim.Existing.Result.Label != domain.AILabelSpam {
		t.Fatalf("重複 ClaimAIDetection()=%+v, %v", aiClaim, err)
	}
	cachedAI, found, err := store.FindCachedAIDetection(ctx, application.AIDetectionCacheKey{
		ContentFingerprint: "ai-fingerprint", Provider: "openai_compatible", Model: "classifier",
		PromptVersion: "ai-spam-v1", RuleVersion: "it", CacheTTL: time.Hour, Now: time.Now().UTC(),
	})
	if err != nil || !found || cachedAI.Result.ReasonCode != "commercial_solicitation" {
		t.Fatalf("FindCachedAIDetection()=%+v found=%v err=%v", cachedAI, found, err)
	}
	_, found, err = store.FindCachedAIDetection(ctx, application.AIDetectionCacheKey{
		ContentFingerprint: "ai-fingerprint", Provider: "openai_compatible", Model: "other",
		PromptVersion: "ai-spam-v1", RuleVersion: "it", CacheTTL: time.Hour, Now: time.Now().UTC(),
	})
	if err != nil || found {
		t.Fatalf("不同模型不應命中快取：found=%v err=%v", found, err)
	}
	retryAIEvent := application.AIDetectionEvent{
		ChatID: chatID, UpdateID: seed + 801, MessageID: 801, UserID: userID,
		ContentFingerprint: "retry-ai-fingerprint", Provider: "bedrock", Model: "classifier",
		PromptVersion: "ai-spam-v1", SchemaVersion: "v1", RuleVersion: "it", CreatedAt: time.Now().UTC(),
	}
	retryAIClaim, err := store.ClaimAIDetection(ctx, retryAIEvent)
	if err != nil || !retryAIClaim.Acquired {
		t.Fatalf("retry ClaimAIDetection()=%+v, %v", retryAIClaim, err)
	}
	if err := store.FailAIDetection(ctx, retryAIEvent, application.AIDetectionResult{Status: "failed", ErrorCode: "provider_timeout", ErrorText: "temporary", Retryable: true}); err != nil {
		t.Fatal(err)
	}
	retryAIClaim, err = store.ClaimAIDetection(ctx, retryAIEvent)
	if err != nil || !retryAIClaim.Acquired {
		t.Fatalf("retryable ClaimAIDetection()=%+v, %v", retryAIClaim, err)
	}
	var sensitiveColumnCount int64
	if err := db.Raw(`
		SELECT count(*)
		FROM information_schema.columns
		WHERE table_name = 'ai_detection_events'
		  AND column_name IN ('text', 'message_text', 'raw_response', 'api_key', 'bot_token', 'webhook_secret')
	`).Scan(&sensitiveColumnCount).Error; err != nil || sensitiveColumnCount != 0 {
		t.Fatalf("ai_detection_events 敏感欄位數=%d err=%v", sensitiveColumnCount, err)
	}

	var firstAction application.EnforcementAction
	for i := 1; i <= 4; i++ {
		event := application.Event{
			ID:          fmt.Sprintf("it-%d-%d", seed, i),
			Message:     domain.Message{UpdateID: seed + int64(i), ChatID: chatID, MessageID: int64(i), UserID: userID},
			Fingerprint: "fingerprint", Mode: application.ModeEnforce,
			Result:    domain.Result{Spam: true, CategoryID: "generic", Severity: domain.SeverityNormal, Action: domain.ActionProgressive, RuleVersion: "it"},
			CreatedAt: time.Now().UTC(),
		}
		count, actions, err := store.Create(ctx, event)
		if err != nil {
			t.Fatal(err)
		}
		if count != i || len(actions) != 2 {
			t.Fatalf("count = %d actions = %v", count, actions)
		}
		if i == 1 {
			firstAction = actions[0]
		}
	}
	if err := store.CompleteAction(ctx, firstAction.Key, application.ActionResult{Succeeded: true, EndedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	target := commanddomain.Target{ID: userID}
	command, err := commanddomain.NewCommand(commanddomain.Command{UpdateID: seed + 100, ChatID: chatID, MessageID: 100, Actor: commanddomain.Actor{ID: seed + 2}, Target: &target, TargetMessage: 1, Name: commanddomain.NameWarn, Args: "人工測試"})
	if err != nil {
		t.Fatal(err)
	}
	claim, err := store.ClaimCommand(ctx, command)
	if err != nil || !claim.Acquired {
		t.Fatalf("ClaimCommand() = %v, %v", claim, err)
	}
	if _, err := store.AddManualWarning(ctx, command, commanddomain.Reason("人工測試"), time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	completed := commanddomain.Result{Status: "completed", Message: "已完成", ErrorCode: "", Retryable: false}
	if err := store.CompleteCommand(ctx, command, completed); err != nil {
		t.Fatal(err)
	}
	replayed, err := store.ClaimCommand(ctx, command)
	if err != nil || replayed.Acquired || replayed.Existing == nil || *replayed.Existing != completed {
		t.Fatalf("重送 ClaimCommand() = %+v, %v", replayed, err)
	}
	summary, err := store.Warnings(ctx, chatID, userID, time.Now().Add(-30*24*time.Hour))
	if err != nil || summary.Total != 5 || summary.Manual != 1 || summary.Automatic != 4 {
		t.Fatalf("Warnings() = %+v, %v", summary, err)
	}
	cleared, err := store.ClearWarnings(ctx, command, commanddomain.Reason("清除測試"), time.Now().UTC())
	if err != nil || cleared != 5 {
		t.Fatalf("ClearWarnings() = %d, %v", cleared, err)
	}

	missingAuditTarget := commanddomain.Target{ID: userID + 1000}
	missingAuditCommand, _ := commanddomain.NewCommand(commanddomain.Command{
		UpdateID: seed + 200, ChatID: chatID, MessageID: 200, Actor: commanddomain.Actor{ID: seed + 2},
		Target: &missingAuditTarget, TargetMessage: 1, Name: commanddomain.NameWarn,
	})
	if _, err := store.AddManualWarning(ctx, missingAuditCommand, commanddomain.Reason("應回滾"), time.Now().UTC()); err == nil {
		t.Fatal("缺少 command execution 時應回滾人工警告")
	}
	rollbackSummary, err := store.Warnings(ctx, chatID, missingAuditTarget.ID, time.Now().Add(-30*24*time.Hour))
	if err != nil || rollbackSummary.Total != 0 {
		t.Fatalf("部分失敗未回滾：summary=%+v err=%v", rollbackSummary, err)
	}
}
