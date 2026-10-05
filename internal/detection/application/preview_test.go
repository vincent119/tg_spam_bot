package application

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/vincent119/tg_spam_bot/internal/detection/domain"
)

type previewDetectorStub struct {
	result  domain.Result
	signals []string
}

func (d *previewDetectorStub) Detect(_ domain.Message, signals ...string) domain.Result {
	d.signals = append([]string(nil), signals...)
	result := d.result
	result.Signals = append(result.SignalsCopy(), signals...)
	return result
}

type previewBehaviorSpy struct {
	snapshot     RepeatSnapshot
	err          error
	fingerprint  string
	observeCalls int
}

func (s *previewBehaviorSpy) PeekRepeat(_ context.Context, _ domain.Message, fingerprint string) (RepeatSnapshot, error) {
	s.fingerprint = fingerprint
	return s.snapshot, s.err
}

func (s *previewBehaviorSpy) Observe(context.Context, domain.Message, string) ([]string, error) {
	s.observeCalls++
	return nil, errors.New("preview must not observe")
}

type previewExemptionStub struct {
	exempt bool
	reason string
	err    error
}

func (s previewExemptionStub) IsExempt(context.Context, int64, int64) (bool, string, error) {
	return s.exempt, s.reason, s.err
}

type previewHistoryStub struct {
	record HistoricalDetection
	found  bool
	err    error
}

func (s previewHistoryStub) FindHistoricalDetection(context.Context, int64, int64) (HistoricalDetection, bool, error) {
	return s.record, s.found, s.err
}

func TestDetectionPreviewReadOnlyAndReasons(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		snapshot      RepeatSnapshot
		repeatErr     error
		exemption     previewExemptionStub
		mode          Mode
		repeatAction  RepeatAction
		aiEnabled     bool
		rule          domain.Result
		wantCount     int
		wantKnown     bool
		wantReason    string
		wantActions   []ActionKind
		wantRepeatSig bool
	}{
		{
			name: "未觀測第三則預估清三則", snapshot: RepeatSnapshot{Available: true, Count: 2, MessageIDs: []int64{10, 11}, Window: 30 * time.Minute},
			mode: ModeEnforce, repeatAction: RepeatActionDelete, wantCount: 3, wantKnown: true,
			wantReason: "hypothetical_unobserved", wantActions: []ActionKind{ActionDelete}, wantRepeatSig: true,
		},
		{
			name: "已觀測第三則不重計", snapshot: RepeatSnapshot{Available: true, Count: 3, MessageIDs: []int64{10, 11, 12}, CurrentObserved: true, Window: 30 * time.Minute},
			mode: ModeEnforce, repeatAction: RepeatActionBan, wantCount: 3, wantKnown: true,
			wantReason: "current_policy", wantActions: []ActionKind{ActionDelete, ActionBan}, wantRepeatSig: true,
		},
		{
			name: "刪除模式不升級封鎖", snapshot: RepeatSnapshot{Available: true, Count: 3, MessageIDs: []int64{10, 11, 12}, CurrentObserved: true},
			mode: ModeDeleteOnly, repeatAction: RepeatActionBan, wantCount: 3, wantKnown: true,
			wantReason: "current_policy", wantActions: []ActionKind{ActionDelete}, wantRepeatSig: true,
		},
		{
			name: "觀察模式無處置", snapshot: RepeatSnapshot{Available: true, Count: 3, CurrentObserved: true},
			mode: ModeObserve, repeatAction: RepeatActionBan, wantCount: 3, wantKnown: true,
			wantReason: "observe_mode", wantRepeatSig: true,
		},
		{
			name: "重複狀態失敗不能當零次", repeatErr: errors.New("unavailable"),
			mode: ModeEnforce, repeatAction: RepeatActionBan, wantKnown: false, wantReason: "repeat_count_unknown",
		},
		{
			name: "豁免對象無處置", snapshot: RepeatSnapshot{Available: true, Count: 3, CurrentObserved: true}, exemption: previewExemptionStub{exempt: true, reason: "管理員"},
			mode: ModeEnforce, repeatAction: RepeatActionBan, wantCount: 3, wantKnown: true, wantReason: "exempt",
		},
		{
			name: "豁免查詢失敗無處置結論", snapshot: RepeatSnapshot{Available: true, Count: 3, CurrentObserved: true}, exemption: previewExemptionStub{err: errors.New("unavailable")},
			mode: ModeEnforce, repeatAction: RepeatActionBan, wantCount: 3, wantKnown: false, wantReason: "exemption_unknown", wantRepeatSig: true,
		},
		{
			name: "一般違規階梯未知", snapshot: RepeatSnapshot{Available: true, Count: 1, CurrentObserved: true},
			rule: domain.Result{Spam: true, Action: domain.ActionProgressive, Severity: domain.SeverityHigh, Score: 80, Threshold: 60},
			mode: ModeEnforce, repeatAction: RepeatActionObserve, wantCount: 1, wantKnown: false, wantReason: "violation_count_unknown",
		},
		{
			name: "AI 未執行時不推定放行", snapshot: RepeatSnapshot{Available: true, Count: 1, CurrentObserved: true},
			rule: domain.Result{Score: 40, Threshold: 60}, aiEnabled: true,
			mode: ModeEnforce, repeatAction: RepeatActionObserve, wantCount: 1, wantKnown: false, wantReason: "ai_not_run",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			detector := &previewDetectorStub{result: tt.rule}
			behavior := &previewBehaviorSpy{snapshot: tt.snapshot, err: tt.repeatErr}
			service, err := NewPreviewService(detector, behavior, tt.exemption, PreviewConfig{
				Mode: tt.mode, RepeatAction: tt.repeatAction, RepeatThreshold: 3, HashKey: []byte("test-key"), AIEnabled: tt.aiEnabled,
			})
			if err != nil {
				t.Fatal(err)
			}
			message := domain.Message{ChatID: 1, UserID: 2, MessageID: 12, Text: "收到"}
			first, err := service.Preview(t.Context(), message)
			if err != nil {
				t.Fatal(err)
			}
			second, err := service.Preview(t.Context(), message)
			if err != nil {
				t.Fatal(err)
			}
			if first.Repeat.ProjectedCount != tt.wantCount || first.ActionKnown != tt.wantKnown || first.ActionReason != tt.wantReason || !reflect.DeepEqual(first.PlannedActions, tt.wantActions) {
				t.Fatalf("預覽結果 = %+v，預期次數=%d 已知=%v 原因=%q 處置=%v", first, tt.wantCount, tt.wantKnown, tt.wantReason, tt.wantActions)
			}
			if !reflect.DeepEqual(first, second) || behavior.observeCalls != 0 {
				t.Fatalf("重複預覽改變狀態：first=%+v second=%+v observe=%d", first, second, behavior.observeCalls)
			}
			if (len(detector.signals) > 0) != tt.wantRepeatSig {
				t.Fatalf("重複訊號 = %v，預期=%v", detector.signals, tt.wantRepeatSig)
			}
			if behavior.fingerprint != (&Processor{hashKey: []byte("test-key")}).fingerprint(message.Text) {
				t.Fatalf("指紋與原文 HMAC 契約不符：%q", behavior.fingerprint)
			}
			if tt.aiEnabled && (first.AIStatus != "not_run" || first.AIReason != "preview_does_not_call_ai") {
				t.Fatalf("AI 預覽狀態錯誤：%+v", first)
			}
		})
	}
}

func TestDetectionPreviewHistoricalStatus(t *testing.T) {
	t.Parallel()
	base := PreviewConfig{Mode: ModeObserve, RepeatAction: RepeatActionObserve, RepeatThreshold: 3, HashKey: []byte("key")}
	message := domain.Message{ChatID: 1, UserID: 2, MessageID: 3, Text: "一般訊息"}
	tests := []struct {
		name    string
		history HistoricalDetectionReader
		want    string
	}{
		{name: "未注入唯讀查詢", want: "unavailable"},
		{name: "目標沒有記錄", history: previewHistoryStub{}, want: "not_observed"},
		{name: "目標已記錄", history: previewHistoryStub{record: HistoricalDetection{RuleVersion: "old", Spam: true}, found: true}, want: "observed"},
		{name: "查詢失敗", history: previewHistoryStub{err: errors.New("unavailable")}, want: "unavailable"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var opts []PreviewOption
			if tt.history != nil {
				opts = append(opts, WithPreviewHistory(tt.history))
			}
			service, err := NewPreviewService(&previewDetectorStub{}, &previewBehaviorSpy{snapshot: RepeatSnapshot{Available: true, Count: 0}}, previewExemptionStub{}, base, opts...)
			if err != nil {
				t.Fatal(err)
			}
			got, err := service.Preview(t.Context(), message)
			if err != nil {
				t.Fatal(err)
			}
			if got.Historical.Status != tt.want {
				t.Fatalf("歷史狀態 = %q，預期 %q", got.Historical.Status, tt.want)
			}
			if tt.want == "observed" && got.Historical.Record.RuleVersion != "old" {
				t.Fatalf("歷史資料遺失：%+v", got.Historical)
			}
		})
	}
}

func TestNewPreviewServiceValidation(t *testing.T) {
	t.Parallel()
	base := PreviewConfig{Mode: ModeObserve, RepeatAction: RepeatActionObserve, RepeatThreshold: 3, HashKey: []byte("key")}
	detector := &previewDetectorStub{}
	repeats := &previewBehaviorSpy{}
	exemptions := previewExemptionStub{}
	tests := []struct {
		name       string
		detector   Detector
		repeats    RepeatSnapshotReader
		exemptions ExemptionStore
		config     PreviewConfig
	}{
		{name: "缺判定器", repeats: repeats, exemptions: exemptions, config: base},
		{name: "缺重複讀取", detector: detector, exemptions: exemptions, config: base},
		{name: "缺豁免讀取", detector: detector, repeats: repeats, config: base},
		{name: "非法模式", detector: detector, repeats: repeats, exemptions: exemptions, config: PreviewConfig{Mode: "invalid", RepeatAction: RepeatActionObserve, RepeatThreshold: 3, HashKey: []byte("key")}},
		{name: "非法重複政策", detector: detector, repeats: repeats, exemptions: exemptions, config: PreviewConfig{Mode: ModeObserve, RepeatAction: "invalid", RepeatThreshold: 3, HashKey: []byte("key")}},
		{name: "缺金鑰", detector: detector, repeats: repeats, exemptions: exemptions, config: PreviewConfig{Mode: ModeObserve, RepeatAction: RepeatActionObserve, RepeatThreshold: 3}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if _, err := NewPreviewService(tt.detector, tt.repeats, tt.exemptions, tt.config); err == nil {
				t.Fatal("NewPreviewService() 應拒絕不完整設定")
			}
		})
	}
}
