package processor

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"
	"gorm.io/datatypes"

	"github.com/palemoky/chinese-poetry-api/internal/classifier"
	"github.com/palemoky/chinese-poetry-api/internal/database"
	"github.com/palemoky/chinese-poetry-api/internal/loader"
)

const (
	// Error reporting limits
	MaxErrorsToDisplay = 100 // Maximum number of errors to display
	MaxErrorsToCollect = 100 // Maximum number of errors to collect

	// Sample error display limit
	SampleErrorCount = 5 // Number of sample errors to show
)

// getOptimalConfig returns optimal configuration based on system resources
func getOptimalConfig() (workBuffer, resultBuffer, errorBuffer, defaultBatch, minBatch, maxBatch int) {
	cpuCount := runtime.NumCPU()

	// Adaptive configuration based on CPU count
	// Low-end (CI): 2 cores  → conservative settings
	// Mid-range:    4-8 cores → balanced settings
	// High-end:     10+ cores → aggressive settings

	switch {
	case cpuCount <= 2:
		// GitHub Actions, low-end CI
		return 50, 1000, 50, 200, 50, 300

	case cpuCount <= 4:
		// Entry-level machines
		return 75, 2000, 75, 300, 100, 500

	case cpuCount <= 8:
		// Mid-range machines
		return 100, 3000, 100, 400, 150, 700

	default:
		// High-end machines
		return 500, 10000, 500, 1000, 500, 2000
	}
}

// Processor handles concurrent poetry data processing
type Processor struct {
	repo                 database.RepositoryInterface
	workers              int
	convertToTraditional bool
	batchSize            int // Batch size for database insertion
}

// NewProcessor creates a new processor with caching support
func NewProcessor(repo *database.Repository, workers int, convertToTraditional bool) *Processor {
	if workers <= 0 {
		workers = runtime.NumCPU()
	}

	// Get optimal configuration based on system resources
	_, _, _, defaultBatch, _, _ := getOptimalConfig()

	// Wrap repository with caching for better performance
	cachedRepo := database.NewCachedRepository(repo)

	return &Processor{
		repo:                 cachedRepo,
		workers:              workers,
		convertToTraditional: convertToTraditional,
		batchSize:            defaultBatch,
	}
}

// SetBatchSize sets the batch size for database insertion
func (p *Processor) SetBatchSize(size int) {
	if size > 0 {
		p.batchSize = size
	}
}

// Process processes all poems with concurrent workers and batch insertion
func (p *Processor) Process(poems []loader.PoemWithMeta) error {
	total := len(poems)
	log.Printf("Processing %d poems with %d workers (batch size: %d)...\n", total, p.workers, p.batchSize)

	// Create progress container
	progress := mpb.New(
		mpb.WithWidth(60),
		mpb.WithRefreshRate(100*time.Millisecond),
	)

	// Create progress bar
	bar := progress.AddBar(int64(total),
		mpb.PrependDecorators(
			decor.Name("Processing: ", decor.WC{W: 12, C: decor.DindentRight}),
			decor.CountersNoUnit("%d / %d", decor.WCSyncWidth),
		),
		mpb.AppendDecorators(
			decor.Percentage(decor.WC{W: 5}),
			decor.Name(" | "),
			decor.AverageETA(decor.ET_STYLE_GO, decor.WC{W: 6}),
			decor.Name(" | "),
			decor.AverageSpeed(0, "%.0f poems/s", decor.WC{W: 12}),
		),
	)

	// Channels for work distribution
	// Buffer sizes are adaptive based on system resources
	// Get optimal configuration
	workBuffer, resultBuffer, errorBuffer, _, _, _ := getOptimalConfig()

	workCh := make(chan PoemWork, workBuffer)
	resultCh := make(chan *database.Poem, resultBuffer)
	errorCh := make(chan error, errorBuffer)
	var wg sync.WaitGroup

	// Progress counter
	var processed atomic.Int64
	var errorCount atomic.Int64

	// Start workers to process poems (CPU-intensive work)
	for i := range p.workers {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for work := range workCh {
				poem, err := p.processPoem(work)
				if err != nil {
					errorCount.Add(1)
					// Non-blocking error recording
					select {
					case errorCh <- fmt.Errorf("worker %d: %s - %w", workerID, work.Title, err):
					default:
						// Discard error to avoid blocking
					}
					processed.Add(1)
					bar.Increment()
					continue
				}

				// Send processed poem to result channel
				resultCh <- poem
				processed.Add(1)
				bar.Increment()
			}
		}(i)
	}

	// Start batch inserter goroutine
	insertDone := make(chan error, 1)
	go func() {
		insertDone <- p.batchInserter(resultCh)
	}()

	// Send work to workers
	go func() {
		for i, poem := range poems {
			workCh <- PoemWork{
				PoemWithMeta: poem,
				ID:           int64(i + 1), // Sequential ID starting from 1
			}
		}
		close(workCh)
	}()

	// Wait for all workers to finish processing
	wg.Wait()

	// Complete the processing progress bar before starting insertion
	bar.SetTotal(int64(total), true) // Mark as complete
	progress.Wait()                  // Wait for processing bar to finish rendering

	close(resultCh) // Signal batch inserter to finish

	// Wait for batch inserter to complete
	if err := <-insertDone; err != nil {
		return fmt.Errorf("batch insertion failed: %w", err)
	}

	close(errorCh)

	// Collect errors (non-blocking)
	var errors []error
	for err := range errorCh {
		errors = append(errors, err)
		if len(errors) >= MaxErrorsToCollect {
			break
		}
	}

	// Print summary
	successCount := processed.Load()
	failCount := errorCount.Load()

	if failCount > 0 {
		log.Printf("✓ Successfully processed: %d/%d poems", successCount-failCount, total)
		log.Printf("✗ Failed: %d poems", failCount)
		if len(errors) > 0 {
			log.Printf("Sample errors (showing %d):", min(len(errors), SampleErrorCount))
			for i := range min(len(errors), SampleErrorCount) {
				log.Printf("  %d. %v", i+1, errors[i])
			}
		}
		return fmt.Errorf("processing completed with %d errors", failCount)
	}

	log.Printf("✓ Successfully processed all %d poems", total)
	return nil
}

// batchInserter collects poems and inserts them using large transactions
// This approach reduces fsync overhead by grouping many inserts into fewer transactions
func (p *Processor) batchInserter(resultCh <-chan *database.Poem) error {
	// Collect all poems first (they're already processed)
	allPoems := make([]*database.Poem, 0, cap(resultCh))

	for poem := range resultCh {
		allPoems = append(allPoems, poem)
	}

	if len(allPoems) == 0 {
		return nil
	}

	log.Printf("[Batch Inserter] Collected %d poems, starting transaction-based insertion...", len(allPoems))

	// Create a new progress container for insertion
	progress := mpb.New(
		mpb.WithWidth(60),
		mpb.WithRefreshRate(100*time.Millisecond),
	)

	// Use large transactions for maximum performance
	// Transaction size: 20,000 poems per transaction (reduces fsync calls)
	// Batch size: use current configured batch size for inserts within transaction
	transactionSize := 20000

	err := p.repo.BatchInsertPoemsWithTransaction(allPoems, transactionSize, p.batchSize, progress)

	// Wait for progress bar to finish rendering
	progress.Wait()

	if err != nil {
		return fmt.Errorf("failed to insert poems with transactions: %w", err)
	}

	log.Printf("[Batch Inserter] Successfully inserted %d poems using large transactions", len(allPoems))
	return nil
}

// resolveTitleByCategory determines the final title based on poetry type category
// Different categories use different source fields:
// - 词 (Ci): use rhythmic (词牌名) as title, merge with subtitle if present
// - 论语/四书五经: use chapter as title
// - Others (诗/曲/诗经/楚辞/蒙学): use title
func resolveTitleByCategory(poem loader.PoemData, category string) string {
	switch category {
	case "词": // 宋词 - use rhythmic (词牌名) as title
		if poem.Rhythmic != "" {
			// Rhythmic is the main title (词牌名)
			// If there's also a title, merge them as "词牌名·副标题"
			if poem.Title != "" && poem.Title != poem.Rhythmic {
				return poem.Rhythmic + "·" + poem.Title
			}
			return poem.Rhythmic
		}
		// Fallback to title if no rhythmic
		return poem.Title

	case "论语", "四书五经": // Use chapter as title
		if poem.Chapter != "" {
			return poem.Chapter
		}
		// Fallback to title if no chapter
		return poem.Title

	default: // 唐诗, 元曲, 诗经, 楚辞, 蒙学, etc. - use title
		return poem.Title
	}
}

func (p *Processor) processPoem(work PoemWork) (*database.Poem, error) {
	poem := work.PoemData

	// Normalize all text fields (trim whitespace)
	author := classifier.NormalizeText(poem.Author)
	paragraphs := classifier.NormalizeTextArray(poem.Paragraphs)
	rhythmic := classifier.NormalizeText(poem.Rhythmic)

	// Assign default author for poems without author
	if author == "" {
		author = "佚名" // Anonymous/Unknown author
	}
	// Allow poems without title if they have content
	// Some poems may only have paragraphs without a formal title

	// Normalize Chinese characters for consistency
	// Traditional DB: convert to traditional
	// Simplified DB: convert to simplified

	author, err := p.convertText(author, p.convertToTraditional)
	if err != nil {
		return nil, fmt.Errorf("failed to convert author: %w", err)
	}

	paragraphs, err = p.convertTextArray(paragraphs, p.convertToTraditional)
	if err != nil {
		return nil, fmt.Errorf("failed to convert paragraphs: %w", err)
	}

	if rhythmic != "" {
		rhythmic, err = p.convertText(rhythmic, p.convertToTraditional)
		if err != nil {
			return nil, fmt.Errorf("failed to convert rhythmic: %w", err)
		}
	}

	// Get or create dynasty
	// Convert dynasty name to match database encoding (traditional or simplified)
	dynastyName, err := p.convertText(work.Dynasty, p.convertToTraditional)
	if err != nil {
		return nil, fmt.Errorf("failed to convert dynasty name: %w", err)
	}
	dynastyID, err := p.repo.GetOrCreateDynasty(dynastyName)
	if err != nil {
		return nil, fmt.Errorf("failed to get/create dynasty: %w", err)
	}

	// Get or create author
	authorID, err := p.repo.GetOrCreateAuthor(author, dynastyID)
	if err != nil {
		return nil, fmt.Errorf("failed to get/create author: %w", err)
	}

	// Classify poetry type using dataset source information
	typeInfo := classifier.ClassifyPoetryTypeWithDataset(paragraphs, rhythmic, work.DatasetKey)

	// Convert type name to match database encoding
	typeName, err := p.convertText(typeInfo.TypeName, p.convertToTraditional)
	if err != nil {
		return nil, fmt.Errorf("failed to convert type name: %w", err)
	}

	typeID, err := p.repo.GetPoetryTypeID(typeName)
	if err != nil {
		return nil, fmt.Errorf("failed to get poetry type: %w", err)
	}

	// Resolve final title based on category (handles 词/论语/四书五经/etc.)
	// This intelligently maps different source fields (title/rhythmic/chapter) to the final title
	finalTitle := resolveTitleByCategory(poem, typeInfo.Category)

	// Convert final title
	finalTitle, err = p.convertText(finalTitle, p.convertToTraditional)
	if err != nil {
		return nil, fmt.Errorf("failed to convert final title: %w", err)
	}

	// Use sequential ID assigned during processing
	poemID := work.ID

	// Convert paragraphs to JSON for storage
	contentJSON, err := json.Marshal(paragraphs)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal paragraphs: %w", err)
	}

	// Calculate content hash for deduplication
	hash := sha256.Sum256(contentJSON)
	contentHash := hex.EncodeToString(hash[:])

	// Create poem record
	dbPoem := &database.Poem{
		ID:          poemID,
		Title:       finalTitle, // Category-aware title (may be from title/rhythmic/chapter)
		AuthorID:    &authorID,
		DynastyID:   &dynastyID,
		TypeID:      &typeID,
		Content:     datatypes.JSON(contentJSON),
		ContentHash: contentHash,
	}

	return dbPoem, nil
}

// convertText converts text to either traditional or simplified Chinese based on the flag
func (p *Processor) convertText(text string, toTraditional bool) (string, error) {
	if toTraditional {
		return classifier.ToTraditional(text)
	}
	return classifier.ToSimplified(text)
}

// convertTextArray converts an array of text to either traditional or simplified Chinese
func (p *Processor) convertTextArray(texts []string, toTraditional bool) ([]string, error) {
	if toTraditional {
		return classifier.ToTraditionalArray(texts)
	}
	return classifier.ToSimplifiedArray(texts)
}
