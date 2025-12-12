package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/palemoky/chinese-poetry-api/internal/database"
)

// PoetryTypeHandler handles poetry type-related requests
type PoetryTypeHandler struct {
	repo *database.Repository
}

// NewPoetryTypeHandler creates a new poetry type handler
func NewPoetryTypeHandler(repo *database.Repository) *PoetryTypeHandler {
	return &PoetryTypeHandler{repo: repo}
}

// ListPoetryTypes returns a list of poetry types
// Supports ?lang=zh-Hans (default) or ?lang=zh-Hant
func (h *PoetryTypeHandler) ListPoetryTypes(c *gin.Context) {
	lang := parseLang(c)
	repo := h.repo.WithLang(lang)

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

// GetPoetryType returns a specific poetry type by ID
// Supports ?lang=zh-Hans (default) or ?lang=zh-Hant
func (h *PoetryTypeHandler) GetPoetryType(c *gin.Context) {
	lang := parseLang(c)
	repo := h.repo.WithLang(lang)

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
