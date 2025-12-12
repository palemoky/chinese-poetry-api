package handler

import (
	"net/http"

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
	pagination := ParsePagination(c)

	authors, err := h.repo.GetAuthorsWithStats(pagination.PageSize, pagination.Offset())
	if err != nil {
		respondError(c, http.StatusInternalServerError, "Failed to fetch authors")
		return
	}

	total, err := h.repo.CountAuthors()
	if err != nil {
		respondError(c, http.StatusInternalServerError, "Failed to count authors")
		return
	}

	data := make([]map[string]any, len(authors))
	for i, author := range authors {
		data[i] = formatAuthorWithStats(&author)
	}

	c.JSON(http.StatusOK, NewPaginationResponse(data, pagination, int64(total)))
}

// GetAuthor returns a specific author by ID
func (h *AuthorHandler) GetAuthor(c *gin.Context) {
	id, ok := parseID(c, "id", "author")
	if !ok {
		return
	}

	author, err := h.repo.GetAuthorByID(id)
	if err != nil {
		respondError(c, http.StatusNotFound, "Author not found")
		return
	}

	respondOK(c, formatAuthor(author))
}
