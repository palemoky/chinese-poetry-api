package graph

import (
	"strconv"

	"github.com/palemoky/chinese-poetry-api/internal/database"
	"github.com/palemoky/chinese-poetry-api/internal/helpers"
)

// Pagination holds parsed pagination parameters
type Pagination struct {
	Page     int
	PageSize int
	Offset   int
}

// parsePagination extracts and validates pagination parameters with defaults.
// Default: page=1, pageSize=20, max pageSize=100
func parsePagination(page, pageSize *int) Pagination {
	p := 1
	if page != nil && *page > 0 {
		p = *page
	}
	ps := 20
	if pageSize != nil && *pageSize > 0 {
		ps = *pageSize
		if ps > 100 {
			ps = 100
		}
	}
	return Pagination{
		Page:     p,
		PageSize: ps,
		Offset:   (p - 1) * ps,
	}
}

// parseOptionalID parses an optional string ID to int64 pointer.
// Uses common helper function.
func parseOptionalID(id *string) (*int64, error) {
	return helpers.ParseOptionalInt64(id)
}

// parseLang converts an optional Lang pointer to a Lang value.
// Uses common helper function.
func parseLang(lang *database.Lang) database.Lang {
	return helpers.ParseLangPointer(lang)
}

// buildPoemConnection creates a PoemConnection from poems slice and pagination info.
func buildPoemConnection(poems []database.Poem, pag Pagination, totalCount int) *database.PoemConnection {
	edges := make([]database.PoemEdge, len(poems))
	for i, poem := range poems {
		edges[i] = database.PoemEdge{
			Node:   poem,
			Cursor: strconv.Itoa(pag.Offset + i),
		}
	}

	hasNextPage := pag.Offset+len(poems) < totalCount
	hasPreviousPage := pag.Page > 1

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

// buildAuthorConnection creates an AuthorConnection from authors slice and pagination info.
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
