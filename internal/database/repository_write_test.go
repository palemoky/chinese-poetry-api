package database

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// setupTestDB creates an in-memory database for testing
func setupGetPoetryTypeIDsTestDB(t *testing.T) (*DB, *Repository) {
	gormDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	db := &DB{DB: gormDB}
	err = db.Migrate()
	require.NoError(t, err)

	repo := NewRepository(db)
	return db, repo
}

func TestGetPoetryTypeIDs(t *testing.T) {
	_, repo := setupGetPoetryTypeIDsTestDB(t)

	// Create test poetry types with ON CONFLICT to handle unique constraint
	types := []string{"五言绝句", "七言绝句", "五言律诗", "七言律诗"}
	for _, typeName := range types {
		poetryType := PoetryType{Name: typeName}
		err := repo.db.Table(repo.poetryTypesTable()).
			Clauses(clause.OnConflict{DoNothing: true}).
			Create(&poetryType).Error
		require.NoError(t, err)
	}

	tests := []struct {
		name          string
		inputNames    []string
		expectError   bool
		expectedCount int
	}{
		{
			name:          "fetch multiple existing types",
			inputNames:    []string{"五言绝句", "七言绝句"},
			expectError:   false,
			expectedCount: 2,
		},
		{
			name:          "fetch all types",
			inputNames:    types,
			expectError:   false,
			expectedCount: 4,
		},
		{
			name:          "fetch single type",
			inputNames:    []string{"五言绝句"},
			expectError:   false,
			expectedCount: 1,
		},
		{
			name:          "empty input",
			inputNames:    []string{},
			expectError:   false,
			expectedCount: 0,
		},
		{
			name:        "non-existent type",
			inputNames:  []string{"不存在的类型"},
			expectError: true,
		},
		{
			name:        "mixed existing and non-existent",
			inputNames:  []string{"五言绝句", "不存在的类型"},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ids, err := repo.GetPoetryTypeIDs(tt.inputNames)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, ids)
			} else {
				require.NoError(t, err)
				assert.Len(t, ids, tt.expectedCount)

				// Verify IDs are returned in the same order as input
				if len(tt.inputNames) > 0 {
					for i, name := range tt.inputNames {
						// Verify we can look up the type by ID and get the same name
						var poetryType PoetryType
						err := repo.db.Table(repo.poetryTypesTable()).First(&poetryType, ids[i]).Error
						require.NoError(t, err)
						assert.Equal(t, name, poetryType.Name)
					}
				}
			}
		})
	}
}

func TestGetPoetryTypeIDsWithCache(t *testing.T) {
	db, repo := setupGetPoetryTypeIDsTestDB(t)
	cachedRepo := NewCachedRepository(repo)

	// Create test poetry types with ON CONFLICT to handle unique constraint
	types := []string{"五言绝句", "七言绝句", "五言律诗"}
	for _, typeName := range types {
		poetryType := PoetryType{Name: typeName}
		err := db.Table(repo.poetryTypesTable()).
			Clauses(clause.OnConflict{DoNothing: true}).
			Create(&poetryType).Error
		require.NoError(t, err)
	}

	// First call - should populate cache
	ids1, err := cachedRepo.GetPoetryTypeIDs(types)
	require.NoError(t, err)
	assert.Len(t, ids1, 3)

	// Second call - should use cache
	ids2, err := cachedRepo.GetPoetryTypeIDs(types)
	require.NoError(t, err)
	assert.Equal(t, ids1, ids2)

	// Partial cache hit
	partialTypes := []string{"五言绝句", "七言绝句"} // These should be in cache
	ids3, err := cachedRepo.GetPoetryTypeIDs(partialTypes)
	require.NoError(t, err)
	assert.Len(t, ids3, 2)
	assert.Equal(t, ids1[0], ids3[0])
	assert.Equal(t, ids1[1], ids3[1])

	// Verify cache stats
	stats := cachedRepo.GetCacheStats()
	assert.Equal(t, 3, stats["types"])
}
