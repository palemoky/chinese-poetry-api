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
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/palemoky/chinese-poetry-api/internal/database"
)

// setupPoemTestRouter 创建带数据库的测试路由。
func setupPoemTestRouter(t *testing.T) (*gin.Engine, *database.Repository) {
	gin.SetMode(gin.TestMode)

	// 创建内存数据库
	gormDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	db := &database.DB{DB: gormDB}

	// 用 Migrate 建出各语言变体的表
	err = db.Migrate()
	require.NoError(t, err)

	repo := database.NewRepository(db)

	router := gin.New()
	return router, repo
}

// createTestPoem 向数据库写入一首测试诗词。
func createTestPoem(t *testing.T, repo *database.Repository, id int64, title, content string) *database.Poem {
	// 先写入朝代与作者
	dynastyID, err := repo.GetOrCreateDynasty("唐")
	require.NoError(t, err)

	authorID, err := repo.GetOrCreateAuthor("李白", dynastyID)
	require.NoError(t, err)

	// 写入诗词
	contentJSON, err := json.Marshal([]string{content})
	require.NoError(t, err)

	poem := &database.Poem{
		ID:        id,
		Title:     title,
		Content:   datatypes.JSON(contentJSON),
		AuthorID:  &authorID,
		DynastyID: &dynastyID,
	}
	err = repo.InsertPoem(poem)
	require.NoError(t, err)

	return poem
}

func TestListPoems(t *testing.T) {
	router, repo := setupPoemTestRouter(t)
	handler := NewPoemHandler(repo)

	// 写入测试诗词
	createTestPoem(t, repo, 1, "静夜思", "test content")
	createTestPoem(t, repo, 2, "春晓", "test content 2")

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

				// 校验首条结果的嵌套结构
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
				assert.Len(t, data, 1) // 应只返回一条

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

// TestListPoemsFilters 覆盖 /poems 上的各项过滤条件——它们此前来者不拒又一概忽略：
// dynasty_id=6、dynasty=唐、dynastyId=6 都会在未过滤的全量语料上返回 200，
// 客户端根本无从察觉自己把参数写错了。
func TestListPoemsFilters(t *testing.T) {
	router, repo := setupPoemTestRouter(t)
	handler := NewPoemHandler(repo)

	tangID, err := repo.GetOrCreateDynasty("唐")
	require.NoError(t, err)
	songID, err := repo.GetOrCreateDynasty("宋")
	require.NoError(t, err)
	libaiID, err := repo.GetOrCreateAuthor("李白", tangID)
	require.NoError(t, err)
	sushiID, err := repo.GetOrCreateAuthor("苏轼", songID)
	require.NoError(t, err)

	// Poetry types are pre-seeded by Migrate; 11 and 12 are 五言绝句/七言绝句.
	jueju5, jueju7 := int64(11), int64(12)
	poems := []*database.Poem{
		{ID: 1, Title: "静夜思", AuthorID: &libaiID, DynastyID: &tangID, TypeID: &jueju5},
		{ID: 2, Title: "早发白帝城", AuthorID: &libaiID, DynastyID: &tangID, TypeID: &jueju7},
		{ID: 3, Title: "题西林壁", AuthorID: &sushiID, DynastyID: &songID, TypeID: &jueju7},
	}
	for _, p := range poems {
		p.Content = datatypes.JSON([]byte(`["内容"]`))
		require.NoError(t, repo.InsertPoem(p))
	}

	router.GET("/poems", handler.ListPoems)

	// titles 发起一次请求并返回其命中的标题，
	// 使每个用例都能断言「返回了哪些诗」，而不只是断言状态码。
	titles := func(t *testing.T, query string, wantStatus int) []string {
		req := httptest.NewRequest(http.MethodGet, "/poems"+query, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, wantStatus, w.Code, "body: %s", w.Body.String())

		if wantStatus != http.StatusOK {
			return nil
		}

		var resp struct {
			Data []struct {
				Title string `json:"title"`
			} `json:"data"`
			Pagination struct {
				Total int `json:"total"`
			} `json:"pagination"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

		got := make([]string, len(resp.Data))
		for i, d := range resp.Data {
			got[i] = d.Title
		}
		// 返回的总数应是过滤后的数量，而非全量语料的数量
		assert.Equal(t, len(got), resp.Pagination.Total)
		return got
	}

	t.Run("filters actually filter", func(t *testing.T) {
		assert.Equal(t, []string{"静夜思", "早发白帝城", "题西林壁"}, titles(t, "", http.StatusOK))
		assert.Equal(t, []string{"静夜思", "早发白帝城"}, titles(t, "?dynasty_id="+strconv.FormatInt(tangID, 10), http.StatusOK))
		assert.Equal(t, []string{"静夜思", "早发白帝城"}, titles(t, "?dynasty=唐", http.StatusOK))
		assert.Equal(t, []string{"题西林壁"}, titles(t, "?author=苏轼", http.StatusOK))
		assert.Equal(t, []string{"静夜思"}, titles(t, "?author=李白&type_id=11", http.StatusOK))
	})

	t.Run("repeated type_id is combined with OR", func(t *testing.T) {
		assert.Equal(t, []string{"静夜思", "早发白帝城", "题西林壁"}, titles(t, "?type_id=11&type_id=12", http.StatusOK))
		assert.Equal(t, []string{"早发白帝城", "题西林壁"}, titles(t, "?type_id=12", http.StatusOK))
		assert.Equal(t, []string{"静夜思", "早发白帝城", "题西林壁"}, titles(t, "?type=五言绝句&type=七言绝句", http.StatusOK))
	})

	t.Run("a repeated type is not an error", func(t *testing.T) {
		// 同一名称传两次，在把名称解析为 ID 的 IN 子句中会合并成一行，
		assert.Equal(t, []string{"静夜思"}, titles(t, "?type=五言绝句&type=五言绝句", http.StatusOK))
		assert.Equal(t, []string{"静夜思"}, titles(t, "?type_id=11&type_id=11", http.StatusOK))
	})

	t.Run("misspelled and malformed filters are rejected", func(t *testing.T) {
		// 典型场景是把 GraphQL 的参数拼写误用到了 REST 上
		titles(t, "?dynastyId=6", http.StatusBadRequest)
		titles(t, "?dynasty_ids=6", http.StatusBadRequest)
		titles(t, "?dynasty_id=abc", http.StatusBadRequest)
		titles(t, "?type_id=abc", http.StatusBadRequest)
		titles(t, "?author_id=1.5", http.StatusBadRequest)
		titles(t, "?lang=en", http.StatusBadRequest)
	})

	t.Run("unknown filter values are 404, not a silent full listing", func(t *testing.T) {
		titles(t, "?dynasty=不存在的朝代", http.StatusNotFound)
		titles(t, "?author=不存在的作者", http.StatusNotFound)
	})

	t.Run("a known filter value with no poems is an empty page, not a 404", func(t *testing.T) {
		// 元 is seeded by the schema but has no poems in this fixture.
		assert.Empty(t, titles(t, "?dynasty=元", http.StatusOK))
	})
}

func TestSearchPoems(t *testing.T) {
	router, repo := setupPoemTestRouter(t)
	handler := NewPoemHandler(repo)

	// 写入测试诗词
	createTestPoem(t, repo, 1, "静夜思", "test content")

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
			expectedStatus: http.StatusBadRequest,
			checkResponse: func(t *testing.T, resp map[string]any) {
				assert.Contains(t, resp["error"], "page_size")
			},
		},
		{
			// 未知的搜索类型曾会静默落到 "all"，于是一个拼写错误会搜索全部字段，
			// 看上去却像是一次正常的定向搜索。
			name:           "unknown search type is rejected",
			query:          "?q=李白&type=titel",
			expectedStatus: http.StatusBadRequest,
			checkResponse: func(t *testing.T, resp map[string]any) {
				assert.Contains(t, resp["error"], "titel")
			},
		},
		{
			name:           "unknown query parameter is rejected",
			query:          "?q=李白&searchType=author",
			expectedStatus: http.StatusBadRequest,
			checkResponse: func(t *testing.T, resp map[string]any) {
				assert.Contains(t, resp["error"], "searchType")
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
	router, repo := setupPoemTestRouter(t)
	handler := NewPoemHandler(repo)

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
			// 每个用例都新建路由，避免数据相互污染
			router, repo := setupPoemTestRouter(t)
			handler := NewPoemHandler(repo)
			router.GET("/random", handler.RandomPoem)

			if tt.setupData {
				createTestPoem(t, repo, 1, "静夜思", "test content")
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

// 数据库故障要返回 500：此前一律回 404「没有符合条件的诗」，
// 服务端故障在客户端看来只是筛选条件太窄。
func TestRandomPoemDatabaseErrorIs500(t *testing.T) {
	router, repo := setupPoemTestRouter(t)
	createTestPoem(t, repo, 1, "静夜思", "床前明月光")

	h := NewPoemHandler(repo)
	router.GET("/poems/random", h.RandomPoem)

	// 先确认正常情况下能取到
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/poems/random?char=床", nil))
	require.Equal(t, http.StatusOK, w.Code)

	w = httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/poems/random?char=雪", nil))
	require.Equal(t, http.StatusNotFound, w.Code)

	sqlDB, err := repo.DB().DB.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	for _, url := range []string{"/poems/random?author_id=1", "/poems/random?char=春"} {
		w = httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, url, nil))
		assert.Equal(t, http.StatusInternalServerError, w.Code, url)
	}
}

// 同名异人：?author= 未带朝代时取作品最多的那位，带上朝代即可指定。
func TestPoemsFilterByHomonymAuthor(t *testing.T) {
	router, repo := setupPoemTestRouter(t)
	h := NewPoemHandler(repo)
	router.GET("/poems", h.ListPoems)

	tang, err := repo.GetOrCreateDynasty("唐")
	require.NoError(t, err)
	qing, err := repo.GetOrCreateDynasty("清")
	require.NoError(t, err)
	tangAuthor, err := repo.GetOrCreateAuthor("张潮", tang)
	require.NoError(t, err)
	qingAuthor, err := repo.GetOrCreateAuthor("张潮", qing)
	require.NoError(t, err)

	insert := func(id int64, title string, author, dynasty int64) {
		require.NoError(t, repo.InsertPoem(&database.Poem{
			ID: id, Title: title, Content: datatypes.JSON([]byte(`["` + title + `"]`)),
			AuthorID: &author, DynastyID: &dynasty,
		}))
	}
	insert(1, "唐诗一", tangAuthor, tang)
	insert(2, "幽梦影一", qingAuthor, qing)
	insert(3, "幽梦影二", qingAuthor, qing)

	titles := func(url string) []string {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, url, nil))
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var resp struct {
			Data []struct{ Title string } `json:"data"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		var out []string
		for _, p := range resp.Data {
			out = append(out, p.Title)
		}
		return out
	}

	assert.Equal(t, []string{"幽梦影一", "幽梦影二"}, titles("/poems?author=张潮"), "defaults to the author with more poems")
	assert.Equal(t, []string{"唐诗一"}, titles("/poems?author=张潮&dynasty=唐"))
	assert.Equal(t, []string{"幽梦影一", "幽梦影二"}, titles("/poems?author=张潮&dynasty_id="+strconv.FormatInt(qing, 10)))
}
