package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/palemoky/chinese-poetry-api/internal/database"
)

// HealthHandler handles health check requests
func HealthHandler(db *database.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Check database connection
		sqlDB, err := db.DB.DB()
		if err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status": "unhealthy",
				"error":  "failed to get database connection",
			})
			return
		}

		if err := sqlDB.Ping(); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status": "unhealthy",
				"error":  "database connection failed",
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status": "healthy",
		})
	}
}

// StatsHandler returns overall statistics
func StatsHandler(repo *database.Repository) gin.HandlerFunc {
	return func(c *gin.Context) {
		stats, err := repo.GetStatistics()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "failed to get statistics",
			})
			return
		}

		c.JSON(http.StatusOK, stats)
	}
}
