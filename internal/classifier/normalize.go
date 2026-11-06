package classifier

import (
	"strings"
	"unicode"
)

// NormalizeText 归一化文本：去除首尾空白，并把连续空白压缩为单个空格。
func NormalizeText(text string) string {
	text = strings.TrimSpace(text)

	// 连续空白压缩为一个空格
	text = strings.Join(strings.Fields(text), " ")

	return text
}

// hasValidContent 判断文本除标点和空白外是否还有实际内容。
func hasValidContent(text string) bool {
	for _, r := range text {
		// 只要出现一个既非标点也非空白的字符，就认为有实际内容
		if !unicode.IsPunct(r) && !unicode.IsSpace(r) {
			return true
		}
	}
	return false
}

// NormalizeTextArray 批量归一化文本并剔除无效项，
// 无效项包括空串、纯空白以及只有标点的内容。
func NormalizeTextArray(texts []string) []string {
	result := make([]string, 0, len(texts))
	for _, text := range texts {
		normalized := NormalizeText(text)
		if normalized != "" && hasValidContent(normalized) {
			result = append(result, normalized)
		}
	}
	return result
}

// placeholderPhrases 是原始数据中表示「本篇无正文」的占位串，导入时应当整条跳过。
var placeholderPhrases = []string{
	"无正文。",
	"無正文。",
	"空。",
}

// IsPlaceholderContent 判断整段正文是否只是「无正文。」「空。」这类占位内容。
func IsPlaceholderContent(paragraphs []string) bool {
	if len(paragraphs) == 0 {
		return false
	}
	// 拼接后与各占位串逐一比对
	joined := strings.Join(paragraphs, "")
	for _, p := range placeholderPhrases {
		if joined == p {
			return true
		}
	}
	return false
}

// isClosingQuote 判断 r 是否为中文右引号或右括号类符号。
func isClosingQuote(r rune) bool {
	return r == '\u201D' || // "
		r == '\u300B' || // 》
		r == '\u3011' || // 】
		r == '\u300D' || // 」
		r == '\u300F' // 』
}

// SplitSentences 按句末标点（。！？）把中文文本切分成单句，
// 句末标点后紧跟的右引号会一并归入该句。
// 若文本中不含任何句末标点，则原样返回长度为 1 的切片。
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
			// 句末标点后若紧跟右引号，一并纳入本句
			if end < n && isClosingQuote(runes[end]) {
				end++
				i++ // 下一轮循环跳过该右引号
			}
			s := strings.TrimSpace(string(runes[start:end]))
			if s != "" && hasValidContent(s) {
				sentences = append(sentences, s)
			}
			start = end
		}
	}

	// 收尾：处理末尾没有句末标点的残余内容
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

// NormalizeAndSplitParagraphs 逐段归一化，并把粘连在一起的句子拆成独立元素。
// 当原始数据可能把多句合并成一个字符串时（如 "A。B。" 而非 ["A。","B。"]），
// 应使用本函数而非 NormalizeTextArray。
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
