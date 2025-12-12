package handler

import (
	"net/http"
	"strconv"

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

// formatDynasty formats a dynasty excluding created_at
func formatDynasty(d *database.Dynasty) map[string]any {
	result := map[string]any{
		"id":   d.ID,
		"name": d.Name,
	}
	if d.NameEn != nil {
		result["name_en"] = *d.NameEn
	}
	if d.StartYear != nil {
		result["start_year"] = *d.StartYear
	}
	if d.EndYear != nil {
		result["end_year"] = *d.EndYear
	}
	return result
}

// formatDynastyWithStats formats a dynasty with stats excluding created_at
func formatDynastyWithStats(d *database.DynastyWithStats) map[string]any {
	result := formatDynasty(&d.Dynasty)
	result["poem_count"] = d.PoemCount
	result["author_count"] = d.AuthorCount
	return result
}

// ListDynasties returns a list of dynasties
func (h *DynastyHandler) ListDynasties(c *gin.Context) {
	dynasties, err := h.repo.GetDynastiesWithStats()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch dynasties"})
		return
	}

	data := make([]map[string]any, len(dynasties))
	for i, d := range dynasties {
		data[i] = formatDynastyWithStats(&d)
	}

	c.JSON(http.StatusOK, gin.H{"data": data})
}

// GetDynasty returns a specific dynasty by ID
func (h *DynastyHandler) GetDynasty(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid dynasty ID"})
		return
	}

	dynasty, err := h.repo.GetDynastyByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Dynasty not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": formatDynasty(dynasty)})
}
