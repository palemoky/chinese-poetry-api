package database

import (
	"time"

	"gorm.io/datatypes"
)

// Dynasty 表示一个历史朝代。
type Dynasty struct {
	ID        int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	Name      string    `gorm:"not null;uniqueIndex"     json:"name"`
	NameEn    *string   `                                json:"name_en,omitempty"`
	StartYear *int      `                                json:"start_year,omitempty"`
	EndYear   *int      `                                json:"end_year,omitempty"`
	CreatedAt time.Time `gorm:"autoCreateTime"           json:"created_at"`
}

// TableName 返回 Dynasty 的默认表名。
func (Dynasty) TableName() string {
	return "dynasties"
}

// Author 表示一位诗人或作者。
type Author struct {
	ID          int64     `gorm:"primaryKey;autoIncrement" json:"id"` // 自增主键
	Name        string    `gorm:"not null;uniqueIndex" json:"name"`   // 唯一索引，防止重复
	DynastyID   *int64    `gorm:"index"                json:"dynasty_id,omitempty"`
	Dynasty     *Dynasty  `gorm:"foreignKey:DynastyID" json:"dynasty,omitempty"`
	Description *string   `                            json:"description,omitempty"`
	CreatedAt   time.Time `gorm:"autoCreateTime"       json:"created_at"`
}

// TableName 返回 Author 的默认表名。
func (Author) TableName() string {
	return "authors"
}

// PoetryType 表示一种诗词体裁。
type PoetryType struct {
	ID           int64     `gorm:"primaryKey"               json:"id"`
	Name         string    `gorm:"not null;uniqueIndex"     json:"name"`
	Category     string    `gorm:"not null"                 json:"category"`
	Lines        *int      `                                json:"lines,omitempty"`
	CharsPerLine *int      `                                json:"chars_per_line,omitempty"`
	Description  *string   `                                json:"description,omitempty"`
	CreatedAt    time.Time `gorm:"autoCreateTime"           json:"created_at"`
}

// TableName 返回 PoetryType 的默认表名。
func (PoetryType) TableName() string {
	return "poetry_types"
}

// Poem 表示一首诗或词。
type Poem struct {
	ID          int64          `gorm:"primaryKey"                                                json:"id"`
	TypeID      *int64         `gorm:"index"                                                     json:"type_id,omitempty"`
	Type        *PoetryType    `gorm:"foreignKey:TypeID"                                         json:"type,omitempty"`
	Title       string         `gorm:"not null;index;uniqueIndex:idx_unique_poem,composite:title" json:"title"`
	Content     datatypes.JSON `gorm:"type:json;not null"                                        json:"content"` // 以 JSON 数组存放的正文段落
	ContentHash string         `gorm:"size:64;uniqueIndex:idx_unique_poem,composite:content_hash" json:"-"`      // 正文规整（去标点、空白）后的 SHA256，用于去重
	AuthorID    *int64         `gorm:"index"                                                     json:"author_id,omitempty"`
	Author      *Author        `gorm:"foreignKey:AuthorID"                                       json:"author,omitempty"`
	DynastyID   *int64         `gorm:"index"                                                     json:"dynasty_id,omitempty"`
	Dynasty     *Dynasty       `gorm:"foreignKey:DynastyID"                                      json:"dynasty,omitempty"`
	CreatedAt   time.Time      `gorm:"autoCreateTime"                                            json:"created_at"`

	// FirstLine 仅在导入阶段使用、不入库：无副标题的词记下首句，
	// 供同一作者同一词牌有多首时拼成「词牌·首句」区分标题。
	FirstLine string `gorm:"-" json:"-"`
}

// TableName 返回 Poem 的默认表名。
func (Poem) TableName() string {
	return "poems"
}

// AuthorWithStats 是附带统计数据的作者。
type AuthorWithStats struct {
	Author
	PoemCount int `json:"poem_count"`
}

// DynastyWithStats 是附带统计数据的朝代。
type DynastyWithStats struct {
	Dynasty
	PoemCount   int `json:"poem_count"`
	AuthorCount int `json:"author_count"`
}

// PoetryTypeWithStats 是附带统计数据的体裁。
type PoetryTypeWithStats struct {
	PoetryType
	PoemCount int `json:"poem_count"`
}

// Statistics 汇总全库的整体统计数据。
type Statistics struct {
	TotalPoems     int                   `json:"total_poems"`
	TotalAuthors   int                   `json:"total_authors"`
	TotalDynasties int                   `json:"total_dynasties"`
	PoemsByDynasty []DynastyWithStats    `json:"poems_by_dynasty"`
	PoemsByType    []PoetryTypeWithStats `json:"poems_by_type"`
}

// PageInfo 描述分页信息。
type PageInfo struct {
	HasNextPage     bool    `json:"has_next_page"`
	HasPreviousPage bool    `json:"has_previous_page"`
	StartCursor     *string `json:"start_cursor,omitempty"`
	EndCursor       *string `json:"end_cursor,omitempty"`
}

// PoemConnection 是诗词的分页结果集。
type PoemConnection struct {
	Edges      []PoemEdge `json:"edges"`
	PageInfo   PageInfo   `json:"page_info"`
	TotalCount int        `json:"total_count"`
}

// PoemEdge 是结果集中的单条诗词。
type PoemEdge struct {
	Node   Poem   `json:"node"`
	Cursor string `json:"cursor"`
}

// AuthorConnection 是作者的分页结果集。
type AuthorConnection struct {
	Edges      []AuthorEdge `json:"edges"`
	PageInfo   PageInfo     `json:"page_info"`
	TotalCount int          `json:"total_count"`
}

// AuthorEdge 是结果集中的单个作者。
type AuthorEdge struct {
	Node   AuthorWithStats `json:"node"`
	Cursor string          `json:"cursor"`
}
