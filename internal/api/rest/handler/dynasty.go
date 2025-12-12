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
func (h *DynastyHandler) ListDynasties(c *gin.Context) {
	dynasties, err := h.repo.GetDynastiesWithStats()
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
func (h *DynastyHandler) GetDynasty(c *gin.Context) {
	id, ok := parseID(c, "id", "dynasty")
	if !ok {
		return
	}

	dynasty, err := h.repo.GetDynastyByID(id)
	if err != nil {
		respondError(c, http.StatusNotFound, "Dynasty not found")
		return
	}

	respondOK(c, formatDynasty(dynasty))
}
