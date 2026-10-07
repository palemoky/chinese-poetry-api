package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func newCacheRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	cached := router.Group("", Cache(time.Hour, `W/"v1"`))
	cached.GET("/ok", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })
	cached.GET("/missing", func(c *gin.Context) { c.JSON(http.StatusNotFound, gin.H{"error": "not found"}) })
	cached.GET("/broken", func(c *gin.Context) { c.AbortWithStatus(http.StatusInternalServerError) })
	cached.POST("/ok", func(c *gin.Context) { c.Status(http.StatusOK) })
	router.GET("/random", NoStore(), func(c *gin.Context) { c.Status(http.StatusOK) })
	return router
}

func serve(router *gin.Engine, method, path, ifNoneMatch string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	if ifNoneMatch != "" {
		req.Header.Set("If-None-Match", ifNoneMatch)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func TestCacheHeadersOnSuccess(t *testing.T) {
	w := serve(newCacheRouter(), http.MethodGet, "/ok", "")
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "public, max-age=3600", w.Header().Get("Cache-Control"))
	assert.Equal(t, `W/"v1"`, w.Header().Get("ETag"))
}

func TestCacheRevalidation(t *testing.T) {
	router := newCacheRouter()

	for _, inm := range []string{`W/"v1"`, `"v1"`, `"v0", W/"v1"`, "*"} {
		w := serve(router, http.MethodGet, "/ok", inm)
		assert.Equal(t, http.StatusNotModified, w.Code, "If-None-Match: %s", inm)
		assert.Empty(t, w.Body.String())
		assert.Equal(t, `W/"v1"`, w.Header().Get("ETag"))
	}

	// 数据版本变了，要返回完整响应
	w := serve(router, http.MethodGet, "/ok", `W/"v0"`)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.NotEmpty(t, w.Body.String())
}

func TestCacheSkipsErrorsAndNonGet(t *testing.T) {
	router := newCacheRouter()

	for _, path := range []string{"/missing", "/broken"} {
		w := serve(router, http.MethodGet, path, "")
		assert.Equal(t, "no-store", w.Header().Get("Cache-Control"), path)
		assert.Empty(t, w.Header().Get("ETag"), path)
	}

	w := serve(router, http.MethodPost, "/ok", `W/"v1"`)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Empty(t, w.Header().Get("Cache-Control"))
}

func TestNoStore(t *testing.T) {
	w := serve(newCacheRouter(), http.MethodGet, "/random", `W/"v1"`)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))
}
