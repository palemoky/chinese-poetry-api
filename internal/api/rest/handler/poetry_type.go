package handler

import (
	"net/http"
	"strconv"

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
func (h *PoetryTypeHandler) ListPoetryTypes(c *gin.Context) {
	types, err := h.repo.GetPoetryTypesWithStats()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch poetry types"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": types})
}

// GetPoetryType returns a specific poetry type by ID
func (h *PoetryTypeHandler) GetPoetryType(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid poetry type ID"})
		return
	}

	poetryType, err := h.repo.GetPoetryTypeByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Poetry type not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": poetryType})
}

// GetPoetryTypePoems returns poems of a specific type
func (h *PoetryTypeHandler) GetPoetryTypePoems(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid poetry type ID"})
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
	poems, err := h.repo.GetPoemsByType(id, pageSize, offset)
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
