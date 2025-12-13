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

// SearchPoems searches for poems by query string
func (h *PoemHandler) SearchPoems(c *gin.Context) {
	lang := parseLang(c)
	repo := h.repo.WithLang(lang)

	query := c.Query("q")
	if query == "" {
		respondError(c, http.StatusBadRequest, "query parameter 'q' is required")
		return
	}

	searchType := c.DefaultQuery("type", "all")
	pagination := ParsePagination(c)

	// Use repository's search method instead of search engine
	poems, total, err := repo.SearchPoems(query, searchType, pagination.Page, pagination.PageSize)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "search failed")
		return
	}

	data := make([]map[string]any, len(poems))
	for i, poem := range poems {
		data[i] = formatPoem(&poem)
	}

	c.JSON(http.StatusOK, NewPaginationResponse(data, pagination, total))
}

// RandomPoem returns a random poem with optional filters
// Supports ?lang=zh-Hans (default) or ?lang=zh-Hant
// Supports filters: ?author=李白&type=五言绝句&dynasty=唐
// Or by ID: ?author_id=123&type_id=456&dynasty_id=789
func (h *PoemHandler) RandomPoem(c *gin.Context) {
	lang := parseLang(c)
	repo := h.repo.WithLang(lang)

	// Parse filter parameters
	var authorID, typeID, dynastyID *int64

	// Parse author filter (by ID or name)
	if authorIDStr := c.Query("author_id"); authorIDStr != "" {
		if id, err := strconv.ParseInt(authorIDStr, 10, 64); err == nil {
			authorID = &id
		}
	} else if authorName := c.Query("author"); authorName != "" {
		// Look up author by name
		author, err := repo.GetAuthorByName(authorName)
		if err != nil {
			respondError(c, http.StatusNotFound, "author not found")
			return
		}
		authorID = &author.ID
	}

	// Parse type filter (by ID or name)
	if typeIDStr := c.Query("type_id"); typeIDStr != "" {
		if id, err := strconv.ParseInt(typeIDStr, 10, 64); err == nil {
			typeID = &id
		}
	} else if typeName := c.Query("type"); typeName != "" {
		// Look up type by name
		id, err := repo.GetPoetryTypeID(typeName)
		if err != nil {
			respondError(c, http.StatusNotFound, "poetry type not found")
			return
		}
		typeID = &id
	}

	// Parse dynasty filter (by ID or name)
	if dynastyIDStr := c.Query("dynasty_id"); dynastyIDStr != "" {
		if id, err := strconv.ParseInt(dynastyIDStr, 10, 64); err == nil {
			dynastyID = &id
		}
	} else if dynastyName := c.Query("dynasty"); dynastyName != "" {
		// Look up dynasty by name
		dynasty, err := repo.GetDynastyByName(dynastyName)
		if err != nil {
			respondError(c, http.StatusNotFound, "dynasty not found")
			return
		}
		dynastyID = &dynasty.ID
	}

	// Get count of poems matching filters
	_, count, err := repo.ListPoemsWithFilter(1, 0, dynastyID, authorID, typeID)
	if err != nil || count == 0 {
		respondError(c, http.StatusNotFound, "no poems found matching the criteria")
		return
	}

	// Get a random poem from the filtered set
	offset := rand.Intn(count)
	poems, _, err := repo.ListPoemsWithFilter(1, offset, dynastyID, authorID, typeID)

	if err != nil || len(poems) == 0 {
		respondError(c, http.StatusInternalServerError, "failed to get random poem")
		return
	}

	c.JSON(http.StatusOK, formatPoem(&poems[0]))
}
