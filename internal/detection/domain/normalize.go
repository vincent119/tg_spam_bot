package domain

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/caiguanhao/opencc"
	"golang.org/x/text/unicode/norm"
)

type ChineseConverter interface {
	ToTraditional(text string) string
}

type OpenCCConverter struct{}

func (OpenCCConverter) ToTraditional(text string) string {
	return opencc.Convert("s2twp", text)
}

type Normalizer struct {
	converter ChineseConverter
	maxRunes  int
}

func NewNormalizer(converter ChineseConverter, maxRunes int) Normalizer {
	return Normalizer{converter: converter, maxRunes: maxRunes}
}

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

func limitRunes(text string, max int) string {
	if max <= 0 || utf8.RuneCountInString(text) <= max {
		return text
	}
	runes := []rune(text)
	return string(runes[:max])
}
