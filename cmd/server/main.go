package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/gin-gonic/gin"
	"github.com/palemoky/chinese-poetry-api/internal/api/rest"
	"github.com/palemoky/chinese-poetry-api/internal/config"
	"github.com/palemoky/chinese-poetry-api/internal/database"
	"github.com/palemoky/chinese-poetry-api/internal/graph"
	"github.com/palemoky/chinese-poetry-api/internal/graph/generated"
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
	// Load configuration
	cfg, err := config.Load("config.yaml")
	if err != nil {
		log.Printf("Warning: failed to load config file: %v, using defaults", err)
		cfg, _ = config.Load("")
	}

	log.Printf("Starting Chinese Poetry API server...")
	log.Printf("Database: %s", cfg.Database.Path)
	log.Printf("Port: %d", cfg.Server.Port)

	// Open database
	db, err := database.Open(cfg.Database.Path)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
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
		log.Println("GraphQL Playground enabled at /playground")
	}

	// Create HTTP server
	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Server.Port),
		Handler: router,
	}

	// Start server in goroutine
	go func() {
		log.Printf("Server listening on port %d", cfg.Server.Port)
		log.Printf("REST API: http://localhost:%d/api/v1", cfg.Server.Port)
		log.Printf("GraphQL: http://localhost:%d/graphql", cfg.Server.Port)
		if cfg.GraphQL.Playground {
			log.Printf("Playground: http://localhost:%d/playground", cfg.Server.Port)
		}

		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")

	// Graceful shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("Server forced to shutdown: %v", err)
	}

	log.Println("Server exited")
}
