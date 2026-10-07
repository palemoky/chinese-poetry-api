package database

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

func TestSubstringMatch(t *testing.T) {
	tests := []struct {
		in, cond, arg string
	}{
		// 普通查询保持 LIKE，FTS5 trigram 才能走索引
		{"明月", "c LIKE ?", "%明月%"},
		// 含 LIKE 通配符时改用 GLOB，% 与 _ 在 GLOB 中是普通字符
		{"100%", "c GLOB ?", "*100%*"},
		{"a_b", "c GLOB ?", "*a_b*"},
		// GLOB 自己的元字符要放进字符类
		{"%*?[", "c GLOB ?", "*%[*][?][[]*"},
	}
	for _, tt := range tests {
		cond, arg := substringMatch("c", tt.in)
		assert.Equal(t, tt.cond, cond, tt.in)
		assert.Equal(t, tt.arg, arg, tt.in)
	}
}

// seedWildcardPoems 写入两首诗：只有一首的标题与正文含字面意义上的 % 与 _。
func seedWildcardPoems(t *testing.T, repo *Repository) {
	t.Helper()
	dynastyID, err := repo.GetOrCreateDynasty("唐")
	require.NoError(t, err)
	authorID, err := repo.GetOrCreateAuthor("李白", dynastyID)
	require.NoError(t, err)

	for _, p := range []*Poem{
		{ID: 1, Title: "静夜思", Content: datatypes.JSON([]byte(`["床前明月光"]`))},
		{ID: 2, Title: "百分_之%百", Content: datatypes.JSON([]byte(`["春%风_十里"]`))},
	} {
		p.AuthorID, p.DynastyID = &authorID, &dynastyID
		require.NoError(t, repo.InsertPoem(p))
	}
}

// 搜索词里的 % 与 _ 必须按字面匹配：此前它们被当作 LIKE 通配符，
// 搜 "%" 会返回整个语料库。
func TestSearchPoemsTreatsWildcardsLiterally(t *testing.T) {
	repo := NewRepository(setupTestDB(t))
	seedWildcardPoems(t, repo)

	for _, searchType := range []string{"all", "title", "content"} {
		for _, q := range []string{"%", "_"} {
			poems, total, err := repo.SearchPoems(q, searchType, 1, 10)
			require.NoError(t, err)
			require.Len(t, poems, 1, "type=%s q=%q", searchType, q)
			assert.Equal(t, int64(2), poems[0].ID)
			assert.Equal(t, int64(1), total)
		}
	}

	poems, _, err := repo.SearchPoems("%", "author", 1, 10)
	require.NoError(t, err)
	assert.Empty(t, poems, "no author name contains a literal %")

	// 不含通配符的普通查询不受影响
	poems, _, err = repo.SearchPoems("明月", "content", 1, 10)
	require.NoError(t, err)
	require.Len(t, poems, 1)
	assert.Equal(t, int64(1), poems[0].ID)
}

func TestGetRandomPoemByCharTreatsWildcardsLiterally(t *testing.T) {
	repo := NewRepository(setupTestDB(t))
	seedWildcardPoems(t, repo)

	for range 10 {
		poem, err := repo.GetRandomPoemByChar("%")
		require.NoError(t, err)
		assert.Equal(t, int64(2), poem.ID)
	}

	_, err := repo.GetRandomPoemByChar("*")
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
}

// 随机取到的诗要和按 ID 查询一样带齐关联数据。
func TestGetRandomPoemLoadsRelations(t *testing.T) {
	repo := NewRepository(setupTestDB(t))
	seedWildcardPoems(t, repo)

	poem, err := repo.GetRandomPoem(nil, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, poem.Author)
	assert.Equal(t, "李白", poem.Author.Name)
	require.NotNil(t, poem.Author.Dynasty)
	require.NotNil(t, poem.Dynasty)
	assert.Equal(t, "唐", poem.Dynasty.Name)

	poem, err = repo.GetRandomPoemByChar("床")
	require.NoError(t, err)
	assert.Equal(t, int64(1), poem.ID)
	require.NotNil(t, poem.Author)
}

// 数据库故障不能再被当成「没有符合条件的诗」：调用方要据此区分 404 与 500。
func TestRandomPoemPropagatesDatabaseErrors(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRepository(db)
	seedWildcardPoems(t, repo)

	sqlDB, err := db.DB.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	authorID := int64(1)
	_, err = repo.GetRandomPoem(nil, &authorID, nil)
	require.Error(t, err)
	assert.False(t, errors.Is(err, gorm.ErrRecordNotFound), "got %v", err)

	_, err = repo.GetRandomPoemByChar("床")
	require.Error(t, err)
	assert.False(t, errors.Is(err, gorm.ErrRecordNotFound), "got %v", err)
}
