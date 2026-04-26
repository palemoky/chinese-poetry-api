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

// placeholderPhrases are sentinel strings used in source data to indicate that
// a poem has no actual content and should be skipped during import.
var placeholderPhrases = []string{
	"无正文。",
	"無正文。",
	"空。",
}

// IsPlaceholderContent reports whether all content in paragraphs is a
// placeholder indicating the poem has no real text (e.g. "无正文。", "空。").
func IsPlaceholderContent(paragraphs []string) bool {
	if len(paragraphs) == 0 {
		return false
	}
	// Join all paragraphs and compare against each placeholder
	joined := strings.Join(paragraphs, "")
	for _, p := range placeholderPhrases {
		if joined == p {
			return true
		}
	}
	return false
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

// isClosingQuote reports whether r is a Chinese closing quotation mark.
func isClosingQuote(r rune) bool {
	return r == '\u201D' || // "
		r == '\u300B' || // 》
		r == '\u3011' || // 】
		r == '\u300D' || // 」
		r == '\u300F' // 』
}

// SplitSentences splits a Chinese text string into individual sentences by
// breaking on sentence-ending punctuation (。！？), optionally followed by a
// closing quotation mark. If the text contains no sentence-ending punctuation
// the original text is returned as a single-element slice.
func SplitSentences(text string) []string {
	runes := []rune(text)
	n := len(runes)
	if n == 0 {
		return nil
	}

	var sentences []string
	start := 0
	for i := 0; i < n; i++ {
		r := runes[i]
		if r == '。' || r == '！' || r == '？' {
			end := i + 1
			// Include optional trailing closing quote
			if end < n && isClosingQuote(runes[end]) {
				end++
				i++ // skip closing quote in the next iteration
			}
			s := strings.TrimSpace(string(runes[start:end]))
			if s != "" && hasValidContent(s) {
				sentences = append(sentences, s)
			}
			start = end
		}
	}

	// Remaining content that does not end with terminal punctuation
	if start < n {
		s := strings.TrimSpace(string(runes[start:]))
		if s != "" && hasValidContent(s) {
			sentences = append(sentences, s)
		}
	}

	if len(sentences) == 0 {
		return []string{text}
	}
	return sentences
}

// NormalizeAndSplitParagraphs normalizes each paragraph and splits merged
// sentences into individual elements. Use this instead of NormalizeTextArray
// when the source data may contain multiple sentences concatenated into a
// single string (e.g. "A。B。" instead of ["A。","B。"]).
func NormalizeAndSplitParagraphs(paragraphs []string) []string {
	var result []string
	for _, p := range paragraphs {
		normalized := NormalizeText(p)
		if normalized == "" || !hasValidContent(normalized) {
			continue
		}
		result = append(result, SplitSentences(normalized)...)
	}
	return result
}
