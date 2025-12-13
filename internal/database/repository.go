package database

import (
	"github.com/vbauerster/mpb/v8"
)

// RepositoryInterface defines the interface for repository operations
type RepositoryInterface interface {
	GetOrCreateDynasty(name string) (int64, error)
	GetOrCreateAuthor(name string, dynastyID int64) (int64, error)
	GetPoetryTypeID(name string) (int64, error)
	InsertPoem(poem *Poem) error
	BatchInsertPoems(poems []*Poem, batchSize int) error
	BatchInsertPoemsWithTransaction(poems []*Poem, transactionSize, batchSize int, progress *mpb.Progress) error
	UpsertPoem(poem *Poem) error
	GetPoemByID(id string) (*Poem, error)
	CountPoems() (int, error)
	CountAuthors() (int, error)
	GetStatistics() (*Statistics, error)
	ListPoems(limit, offset int) ([]Poem, error)
	ListPoemsWithFilter(limit, offset int, dynastyID, authorID, typeID *int64) ([]Poem, int, error)
	ListAuthorPoems(authorID int64, limit, offset int) ([]Poem, int, error)
	ListAuthorsWithFilter(limit, offset int, dynastyID *int64) ([]AuthorWithStats, int, error)
	SearchPoems(query string, searchType string, page, pageSize int) ([]Poem, int64, error)
}

// Repository handles database operations
type Repository struct {
	db   *DB
	lang Lang // Language variant for table selection (empty = default/legacy mode)
}

// NewRepository creates a new repository with default language (simplified)
func NewRepository(db *DB) *Repository {
	return &Repository{db: db, lang: LangHans}
}

// NewRepositoryWithLang creates a new repository for a specific language variant
func NewRepositoryWithLang(db *DB, lang Lang) *Repository {
	return &Repository{db: db, lang: lang}
}

// WithLang returns a new Repository instance with the specified language variant.
// This allows runtime language switching without modifying the original repository.
func (r *Repository) WithLang(lang Lang) *Repository {
	return &Repository{db: r.db, lang: lang}
}

// Table name helpers for this repository's language
func (r *Repository) poemsTable() string       { return PoemsTable(r.lang) }
func (r *Repository) authorsTable() string     { return AuthorsTable(r.lang) }
func (r *Repository) dynastiesTable() string   { return DynastiesTable(r.lang) }
func (r *Repository) poetryTypesTable() string { return PoetryTypesTable(r.lang) }
