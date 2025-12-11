package classifier

import (
	"strings"
	"unicode/utf8"
)

// Poetry type constants
const (
	// Categories
	CategoryPoetry = "唐诗"
	CategoryCi     = "宋词"
	CategoryOther  = "其他"

	// Specific types
	TypeWuyanJueju = "五言绝句"
	TypeQiyanJueju = "七言绝句"
	TypeWuyanLvshi = "五言律诗"
	TypeQiyanLvshi = "七言律诗"
	TypeCi         = "宋词"
	TypeOther      = "其他"

	// Structure constraints
	JuejuLines = 4
	LvshiLines = 8
	WuyanChars = 5
	QiyanChars = 7
)

// PoetryTypeInfo contains information about a classified poetry type
type PoetryTypeInfo struct {
	TypeName     string
	Category     string
	Lines        *int
	CharsPerLine *int
}

// ClassifyPoetryType determines the type of poetry based on its structure
func ClassifyPoetryType(paragraphs []string, rhythmic string) PoetryTypeInfo {
	return ClassifyPoetryTypeWithDataset(paragraphs, rhythmic, "", "")
}

// ClassifyPoetryTypeWithDataset determines the type of poetry based on dataset source and structure
// Priority order:
// 1. Dataset-based direct mapping (for shijing, chuci, lunyu, mengzi, yuanqu)
// 2. Rhythmic field check (for songci)
// 2.5. Yuefu poem title check
// 3. Structure analysis (for tangshi)
func ClassifyPoetryTypeWithDataset(paragraphs []string, rhythmic string, datasetKey string, title string) PoetryTypeInfo {
	// Priority 1: Check dataset key for direct type mapping
	if typeInfo, ok := getTypeFromDataset(datasetKey); ok {
		return typeInfo
	}

	// Priority 2: If it has a rhythmic field, it's ci (词)
	if rhythmic != "" {
		return PoetryTypeInfo{
			TypeName: TypeCi,
			Category: CategoryCi,
		}
	}

	// Priority 2.5: Check if it's a Yuefu poem by title
	if title != "" && isYuefuPoem(title) {
		return PoetryTypeInfo{
			TypeName: "乐府诗",
			Category: "唐诗",
		}
	}

	// Priority 3: Structure-based classification for Tang poetry
	if len(paragraphs) == 0 {
		return PoetryTypeInfo{
			TypeName: TypeOther,
			Category: CategoryOther,
		}
	}

	// Split merged lines (e.g., "江南有美人，别后长相忆。" → ["江南有美人", "别后长相忆"])
	expandedLines := expandParagraphs(paragraphs)

	// Check if expansion resulted in empty lines
	if len(expandedLines) == 0 {
		return PoetryTypeInfo{
			TypeName: TypeOther,
			Category: CategoryOther,
		}
	}

	// Count lines and characters per line
	lineCount := len(expandedLines)
	charCounts := make([]int, lineCount)

	for i, line := range expandedLines {
		// Remove punctuation and count characters
		cleaned := removePunctuation(line)
		charCounts[i] = utf8.RuneCountInString(cleaned)
	}

	// Check if all lines have the same character count
	if !isUniform(charCounts) {
		// Irregular structure
		return PoetryTypeInfo{
			TypeName: TypeOther,
			Category: CategoryOther,
		}
	}

	charsPerLine := charCounts[0]

	// Classify based on line count and characters per line
	typeName, category := classifyByStructure(lineCount, charsPerLine)

	return PoetryTypeInfo{
		TypeName:     typeName,
		Category:     category,
		Lines:        &lineCount,
		CharsPerLine: &charsPerLine,
	}
}

// getTypeFromDataset returns poetry type info based on dataset key
// Returns (typeInfo, true) if dataset has a direct mapping, (empty, false) otherwise
func getTypeFromDataset(datasetKey string) (PoetryTypeInfo, bool) {
	// Map dataset keys to their corresponding poetry types
	datasetTypeMap := map[string]PoetryTypeInfo{
		"shijing": {
			TypeName: "诗经",
			Category: "诗经",
		},
		"chuci": {
			TypeName: "楚辞",
			Category: "楚辞",
		},
		"lunyu": {
			TypeName: "论语",
			Category: "论语",
		},
		"mengzi": {
			TypeName: "四书五经",
			Category: "四书五经",
		},
		"yuanqu": {
			TypeName: "元曲",
			Category: "曲",
		},
		"wudai-huajianji": {
			TypeName: "五代词",
			Category: "词",
		},
		"wudai-nantang": {
			TypeName: "五代词",
			Category: "词",
		},
		"nalanxingde": {
			TypeName: "宋词", // 纳兰性德是清代，但词的形式与宋词相同
			Category: "宋词",
		},
		"caocao": {
			TypeName: "乐府诗",
			Category: "唐诗",
		},
	}

	if typeInfo, ok := datasetTypeMap[datasetKey]; ok {
		return typeInfo, true
	}

	return PoetryTypeInfo{}, false
}

// classifyByStructure classifies poetry based on line count and characters per line
func classifyByStructure(lines, chars int) (typeName, category string) {
	switch {
	case lines == JuejuLines && chars == WuyanChars:
		return TypeWuyanJueju, CategoryPoetry
	case lines == JuejuLines && chars == QiyanChars:
		return TypeQiyanJueju, CategoryPoetry
	case lines == LvshiLines && chars == WuyanChars:
		return TypeWuyanLvshi, CategoryPoetry
	case lines == LvshiLines && chars == QiyanChars:
		return TypeQiyanLvshi, CategoryPoetry
	default:
		return TypeOther, CategoryOther
	}
}

// isUniform checks if all integers in a slice are equal
func isUniform(nums []int) bool {
	if len(nums) == 0 {
		return true
	}
	first := nums[0]
	for _, n := range nums[1:] {
		if n != first {
			return false
		}
	}
	return true
}

// expandParagraphs splits paragraphs by sentence-ending punctuation
func expandParagraphs(paragraphs []string) []string {
	var result []string

	for _, para := range paragraphs {
		// Split by common sentence-ending punctuation
		// 。！？；are common Chinese sentence enders
		lines := splitBySentence(para)
		result = append(result, lines...)
	}

	return result
}

// splitBySentence splits text by sentence-ending punctuation
func splitBySentence(text string) []string {
	// Replace sentence-ending punctuation with a delimiter
	delimiters := []string{"。", "！", "？", "；", "，"}
	for _, delim := range delimiters {
		text = strings.ReplaceAll(text, delim, "\n")
	}

	// Split by newline and filter empty strings
	lines := strings.Split(text, "\n")
	var result []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			result = append(result, line)
		}
	}

	return result
}

// removePunctuation removes all punctuation from text
func removePunctuation(text string) string {
	// Common Chinese and English punctuation
	punctuation := `，。！？；：""''（）《》【】、·—…,.!?;:'"()[]{}/-`

	// Use strings.Map for efficient single-pass filtering
	result := strings.Map(func(r rune) rune {
		if strings.ContainsRune(punctuation, r) {
			return -1 // Remove this character
		}
		return r
	}, text)

	return strings.TrimSpace(result)
}

// isYuefuPoem checks if a poem is a Yuefu poem based on its title
func isYuefuPoem(title string) bool {
	// Common Yuefu poem titles
	yuefuTitles := []string{
		// 边塞乐府
		"凉州词", "出塞", "从军行", "塞下曲", "塞上曲",
		"关山月", "渡荆门", "渡远荆门外",

		// 送别乐府
		"送友人", "送孟浩然", "送元二使安西",
		"送友人入蜀", "宣州送裴坡判", "宣州送裴坡判官归京",

		// 抒情乐府
		"将进酒", "行路难", "长相思", "春思", "秋思",
		"子夜吴歌", "清平调",

		// 山水游历
		"蜀道难", "梦游天姥", "侠客行",
		"登金陵凤凰台", "黄鹤楼",
		"宣州谢脸楼", "宣城见杜鹃花",
		"宣州谢脸楼饿别校书叔云",
		"渡浙江问舟中人",

		// 白居易乐府
		"琵琶行", "长恨歌", "卖炭翁", "观刈麦",
		"新丰折臂翁", "上阳白发人", "井底引银瓶",
		"杜陵叟", "缭绫",

		// 杜甫乐府
		"兵车行", "丽人行", "哀江头", "哀王孙",
		"新安吏", "石壕吏", "潼关吏",
		"新婚别", "垂老别", "无家别",

		// 王维乐府
		"老将行", "桃源行", "洛阳女儿行",

		// 高适乐府
		"燕歌行", "别董大", "营州歌",

		// 岑参乐府
		"白雪歌", "走马川", "轮台歌",

		// 王昌龄乐府
		"芙蓉楼", "闺怨",

		// 刘禹锡乐府
		"竹枝词", "杨柳枝", "浪淘沙", "乌衣巷",
		"石头城", "西塞山怀古",

		// 韩愈乐府
		"山石", "谒衡岳庙", "八月十五夜赠张功曹",

		// 柳宗元乐府
		"渔翁", "江雪",

		// 孟郊乐府
		"游子吟", "秋怀", "烈女操",

		// 元稹乐府
		"遣悲怀", "离思", "行宫",

		// 李贺乐府
		"雁门太守行", "金铜仙人辞汉歌", "苏小小墓",
		"梦天", "李凭箜篌引",

		// 其他常见乐府题
		"古风", "古意", "拟古", "采莲曲", "江南曲",
		"白头吟", "怨歌行", "短歌行", "长歌行",
		"陇西行", "陌上桑", "木兰诗",
		"孔雀东南飞", "悲愤诗",

		// 汉魏六朝乐府
		"饮马长城窟行", "十五从军征", "上邪",
		"有所思", "上山采蘼芜", "江南",
	}

	// Check exact title matches
	for _, yuefuTitle := range yuefuTitles {
		if strings.Contains(title, yuefuTitle) {
			return true
		}
	}

	// Check for common Yuefu patterns (suffixes)
	// 曲辞、歌辞、歌行、乐府 are typical Yuefu markers
	yuefuPatterns := []string{
		"曲辞", "歌辞", "歌行", "乐府",
	}

	for _, pattern := range yuefuPatterns {
		if strings.Contains(title, pattern) {
			return true
		}
	}

	return false
}
