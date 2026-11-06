package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/palemoky/chinese-poetry-api/internal/database"
)

func TestHealthHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// 创建内存数据库
	gormDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	db := &database.DB{DB: gormDB}

	router := gin.New()
	router.GET("/health", HealthHandler(db))

	tests := []struct {
		name           string
		expectedStatus int
		checkResponse  func(*testing.T, map[string]any)
	}{
		{
			name:           "healthy database",
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, resp map[string]any) {
				assert.Equal(t, "healthy", resp["status"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/health", nil)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.checkResponse != nil {
				var response map[string]any
				err := json.Unmarshal(w.Body.Bytes(), &response)
				require.NoError(t, err)
				tt.checkResponse(t, response)
			}
		})
	}
}

func TestStatsHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// 创建内存数据库
	gormDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	db := &database.DB{DB: gormDB}

	// 用 Migrate 建出各语言变体的表
	err = db.Migrate()
	require.NoError(t, err)

	repo := database.NewRepository(db)

	// 写入若干测试数据
	dynastyID, _ := repo.GetOrCreateDynasty("唐")
	_, _ = repo.GetOrCreateAuthor("李白", dynastyID)

	router := gin.New()
	router.GET("/stats", StatsHandler(repo))

	tests := []struct {
		name           string
		expectedStatus int
		checkResponse  func(*testing.T, map[string]any)
	}{
		{
			name:           "get statistics",
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, resp map[string]any) {
				data, ok := resp["data"].(map[string]any)
				require.True(t, ok, "stats must be wrapped in data")
				assert.Contains(t, data, "total_poems")
				assert.Contains(t, data, "total_authors")
				assert.Contains(t, data, "total_dynasties")

				// 朝代与体裁的格式与列表接口相同，不带 created_at
				for _, key := range []string{"poems_by_dynasty", "poems_by_type"} {
					items, ok := data[key].([]any)
					require.True(t, ok, key)
					require.NotEmpty(t, items, key)
					for _, item := range items {
						assert.NotContains(t, item, "created_at", key)
					}
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/stats", nil)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.checkResponse != nil {
				var response map[string]any
				err := json.Unmarshal(w.Body.Bytes(), &response)
				require.NoError(t, err)
				tt.checkResponse(t, response)
			}
		})
	}
}
