package database

import "time"

// Dynasty represents a historical dynasty
type Dynasty struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	NameEn    *string   `json:"name_en,omitempty"`
	StartYear *int      `json:"start_year,omitempty"`
	EndYear   *int      `json:"end_year,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// Author represents a poet or author
type Author struct {
	ID             int64     `json:"id"`
	Name           string    `json:"name"`
	NamePinyin     *string   `json:"name_pinyin,omitempty"`
	NamePinyinAbbr *string   `json:"name_pinyin_abbr,omitempty"`
	DynastyID      *int64    `json:"dynasty_id,omitempty"`
	Description    *string   `json:"description,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

// PoetryType represents a type of poetry
type PoetryType struct {
	ID           int64     `json:"id"`
	Name         string    `json:"name"`
	Category     string    `json:"category"`
	Lines        *int      `json:"lines,omitempty"`
	CharsPerLine *int      `json:"chars_per_line,omitempty"`
	Description  *string   `json:"description,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

// Poem represents a poem or ci
type Poem struct {
	ID              string    `json:"id"`
	Title           string    `json:"title"`
	TitlePinyin     *string   `json:"title_pinyin,omitempty"`
	TitlePinyinAbbr *string   `json:"title_pinyin_abbr,omitempty"`
	AuthorID        *int64    `json:"author_id,omitempty"`
	DynastyID       *int64    `json:"dynasty_id,omitempty"`
	TypeID          *int64    `json:"type_id,omitempty"`
	Content         string    `json:"content"` // JSON array of paragraphs
	Rhythmic        *string   `json:"rhythmic,omitempty"`
	RhythmicPinyin  *string   `json:"rhythmic_pinyin,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}

// PoemWithRelations includes related entities
type PoemWithRelations struct {
	Poem
	Author  *Author     `json:"author,omitempty"`
	Dynasty *Dynasty    `json:"dynasty,omitempty"`
	Type    *PoetryType `json:"type,omitempty"`
}

// AuthorWithStats includes statistics
type AuthorWithStats struct {
	Author
	PoemCount int `json:"poem_count"`
}

// DynastyWithStats includes statistics
type DynastyWithStats struct {
	Dynasty
	PoemCount   int `json:"poem_count"`
	AuthorCount int `json:"author_count"`
}

// PoetryTypeWithStats includes statistics
type PoetryTypeWithStats struct {
	PoetryType
	PoemCount int `json:"poem_count"`
}

// Statistics holds overall statistics
type Statistics struct {
	TotalPoems     int                   `json:"total_poems"`
	TotalAuthors   int                   `json:"total_authors"`
	TotalDynasties int                   `json:"total_dynasties"`
	PoemsByDynasty []DynastyWithStats    `json:"poems_by_dynasty"`
	PoemsByType    []PoetryTypeWithStats `json:"poems_by_type"`
}

// PageInfo represents pagination information
type PageInfo struct {
	HasNextPage     bool    `json:"has_next_page"`
	HasPreviousPage bool    `json:"has_previous_page"`
	StartCursor     *string `json:"start_cursor,omitempty"`
	EndCursor       *string `json:"end_cursor,omitempty"`
}

// PoemConnection represents a paginated list of poems
type PoemConnection struct {
	Edges      []PoemEdge `json:"edges"`
	PageInfo   PageInfo   `json:"page_info"`
	TotalCount int        `json:"total_count"`
}

// PoemEdge represents a single poem in a connection
type PoemEdge struct {
	Node   PoemWithRelations `json:"node"`
	Cursor string            `json:"cursor"`
}

// AuthorConnection represents a paginated list of authors
type AuthorConnection struct {
	Edges      []AuthorEdge `json:"edges"`
	PageInfo   PageInfo     `json:"page_info"`
	TotalCount int          `json:"total_count"`
}

// AuthorEdge represents a single author in a connection
type AuthorEdge struct {
	Node   AuthorWithStats `json:"node"`
	Cursor string          `json:"cursor"`
}
