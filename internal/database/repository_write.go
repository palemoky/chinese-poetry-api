package database

import (
	"fmt"

	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/palemoky/chinese-poetry-api/internal/logger"
)

// Write operations for data processing

// GetOrCreateDynasty gets or creates a dynasty by name in a thread-safe manner
// Uses ON CONFLICT to handle concurrent inserts gracefully
func (r *Repository) GetOrCreateDynasty(name string) (int64, error) {
	dynasty := Dynasty{Name: name}

	// Try to create the dynasty with ON CONFLICT DO NOTHING
	err := r.db.Table(r.dynastiesTable()).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "name"}},
		DoNothing: true, // Ignore if already exists
	}).Create(&dynasty).Error
	if err != nil {
		return 0, err
	}

	// If dynasty.ID is 0, it means the insert was skipped (already exists)
	// We need to fetch the existing dynasty
	if dynasty.ID == 0 {
		err = r.db.Table(r.dynastiesTable()).Where("name = ?", name).First(&dynasty).Error
		if err != nil {
			return 0, err
		}
	}

	return dynasty.ID, nil
}

// GetOrCreateAuthor gets or creates an author in a thread-safe manner
// Uses Name as unique key and ON CONFLICT to handle concurrent inserts
// Note: Author's dynasty_id is set on first creation and not updated
// This is because some authors appear in multiple dynasty datasets
func (r *Repository) GetOrCreateAuthor(name string, dynastyID int64) (int64, error) {
	author := Author{
		Name:      name,
		DynastyID: &dynastyID,
	}

	// Try to create the author with ON CONFLICT DO NOTHING
	// This handles concurrent inserts gracefully
	err := r.db.Table(r.authorsTable()).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "name"}},
		DoNothing: true, // Ignore if already exists
	}).Create(&author).Error
	if err != nil {
		return 0, err
	}

	// If author.ID is 0, it means the insert was skipped (already exists)
	// We need to fetch the existing author
	if author.ID == 0 {
		err = r.db.Table(r.authorsTable()).Where("name = ?", name).First(&author).Error
		if err != nil {
			return 0, err
		}
	}

	return author.ID, nil
}

// GetPoetryTypeID gets the ID of a poetry type by name
func (r *Repository) GetPoetryTypeID(name string) (int64, error) {
	var poetryType PoetryType
	err := r.db.Table(r.poetryTypesTable()).Where("name = ?", name).First(&poetryType).Error
	if err != nil {
		return 0, err
	}
	return poetryType.ID, nil
}

// GetPoetryTypeIDs gets IDs for multiple poetry types by name in a single query
// Returns IDs in the same order as the input names
// Returns error if any of the requested types are not found
func (r *Repository) GetPoetryTypeIDs(names []string) ([]int64, error) {
	if len(names) == 0 {
		return []int64{}, nil
	}

	var poetryTypes []PoetryType
	err := r.db.Table(r.poetryTypesTable()).
		Where("name IN ?", names).
		Find(&poetryTypes).Error
	if err != nil {
		return nil, err
	}

	// Check if we found all requested types
	if len(poetryTypes) != len(names) {
		return nil, gorm.ErrRecordNotFound
	}

	// Create a map for O(1) lookup
	typeMap := make(map[string]int64, len(poetryTypes))
	for _, pt := range poetryTypes {
		typeMap[pt.Name] = pt.ID
	}

	// Return IDs in the same order as input names
	ids := make([]int64, len(names))
	for i, name := range names {
		id, ok := typeMap[name]
		if !ok {
			return nil, gorm.ErrRecordNotFound
		}
		ids[i] = id
	}

	return ids, nil
}

// InsertPoem inserts a poem into the database
func (r *Repository) InsertPoem(poem *Poem) error {
	return r.db.Table(r.poemsTable()).Create(poem).Error
}

// BatchInsertPoems inserts multiple poems in batches for better performance
// Handles duplicate IDs by skipping them (ON CONFLICT DO NOTHING)
func (r *Repository) BatchInsertPoems(poems []*Poem, batchSize int) error {
	if len(poems) == 0 {
		return nil
	}

	if batchSize <= 0 {
		batchSize = 100 // Default batch size
	}

	// Use GORM's CreateInBatches with OnConflict to handle duplicates
	// Skip duplicates based on composite unique index (title, author_id, content_hash)
	return r.db.Table(r.poemsTable()).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "title"}, {Name: "author_id"}, {Name: "content_hash"}},
		DoNothing: true, // Skip duplicates
	}).CreateInBatches(poems, batchSize).Error
}

// BatchInsertPoemsWithTransaction inserts poems in large transactions for maximum performance
// This reduces fsync overhead by grouping multiple batches into one transaction
// transactionSize: number of poems per transaction (e.g., 10000)
// batchSize: number of poems per insert statement (e.g., 1000)
// progress: progress container for displaying transaction progress
func (r *Repository) BatchInsertPoemsWithTransaction(poems []*Poem, transactionSize, batchSize int, progress *mpb.Progress) error {
	if len(poems) == 0 {
		return nil
	}

	if transactionSize <= 0 {
		transactionSize = 20000 // Default: 20k poems per transaction
	}
	if batchSize <= 0 {
		batchSize = 1000 // Default: 1000 poems per insert
	}

	totalTransactions := (len(poems) + transactionSize - 1) / transactionSize

	// Create progress bar for poems (not transactions) for smoother updates
	var poemBar *mpb.Bar
	if progress != nil {
		poemBar = progress.AddBar(int64(len(poems)),
			mpb.PrependDecorators(
				decor.Name("Inserting Poems: ", decor.WC{W: 17, C: decor.DindentRight}),
				decor.CountersNoUnit("%d / %d", decor.WCSyncWidth),
			),
			mpb.AppendDecorators(
				decor.Percentage(decor.WC{W: 5}),
				decor.Name(" | "),
				decor.AverageETA(decor.ET_STYLE_GO, decor.WC{W: 6}),
			),
		)
	}

	logger.Info("Starting batch insertion",
		zap.Int("poems", len(poems)),
		zap.Int("transactions", totalTransactions),
		zap.Int("batch_size", batchSize),
	)

	// Process poems in large transaction chunks
	for i := 0; i < len(poems); i += transactionSize {
		end := min(i+transactionSize, len(poems))
		transactionChunk := poems[i:end]

		// Execute one large transaction with manual batching for progress updates
		err := r.db.Transaction(func(tx *gorm.DB) error {
			// Manually batch insert within transaction to update progress bar
			for j := 0; j < len(transactionChunk); j += batchSize {
				batchEnd := min(j+batchSize, len(transactionChunk))
				batch := transactionChunk[j:batchEnd]

				// Insert this batch with deduplication
				err := tx.Table(r.poemsTable()).Clauses(clause.OnConflict{
					Columns:   []clause.Column{{Name: "title"}, {Name: "author_id"}, {Name: "content_hash"}},
					DoNothing: true,
				}).Create(&batch).Error
				if err != nil {
					return err
				}

				// Update progress bar after each batch
				if poemBar != nil {
					poemBar.IncrBy(len(batch))
				}
			}
			return nil
		})
		if err != nil {
			txNum := i/transactionSize + 1
			return fmt.Errorf("failed to insert transaction %d/%d (poems %d-%d): %w",
				txNum, totalTransactions, i, end, err)
		}
	}

	return nil
}

// UpsertPoem inserts or updates a poem (for handling duplicates)
func (r *Repository) UpsertPoem(poem *Poem) error {
	return r.db.Table(r.poemsTable()).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{"title", "content", "author_id", "dynasty_id", "type_id"}),
	}).Create(poem).Error
}
