package rest

import (
	"fmt"
	"os"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/palemoky/chinese-poetry-api/internal/api/middleware"
	"github.com/palemoky/chinese-poetry-api/internal/api/rest/handler"
	"github.com/palemoky/chinese-poetry-api/internal/config"
	"github.com/palemoky/chinese-poetry-api/internal/database"
	"github.com/palemoky/chinese-poetry-api/internal/logger"
)

// cacheMaxAge 是可缓存接口的 max-age。数据只在发布时更新，过期后凭 ETag 校验，
// 数据没换只需一次 304，因此不必设得很长：发布新数据后最多一小时各处缓存都会更新。
const cacheMaxAge = time.Hour

// dataVersionETag 用数据库文件的大小与修改时间标识数据版本。
// 发布新数据会替换整个文件，两者随之改变，旧的缓存校验时就会失效。
// 读不到文件信息时返回空串，此时只发 Cache-Control。
func dataVersionETag(path string) string {
	info, err := os.Stat(path)
	if err != nil {
		return ""
	}
	return fmt.Sprintf(`W/"%x-%x"`, info.Size(), info.ModTime().UnixNano())
}

// SetupRouter 初始化 Gin 路由并注册全部接口。
func SetupRouter(cfg *config.Config, db *database.DB, repo *database.Repository) (*gin.Engine, error) {
	// 设置 Gin 运行模式
	gin.SetMode(cfg.Server.Mode)

	router := gin.New()

	// Gin 默认信任所有代理，ClientIP 会直接采信请求方自填的 X-Forwarded-For，
	// 按 IP 限流因此形同虚设；这里只信任配置中列出的代理
	if err := router.SetTrustedProxies(cfg.Server.TrustedProxies); err != nil {
		return nil, fmt.Errorf("invalid trusted proxies: %w", err)
	}
	router.Use(middleware.RequestLogger(logger.Default(), "/api/v1/health"))
	router.Use(gin.Recovery())

	// 跨域中间件
	router.Use(middleware.CORS())

	// 限流中间件
	if cfg.RateLimit.Enabled {
		rateLimiter := middleware.NewRateLimiter(cfg.RateLimit.RequestsPerSecond, cfg.RateLimit.Burst)
		router.Use(rateLimiter.Middleware())
	}

	// v1 版本接口
	v1 := router.Group("/api/v1")
	{
		// 每次结果都可能不同的接口不缓存
		v1.GET("/health", middleware.NoStore(), handler.HealthHandler(db))

		poemHandler := handler.NewPoemHandler(repo)
		v1.GET("/poems/random", middleware.NoStore(), poemHandler.RandomPoem)

		// 其余接口的结果只随数据库变化
		cached := v1.Group("", middleware.Cache(cacheMaxAge, dataVersionETag(cfg.Database.Path)))

		// 统计数据
		cached.GET("/stats", handler.StatsHandler(repo))

		// 诗词相关
		cached.GET("/poems", poemHandler.ListPoems)
		cached.GET("/poems/search", poemHandler.SearchPoems)

		// 作者相关
		authorHandler := handler.NewAuthorHandler(repo)
		cached.GET("/authors", authorHandler.ListAuthors)
		cached.GET("/authors/:id", authorHandler.GetAuthor)

		// 朝代相关
		dynastyHandler := handler.NewDynastyHandler(repo)
		cached.GET("/dynasties", dynastyHandler.ListDynasties)
		cached.GET("/dynasties/:id", dynastyHandler.GetDynasty)

		// 体裁相关
		poetryTypeHandler := handler.NewPoetryTypeHandler(repo)
		cached.GET("/types", poetryTypeHandler.ListPoetryTypes)
		cached.GET("/types/:id", poetryTypeHandler.GetPoetryType)
	}

	return router, nil
}
