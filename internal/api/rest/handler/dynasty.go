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

// ListDynasties returns a list of dynasties
func (h *DynastyHandler) ListDynasties(c *gin.Context) {
	dynasties, err := h.repo.GetDynastiesWithStats()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch dynasties"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": dynasties})
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

	c.JSON(http.StatusOK, gin.H{"data": dynasty})
}

// GetDynastyPoems returns poems from a specific dynasty
func (h *DynastyHandler) GetDynastyPoems(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid dynasty ID"})
		return
	}

	// Parse pagination
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	offset := (page - 1) * pageSize

	// Get poems
	poems, err := h.repo.GetPoemsByDynasty(id, pageSize, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch poems"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": poems,
		"pagination": gin.H{
			"page":      page,
			"page_size": pageSize,
		},
	})
}
