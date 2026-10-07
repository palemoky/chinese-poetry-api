package database

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 作者以名字 + 朝代区分：唐代张潮与清代张潮是两个人。
func TestAuthorsAreDistinguishedByDynasty(t *testing.T) {
	repo := NewRepository(setupTestDB(t))
	tang, err := repo.GetOrCreateDynasty("唐")
	require.NoError(t, err)
	qing, err := repo.GetOrCreateDynasty("清")
	require.NoError(t, err)

	tangID, err := repo.GetOrCreateAuthor("张潮", tang)
	require.NoError(t, err)
	qingID, err := repo.GetOrCreateAuthor("张潮", qing)
	require.NoError(t, err)
	assert.NotEqual(t, tangID, qingID)

	again, err := repo.GetOrCreateAuthor("张潮", qing)
	require.NoError(t, err)
	assert.Equal(t, qingID, again, "same name and dynasty is the same author")

	id, err := repo.CreateAuthorWithID(100, "张潮", tang)
	require.NoError(t, err)
	assert.Equal(t, tangID, id, "an existing name+dynasty keeps its ID")

	// 清代张潮作品更多：不指定朝代时取作品最多者，指定朝代时取该朝代的那位
	require.NoError(t, repo.db.Exec("UPDATE "+repo.authorsTable()+" SET poem_count = 7 WHERE id = ?", qingID).Error)
	got, err := repo.GetAuthorByName("张潮", nil)
	require.NoError(t, err)
	assert.Equal(t, qingID, got.ID)
	got, err = repo.GetAuthorByName("张潮", &tang)
	require.NoError(t, err)
	assert.Equal(t, tangID, got.ID)
	require.NotNil(t, got.Dynasty)
	assert.Equal(t, "唐", got.Dynasty.Name)
}

func TestSetAuthorDescriptions(t *testing.T) {
	repo := NewRepository(setupTestDB(t))
	dynasty, err := repo.GetOrCreateDynasty("宋")
	require.NoError(t, err)
	id, err := repo.GetOrCreateAuthor("苏轼", dynasty)
	require.NoError(t, err)

	require.NoError(t, repo.SetAuthorDescriptions(map[int64]string{id: "苏轼，字子瞻。"}))
	author, err := repo.GetAuthorByID(id)
	require.NoError(t, err)
	require.NotNil(t, author.Description)
	assert.Equal(t, "苏轼，字子瞻。", *author.Description)
}

// 老库的作者表在 name 列上带 UNIQUE。迁移要能在老库上补建 (name, dynasty_id) 索引，
// 新的写入路径（ON CONFLICT (name, dynasty_id)）在老库上也要能用。
func TestAuthorNameDynastyIndexOnLegacySchema(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "legacy.db"), 1, 1)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	require.NoError(t, db.Exec(`CREATE TABLE authors_zh_hans (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL UNIQUE,
		dynasty_id INTEGER,
		description TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`).Error)
	require.NoError(t, db.Migrate())

	repo := NewRepository(db)
	dynasty, err := repo.GetOrCreateDynasty("唐")
	require.NoError(t, err)
	first, err := repo.GetOrCreateAuthor("李白", dynasty)
	require.NoError(t, err)
	second, err := repo.GetOrCreateAuthor("李白", dynasty)
	require.NoError(t, err)
	assert.Equal(t, first, second)
}
