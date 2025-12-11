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
	data := make([]map[string]any, len(authors))
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

		data[i] = map[string]any{
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

	c.JSON(http.StatusOK, gin.H{
		"data": map[string]any{
			"id":               author.ID,
			"name":             author.Name,
			"name_pinyin":      namePinyin,
			"name_pinyin_abbr": namePinyinAbbr,
			"dynasty":          dynastyName,
		},
	})
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

	// Map keys to response format
	data := make([]map[string]any, len(poems))
	for i, poem := range poems {
		var typeData map[string]any
		if poem.Type != nil {
			typeData = map[string]any{
				"id":       poem.Type.ID,
				"name":     poem.Type.Name,
				"category": poem.Type.Category,
			}
			if poem.Type.Description != nil {
				typeData["description"] = *poem.Type.Description
			}
		}

		var authorData map[string]any
		if poem.Author != nil {
			a := poem.Author
			namePinyin := ""
			if a.NamePinyin != nil {
				namePinyin = *a.NamePinyin
			}
			namePinyinAbbr := ""
			if a.NamePinyinAbbr != nil {
				namePinyinAbbr = *a.NamePinyinAbbr
			}
			authorData = map[string]any{
				"id":               a.ID,
				"name":             a.Name,
				"name_pinyin":      namePinyin,
				"name_pinyin_abbr": namePinyinAbbr,
			}
		}

		var dynastyData map[string]any
		if poem.Dynasty != nil {
			d := poem.Dynasty
			dynastyData = map[string]any{
				"id":   d.ID,
				"name": d.Name,
			}
			if d.NameEn != nil {
				dynastyData["name_en"] = *d.NameEn
			}
			if d.StartYear != nil {
				dynastyData["start_year"] = *d.StartYear
			}
			if d.EndYear != nil {
				dynastyData["end_year"] = *d.EndYear
			}
		}

		titlePinyin := ""
		if poem.TitlePinyin != nil {
			titlePinyin = *poem.TitlePinyin
		}
		titlePinyinAbbr := ""
		if poem.TitlePinyinAbbr != nil {
			titlePinyinAbbr = *poem.TitlePinyinAbbr
		}
		rhythmic := ""
		if poem.Rhythmic != nil {
			rhythmic = *poem.Rhythmic
		}
		rhythmicPinyin := ""
		if poem.RhythmicPinyin != nil {
			rhythmicPinyin = *poem.RhythmicPinyin
		}

		data[i] = map[string]any{
			"type":              typeData,
			"title":             poem.Title,
			"title_pinyin":      titlePinyin,
			"title_pinyin_abbr": titlePinyinAbbr,
			"rhythmic":          rhythmic,
			"rhythmic_pinyin":   rhythmicPinyin,
			"content":           poem.Content,
			"author":            authorData,
			"dynasty":           dynastyData,
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"data": data,
		"pagination": gin.H{
			"page":      page,
			"page_size": pageSize,
		},
	})
}
