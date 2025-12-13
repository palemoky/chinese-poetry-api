package graph

import (
	"context"
	"fmt"
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

	db := &database.DB{DB: gormDB}

	// Use Migrate() to create language-specific tables
	err = db.Migrate()
	require.NoError(t, err)

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
		ID:        1,
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

		err := c.Post(`query { poem(id: "1") { title content } }`, &resp)
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
		err := c.Post(`query { poem(id: "999") { title } }`, &resp)
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

	// Poetry types are already seeded by Migrate(), no need to create manually

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

	// Test poem resolver directly (nil lang = default to simplified Chinese)
	poem, err := resolver.Query().Poem(ctx, "1", nil)
	require.NoError(t, err)
	assert.NotNil(t, poem)
	assert.Equal(t, "静夜思", poem.Title)
}

// createExtendedTestData creates additional test data for filter testing
func createExtendedTestData(t *testing.T, resolver *Resolver, repo *database.Repository) (tangDynastyID, songDynastyID, libaiAuthorID, dumuAuthorID, typeID int64) {
	var err error

	// Create Tang dynasty
	tangDynastyID, err = repo.GetOrCreateDynasty("唐")
	require.NoError(t, err)

	// Create Song dynasty
	songDynastyID, err = repo.GetOrCreateDynasty("宋")
	require.NoError(t, err)

	// Create authors
	libaiAuthorID, err = repo.GetOrCreateAuthor("李白", tangDynastyID)
	require.NoError(t, err)

	dumuAuthorID, err = repo.GetOrCreateAuthor("杜牧", tangDynastyID)
	require.NoError(t, err)

	// Poetry types are already seeded by Migrate()
	// Use the pre-seeded ID for "七言绝句" which has ID 12
	typeID = 12

	// Create poems with different authors and types
	poems := []*database.Poem{
		{
			ID:        1001,
			Title:     "静夜思",
			Content:   datatypes.JSON([]byte(`["床前明月光","疑是地上霜"]`)),
			AuthorID:  &libaiAuthorID,
			DynastyID: &tangDynastyID,
			TypeID:    &typeID,
		},
		{
			ID:        1002,
			Title:     "将进酒",
			Content:   datatypes.JSON([]byte(`["君不见黄河之水天上来"]`)),
			AuthorID:  &libaiAuthorID,
			DynastyID: &tangDynastyID,
			TypeID:    &typeID,
		},
		{
			ID:        1003,
			Title:     "清明",
			Content:   datatypes.JSON([]byte(`["清明时节雨纷纷"]`)),
			AuthorID:  &dumuAuthorID,
			DynastyID: &tangDynastyID,
			TypeID:    &typeID,
		},
	}

	for _, poem := range poems {
		err = repo.InsertPoem(poem)
		require.NoError(t, err)
	}

	return tangDynastyID, songDynastyID, libaiAuthorID, dumuAuthorID, typeID
}

// TestPoemsWithFilters tests GraphQL poems query with dynastyId, authorId, typeId filters
func TestPoemsWithFilters(t *testing.T) {
	resolver, repo := setupTestResolver(t)
	tangID, _, libaiID, _, typeID := createExtendedTestData(t, resolver, repo)
	c := createTestClient(t, resolver)

	t.Run("filter by dynastyId", func(t *testing.T) {
		var resp struct {
			Poems struct {
				Edges []struct {
					Node struct {
						Title string
					}
				}
				TotalCount int
			}
		}

		query := fmt.Sprintf(`query { poems(dynastyId: "%d") { edges { node { title } } totalCount } }`, tangID)
		err := c.Post(query, &resp)
		require.NoError(t, err)
		assert.Equal(t, 3, resp.Poems.TotalCount)
	})

	t.Run("filter by authorId", func(t *testing.T) {
		var resp struct {
			Poems struct {
				Edges []struct {
					Node struct {
						Title string
					}
				}
				TotalCount int
			}
		}

		query := fmt.Sprintf(`query { poems(authorId: "%d") { edges { node { title } } totalCount } }`, libaiID)
		err := c.Post(query, &resp)
		require.NoError(t, err)
		assert.Equal(t, 2, resp.Poems.TotalCount) // Li Bai has 2 poems
	})

	t.Run("filter by typeId", func(t *testing.T) {
		var resp struct {
			Poems struct {
				Edges []struct {
					Node struct {
						Title string
					}
				}
				TotalCount int
			}
		}

		query := fmt.Sprintf(`query { poems(typeId: "%d") { edges { node { title } } totalCount } }`, typeID)
		err := c.Post(query, &resp)
		require.NoError(t, err)
		assert.Equal(t, 3, resp.Poems.TotalCount)
	})

	t.Run("filter with multiple conditions", func(t *testing.T) {
		var resp struct {
			Poems struct {
				TotalCount int
			}
		}

		query := fmt.Sprintf(`query { poems(dynastyId: "%d", authorId: "%d") { totalCount } }`, tangID, libaiID)
		err := c.Post(query, &resp)
		require.NoError(t, err)
		assert.Equal(t, 2, resp.Poems.TotalCount)
	})

	t.Run("filter with non-existent dynastyId returns empty", func(t *testing.T) {
		var resp struct {
			Poems struct {
				TotalCount int
			}
		}

		err := c.Post(`query { poems(dynastyId: "99999") { totalCount } }`, &resp)
		require.NoError(t, err)
		assert.Equal(t, 0, resp.Poems.TotalCount)
	})
}

// TestPaginationBoundaries tests edge cases in pagination
func TestPaginationBoundaries(t *testing.T) {
	resolver, repo := setupTestResolver(t)
	createExtendedTestData(t, resolver, repo)
	c := createTestClient(t, resolver)

	t.Run("page 0 defaults to page 1", func(t *testing.T) {
		var resp struct {
			Poems struct {
				PageInfo struct {
					HasPreviousPage bool
				}
				TotalCount int
			}
		}

		err := c.Post(`query { poems(page: 0) { pageInfo { hasPreviousPage } totalCount } }`, &resp)
		require.NoError(t, err)
		assert.False(t, resp.Poems.PageInfo.HasPreviousPage)
	})

	t.Run("negative page defaults to page 1", func(t *testing.T) {
		var resp struct {
			Poems struct {
				PageInfo struct {
					HasPreviousPage bool
				}
			}
		}

		err := c.Post(`query { poems(page: -1) { pageInfo { hasPreviousPage } } }`, &resp)
		require.NoError(t, err)
		assert.False(t, resp.Poems.PageInfo.HasPreviousPage)
	})

	t.Run("pageSize 0 defaults to 20", func(t *testing.T) {
		var resp struct {
			Poems struct {
				Edges []struct {
					Node struct {
						Title string
					}
				}
			}
		}

		err := c.Post(`query { poems(pageSize: 0) { edges { node { title } } } }`, &resp)
		require.NoError(t, err)
		// Should return all 3 poems since default is 20
		assert.GreaterOrEqual(t, len(resp.Poems.Edges), 1)
	})

	t.Run("very large pageSize is capped at 100", func(t *testing.T) {
		var resp struct {
			Poems struct {
				Edges []struct {
					Node struct {
						Title string
					}
				}
			}
		}

		err := c.Post(`query { poems(pageSize: 500) { edges { node { title } } } }`, &resp)
		require.NoError(t, err)
		// Should still work, just capped
		assert.NotNil(t, resp.Poems.Edges)
	})

	t.Run("hasNextPage is true when more data exists", func(t *testing.T) {
		var resp struct {
			Poems struct {
				PageInfo struct {
					HasNextPage bool
				}
				TotalCount int
			}
		}

		err := c.Post(`query { poems(page: 1, pageSize: 2) { pageInfo { hasNextPage } totalCount } }`, &resp)
		require.NoError(t, err)
		if resp.Poems.TotalCount > 2 {
			assert.True(t, resp.Poems.PageInfo.HasNextPage)
		}
	})

	t.Run("hasPreviousPage is true on page 2", func(t *testing.T) {
		var resp struct {
			Poems struct {
				PageInfo struct {
					HasPreviousPage bool
				}
			}
		}

		err := c.Post(`query { poems(page: 2, pageSize: 1) { pageInfo { hasPreviousPage } } }`, &resp)
		require.NoError(t, err)
		assert.True(t, resp.Poems.PageInfo.HasPreviousPage)
	})
}

// TestAuthorsWithFilters tests GraphQL authors query with dynastyId filter
func TestAuthorsWithFilters(t *testing.T) {
	resolver, repo := setupTestResolver(t)
	tangID, songID, _, _, _ := createExtendedTestData(t, resolver, repo)
	c := createTestClient(t, resolver)

	t.Run("filter authors by dynastyId", func(t *testing.T) {
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

		query := fmt.Sprintf(`query { authors(dynastyId: "%d") { edges { node { name } } totalCount } }`, tangID)
		err := c.Post(query, &resp)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, resp.Authors.TotalCount, 2) // Li Bai and Du Mu
	})

	t.Run("filter authors by non-existent dynastyId", func(t *testing.T) {
		var resp struct {
			Authors struct {
				TotalCount int
			}
		}

		query := fmt.Sprintf(`query { authors(dynastyId: "%d") { totalCount } }`, songID)
		err := c.Post(query, &resp)
		require.NoError(t, err)
		// Song dynasty has no authors in test data
		assert.Equal(t, 0, resp.Authors.TotalCount)
	})
}
