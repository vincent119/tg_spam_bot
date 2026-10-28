package rules

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/vincent119/tg_spam_bot/internal/detection/application"
	"github.com/vincent119/tg_spam_bot/internal/detection/domain"
)

func TestEnabledYAMLCategoryEnforcementMatrix(t *testing.T) {
	rules, err := LoadDir(filepath.Join("..", "..", "..", "configs", "rules"))
	if err != nil {
		t.Fatal(err)
	}
	for _, category := range rules.Categories {
		if !category.Enabled {
			continue
		}
		t.Run(category.ID, func(t *testing.T) {
			detector, err := domain.NewDetector(domain.RuleSet{Version: rules.Version, Categories: []domain.Category{category}}, domain.NewNormalizer(domain.OpenCCConverter{}, 100000), nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			terms := append(slices.Clone(category.Terms), category.Aliases...)
			message := domain.Message{Text: strings.Join(terms, " | ") + " @contact t.me/contact 出售 日結"}
			result := detector.Detect(message, category.RequireAny...)
			if !result.Spam {
				t.Fatalf("分類未達門檻：%+v", result)
			}
			want := []application.ActionKind{application.ActionDelete, application.ActionBan}
			got := application.PlanEventActions(application.Event{Result: result, YAMLSpam: true, Mode: application.ModeEnforce}, 1)
			if !slices.Equal(got, want) {
				t.Fatalf("處置=%v，預期=%v", got, want)
			}
			if normal := detector.Detect(domain.Message{Text: "請問今天的天氣如何？ @contact"}); normal.Spam {
				t.Fatalf("一般訊息誤判：%+v", normal)
			}
		})
	}
}
