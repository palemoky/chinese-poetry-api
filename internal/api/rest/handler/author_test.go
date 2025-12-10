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

// setupTestRouter creates a test router with a test database
func setupTestRouter(t *testing.T) (*gin.Engine, *database.Repository) {
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

	router := gin.New()
	return router, repo
}

func TestListAuthors(t *testing.T) {
	router, repo := setupTestRouter(t)
	handler := NewAuthorHandler(repo)

	// Create test data
	dynastyID, _ := repo.GetOrCreateDynasty("唐")
	_, _ = repo.GetOrCreateAuthor("李白", "li bai", "lb", dynastyID)
	_, _ = repo.GetOrCreateAuthor("杜甫", "du fu", "df", dynastyID)

	router.GET("/authors", handler.ListAuthors)

	tests := []struct {
		name           string
		query          string
		expectedStatus int
		checkResponse  func(*testing.T, map[string]interface{})
	}{
		{
			name:           "default pagination",
			query:          "",
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, resp map[string]interface{}) {
				data := resp["data"].([]interface{})
				assert.GreaterOrEqual(t, len(data), 2)

				pagination := resp["pagination"].(map[string]interface{})
				assert.Equal(t, float64(1), pagination["page"])
				assert.Equal(t, float64(20), pagination["page_size"])
			},
		},
		{
			name:           "custom page size",
			query:          "?page=1&page_size=1",
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, resp map[string]interface{}) {
				data := resp["data"].([]interface{})
				assert.Len(t, data, 1)

				pagination := resp["pagination"].(map[string]interface{})
				assert.Equal(t, float64(1), pagination["page_size"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/authors"+tt.query, nil)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.checkResponse != nil {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				require.NoError(t, err)
				tt.checkResponse(t, response)
			}
		})
	}
}

func TestGetAuthor(t *testing.T) {
	router, repo := setupTestRouter(t)
	handler := NewAuthorHandler(repo)

	// Create test data
	dynastyID, _ := repo.GetOrCreateDynasty("唐")
	authorID, _ := repo.GetOrCreateAuthor("李白", "li bai", "lb", dynastyID)

	router.GET("/authors/:id", handler.GetAuthor)

	tests := []struct {
		name           string
		authorID       string
		expectedStatus int
		checkResponse  func(*testing.T, map[string]interface{})
	}{
		{
			name:           "get existing author",
			authorID:       strconv.FormatInt(authorID, 10),
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, resp map[string]interface{}) {
				data := resp["data"].(map[string]interface{})
				assert.NotNil(t, data)
			},
		},
		{
			name:           "get non-existent author",
			authorID:       "999999",
			expectedStatus: http.StatusNotFound,
			checkResponse:  nil,
		},
		{
			name:           "invalid author ID",
			authorID:       "invalid",
			expectedStatus: http.StatusBadRequest,
			checkResponse:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/authors/"+tt.authorID, nil)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.checkResponse != nil {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				require.NoError(t, err)
				tt.checkResponse(t, response)
			}
		})
	}
}

func TestGetAuthorPoems(t *testing.T) {
	router, repo := setupTestRouter(t)
	handler := NewAuthorHandler(repo)

	// Create test data
	dynastyID, _ := repo.GetOrCreateDynasty("唐")
	authorID, _ := repo.GetOrCreateAuthor("李白", "li bai", "lb", dynastyID)

	router.GET("/authors/:id/poems", handler.GetAuthorPoems)

	tests := []struct {
		name           string
		authorID       string
		query          string
		expectedStatus int
	}{
		{
			name:           "get author poems",
			authorID:       strconv.FormatInt(authorID, 10),
			query:          "",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "with pagination",
			authorID:       strconv.FormatInt(authorID, 10),
			query:          "?page=1&page_size=10",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "invalid author ID",
			authorID:       "invalid",
			query:          "",
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(
				http.MethodGet,
				"/authors/"+tt.authorID+"/poems"+tt.query,
				nil,
			)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}
