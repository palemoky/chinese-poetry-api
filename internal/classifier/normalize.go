package classifier

import (
	"strings"
	"unicode"
)

// NormalizeText normalizes text by trimming whitespace and removing extra spaces
func NormalizeText(text string) string {
	// Trim leading and trailing whitespace
	text = strings.TrimSpace(text)

	// Replace multiple spaces with single space
	text = strings.Join(strings.Fields(text), " ")

	return text
}

// NormalizeTextArray normalizes an array of text strings
func NormalizeTextArray(texts []string) []string {
	result := make([]string, 0, len(texts))
	for _, text := range texts {
		normalized := NormalizeText(text)
		if normalized != "" {
			result = append(result, normalized)
		}
	}
	return result
}

// TrimAllWhitespace removes all whitespace characters from text
func TrimAllWhitespace(text string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, text)
}

// NormalizePointer normalizes a pointer to string
func NormalizePointer(text *string) *string {
	if text == nil {
		return nil
	}
	normalized := NormalizeText(*text)
	if normalized == "" {
		return nil
	}
	return &normalized
}
