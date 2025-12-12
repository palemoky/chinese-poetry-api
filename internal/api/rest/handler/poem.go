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

// ListPoems retrieves a paginated list of poems
func (h *PoemHandler) ListPoems(c *gin.Context) {
	pagination := ParsePagination(c)

	poems, err := h.repo.ListPoems(pagination.PageSize, pagination.Offset())
	if err != nil {
		respondError(c, http.StatusInternalServerError, "failed to retrieve poems")
		return
	}

	total, err := h.repo.CountPoems()
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
func (h *PoemHandler) SearchPoems(c *gin.Context) {
	query := c.Query("q")
	if query == "" {
		respondError(c, http.StatusBadRequest, "query parameter 'q' is required")
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
		respondError(c, http.StatusInternalServerError, "search failed")
		return
	}

	data := make([]map[string]any, len(result.Poems))
	for i, poem := range result.Poems {
		data[i] = formatPoem(&poem)
	}

	c.JSON(http.StatusOK, NewPaginationResponse(data, pagination, int64(result.TotalCount)))
}

// RandomPoem returns a random poem
func (h *PoemHandler) RandomPoem(c *gin.Context) {
	count, err := h.repo.CountPoems()
	if err != nil || count == 0 {
		respondError(c, http.StatusInternalServerError, "failed to get random poem")
		return
	}

	offset := rand.Intn(count)
	poems, err := h.repo.ListPoems(1, offset)

	if err != nil || len(poems) == 0 {
		respondError(c, http.StatusInternalServerError, "failed to get random poem")
		return
	}

	c.JSON(http.StatusOK, formatPoem(&poems[0]))
}
