package rest

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/palemoky/chinese-poetry-api/internal/config"
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
