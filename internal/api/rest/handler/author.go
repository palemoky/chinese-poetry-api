package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/palemoky/chinese-poetry-api/internal/database"
)

// AuthorHandler handles author-related requests
type AuthorHandler struct {
	repo *database.Repository
}

// NewAuthorHandler creates a new author handler
func NewAuthorHandler(repo *database.Repository) *AuthorHandler {
	return &AuthorHandler{repo: repo}
}

// ListAuthors returns a list of authors
func (h *AuthorHandler) ListAuthors(c *gin.Context) {
	// Parse pagination parameters
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	offset := (page - 1) * pageSize

	// Get authors from database
	authors, err := h.repo.GetAuthorsWithStats(pageSize, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch authors"})
		return
	}

	// Get total count
	total, err := h.repo.CountAuthors()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to count authors"})
		return
	}

	// Map to response format
	data := make([]map[string]interface{}, len(authors))
	for i, author := range authors {
		dynastyName := ""
		if author.Dynasty != nil {
			dynastyName = author.Dynasty.Name
		}

		namePinyin := ""
		if author.NamePinyin != nil {
			namePinyin = *author.NamePinyin
		}

		namePinyinAbbr := ""
		if author.NamePinyinAbbr != nil {
			namePinyinAbbr = *author.NamePinyinAbbr
		}

		data[i] = map[string]interface{}{
			"id":               author.ID,
			"name":             author.Name,
			"name_pinyin":      namePinyin,
			"name_pinyin_abbr": namePinyinAbbr,
			"dynasty":          dynastyName,
			"poem_count":       author.PoemCount,
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"data": data,
		"pagination": gin.H{
			"page":        page,
			"page_size":   pageSize,
			"total":       total,
			"total_pages": (total + pageSize - 1) / pageSize,
		},
	})
}

// GetAuthor returns a specific author by ID
func (h *AuthorHandler) GetAuthor(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid author ID"})
		return
	}

	author, err := h.repo.GetAuthorByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Author not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": author})
}

// GetAuthorPoems returns poems by a specific author
func (h *AuthorHandler) GetAuthorPoems(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid author ID"})
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
	poems, err := h.repo.GetPoemsByAuthor(id, pageSize, offset)
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
