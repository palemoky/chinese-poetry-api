package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/palemoky/chinese-poetry-api/internal/database"
)

// AuthorHandler 处理作者相关的请求。
type AuthorHandler struct {
	repo *database.Repository
}

// NewAuthorHandler 创建作者 handler。
func NewAuthorHandler(repo *database.Repository) *AuthorHandler {
	return &AuthorHandler{repo: repo}
}

// ListAuthors 分页返回作者列表。
// 语言：?lang=zh-Hans（默认）或 ?lang=zh-Hant
func (h *AuthorHandler) ListAuthors(c *gin.Context) {
	if !checkQueryParams(c, queryLang, queryPage, queryPageSize) {
		return
	}

	lang, ok := parseLang(c)
	if !ok {
		return
	}
	repo := h.repo.WithLang(lang)

	pagination, ok := ParsePagination(c)
	if !ok {
		return
	}

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

// GetAuthor 按 ID 返回指定作者。
// 语言：?lang=zh-Hans（默认）或 ?lang=zh-Hant
func (h *AuthorHandler) GetAuthor(c *gin.Context) {
	if !checkQueryParams(c, queryLang) {
		return
	}

	lang, ok := parseLang(c)
	if !ok {
		return
	}
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

	respondOK(c, formatAuthorDetail(author))
}
