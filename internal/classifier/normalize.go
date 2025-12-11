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

// hasValidContent checks if text contains actual content beyond punctuation and whitespace
func hasValidContent(text string) bool {
	for _, r := range text {
		// If we find any character that's not punctuation or whitespace, it's valid content
		if !unicode.IsPunct(r) && !unicode.IsSpace(r) {
			return true
		}
	}
	return false
}

// NormalizeTextArray normalizes an array of text strings and filters out invalid entries
// Invalid entries include: empty strings, whitespace-only, or punctuation-only content
func NormalizeTextArray(texts []string) []string {
	result := make([]string, 0, len(texts))
	for _, text := range texts {
		normalized := NormalizeText(text)
		// Filter out empty strings and punctuation-only content
		if normalized != "" && hasValidContent(normalized) {
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
