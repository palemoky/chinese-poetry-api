package processor

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/palemoky/chinese-poetry-api/internal/database"
	"github.com/palemoky/chinese-poetry-api/internal/loader"
)

func TestMatchLines(t *testing.T) {
	got := matchLines([]string{"床前明月光，疑是地上霜。", "举头望明月，低头思故乡。床前明月光！", "春眠□不觉晓", "短句。abc def"})
	// 重复的句子、不足四个汉字的句子（□ 也是断句处）与非汉字都去掉
	assert.Equal(t, []string{"床前明月光", "疑是地上霜", "举头望明月", "低头思故乡"}, got)
}

func poemWithLines(id int64, lines ...string) *database.Poem {
	return &database.Poem{ID: id, SimplifiedContent: lines}
}

func TestMarkAnthologies(t *testing.T) {
	anthology := anthologyEntry{bit: database.AnthologyTangShi300, lines: matchLines([]string{
		"白日依山尽，黄河入海流。", "欲穷千里目，更上一层楼。",
	})}

	poems := []*database.Poem{
		// 全集里的版本有一处异文，其余三句相同
		poemWithLines(1, "白日依山尽，黄河入海流。", "欲穷千里目，更上一重楼。"),
		// 同一首诗挂在另一位作者名下的另一个版本，也要标记
		poemWithLines(2, "白日依山尽，黄河入海流。", "欲穷千里目，更上一层楼。"),
		// 只借用了一句
		poemWithLines(3, "白日依山尽，青山不见人。"),
		poemWithLines(4, "春眠不觉晓，处处闻啼鸟。"),
	}

	matched := markAnthologies(poems, []anthologyEntry{anthology})

	assert.Equal(t, map[int]int{database.AnthologyTangShi300: 1}, matched)
	assert.Equal(t, database.AnthologyTangShi300, poems[0].Anthology)
	assert.Equal(t, database.AnthologyTangShi300, poems[1].Anthology)
	assert.Zero(t, poems[2].Anthology, "one shared line is not enough")
	assert.Zero(t, poems[3].Anthology)
}

func TestMarkAnthologiesIgnoresCommonLines(t *testing.T) {
	// 两句都是套语：各自出现在 maxLineOwners 首以上的诗里，不能据此判断
	common := []string{"一年三百六十日", "风刀霜剑严相逼"}
	var poems []*database.Poem
	for i := range maxLineOwners + 1 {
		poems = append(poems, poemWithLines(int64(i+1), common[0]+"，"+common[1]+"。", fmt.Sprintf("第%d首另有句子", i)))
	}

	matched := markAnthologies(poems, []anthologyEntry{{bit: database.AnthologyQianJiaShi, lines: matchLines(common)}})
	assert.Empty(t, matched)
	for _, p := range poems {
		assert.Zero(t, p.Anthology)
	}
}

func TestMarkAnthologiesCombinesBits(t *testing.T) {
	lines := []string{"春眠不觉晓，处处闻啼鸟。", "夜来风雨声，花落知多少。"}
	poems := []*database.Poem{poemWithLines(1, lines...)}
	markAnthologies(poems, []anthologyEntry{
		{bit: database.AnthologyTangShi300, lines: matchLines(lines)},
		{bit: database.AnthologyQianJiaShi, lines: matchLines(lines)},
	})
	assert.Equal(t, database.AnthologyTangShi300|database.AnthologyQianJiaShi, poems[0].Anthology)
}

func TestSetAnthologiesConvertsToSimplified(t *testing.T) {
	p := &Processor{}
	require.NoError(t, p.SetAnthologies([]loader.AnthologyPoem{
		{Source: "tangshi300", Paragraphs: []string{"白日依山盡，黃河入海流。"}},
	}))
	require.Len(t, p.anthologies, 1)
	assert.Equal(t, []string{"白日依山尽", "黄河入海流"}, p.anthologies[0].lines)

	assert.Error(t, p.SetAnthologies([]loader.AnthologyPoem{{Source: "unknown", Paragraphs: []string{"一二三四"}}}))
}
