package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"

	"github.com/palemoky/chinese-poetry-api/internal/database"
	"github.com/palemoky/chinese-poetry-api/internal/testutil"
)

// setupRepo 创建测试数据库。内存库的每条连接各是一个独立的库，
// 而 HTTP 请求在各自的协程里查询，因此限制为单连接。
func setupRepo(t *testing.T) (*database.DB, *database.Repository) {
	t.Helper()

	db, repo := testutil.SetupTestDB(t)
	sqlDB, err := testutil.GormDB(db).DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	return db, repo
}

// seedPoem 写入一首五言绝句及其作者、朝代。
func seedPoem(t *testing.T, repo *database.Repository, id int64, title, author, dynasty string, content ...string) {
	t.Helper()
	seedTypedPoem(t, repo, "五言绝句", id, title, author, dynasty, content...)
}

// seedTypedPoem 写入一首指定体裁的诗词。typeName 须与 repo 的语言变体一致。
func seedTypedPoem(t *testing.T, repo *database.Repository, typeName string, id int64, title, author, dynasty string, content ...string) {
	t.Helper()

	dynastyID, err := repo.GetOrCreateDynasty(dynasty)
	require.NoError(t, err)
	authorID, err := repo.GetOrCreateAuthor(author, dynastyID)
	require.NoError(t, err)
	typeID, err := repo.GetPoetryTypeID(typeName)
	require.NoError(t, err)

	contentJSON, err := json.Marshal(content)
	require.NoError(t, err)

	require.NoError(t, repo.InsertPoem(&database.Poem{
		ID:        id,
		Title:     title,
		Content:   datatypes.JSON(contentJSON),
		AuthorID:  &authorID,
		DynastyID: &dynastyID,
		TypeID:    &typeID,
	}))
}

// connect 启动 HTTP 服务并用 SDK 的客户端连上，覆盖序列化与 schema 校验的完整链路。
func connect(t *testing.T, repo *database.Repository) *mcp.ClientSession {
	t.Helper()

	srv := httptest.NewServer(NewHandler(repo))
	t.Cleanup(srv.Close)

	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{Endpoint: srv.URL}, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close() })
	return session
}

// call 调用工具，返回结果文本与是否为工具错误。
func call(t *testing.T, session *mcp.ClientSession, name string, args map[string]any) (string, bool) {
	t.Helper()

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	require.NoError(t, err)
	require.Len(t, result.Content, 1)
	text, ok := result.Content[0].(*mcp.TextContent)
	require.True(t, ok)
	return text.Text, result.IsError
}

// callInto 调用工具并要求成功，把结果解码到 T。
func callInto[T any](t *testing.T, session *mcp.ClientSession, name string, args map[string]any) T {
	t.Helper()

	text, isError := call(t, session, name, args)
	require.False(t, isError, text)

	var out T
	require.NoError(t, json.Unmarshal([]byte(text), &out))
	return out
}

func TestListToolsAreReadOnly(t *testing.T) {
	_, repo := setupRepo(t)
	session := connect(t, repo)

	result, err := session.ListTools(context.Background(), nil)
	require.NoError(t, err)

	names := make([]string, len(result.Tools))
	for i, tool := range result.Tools {
		names[i] = tool.Name
		require.NotNil(t, tool.Annotations)
		assert.True(t, tool.Annotations.ReadOnlyHint, tool.Name)
		require.NotNil(t, tool.OutputSchema, tool.Name)
	}
	assert.ElementsMatch(t, []string{"search_poems", "list_poems", "random_poem", "get_author", "get_statistics"}, names)
}

func TestSearchPoems(t *testing.T) {
	_, repo := setupRepo(t)
	seedPoem(t, repo, 1, "静夜思", "李白", "唐", "床前明月光，", "疑是地上霜。")
	seedPoem(t, repo, 2, "春晓", "孟浩然", "唐", "春眠不觉晓，", "处处闻啼鸟。")
	session := connect(t, repo)

	t.Run("matches and keeps paragraphs", func(t *testing.T) {
		out := callInto[PoemsOutput](t, session, "search_poems", map[string]any{"query": "明月"})
		require.Len(t, out.Poems, 1)
		assert.Equal(t, PoemOutput{
			Title:   "静夜思",
			Author:  "李白",
			Dynasty: "唐",
			Type:    "五言绝句",
			Content: []string{"床前明月光，", "疑是地上霜。"},
		}, out.Poems[0])
		assert.False(t, out.HasMore)
	})

	t.Run("never exposes IDs", func(t *testing.T) {
		text, _ := call(t, session, "search_poems", map[string]any{"query": "明月"})
		assert.NotContains(t, text, `"id"`)
		assert.NotContains(t, text, `_id"`)
	})

	t.Run("no match is an empty list, not an error", func(t *testing.T) {
		text, isError := call(t, session, "search_poems", map[string]any{"query": "不存在的内容"})
		assert.False(t, isError)
		assert.Contains(t, text, `"poems":[]`)
	})

	for name, args := range map[string]map[string]any{
		"blank query":     {"query": "   "},
		"bad search_type": {"query": "明月", "search_type": "meaning"},
		"bad lang":        {"query": "明月", "lang": "en"},
		"page too large":  {"query": "明月", "page_size": maxPageSize + 1},
	} {
		t.Run(name, func(t *testing.T) {
			_, isError := call(t, session, "search_poems", args)
			assert.True(t, isError)
		})
	}
}

func TestSearchPoemsHasMore(t *testing.T) {
	_, repo := setupRepo(t)
	for i := range 3 {
		seedPoem(t, repo, int64(i+1), fmt.Sprintf("明月%d", i), "李白", "唐", "明月出天山，")
	}
	session := connect(t, repo)

	first := callInto[PoemsOutput](t, session, "search_poems", map[string]any{"query": "明月", "page_size": 2})
	assert.Len(t, first.Poems, 2)
	assert.True(t, first.HasMore)

	last := callInto[PoemsOutput](t, session, "search_poems", map[string]any{"query": "明月", "page_size": 2, "page": 2})
	assert.Len(t, last.Poems, 1)
	assert.False(t, last.HasMore)
}

func TestListPoems(t *testing.T) {
	_, repo := setupRepo(t)
	seedPoem(t, repo, 1, "静夜思", "李白", "唐", "床前明月光，")
	seedPoem(t, repo, 2, "望庐山瀑布", "李白", "唐", "日照香炉生紫烟，")
	seedPoem(t, repo, 3, "水调歌头", "苏轼", "宋", "明月几时有，")
	session := connect(t, repo)

	t.Run("by author", func(t *testing.T) {
		out := callInto[PoemsOutput](t, session, "list_poems", map[string]any{"author": "李白", "page_size": 1})
		require.Len(t, out.Poems, 1)
		assert.Equal(t, "静夜思", out.Poems[0].Title)
		assert.True(t, out.HasMore)
	})

	t.Run("by dynasty and type", func(t *testing.T) {
		out := callInto[PoemsOutput](t, session, "list_poems", map[string]any{"dynasty": "宋", "types": []string{"五言绝句"}})
		require.Len(t, out.Poems, 1)
		assert.Equal(t, "水调歌头", out.Poems[0].Title)
		assert.False(t, out.HasMore)
	})

	for name, tc := range map[string]struct {
		args map[string]any
		want string
	}{
		"unknown author":  {map[string]any{"author": "无名氏甲"}, "author not found"},
		"unknown dynasty": {map[string]any{"dynasty": "秦汉"}, "dynasty not found"},
		"unknown type":    {map[string]any{"types": []string{"十四行诗"}}, "poetry type not found"},
	} {
		t.Run(name, func(t *testing.T) {
			text, isError := call(t, session, "list_poems", tc.args)
			assert.True(t, isError)
			assert.Contains(t, text, tc.want)
		})
	}
}

func TestRandomPoem(t *testing.T) {
	_, repo := setupRepo(t)
	seedPoem(t, repo, 1, "静夜思", "李白", "唐", "床前明月光，")
	seedPoem(t, repo, 2, "春晓", "孟浩然", "唐", "春眠不觉晓，")
	session := connect(t, repo)

	t.Run("by author", func(t *testing.T) {
		out := callInto[PoemOutput](t, session, "random_poem", map[string]any{"author": "孟浩然"})
		assert.Equal(t, "春晓", out.Title)
	})

	t.Run("by char", func(t *testing.T) {
		out := callInto[PoemOutput](t, session, "random_poem", map[string]any{"char": "眠"})
		assert.Equal(t, "春晓", out.Title)
	})

	t.Run("no match", func(t *testing.T) {
		text, isError := call(t, session, "random_poem", map[string]any{"char": "龘"})
		assert.True(t, isError)
		assert.Contains(t, text, "no poems match")
	})

	t.Run("char excludes filters", func(t *testing.T) {
		text, isError := call(t, session, "random_poem", map[string]any{"char": "春", "author": "李白"})
		assert.True(t, isError)
		assert.Contains(t, text, "cannot be combined")
	})

	t.Run("char must be single", func(t *testing.T) {
		_, isError := call(t, session, "random_poem", map[string]any{"char": "春天"})
		assert.True(t, isError)
	})
}

func TestGetAuthor(t *testing.T) {
	db, repo := setupRepo(t)
	seedPoem(t, repo, 1, "静夜思", "李白", "唐", "床前明月光，")
	seedPoem(t, repo, 2, "望庐山瀑布", "李白", "唐", "日照香炉生紫烟，")
	require.NoError(t, testutil.GormDB(db).Exec(
		"UPDATE "+repo.AuthorsTable()+" SET description = ? WHERE name = ?", "字太白", "李白").Error)
	session := connect(t, repo)

	out := callInto[AuthorOutput](t, session, "get_author", map[string]any{"name": "李白"})
	assert.Equal(t, AuthorOutput{Name: "李白", Dynasty: "唐", Description: "字太白", PoemCount: 2}, out)

	text, isError := call(t, session, "get_author", map[string]any{"name": "无名氏甲"})
	assert.True(t, isError)
	assert.Contains(t, text, "author not found")
}

func TestGetStatistics(t *testing.T) {
	_, repo := setupRepo(t)
	seedPoem(t, repo, 1, "静夜思", "李白", "唐", "床前明月光，")
	session := connect(t, repo)

	out := callInto[StatisticsOutput](t, session, "get_statistics", nil)
	assert.Equal(t, 1, out.TotalPoems)
	assert.Equal(t, 1, out.TotalAuthors)
	assert.Contains(t, out.Dynasties, DynastyStats{Name: "唐", StartYear: out.Dynasties[0].StartYear, EndYear: out.Dynasties[0].EndYear, PoemCount: 1})
	require.NotEmpty(t, out.Types)
	assert.Equal(t, TypeStats{Name: "五言绝句", Category: out.Types[0].Category, PoemCount: 1}, out.Types[0])
}

func TestLangSelectsVariant(t *testing.T) {
	_, repo := setupRepo(t)
	seedPoem(t, repo, 1, "静夜思", "李白", "唐", "床前明月光，")
	seedTypedPoem(t, repo.WithLang(database.LangHant), "五言絕句", 1, "靜夜思", "李白", "唐", "床前明月光，")
	session := connect(t, repo)

	for _, lang := range []string{"zh-Hant", "hant"} {
		out := callInto[PoemsOutput](t, session, "search_poems", map[string]any{"query": "明月", "lang": lang})
		require.Len(t, out.Poems, 1)
		assert.Equal(t, "靜夜思", out.Poems[0].Title)
	}
}

func TestInternalErrorsAreNotLeaked(t *testing.T) {
	db, repo := setupRepo(t)
	session := connect(t, repo)

	require.NoError(t, testutil.GormDB(db).Exec("DROP TABLE "+repo.PoemsTable()).Error)

	text, isError := call(t, session, "list_poems", nil)
	assert.True(t, isError)
	assert.Contains(t, text, errInternal.Error())
	assert.NotContains(t, text, "no such table")
}

func TestNormalizePage(t *testing.T) {
	tests := []struct {
		name         string
		page, size   int
		wantPage     int
		wantPageSize int
		wantErr      bool
	}{
		{name: "defaults", wantPage: 1, wantPageSize: defaultPageSize},
		{name: "explicit", page: 3, size: 5, wantPage: 3, wantPageSize: 5},
		{name: "max size", size: maxPageSize, wantPage: 1, wantPageSize: maxPageSize},
		{name: "deepest page", page: 201, size: 50, wantPage: 201, wantPageSize: 50},
		{name: "negative page", page: -1, wantErr: true},
		{name: "negative size", size: -1, wantErr: true},
		{name: "size over limit", size: maxPageSize + 1, wantErr: true},
		{name: "too deep", page: 202, size: 50, wantErr: true},
		{name: "huge page does not overflow", page: math.MaxInt, size: 50, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			page, pageSize, err := normalizePage(tt.page, tt.size)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantPage, page)
			assert.Equal(t, tt.wantPageSize, pageSize)
		})
	}
}

// post 向 handler 发送原始请求。
func post(t *testing.T, h http.Handler, body string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func TestRejectsBatches(t *testing.T) {
	_, repo := setupRepo(t)
	h := NewHandler(repo)

	call := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"get_statistics","arguments":{}}}`
	for name, body := range map[string]string{
		"batch":                  "[" + call + "," + call + "]",
		"batch after whitespace": " \r\n\t[" + call + "]",
	} {
		t.Run(name, func(t *testing.T) {
			w := post(t, h, body)
			assert.Equal(t, http.StatusBadRequest, w.Code)
			assert.Contains(t, w.Body.String(), "batching")
		})
	}
}

func TestRejectsOversizedBody(t *testing.T) {
	_, repo := setupRepo(t)

	w := post(t, NewHandler(repo), strings.Repeat(" ", maxBodyBytes+1))
	assert.Equal(t, http.StatusRequestEntityTooLarge, w.Code)
}

// 反代转发过来的请求 Host 是公网域名，SDK 默认的 DNS 重绑定保护会把它们 403。
func TestAcceptsProxiedHost(t *testing.T) {
	_, repo := setupRepo(t)
	h := NewHandler(repo)

	body := `{"jsonrpc":"2.0","id":1,"method":"ping"}`
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Host = "poetry.example.com"
	addr := &localAddr{"127.0.0.1:1279"}
	req = req.WithContext(context.WithValue(req.Context(), http.LocalAddrContextKey, addr))

	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	assert.NotEqual(t, http.StatusForbidden, w.Code)
}

type localAddr struct{ addr string }

func (a *localAddr) Network() string { return "tcp" }
func (a *localAddr) String() string  { return a.addr }

func TestGetIsMethodNotAllowed(t *testing.T) {
	_, repo := setupRepo(t)

	req := httptest.NewRequest(http.MethodGet, "/mcp", nil)
	req.Header.Set("Accept", "text/event-stream")
	w := httptest.NewRecorder()
	NewHandler(repo).ServeHTTP(w, req)
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}
