package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/palemoky/chinese-poetry-api/internal/database"
	"github.com/palemoky/chinese-poetry-api/internal/loader"
	"github.com/palemoky/chinese-poetry-api/internal/logger"
	"github.com/palemoky/chinese-poetry-api/internal/processor"
)

var (
	inputDir   string
	outputDB   string
	workers    int
	configPath string
)

func main() {
	// Initialize logger (always debug mode for processor)
	logger.Init(true)
	defer logger.Sync()

	rootCmd := &cobra.Command{
		Use:   "processor",
		Short: "Chinese Poetry Data Processor",
		Long:  "Process Chinese poetry JSON data and generate a unified SQLite database with both simplified and traditional Chinese versions",
		RunE:  run,
	}

	rootCmd.Flags().StringVarP(&inputDir, "input", "i", "poetry-data", "Input directory containing poetry JSON files")
	rootCmd.Flags().StringVarP(&outputDB, "output", "o", "poetry.db", "Output unified SQLite database")
	rootCmd.Flags().IntVarP(&workers, "workers", "w", 0, "Number of concurrent workers (0 = number of CPUs)")
	rootCmd.Flags().StringVarP(&configPath, "config", "c", "", "Path to datas.json config file (default: <input>/loader/datas.json)")

	if err := rootCmd.Execute(); err != nil {
		logger.Fatal("Command execution failed", zap.Error(err))
	}
}

func run(cmd *cobra.Command, args []string) error {
	// Determine config path
	if configPath == "" {
		configPath = filepath.Join(inputDir, "loader", "datas.json")
	}

	logger.Info("Loading poetry data", zap.String("config", configPath))

	// Load all poetry data
	jsonLoader, err := loader.NewJSONLoader(configPath)
	if err != nil {
		return fmt.Errorf("failed to create loader: %w", err)
	}

	poems, err := jsonLoader.LoadAll()
	if err != nil {
		return fmt.Errorf("failed to load poems: %w", err)
	}

	logger.Info("Loaded poems from JSON files", zap.Int("count", len(poems)))

	// Process unified database with both language variants
	logger.Info("Processing unified database")
	if err := processUnifiedDatabase(outputDB, poems, workers); err != nil {
		return fmt.Errorf("failed to process database: %w", err)
	}

	logger.Info("Processing complete", zap.String("database", outputDB))

	// Print statistics
	if err := printStatistics(outputDB); err != nil {
		logger.Warn("Failed to print statistics", zap.Error(err))
	}

	return nil
}

func processUnifiedDatabase(dbPath string, poems []loader.PoemWithMeta, workers int) error {
	// Remove existing database
	if err := os.Remove(dbPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove existing database: %w", err)
	}

	// Open database with single connection (safe for data processing)
	db, err := database.Open(dbPath, 1, 1)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer func() { _ = db.Close() }()

	// Run migrations - creates tables for both language variants
	logger.Info("Creating database schema (simplified + traditional tables)")
	if err := db.Migrate(); err != nil {
		return fmt.Errorf("failed to migrate database: %w", err)
	}

	// Process simplified Chinese version
	logger.Info("Processing language variant", zap.String("lang", "zh-Hans"))
	repoSimp := database.NewRepositoryWithLang(db, database.LangHans)
	procSimp := processor.NewProcessor(repoSimp, workers, false)
	if err := procSimp.Process(poems); err != nil {
		return fmt.Errorf("failed to process simplified poems: %w", err)
	}

	// Process traditional Chinese version
	logger.Info("Processing language variant", zap.String("lang", "zh-Hant"))
	repoTrad := database.NewRepositoryWithLang(db, database.LangHant)
	procTrad := processor.NewProcessor(repoTrad, workers, true)
	if err := procTrad.Process(poems); err != nil {
		return fmt.Errorf("failed to process traditional poems: %w", err)
	}

	// Optimize database
	logger.Info("Optimizing database")
	if err := db.Exec("VACUUM").Error; err != nil {
		logger.Warn("Failed to vacuum database", zap.Error(err))
	}

	if err := db.Exec("ANALYZE").Error; err != nil {
		logger.Warn("Failed to analyze database", zap.Error(err))
	}

	return nil
}

func printStatistics(dbPath string) error {
	// Use single connection for statistics (read-only)
	db, err := database.Open(dbPath, 1, 1)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()

	fmt.Println("\n=== Database Statistics ===")
	fmt.Println("+-----------------------+----------+----------+----------+-------------+")
	fmt.Println("| Language              | Poems    | Authors  | Dynasties| Poetry Types|")
	fmt.Println("+-----------------------+----------+----------+----------+-------------+")

	for _, lang := range []database.Lang{database.LangHans, database.LangHant} {
		var poemCount, authorCount, dynastyCount, typeCount int64

		db.Table(database.PoemsTable(lang)).Count(&poemCount)
		db.Table(database.AuthorsTable(lang)).Count(&authorCount)
		db.Table(database.DynastiesTable(lang)).Count(&dynastyCount)
		db.Table(database.PoetryTypesTable(lang)).Count(&typeCount)

		langName := "Simplified (zh-Hans)"
		if lang == database.LangHant {
			langName = "Traditional (zh-Hant)"
		}

		fmt.Printf("| %-21s | %8d | %8d | %8d | %11d |\n",
			langName, poemCount, authorCount, dynastyCount, typeCount)
	}

	fmt.Println("+-----------------------+----------+----------+----------+-------------+")

	return nil
}
