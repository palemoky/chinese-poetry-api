package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/palemoky/chinese-poetry-api/internal/database"
)

func setupDynastyTestRouter(t *testing.T) (*gin.Engine, *database.Repository) {
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

func TestListDynasties(t *testing.T) {
	router, repo := setupDynastyTestRouter(t)
	handler := NewDynastyHandler(repo)

	// Create test data
	_, _ = repo.GetOrCreateDynasty("唐")
	_, _ = repo.GetOrCreateDynasty("宋")

	router.GET("/dynasties", handler.ListDynasties)

	req := httptest.NewRequest(http.MethodGet, "/dynasties", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	data := response["data"].([]interface{})
	assert.GreaterOrEqual(t, len(data), 2)
}

func TestGetDynasty(t *testing.T) {
	router, repo := setupDynastyTestRouter(t)
	handler := NewDynastyHandler(repo)

	dynastyID, _ := repo.GetOrCreateDynasty("唐")

	router.GET("/dynasties/:id", handler.GetDynasty)

	tests := []struct {
		name           string
		dynastyID      string
		expectedStatus int
	}{
		{
			name:           "get existing dynasty",
			dynastyID:      strconv.FormatInt(dynastyID, 10),
			expectedStatus: http.StatusOK,
		},
		{
			name:           "get non-existent dynasty",
			dynastyID:      "999999",
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "invalid dynasty ID",
			dynastyID:      "invalid",
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/dynasties/"+tt.dynastyID, nil)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}

func TestGetDynastyPoems(t *testing.T) {
	router, repo := setupDynastyTestRouter(t)
	handler := NewDynastyHandler(repo)

	dynastyID, _ := repo.GetOrCreateDynasty("唐")

	router.GET("/dynasties/:id/poems", handler.GetDynastyPoems)

	tests := []struct {
		name           string
		dynastyID      string
		query          string
		expectedStatus int
	}{
		{
			name:           "get dynasty poems",
			dynastyID:      strconv.FormatInt(dynastyID, 10),
			query:          "",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "with pagination",
			dynastyID:      strconv.FormatInt(dynastyID, 10),
			query:          "?page=1&page_size=10",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "invalid dynasty ID",
			dynastyID:      "invalid",
			query:          "",
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(
				http.MethodGet,
				"/dynasties/"+tt.dynastyID+"/poems"+tt.query,
				nil,
			)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}
