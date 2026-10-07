package middleware

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// RequestLogger 返回用 zap 记录访问日志的中间件。
//
// 取代 gin.Logger：后者输出的是自有的纯文本格式，与服务其余部分的 zap 结构化日志
// 混在一起，无法按字段检索。日志级别随状态码升高（5xx 为 Error、4xx 为 Warn），
// 便于直接按级别过滤出失败的请求。
//
// skipPaths 中的路径在成功时不记录，用于 Docker HEALTHCHECK 这类每隔几十秒
// 就打一次的探活请求；失败的探活照常记录。
func RequestLogger(l *zap.Logger, skipPaths ...string) gin.HandlerFunc {
	skip := make(map[string]struct{}, len(skipPaths))
	for _, p := range skipPaths {
		skip[p] = struct{}{}
	}

	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		status := c.Writer.Status()
		if _, ok := skip[c.Request.URL.Path]; ok && status < http.StatusBadRequest {
			return
		}

		level := zapcore.InfoLevel
		switch {
		case status >= http.StatusInternalServerError:
			level = zapcore.ErrorLevel
		case status >= http.StatusBadRequest:
			level = zapcore.WarnLevel
		}

		fields := []zap.Field{
			zap.String("method", c.Request.Method),
			zap.String("path", c.Request.URL.Path),
			zap.String("query", c.Request.URL.RawQuery),
			zap.Int("status", status),
			zap.Duration("latency", time.Since(start)),
			zap.String("client_ip", c.ClientIP()),
			zap.Int("bytes", c.Writer.Size()),
		}
		if errs := c.Errors.ByType(gin.ErrorTypePrivate).String(); errs != "" {
			fields = append(fields, zap.String("errors", errs))
		}

		l.Log(level, "request", fields...)
	}
}
