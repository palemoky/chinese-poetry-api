package integration

import (
	"path/filepath"
	"strconv"
	"testing"

	"github.com/99designs/gqlgen/client"
	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/palemoky/chinese-poetry-api/internal/database"
	"github.com/palemoky/chinese-poetry-api/internal/graph"
	"github.com/palemoky/chinese-poetry-api/internal/graph/generated"
)

// setupLangTestEnv 基于文件型数据库构建 GraphQL 测试客户端。
//
// 这里用文件而非 ":memory:"：每条连到 ":memory:" 的 SQLite 连接都会得到
// 各自独立的库，一旦 gqlgen 并发解析字段，连接池中新增的连接看到的是
// 未迁移的空库，查询会莫名报 "no such table"。
func setupLangTestEnv(t *testing.T) (*client.Client, *database.Repository) {
	gormDB, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "test.db")), &gorm.Config{})
	require.NoError(t, err)

	db := &database.DB{DB: gormDB}
	require.NoError(t, db.Migrate())

	repo := database.NewRepository(db)
	resolver := graph.NewResolver(db, repo)
	srv := handler.NewDefaultServer(generated.NewExecutableSchema(generated.Config{Resolvers: resolver}))

	return client.New(srv), repo
}

// seedVariant 为每个语言变体写入一首标题各不相同的诗，
// 这样一旦查错了表，结果本身就会暴露问题，而不是看上去似乎也说得通。
func seedVariant(t *testing.T, repo *database.Repository, lang database.Lang, title, author string) {
	langRepo := repo.WithLang(lang)

	dynastyID, err := langRepo.GetOrCreateDynasty("唐")
	require.NoError(t, err)
	authorID, err := langRepo.GetOrCreateAuthor(author, dynastyID)
	require.NoError(t, err)

	require.NoError(t, langRepo.InsertPoem(&database.Poem{
		ID:        1,
		Title:     title,
		Content:   datatypes.JSON([]byte(`["床前明月光"]`)),
		AuthorID:  &authorID,
		DynastyID: &dynastyID,
	}))
}

// TestGraphQLLangSelectsVariant 覆盖 lang 参数——它曾经在整个 GraphQL API 中都不起作用。
// 起因是两个缺陷叠加：gqlgen 的 autobind 把枚举字面量直接转成 database.Lang("ZH_HANT")，
// 该取值与两个变体常量都不相等，导致所有表名辅助函数都落到简体分支；
// 而 poems/poem/authors/author 这几个 resolver 干脆从未调用过 WithLang。
func TestGraphQLLangSelectsVariant(t *testing.T) {
	c, repo := setupLangTestEnv(t)
	seedVariant(t, repo, database.LangHans, "简体标题", "李白")
	seedVariant(t, repo, database.LangHant, "繁體標題", "李白傳統")

	t.Run("poems", func(t *testing.T) {
		for lang, want := range map[string]string{"ZH_HANS": "简体标题", "ZH_HANT": "繁體標題"} {
			var resp struct {
				Poems struct {
					Edges []struct {
						Node struct{ Title string }
					}
				}
			}
			require.NoError(t, c.Post(`query { poems(lang: `+lang+`) { edges { node { title } } } }`, &resp))
			require.Len(t, resp.Poems.Edges, 1)
			assert.Equal(t, want, resp.Poems.Edges[0].Node.Title, "lang: %s read the wrong table", lang)
		}
	})

	t.Run("poem by id", func(t *testing.T) {
		for lang, want := range map[string]string{"ZH_HANS": "简体标题", "ZH_HANT": "繁體標題"} {
			var resp struct {
				Poem struct{ Title string }
			}
			require.NoError(t, c.Post(`query { poem(id: "1", lang: `+lang+`) { title } }`, &resp))
			assert.Equal(t, want, resp.Poem.Title)
		}
	})

	t.Run("authors", func(t *testing.T) {
		for lang, want := range map[string]string{"ZH_HANS": "李白", "ZH_HANT": "李白傳統"} {
			var resp struct {
				Authors struct {
					Edges []struct {
						Node struct{ Name string }
					}
				}
			}
			require.NoError(t, c.Post(`query { authors(lang: `+lang+`) { edges { node { name } } } }`, &resp))
			require.Len(t, resp.Authors.Edges, 1)
			assert.Equal(t, want, resp.Authors.Edges[0].Node.Name)
		}
	})

	t.Run("searchPoems", func(t *testing.T) {
		var resp struct {
			SearchPoems struct{ TotalCount int }
		}
		require.NoError(t, c.Post(`query { searchPoems(query: "繁體", lang: ZH_HANT) { totalCount } }`, &resp))
		assert.Equal(t, 1, resp.SearchPoems.TotalCount)

		require.NoError(t, c.Post(`query { searchPoems(query: "繁體", lang: ZH_HANS) { totalCount } }`, &resp))
		assert.Equal(t, 0, resp.SearchPoems.TotalCount, "simplified table has no traditional title")
	})

	t.Run("an unknown enum literal is rejected", func(t *testing.T) {
		var resp struct{}
		err := c.Post(`query { poems(lang: ZH_HANZ) { totalCount } }`, &resp)
		require.Error(t, err)
	})
}

// TestSearchPoemsCursors 覆盖 searchPoems 的 connection——它曾是手写构造的：
// 游标在每页内都从 0 开始编号，于是第 2 页首条边的游标与第 1 页的相同；
// 而且没有填 startCursor 与 endCursor，与其他所有 connection 的行为不一致。
func TestSearchPoemsCursors(t *testing.T) {
	c, repo := setupLangTestEnv(t)

	dynastyID, err := repo.GetOrCreateDynasty("唐")
	require.NoError(t, err)
	authorID, err := repo.GetOrCreateAuthor("李白", dynastyID)
	require.NoError(t, err)
	for i := range 4 {
		require.NoError(t, repo.InsertPoem(&database.Poem{
			ID:        int64(i + 1),
			Title:     "春日" + string(rune('A'+i)),
			Content:   datatypes.JSON([]byte(`["春风"]`)),
			AuthorID:  &authorID,
			DynastyID: &dynastyID,
		}))
	}

	page := func(n int) (cursors []string, start, end *string) {
		var resp struct {
			SearchPoems struct {
				Edges []struct {
					Cursor string
					Node   struct{ Title string }
				}
				PageInfo struct {
					StartCursor *string
					EndCursor   *string
				}
			}
		}
		q := `query { searchPoems(query: "春日", page: ` + strconv.Itoa(n) + `, pageSize: 2) {
			edges { cursor node { title } }
			pageInfo { startCursor endCursor }
		} }`
		require.NoError(t, c.Post(q, &resp))

		for _, e := range resp.SearchPoems.Edges {
			cursors = append(cursors, e.Cursor)
		}
		return cursors, resp.SearchPoems.PageInfo.StartCursor, resp.SearchPoems.PageInfo.EndCursor
	}

	first, start1, end1 := page(1)
	second, start2, _ := page(2)

	// 游标对外不透明，但必须能解回它所标识的那一条诗词：
	// 诗词按 id 升序排列，第 1 页是 id 1、2，第 2 页是 id 3、4。
	assert.Equal(t, []string{
		database.EncodePoemCursor(1),
		database.EncodePoemCursor(2),
	}, first)
	assert.Equal(t, []string{
		database.EncodePoemCursor(3),
		database.EncodePoemCursor(4),
	}, second, "page 2 cursors must continue from page 1, not restart at the first poem")

	require.NotNil(t, start1)
	require.NotNil(t, end1)
	require.NotNil(t, start2)
	assert.Equal(t, database.EncodePoemCursor(1), *start1)
	assert.Equal(t, database.EncodePoemCursor(2), *end1)
	assert.Equal(t, database.EncodePoemCursor(3), *start2)
}

// TestPoemsCursorPagination 覆盖 poems 的游标翻页：
// 它不受 page 分页的深度上限约束，因此必须能只靠 endCursor 一路走到底。
func TestPoemsCursorPagination(t *testing.T) {
	c, repo := setupLangTestEnv(t)

	dynastyID, err := repo.GetOrCreateDynasty("唐")
	require.NoError(t, err)
	authorID, err := repo.GetOrCreateAuthor("李白", dynastyID)
	require.NoError(t, err)

	const total = 5
	for i := range total {
		require.NoError(t, repo.InsertPoem(&database.Poem{
			ID:        int64(i + 1),
			Title:     "诗" + strconv.Itoa(i),
			Content:   datatypes.JSON([]byte(`["内容"]`)),
			AuthorID:  &authorID,
			DynastyID: &dynastyID,
		}))
	}

	type poemsResponse struct {
		Poems struct {
			Edges []struct {
				Node struct{ Title string }
			}
			PageInfo struct {
				HasNextPage bool
				EndCursor   *string
			}
			TotalCount int
		}
	}

	// 不带 after 的首次查询也要给出 endCursor，否则没有入口切换到游标翻页
	var resp poemsResponse
	require.NoError(t, c.Post(`query { poems(pageSize: 2) {
		edges { node { title } }
		pageInfo { hasNextPage endCursor }
		totalCount
	} }`, &resp))
	require.True(t, resp.Poems.PageInfo.HasNextPage)
	require.NotNil(t, resp.Poems.PageInfo.EndCursor)

	var visited []string
	for {
		for _, e := range resp.Poems.Edges {
			visited = append(visited, e.Node.Title)
		}
		assert.Equal(t, total, resp.Poems.TotalCount,
			"totalCount is the size of the whole result set, not the rows left after the cursor")

		if !resp.Poems.PageInfo.HasNextPage {
			break
		}
		require.NotNil(t, resp.Poems.PageInfo.EndCursor)

		q := `query { poems(pageSize: 2, after: "` + *resp.Poems.PageInfo.EndCursor + `") {
			edges { node { title } }
			pageInfo { hasNextPage endCursor }
			totalCount
		} }`
		resp = poemsResponse{}
		require.NoError(t, c.Post(q, &resp))
	}

	assert.Equal(t, []string{"诗0", "诗1", "诗2", "诗3", "诗4"}, visited,
		"cursor pagination must visit every poem exactly once, in id order")
}

// TestPoemsCursorRejectsBadInput 覆盖游标参数的错误输入。
func TestPoemsCursorRejectsBadInput(t *testing.T) {
	c, _ := setupLangTestEnv(t)

	t.Run("a cursor this API did not issue is rejected", func(t *testing.T) {
		var resp struct{}
		// 客户端最容易犯的错：把游标当成可以自己构造的行号
		err := c.Post(`query { poems(after: "3") { totalCount } }`, &resp)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cursor")
	})

	t.Run("page and after cannot be combined", func(t *testing.T) {
		var resp struct{}
		q := `query { poems(page: 2, after: "` + database.EncodePoemCursor(1) + `") { totalCount } }`
		err := c.Post(q, &resp)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "mutually exclusive")
	})
}

// TestAuthorListingTieBreak 覆盖 poem_count 相同的作者的分页场景。
// 仅按 poem_count 排序并非全序，因此某个 LIMIT/OFFSET 窗口
// 究竟返回并列行中的哪几条是不确定的。
func TestAuthorListingTieBreak(t *testing.T) {
	c, repo := setupLangTestEnv(t)

	dynastyID, err := repo.GetOrCreateDynasty("唐")
	require.NoError(t, err)

	// 每位作者都恰好一首诗，于是所有人的 poem_count 全部并列
	const authorCount = 12
	for i := range authorCount {
		name := "作者" + strconv.Itoa(i)
		authorID, err := repo.GetOrCreateAuthor(name, dynastyID)
		require.NoError(t, err)
		require.NoError(t, repo.InsertPoem(&database.Poem{
			ID:        int64(i + 1),
			Title:     "诗" + strconv.Itoa(i),
			Content:   datatypes.JSON([]byte(`["内容"]`)),
			AuthorID:  &authorID,
			DynastyID: &dynastyID,
		}))
	}

	seen := map[string]int{}
	for page := 1; page <= authorCount/3; page++ {
		var resp struct {
			Authors struct {
				Edges []struct {
					Node struct{ Name string }
				}
			}
		}
		q := `query { authors(page: ` + strconv.Itoa(page) + `, pageSize: 3) { edges { node { name } } } }`
		require.NoError(t, c.Post(q, &resp))
		for _, e := range resp.Authors.Edges {
			seen[e.Node.Name]++
		}
	}

	require.Len(t, seen, authorCount, "paging must visit every author exactly once")
	for name, n := range seen {
		assert.Equal(t, 1, n, "%s appeared on more than one page", name)
	}
}

// TestGraphQLCountFields 覆盖 poemCount/authorCount 字段
func TestGraphQLCountFields(t *testing.T) {
	c, repo := setupLangTestEnv(t)
	seedVariant(t, repo, database.LangHans, "简体标题", "李白")

	t.Run("dynasty counts", func(t *testing.T) {
		var resp struct {
			Dynasties []struct {
				Name        string
				PoemCount   int
				AuthorCount int
			}
		}
		require.NoError(t, c.Post(`query { dynasties { name poemCount authorCount } }`, &resp))

		var found bool
		for _, d := range resp.Dynasties {
			if d.Name == "唐" {
				found = true
				assert.Equal(t, 1, d.PoemCount)
				assert.Equal(t, 1, d.AuthorCount)
			}
		}
		assert.True(t, found, "seeded dynasty missing from result")
	})

	t.Run("author poem count", func(t *testing.T) {
		var resp struct {
			Authors struct {
				Edges []struct {
					Node struct {
						Name      string
						PoemCount int
					}
				}
			}
		}
		require.NoError(t, c.Post(`query { authors { edges { node { name poemCount } } } }`, &resp))
		require.Len(t, resp.Authors.Edges, 1)
		assert.Equal(t, 1, resp.Authors.Edges[0].Node.PoemCount)
	})

	t.Run("poetry type poem count", func(t *testing.T) {
		var resp struct {
			PoemTypes []struct {
				Name      string
				PoemCount int
			}
		}
		require.NoError(t, c.Post(`query { poemTypes { name poemCount } }`, &resp))
		assert.NotEmpty(t, resp.PoemTypes, "poetry types are seeded by the schema")
	})
}
