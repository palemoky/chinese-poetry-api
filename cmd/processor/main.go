package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/palemoky/chinese-poetry-api/internal/database"
	"github.com/palemoky/chinese-poetry-api/internal/loader"
	"github.com/palemoky/chinese-poetry-api/internal/processor"
)

var (
	inputDir   string
	outputDB   string
	workers    int
	configPath string
)

func main() {
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
		log.Fatal(err)
	}
}

func run(cmd *cobra.Command, args []string) error {
	// Determine config path
	if configPath == "" {
		configPath = filepath.Join(inputDir, "loader", "datas.json")
	}

	log.Printf("Loading poetry data from %s...", configPath)

	// Load all poetry data
	jsonLoader, err := loader.NewJSONLoader(configPath)
	if err != nil {
		return fmt.Errorf("failed to create loader: %w", err)
	}

	poems, err := jsonLoader.LoadAll()
	if err != nil {
		return fmt.Errorf("failed to load poems: %w", err)
	}

	log.Printf("Loaded %d poems from JSON files", len(poems))

	// Process unified database with both language variants
	log.Println("\n=== Processing Unified Database ===")
	if err := processUnifiedDatabase(outputDB, poems, workers); err != nil {
		return fmt.Errorf("failed to process database: %w", err)
	}

	log.Println("\n=== Processing Complete ===")
	log.Printf("Unified database: %s", outputDB)

	// Print statistics
	if err := printStatistics(outputDB); err != nil {
		log.Printf("Warning: failed to print statistics: %v", err)
	}

	return nil
}

func processUnifiedDatabase(dbPath string, poems []loader.PoemWithMeta, workers int) error {
	// Remove existing database
	if err := os.Remove(dbPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove existing database: %w", err)
	}

	// Open database
	db, err := database.Open(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer func() { _ = db.Close() }()

	// Run migrations - creates tables for both language variants
	log.Println("Creating database schema (simplified + traditional tables)...")
	if err := db.Migrate(); err != nil {
		return fmt.Errorf("failed to migrate database: %w", err)
	}

	// Process simplified Chinese version
	log.Println("\n--- Processing Simplified Chinese (zh-Hans) ---")
	repoSimp := database.NewRepositoryWithLang(db, database.LangHans)
	procSimp := processor.NewProcessor(repoSimp, workers, false)
	if err := procSimp.Process(poems); err != nil {
		return fmt.Errorf("failed to process simplified poems: %w", err)
	}

	// Process traditional Chinese version
	log.Println("\n--- Processing Traditional Chinese (zh-Hant) ---")
	repoTrad := database.NewRepositoryWithLang(db, database.LangHant)
	procTrad := processor.NewProcessor(repoTrad, workers, true)
	if err := procTrad.Process(poems); err != nil {
		return fmt.Errorf("failed to process traditional poems: %w", err)
	}

	// Optimize database
	log.Println("Optimizing database...")
	if err := db.Exec("VACUUM").Error; err != nil {
		log.Printf("Warning: failed to vacuum database: %v", err)
	}

	if err := db.Exec("ANALYZE").Error; err != nil {
		log.Printf("Warning: failed to analyze database: %v", err)
	}

	return nil
}

func printStatistics(dbPath string) error {
	db, err := database.Open(dbPath)
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
