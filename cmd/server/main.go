package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/palemoky/chinese-poetry-api/internal/api/middleware"
	"github.com/palemoky/chinese-poetry-api/internal/api/rest"
	"github.com/palemoky/chinese-poetry-api/internal/config"
	"github.com/palemoky/chinese-poetry-api/internal/database"
	"github.com/palemoky/chinese-poetry-api/internal/graph"
	"github.com/palemoky/chinese-poetry-api/internal/logger"
)

// HTTP 服务的超时设置。不设超时时，慢速发送请求头或请求体的连接会一直占着，
// 少量客户端就能耗尽服务端的连接与协程。
const (
	readHeaderTimeout = 5 * time.Second
	readTimeout       = 15 * time.Second
	writeTimeout      = 30 * time.Second
	idleTimeout       = 120 * time.Second
)

// maxGraphQLBodyBytes 是 GraphQL 请求体的上限。正常的查询连同变量不过几 KB，
// 64 KB 绰绰有余，同时让超大的请求体在解析前就被拒绝。
const maxGraphQLBodyBytes = 64 << 10

// graphqlHandler 构造 GraphQL 请求的 Gin handler。
func graphqlHandler(resolver *graph.Resolver, cfg config.GraphQLConfig) gin.HandlerFunc {
	h := graph.NewServer(resolver, graph.ServerOptions{
		ComplexityLimit: cfg.ComplexityLimit,
		Introspection:   cfg.Introspection,
	})

	return func(c *gin.Context) {
		h.ServeHTTP(c.Writer, c.Request)
	}
}

// playgroundHandler 构造 GraphQL Playground 页面的 Gin handler。
func playgroundHandler() gin.HandlerFunc {
	h := playground.Handler("GraphQL", "/graphql")

	return func(c *gin.Context) {
		h.ServeHTTP(c.Writer, c.Request)
	}
}

func main() {
	// 初始化日志
	debug := os.Getenv("GIN_MODE") != "release"
	logger.Init(debug)
	defer logger.Sync()

	// 加载配置，失败则退回默认配置
	cfg, err := config.Load("config.yaml")
	if err != nil {
		logger.Warn("Failed to load config file, using defaults", zap.Error(err))
		cfg, _ = config.Load("")
	}

	logger.Info("Starting Chinese Poetry API server",
		zap.String("database", cfg.Database.Path),
		zap.Int("port", cfg.Server.Port),
		zap.Int("max_open_conns", cfg.Database.MaxOpenConns),
		zap.Int("max_idle_conns", cfg.Database.MaxIdleConns),
	)

	// 补齐数据库结构后以只读方式打开，详见 OpenForServing
	db, err := database.OpenForServing(cfg.Database.Path, cfg.Database.MaxOpenConns, cfg.Database.MaxIdleConns)
	if err != nil {
		logger.Fatal("Failed to open database", zap.Error(err))
	}
	defer func() { _ = db.Close() }()

	// 创建仓储
	repo := database.NewRepository(db)

	// 创建 GraphQL resolver
	resolver := graph.NewResolver(db, repo)

	// 初始化 Gin 路由
	router, err := rest.SetupRouter(cfg, db, repo)
	if err != nil {
		logger.Fatal("Failed to set up router", zap.Error(err))
	}

	// 注册 GraphQL 相关路由
	router.POST("/graphql", middleware.BodyLimit(maxGraphQLBodyBytes), graphqlHandler(resolver, cfg.GraphQL))
	if cfg.GraphQL.Playground {
		router.GET("/playground", playgroundHandler())
		logger.Info("GraphQL Playground enabled", zap.String("path", "/playground"))
	}

	// 构造 HTTP 服务
	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Server.Port),
		Handler:           router,
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
	}

	// 在独立协程中启动服务
	go func() {
		logger.Info("Server started",
			zap.Int("port", cfg.Server.Port),
			zap.String("rest_api", fmt.Sprintf("http://localhost:%d/api/v1", cfg.Server.Port)),
			zap.String("graphql", fmt.Sprintf("http://localhost:%d/graphql", cfg.Server.Port)),
		)

		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("Failed to start server", zap.Error(err))
		}
	}()

	// 等待中断信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("Shutting down server...")

	// 带超时的优雅退出
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		logger.Warn("Server forced to shutdown", zap.Error(err))
	}

	logger.Info("Server exited")
}
