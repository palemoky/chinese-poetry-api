package classifier

import (
	"testing"
	"unicode/utf8"
)

// FuzzClassifyPoetryType 用随机输入对 ClassifyPoetryType 做模糊测试。
func FuzzClassifyPoetryType(f *testing.F) {
	// 用已知的诗词结构作为种子语料
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

		// 不应 panic
		result := ClassifyPoetryType(paragraphs, rhythmic)

		// 返回结构的各字段都应有效
		if result.TypeName == "" {
			t.Error("ClassifyPoetryType returned empty TypeName")
		}
		if result.Category == "" {
			t.Error("ClassifyPoetryType returned empty Category")
		}

		// TypeName 应属于已知体裁之一
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

		// Category 应属于已知大类之一
		validCategories := map[string]bool{
			CategoryPoetry: true,
			CategoryCi:     true,
			CategoryOther:  true,
		}
		if !validCategories[result.Category] {
			t.Errorf("ClassifyPoetryType returned invalid Category: %q", result.Category)
		}

		// 带词牌名的应判定为词
		if rhythmic != "" && result.TypeName != TypeCi {
			t.Errorf("ClassifyPoetryType with rhythmic %q should return TypeCi, got %q", rhythmic, result.TypeName)
		}
	})
}

// FuzzRemovePunctuation 对 removePunctuation 做模糊测试。
func FuzzRemovePunctuation(f *testing.F) {
	// 种子语料
	f.Add("床前明月光，疑是地上霜。")
	f.Add("！@#$%^&*()")
	f.Add("测试，。！？；：")
	f.Add("")
	f.Add("no punctuation")
	f.Add("混合text，with。punctuation！")

	f.Fuzz(func(t *testing.T, input string) {
		// 不应 panic
		result := removePunctuation(input)

		// 结果应是合法的 UTF-8
		if !utf8.ValidString(result) {
			t.Errorf("removePunctuation(%q) returned invalid UTF-8: %q", input, result)
		}

		// 结果中不应残留常见标点
		commonPunct := []string{"，", "。", "！", "？", "；", "：", ",", ".", "!", "?"}
		for _, p := range commonPunct {
			if len(result) > 0 && contains(result, p) {
				t.Errorf("removePunctuation(%q) still contains punctuation %q: %q", input, p, result)
			}
		}
	})
}

// FuzzSplitBySentence 对 splitBySentence 做模糊测试。
func FuzzSplitBySentence(f *testing.F) {
	// 种子语料
	f.Add("床前明月光。疑是地上霜。")
	f.Add("测试！多个？句子；分割，逗号")
	f.Add("")
	f.Add("no delimiters")
	f.Add("。。。")

	f.Fuzz(func(t *testing.T, input string) {
		// 不应 panic
		result := splitBySentence(input)

		// 每一项结果都应非空；输入为合法 UTF-8 时，切分结果也应合法
		// （非法字节只会原样保留，不应要求输出比输入更合法）
		validInput := utf8.ValidString(input)
		for i, s := range result {
			if s == "" {
				t.Errorf("splitBySentence(%q)[%d] returned empty string", input, i)
			}
			if validInput && !utf8.ValidString(s) {
				t.Errorf("splitBySentence(%q)[%d] returned invalid UTF-8: %q", input, i, s)
			}
		}
	})
}

// 判断字符串是否包含指定子串的辅助函数
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
