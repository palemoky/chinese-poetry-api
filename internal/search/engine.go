package search

import (
	"fmt"
	"unicode"

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

	// Determine if query is pinyin or Chinese
	isPinyin := isPinyinQuery(params.Query)

	var query string
	var args []any
	var countQuery string
	var countArgs []any

	switch params.SearchType {
	case SearchTypePinyin:
		// Force pinyin search
		query, args = e.buildPinyinQuery(params.Query, params.PageSize, offset)
		countQuery, countArgs = e.buildPinyinCountQuery(params.Query)

	case SearchTypeTitle:
		if isPinyin {
			query, args = e.buildTitlePinyinQuery(params.Query, params.PageSize, offset)
			countQuery, countArgs = e.buildTitlePinyinCountQuery(params.Query)
		} else {
			query, args = e.buildTitleQuery(params.Query, params.PageSize, offset)
			countQuery, countArgs = e.buildTitleCountQuery(params.Query)
		}

	case SearchTypeContent:
		query, args = e.buildContentQuery(params.Query, params.PageSize, offset)
		countQuery, countArgs = e.buildContentCountQuery(params.Query)

	case SearchTypeAuthor:
		if isPinyin {
			query, args = e.buildAuthorPinyinQuery(params.Query, params.PageSize, offset)
			countQuery, countArgs = e.buildAuthorPinyinCountQuery(params.Query)
		} else {
			query, args = e.buildAuthorQuery(params.Query, params.PageSize, offset)
			countQuery, countArgs = e.buildAuthorCountQuery(params.Query)
		}

	default: // SearchTypeAll
		if isPinyin {
			query, args = e.buildPinyinQuery(params.Query, params.PageSize, offset)
			countQuery, countArgs = e.buildPinyinCountQuery(params.Query)
		} else {
			query, args = e.buildFTSQuery(params.Query, params.PageSize, offset)
			countQuery, countArgs = e.buildFTSCountQuery(params.Query)
		}
	}

	// Execute count query
	var totalCount int
	if err := e.db.Raw(countQuery, countArgs...).Scan(&totalCount).Error; err != nil {
		return nil, fmt.Errorf("failed to count results: %w", err)
	}

	// Execute search query
	var poems []database.Poem
	if err := e.db.Raw(query, args...).Scan(&poems).Error; err != nil {
		return nil, fmt.Errorf("failed to execute search: %w", err)
	}

	// Batch preload relationships to avoid N+1 query problem
	if len(poems) > 0 {
		poemIDs := make([]int64, len(poems))
		for i, poem := range poems {
			poemIDs[i] = poem.ID
		}

		// Load all poems with relationships in a single query
		var fullPoems []database.Poem
		if err := e.db.Preload("Author").Preload("Dynasty").Preload("Type").
			Where("id IN ?", poemIDs).Find(&fullPoems).Error; err != nil {
			return nil, fmt.Errorf("failed to preload relationships: %w", err)
		}

		// Create a map for quick lookup and preserve original order
		poemMap := make(map[int64]database.Poem, len(fullPoems))
		for _, p := range fullPoems {
			poemMap[p.ID] = p
		}

		// Replace poems with fully loaded versions in original order
		for i, poem := range poems {
			if fullPoem, ok := poemMap[poem.ID]; ok {
				poems[i] = fullPoem
			}
		}
	}

	return &SearchResult{
		Poems:      poems,
		TotalCount: totalCount,
		HasMore:    offset+len(poems) < totalCount,
	}, nil
}

// FTS5 full-text search
func (e *Engine) buildFTSQuery(query string, limit, offset int) (string, []any) {
	sql := `
		SELECT DISTINCT
			p.id, p.title, p.title_pinyin, p.title_pinyin_abbr,
			p.content, p.rhythmic, p.rhythmic_pinyin, p.created_at,
			a.id, a.name, a.name_pinyin, a.name_pinyin_abbr, a.created_at,
			d.id, d.name, d.name_en, d.start_year, d.end_year, d.created_at,
			t.id, t.name, t.category, t.lines, t.chars_per_line, t.created_at
		FROM poems_fts
		JOIN poems p ON poems_fts.poem_id = p.id
		LEFT JOIN authors a ON p.author_id = a.id
		LEFT JOIN dynasties d ON p.dynasty_id = d.id
		LEFT JOIN poetry_types t ON p.type_id = t.id
		WHERE poems_fts MATCH ?
		ORDER BY rank
		LIMIT ? OFFSET ?
	`
	return sql, []any{query, limit, offset}
}

func (e *Engine) buildFTSCountQuery(query string) (string, []any) {
	sql := `SELECT COUNT(DISTINCT poem_id) FROM poems_fts WHERE poems_fts MATCH ?`
	return sql, []any{query}
}

// Pinyin search (full or abbreviation)
func (e *Engine) buildPinyinQuery(query string, limit, offset int) (string, []any) {
	pattern := "%" + query + "%"
	sql := `
		SELECT
			p.id, p.title, p.title_pinyin, p.title_pinyin_abbr,
			p.content, p.rhythmic, p.rhythmic_pinyin, p.created_at,
			a.id, a.name, a.name_pinyin, a.name_pinyin_abbr, a.created_at,
			d.id, d.name, d.name_en, d.start_year, d.end_year, d.created_at,
			t.id, t.name, t.category, t.lines, t.chars_per_line, t.created_at
		FROM poems p
		LEFT JOIN authors a ON p.author_id = a.id
		LEFT JOIN dynasties d ON p.dynasty_id = d.id
		LEFT JOIN poetry_types t ON p.type_id = t.id
		WHERE p.title_pinyin LIKE ?
			OR p.title_pinyin_abbr LIKE ?
			OR a.name_pinyin LIKE ?
			OR a.name_pinyin_abbr LIKE ?
		LIMIT ? OFFSET ?
	`
	return sql, []any{pattern, pattern, pattern, pattern, limit, offset}
}

func (e *Engine) buildPinyinCountQuery(query string) (string, []any) {
	pattern := "%" + query + "%"
	sql := `
		SELECT COUNT(*)
		FROM poems p
		LEFT JOIN authors a ON p.author_id = a.id
		WHERE p.title_pinyin LIKE ?
			OR p.title_pinyin_abbr LIKE ?
			OR a.name_pinyin LIKE ?
			OR a.name_pinyin_abbr LIKE ?
	`
	return sql, []any{pattern, pattern, pattern, pattern}
}

// Title search
func (e *Engine) buildTitleQuery(query string, limit, offset int) (string, []any) {
	pattern := "%" + query + "%"
	sql := e.getBaseQuery() + ` WHERE p.title LIKE ? LIMIT ? OFFSET ?`
	return sql, []any{pattern, limit, offset}
}

func (e *Engine) buildTitleCountQuery(query string) (string, []any) {
	pattern := "%" + query + "%"
	return `SELECT COUNT(*) FROM poems WHERE title LIKE ?`, []any{pattern}
}

func (e *Engine) buildTitlePinyinQuery(query string, limit, offset int) (string, []any) {
	pattern := "%" + query + "%"
	sql := e.getBaseQuery() + ` WHERE p.title_pinyin LIKE ? OR p.title_pinyin_abbr LIKE ? LIMIT ? OFFSET ?`
	return sql, []any{pattern, pattern, limit, offset}
}

func (e *Engine) buildTitlePinyinCountQuery(query string) (string, []any) {
	pattern := "%" + query + "%"
	return `SELECT COUNT(*) FROM poems WHERE title_pinyin LIKE ? OR title_pinyin_abbr LIKE ?`, []any{pattern, pattern}
}

// Content search
func (e *Engine) buildContentQuery(query string, limit, offset int) (string, []any) {
	pattern := "%" + query + "%"
	sql := e.getBaseQuery() + ` WHERE p.content LIKE ? LIMIT ? OFFSET ?`
	return sql, []any{pattern, limit, offset}
}

func (e *Engine) buildContentCountQuery(query string) (string, []any) {
	pattern := "%" + query + "%"
	return `SELECT COUNT(*) FROM poems WHERE content LIKE ?`, []any{pattern}
}

// Author search
func (e *Engine) buildAuthorQuery(query string, limit, offset int) (string, []any) {
	pattern := "%" + query + "%"
	sql := e.getBaseQuery() + ` WHERE a.name LIKE ? LIMIT ? OFFSET ?`
	return sql, []any{pattern, limit, offset}
}

func (e *Engine) buildAuthorCountQuery(query string) (string, []any) {
	pattern := "%" + query + "%"
	return `SELECT COUNT(*) FROM poems p JOIN authors a ON p.author_id = a.id WHERE a.name LIKE ?`, []any{pattern}
}

func (e *Engine) buildAuthorPinyinQuery(query string, limit, offset int) (string, []any) {
	pattern := "%" + query + "%"
	sql := e.getBaseQuery() + ` WHERE a.name_pinyin LIKE ? OR a.name_pinyin_abbr LIKE ? LIMIT ? OFFSET ?`
	return sql, []any{pattern, pattern, limit, offset}
}

func (e *Engine) buildAuthorPinyinCountQuery(query string) (string, []any) {
	pattern := "%" + query + "%"
	return `SELECT COUNT(*) FROM poems p JOIN authors a ON p.author_id = a.id WHERE a.name_pinyin LIKE ? OR a.name_pinyin_abbr LIKE ?`, []any{pattern, pattern}
}

func (e *Engine) getBaseQuery() string {
	return `
		SELECT
			p.id, p.title, p.title_pinyin, p.title_pinyin_abbr,
			p.content, p.rhythmic, p.rhythmic_pinyin, p.created_at,
			a.id, a.name, a.name_pinyin, a.name_pinyin_abbr, a.created_at,
			d.id, d.name, d.name_en, d.start_year, d.end_year, d.created_at,
			t.id, t.name, t.category, t.lines, t.chars_per_line, t.created_at
		FROM poems p
		LEFT JOIN authors a ON p.author_id = a.id
		LEFT JOIN dynasties d ON p.dynasty_id = d.id
		LEFT JOIN poetry_types t ON p.type_id = t.id
	`
}

// isPinyinQuery checks if a query string is pinyin
func isPinyinQuery(s string) bool {
	if s == "" {
		return false
	}

	// Count ASCII letters
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
