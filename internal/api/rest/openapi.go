package rest

import (
	_ "embed"
	"net/http"

	"github.com/gin-gonic/gin"
)

// openAPISpec 是 REST API 的 OpenAPI 文档，随二进制一起发布。
// 与路由、参数、响应结构的一致性由 openapi_test.go 校验。
//
//go:embed openapi.yaml
var openAPISpec []byte

// serveOpenAPISpec 返回 OpenAPI 文档。
//
// 不走数据缓存中间件：那里的 ETag 跟随数据库文件，而文档随程序版本变化，
// 只升级程序不换数据时，客户端会凭旧 ETag 拿到 304 而看不到新文档。
func serveOpenAPISpec(c *gin.Context) {
	c.Header("Cache-Control", "no-cache")
	c.Data(http.StatusOK, "application/yaml; charset=utf-8", openAPISpec)
}
