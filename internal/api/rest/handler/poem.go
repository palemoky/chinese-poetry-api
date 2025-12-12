package handler

import (
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

// ListPoems retrieves a paginated list of poems
// Supports ?lang=zh-Hans (default) or ?lang=zh-Hant
func (h *PoemHandler) ListPoems(c *gin.Context) {
	lang := parseLang(c)
	repo := h.repo.WithLang(lang)
	pagination := ParsePagination(c)

	poems, err := repo.ListPoems(pagination.PageSize, pagination.Offset())
	if err != nil {
		respondError(c, http.StatusInternalServerError, "failed to retrieve poems")
		return
	}

	total, err := repo.CountPoems()
	if err != nil {
		total = 0
	}

	data := make([]map[string]any, len(poems))
	for i, poem := range poems {
		data[i] = formatPoem(&poem)
	}

	c.JSON(http.StatusOK, NewPaginationResponse(data, pagination, int64(total)))
}

// SearchPoems searches for poems
// Supports ?lang=zh-Hans (default) or ?lang=zh-Hant
func (h *PoemHandler) SearchPoems(c *gin.Context) {
	query := c.Query("q")
	if query == "" {
		respondError(c, http.StatusBadRequest, "query parameter 'q' is required")
		return
	}

	lang := parseLang(c)
	repo := h.repo.WithLang(lang)
	searchType := search.SearchType(c.DefaultQuery("type", "all"))
	pagination := ParsePagination(c)

	// Note: Search engine uses the default repo, but results are filtered by lang
	result, err := h.search.Search(search.SearchParams{
		Query:      query,
		SearchType: searchType,
		Page:       pagination.Page,
		PageSize:   pagination.PageSize,
	})
	if err != nil {
		respondError(c, http.StatusInternalServerError, "search failed")
		return
	}

	// Re-fetch poems with lang-aware repo to get correct content
	data := make([]map[string]any, len(result.Poems))
	for i, poem := range result.Poems {
		// Get poem from lang-aware repo if needed for proper content
		poemID := strconv.FormatInt(poem.ID, 10)
		if p, err := repo.GetPoemByID(poemID); err == nil {
			data[i] = formatPoem(p)
		} else {
			data[i] = formatPoem(&poem)
		}
	}

	c.JSON(http.StatusOK, NewPaginationResponse(data, pagination, int64(result.TotalCount)))
}

// RandomPoem returns a random poem
// Supports ?lang=zh-Hans (default) or ?lang=zh-Hant
func (h *PoemHandler) RandomPoem(c *gin.Context) {
	lang := parseLang(c)
	repo := h.repo.WithLang(lang)

	count, err := repo.CountPoems()
	if err != nil || count == 0 {
		respondError(c, http.StatusInternalServerError, "failed to get random poem")
		return
	}

	offset := rand.Intn(count)
	poems, err := repo.ListPoems(1, offset)

	if err != nil || len(poems) == 0 {
		respondError(c, http.StatusInternalServerError, "failed to get random poem")
		return
	}

	c.JSON(http.StatusOK, formatPoem(&poems[0]))
}
