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
	searchEngine := search.NewEngine(db)

	router := gin.New()
	return router, repo, searchEngine
}

// createTestPoem creates a test poem in the database
func createTestPoem(t *testing.T, repo *database.Repository, title, content string) *database.Poem {
	// Create dynasty and author first
	dynastyID, err := repo.GetOrCreateDynasty("唐")
	require.NoError(t, err)

	authorID, err := repo.GetOrCreateAuthor("李白", "li bai", "lb", dynastyID)
	require.NoError(t, err)

	// Create poem
	poem := &database.Poem{
		ID:          12345678901234,
		Title:       title,
		TitlePinyin: stringPtr("jing ye si"),
		Content:     datatypes.JSON([]byte(`["床前明月光","疑是地上霜","举头望明月","低头思故乡"]`)),
		AuthorID:    &authorID,
		DynastyID:   &dynastyID,
	}
	err = repo.InsertPoem(poem)
	require.NoError(t, err)

	return poem
}

func stringPtr(s string) *string {
	return &s
}

func TestGetPoem(t *testing.T) {
	router, repo, searchEngine := setupPoemTestRouter(t)
	handler := NewPoemHandler(repo, searchEngine)

	// Create test poem
	poem := createTestPoem(t, repo, "静夜思", "test content")

	router.GET("/poems/:id", handler.GetPoem)

	tests := []struct {
		name           string
		poemID         string
		expectedStatus int
		checkResponse  func(*testing.T, map[string]any)
	}{
		{
			name:           "get existing poem",
			poemID:         "12345678901234",
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, resp map[string]any) {
				assert.Equal(t, "静夜思", resp["title"])
				paragraphs := resp["paragraphs"].([]any)
				assert.Len(t, paragraphs, 4)
				assert.Equal(t, "床前明月光", paragraphs[0])
			},
		},
		{
			name:           "get non-existent poem",
			poemID:         "99999999999999",
			expectedStatus: http.StatusNotFound,
			checkResponse: func(t *testing.T, resp map[string]any) {
				assert.Equal(t, "poem not found", resp["error"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/poems/"+tt.poemID, nil)
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

	_ = poem // use poem to avoid unused variable warning
}

func TestSearchPoems(t *testing.T) {
	router, repo, searchEngine := setupPoemTestRouter(t)
	handler := NewPoemHandler(repo, searchEngine)

	// Create test poems
	createTestPoem(t, repo, "静夜思", "test content")

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
				assert.Contains(t, resp, "poems")
				assert.Contains(t, resp, "total_count")
				assert.Contains(t, resp, "page")
				assert.Contains(t, resp, "page_size")
				assert.Contains(t, resp, "has_more")
			},
		},
		{
			name:           "search with type parameter",
			query:          "?q=李白&type=author",
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, resp map[string]any) {
				assert.Contains(t, resp, "poems")
			},
		},
		{
			name:           "search with pagination",
			query:          "?q=test&page=1&page_size=10",
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, resp map[string]any) {
				assert.Equal(t, float64(1), resp["page"])
				assert.Equal(t, float64(10), resp["page_size"])
			},
		},
		{
			name:           "search with pinyin",
			query:          "?q=jingye&type=pinyin",
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, resp map[string]any) {
				assert.Contains(t, resp, "poems")
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
				assert.Equal(t, float64(100), resp["page_size"])
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
		setupData      bool
		expectedStatus int
	}{
		{
			name:           "get random poem when database is empty",
			setupData:      false,
			expectedStatus: http.StatusInternalServerError,
		},
		{
			name:           "get random poem with data",
			setupData:      true,
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create fresh router for each test to avoid data pollution
			router, repo, searchEngine := setupPoemTestRouter(t)
			handler := NewPoemHandler(repo, searchEngine)
			router.GET("/random", handler.RandomPoem)

			if tt.setupData {
				createTestPoem(t, repo, "静夜思", "test content")
			}

			req := httptest.NewRequest(http.MethodGet, "/random", nil)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}
