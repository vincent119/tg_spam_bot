package rules

import (
	"path/filepath"
	"testing"

	"github.com/vincent119/tg_spam_bot/internal/detection/domain"
)

func TestSpamRulesDetectGamblingAccountRental(t *testing.T) {
	t.Parallel()

	ruleSet, err := LoadDir(filepath.Join("..", "..", "..", "configs", "rules"))
	if err != nil {
		t.Fatalf("LoadDir() error = %v", err)
	}
	detector, err := domain.NewDetector(ruleSet, domain.NewNormalizer(domain.OpenCCConverter{}, 4096), nil, nil)
	if err != nil {
		t.Fatalf("NewDetector() error = %v", err)
	}

	got := detector.Detect(domain.Message{Text: "皇冠体育新版私网｜登3到超管账号出租｜稳定流畅无套路"})
	if !got.Spam || got.CategoryID != "gambling_account_rental" {
		t.Fatalf("Detect() = spam %v category %q score %d matches=%+v", got.Spam, got.CategoryID, got.Score, got.Matches)
	}
}

func TestSpamRulesDetectReportedCampaigns(t *testing.T) {
	t.Parallel()

	ruleSet, err := LoadDir(filepath.Join("..", "..", "..", "configs", "rules"))
	if err != nil {
		t.Fatalf("LoadDir() error = %v", err)
	}
	detector, err := domain.NewDetector(ruleSet, domain.NewNormalizer(domain.OpenCCConverter{}, 4096), nil, nil)
	if err != nil {
		t.Fatalf("NewDetector() error = %v", err)
	}

	tests := []struct {
		name       string
		message    domain.Message
		categoryID string
	}{
		{
			name:       "抖音新玩法招募",
			message:    domain.Message{Text: "抖音全新玩法上线｜无违规账号即可"},
			categoryID: "spam_campaign_promo",
		},
		{
			name:       "抖音代刷禮物與收益宣稱",
			message:    domain.Message{Text: "🚀有抖音即可参与｜代刷礼物来人多｜收益日结5000+"},
			categoryID: "douyin_gift_fraud",
		},
		{
			name:       "引用外圍廣告並附聯絡帳號",
			message:    domain.Message{Text: "@maoge", ReferenceText: "猫哥外围 · 全国商K · 包养\n多年运营 扎根成都\n靠谱外围 精品优选"},
			categoryID: "spam_campaign_promo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := detector.Detect(tt.message)
			if !got.Spam || got.CategoryID != tt.categoryID {
				t.Fatalf("Detect() = spam %v category %q score %d matches=%+v signals=%v", got.Spam, got.CategoryID, got.Score, got.Matches, got.Signals)
			}
		})
	}
}
