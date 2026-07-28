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
