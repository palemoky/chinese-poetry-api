package database

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
)

// Test new repository methods added in repository_additional.go

func TestGetAuthorsWithStats(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRepository(db)

	// Create test data
	dynastyID, _ := repo.GetOrCreateDynasty("唐")
	author1ID, _ := repo.GetOrCreateAuthor("李白", "li bai", "lb", dynastyID)
	_, _ = repo.GetOrCreateAuthor("杜甫", "du fu", "df", dynastyID)
	typeID, _ := repo.GetPoetryTypeID("五言绝句")

	// Create poems for authors
	_ = db.Create(&Poem{
		ID:        100001,
		Title:     "静夜思",
		Content:   datatypes.JSON([]byte(`["床前明月光"]`)),
		AuthorID:  &author1ID,
		DynastyID: &dynastyID,
		TypeID:    &typeID,
	}).Error

	_ = db.Create(&Poem{
		ID:        100002,
		Title:     "望庐山瀑布",
		Content:   datatypes.JSON([]byte(`["日照香炉生紫烟"]`)),
		AuthorID:  &author1ID,
		DynastyID: &dynastyID,
		TypeID:    &typeID,
	}).Error

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
	authorID, _ := repo.GetOrCreateAuthor("李白", "li bai", "lb", dynastyID)

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
	authorID, _ := repo.GetOrCreateAuthor("李白", "li bai", "lb", dynastyID)
	typeID, _ := repo.GetPoetryTypeID("五言绝句")

	// Create test poems
	for i := 0; i < 5; i++ {
		_ = db.Create(&Poem{
			ID:        int64(200000 + i),
			Title:     "测试诗歌",
			Content:   datatypes.JSON([]byte(`["测试内容"]`)),
			AuthorID:  &authorID,
			DynastyID: &dynastyID,
			TypeID:    &typeID,
		}).Error
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
			wantLen: 5,
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

	author1ID, _ := repo.GetOrCreateAuthor("李白", "li bai", "lb", dynasty1ID)
	typeID, _ := repo.GetPoetryTypeID("五言绝句")

	_ = db.Create(&Poem{
		ID:        300001,
		Title:     "静夜思",
		Content:   datatypes.JSON([]byte(`["床前明月光"]`)),
		AuthorID:  &author1ID,
		DynastyID: &dynasty1ID,
		TypeID:    &typeID,
	}).Error

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
		{
			name:    "get existing dynasty",
			id:      dynastyID,
			wantErr: false,
		},
		{
			name:    "get non-existent dynasty",
			id:      999999,
			wantErr: true,
		},
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
	authorID, _ := repo.GetOrCreateAuthor("李白", "li bai", "lb", dynastyID)

	// Create poetry type first
	poetryType := &PoetryType{
		Name:     "五言绝句",
		Category: "诗",
	}
	_ = db.Create(poetryType).Error
	typeID := poetryType.ID

	_ = db.Create(&Poem{
		ID:        400001,
		Title:     "静夜思",
		Content:   datatypes.JSON([]byte(`["床前明月光"]`)),
		AuthorID:  &authorID,
		DynastyID: &dynastyID,
		TypeID:    &typeID,
	}).Error

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
	err := db.Create(poetryType).Error
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
	authorID, _ := repo.GetOrCreateAuthor("李白", "li bai", "lb", dynastyID)
	typeID, _ := repo.GetPoetryTypeID("五言绝句")

	// Create test poems
	for i := 0; i < 3; i++ {
		_ = db.Create(&Poem{
			ID:        int64(500000 + i),
			Title:     "测试诗歌",
			Content:   datatypes.JSON([]byte(`["测试内容"]`)),
			AuthorID:  &authorID,
			DynastyID: &dynastyID,
			TypeID:    &typeID,
		}).Error
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
