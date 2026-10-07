package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/palemoky/chinese-poetry-api/internal/database"
)

// PoetryTypeHandler 处理体裁相关的请求。
type PoetryTypeHandler struct {
	repo *database.Repository
}

// NewPoetryTypeHandler 创建体裁 handler。
func NewPoetryTypeHandler(repo *database.Repository) *PoetryTypeHandler {
	return &PoetryTypeHandler{repo: repo}
}

// ListPoetryTypes 返回体裁列表。
// 语言：?lang=zh-Hans（默认）或 ?lang=zh-Hant
func (h *PoetryTypeHandler) ListPoetryTypes(c *gin.Context) {
	if !checkQueryParams(c, queryLang) {
		return
	}

	lang, ok := parseLang(c)
	if !ok {
		return
	}
	repo := h.repo.WithContext(c.Request.Context()).WithLang(lang)

	types, err := repo.GetPoetryTypesWithStats()
	if err != nil {
		respondError(c, http.StatusInternalServerError, "Failed to fetch poetry types")
		return
	}

	data := make([]map[string]any, len(types))
	for i, t := range types {
		data[i] = formatPoetryTypeWithStats(&t)
	}

	respondOK(c, data)
}

// GetPoetryType 按 ID 返回指定体裁。
// 语言：?lang=zh-Hans（默认）或 ?lang=zh-Hant
func (h *PoetryTypeHandler) GetPoetryType(c *gin.Context) {
	if !checkQueryParams(c, queryLang) {
		return
	}

	lang, ok := parseLang(c)
	if !ok {
		return
	}
	repo := h.repo.WithContext(c.Request.Context()).WithLang(lang)

	id, ok := parseID(c, "id", "poetry type")
	if !ok {
		return
	}

	poetryType, err := repo.GetPoetryTypeByID(id)
	if err != nil {
		respondError(c, http.StatusNotFound, "Poetry type not found")
		return
	}

	respondOK(c, formatPoetryType(poetryType))
}
