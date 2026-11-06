package classifier

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToTraditional(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"simple conversion", "中国", "中國"},
		{"poetry text", "春眠不觉晓", "春眠不覺曉"},
		{"already traditional", "詩詞", "詩詞"},
		{"empty string", "", ""},
		{"mixed text", "李白的诗歌很优美", "李白的詩歌很優美"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ToTraditional(tt.input)
			require.NoError(t, err, "ToTraditional should not error")
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestToSimplified(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"simple conversion", "中國", "中国"},
		{"poetry text", "春眠不覺曉", "春眠不觉晓"},
		{"already simplified", "诗词", "诗词"},
		{"empty string", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ToSimplified(tt.input)
			require.NoError(t, err, "ToSimplified should not error")
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestToTraditionalArray(t *testing.T) {
	tests := []struct {
		name  string
		input []string
		want  []string
	}{
		{"convert array", []string{"中国", "诗歌"}, []string{"中國", "詩歌"}},
		{"empty array", []string{}, []string{}},
		{"single element", []string{"春眠不觉晓"}, []string{"春眠不覺曉"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ToTraditionalArray(tt.input)
			require.NoError(t, err, "ToTraditionalArray should not error")
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestToSimplifiedArray(t *testing.T) {
	tests := []struct {
		name  string
		input []string
		want  []string
	}{
		{
			name:  "convert array",
			input: []string{"中國", "詩歌"},
			want:  []string{"中国", "诗歌"},
		},
		{
			name:  "empty array",
			input: []string{},
			want:  []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ToSimplifiedArray(tt.input)
			require.NoError(t, err, "ToSimplifiedArray should not error")
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestIsTraditional(t *testing.T) {
	tests := []struct {
		name string
		text string
		want bool
	}{
		{"traditional poem", "國破山河在，城春草木深。感時花濺淚，恨別鳥驚心。", true},
		{"simplified poem", "国破山河在，城春草木深。感时花溅泪，恨别鸟惊心。", false},
		// 「游」「里」「云」在繁转简时不变，简转繁时却会被改掉；不能因此把简体判成繁体
		{"simplified with ambiguous chars", "陆游行十里路，白云深处有人家。", false},
		{"traditional with ambiguous chars", "陸游行十里路，白雲深處有人家。", true},
		// 简繁同形、两边改动一样多时视为简体
		{"no difference", "山水", false},
		{"empty", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := IsTraditional(tt.text)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRuneDiff(t *testing.T) {
	assert.Equal(t, 0, runeDiff("", ""))
	assert.Equal(t, 0, runeDiff("静夜思", "静夜思"))
	assert.Equal(t, 3, runeDiff("感时花溅泪", "感時花濺淚"))
	assert.Equal(t, 1, runeDiff("春眠", "春睡"))
	// 长度不同时只比较重叠部分
	assert.Equal(t, 0, runeDiff("春眠不觉晓", "春眠"))
	assert.Equal(t, 1, runeDiff("春", "夏眠不觉晓"))
}
