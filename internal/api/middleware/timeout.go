package middleware

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"
)

// RequestTimeout 给每个请求的上下文加上截止时间。
//
// 仓储层把请求上下文传给数据库驱动，超时后正在执行的查询会被中断。
// 它与 http.Server 的 WriteTimeout 互补：后者到点只会断开连接，
// 不会停下仍在为这个请求跑的查询。
func RequestTimeout(d time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), d)
		defer cancel()

		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}
