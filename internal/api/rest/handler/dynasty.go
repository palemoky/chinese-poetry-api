package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/palemoky/chinese-poetry-api/internal/database"
)

// DynastyHandler 处理朝代相关的请求。
type DynastyHandler struct {
	repo *database.Repository
}

// NewDynastyHandler 创建朝代 handler。
func NewDynastyHandler(repo *database.Repository) *DynastyHandler {
	return &DynastyHandler{repo: repo}
}

// ListDynasties 返回朝代列表。
// 语言：?lang=zh-Hans（默认）或 ?lang=zh-Hant
func (h *DynastyHandler) ListDynasties(c *gin.Context) {
	if !checkQueryParams(c, queryLang) {
		return
	}

	lang, ok := parseLang(c)
	if !ok {
		return
	}
	repo := h.repo.WithContext(c.Request.Context()).WithLang(lang)

	dynasties, err := repo.GetDynastiesWithStats()
	if err != nil {
		respondError(c, http.StatusInternalServerError, "Failed to fetch dynasties")
		return
	}

	data := make([]map[string]any, len(dynasties))
	for i, d := range dynasties {
		data[i] = formatDynastyWithStats(&d)
	}

	respondOK(c, data)
}

// GetDynasty 按 ID 返回指定朝代。
// 语言：?lang=zh-Hans（默认）或 ?lang=zh-Hant
func (h *DynastyHandler) GetDynasty(c *gin.Context) {
	if !checkQueryParams(c, queryLang) {
		return
	}

	lang, ok := parseLang(c)
	if !ok {
		return
	}
	repo := h.repo.WithContext(c.Request.Context()).WithLang(lang)

	id, ok := parseID(c, "id", "dynasty")
	if !ok {
		return
	}

	dynasty, err := repo.GetDynastyByID(id)
	if err != nil {
		respondError(c, http.StatusNotFound, "Dynasty not found")
		return
	}

	respondOK(c, formatDynasty(dynasty))
}
