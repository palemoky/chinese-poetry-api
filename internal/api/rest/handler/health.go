package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/palemoky/chinese-poetry-api/internal/database"
)

// HealthHandler 处理健康检查请求。
func HealthHandler(db *database.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 检查数据库连接是否可用
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

// StatsHandler 返回全库的整体统计数据。
//
// 本接口不接受任何查询参数，尤其是 lang：GetStatistics 统计的行数在简繁两套表中一致。
// 而 /health 则有意保持宽松，因为探针常会附加防缓存参数，
// 若一并拒绝，会让本来健康的服务在健康检查中被判为异常。
func StatsHandler(repo *database.Repository) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !checkQueryParams(c) {
			return
		}

		stats, err := repo.WithContext(c.Request.Context()).GetStatistics()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "failed to get statistics",
			})
			return
		}

		c.JSON(http.StatusOK, stats)
	}
}
