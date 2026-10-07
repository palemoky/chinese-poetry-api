package integration

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/99designs/gqlgen/client"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/palemoky/chinese-poetry-api/internal/api/rest"
	"github.com/palemoky/chinese-poetry-api/internal/config"
	"github.com/palemoky/chinese-poetry-api/internal/database"
	"github.com/palemoky/chinese-poetry-api/internal/graph"
)

// setupTestEnv 搭建同时包含 REST 与 GraphQL 的测试环境。
func setupTestEnv(t *testing.T) (*gin.Engine, *client.Client, *database.Repository) {
	gin.SetMode(gin.TestMode)

	// 创建内存数据库
	gormDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	db := &database.DB{DB: gormDB}
	err = db.Migrate()
	require.NoError(t, err)

	repo := database.NewRepository(db)

	// 初始化 REST 路由
	cfg := &config.Config{
		Server: config.ServerConfig{Mode: "test"},
	}
	restRouter, err := rest.SetupRouter(cfg, db, repo)
	require.NoError(t, err)

	// 初始化 GraphQL 客户端
	resolver := graph.NewResolver(db, repo)
	srv := graph.NewServer(resolver, graph.ServerOptions{ComplexityLimit: 5000, Introspection: true})
	graphqlClient := client.New(srv)

	return restRouter, graphqlClient, repo
}

// createTestData 写入两套 API 共用的测试数据。
func createTestData(t *testing.T, repo *database.Repository) (dynastyID, authorID, typeID int64) {
	var err error

	// 写入朝代
	dynastyID, err = repo.GetOrCreateDynasty("唐")
	require.NoError(t, err)

	// 写入作者
	authorID, err = repo.GetOrCreateAuthor("李白", dynastyID)
	require.NoError(t, err)

	// 体裁由 Migrate 预置，直接取用
	typeID = 12 // 七言绝句

	// 写入诗词
	poems := []*database.Poem{
		{
			ID:        1001,
			Title:     "静夜思",
			Content:   datatypes.JSON([]byte(`["床前明月光","疑是地上霜","举头望明月","低头思故乡"]`)),
			AuthorID:  &authorID,
			DynastyID: &dynastyID,
			TypeID:    &typeID,
		},
		{
			ID:        1002,
			Title:     "将进酒",
			Content:   datatypes.JSON([]byte(`["君不见黄河之水天上来","奔流到海不复回"]`)),
			AuthorID:  &authorID,
			DynastyID: &dynastyID,
			TypeID:    &typeID,
		},
	}

	for _, poem := range poems {
		err = repo.InsertPoem(poem)
		require.NoError(t, err)
	}

	return dynastyID, authorID, typeID
}

// TestSearchConsistency 验证 REST 与 GraphQL 的搜索结果一致。
func TestSearchConsistency(t *testing.T) {
	restRouter, graphqlClient, repo := setupTestEnv(t)
	createTestData(t, repo)

	tests := []struct {
		name       string
		query      string
		searchType string
	}{
		{"search by title", "静夜思", "title"},
		{"search by content", "明月", "content"},
		{"search by author", "李白", "author"},
		{"search all", "李白", "all"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 调用 REST 接口
			restReq := httptest.NewRequest(http.MethodGet, "/api/v1/poems/search?q="+tt.query+"&type="+tt.searchType, nil)
			restResp := httptest.NewRecorder()
			restRouter.ServeHTTP(restResp, restReq)

			require.Equal(t, http.StatusOK, restResp.Code)

			var restResult struct {
				Data []struct {
					ID    int64  `json:"id"`
					Title string `json:"title"`
				} `json:"data"`
				Pagination struct {
					Total int `json:"total"`
				} `json:"pagination"`
			}
			err := json.Unmarshal(restResp.Body.Bytes(), &restResult)
			require.NoError(t, err)

			// 调用 GraphQL 接口
			var graphqlResult struct {
				SearchPoems struct {
					Edges []struct {
						Node struct {
							ID    int64
							Title string
						}
					}
					TotalCount int
				}
			}

			searchTypeGQL := "ALL"
			switch tt.searchType {
			case "title":
				searchTypeGQL = "TITLE"
			case "content":
				searchTypeGQL = "CONTENT"
			case "author":
				searchTypeGQL = "AUTHOR"
			}

			query := `query { searchPoems(query: "` + tt.query + `", searchType: ` + searchTypeGQL + `) {
				edges { node { id title } }
				totalCount
			} }`
			err = graphqlClient.Post(query, &graphqlResult)
			require.NoError(t, err)

			// 比对两者是否一致
			assert.Equal(t, restResult.Pagination.Total, graphqlResult.SearchPoems.TotalCount,
				"Total count should match between REST and GraphQL")

			assert.Equal(t, len(restResult.Data), len(graphqlResult.SearchPoems.Edges),
				"Number of results should match")

			// 返回的 ID 集合也应完全相同
			for i := range restResult.Data {
				assert.Equal(t, restResult.Data[i].ID, graphqlResult.SearchPoems.Edges[i].Node.ID,
					"Poem ID should match at position %d", i)
				assert.Equal(t, restResult.Data[i].Title, graphqlResult.SearchPoems.Edges[i].Node.Title,
					"Poem title should match at position %d", i)
			}
		})
	}
}

// TestRandomConsistency 验证 REST 与 GraphQL 的随机取词走同一套逻辑。
func TestRandomConsistency(t *testing.T) {
	restRouter, graphqlClient, repo := setupTestEnv(t)
	dynastyID, _, typeID := createTestData(t, repo)

	tests := []struct {
		name      string
		restQuery string
		gqlFilter string
	}{
		{"no filter", "", ""},
		// REST 用名称过滤，GraphQL 用 ID 过滤
		{"dynasty filter", "?dynasty=唐", "dynastyId: \"" + strconv.FormatInt(dynastyID, 10) + "\""},
		{"type filter", "?type_id=" + strconv.FormatInt(typeID, 10), "typeId: \"" + strconv.FormatInt(typeID, 10) + "\""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 反复调用两套接口，确认都能返回有效结果
			for range 5 {
				// REST 接口
				restReq := httptest.NewRequest(http.MethodGet, "/api/v1/poems/random"+tt.restQuery, nil)
				restResp := httptest.NewRecorder()
				restRouter.ServeHTTP(restResp, restReq)

				require.Equal(t, http.StatusOK, restResp.Code)

				var restResult struct {
					ID    int64  `json:"id"`
					Title string `json:"title"`
				}
				err := json.Unmarshal(restResp.Body.Bytes(), &restResult)
				require.NoError(t, err)
				assert.NotZero(t, restResult.ID, "REST should return a poem")

				// GraphQL 接口
				var graphqlResult struct {
					RandomPoem struct {
						ID    int64
						Title string
					}
				}

				query := `query { randomPoem`
				if tt.gqlFilter != "" {
					query += "(" + tt.gqlFilter + ")"
				}
				query += ` { id title } }`

				err = graphqlClient.Post(query, &graphqlResult)
				require.NoError(t, err)
				assert.NotZero(t, graphqlResult.RandomPoem.ID, "GraphQL should return a poem")

				// 两者应从同一份数据中取词。结果随机，无法比对具体 ID，但可以校验返回结构是否正确
				assert.NotEmpty(t, restResult.Title)
				assert.NotEmpty(t, graphqlResult.RandomPoem.Title)
			}
		})
	}
}

// TestListOrderConsistency 验证 REST 与 GraphQL 的诗词列表以相同顺序遍历语料。
func TestListOrderConsistency(t *testing.T) {
	restRouter, graphqlClient, repo := setupTestEnv(t)
	createTestData(t, repo)

	restIDs := func(page int) []int64 {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/poems?page="+strconv.Itoa(page)+"&page_size=1", nil)
		resp := httptest.NewRecorder()
		restRouter.ServeHTTP(resp, req)

		var result struct {
			Data []struct {
				ID int64 `json:"id"`
			} `json:"data"`
			Pagination struct {
				Total int `json:"total"`
			} `json:"pagination"`
		}
		require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &result))
		assert.Equal(t, 2, result.Pagination.Total, "REST must report the full corpus size, not the page size")

		ids := make([]int64, len(result.Data))
		for i, d := range result.Data {
			ids[i] = d.ID
		}
		return ids
	}

	graphqlIDs := func(page int) []int64 {
		var result struct {
			Poems struct {
				Edges []struct {
					Node struct {
						ID int64
					}
				}
				TotalCount int
			}
		}
		query := `query { poems(page: ` + strconv.Itoa(page) + `, pageSize: 1) {
			edges { node { id } }
			totalCount
		} }`
		require.NoError(t, graphqlClient.Post(query, &result))
		assert.Equal(t, 2, result.Poems.TotalCount)

		ids := make([]int64, len(result.Poems.Edges))
		for i, e := range result.Poems.Edges {
			ids[i] = e.Node.ID
		}
		return ids
	}

	// id 升序即诗词导入语料时的顺序
	assert.Equal(t, []int64{1001}, restIDs(1))
	assert.Equal(t, []int64{1002}, restIDs(2))

	for page := 1; page <= 2; page++ {
		assert.Equal(t, restIDs(page), graphqlIDs(page), "page %d must be the same poems in both APIs", page)
	}
}

// TestPaginationConsistency 验证 REST 与 GraphQL 的分页行为一致。
func TestPaginationConsistency(t *testing.T) {
	restRouter, graphqlClient, repo := setupTestEnv(t)
	createTestData(t, repo)

	// REST 接口
	restReq := httptest.NewRequest(http.MethodGet, "/api/v1/poems/search?q=李白&page=1&page_size=1", nil)
	restResp := httptest.NewRecorder()
	restRouter.ServeHTTP(restResp, restReq)

	var restResult struct {
		Data []struct {
			ID int64 `json:"id"`
		} `json:"data"`
		Pagination struct {
			Page     int `json:"page"`
			PageSize int `json:"page_size"`
			Total    int `json:"total"`
		} `json:"pagination"`
	}
	err := json.Unmarshal(restResp.Body.Bytes(), &restResult)
	require.NoError(t, err)

	// GraphQL 接口
	var graphqlResult struct {
		SearchPoems struct {
			Edges []struct {
				Node struct {
					ID int64
				}
			}
			PageInfo struct {
				HasNextPage     bool
				HasPreviousPage bool
			}
			TotalCount int
		}
	}

	query := `query { searchPoems(query: "李白", page: 1, pageSize: 1) {
		edges { node { id } }
		pageInfo { hasNextPage hasPreviousPage }
		totalCount
	} }`
	err = graphqlClient.Post(query, &graphqlResult)
	require.NoError(t, err)

	// 比对分页结果是否一致
	assert.Equal(t, restResult.Pagination.Total, graphqlResult.SearchPoems.TotalCount)
	assert.Equal(t, len(restResult.Data), len(graphqlResult.SearchPoems.Edges))
	assert.Equal(t, 1, len(restResult.Data), "Should return exactly 1 result per page")

	// 校验 hasNextPage 是否正确
	hasMore := restResult.Pagination.Page*restResult.Pagination.PageSize < restResult.Pagination.Total
	assert.Equal(t, hasMore, graphqlResult.SearchPoems.PageInfo.HasNextPage)
}
