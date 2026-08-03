package handler

import (
	"math"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/palemoky/chinese-poetry-api/internal/database"
)

// PaginationParams 保存分页参数。
//
// 支持两种翻页方式，二者互斥：
//   - 页码分页（page + page_size）：可以直接跳到第 N 页，但深度受 MaxOffset 限制；
//   - 游标分页（after + page_size）：只能顺序前进，代价与位置无关，没有深度限制。
//
// After 非 nil 时表示走游标分页。
type PaginationParams struct {
	Page     int
	PageSize int
	After    *int64
}

// IsCursor 表示本次请求走的是游标分页。
func (p PaginationParams) IsCursor() bool {
	return p.After != nil
}

// Offset 换算出查询时使用的偏移量。
func (p PaginationParams) Offset() int {
	return (p.Page - 1) * p.PageSize
}

// 分页的默认值与上限。
const (
	DefaultPage     = 1
	DefaultPageSize = 20
	MaxPageSize     = 100

	// MaxOffset 限制 LIMIT/OFFSET 分页能达到的最大深度。
	//
	// OFFSET 的代价与偏移量成正比：SQLite 必须逐行读出并丢弃前 N 行，
	// 而无过滤的诗词列表走的是全表扫描（ORDER BY id 即 rowid 顺序），
	// 每行还要读出 content 这个大 JSON 字段。在 30 万首的语料上实测，
	// offset=0 约 0.6ms，offset=300000 约 158ms——差 250 倍，且完全线性。
	// 由于 page 上限原先是 math.MaxInt32，任何人都能用 ?page=1000000
	// 廉价地放大服务端开销。
	//
	// 需要越过这个深度的客户端应改用 cursor 分页（?after=），它是 O(1) 的。
	MaxOffset = 10000
)

// ParsePagination 从请求上下文中解析分页参数。
func ParsePagination(c *gin.Context) (PaginationParams, bool) {
	page, ok := parseIntQuery(c, queryPage, DefaultPage, 1, math.MaxInt32)
	if !ok {
		return PaginationParams{}, false
	}

	pageSize, ok := parseIntQuery(c, queryPageSize, DefaultPageSize, 1, MaxPageSize)
	if !ok {
		return PaginationParams{}, false
	}

	params := PaginationParams{
		Page:     page,
		PageSize: pageSize,
	}

	// 解析游标。两种翻页方式不能混用：page 与 after 各自给出一个起点，
	// 同时出现时无论采纳哪个，另一个都会被静默忽略。
	if raw := c.Query(queryAfter); raw != "" {
		if c.Query(queryPage) != "" {
			respondError(c, http.StatusBadRequest, "page and after are mutually exclusive; use one or the other")
			return PaginationParams{}, false
		}

		after, err := database.DecodePoemCursor(raw)
		if err != nil {
			respondError(c, http.StatusBadRequest, "after must be a cursor returned by this API, got "+strconv.Quote(raw))
			return PaginationParams{}, false
		}
		params.After = &after
		return params, true
	}

	// 与越界的 page_size 一样，超深分页直接报错而非截断：
	// 悄悄返回另一页的数据会让客户端以为自己拿到了请求的那一页。
	if params.Offset() > MaxOffset {
		respondError(c, http.StatusBadRequest, "pagination too deep: page * page_size must not exceed "+
			strconv.Itoa(MaxOffset)+"; use cursor pagination (after) to go further")
		return PaginationParams{}, false
	}

	return params, true
}

// NewPaginationResponse 构造统一格式的分页响应。
func NewPaginationResponse(data any, params PaginationParams, total int64) gin.H {
	pagination := gin.H{
		"page_size": params.PageSize,
		"total":     total,
	}

	// 走游标时 page/total_pages 没有意义——请求里根本没有页码，
	// 填一个算出来的数字只会让客户端以为自己知道当前在第几页。
	if !params.IsCursor() {
		pagination["page"] = params.Page
		pagination["total_pages"] = (int(total) + params.PageSize - 1) / params.PageSize
	}

	return gin.H{
		"data":       data,
		"pagination": pagination,
	}
}

// WithNextCursor 把游标信息补进分页响应，供支持游标分页的接口使用。
// nextCursor 为空表示已经没有下一页。
//
// 两种翻页方式下都会带上：客户端总是从不带 after 的第一页开始，
// 若首个响应里没有游标，就没有任何入口可以切换到游标分页。
func WithNextCursor(response gin.H, nextCursor string) gin.H {
	pagination, ok := response["pagination"].(gin.H)
	if !ok {
		return response
	}

	pagination["has_next_page"] = nextCursor != ""
	if nextCursor != "" {
		pagination["next_cursor"] = nextCursor
	}
	return response
}
