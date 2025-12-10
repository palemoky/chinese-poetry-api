package classifier

import (
	"testing"
)

// BenchmarkClassifyPoetryType benchmarks the ClassifyPoetryType function
func BenchmarkClassifyPoetryType(b *testing.B) {
	testCases := []struct {
		name       string
		paragraphs []string
		rhythmic   string
	}{
		{
			name:       "wuyan_jueju",
			paragraphs: []string{"床前明月光", "疑是地上霜", "举头望明月", "低头思故乡"},
			rhythmic:   "",
		},
		{
			name:       "qiyan_jueju",
			paragraphs: []string{"两个黄鹂鸣翠柳", "一行白鹭上青天", "窗含西岭千秋雪", "门泊东吴万里船"},
			rhythmic:   "",
		},
		{
			name:       "wuyan_lvshi",
			paragraphs: []string{"春眠不觉晓", "处处闻啼鸟", "夜来风雨声", "花落知多少", "空山新雨后", "天气晚来秋", "明月松间照", "清泉石上流"},
			rhythmic:   "",
		},
		{
			name:       "ci",
			paragraphs: []string{"红酥手", "黄縢酒"},
			rhythmic:   "钗头凤",
		},
		{
			name:       "irregular",
			paragraphs: []string{"这是一首", "不规则的", "诗词"},
			rhythmic:   "",
		},
		{
			name:       "empty",
			paragraphs: []string{},
			rhythmic:   "",
		},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = ClassifyPoetryType(tc.paragraphs, tc.rhythmic)
			}
		})
	}
}

// BenchmarkRemovePunctuation benchmarks the removePunctuation function
func BenchmarkRemovePunctuation(b *testing.B) {
	testCases := []struct {
		name  string
		input string
	}{
		{"no_punct", "床前明月光疑是地上霜"},
		{"chinese_punct", "床前明月光，疑是地上霜。举头望明月，低头思故乡。"},
		{"mixed_punct", "测试，。！？；：\"\"''（）《》【】"},
		{"english_punct", "Hello, world! How are you?"},
		{"heavy_punct", "！！！测试？？？内容。。。"},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = removePunctuation(tc.input)
			}
		})
	}
}

// BenchmarkSplitBySentence benchmarks the splitBySentence function
func BenchmarkSplitBySentence(b *testing.B) {
	testCases := []struct {
		name  string
		input string
	}{
		{"single", "床前明月光"},
		{"multiple", "床前明月光。疑是地上霜。举头望明月。低头思故乡。"},
		{"mixed_delim", "测试！多个？句子；分割，逗号。"},
		{"no_delim", "没有分隔符的长文本内容"},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = splitBySentence(tc.input)
			}
		})
	}
}

// BenchmarkExpandParagraphs benchmarks the expandParagraphs function
func BenchmarkExpandParagraphs(b *testing.B) {
	testCases := []struct {
		name       string
		paragraphs []string
	}{
		{"simple", []string{"床前明月光", "疑是地上霜"}},
		{"with_punct", []string{"床前明月光，疑是地上霜。", "举头望明月，低头思故乡。"}},
		{"mixed", []string{"简单句子", "复杂句子，包含标点。还有更多！"}},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = expandParagraphs(tc.paragraphs)
			}
		})
	}
}
