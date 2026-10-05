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
			name:       "洗錢收款招攬",
			message:    domain.Message{Text: "来几个能收款的洗钱直接1我"},
			categoryID: "money_laundering_recruitment",
		},
		{
			name:       "收米日賺招攬",
			message:    domain.Message{Text: "来帮我收米 日赚1w"},
			categoryID: "money_laundering_recruitment",
		},
		{
			name:       "拉付費註冊與消費提成招攬",
			message:    domain.Message{Text: "招人啊招聘SubRouter算力平台推广专员，9000底薪（完成3000提成要求获得底薪）+百分之十消费提成+每拉一个付费注册用户1美金奖金+每拉一个注册用户0.1美金奖励，@subrouter_ai"},
			categoryID: "performance_recruitment_promo",
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

func TestSpamRulesMoneyLaunderingVariants(t *testing.T) {
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
		name string
		text string
		spam bool
	}{
		{name: "截圖帶碼收米", text: "带码来干 收米一天1W", spam: true},
		{name: "截圖風口收米", text: "国庆风口来收米 赚1W", spam: true},
		{name: "截圖洗洗米", text: "帮我洗洗米 赚九千", spam: true},
		{name: "截圖洗米秒結", text: "有码的来帮我洗米稳定秒结日挣1W", spam: true},
		{name: "截圖有碼洗米", text: "有码来洗米 日挣1W", spam: true},
		{name: "截圖幫忙洗米", text: "能帮我洗米的来 日挣1w", spam: true},
		{name: "繁體帶碼收米", text: "帶碼來幹 收米一天1W", spam: true},
		{name: "繁體風口收米", text: "國慶風口來收米 賺1W", spam: true},
		{name: "繁體洗洗米", text: "幫我洗洗米 賺九千", spam: true},
		{name: "繁體有碼洗米", text: "有碼來洗米 日掙1W", spam: true},
		{name: "全形數字與大小寫", text: "有碼來洗米 日掙１Ｗ", spam: true},
		{name: "零寬字元", text: "有碼來洗\u200b米 日\u200b掙1W", spam: true},
		{name: "換行收益", text: "幫我洗洗米\n\t賺九千", spam: true},
		{name: "不同一天收益", text: "带码来干 收米一天8千", spam: true},
		{name: "不同收米收益", text: "国庆风口来收米 赚8000", spam: true},
		{name: "不同洗米收益", text: "帮我洗洗米 赚两千", spam: true},
		{name: "收米收益無空白", text: "国庆风口来收米赚8000", spam: true},
		{name: "洗米收益無空白", text: "帮我洗洗米赚两千", spam: true},
		{name: "煮飯洗米", text: "請幫我洗米煮飯"},
		{name: "簡體洗洗米煮飯", text: "帮我洗洗米再煮饭"},
		{name: "洗米教學", text: "洗米時不用一直搓揉，輕輕沖洗即可"},
		{name: "一般收款", text: "請掃描付款碼完成收款"},
		{name: "帶碼收款", text: "記得帶碼出門收款"},
		{name: "稻作收米", text: "今年稻作收米後先晾乾再入倉"},
		{name: "一般收益", text: "今天賺1W"},
		{name: "日掙收益", text: "這份工作日掙一千"},
		{name: "秒結討論", text: "秒結是什麼意思"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := detector.Detect(domain.Message{Text: tt.text})
			if got.Spam != tt.spam {
				t.Fatalf("Detect() = spam %v category %q score %d matches=%+v signals=%v", got.Spam, got.CategoryID, got.Score, got.Matches, got.Signals)
			}
			if tt.spam && (got.CategoryID != "money_laundering_recruitment" || got.Action != domain.ActionProgressive) {
				t.Fatalf("Detect() = category %q action %q", got.CategoryID, got.Action)
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
