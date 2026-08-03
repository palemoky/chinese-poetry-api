package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/palemoky/chinese-poetry-api/internal/database"
)

// getPoems 请求 /poems 并返回状态码与解析后的响应体。
func getPoems(t *testing.T, router http.Handler, query string) (int, map[string]any) {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/poems"+query, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var response map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	return w.Code, response
}

// TestListPoemsRejectsDeepPagination 覆盖 OFFSET 分页的深度上限。
// OFFSET 的代价与偏移量成正比，page 上限原先是 math.MaxInt32，
// 一个 ?page=1000000 就能让服务端做一次全表扫描。
func TestListPoemsRejectsDeepPagination(t *testing.T) {
	router, repo := setupPoemTestRouter(t)
	router.GET("/poems", NewPoemHandler(repo).ListPoems)

	createTestPoem(t, repo, 1, "静夜思", "test content")

	t.Run("just within the limit is accepted", func(t *testing.T) {
		// offset 恰好等于 MaxOffset
		code, _ := getPoems(t, router, "?page=101&page_size=100")
		assert.Equal(t, http.StatusOK, code)
	})

	t.Run("beyond the limit is rejected", func(t *testing.T) {
		code, resp := getPoems(t, router, "?page=102&page_size=100")
		assert.Equal(t, http.StatusBadRequest, code)
		assert.Contains(t, resp["error"], "pagination too deep")
	})

	t.Run("a huge page number is rejected rather than scanned", func(t *testing.T) {
		code, _ := getPoems(t, router, "?page=1000000")
		assert.Equal(t, http.StatusBadRequest, code)
	})
}

// TestListPoemsCursorPagination 覆盖游标翻页：它不受深度限制，
// 因此必须能只靠上一页的 next_cursor 一路走到底。
func TestListPoemsCursorPagination(t *testing.T) {
	router, repo := setupPoemTestRouter(t)
	router.GET("/poems", NewPoemHandler(repo).ListPoems)

	const total = 5
	for i := range total {
		createTestPoem(t, repo, int64(i+1), "诗词"+string(rune('A'+i)), "content")
	}

	titlesOf := func(resp map[string]any) []string {
		data := resp["data"].([]any)
		titles := make([]string, len(data))
		for i, item := range data {
			titles[i] = item.(map[string]any)["title"].(string)
		}
		return titles
	}

	// 不带 after 的首个响应也要给出游标，否则客户端没有入口切换到游标翻页
	code, resp := getPoems(t, router, "?page_size=2")
	require.Equal(t, http.StatusOK, code)
	pagination := resp["pagination"].(map[string]any)
	require.Equal(t, true, pagination["has_next_page"])
	require.NotEmpty(t, pagination["next_cursor"])

	// 顺着 next_cursor 走完全部数据
	var visited []string
	visited = append(visited, titlesOf(resp)...)
	cursor := pagination["next_cursor"].(string)

	for range total { // 循环上限只为防止游标不前进时测试挂死
		code, resp = getPoems(t, router, "?page_size=2&after="+cursor)
		require.Equal(t, http.StatusOK, code)
		visited = append(visited, titlesOf(resp)...)

		pagination = resp["pagination"].(map[string]any)
		if pagination["has_next_page"] != true {
			break
		}
		cursor = pagination["next_cursor"].(string)
	}

	assert.Equal(t, []string{"诗词A", "诗词B", "诗词C", "诗词D", "诗词E"}, visited,
		"cursor pagination must visit every poem exactly once, in id order")

	// 走游标时没有页码可言，不应凭空造一个
	assert.NotContains(t, pagination, "page")
	assert.NotContains(t, pagination, "total_pages")
	// total 仍是整个结果集的大小，而非剩余条数
	assert.Equal(t, float64(total), pagination["total"])
}

func TestListPoemsRejectsBadCursor(t *testing.T) {
	router, repo := setupPoemTestRouter(t)
	router.GET("/poems", NewPoemHandler(repo).ListPoems)

	createTestPoem(t, repo, 1, "静夜思", "test content")

	t.Run("a cursor this API did not issue is rejected", func(t *testing.T) {
		// 客户端最容易犯的错：把游标当成可以自己构造的行号
		code, resp := getPoems(t, router, "?after=100")
		assert.Equal(t, http.StatusBadRequest, code)
		assert.Contains(t, resp["error"], "cursor")
	})

	t.Run("page and after cannot be combined", func(t *testing.T) {
		code, resp := getPoems(t, router, "?page=2&after="+database.EncodePoemCursor(1))
		assert.Equal(t, http.StatusBadRequest, code)
		assert.Contains(t, resp["error"], "mutually exclusive")
	})
}
