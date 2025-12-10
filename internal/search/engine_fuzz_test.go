package search

import (
	"testing"
)

// FuzzIsPinyinQuery tests the isPinyinQuery function with random inputs
func FuzzIsPinyinQuery(f *testing.F) {
	// Seed corpus with known cases
	f.Add("jing ye si")
	f.Add("静夜思")
	f.Add("libai")
	f.Add("李白")
	f.Add("")
	f.Add("123")
	f.Add("abc123中文")
	f.Add("   ")
	f.Add("a")
	f.Add("中")
	f.Add("UPPERCASE")
	f.Add("MixedCase123")

	f.Fuzz(func(t *testing.T, input string) {
		// Should not panic
		result := isPinyinQuery(input)

		// Result should be a boolean (always true)
		_ = result

		// For empty strings, should return false
		if input == "" && result {
			t.Error("isPinyinQuery(\"\") should return false")
		}

		// For strings with only spaces, should return false
		allSpaces := true
		for _, r := range input {
			if r != ' ' && r != '\t' && r != '\n' {
				allSpaces = false
				break
			}
		}
		if allSpaces && len(input) > 0 && result {
			t.Errorf("isPinyinQuery(%q) with only spaces should return false", input)
		}

		// For strings with >50% ASCII letters, should return true
		letterCount := 0
		totalCount := 0
		for _, r := range input {
			if r == ' ' || r == '\t' || r == '\n' {
				continue
			}
			totalCount++
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				letterCount++
			}
		}

		if totalCount > 0 {
			ratio := float64(letterCount) / float64(totalCount)
			expected := ratio > 0.5

			if result != expected {
				t.Errorf("isPinyinQuery(%q) = %v, want %v (letters: %d/%d, ratio: %.2f)",
					input, result, expected, letterCount, totalCount, ratio)
			}
		}
	})
}
