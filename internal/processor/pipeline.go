package processor

import (
	"encoding/json"
	"fmt"
	"log"
	"runtime"
	"strings"
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
	// Dynamic batch sizing thresholds (percentage of channel capacity)
	channelPressureHigh   = 0.8 // 80% full - reduce batch size
	channelPressureMedium = 0.5 // 50% full - normal batch size
	channelPressureLow    = 0.2 // 20% full - increase batch size

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
		return 300, 5000, 300, 500, 200, 1000
	}
}

// Processor handles concurrent poetry data processing
type Processor struct {
	repo                 database.RepositoryInterface
	workers              int
	convertToTraditional bool
	batchSize            int // Base batch size for database insertion
	minBatchSize         int // Minimum batch size (for high pressure)
	maxBatchSize         int // Maximum batch size (for low pressure)
}

// NewProcessor creates a new processor with caching support
func NewProcessor(repo *database.Repository, workers int, convertToTraditional bool) *Processor {
	if workers <= 0 {
		workers = runtime.NumCPU()
	}

	// Get optimal configuration based on system resources
	_, _, _, defaultBatch, minBatch, maxBatch := getOptimalConfig()

	// Wrap repository with caching for better performance
	cachedRepo := database.NewCachedRepository(repo)

	return &Processor{
		repo:                 cachedRepo,
		workers:              workers,
		convertToTraditional: convertToTraditional,
		batchSize:            defaultBatch,
		minBatchSize:         minBatch,
		maxBatchSize:         maxBatch,
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

	workCh := make(chan loader.PoemWithMeta, workBuffer)
	resultCh := make(chan *database.Poem, resultBuffer)
	errorCh := make(chan error, errorBuffer)
	var wg sync.WaitGroup

	// Progress counter
	var processed atomic.Int64
	var errorCount atomic.Int64

	// Start workers to process poems (CPU-intensive work)
	for i := 0; i < p.workers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for poemMeta := range workCh {
				poem, err := p.processPoem(poemMeta)
				if err != nil {
					errorCount.Add(1)
					// Non-blocking error recording
					select {
					case errorCh <- fmt.Errorf("worker %d: %s - %w", workerID, poemMeta.Title, err):
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
		for _, poem := range poems {
			workCh <- poem
		}
		close(workCh)
	}()

	// Wait for all workers to finish processing
	wg.Wait()
	close(resultCh) // Signal batch inserter to finish

	// Wait for batch inserter to complete
	if err := <-insertDone; err != nil {
		return fmt.Errorf("batch insertion failed: %w", err)
	}

	close(errorCh)

	// Wait for progress bar to finish rendering
	progress.Wait()

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
			for i := 0; i < min(len(errors), SampleErrorCount); i++ {
				log.Printf("  %d. %v", i+1, errors[i])
			}
		}
		return fmt.Errorf("processing completed with %d errors", failCount)
	}

	log.Printf("✓ Successfully processed all %d poems", total)
	return nil
}

// batchInserter collects poems and inserts them in batches with dynamic sizing
// Adjusts batch size based on channel pressure to prevent blocking
func (p *Processor) batchInserter(resultCh <-chan *database.Poem) error {
	batch := make([]*database.Poem, 0, p.maxBatchSize)
	currentBatchSize := p.batchSize // Start with configured batch size

	for poem := range resultCh {
		batch = append(batch, poem)

		// Calculate channel utilization (pressure)
		channelLen := len(resultCh)
		channelCap := cap(resultCh)
		utilization := float64(channelLen) / float64(channelCap)

		// Dynamically adjust batch size based on channel pressure
		newBatchSize := p.calculateBatchSize(utilization, currentBatchSize)

		// Log batch size changes for debugging
		if newBatchSize != currentBatchSize {
			log.Printf("[Batch Inserter] Channel utilization: %.1f%%, adjusting batch size: %d → %d",
				utilization*100, currentBatchSize, newBatchSize)
		}
		currentBatchSize = newBatchSize

		// Insert when batch reaches current size
		if len(batch) >= currentBatchSize {
			if err := p.repo.BatchInsertPoems(batch, len(batch)); err != nil {
				return fmt.Errorf("failed to insert batch of %d poems: %w", len(batch), err)
			}
			batch = batch[:0] // Reset batch
		}
	}

	// Insert remaining poems
	if len(batch) > 0 {
		if err := p.repo.BatchInsertPoems(batch, len(batch)); err != nil {
			return fmt.Errorf("failed to insert final batch of %d poems: %w", len(batch), err)
		}
	}

	return nil
}

// calculateBatchSize determines the optimal batch size based on channel utilization
// Returns the adjusted batch size, or keeps current size for smooth transitions
func (p *Processor) calculateBatchSize(utilization float64, currentSize int) int {
	switch {
	case utilization >= channelPressureHigh:
		// High pressure (≥80% full): reduce batch size for faster consumption
		return p.minBatchSize

	case utilization >= channelPressureMedium:
		// Medium pressure (≥50% full): use base batch size
		return p.batchSize

	case utilization <= channelPressureLow:
		// Low pressure (≤20% full): increase batch size for efficiency
		return p.maxBatchSize

	default:
		// Between 20-50%: keep current batch size for smooth transition
		return currentSize
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (p *Processor) processPoem(poemMeta loader.PoemWithMeta) (*database.Poem, error) {
	poem := poemMeta.PoemData

	// Normalize all text fields (trim whitespace)
	title := classifier.NormalizeText(poem.Title)
	author := classifier.NormalizeText(poem.Author)
	paragraphs := classifier.NormalizeTextArray(poem.Paragraphs)
	rhythmic := classifier.NormalizeText(poem.Rhythmic)

	// Normalize Chinese characters for consistency
	// Traditional DB: convert to traditional
	// Simplified DB: convert to simplified
	var err error

	title, err = p.convertText(title, p.convertToTraditional)
	if err != nil {
		return nil, fmt.Errorf("failed to convert title: %w", err)
	}

	author, err = p.convertText(author, p.convertToTraditional)
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
	dynastyID, err := p.repo.GetOrCreateDynasty(poemMeta.Dynasty)
	if err != nil {
		return nil, fmt.Errorf("failed to get/create dynasty: %w", err)
	}

	// Generate pinyin for author
	authorPinyin := classifier.ToPinyinNoTone(author)
	authorPinyinAbbr := classifier.ToPinyinAbbr(author)

	// Get or create author
	authorID, err := p.repo.GetOrCreateAuthor(author, authorPinyin, authorPinyinAbbr, dynastyID)
	if err != nil {
		return nil, fmt.Errorf("failed to get/create author: %w", err)
	}

	// Classify poetry type
	typeInfo := classifier.ClassifyPoetryType(paragraphs, rhythmic)

	// Convert type name to match database encoding
	typeName, err := p.convertText(typeInfo.TypeName, p.convertToTraditional)
	if err != nil {
		return nil, fmt.Errorf("failed to convert type name: %w", err)
	}

	typeID, err := p.repo.GetPoetryTypeID(typeName)
	if err != nil {
		return nil, fmt.Errorf("failed to get poetry type: %w", err)
	}

	// Merge title and rhythmic for better API design
	// For 词/曲: rhythmic is the main title (词牌名/曲牌名), title is subtitle
	// Format: "词牌名·副标题" or just "词牌名" if no subtitle
	finalTitle := title
	if rhythmic != "" && rhythmic != title {
		// Has rhythmic and it's different from title
		if title != "" {
			var builder strings.Builder
			builder.WriteString(rhythmic)
			builder.WriteString("·")
			builder.WriteString(title)
			finalTitle = builder.String() // 词牌名·副标题
		} else {
			finalTitle = rhythmic // Only 词牌名
		}
	}

	// Generate pinyin for final title
	titlePinyin := classifier.ToPinyinNoTone(finalTitle)
	titlePinyinAbbr := classifier.ToPinyinAbbr(finalTitle)

	// Generate pinyin for rhythmic (keep for search/classification)
	var rhythmicPinyin *string
	if rhythmic != "" {
		rp := classifier.ToPinyinNoTone(rhythmic)
		rhythmicPinyin = &rp
	}

	// Generate stable numeric ID based on poem content
	// This ensures the same poem always gets the same ID
	poemID := classifier.GenerateStablePoemID(finalTitle, author, paragraphs)

	// Convert paragraphs to JSON for storage
	contentJSON, err := json.Marshal(paragraphs)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal paragraphs: %w", err)
	}

	// Create poem record
	dbPoem := &database.Poem{
		ID:              poemID,
		Title:           finalTitle, // Use merged title (includes rhythmic if present)
		TitlePinyin:     &titlePinyin,
		TitlePinyinAbbr: &titlePinyinAbbr,
		AuthorID:        &authorID,
		DynastyID:       &dynastyID,
		TypeID:          &typeID,
		Content:         datatypes.JSON(contentJSON),
		Rhythmic:        &rhythmic, // Keep for search/classification
		RhythmicPinyin:  rhythmicPinyin,
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
