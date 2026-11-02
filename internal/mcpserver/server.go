// Package mcpserver 通过 Model Context Protocol 向大模型客户端提供诗词查询工具。
//
// 工具一律按名称和内容访问数据，返回结果也不含 ID：诗词与作者的 ID 在每次发布数据时
// 都可能变化，暴露给模型后很容易被记下来跨版本使用。
package mcpserver

import (
	"bytes"
	"errors"
	"io"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/palemoky/chinese-poetry-api/internal/database"
	"github.com/palemoky/chinese-poetry-api/internal/semantic"
)

const (
	serverName = "chinese-poetry"
	// serverVersion 是工具集的版本，与数据发布的版本无关。
	serverVersion = "1.0.0"

	// maxBodyBytes 是请求体的上限。一次工具调用不过几百字节，64 KB 绰绰有余。
	maxBodyBytes = 64 << 10
)

const instructions = `Chinese Poetry provides a read-only corpus of classical Chinese poems.

search_poems matches literal substrings, not meanings. To find poems about a theme, search for the
words poets actually used: for homesickness try 故乡 or 思归, not 思乡诗.

Dynasty, author and type names must be written in the script selected by lang. get_statistics lists
the available dynasties and poetry types.`

// semanticInstructions 在启用语义检索时追加到 instructions 后面。
const semanticInstructions = `

search_poems_by_meaning finds poems from a description in any language: a theme, a scene, a feeling
or a half-remembered line. It returns candidates, not answers: the best match is often not first,
so read them and pick the ones that fit. Use search_poems when you know the exact words.`

// NewHandler 构造 MCP 的 Streamable HTTP handler。
//
// 使用无状态模式：工具都是只读查询，不需要会话，服务重启或多实例部署时也不会丢状态。
// JSONResponse 让每个请求直接返回一个 JSON 响应，不经过 SSE。
//
// 关闭 DNS 重绑定保护：它拦截的是从回环地址进来、Host 却不是本机的请求，
// 而本机反代转发的公网请求正是这样，开着会让反代后的服务全部 403。
// 这里只提供公开数据，没有凭证或本地资源可被重绑定页面利用。
//
// searcher 为 nil 时不提供语义检索工具。
func NewHandler(repo *database.Repository, searcher *semantic.Searcher) http.Handler {
	server := newServer(repo, searcher)
	h := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		return server
	}, &mcp.StreamableHTTPOptions{
		Stateless:                    true,
		JSONResponse:                 true,
		DisableLocalhostProtection:   true,
		PropagateRequestCancellation: true,
	})
	return rejectBatches(h)
}

// newServer 创建 MCP 服务并注册全部工具。
func newServer(repo *database.Repository, searcher *semantic.Searcher) *mcp.Server {
	text := instructions
	if searcher != nil {
		text += semanticInstructions
	}
	server := mcp.NewServer(
		&mcp.Implementation{Name: serverName, Title: "Chinese Poetry", Version: serverVersion},
		&mcp.ServerOptions{Instructions: text},
	)
	registerTools(server, repo, searcher)
	return server
}

// rejectBatches 限制请求体大小，并拒绝 JSON-RPC 批量请求。
//
// 限流按 HTTP 请求计数，而 SDK 为兼容 2025-03-26 协议仍接受批量请求（没带协议版本头的
// 请求也按这个版本处理），一个 POST 就能塞进几百次工具调用。协议从 2025-06-18 起已经
// 移除批量请求，这里统一拒绝，不影响正常客户端。
func rejectBatches(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			next.ServeHTTP(w, r)
			return
		}

		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
		if err != nil {
			var tooLarge *http.MaxBytesError
			if errors.As(err, &tooLarge) {
				http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
				return
			}
			http.Error(w, "failed to read request body", http.StatusBadRequest)
			return
		}

		if trimmed := bytes.TrimLeft(body, " \t\r\n"); len(trimmed) > 0 && trimmed[0] == '[' {
			http.Error(w, "JSON-RPC batching is not supported", http.StatusBadRequest)
			return
		}

		r.Body = io.NopCloser(bytes.NewReader(body))
		next.ServeHTTP(w, r)
	})
}
