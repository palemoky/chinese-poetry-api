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

	// Create in-memory database
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

	// Create in-memory database
	gormDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// Auto migrate
	err = gormDB.AutoMigrate(
		&database.Dynasty{},
		&database.Author{},
		&database.PoetryType{},
		&database.Poem{},
	)
	require.NoError(t, err)

	db := &database.DB{DB: gormDB}
	repo := database.NewRepository(db)

	// Create some test data
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
				assert.Contains(t, resp, "total_poems")
				assert.Contains(t, resp, "total_authors")
				assert.Contains(t, resp, "total_dynasties")
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
