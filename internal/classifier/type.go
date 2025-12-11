package classifier

import (
	"strings"
	"unicode/utf8"
)

// Poetry type constants
const (
	// Categories
	CategoryPoetry = "唐诗"
	CategoryCi     = "宋词"
	CategoryOther  = "其他"

	// Specific types
	TypeWuyanJueju = "五言绝句"
	TypeQiyanJueju = "七言绝句"
	TypeWuyanLvshi = "五言律诗"
	TypeQiyanLvshi = "七言律诗"
	TypeCi         = "宋词"
	TypeOther      = "其他"

	// Structure constraints
	JuejuLines = 4
	LvshiLines = 8
	WuyanChars = 5
	QiyanChars = 7
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
			TypeName: TypeCi,
			Category: CategoryCi,
		}
	}

	if len(paragraphs) == 0 {
		return PoetryTypeInfo{
			TypeName: TypeOther,
			Category: CategoryOther,
		}
	}

	// Split merged lines (e.g., "江南有美人，别后长相忆。" → ["江南有美人", "别后长相忆"])
	expandedLines := expandParagraphs(paragraphs)

	// Check if expansion resulted in empty lines
	if len(expandedLines) == 0 {
		return PoetryTypeInfo{
			TypeName: TypeOther,
			Category: CategoryOther,
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
			TypeName: TypeOther,
			Category: CategoryOther,
		}
	}

	charsPerLine := charCounts[0]

	// Classify based on line count and characters per line
	typeName, category := classifyByStructure(lineCount, charsPerLine)

	return PoetryTypeInfo{
		TypeName:     typeName,
		Category:     category,
		Lines:        &lineCount,
		CharsPerLine: &charsPerLine,
	}
}

// classifyByStructure classifies poetry based on line count and characters per line
func classifyByStructure(lines, chars int) (typeName, category string) {
	switch {
	case lines == JuejuLines && chars == WuyanChars:
		return TypeWuyanJueju, CategoryPoetry
	case lines == JuejuLines && chars == QiyanChars:
		return TypeQiyanJueju, CategoryPoetry
	case lines == LvshiLines && chars == WuyanChars:
		return TypeWuyanLvshi, CategoryPoetry
	case lines == LvshiLines && chars == QiyanChars:
		return TypeQiyanLvshi, CategoryPoetry
	default:
		return TypeOther, CategoryOther
	}
}

// isUniform checks if all integers in a slice are equal
func isUniform(nums []int) bool {
	if len(nums) == 0 {
		return true
	}
	first := nums[0]
	for _, n := range nums[1:] {
		if n != first {
			return false
		}
	}
	return true
}

// expandParagraphs splits paragraphs by sentence-ending punctuation
func expandParagraphs(paragraphs []string) []string {
	var result []string

	for _, para := range paragraphs {
		// Split by common sentence-ending punctuation
		// 。！？；are common Chinese sentence enders
		lines := splitBySentence(para)
		result = append(result, lines...)
	}

	return result
}

// splitBySentence splits text by sentence-ending punctuation
func splitBySentence(text string) []string {
	// Replace sentence-ending punctuation with a delimiter
	delimiters := []string{"。", "！", "？", "；", "，"}
	for _, delim := range delimiters {
		text = strings.ReplaceAll(text, delim, "\n")
	}

	// Split by newline and filter empty strings
	lines := strings.Split(text, "\n")
	var result []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			result = append(result, line)
		}
	}

	return result
}

// removePunctuation removes all punctuation from text
func removePunctuation(text string) string {
	// Common Chinese and English punctuation
	punctuation := `，。！？；：""''（）《》【】、·—…,.!?;:'"()[]{}/-`

	// Use strings.Map for efficient single-pass filtering
	result := strings.Map(func(r rune) rune {
		if strings.ContainsRune(punctuation, r) {
			return -1 // Remove this character
		}
		return r
	}, text)

	return strings.TrimSpace(result)
}
