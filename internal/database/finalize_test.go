package database

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
)

func indexNames(t *testing.T, db *DB, table string) []string {
	t.Helper()
	var names []string
	require.NoError(t, db.Raw("SELECT name FROM sqlite_master WHERE type = 'index' AND tbl_name = ? AND sql IS NOT NULL", table).
		Scan(&names).Error)
	return names
}

func columnNames(t *testing.T, db *DB, table string) []string {
	t.Helper()
	var names []string
	require.NoError(t, db.Raw("SELECT name FROM pragma_table_info(?)", table).Scan(&names).Error)
	return names
}

func TestMigrateDropsRedundantPoemIndexes(t *testing.T) {
	db := setupTestDB(t)
	table := PoemsTable(LangHans)
	require.NoError(t, db.Exec("CREATE INDEX idx_"+table+"_type ON "+table+"(type_id)").Error)
	require.NoError(t, db.Exec("CREATE INDEX idx_"+table+"_title ON "+table+"(title)").Error)

	require.NoError(t, db.migrateTablesForLang(LangHans))

	names := indexNames(t, db, table)
	assert.NotContains(t, names, "idx_"+table+"_type")
	assert.NotContains(t, names, "idx_"+table+"_title")
	assert.Contains(t, names, "idx_"+table+"_type_id")
	assert.Contains(t, names, "idx_"+table+"_unique_author", "an unfinalized database still needs it for imports")
}

func TestFinalizeImport(t *testing.T) {
	path := buildReleasedDB(t)

	db, err := Open(path, 1, 1)
	require.NoError(t, err)
	require.NoError(t, db.FinalizeImport())
	// 收尾后服务启动时还会再迁移一次，不能把删掉的东西重建回来
	require.NoError(t, db.Migrate())

	for _, lang := range []Lang{LangHans, LangHant} {
		table := PoemsTable(lang)
		assert.NotContains(t, indexNames(t, db, table), "idx_"+table+"_unique_author", lang)
		assert.NotContains(t, columnNames(t, db, table), "content_hash", lang)
	}
	require.NoError(t, db.Close())

	served, err := OpenForServing(path, 2, 1)
	require.NoError(t, err)
	t.Cleanup(func() { _ = served.Close() })
	repo := NewRepository(served)

	poem, err := repo.GetPoemByID("1")
	require.NoError(t, err)
	assert.Equal(t, "静夜思", poem.Title)
	assert.Equal(t, datatypes.JSON(`["床前明月光"]`), poem.Content)

	poems, total, err := repo.ListPoemsWithFilter(10, 0, nil, nil, nil)
	require.NoError(t, err)
	assert.Len(t, poems, 1)
	assert.Equal(t, 1, total)

	for _, q := range []string{"明月", "床前明月"} {
		found, n, err := repo.SearchPoems(q, "all", 1, 10)
		require.NoError(t, err, q)
		assert.Len(t, found, 1, q)
		assert.Equal(t, int64(1), n, q)
	}
}
