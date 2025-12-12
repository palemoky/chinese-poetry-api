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
func setupTestRouter(t *testing.T) (*gin.Engine, *database.Repository, *gorm.DB) {
	gin.SetMode(gin.TestMode)

	// Create in-memory database
	gormDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	db := &database.DB{DB: gormDB}

	// Use Migrate() to create language-specific tables
	err = db.Migrate()
	require.NoError(t, err)

	repo := database.NewRepository(db)

	router := gin.New()
	return router, repo, gormDB
}

func TestListAuthors(t *testing.T) {
	router, repo, _ := setupTestRouter(t)
	handler := NewAuthorHandler(repo)

	// Create test data
	dynastyID, _ := repo.GetOrCreateDynasty("唐")
	_, _ = repo.GetOrCreateAuthor("李白", dynastyID)
	_, _ = repo.GetOrCreateAuthor("杜甫", dynastyID)

	router.GET("/authors", handler.ListAuthors)

	tests := []struct {
		name           string
		query          string
		expectedStatus int
		checkResponse  func(*testing.T, map[string]any)
	}{
		{
			name:           "default pagination",
			query:          "",
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, resp map[string]any) {
				data := resp["data"].([]any)
				assert.GreaterOrEqual(t, len(data), 2)

				pagination := resp["pagination"].(map[string]any)
				assert.Equal(t, float64(1), pagination["page"])
				assert.Equal(t, float64(20), pagination["page_size"])
				assert.Equal(t, float64(2), pagination["total"])
			},
		},
		{
			name:           "custom page size",
			query:          "?page=1&page_size=1",
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, resp map[string]any) {
				data := resp["data"].([]any)
				assert.Len(t, data, 1)

				pagination := resp["pagination"].(map[string]any)
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
				var response map[string]any
				err := json.Unmarshal(w.Body.Bytes(), &response)
				require.NoError(t, err)
				tt.checkResponse(t, response)
			}
		})
	}
}

func TestGetAuthor(t *testing.T) {
	router, repo, _ := setupTestRouter(t)
	handler := NewAuthorHandler(repo)

	// Create test data
	dynastyID, _ := repo.GetOrCreateDynasty("唐")
	authorID, _ := repo.GetOrCreateAuthor("李白", dynastyID)

	router.GET("/authors/:id", handler.GetAuthor)

	tests := []struct {
		name           string
		authorID       string
		expectedStatus int
		checkResponse  func(*testing.T, map[string]any)
	}{
		{
			name:           "get existing author",
			authorID:       strconv.FormatInt(authorID, 10),
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, resp map[string]any) {
				data := resp["data"].(map[string]any)
				assert.NotNil(t, data)
				assert.Equal(t, "李白", data["name"])
				assert.Equal(t, "唐", data["dynasty"])
				// Ensure ID is present
				assert.NotNil(t, data["id"])
			},
		},
		{
			name:           "get non-existent author",
			authorID:       "999999",
			expectedStatus: http.StatusNotFound,
			checkResponse: func(t *testing.T, resp map[string]any) {
				assert.Equal(t, "Author not found", resp["error"])
			},
		},
		{
			name:           "invalid author ID",
			authorID:       "invalid",
			expectedStatus: http.StatusBadRequest,
			checkResponse: func(t *testing.T, resp map[string]any) {
				assert.Equal(t, "Invalid author ID", resp["error"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/authors/"+tt.authorID, nil)
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

// TestPaginationBoundariesREST tests edge cases in REST API pagination
func TestPaginationBoundariesREST(t *testing.T) {
	router, repo, _ := setupTestRouter(t)
	handler := NewAuthorHandler(repo)

	// Create test data - 5 authors
	dynastyID, _ := repo.GetOrCreateDynasty("唐")
	for _, name := range []string{"李白", "杜甫", "白居易", "王维", "孟浩然"} {
		_, _ = repo.GetOrCreateAuthor(name, dynastyID)
	}

	router.GET("/authors", handler.ListAuthors)

	tests := []struct {
		name           string
		query          string
		expectedStatus int
		checkResponse  func(*testing.T, map[string]any)
	}{
		{
			name:           "page_size 0 defaults to 20",
			query:          "?page_size=0",
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, resp map[string]any) {
				pagination := resp["pagination"].(map[string]any)
				assert.Equal(t, float64(20), pagination["page_size"])
			},
		},
		{
			name:           "page 0 defaults to 1",
			query:          "?page=0",
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, resp map[string]any) {
				pagination := resp["pagination"].(map[string]any)
				assert.Equal(t, float64(1), pagination["page"])
			},
		},
		{
			name:           "negative page defaults to 1",
			query:          "?page=-1",
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, resp map[string]any) {
				pagination := resp["pagination"].(map[string]any)
				assert.Equal(t, float64(1), pagination["page"])
			},
		},
		{
			name:           "very large page_size is capped at 100",
			query:          "?page_size=500",
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, resp map[string]any) {
				pagination := resp["pagination"].(map[string]any)
				assert.Equal(t, float64(100), pagination["page_size"])
			},
		},
		{
			name:           "total_pages is calculated correctly",
			query:          "?page_size=2",
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, resp map[string]any) {
				pagination := resp["pagination"].(map[string]any)
				total := pagination["total"].(float64)
				pageSize := pagination["page_size"].(float64)
				totalPages := pagination["total_pages"].(float64)
				expectedTotalPages := (int(total) + int(pageSize) - 1) / int(pageSize)
				assert.Equal(t, float64(expectedTotalPages), totalPages)
			},
		},
		{
			name:           "page beyond total returns empty data",
			query:          "?page=100&page_size=10",
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, resp map[string]any) {
				data := resp["data"].([]any)
				assert.Empty(t, data)
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
				var response map[string]any
				err := json.Unmarshal(w.Body.Bytes(), &response)
				require.NoError(t, err)
				tt.checkResponse(t, response)
			}
		})
	}
}
