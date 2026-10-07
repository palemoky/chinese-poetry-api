package database

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// setupTestDB 创建测试用的内存数据库。
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

	// 借助 ON CONFLICT 写入测试用的体裁，规避唯一约束冲突
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
		{
			// 重复的名称在 IN 子句中会合并成一行。早先拿返回行数与 len(names)
			// 比较会因此误拒该请求，表现为
			// as a 404 for ?type=五言绝句&type=五言绝句.
			name:          "repeated name",
			inputNames:    []string{"五言绝句", "五言绝句"},
			expectError:   false,
			expectedCount: 2,
		},
		{
			name:          "repeated name mixed with a distinct one",
			inputNames:    []string{"五言绝句", "七言绝句", "五言绝句"},
			expectError:   false,
			expectedCount: 3,
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

				// 返回的 ID 顺序应与输入名称一致
				if len(tt.inputNames) > 0 {
					for i, name := range tt.inputNames {
						// 反查该 ID 应能得到相同的名称
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

	// 借助 ON CONFLICT 写入测试用的体裁，规避唯一约束冲突
	types := []string{"五言绝句", "七言绝句", "五言律诗"}
	for _, typeName := range types {
		poetryType := PoetryType{Name: typeName}
		err := db.Table(repo.poetryTypesTable()).
			Clauses(clause.OnConflict{DoNothing: true}).
			Create(&poetryType).Error
		require.NoError(t, err)
	}

	// 首次调用应填充缓存
	ids1, err := cachedRepo.GetPoetryTypeIDs(types)
	require.NoError(t, err)
	assert.Len(t, ids1, 3)

	// 第二次调用应命中缓存
	ids2, err := cachedRepo.GetPoetryTypeIDs(types)
	require.NoError(t, err)
	assert.Equal(t, ids1, ids2)

	// 部分命中缓存
	partialTypes := []string{"五言绝句", "七言绝句"} // These should be in cache
	ids3, err := cachedRepo.GetPoetryTypeIDs(partialTypes)
	require.NoError(t, err)
	assert.Len(t, ids3, 2)
	assert.Equal(t, ids1[0], ids3[0])
	assert.Equal(t, ids1[1], ids3[1])

	// 校验缓存统计
	stats := cachedRepo.GetCacheStats()
	assert.Equal(t, 3, stats["types"])
}

// TestBatchInsertPoemsUniqueIncludesAuthor 唯一索引含作者：同标题同正文、作者不同的（重出诗）各自保留，同作者的只保留一条。
func TestBatchInsertPoemsUniqueIncludesAuthor(t *testing.T) {
	_, repo := setupGetPoetryTypeIDsTestDB(t)

	dynastyID, err := repo.GetOrCreateDynasty("唐")
	require.NoError(t, err)
	a1, err := repo.GetOrCreateAuthor("甲", dynastyID)
	require.NoError(t, err)
	a2, err := repo.GetOrCreateAuthor("乙", dynastyID)
	require.NoError(t, err)

	content := []byte(`["同一首诗。"]`)
	poems := []*Poem{
		{ID: 1, Title: "题", Content: content, ContentHash: "h", AuthorID: &a1},
		{ID: 2, Title: "题", Content: content, ContentHash: "h", AuthorID: &a2}, // 不同作者：保留
		{ID: 3, Title: "题", Content: content, ContentHash: "h", AuthorID: &a1}, // 同作者重复：跳过
	}
	require.NoError(t, repo.BatchInsertPoems(poems, 10))

	var count int64
	require.NoError(t, repo.db.Table(repo.poemsTable()).Count(&count).Error)
	assert.Equal(t, int64(2), count)
}
