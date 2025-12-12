// Package testutil provides shared utilities for testing.
package testutil

import (
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/palemoky/chinese-poetry-api/internal/database"
)

// SetupTestDB creates an in-memory SQLite database with migrations applied.
// Returns the DB wrapper and Repository. Automatically cleans up on test completion.
func SetupTestDB(t *testing.T) (*database.DB, *database.Repository) {
	t.Helper()

	gormDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	require.NoError(t, err, "Failed to open in-memory database")

	db := database.NewDBFromGorm(gormDB)
	require.NoError(t, db.Migrate(), "Failed to run migrations")

	repo := database.NewRepository(db)

	t.Cleanup(func() {
		_ = db.Close()
	})

	return db, repo
}

// SetupTestDBWithLang creates an in-memory database with a specific language variant.
func SetupTestDBWithLang(t *testing.T, lang database.Lang) (*database.DB, *database.Repository) {
	t.Helper()

	gormDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	require.NoError(t, err, "Failed to open in-memory database")

	db := database.NewDBFromGorm(gormDB)
	require.NoError(t, db.Migrate(), "Failed to run migrations")

	repo := database.NewRepositoryWithLang(db, lang)

	t.Cleanup(func() {
		_ = db.Close()
	})

	return db, repo
}

// SetupTestGin creates a test Gin engine with test mode enabled.
func SetupTestGin() *gin.Engine {
	gin.SetMode(gin.TestMode)
	return gin.New()
}

// GormDB returns the underlying GORM database from a database.DB wrapper.
// This is useful for direct database manipulation in tests.
func GormDB(db *database.DB) *gorm.DB {
	return db.DB
}
