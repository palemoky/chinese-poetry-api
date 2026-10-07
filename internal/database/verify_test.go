package database

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
)

// buildBothVariants 建一个简繁两表内容对应、已收尾的库，即一个合格的发布库。
func buildBothVariants(t *testing.T) *DB {
	t.Helper()
	db := setupFullTestDB(t)
	for _, lang := range []Lang{LangHans, LangHant} {
		repo := NewRepositoryWithLang(db, lang)
		dynastyID, err := repo.GetOrCreateDynasty("唐")
		require.NoError(t, err)
		authorID, err := repo.CreateAuthorWithID(1, "李白", dynastyID)
		require.NoError(t, err)
		require.NoError(t, repo.InsertPoem(&Poem{
			ID: 1, Title: "静夜思", Content: datatypes.JSON(`["床前明月光"]`),
			AuthorID: &authorID, DynastyID: &dynastyID, TypeID: ptr(int64(12)),
		}))
	}
	require.NoError(t, db.BuildCharIndexes())
	require.NoError(t, db.FinalizeImport())
	return db
}

func failedChecks(r *VerifyReport) []string {
	var names []string
	for _, c := range r.Checks {
		if !c.Passed {
			names = append(names, c.Name)
		}
	}
	return names
}

func TestVerifyReleasePasses(t *testing.T) {
	db := buildBothVariants(t)

	report, err := db.VerifyRelease(1)
	require.NoError(t, err)
	assert.Empty(t, failedChecks(report))
	assert.True(t, report.Passed())
	assert.Equal(t, []DynastyCount{{Name: "唐", Count: 1}}, report.Dynasties)
}

func TestVerifyReleaseCatchesProblems(t *testing.T) {
	t.Run("too few poems", func(t *testing.T) {
		report, err := buildBothVariants(t).VerifyRelease(2)
		require.NoError(t, err)
		assert.Equal(t, []string{"poem count"}, failedChecks(report))
	})

	t.Run("variants out of step", func(t *testing.T) {
		// 繁体表里同一首诗挂到了另一位作者名下，正是简繁作者 ID 错位时的样子
		db := buildBothVariants(t)
		hant := NewRepositoryWithLang(db, LangHant)
		dynastyID, err := hant.GetOrCreateDynasty("唐")
		require.NoError(t, err)
		_, err = hant.CreateAuthorWithID(2, "李白二", dynastyID)
		require.NoError(t, err)
		require.NoError(t, db.Exec("UPDATE "+PoemsTable(LangHant)+" SET author_id = 2").Error)

		report, err := db.VerifyRelease(1)
		require.NoError(t, err)
		// 原作者 1 在繁体表里没了作品，也会被查出来
		assert.Equal(t, []string{"poem relations aligned", "author ids aligned", "zh-Hant author poem counts"}, failedChecks(report))
		assert.False(t, report.Passed())
	})

	t.Run("unfinished import", func(t *testing.T) {
		db := setupFullTestDB(t)
		report, err := db.VerifyRelease(0)
		require.NoError(t, err)
		assert.Equal(t, []string{"import finalized"}, failedChecks(report))
	})
}

// setupFullTestDB 建一个跑过完整 Migrate 的文件库（单连接，避免 :memory: 每条连接各是一个库）。
func setupFullTestDB(t *testing.T) *DB {
	t.Helper()
	db, err := Open(filepath.Join(t.TempDir(), "poetry.db"), 1, 1)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, db.Migrate())
	return db
}

func ptr[T any](v T) *T { return &v }
