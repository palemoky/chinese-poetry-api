package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// BodyLimit 限制请求体的大小，超出 limit 字节的请求以 413 拒绝。
//
// gqlgen 的 POST 传输会先把整个请求体读进内存再解析，本身不设上限；
// 读超时只能限制慢速发送，带宽足够的客户端仍能在超时前送来上百 MB。
// 声明了 Content-Length 的请求直接按声明判断，未声明的（分块传输）
// 读到超限时 MaxBytesReader 会报错，由下游当作读取失败处理。
func BodyLimit(limit int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.ContentLength > limit {
			c.AbortWithStatusJSON(http.StatusRequestEntityTooLarge, gin.H{
				"error":   "request body too large",
				"message": "request body must not exceed the size limit",
			})
			return
		}

		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, limit)
		c.Next()
	}
}
