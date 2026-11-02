package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/palemoky/chinese-poetry-api/internal/database"
	"github.com/palemoky/chinese-poetry-api/internal/logger"
	"github.com/palemoky/chinese-poetry-api/internal/semantic"
)

// defaultSemanticLimit 是语义检索默认返回的诗词数。
const defaultSemanticLimit = 20

// SemanticSearchHandler 按意思检索诗词：?q=描述&lang=zh-Hans&limit=20。
//
// 结果是一份按相关度排序的列表，不分页：靠后的结果与查询的关系越来越弱，翻页没有意义。
// searcher 为 nil 表示服务没有启用语义检索，返回 503。
func SemanticSearchHandler(repo *database.Repository, searcher *semantic.Searcher) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !checkQueryParams(c, queryLang, queryQuery, queryLimit) {
			return
		}
		if searcher == nil {
			respondError(c, http.StatusServiceUnavailable, "semantic search is not enabled on this server")
			return
		}

		lang, ok := parseLang(c)
		if !ok {
			return
		}
		repo := repo.WithContext(c.Request.Context()).WithLang(lang)

		query := c.Query(queryQuery)
		if query == "" {
			respondError(c, http.StatusBadRequest, "query parameter 'q' is required")
			return
		}

		limit := defaultSemanticLimit
		if raw := c.Query(queryLimit); raw != "" {
			n, err := strconv.Atoi(raw)
			if err != nil || n < 1 || n > semantic.MaxLimit {
				respondError(c, http.StatusBadRequest, "limit must be an integer between 1 and "+strconv.Itoa(semantic.MaxLimit))
				return
			}
			limit = n
		}

		ids, err := searcher.Search(c.Request.Context(), repo, query, limit)
		switch {
		case errors.Is(err, semantic.ErrQueryTooLong):
			respondError(c, http.StatusBadRequest, err.Error())
			return
		case errors.Is(err, semantic.ErrEmbedding):
			logger.Warn("Semantic search embedding failed", zap.Error(err))
			respondError(c, http.StatusBadGateway, "embedding service unavailable")
			return
		case err != nil:
			respondError(c, http.StatusInternalServerError, "search failed")
			return
		}

		poems, err := repo.PoemsByIDs(ids)
		if err != nil {
			respondError(c, http.StatusInternalServerError, "search failed")
			return
		}
		data := make([]map[string]any, len(poems))
		for i := range poems {
			data[i] = formatPoem(&poems[i])
		}
		c.JSON(http.StatusOK, gin.H{"data": data})
	}
}
