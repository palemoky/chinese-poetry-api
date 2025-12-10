package classifier

import (
	"testing"
	"unicode/utf8"
)

// FuzzToTraditional tests the ToTraditional function with random inputs
func FuzzToTraditional(f *testing.F) {
	// Seed corpus with known cases
	f.Add("简体中文")
	f.Add("床前明月光")
	f.Add("李白")
	f.Add("")
	f.Add("123abc!@#")
	f.Add("繁體中文") // Already traditional
	f.Add("混合text文字123")

	f.Fuzz(func(t *testing.T, input string) {
		// The function should not panic
		result, err := ToTraditional(input)

		// Should not return error for valid UTF-8 strings
		if utf8.ValidString(input) && err != nil {
			t.Errorf("ToTraditional(%q) returned unexpected error: %v", input, err)
		}

		// Result should be valid UTF-8
		if !utf8.ValidString(result) {
			t.Errorf("ToTraditional(%q) returned invalid UTF-8: %q", input, result)
		}

		// Converting back should give us something (idempotency test)
		if err == nil {
			backConverted, err2 := ToSimplified(result)
			if err2 != nil {
				t.Errorf("ToSimplified(ToTraditional(%q)) failed: %v", input, err2)
			}
			if !utf8.ValidString(backConverted) {
				t.Errorf("Round-trip conversion produced invalid UTF-8")
			}
		}
	})
}

// FuzzToSimplified tests the ToSimplified function with random inputs
func FuzzToSimplified(f *testing.F) {
	// Seed corpus with known cases
	f.Add("繁體中文")
	f.Add("靜夜思")
	f.Add("李白")
	f.Add("")
	f.Add("123abc!@#")
	f.Add("简体中文") // Already simplified
	f.Add("混合text文字123")

	f.Fuzz(func(t *testing.T, input string) {
		// The function should not panic
		result, err := ToSimplified(input)

		// Should not return error for valid UTF-8 strings
		if utf8.ValidString(input) && err != nil {
			t.Errorf("ToSimplified(%q) returned unexpected error: %v", input, err)
		}

		// Result should be valid UTF-8
		if !utf8.ValidString(result) {
			t.Errorf("ToSimplified(%q) returned invalid UTF-8: %q", input, result)
		}

		// Converting back should give us something (idempotency test)
		if err == nil {
			backConverted, err2 := ToTraditional(result)
			if err2 != nil {
				t.Errorf("ToTraditional(ToSimplified(%q)) failed: %v", input, err2)
			}
			if !utf8.ValidString(backConverted) {
				t.Errorf("Round-trip conversion produced invalid UTF-8")
			}
		}
	})
}

// FuzzToTraditionalArray tests the ToTraditionalArray function
func FuzzToTraditionalArray(f *testing.F) {
	// Seed corpus
	f.Add("简体", "中文", "测试")
	f.Add("", "", "")
	f.Add("李白", "杜甫", "白居易")

	f.Fuzz(func(t *testing.T, s1, s2, s3 string) {
		input := []string{s1, s2, s3}

		// Should not panic
		result, err := ToTraditionalArray(input)

		// Check for valid UTF-8 inputs
		allValid := true
		for _, s := range input {
			if !utf8.ValidString(s) {
				allValid = false
				break
			}
		}

		if allValid && err != nil {
			t.Errorf("ToTraditionalArray(%v) returned unexpected error: %v", input, err)
		}

		// Result should have same length
		if err == nil && len(result) != len(input) {
			t.Errorf("ToTraditionalArray(%v) returned wrong length: got %d, want %d", input, len(result), len(input))
		}

		// All results should be valid UTF-8
		if err == nil {
			for i, s := range result {
				if !utf8.ValidString(s) {
					t.Errorf("ToTraditionalArray(%v)[%d] returned invalid UTF-8: %q", input, i, s)
				}
			}
		}
	})
}
