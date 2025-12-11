package classifier

import (
	"testing"
)

// BenchmarkToTraditional benchmarks the ToTraditional function
func BenchmarkToTraditional(b *testing.B) {
	testCases := []struct {
		name  string
		input string
	}{
		{"short", "简体中文"},
		{"medium", "床前明月光，疑是地上霜。举头望明月，低头思故乡。"},
		{"long", "春眠不觉晓，处处闻啼鸟。夜来风雨声，花落知多少。白日依山尽，黄河入海流。欲穷千里目，更上一层楼。"},
		{"mixed", "测试text混合123内容"},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			b.ResetTimer()
			for b.Loop() {
				_, _ = ToTraditional(tc.input)
			}
		})
	}
}

// BenchmarkToSimplified benchmarks the ToSimplified function
func BenchmarkToSimplified(b *testing.B) {
	testCases := []struct {
		name  string
		input string
	}{
		{"short", "繁體中文"},
		{"medium", "牀前明月光，疑是地上霜。舉頭望明月，低頭思故鄉。"},
		{"long", "春眠不覺曉，處處聞啼鳥。夜來風雨聲，花落知多少。白日依山盡，黃河入海流。欲窮千里目，更上一層樓。"},
		{"mixed", "測試text混合123內容"},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			b.ResetTimer()
			for b.Loop() {
				_, _ = ToSimplified(tc.input)
			}
		})
	}
}

// BenchmarkToTraditionalArray benchmarks the ToTraditionalArray function
func BenchmarkToTraditionalArray(b *testing.B) {
	testCases := []struct {
		name  string
		input []string
	}{
		{"small", []string{"简体", "中文", "测试"}},
		{"medium", []string{"床前明月光", "疑是地上霜", "举头望明月", "低头思故乡"}},
		{"large", []string{
			"春眠不觉晓", "处处闻啼鸟", "夜来风雨声", "花落知多少",
			"白日依山尽", "黄河入海流", "欲穷千里目", "更上一层楼",
		}},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			b.ResetTimer()
			for b.Loop() {
				_, _ = ToTraditionalArray(tc.input)
			}
		})
	}
}

// BenchmarkToSimplifiedArray benchmarks the ToSimplifiedArray function
func BenchmarkToSimplifiedArray(b *testing.B) {
	testCases := []struct {
		name  string
		input []string
	}{
		{"small", []string{"繁體", "中文", "測試"}},
		{"medium", []string{"牀前明月光", "疑是地上霜", "舉頭望明月", "低頭思故鄉"}},
		{"large", []string{
			"春眠不覺曉", "處處聞啼鳥", "夜來風雨聲", "花落知多少",
			"白日依山盡", "黃河入海流", "欲窮千里目", "更上一層樓",
		}},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			b.ResetTimer()
			for b.Loop() {
				_, _ = ToSimplifiedArray(tc.input)
			}
		})
	}
}

// BenchmarkConvertPoemToTraditional benchmarks the ConvertPoemToTraditional function
func BenchmarkConvertPoemToTraditional(b *testing.B) {
	title := "静夜思"
	author := "李白"
	content := "床前明月光，疑是地上霜。举头望明月，低头思故乡。"
	rhythmic := ""

	b.ResetTimer()
	for b.Loop() {
		_, _, _, _, _ = ConvertPoemToTraditional(title, author, content, rhythmic)
	}
}
