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
	"github.com/palemoky/chinese-poetry-api/internal/search"
)

// Defining the Graphql handler
func graphqlHandler(resolver *graph.Resolver) gin.HandlerFunc {
	h := handler.NewDefaultServer(generated.NewExecutableSchema(generated.Config{Resolvers: resolver}))

	return func(c *gin.Context) {
		h.ServeHTTP(c.Writer, c.Request)
	}
}

// Defining the Playground handler
func playgroundHandler() gin.HandlerFunc {
	h := playground.Handler("GraphQL", "/graphql")

	return func(c *gin.Context) {
		h.ServeHTTP(c.Writer, c.Request)
	}
}

func main() {
	// Initialize logger
	debug := os.Getenv("GIN_MODE") != "release"
	logger.Init(debug)
	defer logger.Sync()

	// Load configuration
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

	// Open database with configured connection pool
	db, err := database.Open(cfg.Database.Path, cfg.Database.MaxOpenConns, cfg.Database.MaxIdleConns)
	if err != nil {
		logger.Fatal("Failed to open database", zap.Error(err))
	}
	defer func() { _ = db.Close() }()

	// Create repository
	repo := database.NewRepository(db)

	// Create search engine
	searchEngine := search.NewEngine(db)

	// Create GraphQL resolver
	resolver := graph.NewResolver(db, repo, searchEngine)

	// Setup Gin router
	router := rest.SetupRouter(cfg, db, repo, searchEngine)

	// Add GraphQL endpoints
	router.POST("/graphql", graphqlHandler(resolver))
	if cfg.GraphQL.Playground {
		router.GET("/playground", playgroundHandler())
		logger.Info("GraphQL Playground enabled", zap.String("path", "/playground"))
	}

	// Create HTTP server
	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Server.Port),
		Handler: router,
	}

	// Start server in goroutine
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

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("Shutting down server...")

	// Graceful shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		logger.Warn("Server forced to shutdown", zap.Error(err))
	}

	logger.Info("Server exited")
}
