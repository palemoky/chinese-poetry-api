package search

import (
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/palemoky/chinese-poetry-api/internal/database"
)

// setupBenchmarkDB creates an in-memory database for benchmarking
func setupBenchmarkDB(b *testing.B) *database.DB {
	gormDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		b.Fatal(err)
	}

	// Auto migrate
	err = gormDB.AutoMigrate(
		&database.Dynasty{},
		&database.Author{},
		&database.PoetryType{},
		&database.Poem{},
	)
	if err != nil {
		b.Fatal(err)
	}

	return &database.DB{DB: gormDB}
}

func BenchmarkIsPinyinQuery(b *testing.B) {
	testCases := []struct {
		name  string
		query string
	}{
		{"chinese", "静夜思"},
		{"pinyin", "jing ye si"},
		{"mixed", "libai李白"},
		{"english", "hello world"},
		{"numbers", "123456"},
		{"empty", ""},
		{"long_chinese", "床前明月光疑是地上霜举头望明月低头思故乡"},
		{"long_pinyin", "chuang qian ming yue guang yi shi di shang shuang"},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			b.ResetTimer()
			for b.Loop() {
			}
		})
	}
}

// BenchmarkSearch benchmarks the Search function with different search types
func BenchmarkSearch(b *testing.B) {
	db := setupBenchmarkDB(b)
	engine := NewEngine(db)
	repo := database.NewRepository(db)

	// Create test data
	dynastyID, _ := repo.GetOrCreateDynasty("唐")
	authorID, _ := repo.GetOrCreateAuthor("李白", dynastyID)

	for i := range 100 {
		poem := &database.Poem{
			ID:        int64(10000000000000 + i),
			Title:     "静夜思",
			Content:   []byte(`["床前明月光","疑是地上霜","举头望明月","低头思故乡"]`),
			AuthorID:  &authorID,
			DynastyID: &dynastyID,
		}
		_ = repo.InsertPoem(poem)
	}

	testCases := []struct {
		name       string
		searchType SearchType
		query      string
	}{
		{"all_chinese", SearchTypeAll, "静夜思"},
		{"all_pinyin", SearchTypeAll, "jingye"},
		{"title", SearchTypeTitle, "静夜思"},
		{"content", SearchTypeContent, "明月"},
		{"author", SearchTypeAuthor, "李白"},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			params := SearchParams{
				Query:      tc.query,
				SearchType: tc.searchType,
				Page:       1,
				PageSize:   20,
			}

			b.ResetTimer()
			for b.Loop() {
				_, _ = engine.Search(params)
			}
		})
	}
}
