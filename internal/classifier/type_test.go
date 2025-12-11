package classifier

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClassifyPoetryType(t *testing.T) {
	tests := []struct {
		name       string
		paragraphs []string
		rhythmic   string
		want       PoetryTypeInfo
	}{
		// 词
		{
			name:       "词 with rhythmic field",
			paragraphs: []string{"明月几时有", "把酒问青天"},
			rhythmic:   "水调歌头",
			want: PoetryTypeInfo{
				TypeName: "宋词",
				Category: "宋词",
			},
		},
		// 五言绝句
		{
			name:       "五言绝句 - 无标点",
			paragraphs: []string{"春眠不觉晓", "处处闻啼鸟", "夜来风雨声", "花落知多少"},
			rhythmic:   "",
			want: PoetryTypeInfo{
				TypeName:     "五言绝句",
				Category:     "唐诗",
				Lines:        intPtr(4),
				CharsPerLine: intPtr(5),
			},
		},
		{
			name:       "五言绝句 - 带标点格式",
			paragraphs: []string{"春眠不觉晓，处处闻啼鸟。", "夜来风雨声，花落知多少。"},
			rhythmic:   "",
			want: PoetryTypeInfo{
				TypeName:     "五言绝句",
				Category:     "唐诗",
				Lines:        intPtr(4),
				CharsPerLine: intPtr(5),
			},
		},
		// 七言绝句
		{
			name:       "七言绝句 - 无标点",
			paragraphs: []string{"两个黄鹂鸣翠柳", "一行白鹭上青天", "窗含西岭千秋雪", "门泊东吴万里船"},
			rhythmic:   "",
			want: PoetryTypeInfo{
				TypeName:     "七言绝句",
				Category:     "唐诗",
				Lines:        intPtr(4),
				CharsPerLine: intPtr(7),
			},
		},
		{
			name:       "七言绝句 - 带标点格式",
			paragraphs: []string{"岐王宅里寻常见，崔九堂前几度闻。", "正是江南好风景，落花时节又逢君。"},
			rhythmic:   "",
			want: PoetryTypeInfo{
				TypeName:     "七言绝句",
				Category:     "唐诗",
				Lines:        intPtr(4),
				CharsPerLine: intPtr(7),
			},
		},
		// 五言律诗
		{
			name: "五言律诗 - 无标点",
			paragraphs: []string{
				"空山新雨后", "天气晚来秋",
				"明月松间照", "清泉石上流",
				"竹喧归浣女", "莲动下渔舟",
				"随意春芳歇", "王孙自可留",
			},
			rhythmic: "",
			want: PoetryTypeInfo{
				TypeName:     "五言律诗",
				Category:     "唐诗",
				Lines:        intPtr(8),
				CharsPerLine: intPtr(5),
			},
		},
		{
			name:       "五言律诗 - 带标点格式（每段两句）",
			paragraphs: []string{"影暗才分竹，烟低正满簷。", "雨斜侵药裹，风过乱书签。", "篆灭香犹在，尘昏砚未添。", "静中时有兴，著论不为潜。"},
			rhythmic:   "",
			want: PoetryTypeInfo{
				TypeName:     "五言律诗",
				Category:     "唐诗",
				Lines:        intPtr(8),
				CharsPerLine: intPtr(5),
			},
		},
		// 七言律诗
		{
			name: "七言律诗 - 无标点",
			paragraphs: []string{
				"风急天高猿啸哀",
				"渚清沙白鸟飞回",
				"无边落木萧萧下",
				"不尽长江滚滚来",
				"万里悲秋常作客",
				"百年多病独登台",
				"艰难苦恨繁霜鬓",
				"潦倒新停浊酒杯",
			},
			rhythmic: "",
			want: PoetryTypeInfo{
				TypeName:     "七言律诗",
				Category:     "唐诗",
				Lines:        intPtr(8),
				CharsPerLine: intPtr(7),
			},
		},
		{
			name: "七言律诗 - 带标点格式",
			paragraphs: []string{
				"风急天高猿啸哀，渚清沙白鸟飞回。",
				"无边落木萧萧下，不尽长江滚滚来。",
				"万里悲秋常作客，百年多病独登台。",
				"艰难苦恨繁霜鬓，潦倒新停浊酒杯。",
			},
			rhythmic: "",
			want: PoetryTypeInfo{
				TypeName:     "七言律诗",
				Category:     "唐诗",
				Lines:        intPtr(8),
				CharsPerLine: intPtr(7),
			},
		},
		// 边界情况
		{
			name:       "empty paragraphs",
			paragraphs: []string{},
			rhythmic:   "",
			want: PoetryTypeInfo{
				TypeName: "其他",
				Category: "其他",
			},
		},
		{
			name:       "irregular format - 不均匀行长",
			paragraphs: []string{"短", "这是一首很长的句子超过十个字"},
			rhythmic:   "",
			want: PoetryTypeInfo{
				TypeName: "其他",
				Category: "其他",
			},
		},
		{
			name:       "irregular format - 非标准行数",
			paragraphs: []string{"床前明月光", "疑是地上霜", "举头望明月"},
			rhythmic:   "",
			want: PoetryTypeInfo{
				TypeName: "其他",
				Category: "其他",
			},
		},
		{
			name:       "空字符串段落应被过滤",
			paragraphs: []string{"春眠不觉晓。", "", "处处闻啼鸟。", "", "夜来风雨声。", "", "花落知多少。"},
			rhythmic:   "",
			want: PoetryTypeInfo{
				TypeName:     "五言绝句",
				Category:     "唐诗",
				Lines:        intPtr(4),
				CharsPerLine: intPtr(5),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClassifyPoetryType(tt.paragraphs, tt.rhythmic)

			assert.Equal(t, tt.want.TypeName, got.TypeName, "TypeName mismatch")
			assert.Equal(t, tt.want.Category, got.Category, "Category mismatch")

			if tt.want.Lines != nil {
				require.NotNil(t, got.Lines, "Lines should not be nil")
				assert.Equal(t, *tt.want.Lines, *got.Lines, "Lines mismatch")
			}

			if tt.want.CharsPerLine != nil {
				require.NotNil(t, got.CharsPerLine, "CharsPerLine should not be nil")
				assert.Equal(t, *tt.want.CharsPerLine, *got.CharsPerLine, "CharsPerLine mismatch")
			}
		})
	}
}

func TestNormalizeText(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "trim whitespace",
			input: "  春眠不觉晓  ",
			want:  "春眠不觉晓",
		},
		{
			name:  "normalize spaces",
			input: "春眠　不觉晓", // 全角空格
			want:  "春眠 不觉晓", // 半角空格
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
		{
			name:  "only whitespace",
			input: "   ",
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeText(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestNormalizeTextArray(t *testing.T) {
	tests := []struct {
		name  string
		input []string
		want  []string
	}{
		{
			name:  "normalize array",
			input: []string{"  春眠不觉晓  ", "  处处闻啼鸟  "},
			want:  []string{"春眠不觉晓", "处处闻啼鸟"},
		},
		{
			name:  "empty array",
			input: []string{},
			want:  []string{},
		},
		{
			name:  "filter empty strings",
			input: []string{"春眠不觉晓", "   ", "处处闻啼鸟"},
			want:  []string{"春眠不觉晓", "处处闻啼鸟"},
		},
		{
			name:  "filter punctuation-only content",
			input: []string{"春眠不觉晓", "。", "处处闻啼鸟", "，"},
			want:  []string{"春眠不觉晓", "处处闻啼鸟"},
		},
		{
			name:  "filter various punctuation",
			input: []string{"床前明月光", "。", "、", "；", "：", "！", "？", "疑是地上霜"},
			want:  []string{"床前明月光", "疑是地上霜"},
		},
		{
			name:  "filter punctuation with spaces",
			input: []string{"举头望明月", "  。  ", "  ，  ", "低头思故乡"},
			want:  []string{"举头望明月", "低头思故乡"},
		},
		{
			name:  "keep content with punctuation",
			input: []string{"春眠不觉晓，", "处处闻啼鸟。"},
			want:  []string{"春眠不觉晓，", "处处闻啼鸟。"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeTextArray(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRemovePunctuation(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "remove Chinese punctuation",
			input: "春眠不觉晓，处处闻啼鸟。",
			want:  "春眠不觉晓处处闻啼鸟",
		},
		{
			name:  "remove English punctuation",
			input: "Hello, World!",
			want:  "Hello World", // Space is not removed
		},
		{
			name:  "no punctuation",
			input: "春眠不觉晓",
			want:  "春眠不觉晓",
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := removePunctuation(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestClassifyPoetryTypeWithDataset(t *testing.T) {
	tests := []struct {
		name       string
		paragraphs []string
		rhythmic   string
		datasetKey string
		want       PoetryTypeInfo
	}{
		// Dataset-based direct mapping
		{
			name:       "诗经 - dataset mapping",
			paragraphs: []string{"关关雎鸠，在河之洲。", "窈窕淑女，君子好逑。"},
			rhythmic:   "",
			datasetKey: "shijing",
			want: PoetryTypeInfo{
				TypeName: "诗经",
				Category: "诗经",
			},
		},
		{
			name:       "楚辞 - dataset mapping",
			paragraphs: []string{"帝高阳之苗裔兮，朕皇考曰伯庸。"},
			rhythmic:   "",
			datasetKey: "chuci",
			want: PoetryTypeInfo{
				TypeName: "楚辞",
				Category: "楚辞",
			},
		},
		{
			name:       "论语 - dataset mapping",
			paragraphs: []string{"学而时习之，不亦说乎？"},
			rhythmic:   "",
			datasetKey: "lunyu",
			want: PoetryTypeInfo{
				TypeName: "论语",
				Category: "论语",
			},
		},
		{
			name:       "四书五经 - dataset mapping",
			paragraphs: []string{"天时不如地利，地利不如人和。"},
			rhythmic:   "",
			datasetKey: "mengzi",
			want: PoetryTypeInfo{
				TypeName: "四书五经",
				Category: "四书五经",
			},
		},
		{
			name:       "元曲 - dataset mapping",
			paragraphs: []string{"枯藤老树昏鸦，小桥流水人家。"},
			rhythmic:   "",
			datasetKey: "yuanqu",
			want: PoetryTypeInfo{
				TypeName: "元曲",
				Category: "曲",
			},
		},
		// Fallback to structure-based classification
		{
			name:       "五言绝句 - no dataset key",
			paragraphs: []string{"春眠不觉晓", "处处闻啼鸟", "夜来风雨声", "花落知多少"},
			rhythmic:   "",
			datasetKey: "",
			want: PoetryTypeInfo{
				TypeName:     "五言绝句",
				Category:     "唐诗",
				Lines:        intPtr(4),
				CharsPerLine: intPtr(5),
			},
		},
		{
			name:       "五言绝句 - unknown dataset key",
			paragraphs: []string{"春眠不觉晓", "处处闻啼鸟", "夜来风雨声", "花落知多少"},
			rhythmic:   "",
			datasetKey: "tangsong",
			want: PoetryTypeInfo{
				TypeName:     "五言绝句",
				Category:     "唐诗",
				Lines:        intPtr(4),
				CharsPerLine: intPtr(5),
			},
		},
		{
			name:       "宋词 - rhythmic field takes priority",
			paragraphs: []string{"明月几时有", "把酒问青天"},
			rhythmic:   "水调歌头",
			datasetKey: "songci",
			want: PoetryTypeInfo{
				TypeName: "宋词",
				Category: "宋词",
			},
		},
		// Edge case: dataset mapping should override structure analysis
		{
			name:       "诗经 with regular structure - dataset takes priority",
			paragraphs: []string{"关关雎鸠", "在河之洲", "窈窕淑女", "君子好逑"},
			rhythmic:   "",
			datasetKey: "shijing",
			want: PoetryTypeInfo{
				TypeName: "诗经",
				Category: "诗经",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClassifyPoetryTypeWithDataset(tt.paragraphs, tt.rhythmic, tt.datasetKey, "")

			assert.Equal(t, tt.want.TypeName, got.TypeName, "TypeName mismatch")
			assert.Equal(t, tt.want.Category, got.Category, "Category mismatch")

			if tt.want.Lines != nil {
				require.NotNil(t, got.Lines, "Lines should not be nil")
				assert.Equal(t, *tt.want.Lines, *got.Lines, "Lines mismatch")
			}

			if tt.want.CharsPerLine != nil {
				require.NotNil(t, got.CharsPerLine, "CharsPerLine should not be nil")
				assert.Equal(t, *tt.want.CharsPerLine, *got.CharsPerLine, "CharsPerLine mismatch")
			}
		})
	}
}
