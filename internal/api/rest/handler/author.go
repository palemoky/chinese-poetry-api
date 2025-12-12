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
// Supports ?lang=zh-Hans (default) or ?lang=zh-Hant
func (h *AuthorHandler) ListAuthors(c *gin.Context) {
	lang := parseLang(c)
	repo := h.repo.WithLang(lang)
	pagination := ParsePagination(c)

	authors, err := repo.GetAuthorsWithStats(pagination.PageSize, pagination.Offset())
	if err != nil {
		respondError(c, http.StatusInternalServerError, "Failed to fetch authors")
		return
	}

	total, err := repo.CountAuthors()
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
// Supports ?lang=zh-Hans (default) or ?lang=zh-Hant
func (h *AuthorHandler) GetAuthor(c *gin.Context) {
	lang := parseLang(c)
	repo := h.repo.WithLang(lang)

	id, ok := parseID(c, "id", "author")
	if !ok {
		return
	}

	author, err := repo.GetAuthorByID(id)
	if err != nil {
		respondError(c, http.StatusNotFound, "Author not found")
		return
	}

	respondOK(c, formatAuthor(author))
}
