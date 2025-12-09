package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/palemoky/chinese-poetry-api/internal/database"
	"github.com/palemoky/chinese-poetry-api/internal/loader"
	"github.com/palemoky/chinese-poetry-api/internal/processor"
	"github.com/spf13/cobra"
)

var (
	inputDir          string
	outputSimplified  string
	outputTraditional string
	workers           int
	configPath        string
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "processor",
		Short: "Chinese Poetry Data Processor",
		Long:  "Process Chinese poetry JSON data and generate SQLite databases with simplified and traditional Chinese versions",
		RunE:  run,
	}

	rootCmd.Flags().StringVarP(&inputDir, "input", "i", "poetry-data", "Input directory containing poetry JSON files")
	rootCmd.Flags().StringVarP(&outputSimplified, "output-simplified", "s", "poetry-simplified.db", "Output SQLite database for simplified Chinese")
	rootCmd.Flags().StringVarP(&outputTraditional, "output-traditional", "t", "poetry-traditional.db", "Output SQLite database for traditional Chinese")
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

	// Process simplified Chinese version
	log.Println("\n=== Processing Simplified Chinese Database ===")
	if err := processDatabase(outputSimplified, poems, false, workers); err != nil {
		return fmt.Errorf("failed to process simplified database: %w", err)
	}

	// Process traditional Chinese version
	log.Println("\n=== Processing Traditional Chinese Database ===")
	if err := processDatabase(outputTraditional, poems, true, workers); err != nil {
		return fmt.Errorf("failed to process traditional database: %w", err)
	}

	log.Println("\n=== Processing Complete ===")
	log.Printf("Simplified database: %s", outputSimplified)
	log.Printf("Traditional database: %s", outputTraditional)

	// Print statistics
	if err := printStatistics(outputSimplified, "Simplified"); err != nil {
		log.Printf("Warning: failed to print simplified statistics: %v", err)
	}

	if err := printStatistics(outputTraditional, "Traditional"); err != nil {
		log.Printf("Warning: failed to print traditional statistics: %v", err)
	}

	return nil
}

func processDatabase(dbPath string, poems []loader.PoemWithMeta, convertToTraditional bool, workers int) error {
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

	// Run migrations
	log.Println("Creating database schema...")
	if err := db.Migrate(convertToTraditional); err != nil {
		return fmt.Errorf("failed to migrate database: %w", err)
	}

	// Create repository
	repo := database.NewRepository(db)

	// Process poems
	proc := processor.NewProcessor(repo, workers, convertToTraditional)
	if err := proc.Process(poems); err != nil {
		return fmt.Errorf("failed to process poems: %w", err)
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

func printStatistics(dbPath, label string) error {
	db, err := database.Open(dbPath)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()

	repo := database.NewRepository(db)
	stats, err := repo.GetStatistics()
	if err != nil {
		return err
	}

	log.Printf("\n=== %s Database Statistics ===", label)
	log.Printf("Total Poems: %d", stats.TotalPoems)
	log.Printf("Total Authors: %d", stats.TotalAuthors)
	log.Printf("Total Dynasties: %d", stats.TotalDynasties)

	log.Println("\nPoems by Dynasty:")
	for _, ds := range stats.PoemsByDynasty {
		if ds.PoemCount > 0 {
			log.Printf("  %s: %d poems", ds.Name, ds.PoemCount)
		}
	}

	log.Println("\nPoems by Type:")
	for _, ts := range stats.PoemsByType {
		if ts.PoemCount > 0 {
			log.Printf("  %s: %d poems", ts.Name, ts.PoemCount)
		}
	}

	return nil
}
