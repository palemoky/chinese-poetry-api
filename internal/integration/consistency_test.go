package integration

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/99designs/gqlgen/client"
	"github.com/99designs/gqlgen/graphql/handler"
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
	"github.com/palemoky/chinese-poetry-api/internal/graph/generated"
)

// setupTestEnv creates a test environment with both REST and GraphQL
func setupTestEnv(t *testing.T) (*gin.Engine, *client.Client, *database.Repository) {
	gin.SetMode(gin.TestMode)

	// Create in-memory database
	gormDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	db := &database.DB{DB: gormDB}
	err = db.Migrate()
	require.NoError(t, err)

	repo := database.NewRepository(db)

	// Setup REST router
	cfg := &config.Config{
		Server: config.ServerConfig{Mode: "test"},
	}
	restRouter := rest.SetupRouter(cfg, db, repo)

	// Setup GraphQL client
	resolver := graph.NewResolver(db, repo)
	srv := handler.NewDefaultServer(generated.NewExecutableSchema(generated.Config{
		Resolvers: resolver,
	}))
	graphqlClient := client.New(srv)

	return restRouter, graphqlClient, repo
}

// createTestData creates consistent test data
func createTestData(t *testing.T, repo *database.Repository) (dynastyID, authorID, typeID int64) {
	var err error

	// Create dynasty
	dynastyID, err = repo.GetOrCreateDynasty("唐")
	require.NoError(t, err)

	// Create author
	authorID, err = repo.GetOrCreateAuthor("李白", dynastyID)
	require.NoError(t, err)

	// Get poetry type (pre-seeded by Migrate)
	typeID = 12 // 七言绝句

	// Create poems
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

// TestSearchConsistency verifies REST and GraphQL search return same results
func TestSearchConsistency(t *testing.T) {
	restRouter, graphqlClient, repo := setupTestEnv(t)
	createTestData(t, repo)

	tests := []struct {
		name       string
		query      string
		searchType string
	}{
		{"search by title", "静夜思", "title"},
		{"search by author", "李白", "author"},
		{"search all", "李白", "all"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// REST API call
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

			// GraphQL API call
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
			case "author":
				searchTypeGQL = "AUTHOR"
			}

			query := `query { searchPoems(query: "` + tt.query + `", searchType: ` + searchTypeGQL + `) {
				edges { node { id title } }
				totalCount
			} }`
			err = graphqlClient.Post(query, &graphqlResult)
			require.NoError(t, err)

			// Verify consistency
			assert.Equal(t, restResult.Pagination.Total, graphqlResult.SearchPoems.TotalCount,
				"Total count should match between REST and GraphQL")

			assert.Equal(t, len(restResult.Data), len(graphqlResult.SearchPoems.Edges),
				"Number of results should match")

			// Verify same IDs are returned
			for i := range restResult.Data {
				assert.Equal(t, restResult.Data[i].ID, graphqlResult.SearchPoems.Edges[i].Node.ID,
					"Poem ID should match at position %d", i)
				assert.Equal(t, restResult.Data[i].Title, graphqlResult.SearchPoems.Edges[i].Node.Title,
					"Poem title should match at position %d", i)
			}
		})
	}
}

// TestRandomConsistency verifies REST and GraphQL random use same algorithm
func TestRandomConsistency(t *testing.T) {
	restRouter, graphqlClient, repo := setupTestEnv(t)
	dynastyID, _, typeID := createTestData(t, repo)

	tests := []struct {
		name      string
		restQuery string
		gqlFilter string
	}{
		{"no filter", "", ""},
		// REST uses names, GraphQL uses IDs
		{"dynasty filter", "?dynasty=唐", "dynastyId: \"" + strconv.FormatInt(dynastyID, 10) + "\""},
		{"type filter", "?type_id=" + strconv.FormatInt(typeID, 10), "typeId: \"" + strconv.FormatInt(typeID, 10) + "\""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Call both APIs multiple times to ensure they both return valid poems
			for i := 0; i < 5; i++ {
				// REST API
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

				// GraphQL API
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

				// Both should return poems from the same dataset
				// We can't compare exact IDs since they're random, but we can verify structure
				assert.NotEmpty(t, restResult.Title)
				assert.NotEmpty(t, graphqlResult.RandomPoem.Title)
			}
		})
	}
}

// TestPaginationConsistency verifies REST and GraphQL pagination work the same
func TestPaginationConsistency(t *testing.T) {
	restRouter, graphqlClient, repo := setupTestEnv(t)
	createTestData(t, repo)

	// REST API
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

	// GraphQL API
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

	// Verify pagination consistency
	assert.Equal(t, restResult.Pagination.Total, graphqlResult.SearchPoems.TotalCount)
	assert.Equal(t, len(restResult.Data), len(graphqlResult.SearchPoems.Edges))
	assert.Equal(t, 1, len(restResult.Data), "Should return exactly 1 result per page")

	// Verify hasNextPage is correct
	hasMore := restResult.Pagination.Page*restResult.Pagination.PageSize < restResult.Pagination.Total
	assert.Equal(t, hasMore, graphqlResult.SearchPoems.PageInfo.HasNextPage)
}
