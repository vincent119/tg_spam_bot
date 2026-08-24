package rules

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/vincent119/tg_spam_bot/internal/detection/domain"
)

func TestLoadDirAllowsIndependentlyVersionedRuleFiles(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	files := map[string]string{
		"one.yaml": `version: "1.0"
categories:
  - id: one
    severity: normal
    action: progressive
    threshold: 40
    weight: 40
    enabled: true
    terms: [one]
    aliases: []
    require_any: []
`,
		"two.yaml": `version: "1.1"
categories:
  - id: two
    severity: normal
    action: progressive
    threshold: 40
    weight: 40
    enabled: true
    terms: [two]
    aliases: []
    require_any: []
`,
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatalf("WriteFile(%q) error = %v", name, err)
		}
	}

	ruleSet, err := LoadDir(dir)
	if err != nil {
		t.Fatalf("LoadDir() error = %v", err)
	}
	if len(ruleSet.Categories) != 2 {
		t.Fatalf("len(Categories) = %d, want 2", len(ruleSet.Categories))
	}
	if len(ruleSet.Version) != 64 {
		t.Fatalf("snapshot version length = %d, want 64", len(ruleSet.Version))
	}
}

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
		{
			name:       "迷姦藥物招攬",
			message:    domain.Message{Text: "💊迷药/昏迷药/失忆药 @ylousb6"},
			categoryID: "illicit_drug_promo",
		},
		{
			name:       "醚藥招攬",
			message:    domain.Message{Text: "💊醚药/昏迷药/失忆药 @ylousb6"},
			categoryID: "illicit_drug_promo",
		},
		{
			name:       "肉搞價格招攬",
			message:    domain.Message{Text: "✌️ 给肉搞起来可以 1800/g @nsinaaaa"},
			categoryID: "illicit_drug_promo",
		},
		{
			name:       "毒品暗語招攬",
			message:    domain.Message{Text: "👍上头溜冰 好肉好果 @nsinaaaa"},
			categoryID: "illicit_drug_promo",
		},
		{
			name:       "下藥調教招攬",
			message:    domain.Message{Text: "😘催情药/迷玩药/下药调教 @havsvyt1"},
			categoryID: "illicit_drug_promo",
		},
		{
			name:       "催箐藥招攬",
			message:    domain.Message{Text: "😘催箐药/迷玩药/下药调教 @havsvyt1"},
			categoryID: "illicit_drug_promo",
		},
		{
			name:       "幣圈投資招攬",
			message:    domain.Message{Text: "🚀 币圈每日复盘 · 波段机会拆解 · 抱团交流不迷路 @kxyz66"},
			categoryID: "crypto_investment_promo",
		},
		{
			name:       "性功能藥物招攬",
			message:    domain.Message{Text: "😊伟哥/增久/延时 @yaolao6"},
			categoryID: "sexual_enhancement_promo",
		},
		{
			name:       "股票電銷資料招攬",
			message:    domain.Message{Text: "股民数据直出｜手拨股转百1起✅ 分成百5，AI外呼万30＋｜@Oyuge007"},
			categoryID: "stock_lead_promo",
		},
		{
			name:       "引流推廣招攬",
			message:    domain.Message{Text: "✅TG全行业引流推广 @B886S精选优质群全覆盖，高效全天投放"},
			categoryID: "traffic_promo",
		},
		{
			name:       "電子煙煙油招攬",
			message:    domain.Message{Text: "🎁上头电子烟 原料 成品油 依托 @woshifeijingxi6"},
			categoryID: "e_cigarette_promo",
		},
		{
			name:       "抖幣快幣代刷招攬",
			message:    domain.Message{Text: "💌南海集团抖币快币代刷 @nnnh1133"},
			categoryID: "coin_brushing_promo",
		},
		{
			name:       "地推掃碼回流資料招攬",
			message:    domain.Message{Text: "全类型扫码Q 首次回流扫码 私人扫码 地推扫码 电脑pc扫码 @hysc99"},
			categoryID: "ground_promotion_data_promo",
		},
		{
			name:       "地推招人暗語",
			message:    domain.Message{Text: "厕所盖章手写招人（日入一千0本金）"},
			categoryID: "ground_promotion_data_promo",
		},
		{
			name:       "成人服務招攬",
			message:    domain.Message{Text: "外围上门包夜 @seller"},
			categoryID: "adult_service_promo",
		},
		{
			name:       "武器交易招攬",
			message:    domain.Message{Text: "出售气枪，货到付款 @seller"},
			categoryID: "weapon_trade_promo",
		},
		{
			name:       "金融證件詐騙招攬",
			message:    domain.Message{Text: "无抵押贷款，代开发票 @seller"},
			categoryID: "financial_document_fraud",
		},
		{
			name:       "博弈投注招攬",
			message:    domain.Message{Text: "足球投注，免费加盟代理 @seller"},
			categoryID: "gambling",
		},
		{
			name:       "毒品交易招攬",
			message:    domain.Message{Text: "海luo因、k粉、ketamine 可出 @seller"},
			categoryID: "illicit_drug_promo",
		},
		{
			name:       "處方藥物招攬",
			message:    domain.Message{Text: "地西泮、莫达非尼现货 @seller"},
			categoryID: "controlled_medication_promo",
		},
		{
			name:       "禁藥荷爾蒙招攬",
			message:    domain.Message{Text: "testosterone 和 erythropoietin 可供货 @seller"},
			categoryID: "performance_enhancing_drug_promo",
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

func TestSpamRulesDoNotBanDrugTermWithoutContactOrTransactionSignal(t *testing.T) {
	t.Parallel()

	ruleSet, err := LoadDir(filepath.Join("..", "..", "..", "configs", "rules"))
	if err != nil {
		t.Fatalf("LoadDir() error = %v", err)
	}
	detector, err := domain.NewDetector(ruleSet, domain.NewNormalizer(domain.OpenCCConverter{}, 4096), nil, nil)
	if err != nil {
		t.Fatalf("NewDetector() error = %v", err)
	}

	got := detector.Detect(domain.Message{Text: "新聞報導提醒民眾防範迷藥犯罪"})
	if got.Spam {
		t.Fatalf("Detect() = spam %v category %q score %d matches=%+v signals=%v", got.Spam, got.CategoryID, got.Score, got.Matches, got.Signals)
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
