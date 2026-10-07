package graph

import (
	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/99designs/gqlgen/graphql/handler/lru"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/vektah/gqlparser/v2/ast"

	"github.com/palemoky/chinese-poetry-api/internal/database"
	"github.com/palemoky/chinese-poetry-api/internal/graph/generated"
	"github.com/palemoky/chinese-poetry-api/internal/graph/model"
)

// ServerOptions 是构造 GraphQL 服务时的可调参数。
type ServerOptions struct {
	// ComplexityLimit 是单个查询允许的最大复杂度，必须为正数。
	ComplexityLimit int
	// Introspection 控制是否允许内省查询。
	Introspection bool
}

// NewServer 构造 GraphQL 服务。
//
// 不用 handler.NewDefaultServer：它没有任何复杂度限制，而 schema 中
// Poem.author → Author.poems → Poem.author … 可以无限嵌套，
// 一个请求就能放大成成千上万次数据库查询，按请求计数的限流对此无能为力。
// 这里只启用实际用到的 POST 传输，并对分页字段按 pageSize 计算复杂度。
func NewServer(resolver *Resolver, opts ServerOptions) *handler.Server {
	cfg := generated.Config{Resolvers: resolver}
	setComplexity(&cfg.Complexity)

	srv := handler.New(generated.NewExecutableSchema(cfg))

	srv.AddTransport(transport.Options{})
	srv.AddTransport(transport.POST{})

	srv.SetQueryCache(lru.New[*ast.QueryDocument](1000))

	if opts.Introspection {
		srv.Use(extension.Introspection{})
	}
	srv.Use(extension.AutomaticPersistedQuery{
		Cache: lru.New[string](100),
	})
	srv.Use(extension.FixedComplexityLimit(opts.ComplexityLimit))

	return srv
}

// setComplexity 让分页字段的复杂度等于子字段复杂度乘以 pageSize。
//
// gqlgen 默认给每个字段计 1，不论它返回 1 条还是 100 条，
// 嵌套的分页查询因此会被严重低估；按条数相乘后，嵌套层数越深代价增长越快，
// 复杂度上限才真正能限制住一次请求触发的查询量。
func setComplexity(c *generated.ComplexityRoot) {
	c.Query.Poems = func(child int, _ *database.Lang, _ *int, pageSize *int, _ *string, _ *string, _ *string, _ *string) int {
		return pagedComplexity(child, pageSize)
	}
	c.Query.SearchPoems = func(child int, _ string, _ *database.Lang, _ *model.SearchType, _ *int, pageSize *int) int {
		return pagedComplexity(child, pageSize)
	}
	c.Query.Authors = func(child int, _ *database.Lang, _ *int, pageSize *int, _ *string) int {
		return pagedComplexity(child, pageSize)
	}
	c.Author.Poems = func(child int, _ *int, pageSize *int) int {
		return pagedComplexity(child, pageSize)
	}
}

// pagedComplexity 按实际返回条数估算分页字段的复杂度。
// pageSize 越界时按上限计，真正的越界报错交给 resolver。
func pagedComplexity(child int, pageSize *int) int {
	n := defaultPageSize
	if pageSize != nil {
		n = min(max(*pageSize, 1), maxPageSize)
	}
	return 1 + child*n
}
