package rest

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"github.com/getkin/kin-openapi/routers/gorillamux"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"

	"github.com/palemoky/chinese-poetry-api/internal/config"
	"github.com/palemoky/chinese-poetry-api/internal/database"
	"github.com/palemoky/chinese-poetry-api/internal/semantic"
	"github.com/palemoky/chinese-poetry-api/internal/testutil"
	"github.com/palemoky/chinese-poetry-api/internal/vectors"
)

// apiPrefix 是文档 servers 中声明的路径前缀，文档里的 paths 都相对于它。
const apiPrefix = "/api/v1"

func loadOpenAPISpec(t *testing.T) *openapi3.T {
	t.Helper()

	doc, err := openapi3.NewLoader().LoadFromData(openAPISpec)
	require.NoError(t, err)
	return doc
}

func TestOpenAPISpecIsValid(t *testing.T) {
	doc := loadOpenAPISpec(t)
	require.NoError(t, doc.Validate(context.Background()))
	assert.True(t, doc.IsOpenAPI32OrLater(), "spec should declare OpenAPI 3.2, got %s", doc.OpenAPI)
	require.Len(t, doc.Servers, 1)
	assert.Equal(t, apiPrefix, doc.Servers[0].URL)
}

func TestServeOpenAPISpec(t *testing.T) {
	cfg := testConfig(t)
	router, err := SetupRouter(cfg, nil, nil, nil)
	require.NoError(t, err)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, apiPrefix+"/openapi.yaml", nil))

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/yaml; charset=utf-8", w.Header().Get("Content-Type"))
	assert.Equal(t, "no-cache", w.Header().Get("Cache-Control"))
	assert.Empty(t, w.Header().Get("ETag"), "spec must not carry the data-version ETag")
	assert.Equal(t, openAPISpec, w.Body.Bytes())
}

// 路由与文档必须一一对应：新增接口忘了写文档，或删了接口文档还留着，都会在这里失败。
func TestOpenAPICoversAllRoutes(t *testing.T) {
	doc := loadOpenAPISpec(t)
	router, err := SetupRouter(testConfig(t), nil, nil, nil)
	require.NoError(t, err)

	ginParam := regexp.MustCompile(`:(\w+)`)
	var routes []string
	for _, r := range router.Routes() {
		if r.Method != http.MethodGet || !strings.HasPrefix(r.Path, apiPrefix) {
			continue
		}
		routes = append(routes, ginParam.ReplaceAllString(strings.TrimPrefix(r.Path, apiPrefix), "{$1}"))
	}

	documented := make([]string, 0, doc.Paths.Len())
	for path, item := range doc.Paths.Map() {
		assert.Len(t, item.Operations(), 1, "%s should only document GET", path)
		assert.NotNil(t, item.Get, "%s should document GET", path)
		documented = append(documented, path)
	}

	slices.Sort(routes)
	slices.Sort(documented)
	assert.Equal(t, routes, documented)
}

// supportedParams 匹配 checkQueryParams 错误信息里列出的参数名。
var supportedParams = regexp.MustCompile(`; supported: (.*)$`)

// 文档里每个接口声明的查询参数，必须正好是 handler 接受的那些。
// handler 遇到未知参数时会在 400 里列出全部支持的参数，这里直接拿来对照。
func TestOpenAPIQueryParamsMatchHandlers(t *testing.T) {
	doc := loadOpenAPISpec(t)
	_, repo := testutil.SetupTestDB(t)
	router, err := SetupRouter(testConfig(t), nil, repo, nil)
	require.NoError(t, err)

	// 这两个接口不校验查询参数：/health 要容忍探针附加的防缓存参数，文档是静态文件
	unchecked := []string{"/health", "/openapi.yaml"}

	for path, item := range doc.Paths.Map() {
		var documented []string
		for _, p := range item.Get.Parameters {
			if p.Value.In == openapi3.ParameterInQuery {
				documented = append(documented, p.Value.Name)
			}
		}
		slices.Sort(documented)

		if slices.Contains(unchecked, path) {
			assert.Empty(t, documented, "%s accepts any query parameter; document none", path)
			continue
		}

		t.Run(path, func(t *testing.T) {
			target := apiPrefix + strings.ReplaceAll(path, "{id}", "1") + "?zz_probe=1"
			w := httptest.NewRecorder()
			router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, target, nil))
			require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())

			var body struct{ Message string }
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
			m := supportedParams.FindStringSubmatch(body.Message)
			require.NotNil(t, m, "unexpected error: %s", body.Message)

			var accepted []string
			if m[1] != "" {
				accepted = strings.Split(m[1], ", ")
			}
			assert.Equal(t, accepted, documented)
		})
	}
}

// 真实响应（含各类错误）都要符合文档里的结构。文档的 schema 都设了
// additionalProperties: false，响应多出未写进文档的字段也会失败。
func TestOpenAPIResponsesMatchSpec(t *testing.T) {
	doc := loadOpenAPISpec(t)
	// gorillamux 需要完整的服务器地址；httptest 的请求默认发往 example.com
	doc.Servers = openapi3.Servers{{URL: "http://example.com" + apiPrefix}}
	specRouter, err := gorillamux.NewRouter(doc)
	require.NoError(t, err)

	db, repo := testutil.SetupTestDB(t)
	seedOpenAPITestData(t, db, repo)

	idx := &vectors.Index{Meta: vectors.Meta{Dims: 2}, IDs: []int64{1001, 1002}, Data: []int8{0, 127, 127, 0}}
	searcher := semantic.New(idx, nil, stubEmbedder{vector: []float32{1, 0}})

	router, err := SetupRouter(testConfig(t), db, repo, searcher)
	require.NoError(t, err)
	noSemantic, err := SetupRouter(testConfig(t), db, repo, nil)
	require.NoError(t, err)

	validate := func(t *testing.T, r *gin.Engine, target string, wantStatus int) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, target, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		require.Equal(t, wantStatus, w.Code, "%s: %s", target, w.Body.String())

		validateResponse(t, specRouter, req, w)
		return w
	}

	cases := []struct {
		target string
		status int
	}{
		{"/health", http.StatusOK},
		{"/stats", http.StatusOK},
		{"/stats?lang=zh-Hant", http.StatusBadRequest},

		{"/poems", http.StatusOK},
		{"/poems?dynasty=唐&author=李白&type=七言绝句&type=唐诗", http.StatusOK},
		{"/poems?lang=zh-Hant&type=唐詩", http.StatusOK},
		{"/poems?type_id=12&type_id=11&dynasty_id=1", http.StatusOK},
		{"/poems?page=500&page_size=100", http.StatusBadRequest},
		{"/poems?page=1&after=x", http.StatusBadRequest},
		{"/poems?after=not-a-cursor", http.StatusBadRequest},
		{"/poems?dynastyId=1", http.StatusBadRequest},
		{"/poems?author=无名氏", http.StatusNotFound},
		{"/poems?type=不存在", http.StatusNotFound},

		{"/poems/search?q=明月", http.StatusOK},
		{"/poems/search?q=李白&type=author&dynasty=唐&page_size=1", http.StatusOK},
		{"/poems/search", http.StatusBadRequest},
		{"/poems/search?q=月&type=genre", http.StatusBadRequest},
		{"/poems/search?q=月&dynasty=不存在", http.StatusNotFound},

		{"/poems/semantic?q=钓鱼&limit=1", http.StatusOK},
		{"/poems/semantic?q=钓鱼&limit=51", http.StatusBadRequest},
		{"/poems/semantic?q=" + strings.Repeat("月", semantic.MaxQueryRunes+1), http.StatusBadRequest},

		{"/poems/random", http.StatusOK},
		{"/poems/random?char=月", http.StatusOK},
		{"/poems/random?author=李白&type=七言绝句", http.StatusOK},
		{"/poems/random?char=月&dynasty=唐", http.StatusBadRequest},
		{"/poems/random?char=春", http.StatusNotFound},
		{"/poems/random?dynasty_id=999", http.StatusNotFound},

		{"/authors?page_size=1", http.StatusOK},
		{"/authors/1", http.StatusOK},
		{"/authors/1?lang=zh-Hant", http.StatusOK},
		{"/authors/abc", http.StatusBadRequest},
		{"/authors/999", http.StatusNotFound},

		{"/dynasties", http.StatusOK},
		{"/dynasties/1", http.StatusOK},
		{"/dynasties/999", http.StatusNotFound},

		{"/types", http.StatusOK},
		{"/types/12", http.StatusOK},
		{"/types/999", http.StatusNotFound},
		{"/types/12?lang=xx", http.StatusBadRequest},

		{"/openapi.yaml", http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.target, func(t *testing.T) {
			validate(t, router, apiPrefix+escapeQuery(tc.target), tc.status)
		})
	}

	t.Run("cursor pagination", func(t *testing.T) {
		w := validate(t, router, apiPrefix+"/poems?page_size=1", http.StatusOK)
		var first struct {
			Pagination struct {
				NextCursor string `json:"next_cursor"`
			}
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &first))
		require.NotEmpty(t, first.Pagination.NextCursor)
		validate(t, router, apiPrefix+"/poems?page_size=1&after="+url.QueryEscape(first.Pagination.NextCursor), http.StatusOK)
	})

	t.Run("semantic search disabled", func(t *testing.T) {
		validate(t, noSemantic, apiPrefix+"/poems/semantic?q=x", http.StatusServiceUnavailable)
	})

	t.Run("rate limited", func(t *testing.T) {
		cfg := testConfig(t)
		cfg.RateLimit.Enabled = true
		cfg.RateLimit.RequestsPerSecond = 0.001
		cfg.RateLimit.Burst = 1
		limited, err := SetupRouter(cfg, db, repo, nil)
		require.NoError(t, err)

		validate(t, limited, apiPrefix+"/dynasties", http.StatusOK)
		validate(t, limited, apiPrefix+"/dynasties", http.StatusTooManyRequests)
	})
}

// validateResponse 按文档校验一次响应的状态码、Content-Type 与响应体。
func validateResponse(t *testing.T, specRouter routers.Router, req *http.Request, w *httptest.ResponseRecorder) {
	t.Helper()

	route, pathParams, err := specRouter.FindRoute(req)
	require.NoError(t, err, "request not covered by the spec")

	body := w.Body.Bytes()
	if w.Header().Get("Content-Encoding") == "gzip" {
		t.Fatalf("unexpected gzip response for %s", req.URL)
	}
	err = openapi3filter.ValidateResponse(context.Background(), &openapi3filter.ResponseValidationInput{
		RequestValidationInput: &openapi3filter.RequestValidationInput{
			Request:    req,
			PathParams: pathParams,
			Route:      route,
		},
		Status: w.Code,
		Header: w.Header(),
		Body:   io.NopCloser(bytes.NewReader(body)),
		Options: &openapi3filter.Options{
			// 文档里没写的状态码也算不符
			IncludeResponseStatus: true,
			MultiError:            true,
		},
	})
	assert.NoError(t, err, "%s\nbody: %s", req.URL, body)
}

// escapeQuery 对用例里直接写成中文的查询串做 URL 编码，保留重复的参数。
func escapeQuery(target string) string {
	path, rawQuery, found := strings.Cut(target, "?")
	if !found {
		return target
	}
	var parts []string
	for pair := range strings.SplitSeq(rawQuery, "&") {
		k, v, _ := strings.Cut(pair, "=")
		parts = append(parts, url.QueryEscape(k)+"="+url.QueryEscape(v))
	}
	return path + "?" + strings.Join(parts, "&")
}

// seedOpenAPITestData 写入简繁两套数据，让各接口返回的字段尽量齐全（含可选字段）。
func seedOpenAPITestData(t *testing.T, db *database.DB, repo *database.Repository) {
	t.Helper()

	typeID := int64(12) // 七言绝句，由 Migrate 预置
	for _, lang := range []database.Lang{database.LangHans, database.LangHant} {
		r := repo.WithLang(lang)
		dynastyID, err := r.GetOrCreateDynasty("唐")
		require.NoError(t, err)
		authorID, err := r.GetOrCreateAuthor("李白", dynastyID)
		require.NoError(t, err)

		require.NoError(t, db.Table(database.DynastiesTable(lang)).Where("id = ?", dynastyID).
			Updates(map[string]any{"name_en": "Tang", "start_year": 618, "end_year": 907}).Error)
		require.NoError(t, db.Table(database.AuthorsTable(lang)).Where("id = ?", authorID).
			Update("description", "字太白，号青莲居士").Error)

		for _, p := range []struct {
			id      int64
			title   string
			content string
		}{
			{1001, "静夜思", `["床前明月光，疑是地上霜。","举头望明月，低头思故乡。"]`},
			{1002, "江雪", `["千山鸟飞绝，万径人踪灭。","孤舟蓑笠翁，独钓寒江雪。"]`},
		} {
			require.NoError(t, r.InsertPoem(&database.Poem{
				ID:        p.id,
				Title:     p.title,
				Content:   datatypes.JSON(p.content),
				AuthorID:  &authorID,
				DynastyID: &dynastyID,
				TypeID:    &typeID,
			}))
		}

		// 作者、朝代、体裁都缺失的诗，覆盖 Poem 中这三个字段为 null 的情形
		require.NoError(t, r.InsertPoem(&database.Poem{
			ID:      1003,
			Title:   "无题",
			Content: datatypes.JSON(`["白日依山尽，黄河入海流。"]`),
		}))
	}
}

// stubEmbedder 为语义检索返回固定的向量。
type stubEmbedder struct{ vector []float32 }

func (s stubEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i := range out {
		out[i] = s.vector
	}
	return out, nil
}

func testConfig(t *testing.T) *config.Config {
	t.Helper()

	cfg, err := config.Load("")
	require.NoError(t, err)
	cfg.Server.Mode = gin.TestMode
	cfg.RateLimit.Enabled = false
	return cfg
}
