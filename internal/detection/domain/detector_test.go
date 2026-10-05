package domain

import (
	"slices"
	"testing"
)

type fakeConverter struct{ value string }

func (f fakeConverter) ToTraditional(string) string { return f.value }

func TestDetector(t *testing.T) {
	t.Parallel()

	rules := RuleSet{Version: "v1", Categories: []Category{
		{ID: "counterfeit", Name: "偽鈔", Severity: SeverityCritical, Action: ActionBan, Threshold: 80, Weight: 70, Enabled: true, Terms: []string{"假鈔", "高防鈔"}, RequireAny: []string{"telegram_mention", "transaction_signal"}},
		{ID: "job", Name: "工作詐騙", Severity: SeverityNormal, Action: ActionProgressive, Threshold: 40, Weight: 40, Enabled: true, Terms: []string{"earn money fast"}},
	}}
	detector, err := NewDetector(rules, NewNormalizer(OpenCCConverter{}, 4096), nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		text   string
		spam   bool
		action Action
	}{
		{name: "simplified counterfeit with contact", text: "高防钞稳定出货 @seller", spam: true, action: ActionBan},
		{name: "critical term without required signal", text: "新聞討論假鈔辨識", spam: false},
		{name: "english", text: "EARN MONEY FAST", spam: true, action: ActionProgressive},
		{name: "normal", text: "今天午餐吃什麼", spam: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detector.Detect(Message{Text: tt.text})
			if got.Spam != tt.spam || got.Action != tt.action {
				t.Fatalf("Detect() = spam %v action %q score %d, want spam %v action %q", got.Spam, got.Action, got.Score, tt.spam, tt.action)
			}
		})
	}
}

func TestDetectorSeparatesReferenceTermsFromSenderSignals(t *testing.T) {
	t.Parallel()

	rules := RuleSet{Version: "v1", Categories: []Category{{
		ID: "douyin", Severity: SeverityCritical, Action: ActionBan, Threshold: 80, Weight: 70,
		Enabled: true, Terms: []string{"抖音代刷禮物"}, RequireAny: []string{"telegram_mention"},
	}}}
	detector, err := NewDetector(rules, NewNormalizer(OpenCCConverter{}, 4096), nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		message Message
		spam    bool
	}{
		{name: "引用廣告並提供外層聯絡人", message: Message{Text: "聯絡 @seller", ReferenceText: "抖音代刷礼物"}, spam: true},
		{name: "單純回覆含聯絡人的垃圾訊息", message: Message{Text: "這是垃圾訊息", ReferenceText: "抖音代刷礼物 @seller"}, spam: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detector.Detect(tt.message)
			if got.Spam != tt.spam {
				t.Fatalf("Detect().Spam = %v, want %v; result=%#v", got.Spam, tt.spam, got)
			}
		})
	}
}

func TestNormalizerPreservesOriginal(t *testing.T) {
	t.Parallel()
	n := NewNormalizer(fakeConverter{value: "高薪兼職"}, 100)
	got := n.Normalize("高薪兼职")
	if got.Original != "高薪兼职" || got.TraditionalVariant != "高薪兼職" {
		t.Fatalf("unexpected normalized text: %#v", got)
	}
}

func FuzzNormalizer(f *testing.F) {
	f.Add("免费领取 EARN MONEY")
	n := NewNormalizer(nil, 4096)
	f.Fuzz(func(t *testing.T, input string) {
		got := n.Normalize(input)
		if len([]rune(got.Original)) > 4096 {
			t.Fatal("original exceeds limit")
		}
	})
}

func BenchmarkDetector(b *testing.B) {
	rules := RuleSet{Version: "v1", Categories: []Category{{ID: "job", Severity: SeverityNormal, Action: ActionProgressive, Threshold: 40, Weight: 40, Enabled: true, Terms: []string{"高薪兼職", "earn money fast"}}}}
	detector, _ := NewDetector(rules, NewNormalizer(nil, 4096), nil, nil)
	b.ResetTimer()
	for range b.N {
		detector.Detect(Message{Text: "高薪兼職，earn money fast @example"})
	}
}

func TestDetectorRepeatedContentObservationOnly(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		action    Action
		threshold int
		text      string
		spam      bool
		score     int
		disabled  bool
	}{
		{name: "重複不能補足一般門檻", action: ActionProgressive, threshold: 60, text: "測試詞", score: 40},
		{name: "重複不能滿足封鎖條件", action: ActionBan, threshold: 40, text: "測試詞", score: 40},
		{name: "其他訊號仍可加分", action: ActionProgressive, threshold: 60, text: "測試詞 @contact", score: 60, spam: true},
		{name: "其他訊號仍可滿足封鎖條件", action: ActionBan, threshold: 60, text: "測試詞 @contact", score: 60, spam: true},
		{name: "明確詞彙垃圾不降級", action: ActionProgressive, threshold: 40, text: "測試詞", score: 40, spam: true},
		{name: "僅重複正常文案不加分", action: ActionProgressive, threshold: 60, text: "大家早安"},
		{name: "重複不能啟用停用分類", action: ActionProgressive, threshold: 40, text: "測試詞", disabled: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rules := RuleSet{Version: "repeat-test", Categories: []Category{{
				ID: "test", Enabled: !tt.disabled, Severity: SeverityCritical, Action: tt.action,
				Threshold: tt.threshold, Weight: 40, Terms: []string{"測試詞"},
				RequireAny: []string{SignalRepeatedContent, "telegram_mention"},
			}}}
			detector, err := NewDetector(rules, NewNormalizer(nil, 4096), nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			baseline := detector.Detect(Message{Text: tt.text})
			signals := []string{SignalRepeatedContent, SignalRepeatedContent}
			result := detector.Detect(Message{Text: tt.text}, signals...)
			if result.Spam != tt.spam || result.Score != tt.score || result.Action != baseline.Action || result.Threshold != baseline.Threshold || result.Score != baseline.Score {
				t.Fatalf("重複訊號不應改變規則判定：%+v，基準=%+v", result, baseline)
			}
			if !slices.Contains(result.Signals, SignalRepeatedContent) || signals[0] != SignalRepeatedContent || signals[1] != SignalRepeatedContent {
				t.Fatalf("應保留稽核線索且不改輸入 slice：%+v，輸入=%v", result, signals)
			}
		})
	}
}
