package handler

import (
	"math/rand"
	"net/http"

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

// formatPoem formats a poem into a detailed map structure
func formatPoem(poem *database.Poem) map[string]any {
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

	return map[string]any{
		"id":                poem.ID,
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

// ListPoems retrieves a paginated list of poems
func (h *PoemHandler) ListPoems(c *gin.Context) {
	pagination := ParsePagination(c)

	poems, err := h.repo.ListPoems(pagination.PageSize, pagination.Offset())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to retrieve poems",
		})
		return
	}

	// Get total count
	total, err := h.repo.CountPoems()
	if err != nil {
		total = 0
	}

	// Map keys to response format
	data := make([]map[string]any, len(poems))
	for i, poem := range poems {
		data[i] = formatPoem(&poem)
	}

	c.JSON(http.StatusOK, NewPaginationResponse(data, pagination, int64(total)))
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
	pagination := ParsePagination(c)

	result, err := h.search.Search(search.SearchParams{
		Query:      query,
		SearchType: searchType,
		Page:       pagination.Page,
		PageSize:   pagination.PageSize,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "search failed",
		})
		return
	}

	c.JSON(http.StatusOK, NewPaginationResponse(result.Poems, pagination, int64(result.TotalCount)))
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
	// Using ListPoems with limit 1 and random offset is better than search for random
	poems, err := h.repo.ListPoems(1, offset)

	if err != nil || len(poems) == 0 {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to get random poem",
		})
		return
	}

	c.JSON(http.StatusOK, formatPoem(&poems[0]))
}
