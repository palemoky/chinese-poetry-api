package search

import (
	"fmt"
	"unicode"

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
	SearchTypePinyin  SearchType = "pinyin"
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
	isPinyin := isPinyinQuery(params.Query)

	var poems []database.Poem
	var totalCount int64

	switch params.SearchType {
	case SearchTypePinyin:
		poems, totalCount = e.searchByPinyin(params.Query, params.PageSize, offset)

	case SearchTypeTitle:
		if isPinyin {
			poems, totalCount = e.searchByTitlePinyin(params.Query, params.PageSize, offset)
		} else {
			poems, totalCount = e.searchByTitle(params.Query, params.PageSize, offset)
		}

	case SearchTypeContent:
		poems, totalCount = e.searchByContent(params.Query, params.PageSize, offset)

	case SearchTypeAuthor:
		if isPinyin {
			poems, totalCount = e.searchByAuthorPinyin(params.Query, params.PageSize, offset)
		} else {
			poems, totalCount = e.searchByAuthor(params.Query, params.PageSize, offset)
		}

	default: // SearchTypeAll
		if isPinyin {
			poems, totalCount = e.searchByPinyin(params.Query, params.PageSize, offset)
		} else {
			// FTS5 requires raw SQL
			return e.searchByFTS(params.Query, params.PageSize, offset)
		}
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

// searchByTitlePinyin searches poems by title pinyin
func (e *Engine) searchByTitlePinyin(query string, limit, offset int) ([]database.Poem, int64) {
	pattern := "%" + query + "%"
	var poems []database.Poem
	var count int64

	db := e.baseQuery().Where("title_pinyin LIKE ? OR title_pinyin_abbr LIKE ?", pattern, pattern)
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

// searchByAuthorPinyin searches poems by author pinyin
func (e *Engine) searchByAuthorPinyin(query string, limit, offset int) ([]database.Poem, int64) {
	pattern := "%" + query + "%"
	var poems []database.Poem
	var count int64

	db := e.baseQuery().
		Joins("JOIN authors ON poems.author_id = authors.id").
		Where("authors.name_pinyin LIKE ? OR authors.name_pinyin_abbr LIKE ?", pattern, pattern)
	db.Count(&count)
	db.Limit(limit).Offset(offset).Find(&poems)

	return poems, count
}

// searchByPinyin searches by any pinyin field (title, author)
func (e *Engine) searchByPinyin(query string, limit, offset int) ([]database.Poem, int64) {
	pattern := "%" + query + "%"
	var poems []database.Poem
	var count int64

	db := e.baseQuery().
		Joins("LEFT JOIN authors ON poems.author_id = authors.id").
		Where(
			"poems.title_pinyin LIKE ? OR poems.title_pinyin_abbr LIKE ? OR authors.name_pinyin LIKE ? OR authors.name_pinyin_abbr LIKE ?",
			pattern, pattern, pattern, pattern,
		)
	db.Count(&count)
	db.Limit(limit).Offset(offset).Find(&poems)

	return poems, count
}

// searchByFTS uses SQLite FTS5 for full-text search (requires raw SQL)
func (e *Engine) searchByFTS(query string, limit, offset int) (*SearchResult, error) {
	// Count query
	var totalCount int
	countSQL := `SELECT COUNT(DISTINCT poem_id) FROM poems_fts WHERE poems_fts MATCH ?`
	if err := e.db.Raw(countSQL, query).Scan(&totalCount).Error; err != nil {
		return nil, fmt.Errorf("failed to count FTS results: %w", err)
	}

	// Search query - get poem IDs with ranking
	var poemIDs []int64
	searchSQL := `
		SELECT DISTINCT poem_id
		FROM poems_fts
		WHERE poems_fts MATCH ?
		ORDER BY rank
		LIMIT ? OFFSET ?
	`
	if err := e.db.Raw(searchSQL, query, limit, offset).Scan(&poemIDs).Error; err != nil {
		return nil, fmt.Errorf("failed to execute FTS search: %w", err)
	}

	if len(poemIDs) == 0 {
		return &SearchResult{
			Poems:      []database.Poem{},
			TotalCount: totalCount,
			HasMore:    false,
		}, nil
	}

	// Load full poems with relationships, preserving FTS rank order
	var poems []database.Poem
	if err := e.db.Preload("Author").Preload("Dynasty").Preload("Type").
		Where("id IN ?", poemIDs).Find(&poems).Error; err != nil {
		return nil, fmt.Errorf("failed to load poems: %w", err)
	}

	// Preserve original FTS rank order
	poemMap := make(map[int64]database.Poem, len(poems))
	for _, p := range poems {
		poemMap[p.ID] = p
	}

	orderedPoems := make([]database.Poem, 0, len(poemIDs))
	for _, id := range poemIDs {
		if poem, ok := poemMap[id]; ok {
			orderedPoems = append(orderedPoems, poem)
		}
	}

	return &SearchResult{
		Poems:      orderedPoems,
		TotalCount: totalCount,
		HasMore:    offset+len(orderedPoems) < totalCount,
	}, nil
}

// isPinyinQuery checks if a query string is pinyin
func isPinyinQuery(s string) bool {
	if s == "" {
		return false
	}

	letterCount := 0
	totalCount := 0

	for _, r := range s {
		if unicode.IsSpace(r) {
			continue
		}
		totalCount++
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			letterCount++
		}
	}

	// If more than 50% are ASCII letters, consider it pinyin
	return totalCount > 0 && float64(letterCount)/float64(totalCount) > 0.5
}
