package middleware

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// Cache 让成功的 GET 响应可被浏览器与 CDN 缓存。
//
// 数据库只在发布新数据时整体替换，运行期间同一个 URL 的响应不会变，
// 因此可以放心缓存 maxAge；过期后客户端带 If-None-Match 回来校验，
// 只要数据没换就直接回 304，不必再查库。
//
// etag 标识当前的数据版本，为空时只发 Cache-Control。
// 只有 2xx 响应可缓存：出错（尤其 5xx 这类暂时性错误）时改为 no-store，
// 免得 CDN 把一次偶发的失败缓存下来发给所有人。
func Cache(maxAge time.Duration, etag string) gin.HandlerFunc {
	cacheControl := "public, max-age=" + strconv.Itoa(int(maxAge.Seconds()))

	return func(c *gin.Context) {
		if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead {
			c.Next()
			return
		}

		h := c.Writer.Header()
		h.Set("Cache-Control", cacheControl)
		if etag != "" {
			h.Set("ETag", etag)
			if etagMatches(c.GetHeader("If-None-Match"), etag) {
				c.AbortWithStatus(http.StatusNotModified)
				return
			}
		}

		c.Writer = &cacheWriter{ResponseWriter: c.Writer}
		c.Next()
	}
}

// NoStore 禁止缓存响应，用于随机诗词、健康检查这类每次结果都可能不同的接口。
func NoStore() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		c.Next()
	}
}

// cacheWriter 在写出非 2xx 状态码前撤掉缓存相关的响应头。
type cacheWriter struct {
	gin.ResponseWriter
}

func (w *cacheWriter) WriteHeader(code int) {
	if code < 200 || code >= 300 {
		h := w.Header()
		h.Set("Cache-Control", "no-store")
		h.Del("ETag")
	}
	w.ResponseWriter.WriteHeader(code)
}

// etagMatches 按 If-None-Match 的弱比较规则判断 etag 是否在列表中。
func etagMatches(header, etag string) bool {
	if header == "" {
		return false
	}
	want := strings.TrimPrefix(etag, "W/")
	for _, tag := range strings.Split(header, ",") {
		tag = strings.TrimSpace(tag)
		if tag == "*" || strings.TrimPrefix(tag, "W/") == want {
			return true
		}
	}
	return false
}
