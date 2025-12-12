package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/palemoky/chinese-poetry-api/internal/database"
)

// DynastyHandler handles dynasty-related requests
type DynastyHandler struct {
	repo *database.Repository
}

// NewDynastyHandler creates a new dynasty handler
func NewDynastyHandler(repo *database.Repository) *DynastyHandler {
	return &DynastyHandler{repo: repo}
}

// ListDynasties returns a list of dynasties
// Supports ?lang=zh-Hans (default) or ?lang=zh-Hant
func (h *DynastyHandler) ListDynasties(c *gin.Context) {
	lang := parseLang(c)
	repo := h.repo.WithLang(lang)

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

// GetDynasty returns a specific dynasty by ID
// Supports ?lang=zh-Hans (default) or ?lang=zh-Hant
func (h *DynastyHandler) GetDynasty(c *gin.Context) {
	lang := parseLang(c)
	repo := h.repo.WithLang(lang)

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
