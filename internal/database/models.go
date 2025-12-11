package database

import (
	"time"

	"gorm.io/datatypes"
)

// Dynasty represents a historical dynasty
type Dynasty struct {
	ID        int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	Name      string    `gorm:"not null;uniqueIndex"     json:"name"`
	NameEn    *string   `                                json:"name_en,omitempty"`
	StartYear *int      `                                json:"start_year,omitempty"`
	EndYear   *int      `                                json:"end_year,omitempty"`
	CreatedAt time.Time `gorm:"autoCreateTime"           json:"created_at"`
}

// TableName specifies the table name for Dynasty
func (Dynasty) TableName() string {
	return "dynasties"
}

// Author represents a poet or author
type Author struct {
	ID          int64     `gorm:"primaryKey;autoIncrement" json:"id"` // Auto-increment ID
	Name        string    `gorm:"not null;uniqueIndex" json:"name"`   // uniqueIndex prevents duplicates
	DynastyID   *int64    `gorm:"index"                json:"dynasty_id,omitempty"`
	Dynasty     *Dynasty  `gorm:"foreignKey:DynastyID" json:"dynasty,omitempty"`
	Description *string   `                            json:"description,omitempty"`
	CreatedAt   time.Time `gorm:"autoCreateTime"       json:"created_at"`
}

// TableName specifies the table name for Author
func (Author) TableName() string {
	return "authors"
}

// PoetryType represents a type of poetry
type PoetryType struct {
	ID           int64     `gorm:"primaryKey"               json:"id"`
	Name         string    `gorm:"not null;uniqueIndex"     json:"name"`
	Category     string    `gorm:"not null"                 json:"category"`
	Lines        *int      `                                json:"lines,omitempty"`
	CharsPerLine *int      `                                json:"chars_per_line,omitempty"`
	Description  *string   `                                json:"description,omitempty"`
	CreatedAt    time.Time `gorm:"autoCreateTime"           json:"created_at"`
}

// TableName specifies the table name for PoetryType
func (PoetryType) TableName() string {
	return "poetry_types"
}

// Poem represents a poem or ci
type Poem struct {
	ID          int64          `gorm:"primaryKey"                                                json:"id"` // Changed from string to int64
	TypeID      *int64         `gorm:"index"                                                     json:"type_id,omitempty"`
	Type        *PoetryType    `gorm:"foreignKey:TypeID"                                         json:"type,omitempty"`
	Title       string         `gorm:"not null;index;uniqueIndex:idx_unique_poem,composite:title" json:"title"`
	Rhythmic    *string        `                                                                 json:"rhythmic,omitempty"` // 词牌名 or 曲牌名
	Content     datatypes.JSON `gorm:"type:json;not null"                                        json:"content"`            // JSON array of paragraphs
	ContentHash string         `gorm:"size:64;uniqueIndex:idx_unique_poem,composite:content_hash" json:"-"`                 // SHA256 hash for deduplication
	AuthorID    *int64         `gorm:"index;uniqueIndex:idx_unique_poem,composite:author_id"     json:"author_id,omitempty"`
	Author      *Author        `gorm:"foreignKey:AuthorID"                                       json:"author,omitempty"`
	DynastyID   *int64         `gorm:"index"                                                     json:"dynasty_id,omitempty"`
	Dynasty     *Dynasty       `gorm:"foreignKey:DynastyID"                                      json:"dynasty,omitempty"`
	CreatedAt   time.Time      `gorm:"autoCreateTime"                                            json:"created_at"`
}

// TableName specifies the table name for Poem
func (Poem) TableName() string {
	return "poems"
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
	Node   Poem   `json:"node"`
	Cursor string `json:"cursor"`
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
