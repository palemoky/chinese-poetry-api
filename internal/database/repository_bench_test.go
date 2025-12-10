package database

import (
	"testing"

	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupBenchDB creates an in-memory database for benchmarking
func setupBenchDB(b *testing.B) (*DB, *Repository) {
	gormDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		b.Fatal(err)
	}

	err = gormDB.AutoMigrate(&Dynasty{}, &Author{}, &PoetryType{}, &Poem{})
	if err != nil {
		b.Fatal(err)
	}

	db := &DB{DB: gormDB}
	repo := NewRepository(db)
	return db, repo
}

// BenchmarkGetPoemByID benchmarks single poem retrieval
func BenchmarkGetPoemByID(b *testing.B) {
	_, repo := setupBenchDB(b)

	// Create test data
	dynastyID, _ := repo.GetOrCreateDynasty("唐")
	authorID, _ := repo.GetOrCreateAuthor("李白", "li bai", "lb", dynastyID)

	poem := &Poem{
		ID:          12345678901234,
		Title:       "静夜思",
		TitlePinyin: strPtr("jing ye si"),
		Content:     datatypes.JSON([]byte(`["床前明月光","疑是地上霜","举头望明月","低头思故乡"]`)),
		AuthorID:    &authorID,
		DynastyID:   &dynastyID,
	}
	_ = repo.InsertPoem(poem)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = repo.GetPoemByID("12345678901234")
	}
}

// BenchmarkListPoems benchmarks poem listing with pagination
func BenchmarkListPoems(b *testing.B) {
	_, repo := setupBenchDB(b)

	// Create test data
	dynastyID, _ := repo.GetOrCreateDynasty("唐")
	authorID, _ := repo.GetOrCreateAuthor("李白", "li bai", "lb", dynastyID)

	for i := 0; i < 100; i++ {
		poem := &Poem{
			ID:          int64(10000000000000 + i),
			Title:       "静夜思",
			TitlePinyin: strPtr("jing ye si"),
			Content:     datatypes.JSON([]byte(`["床前明月光","疑是地上霜"]`)),
			AuthorID:    &authorID,
			DynastyID:   &dynastyID,
		}
		_ = repo.InsertPoem(poem)
	}

	testCases := []struct {
		name     string
		page     int
		pageSize int
	}{
		{"small_page", 1, 10},
		{"medium_page", 1, 20},
		{"large_page", 1, 50},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, _ = repo.ListPoems(tc.pageSize, (tc.page-1)*tc.pageSize)
			}
		})
	}
}

// BenchmarkGetOrCreateAuthor benchmarks author creation/retrieval
func BenchmarkGetOrCreateAuthor(b *testing.B) {
	_, repo := setupBenchDB(b)

	dynastyID, _ := repo.GetOrCreateDynasty("唐")

	testCases := []struct {
		name   string
		author string
	}{
		{"new", "李白"},
		{"existing", "李白"},
	}

	// Pre-create for "existing" test
	_, _ = repo.GetOrCreateAuthor("李白", "li bai", "lb", dynastyID)

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, _ = repo.GetOrCreateAuthor(tc.author, "li bai", "lb", dynastyID)
			}
		})
	}
}

// BenchmarkInsertPoem benchmarks poem insertion
func BenchmarkInsertPoem(b *testing.B) {
	_, repo := setupBenchDB(b)

	dynastyID, _ := repo.GetOrCreateDynasty("唐")
	authorID, _ := repo.GetOrCreateAuthor("李白", "li bai", "lb", dynastyID)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		poem := &Poem{
			ID:          int64(10000000000000 + i),
			Title:       "静夜思",
			TitlePinyin: strPtr("jing ye si"),
			Content:     datatypes.JSON([]byte(`["床前明月光","疑是地上霜"]`)),
			AuthorID:    &authorID,
			DynastyID:   &dynastyID,
		}
		b.StartTimer()
		_ = repo.InsertPoem(poem)
	}
}

// BenchmarkGetAuthorByID benchmarks author retrieval by ID
func BenchmarkGetAuthorByID(b *testing.B) {
	_, repo := setupBenchDB(b)

	dynastyID, _ := repo.GetOrCreateDynasty("唐")
	authorID, _ := repo.GetOrCreateAuthor("李白", "li bai", "lb", dynastyID)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = repo.GetAuthorByID(authorID)
	}
}

func strPtr(s string) *string {
	return &s
}
