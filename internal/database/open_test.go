package database

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
)

// buildReleasedDB 用导入路径建一个 WAL 模式的数据库文件，并像发布流程那样
// 只留下 .db 本体（gzip 打包时不会带上 -wal/-shm）。
func buildReleasedDB(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "poetry.db")

	db, err := Open(path, 1, 1)
	require.NoError(t, err)
	require.NoError(t, db.Migrate())

	repo := NewRepository(db)
	dynastyID, err := repo.GetOrCreateDynasty("唐")
	require.NoError(t, err)
	authorID, err := repo.GetOrCreateAuthor("李白", dynastyID)
	require.NoError(t, err)
	require.NoError(t, repo.InsertPoem(&Poem{
		ID: 1, Title: "静夜思", Content: datatypes.JSON([]byte(`["床前明月光"]`)),
		AuthorID: &authorID, DynastyID: &dynastyID,
	}))
	require.NoError(t, db.Close())

	for _, suffix := range []string{"-wal", "-shm"} {
		_ = os.Remove(path + suffix)
	}
	return path
}

func TestOpenReadOnlyServesReleasedDB(t *testing.T) {
	path := buildReleasedDB(t)

	db, err := OpenReadOnly(path, 4, 2)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	repo := NewRepository(db)

	// 多条连接并发读：去掉 cache=shared 后读者之间不应互相报 SQLITE_LOCKED
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 20 {
				poems, total, err := repo.SearchPoems("明月", "all", 1, 10)
				assert.NoError(t, err)
				assert.Len(t, poems, 1)
				assert.Equal(t, int64(1), total)
			}
		})
	}
	wg.Wait()

	poem, err := repo.GetRandomPoem(nil, nil, nil)
	require.NoError(t, err)
	assert.Equal(t, "李白", poem.Author.Name)
}

func TestOpenReadOnlyRejectsWrites(t *testing.T) {
	db, err := OpenReadOnly(buildReleasedDB(t), 1, 1)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	_, err = NewRepository(db).GetOrCreateDynasty("宋")
	assert.Error(t, err, "the API server must not be able to modify the corpus")

	// 确认是 mode=ro 本身生效，而不只是 _query_only 拦住了写入：
	// 关掉 _query_only 之后写入仍须失败
	require.NoError(t, db.Exec("PRAGMA query_only = 0").Error)
	err = db.Exec("CREATE TABLE probe (x)").Error
	require.Error(t, err)
	assert.Contains(t, err.Error(), "readonly")
}

// 读写模式下 SQLite 会为不存在的路径悄悄建一个空库，服务随后以「零首诗」正常运行；
// 只读模式下这种配置错误应在启动时就暴露出来。
func TestOpenReadOnlyFailsOnMissingFile(t *testing.T) {
	_, err := OpenReadOnly(filepath.Join(t.TempDir(), "missing.db"), 1, 1)
	assert.Error(t, err)
}

// 用户卷里的老库缺少新版本才有的结构，OpenForServing 必须先补齐再只读打开，
// 否则只读连接上的迁移会因为写权限直接失败，服务起不来。
func TestOpenForServingMigratesOldDatabase(t *testing.T) {
	path := buildReleasedDB(t)

	// 模拟老版本发布的库：没有 counters 计数表
	old, err := Open(path, 1, 1)
	require.NoError(t, err)
	require.NoError(t, old.Exec("DROP TABLE "+countersTable).Error)
	require.NoError(t, old.Close())

	db, err := OpenForServing(path, 2, 1)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	count, err := NewRepository(db).CountPoems()
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	_, err = NewRepository(db).GetOrCreateDynasty("宋")
	assert.Error(t, err, "serving connections must still be read-only")
}

func TestOpenForServingFailsOnMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.db")
	_, err := OpenForServing(path, 1, 1)
	require.Error(t, err)

	_, statErr := os.Stat(path)
	assert.True(t, os.IsNotExist(statErr), "must not create an empty database")
}
