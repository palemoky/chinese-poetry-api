package classifier

import (
	"testing"
	"unicode/utf8"
)

// FuzzClassifyPoetryType tests the ClassifyPoetryType function with random inputs
func FuzzClassifyPoetryType(f *testing.F) {
	// Seed corpus with known poetry structures
	f.Add("床前明月光", "疑是地上霜", "举头望明月", "低头思故乡", "")
	f.Add("春眠不觉晓", "处处闻啼鸟", "夜来风雨声", "花落知多少", "")
	f.Add("", "", "", "", "")
	f.Add("test", "", "", "", "")
	f.Add("很长的一行诗句超过了正常的字数限制", "", "", "", "")

	f.Fuzz(func(t *testing.T, p1, p2, p3, p4, rhythmic string) {
		paragraphs := []string{}
		if p1 != "" {
			paragraphs = append(paragraphs, p1)
		}
		if p2 != "" {
			paragraphs = append(paragraphs, p2)
		}
		if p3 != "" {
			paragraphs = append(paragraphs, p3)
		}
		if p4 != "" {
			paragraphs = append(paragraphs, p4)
		}

		// Should not panic
		result := ClassifyPoetryType(paragraphs, rhythmic)

		// Result should have valid fields
		if result.TypeName == "" {
			t.Error("ClassifyPoetryType returned empty TypeName")
		}
		if result.Category == "" {
			t.Error("ClassifyPoetryType returned empty Category")
		}

		// TypeName should be one of the known types
		validTypes := map[string]bool{
			TypeWuyanJueju: true,
			TypeQiyanJueju: true,
			TypeWuyanLvshi: true,
			TypeQiyanLvshi: true,
			TypeCi:         true,
			TypeOther:      true,
		}
		if !validTypes[result.TypeName] {
			t.Errorf("ClassifyPoetryType returned invalid TypeName: %q", result.TypeName)
		}

		// Category should be one of the known categories
		validCategories := map[string]bool{
			CategoryPoetry: true,
			CategoryCi:     true,
			CategoryOther:  true,
		}
		if !validCategories[result.Category] {
			t.Errorf("ClassifyPoetryType returned invalid Category: %q", result.Category)
		}

		// If rhythmic is provided, should be classified as Ci
		if rhythmic != "" && result.TypeName != TypeCi {
			t.Errorf("ClassifyPoetryType with rhythmic %q should return TypeCi, got %q", rhythmic, result.TypeName)
		}
	})
}

// FuzzRemovePunctuation tests the removePunctuation function
func FuzzRemovePunctuation(f *testing.F) {
	// Seed corpus
	f.Add("床前明月光，疑是地上霜。")
	f.Add("！@#$%^&*()")
	f.Add("测试，。！？；：")
	f.Add("")
	f.Add("no punctuation")
	f.Add("混合text，with。punctuation！")

	f.Fuzz(func(t *testing.T, input string) {
		// Should not panic
		result := removePunctuation(input)

		// Result should be valid UTF-8
		if !utf8.ValidString(result) {
			t.Errorf("removePunctuation(%q) returned invalid UTF-8: %q", input, result)
		}

		// Result should not contain common punctuation
		commonPunct := []string{"，", "。", "！", "？", "；", "：", ",", ".", "!", "?"}
		for _, p := range commonPunct {
			if len(result) > 0 && contains(result, p) {
				t.Errorf("removePunctuation(%q) still contains punctuation %q: %q", input, p, result)
			}
		}
	})
}

// FuzzSplitBySentence tests the splitBySentence function
func FuzzSplitBySentence(f *testing.F) {
	// Seed corpus
	f.Add("床前明月光。疑是地上霜。")
	f.Add("测试！多个？句子；分割，逗号")
	f.Add("")
	f.Add("no delimiters")
	f.Add("。。。")

	f.Fuzz(func(t *testing.T, input string) {
		// Should not panic
		result := splitBySentence(input)

		// All results should be non-empty and valid UTF-8
		for i, s := range result {
			if s == "" {
				t.Errorf("splitBySentence(%q)[%d] returned empty string", input, i)
			}
			if !utf8.ValidString(s) {
				t.Errorf("splitBySentence(%q)[%d] returned invalid UTF-8: %q", input, i, s)
			}
		}
	})
}

// Helper function to check if string contains substring
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
