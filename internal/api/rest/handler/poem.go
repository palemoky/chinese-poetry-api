package handler

import (
	"encoding/json"
	"math/rand"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/palemoky/chinese-poetry-api/internal/database"
	"github.com/palemoky/chinese-poetry-api/internal/search"
)

// PoemHandler handles poem-related requests
type PoemHandler struct {
	repo   *database.Repository
	search *search.Engine
}

// NewPoemHandler creates a new poem handler
func NewPoemHandler(repo *database.Repository, searchEngine *search.Engine) *PoemHandler {
	return &PoemHandler{
		repo:   repo,
		search: searchEngine,
	}
}

// GetPoem retrieves a single poem by ID
func (h *PoemHandler) GetPoem(c *gin.Context) {
	id := c.Param("id")

	poem, err := h.repo.GetPoemByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "poem not found",
		})
		return
	}

	// Parse content JSON
	var paragraphs []string
	if err := json.Unmarshal([]byte(poem.Content), &paragraphs); err == nil {
		// Create response withanyagraphs
		response := map[string]interface{}{
			"id":         poem.ID,
			"title":      poem.Title,
			"paragraphs": paragraphs,
			"author":     poem.Author,
			"dynasty":    poem.Dynasty,
			"type":       poem.Type,
			"rhythmic":   poem.Rhythmic,
			"created_at": poem.CreatedAt,
		}
		c.JSON(http.StatusOK, response)
	} else {
		c.JSON(http.StatusOK, poem)
	}
}

// SearchPoems searches for poems
func (h *PoemHandler) SearchPoems(c *gin.Context) {
	query := c.Query("q")
	if query == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "query parameter 'q' is required",
		})
		return
	}

	searchType := search.SearchType(c.DefaultQuery("type", "all"))
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	if pageSize > 100 {
		pageSize = 100
	}

	result, err := h.search.Search(search.SearchParams{
		Query:      query,
		SearchType: searchType,
		Page:       page,
		PageSize:   pageSize,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "search failed",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"poems":       result.Poems,
		"total_count": result.TotalCount,
		"page":        page,
		"page_size":   pageSize,
		"has_more":    result.HasMore,
	})
}

// RandomPoem returns a random poem
func (h *PoemHandler) RandomPoem(c *gin.Context) {
	// Get total count
	count, err := h.repo.CountPoems()
	if err != nil || count == 0 {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to get random poem",
		})
		return
	}

	// Get random offset
	offset := rand.Intn(count)

	// This is a simplified implementation
	// In production, you'd want a more efficient method
	result, err := h.search.Search(search.SearchParams{
		Query:      "",
		SearchType: search.SearchTypeAll,
		Page:       offset + 1,
		PageSize:   1,
	})

	if err != nil || len(result.Poems) == 0 {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to get random poem",
		})
		return
	}

	c.JSON(http.StatusOK, result.Poems[0])
}
