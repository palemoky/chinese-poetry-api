package database

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// searchAllInDynasty 逐页取完 SearchPoemsInDynasty 的结果。
func searchAllInDynasty(t *testing.T, repo *Repository, query, searchType string, dynastyID *int64) searchResult {
	t.Helper()
	var res searchResult
	for page := 1; ; page++ {
		poems, total, err := repo.SearchPoemsInDynasty(query, searchType, dynastyID, page, 7)
		require.NoError(t, err)
		res.total = total
		for _, p := range poems {
			res.ids = append(res.ids, p.ID)
		}
		if len(poems) < 7 {
			return res
		}
	}
}

// 按朝代筛选的搜索结果，必须恰好是不筛选时的结果去掉其他朝代、顺序不变（#65）。
// 倒排索引与 LIKE 两条路径都要满足，长查询只搜正文时走的是单独的 SQL 分页。
func TestSearchPoemsInDynastyKeepsOrder(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRepository(db)
	charIndexCorpus(t, repo, 300)

	songID, err := repo.GetOrCreateDynasty("宋")
	require.NoError(t, err)
	tangID, err := repo.GetOrCreateDynasty("唐")
	require.NoError(t, err)
	require.NoError(t, db.Table(repo.poemsTable()).Where("id % 3 = 0").Update("dynasty_id", songID).Error)

	dynastyOf := map[int64]int64{}
	var rows []Poem
	require.NoError(t, db.Table(repo.poemsTable()).Select("id", "dynasty_id").Find(&rows).Error)
	for _, p := range rows {
		dynastyOf[p.ID] = *p.DynastyID
	}

	queries := []string{"春", "明月", "春风明", "花落不知", "雪"}
	types := []string{"all", "title", "content", "author"}

	check := func(t *testing.T) {
		for _, q := range queries {
			for _, st := range types {
				all := searchAll(t, repo, q, st)
				for _, d := range []int64{tangID, songID} {
					var want []int64
					for _, id := range all.ids {
						if dynastyOf[id] == d {
							want = append(want, id)
						}
					}
					got := searchAllInDynasty(t, repo, q, st, &d)
					assert.Equal(t, want, got.ids, "type=%s q=%q dynasty=%d", st, q, d)
					assert.Equal(t, int64(len(want)), got.total, "type=%s q=%q dynasty=%d", st, q, d)
				}
				assert.Equal(t, all, searchAllInDynasty(t, repo, q, st, nil), "nil dynasty must not filter")
			}
		}
	}

	t.Run("LIKE path", check)
	require.NoError(t, db.BuildCharIndex(LangHans))
	t.Run("char index path", check)

	t.Run("a dynasty with no poems", func(t *testing.T) {
		yuanID, err := repo.GetOrCreateDynasty("元")
		require.NoError(t, err)
		got := searchAllInDynasty(t, repo, "春", "all", &yuanID)
		assert.Empty(t, got.ids)
		assert.Zero(t, got.total)
	})
}
