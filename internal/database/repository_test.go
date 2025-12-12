package database

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupTestDB creates an in-memory SQLite database for testing
// Creates language-specific tables (zh_hans) for the default language
func setupTestDB(t *testing.T) *DB {
	gormDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err, "Failed to create test database")

	db := &DB{DB: gormDB}

	// Create tables for default language (zh_hans)
	err = db.migrateTablesForLang(LangHans)
	require.NoError(t, err, "Failed to run migrations")

	return db
}

// createTestPoem is a helper to create poems in tests using dynamic table names
func createTestPoem(repo *Repository, poem *Poem) error {
	return repo.db.Table(repo.poemsTable()).Create(poem).Error
}

// createTestPoetryType is a helper to create poetry types in tests using dynamic table names
func createTestPoetryType(repo *Repository, ptype *PoetryType) error {
	return repo.db.Table(repo.poetryTypesTable()).Create(ptype).Error
}

func TestNewRepository(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRepository(db)

	assert.NotNil(t, repo)
	assert.NotNil(t, repo.db)
}

func TestGetOrCreateDynasty(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRepository(db)

	tests := []struct {
		name        string
		dynastyName string
		wantErr     bool
	}{
		{"create new dynasty", "唐", false},
		{"get existing dynasty", "唐", false},
		{"create another dynasty", "宋", false},
	}

	var firstID int64
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, err := repo.GetOrCreateDynasty(tt.dynastyName)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Greater(t, id, int64(0))

				switch i {
				case 0:
					firstID = id
				case 1:
					// Second call should return same ID
					assert.Equal(t, firstID, id, "Should return same ID for existing dynasty")
				}
			}
		})
	}
}

func TestGetOrCreateAuthor(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRepository(db)

	// Create dynasty first
	dynastyID, err := repo.GetOrCreateDynasty("唐")
	require.NoError(t, err)

	tests := []struct {
		name       string
		authorName string
		dynastyID  int64
		wantErr    bool
	}{
		{
			name:       "create new author",
			authorName: "李白",
			dynastyID:  dynastyID,
			wantErr:    false,
		},
		{
			name:       "get existing author",
			authorName: "李白",
			dynastyID:  dynastyID,
			wantErr:    false,
		},
	}

	var firstID int64
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, err := repo.GetOrCreateAuthor(tt.authorName, tt.dynastyID)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Greater(t, id, int64(0))

				switch i {
				case 0:
					firstID = id
				case 1:
					assert.Equal(t, firstID, id, "Should return same ID for existing author")
				}
			}
		})
	}
}

func TestGetPoetryTypeID(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRepository(db)

	// First, create a poetry type for testing using dynamic table name
	poetryType := &PoetryType{
		Name:     "五言绝句",
		Category: "诗",
	}
	err := db.Table(repo.poetryTypesTable()).Create(poetryType).Error
	require.NoError(t, err, "Failed to create test poetry type")

	tests := []struct {
		name     string
		typeName string
		wantErr  bool
	}{
		{"get existing type", "五言绝句", false},
		{"get non-existent type", "不存在的类型", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, err := repo.GetPoetryTypeID(tt.typeName)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Greater(t, id, int64(0))
			}
		})
	}
}

func TestCountPoems(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRepository(db)

	// Initially should be 0
	count, err := repo.CountPoems()
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

func TestCountAuthors(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRepository(db)

	// Initially should be 0
	count, err := repo.CountAuthors()
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

// Benchmark tests
func BenchmarkGetOrCreateDynasty(b *testing.B) {
	gormDB, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	_ = gormDB.AutoMigrate(&Dynasty{})
	db := &DB{DB: gormDB}
	repo := NewRepository(db)

	for b.Loop() {
		_, _ = repo.GetOrCreateDynasty("唐")
	}
}
