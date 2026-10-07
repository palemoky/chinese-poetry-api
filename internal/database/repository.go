package database

import (
	"context"

	"github.com/vbauerster/mpb/v8"
	"gorm.io/gorm"
)

// RepositoryInterface 定义仓储层对外提供的操作集合。
type RepositoryInterface interface {
	GetOrCreateDynasty(name string) (int64, error)
	GetOrCreateAuthor(name string, dynastyID int64) (int64, error)
	CreateAuthorWithID(id int64, name string, dynastyID int64) (int64, error)
	SetAuthorDescriptions(descriptions map[int64]string) error
	GetPoetryTypeID(name string) (int64, error)
	GetPoetryTypeIDs(names []string) ([]int64, error)
	InsertPoem(poem *Poem) error
	BatchInsertPoems(poems []*Poem, batchSize int) error
	BatchInsertPoemsWithTransaction(poems []*Poem, transactionSize, batchSize int, progress *mpb.Progress) error
	UpsertPoem(poem *Poem) error
	GetPoemByID(id string) (*Poem, error)
	CountPoems() (int, error)
	CountAuthors() (int, error)
	GetStatistics() (*Statistics, error)
	ListPoemsWithFilter(limit, offset int, dynastyID, authorID *int64, typeIDs []int64) ([]Poem, int, error)
	ListAuthorPoems(authorID int64, limit, offset int) ([]Poem, int, error)
	ListAuthorsWithFilter(limit, offset int, dynastyID *int64) ([]AuthorWithStats, int, error)
	SearchPoems(query string, searchType string, page, pageSize int) ([]Poem, int64, error)
}

// Repository 负责所有数据库操作。
type Repository struct {
	db   *DB
	lang Lang // 决定使用哪套语言变体的表，留空表示默认（兼容旧模式）

	// ctx 随查询传给数据库驱动。请求的上下文被取消（客户端断开、请求超时）时，
	// 正在执行的 SQLite 查询会被中断，而不是继续占着连接跑完一个没人要的结果。
	ctx context.Context
}

// NewRepository 创建使用默认语言（简体）的仓储。
func NewRepository(db *DB) *Repository {
	return &Repository{db: db, lang: LangHans}
}

// NewRepositoryWithLang 创建指定语言变体的仓储。
func NewRepositoryWithLang(db *DB, lang Lang) *Repository {
	return &Repository{db: db, lang: lang}
}

// WithLang 返回一个切换了语言变体的新 Repository 实例，
// 从而支持运行时切换语言且不影响原实例。
func (r *Repository) WithLang(lang Lang) *Repository {
	return &Repository{db: r.db, lang: lang, ctx: r.ctx}
}

// WithContext 返回一个查询时携带 ctx 的新 Repository 实例，语言变体不变。
func (r *Repository) WithContext(ctx context.Context) *Repository {
	return &Repository{db: r.db, lang: r.lang, ctx: ctx}
}

// conn 返回本仓储查询所用的连接，设置了 ctx 时绑定到它。
func (r *Repository) conn() *gorm.DB {
	if r.ctx == nil {
		return r.db.DB
	}
	return r.db.DB.WithContext(r.ctx)
}

// 按本仓储的语言变体拼接表名
func (r *Repository) poemsTable() string       { return PoemsTable(r.lang) }
func (r *Repository) authorsTable() string     { return AuthorsTable(r.lang) }
func (r *Repository) dynastiesTable() string   { return DynastiesTable(r.lang) }
func (r *Repository) poetryTypesTable() string { return PoetryTypesTable(r.lang) }
func (r *Repository) poemsFtsTable() string    { return PoemsFtsTable(r.lang) }

// 供外部包（如搜索模块）使用的导出访问器
func (r *Repository) DB() *DB                { return r.db }
func (r *Repository) PoemsTable() string     { return r.poemsTable() }
func (r *Repository) AuthorsTable() string   { return r.authorsTable() }
func (r *Repository) DynastiesTable() string { return r.dynastiesTable() }
