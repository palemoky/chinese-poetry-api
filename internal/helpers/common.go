package helpers

import (
	"strconv"

	"github.com/palemoky/chinese-poetry-api/internal/database"
)

// ParseOptionalInt64 parses a string pointer to int64 pointer
// Returns nil if the string is nil or empty
func ParseOptionalInt64(s *string) (*int64, error) {
	if s == nil || *s == "" {
		return nil, nil
	}
	id, err := strconv.ParseInt(*s, 10, 64)
	if err != nil {
		return nil, err
	}
	return &id, nil
}

// ParseFilterIDs parses dynasty, author, and type IDs from string pointers
// Returns three int64 pointers and an error if parsing fails
func ParseFilterIDs(dynastyID, authorID, typeID *string) (*int64, *int64, *int64, error) {
	dID, err := ParseOptionalInt64(dynastyID)
	if err != nil {
		return nil, nil, nil, err
	}
	aID, err := ParseOptionalInt64(authorID)
	if err != nil {
		return nil, nil, nil, err
	}
	tID, err := ParseOptionalInt64(typeID)
	if err != nil {
		return nil, nil, nil, err
	}
	return dID, aID, tID, nil
}

// ParseLangString converts string to Lang enum
// Supports "zh-Hans" (simplified) and "zh-Hant" (traditional)
// Defaults to simplified Chinese
func ParseLangString(langStr string) database.Lang {
	if langStr == "zh-Hant" {
		return database.LangHant
	}
	return database.LangHans
}

// ParseLangPointer converts *Lang to Lang with default
// Returns simplified Chinese if pointer is nil
func ParseLangPointer(lang *database.Lang) database.Lang {
	if lang != nil {
		return *lang
	}
	return database.LangHans
}

// Pagination represents pagination parameters
type Pagination struct {
	Page     int
	PageSize int
}

// NewPagination creates a new Pagination with validation
// Ensures page >= 1, pageSize between 1-100, defaults to page=1, pageSize=20
func NewPagination(page, pageSize int) *Pagination {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return &Pagination{
		Page:     page,
		PageSize: pageSize,
	}
}

// Offset calculates the database offset for the current page
func (p *Pagination) Offset() int {
	return (p.Page - 1) * p.PageSize
}
