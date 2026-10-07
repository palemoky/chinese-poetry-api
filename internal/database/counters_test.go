package database

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
)

// insertCountablePoem 写入一首用于计数的诗词。
func insertCountablePoem(t *testing.T, repo *Repository, id int64, authorID, dynastyID int64) {
	t.Helper()
	require.NoError(t, repo.InsertPoem(&Poem{
		ID:        id,
		Title:     "诗" + string(rune('A'+id)),
		Content:   datatypes.JSON([]byte(`["内容"]`)),
		AuthorID:  &authorID,
		DynastyID: &dynastyID,
	}))
}

// TestPoemCounterStaysInSync 覆盖物化的诗词总数。
// 它是唯一一个代价随表大小增长的计数（COUNT(*) 要扫完整棵索引），
// 因此改为由触发器维护的计数器——代价是它必须始终与实际行数一致。
func TestPoemCounterStaysInSync(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRepository(db)

	dynastyID, err := repo.GetOrCreateDynasty("唐")
	require.NoError(t, err)
	authorID, err := repo.GetOrCreateAuthor("李白", dynastyID)
	require.NoError(t, err)

	count, err := repo.CountPoems()
	require.NoError(t, err)
	assert.Equal(t, 0, count, "an empty corpus must count as zero, not as a missing counter row")

	for i := range 3 {
		insertCountablePoem(t, repo, int64(i+1), authorID, dynastyID)
	}

	count, err = repo.CountPoems()
	require.NoError(t, err)
	assert.Equal(t, 3, count)

	// 删除同样要反映到计数器上
	require.NoError(t, db.Exec("DELETE FROM "+repo.poemsTable()+" WHERE id = ?", 2).Error)

	count, err = repo.CountPoems()
	require.NoError(t, err)
	assert.Equal(t, 2, count)

	// 与实际行数逐一核对，确认触发器没有漏计
	var actual int64
	require.NoError(t, db.Table(repo.poemsTable()).Count(&actual).Error)
	assert.Equal(t, int(actual), count, "the counter must match COUNT(*) exactly")
}

// TestPoemCounterIsPerLang 覆盖简繁两套表各自独立计数。
func TestPoemCounterIsPerLang(t *testing.T) {
	db := setupTestDB(t)
	hans := NewRepositoryWithLang(db, LangHans)
	hant := NewRepositoryWithLang(db, LangHant)

	dynastyID, err := hans.GetOrCreateDynasty("唐")
	require.NoError(t, err)
	authorID, err := hans.GetOrCreateAuthor("李白", dynastyID)
	require.NoError(t, err)
	insertCountablePoem(t, hans, 1, authorID, dynastyID)

	count, err := hans.CountPoems()
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	count, err = hant.CountPoems()
	require.NoError(t, err)
	assert.Equal(t, 0, count, "writing to the simplified table must not move the traditional counter")
}

// TestRefreshPoemCounterRecomputes 覆盖迁移时的回填路径：
// 早于计数器的库里没有触发器可依，取值必须靠一次全量重算补上。
func TestRefreshPoemCounterRecomputes(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRepository(db)

	dynastyID, err := repo.GetOrCreateDynasty("唐")
	require.NoError(t, err)
	authorID, err := repo.GetOrCreateAuthor("李白", dynastyID)
	require.NoError(t, err)
	insertCountablePoem(t, repo, 1, authorID, dynastyID)

	// 手工把计数器改错，模拟触发器尚不存在时写入的数据
	require.NoError(t, db.Exec("UPDATE "+countersTable+" SET value = 999 WHERE name = ?",
		poemCounterName(LangHans)).Error)

	require.NoError(t, db.RefreshPoemCounter(LangHans))

	count, err := repo.CountPoems()
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}

// TestCountPoemsByAuthorUsesMaterializedColumn 确认按作者计数读的是物化列，
// 且该列随写入保持同步——GraphQL 的 Author.poemCount 是逐行解析的，
// 一页作者会调用它很多次。
func TestCountPoemsByAuthorUsesMaterializedColumn(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRepository(db)

	dynastyID, err := repo.GetOrCreateDynasty("唐")
	require.NoError(t, err)
	libai, err := repo.GetOrCreateAuthor("李白", dynastyID)
	require.NoError(t, err)
	dufu, err := repo.GetOrCreateAuthor("杜甫", dynastyID)
	require.NoError(t, err)

	insertCountablePoem(t, repo, 1, libai, dynastyID)
	insertCountablePoem(t, repo, 2, libai, dynastyID)
	insertCountablePoem(t, repo, 3, dufu, dynastyID)

	count, err := repo.CountPoemsByAuthor(libai)
	require.NoError(t, err)
	assert.Equal(t, 2, count)

	count, err = repo.CountPoemsByAuthor(dufu)
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	// 不存在的作者返回 0 而不是报错
	count, err = repo.CountPoemsByAuthor(999999)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}
