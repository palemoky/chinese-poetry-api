package search

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/palemoky/chinese-poetry-api/internal/database"
)

// setupTestEngine creates a test search engine with sample data
func setupTestEngine(t *testing.T) *Engine {
	gormDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// Auto migrate
	err = gormDB.AutoMigrate(&database.Dynasty{}, &database.Author{}, &database.PoetryType{}, &database.Poem{})
	require.NoError(t, err)

	db := &database.DB{DB: gormDB}
	engine := NewEngine(db)

	// Create test data
	dynasty := &database.Dynasty{ID: 1, Name: "唐"}
	gormDB.Create(dynasty)

	author := &database.Author{
		ID:        1,
		Name:      "李白",
		DynastyID: &dynasty.ID,
	}
	gormDB.Create(author)

	poems := []database.Poem{
		{
			ID:        12345678901234,
			Title:     "静夜思",
			Content:   datatypes.JSON([]byte(`["床前明月光","疑是地上霜","举头望明月","低头思故乡"]`)),
			AuthorID:  &author.ID,
			DynastyID: &dynasty.ID,
		},
		{
			ID:        12345678901235,
			Title:     "望庐山瀑布",
			Content:   datatypes.JSON([]byte(`["日照香炉生紫烟","遥看瀑布挂前川","飞流直下三千尺","疑是银河落九天"]`)),
			AuthorID:  &author.ID,
			DynastyID: &dynasty.ID,
		},
		{
			ID:        12345678901236,
			Title:     "早发白帝城",
			Content:   datatypes.JSON([]byte(`["朝辞白帝彩云间","千里江陵一日还","两岸猿声啼不住","轻舟已过万重山"]`)),
			AuthorID:  &author.ID,
			DynastyID: &dynasty.ID,
		},
	}

	for _, poem := range poems {
		gormDB.Create(&poem)
	}

	return engine
}

func TestSearch(t *testing.T) {
	engine := setupTestEngine(t)

	t.Run("search all with Chinese query", func(t *testing.T) {
		result, err := engine.Search(SearchParams{
			Query:      "静夜思",
			SearchType: SearchTypeAll,
			Page:       1,
			PageSize:   10,
		})

		require.NoError(t, err)
		assert.Equal(t, 1, result.TotalCount)
		assert.Len(t, result.Poems, 1)
		assert.Equal(t, "静夜思", result.Poems[0].Title)
		assert.False(t, result.HasMore)
	})

	t.Run("search by title", func(t *testing.T) {
		result, err := engine.Search(SearchParams{
			Query:      "瀑布",
			SearchType: SearchTypeTitle,
			Page:       1,
			PageSize:   10,
		})

		require.NoError(t, err)
		assert.Equal(t, 1, result.TotalCount)
		assert.Contains(t, result.Poems[0].Title, "瀑布")
	})

	t.Run("search by content", func(t *testing.T) {
		result, err := engine.Search(SearchParams{
			Query:      "明月",
			SearchType: SearchTypeContent,
			Page:       1,
			PageSize:   10,
		})

		require.NoError(t, err)
		assert.GreaterOrEqual(t, result.TotalCount, 1)
	})

	t.Run("search by author", func(t *testing.T) {
		result, err := engine.Search(SearchParams{
			Query:      "李白",
			SearchType: SearchTypeAuthor,
			Page:       1,
			PageSize:   10,
		})

		require.NoError(t, err)
		assert.Equal(t, 3, result.TotalCount)
		assert.Len(t, result.Poems, 3)
	})

	t.Run("pagination", func(t *testing.T) {
		result, err := engine.Search(SearchParams{
			Query:      "李白",
			SearchType: SearchTypeAuthor,
			Page:       1,
			PageSize:   2,
		})

		require.NoError(t, err)
		assert.Equal(t, 3, result.TotalCount)
		assert.Len(t, result.Poems, 2)
		assert.True(t, result.HasMore)
	})

	t.Run("default page and page size", func(t *testing.T) {
		result, err := engine.Search(SearchParams{
			Query:      "李白",
			SearchType: SearchTypeAuthor,
			Page:       0,
			PageSize:   0,
		})

		require.NoError(t, err)
		assert.GreaterOrEqual(t, result.TotalCount, 1)
	})

	t.Run("no results", func(t *testing.T) {
		result, err := engine.Search(SearchParams{
			Query:      "不存在的诗词",
			SearchType: SearchTypeAll,
			Page:       1,
			PageSize:   10,
		})

		require.NoError(t, err)
		assert.Equal(t, 0, result.TotalCount)
		assert.Len(t, result.Poems, 0)
		assert.False(t, result.HasMore)
	})
}

func TestSearchByTitle(t *testing.T) {
	engine := setupTestEngine(t)

	t.Run("exact match", func(t *testing.T) {
		poems, count := engine.searchByTitle("静夜思", 10, 0)
		assert.Equal(t, int64(1), count)
		assert.Len(t, poems, 1)
		assert.Equal(t, "静夜思", poems[0].Title)
	})

	t.Run("partial match", func(t *testing.T) {
		poems, count := engine.searchByTitle("瀑布", 10, 0)
		assert.Equal(t, int64(1), count)
		assert.Contains(t, poems[0].Title, "瀑布")
	})

	t.Run("no match", func(t *testing.T) {
		_, count := engine.searchByTitle("不存在", 10, 0)
		assert.Equal(t, int64(0), count)
	})
}

func TestSearchByContent(t *testing.T) {
	engine := setupTestEngine(t)

	t.Run("find by content", func(t *testing.T) {
		_, count := engine.searchByContent("明月", 10, 0)
		assert.GreaterOrEqual(t, count, int64(1))
	})

	t.Run("no match", func(t *testing.T) {
		_, count := engine.searchByContent("不存在的内容", 10, 0)
		assert.Equal(t, int64(0), count)
	})
}

func TestSearchByAuthor(t *testing.T) {
	engine := setupTestEngine(t)

	t.Run("find by author name", func(t *testing.T) {
		_, count := engine.searchByAuthor("李白", 10, 0)
		assert.Equal(t, int64(3), count)
	})

	t.Run("partial author name", func(t *testing.T) {
		_, count := engine.searchByAuthor("李", 10, 0)
		assert.GreaterOrEqual(t, count, int64(1))
	})

	t.Run("no match", func(t *testing.T) {
		_, count := engine.searchByAuthor("杜甫", 10, 0)
		assert.Equal(t, int64(0), count)
	})
}

func TestSearchAll(t *testing.T) {
	engine := setupTestEngine(t)

	t.Run("search across all fields", func(t *testing.T) {
		_, count := engine.searchAll("李白", 10, 0)
		assert.GreaterOrEqual(t, count, int64(1))
	})

	t.Run("search by title", func(t *testing.T) {
		poems, count := engine.searchAll("静夜思", 10, 0)
		assert.Equal(t, int64(1), count)
		assert.Equal(t, "静夜思", poems[0].Title)
	})

	t.Run("search by content", func(t *testing.T) {
		_, count := engine.searchAll("明月", 10, 0)
		assert.GreaterOrEqual(t, count, int64(1))
	})
}
