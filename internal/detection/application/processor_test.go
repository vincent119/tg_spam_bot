package application_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	redislib "github.com/redis/go-redis/v9"

	"github.com/vincent119/tg_spam_bot/internal/detection/application"
	"github.com/vincent119/tg_spam_bot/internal/detection/domain"
	"github.com/vincent119/tg_spam_bot/internal/detection/infra/memory"
	redisstore "github.com/vincent119/tg_spam_bot/internal/detection/infra/redis"
)

type detectorStub struct{ result domain.Result }

func (d detectorStub) Detect(domain.Message, ...string) domain.Result { return d.result }

type telegramSpy struct {
	actions []string
	failOn  string
}

func (s *telegramSpy) DeleteMessage(context.Context, int64, int64) error {
	s.actions = append(s.actions, "delete")
	if s.failOn == "delete" {
		return errors.New("temporary delete failure")
	}
	return nil
}

func TestProcessorModes(t *testing.T) {
	t.Parallel()
	result := domain.Result{Spam: true, Severity: domain.SeverityNormal, Action: domain.ActionProgressive, CategoryID: "ad", RuleVersion: "v1"}
	for _, tt := range []struct {
		name string
		mode application.Mode
		want int
	}{
		{name: "observe", mode: application.ModeObserve, want: 0},
		{name: "delete only", mode: application.ModeDeleteOnly, want: 1},
		{name: "enforce", mode: application.ModeEnforce, want: 2},
	} {
		t.Run(tt.name, func(t *testing.T) {
			store := memory.NewStore(time.Minute, 100)
			telegram := &telegramSpy{}
			processor := application.NewProcessor(detectorStub{result}, store, store, store, store, telegram, tt.mode, []byte("01234567890123456789012345678901"))
			if _, err := processor.Process(context.Background(), domain.Message{UpdateID: 1, ChatID: 2, MessageID: 3, UserID: 4, Text: "ad"}); err != nil {
				t.Fatal(err)
			}
			if len(telegram.actions) != tt.want {
				t.Fatalf("actions = %v, want count %d", telegram.actions, tt.want)
			}
		})
	}
}

func TestProcessorReturnsPartialActionFailure(t *testing.T) {
	t.Parallel()
	store := memory.NewStore(time.Minute, 100)
	telegram := &telegramSpy{failOn: "delete"}
	result := domain.Result{Spam: true, Severity: domain.SeverityNormal, Action: domain.ActionProgressive, CategoryID: "ad", RuleVersion: "v1"}
	processor := application.NewProcessor(detectorStub{result}, store, store, store, store, telegram, application.ModeEnforce, []byte("01234567890123456789012345678901"))
	_, err := processor.Process(context.Background(), domain.Message{UpdateID: 1, ChatID: 2, MessageID: 3, UserID: 4, Text: "ad"})
	if err == nil {
		t.Fatal("expected action failure")
	}
}

func (s *telegramSpy) SendWarning(context.Context, int64, int64, string) error {
	s.actions = append(s.actions, "warn")
	return nil
}

func (s *telegramSpy) RestrictMember(context.Context, int64, int64, time.Time) error {
	s.actions = append(s.actions, "restrict")
	return nil
}

func (s *telegramSpy) BanMember(context.Context, int64, int64) error {
	s.actions = append(s.actions, "ban")
	return nil
}

func TestProcessorCriticalAndDuplicate(t *testing.T) {
	t.Parallel()
	store := memory.NewStore(time.Minute, 100)
	telegram := &telegramSpy{}
	result := domain.Result{Spam: true, Severity: domain.SeverityCritical, Action: domain.ActionBan, CategoryID: "counterfeit", RuleVersion: "v1"}
	processor := application.NewProcessor(detectorStub{result}, store, store, store, store, telegram, application.ModeEnforce, []byte("01234567890123456789012345678901"))
	message := domain.Message{UpdateID: 1, ChatID: 2, MessageID: 3, UserID: 4, Text: "假鈔出售"}
	if _, err := processor.Process(context.Background(), message); err != nil {
		t.Fatal(err)
	}
	if result, err := processor.Process(context.Background(), message); err != nil {
		t.Fatal(err)
	} else if !result.Duplicate {
		t.Fatalf("第二次處理應標示 duplicate：%+v", result)
	}
	if len(telegram.actions) != 2 || telegram.actions[0] != "delete" || telegram.actions[1] != "ban" {
		t.Fatalf("actions = %v", telegram.actions)
	}
}

func TestProcessorExemption(t *testing.T) {
	t.Parallel()
	store := memory.NewStore(time.Minute, 100)
	store.Trust(2, 4, "trusted")
	telegram := &telegramSpy{}
	result := domain.Result{Spam: true, Severity: domain.SeverityCritical, Action: domain.ActionBan}
	processor := application.NewProcessor(detectorStub{result}, store, store, store, store, telegram, application.ModeEnforce, []byte("01234567890123456789012345678901"))
	if result, err := processor.Process(context.Background(), domain.Message{UpdateID: 1, ChatID: 2, MessageID: 3, UserID: 4, Text: "spam"}); err != nil {
		t.Fatal(err)
	} else if !result.Exempt {
		t.Fatalf("可信任成員應標示 exempt：%+v", result)
	}
	if len(telegram.actions) != 0 {
		t.Fatalf("trusted member actions = %v", telegram.actions)
	}
}

type repeatBehaviorSpy struct {
	application.BehaviorStore
	calls int
	err   error
}

func (s *repeatBehaviorSpy) Observe(ctx context.Context, message domain.Message, fingerprint string) ([]string, error) {
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	return s.BehaviorStore.Observe(ctx, message, fingerprint)
}

type repeatViolationSpy struct {
	application.ViolationStore
	events      []application.Event
	creates     int
	failRecords int
}

func (s *repeatViolationSpy) RecordDetection(ctx context.Context, event application.Event) error {
	s.events = append(s.events, event)
	if s.failRecords > 0 {
		s.failRecords--
		return errors.New("測試用紀錄失敗")
	}
	return s.ViolationStore.RecordDetection(ctx, event)
}

func (s *repeatViolationSpy) Create(ctx context.Context, event application.Event) (int, []application.EnforcementAction, error) {
	s.creates++
	s.events = append(s.events, event)
	return s.ViolationStore.Create(ctx, event)
}

type repeatProcessorOptions struct {
	aiMode   application.Mode
	appMode  application.Mode
	category *domain.Category
}

type repeatProcessorHarness struct {
	processor  *application.Processor
	store      *memory.Store
	behaviors  *repeatBehaviorSpy
	violations *repeatViolationSpy
	telegram   *telegramSpy
	classifier *aiClassifierStub
}

func newRepeatProcessor(t *testing.T, opts repeatProcessorOptions) *repeatProcessorHarness {
	t.Helper()
	category := domain.Category{
		ID: "test", Enabled: true, Severity: domain.SeverityNormal,
		Action: domain.ActionProgressive, Terms: []string{"規則詞"}, Weight: 40, Threshold: 60,
		RequireAny: []string{domain.SignalRepeatedContent},
	}
	if opts.category != nil {
		category = *opts.category
	}
	detector, err := domain.NewDetector(domain.RuleSet{Version: "repeat-test", Categories: []domain.Category{category}}, domain.NewNormalizer(nil, 4096), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	store := memory.NewStore(time.Minute, 100)
	h := &repeatProcessorHarness{
		store: store, behaviors: &repeatBehaviorSpy{BehaviorStore: store},
		violations: &repeatViolationSpy{ViolationStore: store}, telegram: &telegramSpy{},
		classifier: &aiClassifierStub{result: domain.AIClassifyResult{
			Label: domain.AILabelSpam, Category: "ai_ad", Confidence: 0.95,
			ConfidenceSource: domain.AIConfidenceModelReported, ReasonCode: "commercial_solicitation", SafeAction: domain.AISafeActionDelete,
		}},
	}
	var processorOptions []application.ProcessorOption
	if opts.aiMode != "" {
		ai := newTestAIProcessor(t, opts.aiMode, h.classifier, &aiStoreStub{claimAcquired: true}, nil)
		processorOptions = append(processorOptions, application.WithAIDetectionProcessor(ai))
	}
	if opts.appMode == "" {
		opts.appMode = application.ModeEnforce
	}
	h.processor = application.NewProcessor(detector, store, store, h.behaviors, h.violations, h.telegram,
		opts.appMode, []byte("01234567890123456789012345678901"), processorOptions...)
	return h
}

func repeatMessage(id int64) domain.Message {
	return domain.Message{UpdateID: id, ChatID: 2, UserID: 4, MessageID: id, Text: "有码来洗米 日挣1W"}
}

func processRepeat(t *testing.T, h *repeatProcessorHarness, message domain.Message) application.ProcessResult {
	t.Helper()
	result, err := h.processor.Process(t.Context(), message)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestProcessorRepeatedContentDoesNotAffectRules(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name      string
		action    domain.Action
		severity  domain.Severity
		threshold int
	}{
		{name: "重複不加分", action: domain.ActionProgressive, severity: domain.SeverityNormal, threshold: 60},
		{name: "重複不滿足封鎖條件", action: domain.ActionBan, severity: domain.SeverityCritical, threshold: 40},
	} {
		t.Run(tt.name, func(t *testing.T) {
			category := domain.Category{
				ID: "test", Enabled: true, Terms: []string{"規則詞"}, Weight: 40,
				Threshold: tt.threshold, Action: tt.action, Severity: tt.severity, RequireAny: []string{domain.SignalRepeatedContent},
			}
			h := newRepeatProcessor(t, repeatProcessorOptions{category: &category})
			for id := int64(1); id <= 3; id++ {
				message := repeatMessage(id)
				message.Text = "規則詞"
				if result := processRepeat(t, h, message); result.Spam {
					t.Fatalf("第 %d 則不應因重複變成垃圾", id)
				}
				event := h.violations.events[len(h.violations.events)-1]
				if event.Result.Score != 40 || slices.Contains(event.Result.Signals, domain.SignalRepeatedContent) != (id == 3) {
					t.Fatalf("分數或稽核訊號不符：%+v", event.Result)
				}
			}
			if h.violations.creates != 0 || len(h.telegram.actions) != 0 {
				t.Fatal("重複線索不可自行建立違規或處置")
			}
		})
	}
}

func TestProcessorRepeatedContentWithoutAI(t *testing.T) {
	t.Parallel()
	h := newRepeatProcessor(t, repeatProcessorOptions{})
	for id := int64(1); id <= 3; id++ {
		message := repeatMessage(id)
		message.Text = "大家早安"
		if result := processRepeat(t, h, message); result.Spam {
			t.Fatal("正常短句重複不應直接處罰")
		}
	}
	last := h.violations.events[2]
	if last.Result.Score != 0 || !slices.Contains(last.Result.Signals, domain.SignalRepeatedContent) || h.violations.creates != 0 || len(h.telegram.actions) != 0 {
		t.Fatalf("未啟用 AI 時應只記錄訊號：%+v，處置=%v", last.Result, h.telegram.actions)
	}
}

func TestProcessorRepeatedContentExemption(t *testing.T) {
	t.Parallel()
	h := newRepeatProcessor(t, repeatProcessorOptions{aiMode: application.ModeEnforce})
	h.store.Trust(2, 4, "可信任")
	for id := int64(1); id <= 3; id++ {
		if result := processRepeat(t, h, repeatMessage(id)); !result.Exempt {
			t.Fatal("可信任成員應豁免")
		}
	}
	if h.behaviors.calls != 0 || h.classifier.calls != 0 || len(h.violations.events) != 0 || len(h.telegram.actions) != 0 {
		t.Fatal("豁免應在觀測與 AI 前略過")
	}
}

func TestProcessorRepeatedContentDuplicate(t *testing.T) {
	t.Parallel()
	h := newRepeatProcessor(t, repeatProcessorOptions{aiMode: application.ModeObserve})
	for id := int64(1); id <= 3; id++ {
		processRepeat(t, h, repeatMessage(id))
	}
	if result := processRepeat(t, h, repeatMessage(3)); !result.Duplicate {
		t.Fatal("已完成更新應略過")
	}
	if h.behaviors.calls != 3 || h.classifier.calls != 1 || len(h.violations.events) != 3 || len(h.telegram.actions) != 0 {
		t.Fatal("已完成重送不可再次觀測、呼叫 AI 或紀錄")
	}
}

func useRedisRepeatBehavior(t *testing.T, h *repeatProcessorHarness) *redislib.Client {
	t.Helper()
	server := miniredis.RunT(t)
	client := redislib.NewClient(&redislib.Options{Addr: server.Addr()})
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Error(err)
		}
	})
	behavior, err := redisstore.NewBehaviorStore(client, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	h.behaviors.BehaviorStore = behavior
	return client
}

func assertRepeatCount(t *testing.T, client *redislib.Client, event application.Event, want int64) {
	t.Helper()
	key := fmt.Sprintf("spam:repeat:v2:%d:%d:%s", event.Message.ChatID, event.Message.UserID, event.Fingerprint)
	count, err := client.ZCard(t.Context(), key).Result()
	if err != nil || count != want {
		t.Fatalf("唯一訊息計數=%d，預期=%d，錯誤=%v", count, want, err)
	}
}

func TestProcessorRepeatedContentRetryDoesNotRecount(t *testing.T) {
	t.Parallel()
	t.Run("紀錄失敗後重試", func(t *testing.T) {
		h := newRepeatProcessor(t, repeatProcessorOptions{aiMode: application.ModeObserve})
		client := useRedisRepeatBehavior(t, h)
		h.violations.failRecords = 1
		if _, err := h.processor.Process(t.Context(), repeatMessage(1)); err == nil {
			t.Fatal("應回傳紀錄失敗")
		}
		if result := processRepeat(t, h, repeatMessage(1)); result.Duplicate {
			t.Fatal("失敗更新應釋放占用以供重試")
		}
		assertRepeatCount(t, client, h.violations.events[0], 1)
		processRepeat(t, h, repeatMessage(2))
		processRepeat(t, h, repeatMessage(3))
		assertRepeatCount(t, client, h.violations.events[0], 3)
		if h.classifier.calls != 1 || len(h.telegram.actions) != 0 {
			t.Fatal("重送不應提早觸發 AI 或直接處置")
		}
	})
	t.Run("Telegram 失敗後重試", func(t *testing.T) {
		h := newRepeatProcessor(t, repeatProcessorOptions{aiMode: application.ModeDeleteOnly})
		client := useRedisRepeatBehavior(t, h)
		processRepeat(t, h, repeatMessage(1))
		processRepeat(t, h, repeatMessage(2))
		h.telegram.failOn = "delete"
		if _, err := h.processor.Process(t.Context(), repeatMessage(3)); err == nil {
			t.Fatal("應回傳 Telegram 失敗")
		}
		h.telegram.failOn = ""
		if result := processRepeat(t, h, repeatMessage(3)); result.Duplicate || !result.Spam {
			t.Fatalf("應接續既有處置：%+v", result)
		}
		assertRepeatCount(t, client, h.violations.events[0], 3)
		if !slices.Equal(h.telegram.actions, []string{"delete", "delete"}) {
			t.Fatalf("預期一次失敗、一次成功的刪除嘗試：%v", h.telegram.actions)
		}
	})
}

func TestProcessorRepeatedContentBehaviorError(t *testing.T) {
	t.Parallel()
	h := newRepeatProcessor(t, repeatProcessorOptions{aiMode: application.ModeEnforce})
	h.behaviors.err = errors.New("測試用 Redis 失敗")
	if _, err := h.processor.Process(t.Context(), repeatMessage(1)); err == nil {
		t.Fatal("行為儲存錯誤應傳回而非靜默忽略")
	}
	if h.classifier.calls != 0 || len(h.violations.events) != 0 || len(h.telegram.actions) != 0 {
		t.Fatal("儲存失敗後不應繼續 AI 或處置")
	}
	h.behaviors.err = nil
	if result := processRepeat(t, h, repeatMessage(1)); result.Duplicate {
		t.Fatal("行為儲存錯誤後應允許重試")
	}
}
