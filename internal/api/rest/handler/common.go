package handler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/palemoky/chinese-poetry-api/internal/database"
)

// parseID 从 URL 路径参数中解析并校验 int64 类型的 ID。
// 成功时返回 ID 与 true；失败时直接写出错误响应并返回 false。
func parseID(c *gin.Context, param, entityName string) (int64, bool) {
	idStr := c.Param(param)
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid "+entityName+" ID")
		return 0, false
	}
	return id, true
}

// respondError 以指定状态码返回 JSON 格式的错误响应。
//
// 与限流中间件的格式一致：error 是状态码的简短说明（如 "bad request"、"not found"），
// 同一状态码下固定不变，便于客户端分类处理；message 是这次出错的具体原因，供人阅读。
func respondError(c *gin.Context, status int, message string) {
	c.JSON(status, errorBody(status, message))
}

// errorBody 构造错误响应体，也供不经 respondError 输出的错误（如健康检查）使用。
func errorBody(status int, message string) gin.H {
	return gin.H{
		"error":   strings.ToLower(http.StatusText(status)),
		"message": message,
	}
}

// respondOK 返回携带数据的 JSON 成功响应。
func respondOK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, gin.H{"data": data})
}

// parseLang 从 lang 查询参数中解析语言变体。
// 可用取值："zh-Hans"（简体）、"zh-Hant"（繁体），以及 database.LookupLang 中的别名；
// 参数缺省时默认简体。
func parseLang(c *gin.Context) (database.Lang, bool) {
	raw := c.Query(queryLang)
	if raw == "" {
		return database.LangHans, true
	}

	lang, ok := database.LookupLang(raw)
	if !ok {
		respondError(c, http.StatusBadRequest, "unsupported lang "+strconv.Quote(raw)+"; supported: zh-Hans, zh-Hant")
		return "", false
	}
	return lang, true
}
