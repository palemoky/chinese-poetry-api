package processor

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/palemoky/chinese-poetry-api/internal/database"
)

func ptr(v int64) *int64 { return &v }

func TestContentHashIgnoresPunctuationAndSplitting(t *testing.T) {
	a := contentHash([]string{"床前明月光，疑是地上霜。", "舉頭望明月，低頭思故鄉。"})
	b := contentHash([]string{"床前明月光疑是地上霜", "舉頭望明月 低頭思故鄉"})
	c := contentHash([]string{"床前明月光，疑是地上霜。舉頭望明月，低頭思故鄉。"})
	assert.Equal(t, a, b)
	assert.Equal(t, a, c)
	assert.NotEqual(t, a, contentHash([]string{"床前明月光，疑是地上雪。舉頭望明月，低頭思故鄉。"}))
}

func TestFirstClause(t *testing.T) {
	assert.Equal(t, "明月几时有", firstClause([]string{"明月几时有？把酒问青天。"}))
	assert.Equal(t, "大江东去", firstClause([]string{"", "大江东去，浪淘尽"}))
	assert.Equal(t, "无标点", firstClause([]string{"无标点"}))
	assert.Equal(t, "", firstClause(nil))
}

func TestFinalizePoemsDeduplicatesSameAuthorOnly(t *testing.T) {
	h := contentHash([]string{"同一首诗。"})
	poems := []*database.Poem{
		{ID: 3, Title: "乙", AuthorID: ptr(1), ContentHash: h}, // 同作者重复（标题不同），应去掉
		{ID: 1, Title: "甲", AuthorID: ptr(1), ContentHash: h}, // ID 最小，保留
		{ID: 2, Title: "甲", AuthorID: ptr(2), ContentHash: h}, // 不同作者：重出诗，保留
	}
	kept, removed := finalizePoems(poems)
	assert.Equal(t, 1, removed)
	if assert.Len(t, kept, 2) {
		assert.Equal(t, int64(1), kept[0].ID)
		assert.Equal(t, int64(2), kept[1].ID)
	}
}

func TestFinalizePoemsDisambiguatesCiTitles(t *testing.T) {
	poems := []*database.Poem{
		{ID: 1, Title: "水调歌头", AuthorID: ptr(8), ContentHash: "a", FirstLine: "明月几时有"},
		{ID: 2, Title: "水调歌头", AuthorID: ptr(8), ContentHash: "b", FirstLine: "落日绣帘卷"},
		{ID: 3, Title: "念奴娇", AuthorID: ptr(8), ContentHash: "c", FirstLine: "大江东去"},  // 只有一首，保持原样
		{ID: 4, Title: "水调歌头", AuthorID: ptr(9), ContentHash: "d", FirstLine: "江山如画"}, // 其他作者，只有一首
		{ID: 5, Title: "水调歌头·快哉亭作", AuthorID: ptr(8), ContentHash: "e"},               // 已有副标题，不参与
	}
	kept, _ := finalizePoems(poems)
	titles := map[int64]string{}
	for _, p := range kept {
		titles[p.ID] = p.Title
	}
	assert.Equal(t, "水调歌头·明月几时有", titles[1])
	assert.Equal(t, "水调歌头·落日绣帘卷", titles[2])
	assert.Equal(t, "念奴娇", titles[3])
	assert.Equal(t, "水调歌头", titles[4])
	assert.Equal(t, "水调歌头·快哉亭作", titles[5])
}

// 同一首诗同时收在唐、宋两部分时，作者按朝代分成两条记录、ID 不同，
// 但仍是同一作者的同一首诗，只保留先出现的一份。
func TestFinalizePoemsDeduplicatesAcrossDynastyRecords(t *testing.T) {
	h := contentHash([]string{"同一首诗。"})
	poems := []*database.Poem{
		{ID: 1, Title: "雪", AuthorID: ptr(10), AuthorName: "幸夤逊", ContentHash: h}, // 宋诗文件中的记录
		{ID: 2, Title: "雪", AuthorID: ptr(20), AuthorName: "幸夤逊", ContentHash: h}, // 唐诗文件中的记录
		{ID: 3, Title: "雪", AuthorID: ptr(30), AuthorName: "别人", ContentHash: h},  // 不同作者：保留
	}
	kept, removed := finalizePoems(poems)
	assert.Equal(t, 1, removed)
	if assert.Len(t, kept, 2) {
		assert.Equal(t, int64(1), kept[0].ID)
		assert.Equal(t, int64(3), kept[1].ID)
	}
}
