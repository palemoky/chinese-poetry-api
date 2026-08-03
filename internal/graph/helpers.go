package graph

import (
	"fmt"
	"strconv"

	"github.com/palemoky/chinese-poetry-api/internal/database"
	"github.com/palemoky/chinese-poetry-api/internal/helpers"
)

// Pagination 保存解析后的分页参数。
// After 非 nil 时表示走游标分页，此时 Page 与 Offset 无意义。
type Pagination struct {
	Page     int
	PageSize int
	Offset   int
	After    *int64
}

// IsCursor 表示本次查询走的是游标分页。
func (p Pagination) IsCursor() bool {
	return p.After != nil
}

// 分页的默认值与上限，与 REST handler 保持一致。
const (
	defaultPage     = 1
	defaultPageSize = 20
	maxPageSize     = 100

	// maxOffset 与 handler.MaxOffset 对应，理由见那里的注释：
	// OFFSET 的代价与偏移量成正比，需要更深的分页应改用 after 游标。
	maxOffset = 10000
)

// parsePagination 解析并校验分页参数，缺省时取默认值：page=1、pageSize=20，pageSize 上限 100。
//
// 越界取值一律报错而非截断，这样客户端传 pageSize: 1000 时能明确知道请求未被采纳。
// 此外，截断的做法在 resolver 直接读取参数而不调用本函数的地方等于没有上限，
// searchPoems 曾因此可以请求任意多的记录。
func parsePagination(page, pageSize *int) (Pagination, error) {
	p := defaultPage
	if page != nil {
		if *page < 1 {
			return Pagination{}, fmt.Errorf("page must be at least 1, got %d", *page)
		}
		p = *page
	}

	ps := defaultPageSize
	if pageSize != nil {
		if *pageSize < 1 || *pageSize > maxPageSize {
			return Pagination{}, fmt.Errorf("pageSize must be between 1 and %d, got %d", maxPageSize, *pageSize)
		}
		ps = *pageSize
	}

	offset := (p - 1) * ps
	if offset > maxOffset {
		return Pagination{}, fmt.Errorf("pagination too deep: page * pageSize must not exceed %d, got %d; "+
			"use cursor pagination (after) to go further", maxOffset, offset)
	}

	return Pagination{
		Page:     p,
		PageSize: ps,
		Offset:   offset,
	}, nil
}

// parsePaginationWithCursor 在 parsePagination 之上支持游标翻页，用于同时提供
// 两种翻页方式的字段。after 取上一页 pageInfo.endCursor 的值。
func parsePaginationWithCursor(page, pageSize *int, after *string) (Pagination, error) {
	if after == nil || *after == "" {
		return parsePagination(page, pageSize)
	}

	// 两种翻页方式各自给出一个起点，同时传入时无论采纳哪个，另一个都会被静默忽略。
	// 注意这里只能拒绝显式传入的 page——schema 里 page 有默认值 1，
	// 客户端只传 after 时 page 仍为 nil，不算冲突。
	if page != nil {
		return Pagination{}, fmt.Errorf("page and after are mutually exclusive; use one or the other")
	}

	pag, err := parsePagination(nil, pageSize)
	if err != nil {
		return Pagination{}, err
	}

	id, err := database.DecodePoemCursor(*after)
	if err != nil {
		return Pagination{}, fmt.Errorf("after must be a cursor returned by this API: %w", err)
	}
	pag.After = &id

	return pag, nil
}

// parseOptionalID 把可选的字符串 ID 解析为 *int64。
func parseOptionalID(id *string) (*int64, error) {
	return helpers.ParseOptionalInt64(id)
}

// parseLang 把可选的 Lang 指针转换为 Lang 取值，为 nil 时返回默认语言。
func parseLang(lang *database.Lang) database.Lang {
	return helpers.ParseLangPointer(lang)
}

// buildPoemConnection 根据诗词切片与分页信息构造 PoemConnection。
//
// 调用方若多取了一条（limit = pageSize+1）用于探测下一页，这里会把它截掉；
// 游标翻页只能这样判断是否还有下一页——totalCount 是整个结果集的大小，
// 与「当前游标之后还剩多少」无关。
func buildPoemConnection(poems []database.Poem, pag Pagination, totalCount int) *database.PoemConnection {
	overFetched := len(poems) > pag.PageSize
	if overFetched {
		poems = poems[:pag.PageSize]
	}

	// 游标取诗词 id，可直接作为下一次查询的 after 回传。
	edges := make([]database.PoemEdge, len(poems))
	for i, poem := range poems {
		edges[i] = database.PoemEdge{
			Node:   poem,
			Cursor: database.EncodePoemCursor(poem.ID),
		}
	}

	hasNextPage := overFetched || (!pag.IsCursor() && pag.Offset+len(poems) < totalCount)
	hasPreviousPage := pag.IsCursor() || pag.Page > 1

	var startCursor, endCursor *string
	if len(edges) > 0 {
		start := edges[0].Cursor
		end := edges[len(edges)-1].Cursor
		startCursor = &start
		endCursor = &end
	}

	return &database.PoemConnection{
		Edges: edges,
		PageInfo: database.PageInfo{
			HasNextPage:     hasNextPage,
			HasPreviousPage: hasPreviousPage,
			StartCursor:     startCursor,
			EndCursor:       endCursor,
		},
		TotalCount: totalCount,
	}
}

// buildAuthorConnection 根据作者切片与分页信息构造 AuthorConnection。
func buildAuthorConnection(authors []database.AuthorWithStats, pag Pagination, totalCount int) *database.AuthorConnection {
	edges := make([]database.AuthorEdge, len(authors))
	for i, author := range authors {
		edges[i] = database.AuthorEdge{
			Node:   author,
			Cursor: strconv.Itoa(pag.Offset + i),
		}
	}

	hasNextPage := pag.Offset+len(authors) < totalCount
	hasPreviousPage := pag.Page > 1

	var startCursor, endCursor *string
	if len(edges) > 0 {
		start := edges[0].Cursor
		end := edges[len(edges)-1].Cursor
		startCursor = &start
		endCursor = &end
	}

	return &database.AuthorConnection{
		Edges: edges,
		PageInfo: database.PageInfo{
			HasNextPage:     hasNextPage,
			HasPreviousPage: hasPreviousPage,
			StartCursor:     startCursor,
			EndCursor:       endCursor,
		},
		TotalCount: totalCount,
	}
}
