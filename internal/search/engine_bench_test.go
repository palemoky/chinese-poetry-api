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
			ID:        int64(i + 1),
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
