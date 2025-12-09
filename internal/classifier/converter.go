package classifier

import (
	"fmt"

	"github.com/liuzl/gocc"
)

var (
	s2t *gocc.OpenCC // Simplified to Traditional
	t2s *gocc.OpenCC // Traditional to Simplified
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
