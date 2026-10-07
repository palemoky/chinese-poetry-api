package rest

import (
	stdgzip "compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/palemoky/chinese-poetry-api/internal/config"
	"github.com/palemoky/chinese-poetry-api/internal/testutil"
)

// newLimitedRouter 返回 burst 为 1 的路由：同一客户端的第二个请求即会被限流。
func newLimitedRouter(t *testing.T, trustedProxies []string) *gin.Engine {
	t.Helper()

	cfg, err := config.Load("")
	require.NoError(t, err)
	cfg.Server.Mode = gin.TestMode
	cfg.RateLimit.Enabled = true
	cfg.RateLimit.RequestsPerSecond = 0.001
	cfg.RateLimit.Burst = 1
	if trustedProxies != nil {
		cfg.Server.TrustedProxies = trustedProxies
	}

	router, err := SetupRouter(cfg, nil, nil)
	require.NoError(t, err)
	router.GET("/ping", func(c *gin.Context) { c.Status(http.StatusOK) })
	return router
}

func get(router *gin.Engine, remoteAddr, forwardedFor string) int {
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.RemoteAddr = remoteAddr
	if forwardedFor != "" {
		req.Header.Set("X-Forwarded-For", forwardedFor)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w.Code
}

// 直接来自公网的请求不能靠伪造 X-Forwarded-For 换一个限流桶。
func TestRouterIgnoresSpoofedForwardedFor(t *testing.T) {
	router := newLimitedRouter(t, nil)

	assert.Equal(t, http.StatusOK, get(router, "203.0.113.7:1000", "198.51.100.1"))
	assert.Equal(t, http.StatusTooManyRequests, get(router, "203.0.113.7:1001", "198.51.100.2"),
		"a fresh X-Forwarded-For must not reset the limit for an untrusted peer")
}

// 经由默认信任的私有网段代理转发时，按真实客户端 IP 分别限流。
func TestRouterHonoursTrustedProxy(t *testing.T) {
	router := newLimitedRouter(t, nil)

	assert.Equal(t, http.StatusOK, get(router, "172.17.0.1:1000", "198.51.100.1"))
	assert.Equal(t, http.StatusOK, get(router, "172.17.0.1:1001", "198.51.100.2"),
		"different clients behind the same trusted proxy get separate limits")
	assert.Equal(t, http.StatusTooManyRequests, get(router, "172.17.0.1:1002", "198.51.100.1"))
}

func TestRouterTrustsNoProxyWhenEmpty(t *testing.T) {
	router := newLimitedRouter(t, []string{})

	assert.Equal(t, http.StatusOK, get(router, "172.17.0.1:1000", "198.51.100.1"))
	assert.Equal(t, http.StatusTooManyRequests, get(router, "172.17.0.1:1001", "198.51.100.2"))
}

func TestRouterCachePolicies(t *testing.T) {
	cfg, err := config.Load("")
	require.NoError(t, err)
	cfg.Server.Mode = gin.TestMode
	cfg.RateLimit.Enabled = false
	cfg.Database.Path = filepath.Join(t.TempDir(), "poetry.db")
	require.NoError(t, os.WriteFile(cfg.Database.Path, []byte("data"), 0o600))

	db, repo := testutil.SetupTestDB(t)
	router, err := SetupRouter(cfg, db, repo)
	require.NoError(t, err)

	serve := func(path, ifNoneMatch string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		if ifNoneMatch != "" {
			req.Header.Set("If-None-Match", ifNoneMatch)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}

	w := serve("/api/v1/dynasties", "")
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "public, max-age=3600", w.Header().Get("Cache-Control"))
	etag := w.Header().Get("ETag")
	require.NotEmpty(t, etag)

	assert.Equal(t, http.StatusNotModified, serve("/api/v1/dynasties", etag).Code)
	assert.Equal(t, http.StatusNotModified, serve("/api/v1/stats", etag).Code)

	// 随机诗词与健康检查不能缓存，也不能因为带了 ETag 就回 304
	for _, path := range []string{"/api/v1/poems/random", "/api/v1/health"} {
		w := serve(path, etag)
		assert.NotEqual(t, http.StatusNotModified, w.Code, path)
		assert.Equal(t, "no-store", w.Header().Get("Cache-Control"), path)
	}
}

// Gin 不会替 GET 路由自动响应 HEAD，此前 curl -I 一律得到 404
func TestRouterAnswersHead(t *testing.T) {
	cfg, err := config.Load("")
	require.NoError(t, err)
	cfg.Server.Mode = gin.TestMode
	cfg.RateLimit.Enabled = false
	cfg.Database.Path = filepath.Join(t.TempDir(), "poetry.db")
	require.NoError(t, os.WriteFile(cfg.Database.Path, []byte("data"), 0o600))

	db, repo := testutil.SetupTestDB(t)
	router, err := SetupRouter(cfg, db, repo)
	require.NoError(t, err)

	for path, cacheControl := range map[string]string{
		"/api/v1/health":    "no-store",
		"/api/v1/dynasties": "public, max-age=3600",
		"/api/v1/stats":     "public, max-age=3600",
	} {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodHead, path, nil))
		assert.Equal(t, http.StatusOK, w.Code, path)
		assert.Equal(t, cacheControl, w.Header().Get("Cache-Control"), path)
	}

	// 写操作的方法仍然不接受
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/dynasties", nil))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestRouterCompressesLargeResponses(t *testing.T) {
	cfg, err := config.Load("")
	require.NoError(t, err)
	cfg.Server.Mode = gin.TestMode
	cfg.RateLimit.Enabled = false
	cfg.Database.Path = filepath.Join(t.TempDir(), "poetry.db")
	require.NoError(t, os.WriteFile(cfg.Database.Path, []byte("data"), 0o600))

	db, repo := testutil.SetupTestDB(t)
	router, err := SetupRouter(cfg, db, repo)
	require.NoError(t, err)

	serveIf := func(path, acceptEncoding, ifNoneMatch string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		if acceptEncoding != "" {
			req.Header.Set("Accept-Encoding", acceptEncoding)
		}
		if ifNoneMatch != "" {
			req.Header.Set("If-None-Match", ifNoneMatch)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}
	serve := func(path, acceptEncoding string) *httptest.ResponseRecorder {
		return serveIf(path, acceptEncoding, "")
	}

	// 体裁列表由 Migrate 预置，超过 1KB
	plain := serve("/api/v1/types", "")
	require.Equal(t, http.StatusOK, plain.Code)
	require.Greater(t, plain.Body.Len(), gzipMinLength)
	assert.Empty(t, plain.Header().Get("Content-Encoding"), "the client did not ask for gzip")

	w := serve("/api/v1/types", "gzip, deflate, br")
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "gzip", w.Header().Get("Content-Encoding"))
	assert.Contains(t, w.Header().Values("Vary"), "Accept-Encoding")
	assert.Less(t, w.Body.Len(), plain.Body.Len())

	zr, err := stdgzip.NewReader(w.Body)
	require.NoError(t, err)
	body, err := io.ReadAll(zr)
	require.NoError(t, err)
	assert.Equal(t, plain.Body.String(), string(body))

	// 压缩不影响缓存：缓存头照旧，带 ETag 回来校验时 304 没有响应体也不标压缩
	assert.Equal(t, "public, max-age=3600", w.Header().Get("Cache-Control"))
	notModified := serveIf("/api/v1/types", "gzip", w.Header().Get("ETag"))
	assert.Equal(t, http.StatusNotModified, notModified.Code)
	assert.Empty(t, notModified.Header().Get("Content-Encoding"))
	assert.Zero(t, notModified.Body.Len())

	// 太小的响应不值得压缩
	small := serve("/api/v1/health", "gzip")
	assert.Empty(t, small.Header().Get("Content-Encoding"))
	assert.JSONEq(t, `{"status":"healthy"}`, small.Body.String())
}
