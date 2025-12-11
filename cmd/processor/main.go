package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"

	"github.com/palemoky/chinese-poetry-api/internal/classifier"
	"github.com/palemoky/chinese-poetry-api/internal/database"
	"github.com/palemoky/chinese-poetry-api/internal/loader"
	"github.com/palemoky/chinese-poetry-api/internal/processor"
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

	// Print comparison statistics
	if err := printComparisonStatistics(outputSimplified, outputTraditional); err != nil {
		log.Printf("Warning: failed to print statistics: %v", err)
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

func printComparisonStatistics(simplifiedPath, traditionalPath string) error {
	// Load simplified stats
	dbSimp, err := database.Open(simplifiedPath)
	if err != nil {
		return fmt.Errorf("failed to open simplified database: %w", err)
	}
	defer func() { _ = dbSimp.Close() }()

	repoSimp := database.NewRepository(dbSimp)
	statsSimp, err := repoSimp.GetStatistics()
	if err != nil {
		return fmt.Errorf("failed to get simplified statistics: %w", err)
	}

	// Load traditional stats
	dbTrad, err := database.Open(traditionalPath)
	if err != nil {
		return fmt.Errorf("failed to open traditional database: %w", err)
	}
	defer func() { _ = dbTrad.Close() }()

	repoTrad := database.NewRepository(dbTrad)
	statsTrad, err := repoTrad.GetStatistics()
	if err != nil {
		return fmt.Errorf("failed to get traditional statistics: %w", err)
	}

	log.Println("\n=== Database Statistics Comparison ===")

	// Overview comparison table
	overviewData := [][]string{
		{"Metric", "Simplified", "Traditional", "Difference"},
		{
			"Total Poems",
			fmt.Sprintf("%d", statsSimp.TotalPoems),
			fmt.Sprintf("%d", statsTrad.TotalPoems),
			fmt.Sprintf("%+d", statsTrad.TotalPoems-statsSimp.TotalPoems),
		},
		{
			"Total Authors",
			fmt.Sprintf("%d", statsSimp.TotalAuthors),
			fmt.Sprintf("%d", statsTrad.TotalAuthors),
			fmt.Sprintf("%+d", statsTrad.TotalAuthors-statsSimp.TotalAuthors),
		},
		{
			"Total Dynasties",
			fmt.Sprintf("%d", statsSimp.TotalDynasties),
			fmt.Sprintf("%d", statsTrad.TotalDynasties),
			fmt.Sprintf("%+d", statsTrad.TotalDynasties-statsSimp.TotalDynasties),
		},
	}
	overviewTable := tablewriter.NewWriter(os.Stdout)
	overviewTable.Header(overviewData[0])
	_ = overviewTable.Bulk(overviewData[1:])
	_ = overviewTable.Render()

	// Poems by Dynasty comparison table
	fmt.Println("\nPoems by Dynasty:")
	dynastyMap := make(map[string][2]int) // [simplified, traditional]
	for _, ds := range statsSimp.PoemsByDynasty {
		if ds.PoemCount > 0 {
			entry := dynastyMap[ds.Name]
			entry[0] = ds.PoemCount
			dynastyMap[ds.Name] = entry
		}
	}
	for _, ds := range statsTrad.PoemsByDynasty {
		if ds.PoemCount > 0 {
			// Convert traditional name to simplified for matching
			simpName, err := classifier.ToSimplified(ds.Name)
			if err != nil {
				simpName = ds.Name // fallback to original if conversion fails
			}
			entry := dynastyMap[simpName]
			entry[1] = ds.PoemCount
			dynastyMap[simpName] = entry
		}
	}

	dynastyData := [][]string{{"Dynasty", "Simplified", "Traditional", "Difference"}}
	for _, ds := range statsSimp.PoemsByDynasty {
		if counts, ok := dynastyMap[ds.Name]; ok && (counts[0] > 0 || counts[1] > 0) {
			diff := counts[1] - counts[0]
			dynastyData = append(dynastyData, []string{
				ds.Name,
				fmt.Sprintf("%d", counts[0]),
				fmt.Sprintf("%d", counts[1]),
				fmt.Sprintf("%+d", diff),
			})
		}
	}
	dynastyTable := tablewriter.NewWriter(os.Stdout)
	dynastyTable.Header(dynastyData[0])
	_ = dynastyTable.Bulk(dynastyData[1:])
	_ = dynastyTable.Render()

	// Poems by Type comparison table
	fmt.Println("\nPoems by Type:")
	typeMap := make(map[string][2]int) // [simplified, traditional]
	for _, ts := range statsSimp.PoemsByType {
		if ts.PoemCount > 0 {
			entry := typeMap[ts.Name]
			entry[0] = ts.PoemCount
			typeMap[ts.Name] = entry
		}
	}
	for _, ts := range statsTrad.PoemsByType {
		if ts.PoemCount > 0 {
			// Convert traditional name to simplified for matching
			simpName, err := classifier.ToSimplified(ts.Name)
			if err != nil {
				simpName = ts.Name // fallback to original if conversion fails
			}
			entry := typeMap[simpName]
			entry[1] = ts.PoemCount
			typeMap[simpName] = entry
		}
	}

	typeData := [][]string{{"Type", "Simplified", "Traditional", "Difference"}}
	for _, ts := range statsSimp.PoemsByType {
		if counts, ok := typeMap[ts.Name]; ok && (counts[0] > 0 || counts[1] > 0) {
			diff := counts[1] - counts[0]
			typeData = append(typeData, []string{
				ts.Name,
				fmt.Sprintf("%d", counts[0]),
				fmt.Sprintf("%d", counts[1]),
				fmt.Sprintf("%+d", diff),
			})
		}
	}
	typeTable := tablewriter.NewWriter(os.Stdout)
	typeTable.Header(typeData[0])
	_ = typeTable.Bulk(typeData[1:])
	_ = typeTable.Render()

	return nil
}
