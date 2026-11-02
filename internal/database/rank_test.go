package database

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
)

// rankCorpus 写入一组覆盖各种得分的诗词，注释里是搜“明月”时的得分。
func rankCorpus(t *testing.T, repo *Repository) {
	t.Helper()
	dynastyID, err := repo.GetOrCreateDynasty("唐")
	require.NoError(t, err)
	author := func(name string) *int64 {
		id, err := repo.GetOrCreateAuthor(name, dynastyID)
		require.NoError(t, err)
		return &id
	}
	libai, moon, moonGuest := author("李白"), author("明月"), author("明月客")

	poems := []struct {
		id      int64
		title   string
		content []string
		author  *int64
	}{
		{1, "月下独酌", []string{"花间一壶酒", "明月夜独酌"}, libai}, // 正文 1
		{2, "明月", []string{"白云"}, libai},               // 标题完全相同 4
		{3, "春江花月夜", []string{"海上明月共潮生"}, libai},       // 正文 1
		{4, "明月·其一", []string{"清风"}, libai},            // 词牌与查询相同 4
		{5, "江上", []string{"绿水"}, moonGuest},           // 作者名包含 1
		{6, "山中", []string{"青山"}, moon},                // 作者名相同 2
		{7, "明月夜", []string{"明月照人"}, libai},            // 标题包含 2 + 正文 1
		{8, "关山明月・其二", []string{"边草"}, libai},          // 标题包含 2：分隔符前是“关山明月”
		{9, "明月・仙吕", []string{"高楼"}, libai},            // 曲的分隔符，标题完全相同 4
		{10, "moonlight", []string{"夜色"}, libai},
		{11, "MOON", []string{"夜深"}, libai},
	}
	for _, p := range poems {
		content, err := json.Marshal(p.content)
		require.NoError(t, err)
		require.NoError(t, repo.InsertPoem(&Poem{
			ID: p.id, Title: p.title, Content: datatypes.JSON(content), AuthorID: p.author, DynastyID: &dynastyID,
		}))
	}
}

func TestSearchRanksByRelevance(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRepository(db)
	rankCorpus(t, repo)

	cases := []struct {
		query, searchType string
		want              []int64
	}{
		// 同分按 ID 升序：4 分 2、4、9，3 分 7，2 分 6、8，1 分 1、3、5
		{"明月", "all", []int64{2, 4, 9, 7, 6, 8, 1, 3, 5}},
		{"明月", "title", []int64{2, 4, 9, 7, 8}},
		{"明月", "author", []int64{6, 5}},
		// 只搜正文时全部同分，保持 ID 顺序
		{"明月", "content", []int64{1, 3, 7}},
		// 3 字查询走 FTS trigram 而不是倒排索引
		{"明月夜", "all", []int64{7, 1}},
		// 与 LIKE 一致，完全相同的比较不区分 ASCII 大小写
		{"moon", "title", []int64{11, 10}},
	}

	check := func(phase string) {
		for _, c := range cases {
			got := searchAll(t, repo, c.query, c.searchType)
			assert.Equal(t, c.want, got.ids, "%s: type=%s q=%q", phase, c.searchType, c.query)
			assert.EqualValues(t, len(c.want), got.total, "%s: type=%s q=%q", phase, c.searchType, c.query)
		}
	}

	check("without index")
	require.NoError(t, db.BuildCharIndex(LangHans))
	_, ok, err := repo.shortTermMatches("明月", "all")
	require.NoError(t, err)
	require.True(t, ok, "the second pass must use the char index")
	check("with index")
}

// 翻页必须按排好的顺序切片，而不是在每页内部重新按 ID 排序。
func TestSearchPagesFollowRanking(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRepository(db)
	rankCorpus(t, repo)

	var got []int64
	for page := 1; page <= 5; page++ {
		poems, total, err := repo.SearchPoems("明月", "all", page, 2)
		require.NoError(t, err)
		assert.EqualValues(t, 9, total)
		for _, p := range poems {
			got = append(got, p.ID)
			require.NotNil(t, p.Author, "relations must be loaded")
		}
	}
	assert.Equal(t, []int64{2, 4, 9, 7, 6, 8, 1, 3, 5}, got)
}

func TestRankByScoreIsStable(t *testing.T) {
	scores := map[int64]int{1: 1, 2: 3, 3: 1, 4: 3, 5: 0}
	got := rankByScore([]int64{1, 2, 3, 4, 5}, func(id int64) int { return scores[id] })
	assert.Equal(t, []int64{2, 4, 1, 3, 5}, got)
}
