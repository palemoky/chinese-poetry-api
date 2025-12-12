package database

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
)

// calculateTestHash generates a SHA256 hash for test content
func calculateTestHash(content []byte) string {
	hash := sha256.Sum256(content)
	return hex.EncodeToString(hash[:])
}

// Test new repository methods added in repository_additional.go

func TestGetAuthorsWithStats(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRepository(db)

	// Create test data
	dynastyID, _ := repo.GetOrCreateDynasty("唐")
	author1ID, _ := repo.GetOrCreateAuthor("李白", dynastyID)
	_, _ = repo.GetOrCreateAuthor("杜甫", dynastyID)
	typeID, _ := repo.GetPoetryTypeID("五言绝句")

	// Create poems for authors
	content1 := []byte(`["床前明月光"]`)
	_ = createTestPoem(repo, &Poem{
		ID:          100001,
		Title:       "静夜思",
		Content:     datatypes.JSON(content1),
		ContentHash: calculateTestHash(content1),
		AuthorID:    &author1ID,
		DynastyID:   &dynastyID,
		TypeID:      &typeID,
	})

	content2 := []byte(`["日照香炉生紫烟"]`)
	_ = createTestPoem(repo, &Poem{
		ID:          100002,
		Title:       "望庐山瀑布",
		Content:     datatypes.JSON(content2),
		ContentHash: calculateTestHash(content2),
		AuthorID:    &author1ID,
		DynastyID:   &dynastyID,
		TypeID:      &typeID,
	})

	tests := []struct {
		name    string
		limit   int
		offset  int
		wantLen int
	}{
		{
			name:    "get first page",
			limit:   10,
			offset:  0,
			wantLen: 2,
		},
		{
			name:    "get with limit",
			limit:   1,
			offset:  0,
			wantLen: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			authors, err := repo.GetAuthorsWithStats(tt.limit, tt.offset)
			require.NoError(t, err)
			assert.Len(t, authors, tt.wantLen)

			if len(authors) > 0 {
				// First author should have most poems
				assert.Greater(t, authors[0].PoemCount, 0)
			}
		})
	}
}

func TestGetAuthorByID(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRepository(db)

	dynastyID, _ := repo.GetOrCreateDynasty("唐")
	authorID, _ := repo.GetOrCreateAuthor("李白", dynastyID)

	tests := []struct {
		name    string
		id      int64
		wantErr bool
	}{
		{
			name:    "get existing author",
			id:      authorID,
			wantErr: false,
		},
		{
			name:    "get non-existent author",
			id:      999999,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			author, err := repo.GetAuthorByID(tt.id)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.id, author.ID)
				assert.NotNil(t, author.Dynasty)
			}
		})
	}
}

func TestGetPoemsByAuthor(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRepository(db)

	dynastyID, _ := repo.GetOrCreateDynasty("唐")
	authorID, _ := repo.GetOrCreateAuthor("李白", dynastyID)
	typeID, _ := repo.GetPoetryTypeID("五言绝句")

	// Create test poems with unique content
	for i := range 5 {
		content := []byte(fmt.Sprintf(`["测试内容%d"]`, i))
		_ = createTestPoem(repo, &Poem{
			ID:          int64(200000 + i),
			Title:       fmt.Sprintf("测试诗歌%d", i),
			Content:     datatypes.JSON(content),
			ContentHash: calculateTestHash(content),
			AuthorID:    &authorID,
			DynastyID:   &dynastyID,
			TypeID:      &typeID,
		})
	}

	tests := []struct {
		name    string
		limit   int
		offset  int
		wantLen int
	}{
		{"get all poems", 10, 0, 5},
		{"get with pagination", 2, 0, 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			poems, err := repo.GetPoemsByAuthor(authorID, tt.limit, tt.offset)
			require.NoError(t, err)
			assert.Len(t, poems, tt.wantLen)
		})
	}
}

func TestGetDynastiesWithStats(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRepository(db)

	// Create test data
	dynasty1ID, _ := repo.GetOrCreateDynasty("唐")
	_, _ = repo.GetOrCreateDynasty("宋")

	author1ID, _ := repo.GetOrCreateAuthor("李白", dynasty1ID)
	typeID, _ := repo.GetPoetryTypeID("五言绝句")

	content3 := []byte(`["床前明月光"]`)
	_ = createTestPoem(repo, &Poem{
		ID:          300001,
		Title:       "静夜思",
		Content:     datatypes.JSON(content3),
		ContentHash: calculateTestHash(content3),
		AuthorID:    &author1ID,
		DynastyID:   &dynasty1ID,
		TypeID:      &typeID,
	})

	dynasties, err := repo.GetDynastiesWithStats()
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(dynasties), 2)

	// Find Tang dynasty
	var tangDynasty *DynastyWithStats
	for i := range dynasties {
		if dynasties[i].Name == "唐" {
			tangDynasty = &dynasties[i]
			break
		}
	}

	require.NotNil(t, tangDynasty)
	assert.Greater(t, tangDynasty.PoemCount, 0)
	assert.Greater(t, tangDynasty.AuthorCount, 0)
}

func TestGetDynastyByID(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRepository(db)

	dynastyID, _ := repo.GetOrCreateDynasty("唐")

	tests := []struct {
		name    string
		id      int64
		wantErr bool
	}{
		{"get existing dynasty", dynastyID, false},
		{"get non-existent dynasty", 999999, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dynasty, err := repo.GetDynastyByID(tt.id)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.id, dynasty.ID)
			}
		})
	}
}

func TestGetPoetryTypesWithStats(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRepository(db)

	// Create test data
	dynastyID, _ := repo.GetOrCreateDynasty("唐")
	authorID, _ := repo.GetOrCreateAuthor("李白", dynastyID)

	// Create poetry type first
	poetryType := &PoetryType{
		Name:     "五言绝句",
		Category: "诗",
	}
	_ = createTestPoetryType(repo, poetryType)
	typeID := poetryType.ID

	content4 := []byte(`["床前明月光"]`)
	_ = createTestPoem(repo, &Poem{
		ID:          400001,
		Title:       "静夜思",
		Content:     datatypes.JSON(content4),
		ContentHash: calculateTestHash(content4),
		AuthorID:    &authorID,
		DynastyID:   &dynastyID,
		TypeID:      &typeID,
	})

	types, err := repo.GetPoetryTypesWithStats()
	require.NoError(t, err)
	assert.Greater(t, len(types), 0)

	// Find the type we created
	var foundType *PoetryTypeWithStats
	for i := range types {
		if types[i].ID == typeID {
			foundType = &types[i]
			break
		}
	}

	require.NotNil(t, foundType)
	assert.Greater(t, foundType.PoemCount, 0)
}

func TestGetPoetryTypeByID(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRepository(db)

	// Create a poetry type for testing
	poetryType := &PoetryType{
		Name:     "五言绝句",
		Category: "诗",
	}
	err := createTestPoetryType(repo, poetryType)
	require.NoError(t, err)
	typeID := poetryType.ID

	tests := []struct {
		name    string
		id      int64
		wantErr bool
	}{
		{
			name:    "get existing type",
			id:      typeID,
			wantErr: false,
		},
		{
			name:    "get non-existent type",
			id:      999999,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			poetryType, err := repo.GetPoetryTypeByID(tt.id)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.id, poetryType.ID)
			}
		})
	}
}

func TestGetPoemsByType(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRepository(db)

	dynastyID, _ := repo.GetOrCreateDynasty("唐")
	authorID, _ := repo.GetOrCreateAuthor("李白", dynastyID)
	typeID, _ := repo.GetPoetryTypeID("五言绝句")

	// Create test poems with unique content
	for i := range 3 {
		content := []byte(fmt.Sprintf(`["测试内容%d"]`, i))
		_ = createTestPoem(repo, &Poem{
			ID:          int64(500000 + i),
			Title:       fmt.Sprintf("测试诗歌%d", i),
			Content:     datatypes.JSON(content),
			ContentHash: calculateTestHash(content),
			AuthorID:    &authorID,
			DynastyID:   &dynastyID,
			TypeID:      &typeID,
		})
	}

	tests := []struct {
		name    string
		limit   int
		offset  int
		wantLen int
	}{
		{
			name:    "get all poems",
			limit:   10,
			offset:  0,
			wantLen: 3,
		},
		{
			name:    "get with pagination",
			limit:   2,
			offset:  0,
			wantLen: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			poems, err := repo.GetPoemsByType(typeID, tt.limit, tt.offset)
			require.NoError(t, err)
			assert.Len(t, poems, tt.wantLen)
		})
	}
}
