package database

import (
	"encoding/json"
	"math/rand/v2"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
)

func TestPostingsRoundTrip(t *testing.T) {
	ids := []int64{1, 2, 3, 130, 131, 70000, 1 << 40}
	var w postingWriter
	for _, id := range ids {
		w.add(id)
	}
	got, err := decodePostings(w.buf)
	require.NoError(t, err)
	assert.Equal(t, ids, got)

	got, err = decodePostings(nil)
	require.NoError(t, err)
	assert.Empty(t, got)

	_, err = decodePostings([]byte{0x80})
	assert.Error(t, err, "a truncated varint must not decode silently")
}

func TestSortedSetOps(t *testing.T) {
	a := []int64{1, 3, 5, 7}
	b := []int64{2, 3, 7, 8}
	assert.Equal(t, []int64{3, 7}, intersectSorted(a, b))
	assert.Equal(t, []int64{1, 2, 3, 5, 7, 8}, unionSorted(a, b))
	assert.Equal(t, a, unionSorted(a, nil))
	assert.Empty(t, intersectSorted(a, nil))
}

// charIndexCorpus 写入一批由小字表随机组成的诗词：字表很小，单字、双字查询
// 都会有大量命中，相邻与不相邻、仅标题与仅正文的情况都会出现。
func charIndexCorpus(t *testing.T, repo *Repository, n int) {
	t.Helper()
	alphabet := []rune("春风明月人间花落不知山水，。")
	rng := rand.New(rand.NewPCG(1, 2))
	text := func(k int) string {
		rs := make([]rune, k)
		for i := range rs {
			rs[i] = alphabet[rng.IntN(len(alphabet))]
		}
		return string(rs)
	}

	dynastyID, err := repo.GetOrCreateDynasty("唐")
	require.NoError(t, err)
	authors := make([]int64, 3)
	for i, name := range []string{"李白", "春风客", "明月"} {
		authors[i], err = repo.GetOrCreateAuthor(name, dynastyID)
		require.NoError(t, err)
	}

	for i := range n {
		content, err := json.Marshal([]string{text(5), text(5)})
		require.NoError(t, err)
		author := authors[rng.IntN(len(authors))]
		require.NoError(t, repo.InsertPoem(&Poem{
			// 数字后缀避免撞上 (title, content_hash, author_id) 唯一索引；ASCII 不进倒排索引，不影响查询
			ID: int64(i + 1), Title: text(3) + strconv.Itoa(i), Content: datatypes.JSON(content),
			AuthorID: &author, DynastyID: &dynastyID,
		}))
	}
}

type searchResult struct {
	ids   []int64
	total int64
}

func searchAll(t *testing.T, repo *Repository, query, searchType string) searchResult {
	t.Helper()
	var res searchResult
	for page := 1; ; page++ {
		poems, total, err := repo.SearchPoems(query, searchType, page, 7)
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

// TestCharIndexMatchesLikePath 是倒排索引的核心保证：对每个短查询，
// 走索引与走原有 LIKE 路径得到的结果（逐页内容与总数）必须完全一致。
func TestCharIndexMatchesLikePath(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRepository(db)
	charIndexCorpus(t, repo, 300)

	queries := []string{"春", "月", "不", "春风", "明月", "风春", "花落", "知山", "，", "春，", "雪", "雪花", "李", "客"}
	types := []string{"all", "title", "content"}

	// 先在索引失效的状态下取一遍（导入写入已让索引失效），作为基准
	ready, err := charIndexReady(db.DB, LangHans)
	require.NoError(t, err)
	require.False(t, ready, "inserts must invalidate the index")

	want := map[string]searchResult{}
	for _, q := range queries {
		for _, st := range types {
			want[st+"|"+q] = searchAll(t, repo, q, st)
		}
	}

	require.NoError(t, db.BuildCharIndex(LangHans))
	ready, err = charIndexReady(db.DB, LangHans)
	require.NoError(t, err)
	require.True(t, ready)

	for _, q := range queries {
		for _, st := range types {
			got := searchAll(t, repo, q, st)
			assert.Equal(t, want[st+"|"+q], got, "type=%s q=%q", st, q)
		}
	}

	// 确认上面确实走了索引，而不是两次都退回了 LIKE
	ids, ok, err := repo.shortTermMatches("春风", "all")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, want["all|春风"].ids, ids)
	assert.NotEmpty(t, ids)
}

func TestCharIndexRandomByChar(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRepository(db)
	charIndexCorpus(t, repo, 100)
	require.NoError(t, db.BuildCharIndex(LangHans))

	expected, ok, err := repo.shortTermMatches("春", "content")
	require.NoError(t, err)
	require.True(t, ok)
	allowed := map[int64]bool{}
	for _, id := range expected {
		allowed[id] = true
	}

	for range 30 {
		poem, err := repo.GetRandomPoemByChar("春")
		require.NoError(t, err)
		assert.True(t, allowed[poem.ID], "poem %d does not contain 春 in its content", poem.ID)
		require.NotNil(t, poem.Author, "relations must be loaded")
	}

	_, err = repo.GetRandomPoemByChar("雪")
	assert.Error(t, err)
}

// 建好索引后再写入，索引必须立刻失效，查询退回 LIKE 路径并看到新数据。
func TestCharIndexInvalidatedByWrites(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRepository(db)
	charIndexCorpus(t, repo, 20)
	require.NoError(t, db.BuildCharIndex(LangHans))

	_, before, err := repo.SearchPoems("雪", "content", 1, 10)
	require.NoError(t, err)
	assert.Zero(t, before)

	authorID := int64(1)
	require.NoError(t, repo.InsertPoem(&Poem{
		ID: 999, Title: "雪", Content: datatypes.JSON([]byte(`["白雪"]`)), AuthorID: &authorID,
	}))

	ready, err := charIndexReady(db.DB, LangHans)
	require.NoError(t, err)
	assert.False(t, ready)

	poems, total, err := repo.SearchPoems("雪", "content", 1, 10)
	require.NoError(t, err)
	assert.EqualValues(t, 1, total)
	require.Len(t, poems, 1)
	assert.EqualValues(t, 999, poems[0].ID)
}

// 版本不符的旧索引视为无效：indexableRune 等逻辑变化后旧数据不可再用。
func TestCharIndexVersionMismatchIsNotReady(t *testing.T) {
	db := setupTestDB(t)
	require.NoError(t, db.BuildCharIndex(LangHans))
	require.NoError(t, db.Exec("UPDATE "+charIndexStateTable+" SET version = version + 1").Error)

	ready, err := charIndexReady(db.DB, LangHans)
	require.NoError(t, err)
	assert.False(t, ready)
}

// referenceAllMode 是 "all" 模式改写前的语义：一条 SQL，标题、正文、作者名三者 OR。
func referenceAllMode(t *testing.T, repo *Repository, query string) []int64 {
	t.Helper()
	pattern := "%" + query + "%"
	var ids []int64
	require.NoError(t, repo.db.Raw(`SELECT p.id FROM poems_zh_hans p
		JOIN poems_fts_zh_hans f ON f.rowid = p.id
		LEFT JOIN authors_zh_hans a ON p.author_id = a.id
		WHERE f.title LIKE ? OR f.content_text LIKE ? OR a.name LIKE ?
		ORDER BY p.id`, pattern, pattern, pattern).Scan(&ids).Error)
	return ids
}

// "all" 模式拆成三条查询再合并后，结果必须与原先那条 OR 查询一致，
// 无论走不走倒排索引。
func TestAllModeMatchesSingleQuerySemantics(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRepository(db)
	charIndexCorpus(t, repo, 300)

	// 3 字以上走拆分后的 FTS 查询；标点走同一路径；"春风客"/"明月" 同时命中作者名
	queries := []string{"春风明", "明月人", "，春", "。", "春风客", "明月", "李白", "不存在的"}

	check := func(phase string) {
		for _, q := range queries {
			want := referenceAllMode(t, repo, q)
			got := searchAll(t, repo, q, "all")
			assert.Equal(t, want, got.ids, "%s: q=%q", phase, q)
			assert.EqualValues(t, len(want), got.total, "%s: q=%q", phase, q)
		}
	}

	check("without index")
	require.NoError(t, db.BuildCharIndex(LangHans))
	check("with index")
}
