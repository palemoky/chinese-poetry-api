package rest

import (
	"github.com/gin-gonic/gin"

	"github.com/palemoky/chinese-poetry-api/internal/api/middleware"
	"github.com/palemoky/chinese-poetry-api/internal/api/rest/handler"
	"github.com/palemoky/chinese-poetry-api/internal/config"
	"github.com/palemoky/chinese-poetry-api/internal/database"
)

// SetupRouter sets up the Gin router with all routes
func SetupRouter(cfg *config.Config, db *database.DB, repo *database.Repository) *gin.Engine {
	// Set Gin mode
	gin.SetMode(cfg.Server.Mode)

	router := gin.New()
	router.Use(gin.Logger())
	router.Use(gin.Recovery())

	// CORS middleware
	router.Use(middleware.CORS())

	// Rate limiting middleware
	if cfg.RateLimit.Enabled {
		rateLimiter := middleware.NewRateLimiter(cfg.RateLimit.RequestsPerSecond, cfg.RateLimit.Burst)
		router.Use(rateLimiter.Middleware())
	}

	// API v1 routes
	v1 := router.Group("/api/v1")
	{
		// Health check
		v1.GET("/health", handler.HealthHandler(db))

		// Statistics
		v1.GET("/stats", handler.StatsHandler(repo))

		// Poem routes
		poemHandler := handler.NewPoemHandler(repo)
		v1.GET("/poems", poemHandler.ListPoems)
		v1.GET("/poems/random", poemHandler.RandomPoem)
		v1.GET("/poems/search", poemHandler.SearchPoems)

		// Author routes
		authorHandler := handler.NewAuthorHandler(repo)
		v1.GET("/authors", authorHandler.ListAuthors)
		v1.GET("/authors/:id", authorHandler.GetAuthor)

		// Dynasty routes
		dynastyHandler := handler.NewDynastyHandler(repo)
		v1.GET("/dynasties", dynastyHandler.ListDynasties)
		v1.GET("/dynasties/:id", dynastyHandler.GetDynasty)

		// Poetry type routes
		poetryTypeHandler := handler.NewPoetryTypeHandler(repo)
		v1.GET("/types", poetryTypeHandler.ListPoetryTypes)
		v1.GET("/types/:id", poetryTypeHandler.GetPoetryType)
	}

	return router
}
