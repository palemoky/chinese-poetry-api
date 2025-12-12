package graph

import (
	"context"
	"testing"

	"github.com/99designs/gqlgen/client"
	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/palemoky/chinese-poetry-api/internal/database"
	"github.com/palemoky/chinese-poetry-api/internal/graph/generated"
	"github.com/palemoky/chinese-poetry-api/internal/search"
)

// setupTestResolver creates a test resolver with an in-memory database
func setupTestResolver(t *testing.T) (*Resolver, *database.Repository) {
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

	resolver := NewResolver(db, repo, searchEngine)
	return resolver, repo
}

// createTestClient creates a GraphQL test client
func createTestClient(t *testing.T, resolver *Resolver) *client.Client {
	srv := handler.NewDefaultServer(generated.NewExecutableSchema(generated.Config{
		Resolvers: resolver,
	}))
	return client.New(srv)
}

// createTestData creates test data in the database
func createTestData(t *testing.T, repo *database.Repository) (dynastyID, authorID int64, poemID int64) {
	var err error

	// Create dynasty
	dynastyID, err = repo.GetOrCreateDynasty("唐")
	require.NoError(t, err)

	// Create author
	authorID, err = repo.GetOrCreateAuthor("李白", dynastyID)
	require.NoError(t, err)

	// Create poem
	poem := &database.Poem{
		ID:        12345678901234,
		Title:     "静夜思",
		Content:   datatypes.JSON([]byte(`["床前明月光","疑是地上霜","举头望明月","低头思故乡"]`)),
		AuthorID:  &authorID,
		DynastyID: &dynastyID,
	}
	err = repo.InsertPoem(poem)
	require.NoError(t, err)

	return dynastyID, authorID, poem.ID
}

func TestPoemQuery(t *testing.T) {
	resolver, repo := setupTestResolver(t)
	_, _, _ = createTestData(t, repo)
	c := createTestClient(t, resolver)

	t.Run("get existing poem", func(t *testing.T) {
		var resp struct {
			Poem struct {
				Title   string
				Content []string
			}
		}

		err := c.Post(`query { poem(id: "12345678901234") { title content } }`, &resp)
		require.NoError(t, err)
		assert.Equal(t, "静夜思", resp.Poem.Title)
		assert.Len(t, resp.Poem.Content, 4)
	})

	t.Run("get non-existent poem returns error", func(t *testing.T) {
		var resp struct {
			Poem *struct {
				Title string
			}
		}

		// Non-existent poem returns an error in GraphQL
		err := c.Post(`query { poem(id: "99999999999999") { title } }`, &resp)
		// The error is expected since the poem doesn't exist
		assert.Error(t, err)
	})
}

func TestPoemsQuery(t *testing.T) {
	resolver, repo := setupTestResolver(t)
	createTestData(t, repo)
	c := createTestClient(t, resolver)

	t.Run("get poems with default pagination", func(t *testing.T) {
		var resp struct {
			Poems struct {
				Edges []struct {
					Node struct {
						Title string
					}
				}
				PageInfo struct {
					HasNextPage     bool
					HasPreviousPage bool
				}
				TotalCount int
			}
		}

		err := c.Post(`query { poems { edges { node { title } } pageInfo { hasNextPage hasPreviousPage } totalCount } }`, &resp)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, resp.Poems.TotalCount, 1)
		assert.GreaterOrEqual(t, len(resp.Poems.Edges), 1)
	})

	t.Run("get poems with pagination", func(t *testing.T) {
		var resp struct {
			Poems struct {
				TotalCount int
			}
		}

		err := c.Post(`query { poems(page: 1, pageSize: 5) { totalCount } }`, &resp)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, resp.Poems.TotalCount, 1)
	})
}

func TestSearchPoemsQuery(t *testing.T) {
	resolver, repo := setupTestResolver(t)
	createTestData(t, repo)
	c := createTestClient(t, resolver)

	t.Run("search poems", func(t *testing.T) {
		var resp struct {
			SearchPoems struct {
				Edges []struct {
					Node struct {
						Title string
					}
				}
				TotalCount int
			}
		}

		err := c.Post(`query { searchPoems(query: "静夜思") { edges { node { title } } totalCount } }`, &resp)
		require.NoError(t, err)
		// Search should work
		assert.NotNil(t, resp.SearchPoems)
	})

	t.Run("search with type", func(t *testing.T) {
		var resp struct {
			SearchPoems struct {
				TotalCount int
			}
		}

		err := c.Post(`query { searchPoems(query: "李白", searchType: AUTHOR) { totalCount } }`, &resp)
		require.NoError(t, err)
	})
}

func TestAuthorsQuery(t *testing.T) {
	resolver, repo := setupTestResolver(t)
	createTestData(t, repo)
	c := createTestClient(t, resolver)

	t.Run("get authors", func(t *testing.T) {
		var resp struct {
			Authors struct {
				Edges []struct {
					Node struct {
						Name string
					}
				}
				TotalCount int
			}
		}

		err := c.Post(`query { authors { edges { node { name } } totalCount } }`, &resp)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, resp.Authors.TotalCount, 1)
	})
}

func TestDynastiesQuery(t *testing.T) {
	resolver, repo := setupTestResolver(t)
	createTestData(t, repo)
	c := createTestClient(t, resolver)

	t.Run("get dynasties", func(t *testing.T) {
		var resp struct {
			Dynasties []struct {
				Name string
			}
		}

		err := c.Post(`query { dynasties { name } }`, &resp)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(resp.Dynasties), 1)
	})
}

func TestPoemTypesQuery(t *testing.T) {
	resolver, _ := setupTestResolver(t)

	// Insert a poetry type manually since it's usually seeded in migration
	resolver.DB.Create(&database.PoetryType{
		ID:       1,
		Name:     "五言绝句",
		Category: "诗",
	})

	c := createTestClient(t, resolver)

	t.Run("get poem types", func(t *testing.T) {
		var resp struct {
			PoemTypes []struct {
				Name     string
				Category string
			}
		}

		err := c.Post(`query { poemTypes { name category } }`, &resp)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(resp.PoemTypes), 1)
	})
}

func TestStatisticsQuery(t *testing.T) {
	resolver, repo := setupTestResolver(t)
	createTestData(t, repo)
	c := createTestClient(t, resolver)

	t.Run("get statistics", func(t *testing.T) {
		var resp struct {
			Statistics struct {
				TotalPoems     int
				TotalAuthors   int
				TotalDynasties int
			}
		}

		err := c.Post(`query { statistics { totalPoems totalAuthors totalDynasties } }`, &resp)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, resp.Statistics.TotalPoems, 1)
		assert.GreaterOrEqual(t, resp.Statistics.TotalAuthors, 1)
	})
}

func TestRandomPoemQuery(t *testing.T) {
	resolver, repo := setupTestResolver(t)
	createTestData(t, repo)
	c := createTestClient(t, resolver)

	t.Run("get random poem", func(t *testing.T) {
		var resp struct {
			RandomPoem *struct {
				Title string
			}
		}

		err := c.Post(`query { randomPoem { title } }`, &resp)
		require.NoError(t, err)
		// Should return a poem since we have data
		assert.NotNil(t, resp.RandomPoem)
	})
}

// Integration test for context passing
func TestResolverWithContext(t *testing.T) {
	resolver, repo := setupTestResolver(t)
	createTestData(t, repo)

	ctx := context.Background()

	// Test poem resolver directly
	poem, err := resolver.Query().Poem(ctx, "12345678901234")
	require.NoError(t, err)
	assert.NotNil(t, poem)
	assert.Equal(t, "静夜思", poem.Title)
}
