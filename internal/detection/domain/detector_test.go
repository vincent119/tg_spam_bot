package domain

import (
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
