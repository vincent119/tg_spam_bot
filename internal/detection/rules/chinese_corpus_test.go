package rules

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/vincent119/tg_spam_bot/internal/detection/domain"
)

type chineseCorpusCase struct {
	Name          string        `json:"name"`
	Text          string        `json:"text"`
	ReferenceText string        `json:"reference_text"`
	Spam          bool          `json:"spam"`
	Category      string        `json:"category"`
	Action        domain.Action `json:"action"`
	ForbidSignal  string        `json:"forbid_signal"`
}

func TestChineseModerationRegressionCorpus(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile(filepath.Join("testdata", "chinese_moderation_corpus.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cases []chineseCorpusCase
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) != 12 {
		t.Fatalf("中文案例數量=%d，預期八則廣告加四則正常文案", len(cases))
	}
	ruleSet, err := LoadDir(filepath.Join("..", "..", "..", "configs", "rules"))
	if err != nil {
		t.Fatal(err)
	}
	detector, err := domain.NewDetector(ruleSet, domain.NewNormalizer(domain.OpenCCConverter{}, 4096), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range cases {
		t.Run(tt.Name, func(t *testing.T) {
			t.Parallel()
			got := detector.Detect(domain.Message{Text: tt.Text, ReferenceText: tt.ReferenceText})
			if got.Spam != tt.Spam {
				t.Fatalf("Spam=%v，預期=%v；分類=%q 分數=%d 命中=%+v", got.Spam, tt.Spam, got.CategoryID, got.Score, got.Matches)
			}
			if tt.Spam && (got.CategoryID != tt.Category || got.Action != tt.Action) {
				t.Fatalf("分類=%q 動作=%q，預期=%q／%q", got.CategoryID, got.Action, tt.Category, tt.Action)
			}
			if tt.ForbidSignal != "" && slices.Contains(got.Signals, tt.ForbidSignal) {
				t.Fatalf("引用內容不能產生發送者訊號 %q：%v", tt.ForbidSignal, got.Signals)
			}
		})
	}
}
