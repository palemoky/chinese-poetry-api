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
		{
			name:       "词 with rhythmic field",
			paragraphs: []string{"明月几时有", "把酒问青天"},
			rhythmic:   "水调歌头",
			want: PoetryTypeInfo{
				TypeName: "词",
				Category: "词",
			},
		},
		{
			name:       "五言绝句",
			paragraphs: []string{"春眠不觉晓", "处处闻啼鸟", "夜来风雨声", "花落知多少"},
			rhythmic:   "",
			want: PoetryTypeInfo{
				TypeName:     "五言绝句",
				Category:     "诗",
				Lines:        intPtr(4),
				CharsPerLine: intPtr(5),
			},
		},
		{
			name:       "七言绝句",
			paragraphs: []string{"两个黄鹂鸣翠柳", "一行白鹭上青天", "窗含西岭千秋雪", "门泊东吴万里船"},
			rhythmic:   "",
			want: PoetryTypeInfo{
				TypeName:     "七言绝句",
				Category:     "诗",
				Lines:        intPtr(4),
				CharsPerLine: intPtr(7),
			},
		},
		{
			name:       "五言律诗",
			paragraphs: []string{"空山新雨后", "天气晚来秋", "明月松间照", "清泉石上流", "竹喧归浣女", "莲动下渔舟", "随意春芳歇", "王孙自可留"},
			rhythmic:   "",
			want: PoetryTypeInfo{
				TypeName:     "五言律诗",
				Category:     "诗",
				Lines:        intPtr(8),
				CharsPerLine: intPtr(5),
			},
		},
		{
			name:       "七言绝句 (4 lines)",
			paragraphs: []string{"岐王宅里寻常见", "崔九堂前几度闻", "正是江南好风景", "落花时节又逢君"},
			rhythmic:   "",
			want: PoetryTypeInfo{
				TypeName:     "七言绝句",
				Category:     "诗",
				Lines:        intPtr(4),
				CharsPerLine: intPtr(7),
			},
		},
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
			name:       "irregular format",
			paragraphs: []string{"短", "这是一首很长的句子超过十个字"},
			rhythmic:   "",
			want: PoetryTypeInfo{
				TypeName: "其他",
				Category: "其他",
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

func TestGenerateStableAuthorID(t *testing.T) {
	tests := []struct {
		name       string
		authorName string
		wantSame   string // Another name that should generate the same ID
		wantDiff   string // Another name that should generate different ID
	}{
		{"same name generates same ID", "李白", "李白", "杜甫"},
		{"different names generate different IDs", "白居易", "白居易", "李商隐"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id1 := GenerateStableAuthorID(tt.authorName)
			id2 := GenerateStableAuthorID(tt.wantSame)
			id3 := GenerateStableAuthorID(tt.wantDiff)

			// Same name should generate same ID
			assert.Equal(t, id1, id2, "Same name should generate same ID")

			// Different name should generate different ID
			assert.NotEqual(t, id1, id3, "Different names should generate different IDs")

			// ID should be non-zero
			assert.NotZero(t, id1, "ID should not be zero")
		})
	}
}
