package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/palemoky/chinese-poetry-api/internal/api/rest/handler"
	"github.com/palemoky/chinese-poetry-api/internal/database"
	"github.com/palemoky/chinese-poetry-api/internal/logger"
)

const (
	// toolTimeout 是单次工具调用的时限，与 REST 接口的 requestTimeout 一致。
	// SDK 给工具的上下文不继承 HTTP 请求的上下文，RequestTimeout 中间件管不到这里，
	// 老版本协议的请求连客户端断开都感知不到，因此必须单独设置。
	toolTimeout = 10 * time.Second

	// 结果要进入模型的上下文，默认值比 REST 接口小。
	defaultPageSize = 10
	maxPageSize     = 50
)

// searchTypes 列出 search_poems 的 search_type 合法取值。
var searchTypes = []string{"all", "title", "content", "author"}

// inputError 表示调用方可以自行修正的错误，消息会原样返回给模型；
// 其余错误只记日志，对外统一成 errInternal，避免泄露 SQL 等内部细节。
type inputError struct {
	message string
}

func (e *inputError) Error() string { return e.message }

func inputErrorf(format string, args ...any) error {
	return &inputError{message: fmt.Sprintf(format, args...)}
}

var errInternal = errors.New("internal error, please try again later")

// tools 实现各个 MCP 工具。
type tools struct {
	repo *database.Repository
}

// PoemOutput 是返回给模型的一首诗词。
type PoemOutput struct {
	Title   string   `json:"title"`
	Author  string   `json:"author,omitempty"`
	Dynasty string   `json:"dynasty,omitempty"`
	Type    string   `json:"type,omitempty"`
	Content []string `json:"content"`
}

// PoemsOutput 是一页诗词。
type PoemsOutput struct {
	Poems    []PoemOutput `json:"poems"`
	Page     int          `json:"page"`
	PageSize int          `json:"page_size"`
	HasMore  bool         `json:"has_more"`
}

// SearchPoemsInput 是 search_poems 的参数。
type SearchPoemsInput struct {
	Query      string `json:"query" jsonschema:"Literal text to find, e.g. 明月 or 静夜思"`
	SearchType string `json:"search_type,omitempty" jsonschema:"Field to search: all (default), title, content or author"`
	Lang       string `json:"lang,omitempty" jsonschema:"Script: zh-Hans (default, simplified) or zh-Hant (traditional)"`
	Page       int    `json:"page,omitempty" jsonschema:"Page number starting at 1"`
	PageSize   int    `json:"page_size,omitempty" jsonschema:"Results per page, default 10, at most 50"`
}

// ListPoemsInput 是 list_poems 的参数。
type ListPoemsInput struct {
	Author   string   `json:"author,omitempty" jsonschema:"Author name, e.g. 李白"`
	Dynasty  string   `json:"dynasty,omitempty" jsonschema:"Dynasty name, e.g. 唐. Also tells apart authors who share a name"`
	Types    []string `json:"types,omitempty" jsonschema:"Poetry type names, e.g. 五言绝句. A poem matches if it has any of them"`
	Lang     string   `json:"lang,omitempty" jsonschema:"Script: zh-Hans (default, simplified) or zh-Hant (traditional)"`
	Page     int      `json:"page,omitempty" jsonschema:"Page number starting at 1"`
	PageSize int      `json:"page_size,omitempty" jsonschema:"Results per page, default 10, at most 50"`
}

// RandomPoemInput 是 random_poem 的参数。
type RandomPoemInput struct {
	Char    string   `json:"char,omitempty" jsonschema:"A single character the poem must contain, as in the 飞花令 game. Cannot be combined with author, dynasty or types"`
	Author  string   `json:"author,omitempty" jsonschema:"Author name, e.g. 李白"`
	Dynasty string   `json:"dynasty,omitempty" jsonschema:"Dynasty name, e.g. 唐"`
	Types   []string `json:"types,omitempty" jsonschema:"Poetry type names, e.g. 五言绝句. A poem matches if it has any of them"`
	Lang    string   `json:"lang,omitempty" jsonschema:"Script: zh-Hans (default, simplified) or zh-Hant (traditional)"`
}

// GetAuthorInput 是 get_author 的参数。
type GetAuthorInput struct {
	Name    string `json:"name" jsonschema:"Author name, e.g. 苏轼"`
	Dynasty string `json:"dynasty,omitempty" jsonschema:"Dynasty name. Without it, the author with the most poems is returned when several share the name"`
	Lang    string `json:"lang,omitempty" jsonschema:"Script: zh-Hans (default, simplified) or zh-Hant (traditional)"`
}

// AuthorOutput 是 get_author 的结果。
type AuthorOutput struct {
	Name        string `json:"name"`
	Dynasty     string `json:"dynasty,omitempty"`
	Description string `json:"description,omitempty"`
	PoemCount   int    `json:"poem_count"`
}

// GetStatisticsInput 是 get_statistics 的参数。
type GetStatisticsInput struct {
	Lang string `json:"lang,omitempty" jsonschema:"Script: zh-Hans (default, simplified) or zh-Hant (traditional)"`
}

// DynastyStats 是单个朝代的统计。
type DynastyStats struct {
	Name      string `json:"name"`
	StartYear *int   `json:"start_year,omitempty"`
	EndYear   *int   `json:"end_year,omitempty"`
	PoemCount int    `json:"poem_count"`
}

// TypeStats 是单个体裁的统计。
type TypeStats struct {
	Name      string `json:"name"`
	Category  string `json:"category"`
	PoemCount int    `json:"poem_count"`
}

// StatisticsOutput 是 get_statistics 的结果。
type StatisticsOutput struct {
	TotalPoems   int            `json:"total_poems"`
	TotalAuthors int            `json:"total_authors"`
	Dynasties    []DynastyStats `json:"dynasties"`
	Types        []TypeStats    `json:"types"`
}

// addTool 注册一个只读工具，并统一设置时限与错误处理。
func addTool[In, Out any](s *mcp.Server, t *mcp.Tool, h func(context.Context, In) (Out, error)) {
	closedWorld := false
	t.Annotations = &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: &closedWorld}

	mcp.AddTool(s, t, func(ctx context.Context, _ *mcp.CallToolRequest, in In) (*mcp.CallToolResult, Out, error) {
		ctx, cancel := context.WithTimeout(ctx, toolTimeout)
		defer cancel()

		out, err := h(ctx, in)
		if err != nil {
			var zero Out
			var invalid *inputError
			if errors.As(err, &invalid) {
				return nil, zero, err
			}
			logger.Error("MCP tool failed", zap.String("tool", t.Name), zap.Error(err))
			return nil, zero, errInternal
		}
		return nil, out, nil
	})
}

func registerTools(s *mcp.Server, repo *database.Repository) {
	t := &tools{repo: repo}

	addTool(s, &mcp.Tool{
		Name:        "search_poems",
		Description: "Find poems containing a literal substring in their title, content or author name. Does not match by meaning.",
	}, t.searchPoems)
	addTool(s, &mcp.Tool{
		Name:        "list_poems",
		Description: "List poems filtered by author, dynasty and poetry type.",
	}, t.listPoems)
	addTool(s, &mcp.Tool{
		Name:        "random_poem",
		Description: "Pick one random poem, optionally by author, dynasty and poetry type, or containing a given character.",
	}, t.randomPoem)
	addTool(s, &mcp.Tool{
		Name:        "get_author",
		Description: "Look up an author's biography and poem count by name.",
	}, t.getAuthor)
	addTool(s, &mcp.Tool{
		Name:        "get_statistics",
		Description: "Get corpus totals and the dynasties and poetry types available as filters, with poem counts.",
	}, t.getStatistics)
}

// repoFor 返回绑定了上下文与语言变体的仓储，lang 为空时默认简体。
func (t *tools) repoFor(ctx context.Context, rawLang string) (*database.Repository, error) {
	lang := database.LangHans
	if rawLang != "" {
		var ok bool
		if lang, ok = database.LookupLang(rawLang); !ok {
			return nil, inputErrorf("unsupported lang %s; supported: zh-Hans, zh-Hant", strconv.Quote(rawLang))
		}
	}
	return t.repo.WithContext(ctx).WithLang(lang), nil
}

func (t *tools) searchPoems(ctx context.Context, in SearchPoemsInput) (PoemsOutput, error) {
	repo, err := t.repoFor(ctx, in.Lang)
	if err != nil {
		return PoemsOutput{}, err
	}

	query := strings.TrimSpace(in.Query)
	if query == "" {
		return PoemsOutput{}, inputErrorf("query must not be empty")
	}

	searchType := in.SearchType
	if searchType == "" {
		searchType = "all"
	}
	if !slices.Contains(searchTypes, searchType) {
		return PoemsOutput{}, inputErrorf("unsupported search_type %s; supported: %s",
			strconv.Quote(searchType), strings.Join(searchTypes, ", "))
	}

	page, pageSize, err := normalizePage(in.Page, in.PageSize)
	if err != nil {
		return PoemsOutput{}, err
	}

	poems, total, err := repo.SearchPoems(query, searchType, page, pageSize)
	if err != nil {
		return PoemsOutput{}, err
	}

	hasMore := int64(page)*int64(pageSize) < total
	return newPoemsOutput(poems, page, pageSize, hasMore)
}

func (t *tools) listPoems(ctx context.Context, in ListPoemsInput) (PoemsOutput, error) {
	repo, err := t.repoFor(ctx, in.Lang)
	if err != nil {
		return PoemsOutput{}, err
	}

	page, pageSize, err := normalizePage(in.Page, in.PageSize)
	if err != nil {
		return PoemsOutput{}, err
	}

	filters, err := resolveFilters(repo, in.Author, in.Dynasty, in.Types)
	if err != nil {
		return PoemsOutput{}, err
	}

	// 多取一条用来判断还有没有下一页
	poems, _, err := repo.ListPoemsWithFilter(pageSize+1, (page-1)*pageSize,
		filters.dynastyID, filters.authorID, filters.typeIDs)
	if err != nil {
		return PoemsOutput{}, err
	}

	hasMore := len(poems) > pageSize
	if hasMore {
		poems = poems[:pageSize]
	}
	return newPoemsOutput(poems, page, pageSize, hasMore)
}

func (t *tools) randomPoem(ctx context.Context, in RandomPoemInput) (PoemOutput, error) {
	repo, err := t.repoFor(ctx, in.Lang)
	if err != nil {
		return PoemOutput{}, err
	}

	var poem *database.Poem
	if in.Char != "" {
		if in.Author != "" || in.Dynasty != "" || len(in.Types) > 0 {
			return PoemOutput{}, inputErrorf("char cannot be combined with author, dynasty or types")
		}
		if utf8.RuneCountInString(in.Char) != 1 {
			return PoemOutput{}, inputErrorf("char must be a single character")
		}
		poem, err = repo.GetRandomPoemByChar(in.Char)
	} else {
		var filters poemFilters
		filters, err = resolveFilters(repo, in.Author, in.Dynasty, in.Types)
		if err != nil {
			return PoemOutput{}, err
		}
		poem, err = repo.GetRandomPoem(filters.dynastyID, filters.authorID, filters.typeIDs)
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return PoemOutput{}, inputErrorf("no poems match these conditions")
	}
	if err != nil {
		return PoemOutput{}, err
	}

	return newPoemOutput(poem)
}

func (t *tools) getAuthor(ctx context.Context, in GetAuthorInput) (AuthorOutput, error) {
	repo, err := t.repoFor(ctx, in.Lang)
	if err != nil {
		return AuthorOutput{}, err
	}

	if in.Name == "" {
		return AuthorOutput{}, inputErrorf("name must not be empty")
	}

	dynastyID, err := resolveDynasty(repo, in.Dynasty)
	if err != nil {
		return AuthorOutput{}, err
	}

	author, err := repo.GetAuthorByName(in.Name, dynastyID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return AuthorOutput{}, inputErrorf("author not found: %s", in.Name)
	}
	if err != nil {
		return AuthorOutput{}, err
	}

	poemCount, err := repo.CountPoemsByAuthor(author.ID)
	if err != nil {
		return AuthorOutput{}, err
	}

	out := AuthorOutput{Name: author.Name, PoemCount: poemCount}
	if author.Dynasty != nil {
		out.Dynasty = author.Dynasty.Name
	}
	if author.Description != nil {
		out.Description = *author.Description
	}
	return out, nil
}

func (t *tools) getStatistics(ctx context.Context, in GetStatisticsInput) (StatisticsOutput, error) {
	repo, err := t.repoFor(ctx, in.Lang)
	if err != nil {
		return StatisticsOutput{}, err
	}

	stats, err := repo.GetStatistics()
	if err != nil {
		return StatisticsOutput{}, err
	}

	out := StatisticsOutput{
		TotalPoems:   stats.TotalPoems,
		TotalAuthors: stats.TotalAuthors,
		Dynasties:    make([]DynastyStats, len(stats.PoemsByDynasty)),
		Types:        make([]TypeStats, len(stats.PoemsByType)),
	}
	for i, d := range stats.PoemsByDynasty {
		out.Dynasties[i] = DynastyStats{Name: d.Name, StartYear: d.StartYear, EndYear: d.EndYear, PoemCount: d.PoemCount}
	}
	for i, pt := range stats.PoemsByType {
		out.Types[i] = TypeStats{Name: pt.Name, Category: pt.Category, PoemCount: pt.PoemCount}
	}
	return out, nil
}

// poemFilters 保存解析后的过滤条件，nil 或空切片表示该字段不过滤。
type poemFilters struct {
	dynastyID *int64
	authorID  *int64
	typeIDs   []int64
}

// resolveFilters 把名称形式的过滤条件解析为 ID。
// 朝代要先解析：同名作者不止一位时，GetAuthorByName 会参考朝代来消除歧义。
func resolveFilters(repo *database.Repository, author, dynasty string, types []string) (poemFilters, error) {
	dynastyID, err := resolveDynasty(repo, dynasty)
	if err != nil {
		return poemFilters{}, err
	}
	filters := poemFilters{dynastyID: dynastyID}

	if author != "" {
		a, err := repo.GetAuthorByName(author, dynastyID)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return poemFilters{}, inputErrorf("author not found: %s", author)
		}
		if err != nil {
			return poemFilters{}, err
		}
		filters.authorID = &a.ID
	}

	if len(types) > 0 {
		ids, err := repo.GetPoetryTypeIDs(types)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return poemFilters{}, inputErrorf("poetry type not found in: %s; get_statistics lists the available types",
				strings.Join(types, ", "))
		}
		if err != nil {
			return poemFilters{}, err
		}
		filters.typeIDs = ids
	}

	return filters, nil
}

// resolveDynasty 把朝代名解析为 ID，name 为空时返回 nil。
func resolveDynasty(repo *database.Repository, name string) (*int64, error) {
	if name == "" {
		return nil, nil
	}
	dynasty, err := repo.GetDynastyByName(name)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, inputErrorf("dynasty not found: %s; get_statistics lists the available dynasties", name)
	}
	if err != nil {
		return nil, err
	}
	return &dynasty.ID, nil
}

// normalizePage 校验分页参数并填上默认值。翻页深度沿用 REST 接口的上限：
// OFFSET 的代价与深度成正比，page 又可以任意大，用乘法比较可能溢出，这里用除法。
func normalizePage(page, pageSize int) (int, int, error) {
	if page == 0 {
		page = 1
	}
	if pageSize == 0 {
		pageSize = defaultPageSize
	}
	if page < 1 {
		return 0, 0, inputErrorf("page must be at least 1")
	}
	if pageSize < 1 || pageSize > maxPageSize {
		return 0, 0, inputErrorf("page_size must be between 1 and %d", maxPageSize)
	}
	if page-1 > handler.MaxOffset/pageSize {
		return 0, 0, inputErrorf("page is too deep; narrow the query with filters instead")
	}
	return page, pageSize, nil
}

func newPoemsOutput(poems []database.Poem, page, pageSize int, hasMore bool) (PoemsOutput, error) {
	out := PoemsOutput{
		Poems:    make([]PoemOutput, len(poems)),
		Page:     page,
		PageSize: pageSize,
		HasMore:  hasMore,
	}
	for i := range poems {
		poem, err := newPoemOutput(&poems[i])
		if err != nil {
			return PoemsOutput{}, err
		}
		out.Poems[i] = poem
	}
	return out, nil
}

func newPoemOutput(poem *database.Poem) (PoemOutput, error) {
	out := PoemOutput{Title: poem.Title}
	if err := json.Unmarshal(poem.Content, &out.Content); err != nil {
		return PoemOutput{}, fmt.Errorf("failed to decode poem content: %w", err)
	}
	if poem.Author != nil {
		out.Author = poem.Author.Name
	}
	if poem.Dynasty != nil {
		out.Dynasty = poem.Dynasty.Name
	}
	if poem.Type != nil {
		out.Type = poem.Type.Name
	}
	return out, nil
}
