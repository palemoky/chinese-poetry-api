package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/palemoky/chinese-poetry-api/internal/database"
	"github.com/palemoky/chinese-poetry-api/internal/search"
)

// setupPoemTestRouter creates a test router with database and search engine
func setupPoemTestRouter(t *testing.T) (*gin.Engine, *database.Repository, *search.Engine) {
	gin.SetMode(gin.TestMode)

	// Create in-memory database
	gormDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	db := &database.DB{DB: gormDB}

	// Use Migrate() to create language-specific tables
	err = db.Migrate()
	require.NoError(t, err)

	repo := database.NewRepository(db)
	searchEngine := search.NewEngine(db)

	router := gin.New()
	return router, repo, searchEngine
}

// createTestPoem creates a test poem in the database
func createTestPoem(t *testing.T, repo *database.Repository, id int64, title, content string) *database.Poem {
	// Create dynasty and author first
	dynastyID, err := repo.GetOrCreateDynasty("唐")
	require.NoError(t, err)

	authorID, err := repo.GetOrCreateAuthor("李白", dynastyID)
	require.NoError(t, err)

	// Create poem
	poem := &database.Poem{
		ID:        id,
		Title:     title,
		Content:   datatypes.JSON([]byte(`["床前明月光","疑是地上霜","举头望明月","低头思故乡"]`)),
		AuthorID:  &authorID,
		DynastyID: &dynastyID,
	}
	err = repo.InsertPoem(poem)
	require.NoError(t, err)

	return poem
}

func TestListPoems(t *testing.T) {
	router, repo, searchEngine := setupPoemTestRouter(t)
	handler := NewPoemHandler(repo, searchEngine)

	// Create test poems
	createTestPoem(t, repo, 12345678901234, "静夜思", "test content")
	createTestPoem(t, repo, 12345678901235, "春晓", "test content 2")

	router.GET("/poems", handler.ListPoems)

	tests := []struct {
		name           string
		query          string
		expectedStatus int
		checkResponse  func(*testing.T, map[string]any)
	}{
		{
			name:           "list poems default pagination",
			query:          "",
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, resp map[string]any) {
				data := resp["data"].([]any)
				assert.Len(t, data, 2)

				pagination := resp["pagination"].(map[string]any)
				assert.Equal(t, float64(1), pagination["page"])
				assert.Equal(t, float64(20), pagination["page_size"])
				assert.Equal(t, float64(2), pagination["total"])

				// Check nested structure of first poem
				poem := data[0].(map[string]any)
				assert.NotEmpty(t, poem["title"])
				assert.NotEmpty(t, poem["content"])

				assert.NotNil(t, poem["author"])
				author := poem["author"].(map[string]any)
				assert.Equal(t, "李白", author["name"])

				assert.NotNil(t, poem["dynasty"])
				dynasty := poem["dynasty"].(map[string]any)
				assert.Equal(t, "唐", dynasty["name"])
			},
		},
		{
			name:           "list poems with pagination",
			query:          "?page=1&page_size=1",
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, resp map[string]any) {
				data := resp["data"].([]any)
				assert.Len(t, data, 1) // Should only return 1

				pagination := resp["pagination"].(map[string]any)
				assert.Equal(t, float64(1), pagination["page"])
				assert.Equal(t, float64(1), pagination["page_size"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/poems"+tt.query, nil)
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

func TestSearchPoems(t *testing.T) {
	router, repo, searchEngine := setupPoemTestRouter(t)
	handler := NewPoemHandler(repo, searchEngine)

	// Create test poems
	createTestPoem(t, repo, 12345678901234, "静夜思", "test content")

	router.GET("/poems/search", handler.SearchPoems)

	tests := []struct {
		name           string
		query          string
		expectedStatus int
		checkResponse  func(*testing.T, map[string]any)
	}{
		{
			name:           "search with query",
			query:          "?q=静夜思",
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, resp map[string]any) {
				assert.Contains(t, resp, "data")
				assert.Contains(t, resp, "pagination")

				pagination := resp["pagination"].(map[string]any)
				assert.Contains(t, pagination, "total")
				assert.Contains(t, pagination, "page")
				assert.Contains(t, pagination, "page_size")
			},
		},
		{
			name:           "search with type parameter",
			query:          "?q=李白&type=author",
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, resp map[string]any) {
				assert.Contains(t, resp, "data")
			},
		},
		{
			name:           "search with pagination",
			query:          "?q=test&page=1&page_size=10",
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, resp map[string]any) {
				pagination := resp["pagination"].(map[string]any)
				assert.Equal(t, float64(1), pagination["page"])
				assert.Equal(t, float64(10), pagination["page_size"])
			},
		},
		{
			name:           "search without query parameter",
			query:          "",
			expectedStatus: http.StatusBadRequest,
			checkResponse: func(t *testing.T, resp map[string]any) {
				assert.Equal(t, "query parameter 'q' is required", resp["error"])
			},
		},
		{
			name:           "page_size exceeds limit",
			query:          "?q=test&page_size=200",
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, resp map[string]any) {
				// Should be capped at 100
				pagination := resp["pagination"].(map[string]any)
				assert.Equal(t, float64(100), pagination["page_size"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/poems/search"+tt.query, nil)
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

func TestRandomPoem(t *testing.T) {
	router, repo, searchEngine := setupPoemTestRouter(t)
	handler := NewPoemHandler(repo, searchEngine)

	router.GET("/random", handler.RandomPoem)

	tests := []struct {
		name           string
		query          string
		setupData      bool
		expectedStatus int
		checkResponse  func(*testing.T, map[string]any)
	}{
		{
			name:           "get random poem when database is empty",
			query:          "",
			setupData:      false,
			expectedStatus: http.StatusNotFound,
			checkResponse: func(t *testing.T, resp map[string]any) {
				assert.Equal(t, "no poems found matching the criteria", resp["error"])
			},
		},
		{
			name:           "get random poem with data",
			query:          "",
			setupData:      true,
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, resp map[string]any) {
				assert.NotEmpty(t, resp["title"])
				assert.NotEmpty(t, resp["content"])

				assert.NotNil(t, resp["author"])
				author := resp["author"].(map[string]any)
				assert.Equal(t, "李白", author["name"])

				assert.NotNil(t, resp["dynasty"])
				dynasty := resp["dynasty"].(map[string]any)
				assert.Equal(t, "唐", dynasty["name"])
			},
		},
		{
			name:           "get random poem with author filter",
			query:          "?author=李白",
			setupData:      true,
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, resp map[string]any) {
				assert.NotEmpty(t, resp["title"])
				author := resp["author"].(map[string]any)
				assert.Equal(t, "李白", author["name"])
			},
		},
		{
			name:           "get random poem with non-existent author filter",
			query:          "?author=不存在的作者",
			setupData:      true,
			expectedStatus: http.StatusNotFound,
			checkResponse: func(t *testing.T, resp map[string]any) {
				assert.Equal(t, "author not found", resp["error"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create fresh router for each test to avoid data pollution
			router, repo, searchEngine := setupPoemTestRouter(t)
			handler := NewPoemHandler(repo, searchEngine)
			router.GET("/random", handler.RandomPoem)

			if tt.setupData {
				createTestPoem(t, repo, 12345678901234, "静夜思", "test content")
			}

			req := httptest.NewRequest(http.MethodGet, "/random"+tt.query, nil)
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
