package database

import (
	"fmt"
	"log"

	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/palemoky/chinese-poetry-api/internal/classifier"
)

// RepositoryInterface defines the interface for repository operations
type RepositoryInterface interface {
	GetOrCreateDynasty(name string) (int64, error)
	GetOrCreateAuthor(name, namePinyin, namePinyinAbbr string, dynastyID int64) (int64, error)
	GetPoetryTypeID(name string) (int64, error)
	InsertPoem(poem *Poem) error
	BatchInsertPoems(poems []*Poem, batchSize int) error
	BatchInsertPoemsWithTransaction(poems []*Poem, transactionSize, batchSize int, progress *mpb.Progress) error
	UpsertPoem(poem *Poem) error
	GetPoemByID(id string) (*Poem, error)
	CountPoems() (int, error)
	CountAuthors() (int, error)
	GetStatistics() (*Statistics, error)
	ListPoems(limit, offset int) ([]Poem, error)
	ListPoemsWithFilter(limit, offset int, dynastyID, authorID, typeID *int64) ([]Poem, int, error)
	ListAuthorPoems(authorID int64, limit, offset int) ([]Poem, int, error)
	ListAuthorsWithFilter(limit, offset int, dynastyID *int64) ([]AuthorWithStats, int, error)
	SearchPoems(query string, limit int) ([]Poem, error)
}

// Repository handles database operations
type Repository struct {
	db *DB
}

// NewRepository creates a new repository
func NewRepository(db *DB) *Repository {
	return &Repository{db: db}
}

// GetOrCreateDynasty gets or creates a dynasty by name in a thread-safe manner
// Uses ON CONFLICT to handle concurrent inserts gracefully
func (r *Repository) GetOrCreateDynasty(name string) (int64, error) {
	dynasty := Dynasty{Name: name}

	// Try to create the dynasty with ON CONFLICT DO NOTHING
	err := r.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "name"}},
		DoNothing: true, // Ignore if already exists
	}).Create(&dynasty).Error
	if err != nil {
		return 0, err
	}

	// If dynasty.ID is 0, it means the insert was skipped (already exists)
	// We need to fetch the existing dynasty
	if dynasty.ID == 0 {
		err = r.db.Where("name = ?", name).First(&dynasty).Error
		if err != nil {
			return 0, err
		}
	}

	return dynasty.ID, nil
}

// GetOrCreateAuthor gets or creates an author in a thread-safe manner
// Uses stable hash-based ID and ON CONFLICT to handle concurrent inserts
// Note: Author's dynasty_id is set on first creation and not updated
// This is because some authors appear in multiple dynasty datasets
func (r *Repository) GetOrCreateAuthor(name, namePinyin, namePinyinAbbr string, dynastyID int64) (int64, error) {
	// Generate stable 6-digit ID based on author name
	authorID := classifier.GenerateStableAuthorID(name)

	author := Author{
		ID:             authorID,
		Name:           name,
		NamePinyin:     &namePinyin,
		NamePinyinAbbr: &namePinyinAbbr,
		DynastyID:      &dynastyID,
	}

	// Try to create the author with ON CONFLICT DO NOTHING
	// This handles concurrent inserts gracefully
	err := r.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}}, // Changed from "name" to "id"
		DoNothing: true,                          // Ignore if already exists
	}).Create(&author).Error
	if err != nil {
		return 0, err
	}

	// If RowsAffected is 0, it means the insert was skipped (already exists)
	// The author variable still has the correct ID from our generation

	return author.ID, nil
}

// GetPoetryTypeID gets the ID of a poetry type by name
func (r *Repository) GetPoetryTypeID(name string) (int64, error) {
	var poetryType PoetryType
	err := r.db.Where("name = ?", name).First(&poetryType).Error
	if err != nil {
		return 0, err
	}
	return poetryType.ID, nil
}

// InsertPoem inserts a poem into the database
func (r *Repository) InsertPoem(poem *Poem) error {
	return r.db.Create(poem).Error
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
	// DoNothing: skip duplicate IDs (same as the single insert behavior)
	return r.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
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

	log.Printf("[Database] Starting batch insertion: %d poems in %d transactions (batch size: %d)",
		len(poems), totalTransactions, batchSize)

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

				// Insert this batch
				err := tx.Clauses(clause.OnConflict{
					Columns:   []clause.Column{{Name: "id"}},
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
	return r.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{"title", "content", "author_id", "dynasty_id", "type_id"}),
	}).Create(poem).Error
}

// GetPoemByID retrieves a poem by ID with all relations preloaded
func (r *Repository) GetPoemByID(id string) (*Poem, error) {
	var poem Poem
	err := r.db.Preload("Author").Preload("Dynasty").Preload("Type").
		First(&poem, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return &poem, nil
}

// CountPoems returns the total number of poems
func (r *Repository) CountPoems() (int, error) {
	var count int64
	err := r.db.Model(&Poem{}).Count(&count).Error
	return int(count), err
}

// CountAuthors returns the total number of authors
func (r *Repository) CountAuthors() (int, error) {
	var count int64
	err := r.db.Model(&Author{}).Count(&count).Error
	return int(count), err
}

// GetStatistics returns overall statistics
func (r *Repository) GetStatistics() (*Statistics, error) {
	stats := &Statistics{}

	// Total counts
	var err error
	stats.TotalPoems, err = r.CountPoems()
	if err != nil {
		return nil, err
	}

	stats.TotalAuthors, err = r.CountAuthors()
	if err != nil {
		return nil, err
	}

	var count int64
	err = r.db.Model(&Dynasty{}).Where("name != ?", "其他").Count(&count).Error
	if err != nil {
		return nil, err
	}
	stats.TotalDynasties = int(count)

	// Poems by dynasty
	var dynastyStats []struct {
		Dynasty
		PoemCount int `gorm:"column:poem_count"`
	}

	err = r.db.Model(&Dynasty{}).
		Select("dynasties.*, COUNT(poems.id) as poem_count").
		Joins("LEFT JOIN poems ON dynasties.id = poems.dynasty_id").
		Group("dynasties.id").
		Order("poem_count DESC").
		Scan(&dynastyStats).Error
	if err != nil {
		return nil, err
	}

	for _, ds := range dynastyStats {
		stats.PoemsByDynasty = append(stats.PoemsByDynasty, DynastyWithStats{
			Dynasty:   ds.Dynasty,
			PoemCount: ds.PoemCount,
		})
	}

	// Poems by type
	var typeStats []struct {
		PoetryType
		PoemCount int `gorm:"column:poem_count"`
	}

	err = r.db.Model(&PoetryType{}).
		Select("poetry_types.*, COUNT(poems.id) as poem_count").
		Joins("LEFT JOIN poems ON poetry_types.id = poems.type_id").
		Group("poetry_types.id").
		Order("poem_count DESC").
		Scan(&typeStats).Error
	if err != nil {
		return nil, err
	}

	for _, ts := range typeStats {
		stats.PoemsByType = append(stats.PoemsByType, PoetryTypeWithStats{
			PoetryType: ts.PoetryType,
			PoemCount:  ts.PoemCount,
		})
	}

	return stats, nil
}

// ListPoems returns a paginated list of poems
func (r *Repository) ListPoems(limit, offset int) ([]Poem, error) {
	var poems []Poem
	err := r.db.Preload("Author").Preload("Dynasty").Preload("Type").
		Limit(limit).Offset(offset).
		Find(&poems).Error
	return poems, err
}

// ListPoemsWithFilter returns a paginated list of poems with optional filters
func (r *Repository) ListPoemsWithFilter(limit, offset int, dynastyID, authorID, typeID *int64) ([]Poem, int, error) {
	query := r.db.Model(&Poem{})

	// Apply filters
	if dynastyID != nil {
		query = query.Where("dynasty_id = ?", *dynastyID)
	}
	if authorID != nil {
		query = query.Where("author_id = ?", *authorID)
	}
	if typeID != nil {
		query = query.Where("type_id = ?", *typeID)
	}

	// Get total count
	var totalCount int64
	if err := query.Count(&totalCount).Error; err != nil {
		return nil, 0, err
	}

	// Get paginated results
	var poems []Poem
	err := query.Preload("Author").Preload("Dynasty").Preload("Type").
		Limit(limit).Offset(offset).
		Order("id DESC").
		Find(&poems).Error

	return poems, int(totalCount), err
}

// ListAuthorPoems returns a paginated list of poems by a specific author
func (r *Repository) ListAuthorPoems(authorID int64, limit, offset int) ([]Poem, int, error) {
	var totalCount int64
	if err := r.db.Model(&Poem{}).Where("author_id = ?", authorID).Count(&totalCount).Error; err != nil {
		return nil, 0, err
	}

	var poems []Poem
	err := r.db.Preload("Author").Preload("Dynasty").Preload("Type").
		Where("author_id = ?", authorID).
		Limit(limit).Offset(offset).
		Order("id DESC").
		Find(&poems).Error

	return poems, int(totalCount), err
}

// ListAuthorsWithFilter returns a paginated list of authors with optional dynasty filter
func (r *Repository) ListAuthorsWithFilter(limit, offset int, dynastyID *int64) ([]AuthorWithStats, int, error) {
	query := r.db.Model(&Author{})

	// Apply dynasty filter
	if dynastyID != nil {
		query = query.Where("dynasty_id = ?", *dynastyID)
	}

	// Get total count
	var totalCount int64
	if err := query.Count(&totalCount).Error; err != nil {
		return nil, 0, err
	}

	// Get authors with poem counts
	var results []struct {
		Author
		PoemCount int `gorm:"column:poem_count"`
	}

	err := query.
		Select("authors.*, COUNT(poems.id) as poem_count").
		Joins("LEFT JOIN poems ON authors.id = poems.author_id").
		Group("authors.id").
		Order("poem_count DESC").
		Limit(limit).Offset(offset).
		Scan(&results).Error
	if err != nil {
		return nil, 0, err
	}

	// Convert to AuthorWithStats
	authors := make([]AuthorWithStats, len(results))
	for i, r := range results {
		authors[i] = AuthorWithStats{
			Author:    r.Author,
			PoemCount: r.PoemCount,
		}
	}

	return authors, int(totalCount), err
}

// SearchPoems searches poems using FTS5
func (r *Repository) SearchPoems(query string, limit int) ([]Poem, error) {
	var poemIDs []string

	// Search in FTS table
	err := r.db.Raw(`
		SELECT poem_id FROM poems_fts
		WHERE poems_fts MATCH ?
		ORDER BY rank
		LIMIT ?
	`, query, limit).Scan(&poemIDs).Error
	if err != nil {
		return nil, err
	}

	if len(poemIDs) == 0 {
		return []Poem{}, nil
	}

	// Get full poem records
	var poems []Poem
	err = r.db.Preload("Author").Preload("Dynasty").Preload("Type").
		Where("id IN ?", poemIDs).
		Find(&poems).Error

	return poems, err
}
