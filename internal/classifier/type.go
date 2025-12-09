package classifier

import (
	"strings"
	"unicode/utf8"
)

// PoetryTypeInfo contains information about a classified poetry type
type PoetryTypeInfo struct {
	TypeName     string
	Category     string
	Lines        *int
	CharsPerLine *int
}

// ClassifyPoetryType determines the type of poetry based on its structure
func ClassifyPoetryType(paragraphs []string, rhythmic string) PoetryTypeInfo {
	// If it has a rhythmic field, it's ci (词)
	if rhythmic != "" {
		return PoetryTypeInfo{
			TypeName: "词",
			Category: "词",
		}
	}

	if len(paragraphs) == 0 {
		return PoetryTypeInfo{
			TypeName: "其他",
			Category: "其他",
		}
	}

	// Count lines and characters per line
	lineCount := len(paragraphs)
	charCounts := make([]int, lineCount)

	for i, para := range paragraphs {
		// Remove punctuation and count characters
		cleaned := removePunctuation(para)
		charCounts[i] = utf8.RuneCountInString(cleaned)
	}

	// Check if all lines have the same character count
	if !isUniform(charCounts) {
		// Irregular structure
		return PoetryTypeInfo{
			TypeName: "其他",
			Category: "其他",
		}
	}

	charsPerLine := charCounts[0]

	// Classify based on line count and characters per line
	switch {
	case lineCount == 4 && charsPerLine == 5:
		lines := 4
		chars := 5
		return PoetryTypeInfo{
			TypeName:     "五言绝句",
			Category:     "诗",
			Lines:        &lines,
			CharsPerLine: &chars,
		}
	case lineCount == 4 && charsPerLine == 7:
		lines := 4
		chars := 7
		return PoetryTypeInfo{
			TypeName:     "七言绝句",
			Category:     "诗",
			Lines:        &lines,
			CharsPerLine: &chars,
		}
	case lineCount == 8 && charsPerLine == 5:
		lines := 8
		chars := 5
		return PoetryTypeInfo{
			TypeName:     "五言律诗",
			Category:     "诗",
			Lines:        &lines,
			CharsPerLine: &chars,
		}
	case lineCount == 8 && charsPerLine == 7:
		lines := 8
		chars := 7
		return PoetryTypeInfo{
			TypeName:     "七言律诗",
			Category:     "诗",
			Lines:        &lines,
			CharsPerLine: &chars,
		}
	case charsPerLine == 5:
		chars := 5
		return PoetryTypeInfo{
			TypeName:     "五言古诗",
			Category:     "诗",
			CharsPerLine: &chars,
		}
	case charsPerLine == 7:
		chars := 7
		return PoetryTypeInfo{
			TypeName:     "七言古诗",
			Category:     "诗",
			CharsPerLine: &chars,
		}
	default:
		return PoetryTypeInfo{
			TypeName: "其他",
			Category: "其他",
		}
	}
}

// removePunctuation removes common Chinese punctuation marks
func removePunctuation(s string) string {
	punctuation := []string{
		"，", "。", "！", "？", "；", "：", "、",
		",", ".", "!", "?", ";", ":", " ",
		"「", "」", "『", "』", "（", "）", "《", "》",
		`"`, `"`, `'`, `'`, "【", "】", "〔", "〕",
	}

	result := s
	for _, p := range punctuation {
		result = strings.ReplaceAll(result, p, "")
	}

	return result
}

// isUniform checks if all elements in the slice are the same
func isUniform(counts []int) bool {
	if len(counts) == 0 {
		return true
	}

	first := counts[0]
	for _, count := range counts[1:] {
		if count != first {
			return false
		}
	}

	return true
}
