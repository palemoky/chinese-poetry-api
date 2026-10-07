package graph

import (
	"context"
	"fmt"
	"testing"

	"github.com/99designs/gqlgen/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/palemoky/chinese-poetry-api/internal/database"
)

// setupTestResolver 基于内存数据库创建测试用的 resolver。
func setupTestResolver(t *testing.T) (*Resolver, *database.Repository) {
	// 创建内存数据库
	gormDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	db := &database.DB{DB: gormDB}

	// 用 Migrate 建出各语言变体的表
	err = db.Migrate()
	require.NoError(t, err)

	repo := database.NewRepository(db)

	resolver := NewResolver(db, repo)
	return resolver, repo
}

// createTestClient 创建 GraphQL 测试客户端。
func createTestClient(t *testing.T, resolver *Resolver) *client.Client {
	t.Helper()
	return client.New(NewServer(resolver, ServerOptions{ComplexityLimit: 5000, Introspection: true}))
}

// createTestData 向数据库写入测试数据。
func createTestData(t *testing.T, repo *database.Repository) (dynastyID, authorID int64, poemID int64) {
	var err error

	// 写入朝代
	dynastyID, err = repo.GetOrCreateDynasty("唐")
	require.NoError(t, err)

	// 写入作者
	authorID, err = repo.GetOrCreateAuthor("李白", dynastyID)
	require.NoError(t, err)

	// 写入诗词
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

		// 查询不存在的诗词时 GraphQL 会返回错误
		err := c.Post(`query { poem(id: "999") { title } }`, &resp)
		// 诗词本就不存在，报错属于预期行为
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
		// 搜索应能正常返回
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

	// 体裁已由 Migrate 预置，无需手动写入

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
		// 已有数据，应能返回一首诗
		assert.NotNil(t, resp.RandomPoem)
	})

	// 格式错误的 ID 曾被直接丢弃，过滤范围于是悄悄扩大到全量语料，
	// 随手返回一首无关的诗且不报任何错误。
	for _, tc := range []struct {
		name  string
		query string
	}{
		{"malformed dynastyId is rejected", `query { randomPoem(dynastyId: "abc") { title } }`},
		{"malformed typeId is rejected", `query { randomPoem(typeId: "1.5") { title } }`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var resp struct{}
			err := c.Post(tc.query, &resp)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "invalid syntax")
		})
	}
}

// 上下文传递的集成测试
func TestResolverWithContext(t *testing.T) {
	resolver, repo := setupTestResolver(t)
	createTestData(t, repo)

	ctx := context.Background()

	// 直接测试 poem resolver（lang 为 nil 时默认简体）
	poem, err := resolver.Query().Poem(ctx, "1", nil)
	require.NoError(t, err)
	assert.NotNil(t, poem)
	assert.Equal(t, "静夜思", poem.Title)
}

// createExtendedTestData 写入用于过滤条件测试的补充数据。
func createExtendedTestData(t *testing.T, resolver *Resolver, repo *database.Repository) (tangDynastyID, songDynastyID, libaiAuthorID, dumuAuthorID, typeID int64) {
	var err error

	// 写入唐朝
	tangDynastyID, err = repo.GetOrCreateDynasty("唐")
	require.NoError(t, err)

	// 写入宋朝
	songDynastyID, err = repo.GetOrCreateDynasty("宋")
	require.NoError(t, err)

	// 写入作者
	libaiAuthorID, err = repo.GetOrCreateAuthor("李白", tangDynastyID)
	require.NoError(t, err)

	dumuAuthorID, err = repo.GetOrCreateAuthor("杜牧", tangDynastyID)
	require.NoError(t, err)

	// 体裁由 Migrate 预置，这里直接使用「七言绝句」的既有 ID 12
	typeID = 12

	// 写入分属不同作者与体裁的诗词
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

// TestPoemsWithFilters 测试 GraphQL poems 查询的 dynastyId、authorId、typeId 过滤。
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
		assert.Equal(t, 2, resp.Poems.TotalCount) // 李白有两首
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

// TestPaginationBoundaries 测试分页的各种边界情况。
func TestPaginationBoundaries(t *testing.T) {
	resolver, repo := setupTestResolver(t)
	createExtendedTestData(t, resolver, repo)
	c := createTestClient(t, resolver)

	// 越界的分页参数一律报错而非截断，这样客户端传 pageSize: 500 时
	// 能明确知道自己并没有拿到 500 条。
	for _, tc := range []struct {
		name  string
		query string
		want  string
	}{
		{"page 0 is rejected", `query { poems(page: 0) { totalCount } }`, "page must be at least 1"},
		{"negative page is rejected", `query { poems(page: -1) { totalCount } }`, "page must be at least 1"},
		{"pageSize 0 is rejected", `query { poems(pageSize: 0) { totalCount } }`, "pageSize must be between"},
		{"pageSize above the cap is rejected", `query { poems(pageSize: 500) { totalCount } }`, "pageSize must be between"},
		// searchPoems 曾直接读取参数，完全没有做上限约束
		{"searchPoems pageSize above the cap is rejected", `query { searchPoems(query: "李白", pageSize: 1000000) { totalCount } }`, "pageSize must be between"},
		{"authors pageSize above the cap is rejected", `query { authors(pageSize: 500) { totalCount } }`, "pageSize must be between"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var resp struct{}
			err := c.Post(tc.query, &resp)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}

	t.Run("pageSize at the cap is accepted", func(t *testing.T) {
		var resp struct {
			Poems struct {
				TotalCount int
			}
		}
		require.NoError(t, c.Post(`query { poems(pageSize: 100) { totalCount } }`, &resp))
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

// TestAuthorsWithFilters 测试 GraphQL authors 查询的 dynastyId 过滤。
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
		assert.GreaterOrEqual(t, resp.Authors.TotalCount, 2) // 李白与杜牧
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
		// 测试数据中宋朝没有作者
		assert.Equal(t, 0, resp.Authors.TotalCount)
	})

	// 列表中的作者曾经不带朝代，dynasty 一律为 null
	t.Run("listed authors carry their dynasty", func(t *testing.T) {
		var resp struct {
			Authors struct {
				Edges []struct {
					Node struct {
						Name    string
						Dynasty *struct{ Name string }
					}
				}
			}
		}

		require.NoError(t, c.Post(`query { authors { edges { node { name dynasty { name } } } } }`, &resp))
		require.NotEmpty(t, resp.Authors.Edges)
		for _, e := range resp.Authors.Edges {
			require.NotNil(t, e.Node.Dynasty, e.Node.Name)
			assert.Equal(t, "唐", e.Node.Dynasty.Name, e.Node.Name)
		}
	})
}

func TestAuthorDescriptionField(t *testing.T) {
	resolver, repo := setupTestResolver(t)
	c := createTestClient(t, resolver)

	dynastyID, err := repo.GetOrCreateDynasty("宋")
	require.NoError(t, err)
	withBio, err := repo.GetOrCreateAuthor("苏轼", dynastyID)
	require.NoError(t, err)
	withoutBio, err := repo.GetOrCreateAuthor("佚名", dynastyID)
	require.NoError(t, err)
	require.NoError(t, repo.SetAuthorDescriptions(map[int64]string{withBio: "苏轼，字子瞻。"}))

	var resp struct {
		Author struct{ Description *string }
	}
	require.NoError(t, c.Post(fmt.Sprintf(`{ author(id: "%d") { description } }`, withBio), &resp))
	require.NotNil(t, resp.Author.Description)
	assert.Equal(t, "苏轼，字子瞻。", *resp.Author.Description)

	require.NoError(t, c.Post(fmt.Sprintf(`{ author(id: "%d") { description } }`, withoutBio), &resp))
	assert.Nil(t, resp.Author.Description)
}
