package search

import (
	"gorm.io/gorm"

	"github.com/palemoky/chinese-poetry-api/internal/database"
)

// Engine handles all search operations
type Engine struct {
	db *database.DB
}

// NewEngine creates a new search engine
func NewEngine(db *database.DB) *Engine {
	return &Engine{db: db}
}

// SearchType defines the type of search
type SearchType string

const (
	SearchTypeAll     SearchType = "all"
	SearchTypeTitle   SearchType = "title"
	SearchTypeContent SearchType = "content"
	SearchTypeAuthor  SearchType = "author"
)

// SearchParams contains search parameters
type SearchParams struct {
	Query      string
	SearchType SearchType
	Page       int
	PageSize   int
}

// SearchResult contains search results
type SearchResult struct {
	Poems      []database.Poem
	TotalCount int
	HasMore    bool
}

// Search performs a search based on the given parameters
func (e *Engine) Search(params SearchParams) (*SearchResult, error) {
	if params.Page < 1 {
		params.Page = 1
	}
	if params.PageSize < 1 {
		params.PageSize = 20
	}

	offset := (params.Page - 1) * params.PageSize

	var poems []database.Poem
	var totalCount int64

	switch params.SearchType {
	case SearchTypeTitle:
		poems, totalCount = e.searchByTitle(params.Query, params.PageSize, offset)

	case SearchTypeContent:
		poems, totalCount = e.searchByContent(params.Query, params.PageSize, offset)

	case SearchTypeAuthor:
		poems, totalCount = e.searchByAuthor(params.Query, params.PageSize, offset)

	default: // SearchTypeAll
		poems, totalCount = e.searchAll(params.Query, params.PageSize, offset)
	}

	return &SearchResult{
		Poems:      poems,
		TotalCount: int(totalCount),
		HasMore:    offset+len(poems) < int(totalCount),
	}, nil
}

// baseQuery returns a GORM query with preloaded relationships
func (e *Engine) baseQuery() *gorm.DB {
	return e.db.Model(&database.Poem{}).
		Preload("Author").
		Preload("Dynasty").
		Preload("Type")
}

// searchByTitle searches poems by title (Chinese)
func (e *Engine) searchByTitle(query string, limit, offset int) ([]database.Poem, int64) {
	pattern := "%" + query + "%"
	var poems []database.Poem
	var count int64

	db := e.baseQuery().Where("title LIKE ?", pattern)
	db.Count(&count)
	db.Limit(limit).Offset(offset).Find(&poems)

	return poems, count
}

// searchByContent searches poems by content
func (e *Engine) searchByContent(query string, limit, offset int) ([]database.Poem, int64) {
	pattern := "%" + query + "%"
	var poems []database.Poem
	var count int64

	db := e.baseQuery().Where("content LIKE ?", pattern)
	db.Count(&count)
	db.Limit(limit).Offset(offset).Find(&poems)

	return poems, count
}

// searchByAuthor searches poems by author name (Chinese)
func (e *Engine) searchByAuthor(query string, limit, offset int) ([]database.Poem, int64) {
	pattern := "%" + query + "%"
	var poems []database.Poem
	var count int64

	db := e.baseQuery().
		Joins("JOIN authors ON poems.author_id = authors.id").
		Where("authors.name LIKE ?", pattern)
	db.Count(&count)
	db.Limit(limit).Offset(offset).Find(&poems)

	return poems, count
}

// searchAll searches across title, content, and author name using LIKE
func (e *Engine) searchAll(query string, limit, offset int) ([]database.Poem, int64) {
	pattern := "%" + query + "%"
	var poems []database.Poem
	var count int64

	db := e.baseQuery().
		Joins("LEFT JOIN authors ON poems.author_id = authors.id").
		Where(
			"poems.title LIKE ? OR poems.content LIKE ? OR authors.name LIKE ?",
			pattern, pattern, pattern,
		)
	db.Count(&count)
	db.Limit(limit).Offset(offset).Find(&poems)

	return poems, count
}
