package classifier

import (
	"fmt"

	"github.com/liuzl/gocc"
)

// s2t 与 t2s 在 init() 中初始化一次，可并发使用：
// 底层的 gocc.OpenCC.Convert 方法本身是并发安全的。
var (
	s2t *gocc.OpenCC // 简转繁
	t2s *gocc.OpenCC // 繁转简
)

func init() {
	var err error

	// 初始化简转繁转换器
	s2t, err = gocc.New("s2t")
	if err != nil {
		panic(fmt.Sprintf("failed to initialize s2t converter: %v", err))
	}

	// 初始化繁转简转换器
	t2s, err = gocc.New("t2s")
	if err != nil {
		panic(fmt.Sprintf("failed to initialize t2s converter: %v", err))
	}
}

// ToTraditional 把简体中文转为繁体。
func ToTraditional(text string) (string, error) {
	return s2t.Convert(text)
}

// ToSimplified 把繁体中文转为简体。
func ToSimplified(text string) (string, error) {
	return t2s.Convert(text)
}

// IsTraditional 判断一段文本是否以繁体书写。
//
// 分别做繁转简与简转繁，看哪个方向改动的字更多：繁体文本繁转简时改动多、
// 简转繁时只会动那些简繁同形的字（游、里、云……），简体文本则正好相反。
// 在全部数据集上验证过：全唐诗 31 万首判为繁体、只有 34 首判为简体，
// 其余以简体书写的数据集无一误判；两边改动一样多时（多为没有简繁差异的短文本）
// 视为简体。
func IsTraditional(text string) (bool, error) {
	toS, err := ToSimplified(text)
	if err != nil {
		return false, err
	}
	toT, err := ToTraditional(text)
	if err != nil {
		return false, err
	}
	return runeDiff(text, toS) > runeDiff(text, toT), nil
}

// runeDiff 统计两段文本逐字比较时不同的字数。简繁转换是逐字（或逐词等长）替换，
// 长度不变的位置一一对应；长度不同的部分不计入。
func runeDiff(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	n := 0
	for i := range min(len(ra), len(rb)) {
		if ra[i] != rb[i] {
			n++
		}
	}
	return n
}

// ToTraditionalArray 批量把字符串转为繁体。
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

// ToSimplifiedArray 批量把字符串转为简体。
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

// ToTraditionalPointer 把字符串指针指向的内容转为繁体，nil 或空串原样返回。
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

// ConvertPoemToTraditional 把一首诗词的各个字段统一转为繁体。
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
