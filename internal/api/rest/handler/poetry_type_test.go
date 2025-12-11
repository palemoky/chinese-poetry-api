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

func setupPoetryTypeTestRouter(t *testing.T) (*gin.Engine, *database.Repository) {
	gin.SetMode(gin.TestMode)

	gormDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	err = gormDB.AutoMigrate(
		&database.Dynasty{},
		&database.Author{},
		&database.PoetryType{},
		&database.Poem{},
	)
	require.NoError(t, err)

	db := &database.DB{DB: gormDB}
	repo := database.NewRepository(db)

	router := gin.New()
	return router, repo
}

func TestListPoetryTypes(t *testing.T) {
	router, repo := setupPoetryTypeTestRouter(t)
	handler := NewPoetryTypeHandler(repo)

	// Create test data - need to create types directly
	// Note: We can't access db directly, so we'll just test the endpoint

	router.GET("/types", handler.ListPoetryTypes)

	req := httptest.NewRequest(http.MethodGet, "/types", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	// Response should have data field
	assert.Contains(t, response, "data")
}

func TestGetPoetryType(t *testing.T) {
	router, repo := setupPoetryTypeTestRouter(t)
	handler := NewPoetryTypeHandler(repo)

	router.GET("/types/:id", handler.GetPoetryType)

	tests := []struct {
		name           string
		typeID         string
		expectedStatus int
	}{
		{
			name:           "get non-existent type",
			typeID:         "999999",
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "invalid type ID",
			typeID:         "invalid",
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/types/"+tt.typeID, nil)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}
