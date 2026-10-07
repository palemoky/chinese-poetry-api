package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
	minPoems   int64
)

func main() {
	// 数据处理程序始终以 debug 模式记录日志
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
	rootCmd.PersistentFlags().Int64Var(&minPoems, "min-poems", 300000, "Fail verification when either variant has fewer poems")

	rootCmd.AddCommand(&cobra.Command{
		Use:   "verify <database>",
		Short: "Check that an imported database is fit for release",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return verifyDatabase(args[0])
		},
	})

	if err := rootCmd.Execute(); err != nil {
		logger.Fatal("Command execution failed", zap.Error(err))
	}
}

// run 是 processor 命令的主流程：加载数据、导入数据库、输出统计。
func run(cmd *cobra.Command, args []string) error {
	// 确定配置文件路径
	if configPath == "" {
		configPath = filepath.Join(inputDir, "loader", "datas.json")
	}

	logger.Info("Loading poetry data", zap.String("config", configPath))

	// 加载全部诗词数据
	jsonLoader, err := loader.NewJSONLoader(configPath)
	if err != nil {
		return fmt.Errorf("failed to create loader: %w", err)
	}

	poems, err := jsonLoader.LoadAll()
	if err != nil {
		return fmt.Errorf("failed to load poems: %w", err)
	}

	logger.Info("Loaded poems from JSON files", zap.Int("count", len(poems)))

	bios, err := jsonLoader.LoadAuthorBios()
	if err != nil {
		return fmt.Errorf("failed to load author bios: %w", err)
	}
	logger.Info("Loaded author bios", zap.Int("count", len(bios)))

	// 生成同时包含简繁两套表的统一数据库
	logger.Info("Processing unified database")
	if err := processUnifiedDatabase(outputDB, poems, bios, workers); err != nil {
		return fmt.Errorf("failed to process database: %w", err)
	}

	logger.Info("Processing complete", zap.String("database", outputDB))

	// 输出统计信息
	if err := printStatistics(outputDB); err != nil {
		logger.Warn("Failed to print statistics", zap.Error(err))
	}

	// 检查不通过时以非零状态退出，发布流程就此中止
	return verifyDatabase(outputDB)
}

// verifyDatabase 对导入完成的数据库做发布前检查，打印结果；
// 在 GitHub Actions 中还会写进任务摘要，方便人工核对朝代分布。
func verifyDatabase(dbPath string) error {
	db, err := database.OpenReadOnly(dbPath, 1, 1)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()

	report, err := db.VerifyRelease(minPoems)
	if err != nil {
		return fmt.Errorf("failed to verify database: %w", err)
	}

	md := renderReport(report)
	fmt.Print("\n" + md)
	if path := os.Getenv("GITHUB_STEP_SUMMARY"); path != "" {
		if err := appendFile(path, md); err != nil {
			logger.Warn("Failed to write job summary", zap.Error(err))
		}
	}

	if !report.Passed() {
		return fmt.Errorf("database %s failed release verification", dbPath)
	}
	return nil
}

// renderReport 把检查结果渲染为 Markdown 表格。
func renderReport(report *database.VerifyReport) string {
	var b strings.Builder
	b.WriteString("### Release verification\n\n| Check | Result | Detail |\n| --- | --- | --- |\n")
	for _, c := range report.Checks {
		result := "✅"
		if !c.Passed {
			result = "❌"
		}
		fmt.Fprintf(&b, "| %s | %s | %s |\n", c.Name, result, c.Detail)
	}
	b.WriteString("\n### Poems by dynasty (zh-Hans)\n\n| Dynasty | Poems |\n| --- | ---: |\n")
	for _, d := range report.Dynasties {
		fmt.Fprintf(&b, "| %s | %d |\n", d.Name, d.Count)
	}
	return b.String()
}

func appendFile(path, content string) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.WriteString(content); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// processUnifiedDatabase 重建数据库，并依次导入简体与繁体两套数据。
func processUnifiedDatabase(dbPath string, poems []loader.PoemWithMeta, bios []loader.AuthorBio, workers int) error {
	// 删除已存在的数据库文件
	if err := os.Remove(dbPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove existing database: %w", err)
	}

	// 数据处理场景下单连接更安全
	db, err := database.Open(dbPath, 1, 1)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer func() { _ = db.Close() }()

	// 执行迁移，创建简繁两套表
	logger.Info("Creating database schema (simplified + traditional tables)")
	if err := db.Migrate(); err != nil {
		return fmt.Errorf("failed to migrate database: %w", err)
	}

	// 处理简体版本
	logger.Info("Processing language variant", zap.String("lang", "zh-Hans"))
	repoSimp := database.NewRepositoryWithLang(db, database.LangHans)
	procSimp := processor.NewProcessor(repoSimp, workers, false)
	procSimp.SetAuthorBios(bios)
	if err := procSimp.Process(poems); err != nil {
		return fmt.Errorf("failed to process simplified poems: %w", err)
	}

	// 处理繁体版本
	logger.Info("Processing language variant", zap.String("lang", "zh-Hant"))
	repoTrad := database.NewRepositoryWithLang(db, database.LangHant)
	procTrad := processor.NewProcessor(repoTrad, workers, true)
	procTrad.SetAuthorBios(bios)
	if err := procTrad.Process(poems); err != nil {
		return fmt.Errorf("failed to process traditional poems: %w", err)
	}

	for _, repo := range []*database.Repository{repoSimp, repoTrad} {
		pruned, err := repo.PruneAuthorsWithoutPoems()
		if err != nil {
			return err
		}
		logger.Info("Pruned authors without poems", zap.Int64("count", pruned))
	}

	// 导入期间每次写入都会让倒排索引失效，全部写完后统一重建
	logger.Info("Building char index")
	if err := db.BuildCharIndexes(); err != nil {
		return fmt.Errorf("failed to build char index: %w", err)
	}

	// 删除只在导入时用到的索引与列，必须放在所有写入之后
	logger.Info("Finalizing database")
	if err := db.FinalizeImport(); err != nil {
		return fmt.Errorf("failed to finalize database: %w", err)
	}

	// 优化数据库文件
	logger.Info("Optimizing database")
	if err := db.Exec("VACUUM").Error; err != nil {
		logger.Warn("Failed to vacuum database", zap.Error(err))
	}

	if err := db.Exec("ANALYZE").Error; err != nil {
		logger.Warn("Failed to analyze database", zap.Error(err))
	}

	return nil
}

// printStatistics 打印各语言变体下的数据量统计。
func printStatistics(dbPath string) error {
	// 统计为只读操作，单连接即可
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
