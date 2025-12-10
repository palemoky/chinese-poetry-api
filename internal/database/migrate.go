package database

import (
	"fmt"
	"strings"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/palemoky/chinese-poetry-api/internal/classifier"
)

// DB wraps the gorm.DB connection
type DB struct {
	*gorm.DB
}

// Open opens a connection to the SQLite database using GORM
func Open(path string) (*DB, error) {
	// Configure GORM
	config := &gorm.Config{
		Logger:  logger.Default.LogMode(logger.Silent), // Change to logger.Info for debugging
		NowFunc: time.Now,
		// Prepare statements for better performance
		PrepareStmt: true,
	}

	// SQLite connection string with optimizations for concurrent writes
	// _busy_timeout: wait up to 5 seconds if database is locked
	// _journal_mode=WAL: Write-Ahead Logging for better concurrency
	// _synchronous=NORMAL: balance between safety and performance
	// cache=shared: allow multiple connections to share cache
	// _cache_size=-64000: 64MB page cache (negative = KB, positive = pages)
	// _temp_store=MEMORY: use memory for temporary tables and indices
	dsn := path + "?_foreign_keys=on&_journal_mode=WAL&_busy_timeout=5000&_synchronous=NORMAL&cache=shared&_cache_size=-64000&_temp_store=MEMORY"

	// Open database with GORM SQLite driver
	db, err := gorm.Open(sqlite.Open(dsn), config)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Get underlying sql.DB for connection pool settings
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get underlying sql.DB: %w", err)
	}

	// Set connection pool settings for SQLite
	// SQLite works best with limited concurrent writers
	// MaxOpenConns=1 ensures serialized writes (no lock conflicts)
	// For read-heavy workloads, you can increase this
	sqlDB.SetMaxOpenConns(1) // Single writer to avoid "database is locked"
	sqlDB.SetMaxIdleConns(1)
	sqlDB.SetConnMaxLifetime(5 * time.Minute)

	// Test connection
	if err := sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &DB{db}, nil
}

// Migrate creates all tables, indexes, and initial data
// convertToTraditional: if true, convert initial data to traditional Chinese
func (db *DB) Migrate(convertToTraditional bool) error {
	// Use GORM AutoMigrate for standard tables
	if err := db.AutoMigrate(
		&Dynasty{},
		&Author{},
		&PoetryType{},
		&Poem{},
	); err != nil {
		return fmt.Errorf("failed to auto-migrate: %w", err)
	}

	// Create metadata table manually (not a model)
	if err := db.Exec(`CREATE TABLE IF NOT EXISTS metadata (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`).Error; err != nil {
		return fmt.Errorf("failed to create metadata table: %w", err)
	}

	// Prepare initial data SQL (convert if needed)
	dynastiesSQL := InitialDynastiesSQL
	poetryTypesSQL := InitialPoetryTypesSQL

	if convertToTraditional {
		// Convert dynasties to traditional Chinese
		var err error
		dynastiesSQL, err = convertSQLToTraditional(InitialDynastiesSQL)
		if err != nil {
			return fmt.Errorf("failed to convert dynasties SQL: %w", err)
		}

		// Convert poetry types to traditional Chinese
		poetryTypesSQL, err = convertSQLToTraditional(InitialPoetryTypesSQL)
		if err != nil {
			return fmt.Errorf("failed to convert poetry types SQL: %w", err)
		}
	}

	// Insert initial dynasties
	if err := db.Exec(dynastiesSQL).Error; err != nil {
		return fmt.Errorf("failed to insert dynasties: %w", err)
	}

	// Insert initial poetry types
	if err := db.Exec(poetryTypesSQL).Error; err != nil {
		return fmt.Errorf("failed to insert poetry types: %w", err)
	}

	// Update schema version
	if err := db.Exec(
		`INSERT OR REPLACE INTO metadata (key, value, updated_at) VALUES (?, ?, ?)`,
		"schema_version",
		fmt.Sprintf("%d", SchemaVersion),
		time.Now(),
	).Error; err != nil {
		return fmt.Errorf("failed to update schema version: %w", err)
	}

	return nil
}

// convertSQLToTraditional converts Chinese characters in SQL string to traditional
// Preserves SQL syntax and only converts Chinese text within quotes
func convertSQLToTraditional(sql string) (string, error) {
	// Split by single quotes to find string literals
	parts := strings.Split(sql, "'")

	for i := range parts {
		// Only convert odd-indexed parts (inside quotes)
		if i%2 == 1 {
			converted, err := classifier.ToTraditional(parts[i])
			if err != nil {
				return "", err
			}
			parts[i] = converted
		}
	}

	return strings.Join(parts, "'"), nil
}

// GetSchemaVersion returns the current schema version
func (db *DB) GetSchemaVersion() (int, error) {
	var version int
	err := db.Raw(`SELECT value FROM metadata WHERE key = ?`, "schema_version").Scan(&version).Error
	if err == gorm.ErrRecordNotFound {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return version, nil
}

// Close closes the database connection
func (db *DB) Close() error {
	sqlDB, err := db.DB.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}
