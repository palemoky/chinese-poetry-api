package handler

import (
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

// REST API 中可用的查询参数名。
const (
	queryLang      = "lang"
	queryPage      = "page"
	queryPageSize  = "page_size"
	queryAfter     = "after"
	queryQuery     = "q"
	queryChar      = "char"
	queryAuthorID  = "author_id"
	queryAuthor    = "author"
	queryTypeID    = "type_id"
	queryDynastyID = "dynasty_id"
	queryDynasty   = "dynasty"

	// queryType 在 /poems 与 /poems/random 上表示体裁过滤，
	// 在 /poems/search 上表示搜索模式——同一个名字，两种互不相干的含义。
	queryType = "type"
)

// checkQueryParams 在请求带有 allowed 之外的查询参数时返回 400 并返回 false。
//
// Gin 会静默忽略无法识别的查询参数，因此在加上这道校验之前，
// 写错的过滤参数（如误用 GraphQL 的 dynastyId 而非 dynasty_id）
// 会在完全未过滤的全量数据上返回一个正常的 200，客户端无从察觉过滤被忽略了。
// 为此每个接口都显式声明自己真正会读取的参数名。
func checkQueryParams(c *gin.Context, allowed ...string) bool {
	var unknown []string
	for key := range c.Request.URL.Query() {
		if !slices.Contains(allowed, key) {
			unknown = append(unknown, key)
		}
	}
	if len(unknown) == 0 {
		return true
	}

	// map 遍历顺序随机，排序后错误信息才稳定可复现
	slices.Sort(unknown)
	sortedAllowed := slices.Clone(allowed)
	slices.Sort(sortedAllowed)

	respondError(c, http.StatusBadRequest, "unknown query parameter(s): "+strings.Join(unknown, ", ")+
		"; supported: "+strings.Join(sortedAllowed, ", "))
	return false
}

// parseIntQuery 把 key 解析为落在 [minValue, maxValue] 区间内的整数，参数缺省时返回 def。
// 取值非整数或越界时返回 400 并返回 false，而非悄悄纠正：只返回被截断的内容
func parseIntQuery(c *gin.Context, key string, def, minValue, maxValue int) (int, bool) {
	raw := c.Query(key)
	if raw == "" {
		return def, true
	}

	value, err := strconv.Atoi(raw)
	if err != nil {
		respondError(c, http.StatusBadRequest, key+" must be an integer, got "+strconv.Quote(raw))
		return 0, false
	}
	if value < minValue || value > maxValue {
		respondError(c, http.StatusBadRequest, key+" must be between "+strconv.Itoa(minValue)+
			" and "+strconv.Itoa(maxValue)+", got "+strconv.Itoa(value))
		return 0, false
	}
	return value, true
}

// parseInt64Query 把 key 解析为 int64 类型的 ID，参数缺省时返回 nil。
// 取值非数字时返回 400 并返回 false；避免查询范围在无声无息中扩大到全量数据。
func parseInt64Query(c *gin.Context, key string) (*int64, bool) {
	raw := c.Query(key)
	if raw == "" {
		return nil, true
	}

	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		respondError(c, http.StatusBadRequest, key+" must be an integer, got "+strconv.Quote(raw))
		return nil, false
	}
	return &id, true
}
