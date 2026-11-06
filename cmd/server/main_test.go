package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/palemoky/chinese-poetry-api/internal/config"
	"github.com/palemoky/chinese-poetry-api/internal/testutil"
)

func newTestRouter(t *testing.T, playground bool) *gin.Engine {
	t.Helper()

	cfg, err := config.Load("")
	require.NoError(t, err)
	cfg.Server.Mode = gin.TestMode
	cfg.RateLimit.Enabled = false
	cfg.GraphQL.Playground = playground

	db, repo := testutil.SetupTestDB(t)
	router, err := newRouter(cfg, db, repo, nil)
	require.NoError(t, err)
	return router
}

func serve(router http.Handler, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// 三套接口都挂在同一个路由上。
func TestNewRouterServesEveryAPI(t *testing.T) {
	router := newTestRouter(t, false)

	assert.Equal(t, http.StatusOK, serve(router, http.MethodGet, "/api/v1/health", "").Code)

	w := serve(router, http.MethodPost, "/graphql", `{"query":"{ statistics { totalPoems } }"}`)
	require.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `{"data":{"statistics":{"totalPoems":0}}}`, w.Body.String())

	// MCP 无状态模式不提供事件流：GET 要回 405 而不是 404，客户端据此判断
	assert.Equal(t, http.StatusMethodNotAllowed, serve(router, http.MethodGet, "/mcp", "").Code)
}

func TestNewRouterLimitsGraphQLBody(t *testing.T) {
	router := newTestRouter(t, false)

	query := `{"query":"{ statistics { totalPoems } }","variables":{"pad":"` + strings.Repeat("x", maxGraphQLBodyBytes) + `"}}`
	assert.Equal(t, http.StatusRequestEntityTooLarge, serve(router, http.MethodPost, "/graphql", query).Code)
}

func TestNewRouterPlayground(t *testing.T) {
	assert.Equal(t, http.StatusNotFound, serve(newTestRouter(t, false), http.MethodGet, "/playground", "").Code,
		"playground is off by default")

	w := serve(newTestRouter(t, true), http.MethodGet, "/playground", "")
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Header().Get("Content-Type"), "text/html")
	assert.Contains(t, w.Body.String(), "/graphql")
}

func TestNewHTTPServerSetsTimeouts(t *testing.T) {
	handler := http.NewServeMux()
	srv := newHTTPServer(1279, handler)

	assert.Equal(t, ":1279", srv.Addr)
	assert.Same(t, handler, srv.Handler)
	assert.Equal(t, readHeaderTimeout, srv.ReadHeaderTimeout)
	assert.Equal(t, readTimeout, srv.ReadTimeout)
	assert.Equal(t, writeTimeout, srv.WriteTimeout)
	assert.Equal(t, idleTimeout, srv.IdleTimeout)
}

// 语义检索没配置或配置有误时只关掉这项功能，服务照常启动。
func TestOpenSemanticFallsBackToDisabled(t *testing.T) {
	db, _ := testutil.SetupTestDB(t)

	assert.Nil(t, openSemantic(config.SemanticConfig{}, db), "not configured")
	assert.Nil(t, openSemantic(config.SemanticConfig{
		EmbeddingURL: "http://127.0.0.1:1/v1",
		VectorsPath:  filepath.Join(t.TempDir(), "missing.db"),
	}, db), "vector file missing")

	broken := filepath.Join(t.TempDir(), "poetry-vectors.db")
	require.NoError(t, os.WriteFile(broken, []byte("not a vector file"), 0o644))
	assert.Nil(t, openSemantic(config.SemanticConfig{EmbeddingURL: "http://127.0.0.1:1/v1", VectorsPath: broken}, db),
		"unreadable vector file")
}
