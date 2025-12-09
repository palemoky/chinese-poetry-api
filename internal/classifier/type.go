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

	// Split merged lines (e.g., "江南有美人，别后长相忆。" → ["江南有美人", "别后长相忆"])
	expandedLines := expandParagraphs(paragraphs)

	// Check if expansion resulted in empty lines
	if len(expandedLines) == 0 {
		return PoetryTypeInfo{
			TypeName: "其他",
			Category: "其他",
		}
	}

	// Count lines and characters per line
	lineCount := len(expandedLines)
	charCounts := make([]int, lineCount)

	for i, line := range expandedLines {
		// Remove punctuation and count characters
		cleaned := removePunctuation(line)
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

// expandParagraphs splits merged lines based on punctuation
// e.g., "江南有美人，别后长相忆。" → ["江南有美人", "别后长相忆"]
func expandParagraphs(paragraphs []string) []string {
	var result []string

	for _, para := range paragraphs {
		// Split by common sentence-ending punctuation
		lines := splitByPunctuation(para)
		result = append(result, lines...)
	}

	return result
}

// splitByPunctuation splits a string by Chinese punctuation marks
func splitByPunctuation(s string) []string {
	// Replace punctuation with a delimiter
	delimiters := []string{"，", "。", "！", "？", "；", "、"}

	result := s
	for _, delim := range delimiters {
		result = strings.ReplaceAll(result, delim, "|")
	}

	// Split by delimiter and filter empty strings
	parts := strings.Split(result, "|")
	var lines []string
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			lines = append(lines, trimmed)
		}
	}

	return lines
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
