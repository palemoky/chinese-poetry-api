package rest

import (
	"fmt"

	"github.com/gin-gonic/gin"

	"github.com/palemoky/chinese-poetry-api/internal/api/middleware"
	"github.com/palemoky/chinese-poetry-api/internal/api/rest/handler"
	"github.com/palemoky/chinese-poetry-api/internal/config"
	"github.com/palemoky/chinese-poetry-api/internal/database"
)

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
	router.Use(gin.Logger())
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
		// 健康检查
		v1.GET("/health", handler.HealthHandler(db))

		// 统计数据
		v1.GET("/stats", handler.StatsHandler(repo))

		// 诗词相关
		poemHandler := handler.NewPoemHandler(repo)
		v1.GET("/poems", poemHandler.ListPoems)
		v1.GET("/poems/random", poemHandler.RandomPoem)
		v1.GET("/poems/search", poemHandler.SearchPoems)

		// 作者相关
		authorHandler := handler.NewAuthorHandler(repo)
		v1.GET("/authors", authorHandler.ListAuthors)
		v1.GET("/authors/:id", authorHandler.GetAuthor)

		// 朝代相关
		dynastyHandler := handler.NewDynastyHandler(repo)
		v1.GET("/dynasties", dynastyHandler.ListDynasties)
		v1.GET("/dynasties/:id", dynastyHandler.GetDynasty)

		// 体裁相关
		poetryTypeHandler := handler.NewPoetryTypeHandler(repo)
		v1.GET("/types", poetryTypeHandler.ListPoetryTypes)
		v1.GET("/types/:id", poetryTypeHandler.GetPoetryType)
	}

	return router, nil
}
