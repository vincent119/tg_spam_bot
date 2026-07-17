package domain

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/caiguanhao/opencc"
	"golang.org/x/text/unicode/norm"
)

// ChineseConverter 隔離繁簡轉換套件，讓規則引擎可測試且可替換字典。
type ChineseConverter interface {
	ToTraditional(text string) string
}

// OpenCCConverter 使用台灣繁體字典產生額外比對副本。
type OpenCCConverter struct{}

// ToTraditional 轉換文字但不覆寫原始內容。
func (OpenCCConverter) ToTraditional(text string) string {
	return opencc.Convert("s2twp", text)
}

// Normalizer 建立長度受限且可穩定比對的文字版本。
type Normalizer struct {
	converter ChineseConverter
	maxRunes  int
}

// NewNormalizer 建立具有輸入字數上限的正規化器。
func NewNormalizer(converter ChineseConverter, maxRunes int) Normalizer {
	return Normalizer{converter: converter, maxRunes: maxRunes}
}

// Normalize 保留原文，並產生 Unicode 正規化與繁體轉換兩條比對軌道。
func (n Normalizer) Normalize(text string) NormalizedText {
	original := limitRunes(text, n.maxRunes)
	normalized := normalizeComparable(original)
	traditional := normalized
	if n.converter != nil {
		traditional = normalizeComparable(n.converter.ToTraditional(original))
	}
	return NormalizedText{Original: original, Normalized: normalized, TraditionalVariant: traditional}
}

func normalizeComparable(text string) string {
	text = strings.ToLower(norm.NFKC.String(text))
	var b strings.Builder
	b.Grow(len(text))
	space := false
	for _, r := range text {
		switch {
		case unicode.IsSpace(r):
			space = true
		case unicode.IsControl(r) || unicode.In(r, unicode.Cf):
			continue
		default:
			if space && b.Len() > 0 {
				b.WriteByte(' ')
			}
			space = false
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(b.String())
}

func limitRunes(text string, limit int) string {
	if limit <= 0 || utf8.RuneCountInString(text) <= limit {
		return text
	}
	runes := []rune(text)
	return string(runes[:limit])
}
