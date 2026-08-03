package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/palemoky/chinese-poetry-api/internal/api/rest"
	"github.com/palemoky/chinese-poetry-api/internal/config"
	"github.com/palemoky/chinese-poetry-api/internal/database"
	"github.com/palemoky/chinese-poetry-api/internal/graph"
	"github.com/palemoky/chinese-poetry-api/internal/graph/generated"
	"github.com/palemoky/chinese-poetry-api/internal/logger"
)

// graphqlHandler 构造 GraphQL 请求的 Gin handler。
func graphqlHandler(resolver *graph.Resolver) gin.HandlerFunc {
	h := handler.NewDefaultServer(generated.NewExecutableSchema(generated.Config{Resolvers: resolver}))

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

	// 按配置的连接池参数打开数据库
	db, err := database.Open(cfg.Database.Path, cfg.Database.MaxOpenConns, cfg.Database.MaxIdleConns)
	if err != nil {
		logger.Fatal("Failed to open database", zap.Error(err))
	}
	defer func() { _ = db.Close() }()

	// 补齐数据库结构后再对外提供服务。
	//
	// 服务端只读数据，但仍要跑一次迁移：查询依赖的部分结构（如物化的
	// authors.poem_count 与 counters 计数器）是随版本新增的，老库里并不存在，
	// 直接起服务只会在第一个请求上报 "no such column"。
	// Migrate 是幂等的，只补缺失的表、列、索引与触发器，不会改动已导入的诗词。
	if err := db.Migrate(); err != nil {
		logger.Fatal("Failed to migrate database", zap.Error(err))
	}

	// 创建仓储
	repo := database.NewRepository(db)

	// 创建 GraphQL resolver
	resolver := graph.NewResolver(db, repo)

	// 初始化 Gin 路由
	router := rest.SetupRouter(cfg, db, repo)

	// 注册 GraphQL 相关路由
	router.POST("/graphql", graphqlHandler(resolver))
	if cfg.GraphQL.Playground {
		router.GET("/playground", playgroundHandler())
		logger.Info("GraphQL Playground enabled", zap.String("path", "/playground"))
	}

	// 构造 HTTP 服务
	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Server.Port),
		Handler: router,
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
