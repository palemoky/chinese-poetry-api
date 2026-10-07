package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// CORS 返回处理跨域请求的中间件。
//
// 本服务是不需要登录的公开只读 API，允许任意来源即可。
// 不设置 Access-Control-Allow-Credentials：规范不允许它与 Allow-Origin: * 同时出现，
// 浏览器会直接拒绝这样的响应，而这里本来也没有需要携带的凭证。
func CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.Writer.Header()
		h.Set("Access-Control-Allow-Origin", "*")
		h.Set("Access-Control-Allow-Headers", "Content-Type, Accept, Cache-Control, X-Requested-With")
		h.Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		// 预检结果缓存一天，避免浏览器每个 GraphQL POST 前都先发一次 OPTIONS
		h.Set("Access-Control-Max-Age", "86400")

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}
