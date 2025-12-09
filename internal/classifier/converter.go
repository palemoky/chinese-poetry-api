package classifier

import (
	"fmt"

	"github.com/liuzl/gocc"
)

// s2t and t2s are initialized once in init() and are safe for concurrent use.
// The underlying gocc.OpenCC.Convert method is thread-safe.
var (
	s2t *gocc.OpenCC // Simplified to Traditional (thread-safe)
	t2s *gocc.OpenCC // Traditional to Simplified (thread-safe)
)

func init() {
	var err error

	// Initialize simplified to traditional converter
	s2t, err = gocc.New("s2t")
	if err != nil {
		panic(fmt.Sprintf("failed to initialize s2t converter: %v", err))
	}

	// Initialize traditional to simplified converter
	t2s, err = gocc.New("t2s")
	if err != nil {
		panic(fmt.Sprintf("failed to initialize t2s converter: %v", err))
	}
}

// ToTraditional converts simplified Chinese to traditional Chinese
func ToTraditional(text string) (string, error) {
	return s2t.Convert(text)
}

// ToSimplified converts traditional Chinese to simplified Chinese
func ToSimplified(text string) (string, error) {
	return t2s.Convert(text)
}

// ToTraditionalArray converts an array of strings to traditional Chinese
func ToTraditionalArray(texts []string) ([]string, error) {
	result := make([]string, len(texts))
	for i, text := range texts {
		converted, err := ToTraditional(text)
		if err != nil {
			return nil, fmt.Errorf("failed to convert text at index %d: %w", i, err)
		}
		result[i] = converted
	}
	return result, nil
}

// ToSimplifiedArray converts an array of strings to simplified Chinese
func ToSimplifiedArray(texts []string) ([]string, error) {
	result := make([]string, len(texts))
	for i, text := range texts {
		converted, err := ToSimplified(text)
		if err != nil {
			return nil, fmt.Errorf("failed to convert text at index %d: %w", i, err)
		}
		result[i] = converted
	}
	return result, nil
}

// ToTraditionalPointer converts a pointer to string to traditional Chinese
func ToTraditionalPointer(text *string) (*string, error) {
	if text == nil || *text == "" {
		return text, nil
	}
	converted, err := ToTraditional(*text)
	if err != nil {
		return nil, err
	}
	return &converted, nil
}

// ConvertPoemToTraditional converts all fields of a poem to traditional Chinese
func ConvertPoemToTraditional(title, author, content, rhythmic string) (string, string, string, string, error) {
	t, err := ToTraditional(title)
	if err != nil {
		return "", "", "", "", fmt.Errorf("failed to convert title: %w", err)
	}

	a, err := ToTraditional(author)
	if err != nil {
		return "", "", "", "", fmt.Errorf("failed to convert author: %w", err)
	}

	c, err := ToTraditional(content)
	if err != nil {
		return "", "", "", "", fmt.Errorf("failed to convert content: %w", err)
	}

	r := rhythmic
	if rhythmic != "" {
		r, err = ToTraditional(rhythmic)
		if err != nil {
			return "", "", "", "", fmt.Errorf("failed to convert rhythmic: %w", err)
		}
	}

	return t, a, c, r, nil
}
