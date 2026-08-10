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
			name:       "抖音日結專案招人",
			message:    domain.Message{Text: "抖音日结项目招人｜日结5K起，量大不限，不拖欠 @qianshen8899"},
			categoryID: "douyin_gift_fraud",
		},
		{
			name:       "抖音日結專案招募團隊",
			message:    domain.Message{Text: "抖音日结项目｜招人手、招团队，长期稳定，结算及时 @qianshen8899"},
			categoryID: "douyin_gift_fraud",
		},
		{
			name:       "抖音全新專案單號參與",
			message:    domain.Message{Text: "2026抖音全新项目上线｜单号即可参与 @kaixuanyule666"},
			categoryID: "douyin_gift_fraud",
		},
		{
			name:       "抖音新專案低門檻招攬",
			message:    domain.Message{Text: "2026抖音新项目｜低门槛高机会｜不要观望 @kaixuanyule666"},
			categoryID: "douyin_gift_fraud",
		},
		{
			name:       "保養讀書妹成人招攬",
			message:    domain.Message{Text: "可保养读书妹，找零装逼专用 @jkshibd88"},
			categoryID: "spam_campaign_promo",
		},
		{
			name:       "博弈平台百分比分紅招商",
			message:    domain.Message{Text: "📢【九台招商】直营24小时在线55%分红\n开云 乐鱼 爱游戏\n星空 米兰 乐彩\n免费加盟代理分红，佣金稳出款快"},
			categoryID: "gambling",
		},
		{
			name:       "引用外圍廣告並附聯絡帳號",
			message:    domain.Message{Text: "@maoge", ReferenceText: "猫哥外围 · 全国商K · 包养\n多年运营 扎根成都\n靠谱外围 精品优选"},
			categoryID: "spam_campaign_promo",
		},
		{
			name:       "引用帳號交易廣告並附聯絡帳號",
			message:    domain.Message{Text: "b @cx68688", ReferenceText: "出 微信 美短 A16 数据号 飞书\n企业微信 绿标主体 带超管\n各种备份包 国内私人号\n抖音 地推实名 包评论直播\n快手 实名号 国内白号"},
			categoryID: "social_account_trading",
		},
		{
			name:       "商務娛樂成人廣告",
			message:    domain.Message{Text: "杭州商K真空场｜妹子05 06会玩·节目齐全·沙发秀"},
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

func TestSpamRulesDoNotTreatOrdinaryDividendAsProfitClaim(t *testing.T) {
	t.Parallel()

	ruleSet, err := LoadDir(filepath.Join("..", "..", "..", "configs", "rules"))
	if err != nil {
		t.Fatalf("LoadDir() error = %v", err)
	}
	detector, err := domain.NewDetector(ruleSet, domain.NewNormalizer(domain.OpenCCConverter{}, 4096), nil, nil)
	if err != nil {
		t.Fatalf("NewDetector() error = %v", err)
	}

	got := detector.Detect(domain.Message{Text: "公司董事會通過今年分红方案"})
	if got.Spam {
		t.Fatalf("Detect() = spam %v category %q score %d matches=%+v signals=%v", got.Spam, got.CategoryID, got.Score, got.Matches, got.Signals)
	}
}
