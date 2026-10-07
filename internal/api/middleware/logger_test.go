package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestRequestLogger(t *testing.T) {
	gin.SetMode(gin.TestMode)
	core, logs := observer.New(zapcore.DebugLevel)

	router := gin.New()
	router.Use(RequestLogger(zap.New(core), "/health"))
	router.GET("/ok", func(c *gin.Context) { c.String(http.StatusOK, "hi") })
	router.GET("/bad", func(c *gin.Context) { c.Status(http.StatusBadRequest) })
	router.GET("/boom", func(c *gin.Context) { c.Status(http.StatusInternalServerError) })
	healthy := true
	router.GET("/health", func(c *gin.Context) {
		if healthy {
			c.Status(http.StatusOK)
		} else {
			c.Status(http.StatusServiceUnavailable)
		}
	})

	serve := func(path string) {
		router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, path, nil))
	}

	serve("/ok?q=1")
	serve("/bad")
	serve("/boom")
	serve("/health")
	healthy = false
	serve("/health")

	entries := logs.AllUntimed()
	require.Len(t, entries, 4, "successful health checks are not logged")

	assert.Equal(t, zapcore.InfoLevel, entries[0].Level)
	fields := entries[0].ContextMap()
	assert.Equal(t, "/ok", fields["path"])
	assert.Equal(t, "q=1", fields["query"])
	assert.EqualValues(t, http.StatusOK, fields["status"])
	assert.EqualValues(t, 2, fields["bytes"])

	assert.Equal(t, zapcore.WarnLevel, entries[1].Level)
	assert.Equal(t, zapcore.ErrorLevel, entries[2].Level)
	assert.Equal(t, "/health", entries[3].ContextMap()["path"], "a failing health check is still logged")
}

func TestCORS(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(CORS())
	router.GET("/x", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "*", w.Header().Get("Access-Control-Allow-Origin"))
	// 与通配来源同时出现会被浏览器拒绝
	assert.Empty(t, w.Header().Get("Access-Control-Allow-Credentials"))

	w = httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodOptions, "/x", nil))
	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.Equal(t, "86400", w.Header().Get("Access-Control-Max-Age"))
}
