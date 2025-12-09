package database

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// DB wraps the sql.DB connection
type DB struct {
	*sql.DB
}

// Open opens a connection to the SQLite database
func Open(path string) (*DB, error) {
	db, err := sql.Open("sqlite3", path+"?_foreign_keys=on&_journal_mode=WAL")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Test connection
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	// Set connection pool settings
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	return &DB{db}, nil
}

// Migrate creates all tables, indexes, and initial data
func (db *DB) Migrate() error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Create tables
	for _, sql := range CreateTablesSQL {
		if _, err := tx.Exec(sql); err != nil {
			return fmt.Errorf("failed to create table: %w", err)
		}
	}

	// Create indexes
	for _, sql := range CreateIndexesSQL {
		if _, err := tx.Exec(sql); err != nil {
			return fmt.Errorf("failed to create index: %w", err)
		}
	}

	// Insert initial data
	if _, err := tx.Exec(InitialDynastiesSQL); err != nil {
		return fmt.Errorf("failed to insert dynasties: %w", err)
	}

	if _, err := tx.Exec(InitialPoetryTypesSQL); err != nil {
		return fmt.Errorf("failed to insert poetry types: %w", err)
	}

	// Create triggers
	for _, sql := range TriggersSQL {
		if _, err := tx.Exec(sql); err != nil {
			return fmt.Errorf("failed to create trigger: %w", err)
		}
	}

	// Update schema version
	if _, err := tx.Exec(
		`INSERT OR REPLACE INTO metadata (key, value, updated_at) VALUES (?, ?, ?)`,
		"schema_version",
		fmt.Sprintf("%d", SchemaVersion),
		time.Now(),
	); err != nil {
		return fmt.Errorf("failed to update schema version: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// GetSchemaVersion returns the current schema version
func (db *DB) GetSchemaVersion() (int, error) {
	var version int
	err := db.QueryRow(`SELECT value FROM metadata WHERE key = ?`, "schema_version").Scan(&version)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return version, nil
}

// Close closes the database connection
func (db *DB) Close() error {
	return db.DB.Close()
}
