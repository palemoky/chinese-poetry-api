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

	"github.com/google/uuid"
	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"
	"github.com/palemoky/chinese-poetry-api/internal/classifier"
	"github.com/palemoky/chinese-poetry-api/internal/database"
	"github.com/palemoky/chinese-poetry-api/internal/loader"
)

// Processor handles concurrent poetry data processing
type Processor struct {
	repo                 *database.Repository
	workers              int
	convertToTraditional bool
}

// NewProcessor creates a new processor
func NewProcessor(repo *database.Repository, workers int, convertToTraditional bool) *Processor {
	if workers <= 0 {
		workers = runtime.NumCPU()
	}

	return &Processor{
		repo:                 repo,
		workers:              workers,
		convertToTraditional: convertToTraditional,
	}
}

// Process processes all poems with concurrent workers
func (p *Processor) Process(poems []loader.PoemWithMeta) error {
	total := len(poems)
	log.Printf("Processing %d poems with %d workers...\n", total, p.workers)

	// Create progress container
	progress := mpb.New(
		mpb.WithWidth(60),
		mpb.WithRefreshRate(100*time.Millisecond), // 更快的刷新率
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
			decor.AverageETA(decor.ET_STYLE_GO, decor.WC{W: 6}), // 使用平均ETA，立即显示
			decor.Name(" | "),
			decor.AverageSpeed(0, "%.0f poems/s", decor.WC{W: 12}), // 使用平均速度，立即显示
		),
	)

	// Create work channel
	workCh := make(chan loader.PoemWithMeta, 100)
	errorCh := make(chan error, 100)
	var wg sync.WaitGroup

	// Progress counter
	var processed atomic.Int64
	var errorCount atomic.Int64

	// Start workers
	for i := 0; i < p.workers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for poem := range workCh {
				if err := p.processPoem(poem); err != nil {
					errorCount.Add(1)
					// 非阻塞错误记录
					select {
					case errorCh <- fmt.Errorf("worker %d: %s - %w", workerID, poem.Title, err):
					default:
						// 丢弃错误，避免阻塞
					}
					processed.Add(1)
					bar.Increment() // mpb 是并发安全的
					continue
				}
				processed.Add(1)
				bar.Increment() // 直接增加进度条，并发安全
			}
		}(i)
	}

	// Send work to workers
	go func() {
		for _, poem := range poems {
			workCh <- poem
		}
		close(workCh)
	}()

	// Wait for completion
	wg.Wait()
	close(errorCh)

	// Wait for progress bar to finish rendering
	progress.Wait()

	// Collect errors (non-blocking)
	var errors []error
	for err := range errorCh {
		errors = append(errors, err)
		if len(errors) >= 100 {
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
			log.Printf("Sample errors (showing %d):", min(len(errors), 5))
			for i := 0; i < min(len(errors), 5); i++ {
				log.Printf("  %d. %v", i+1, errors[i])
			}
		}
		return fmt.Errorf("processing completed with %d errors", failCount)
	}

	log.Printf("✓ Successfully processed all %d poems", total)
	return nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (p *Processor) processPoem(poemMeta loader.PoemWithMeta) error {
	poem := poemMeta.PoemData

	// Convert to traditional if needed
	title := poem.Title
	author := poem.Author
	paragraphs := poem.Paragraphs
	rhythmic := poem.Rhythmic

	if p.convertToTraditional {
		contentJSON, _ := json.Marshal(paragraphs)
		t, a, c, r, err := classifier.ConvertPoemToTraditional(
			title,
			author,
			string(contentJSON),
			rhythmic,
		)
		if err != nil {
			return fmt.Errorf("failed to convert to traditional: %w", err)
		}
		title = t
		author = a
		rhythmic = r

		// Parse back paragraphs
		if err := json.Unmarshal([]byte(c), &paragraphs); err != nil {
			return fmt.Errorf("failed to parse converted content: %w", err)
		}
	}

	// Get or create dynasty
	dynastyID, err := p.repo.GetOrCreateDynasty(poemMeta.Dynasty)
	if err != nil {
		return fmt.Errorf("failed to get/create dynasty: %w", err)
	}

	// Generate pinyin for author
	authorPinyin := classifier.ToPinyinNoTone(author)
	authorPinyinAbbr := classifier.ToPinyinAbbr(author)

	// Get or create author
	authorID, err := p.repo.GetOrCreateAuthor(author, authorPinyin, authorPinyinAbbr, dynastyID)
	if err != nil {
		return fmt.Errorf("failed to get/create author: %w", err)
	}

	// Classify poetry type
	typeInfo := classifier.ClassifyPoetryType(paragraphs, rhythmic)
	typeID, err := p.repo.GetPoetryTypeID(typeInfo.TypeName)
	if err != nil {
		return fmt.Errorf("failed to get poetry type: %w", err)
	}

	// Generate pinyin for title
	titlePinyin := classifier.ToPinyinNoTone(title)
	titlePinyinAbbr := classifier.ToPinyinAbbr(title)

	// Generate pinyin for rhythmic
	var rhythmicPinyin *string
	if rhythmic != "" {
		rp := classifier.ToPinyinNoTone(rhythmic)
		rhythmicPinyin = &rp
	}

	// Generate ID if not present
	poemID := poem.ID
	if poemID == "" {
		poemID = uuid.New().String()
	}

	// Convert paragraphs to JSON string for storage
	contentJSON, err := json.Marshal(paragraphs)
	if err != nil {
		return fmt.Errorf("failed to marshal paragraphs: %w", err)
	}

	// Create poem record
	dbPoem := &database.Poem{
		ID:              poemID,
		Title:           title,
		TitlePinyin:     &titlePinyin,
		TitlePinyinAbbr: &titlePinyinAbbr,
		AuthorID:        &authorID,
		DynastyID:       &dynastyID,
		TypeID:          &typeID,
		Content:         string(contentJSON),
		Rhythmic:        &rhythmic,
		RhythmicPinyin:  rhythmicPinyin,
	}

	// Insert poem - handle duplicates intelligently
	if err := p.repo.InsertPoem(dbPoem); err != nil {
		// Check if it's a duplicate ID error
		if strings.Contains(err.Error(), "UNIQUE constraint failed: poems.id") {
			// Check if the existing poem has the same content (deduplication)
			existingPoem, err := p.repo.GetPoemByID(poemID)
			if err == nil && existingPoem != nil {
				// Compare content to determine if it's truly a duplicate
				// Parse both contents to compare
				var existingContent, newContent string
				if existingPoem.Content != "" { // Content is string, not *string, so check for empty string
					existingContent = existingPoem.Content
				}
				newContent = dbPoem.Content

				// If content and title match, it's a duplicate - skip it
				if existingContent == newContent && existingPoem.Title == dbPoem.Title {
					// Same content, skip this duplicate silently
					return nil
				}
			}

			// Different content with same ID, generate new unique ID
			dbPoem.ID = poemID + "-" + uuid.New().String()[:8]
			// Retry insertion with new ID
			if err := p.repo.InsertPoem(dbPoem); err != nil {
				return fmt.Errorf("failed to insert poem with new ID: %w", err)
			}
			return nil
		}
		return fmt.Errorf("failed to insert poem: %w", err)
	}

	return nil
}
