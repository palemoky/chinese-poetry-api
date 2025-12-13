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

// RepositoryInterface defines the interface for repository operations
type RepositoryInterface interface {
	GetOrCreateDynasty(name string) (int64, error)
	GetOrCreateAuthor(name string, dynastyID int64) (int64, error)
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
	SearchPoems(query string, searchType string, page, pageSize int) ([]Poem, int64, error)
}

// Repository handles database operations
type Repository struct {
	db   *DB
	lang Lang // Language variant for table selection (empty = default/legacy mode)
}

// NewRepository creates a new repository with default language (simplified)
func NewRepository(db *DB) *Repository {
	return &Repository{db: db, lang: LangHans}
}

// NewRepositoryWithLang creates a new repository for a specific language variant
func NewRepositoryWithLang(db *DB, lang Lang) *Repository {
	return &Repository{db: db, lang: lang}
}

// WithLang returns a new Repository instance with the specified language variant.
// This allows runtime language switching without modifying the original repository.
func (r *Repository) WithLang(lang Lang) *Repository {
	return &Repository{db: r.db, lang: lang}
}

// Table name helpers for this repository's language
func (r *Repository) poemsTable() string       { return PoemsTable(r.lang) }
func (r *Repository) authorsTable() string     { return AuthorsTable(r.lang) }
func (r *Repository) dynastiesTable() string   { return DynastiesTable(r.lang) }
func (r *Repository) poetryTypesTable() string { return PoetryTypesTable(r.lang) }

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

// GetOrCreateAuthor gets or creates an author in a thread-safe manner
// Uses stable hash-based ID and ON CONFLICT to handle concurrent inserts
// Note: Author's dynasty_id is set on first creation and not updated
// This is because some authors appear in multiple dynasty datasets

// GetPoetryTypeID gets the ID of a poetry type by name
func (r *Repository) GetPoetryTypeID(name string) (int64, error) {
	var poetryType PoetryType
	err := r.db.Table(r.poetryTypesTable()).Where("name = ?", name).First(&poetryType).Error
	if err != nil {
		return 0, err
	}
	return poetryType.ID, nil
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

// GetPoemByID retrieves a poem by ID with all relations preloaded
func (r *Repository) GetPoemByID(id string) (*Poem, error) {
	var poem Poem
	// Note: For Preload to work correctly with dynamic table names,
	// we use raw queries for related tables
	err := r.db.Table(r.poemsTable()).
		Where("id = ?", id).
		First(&poem).Error
	if err != nil {
		return nil, err
	}

	// Load author manually
	if poem.AuthorID != nil {
		var author Author
		if err := r.db.Table(r.authorsTable()).First(&author, *poem.AuthorID).Error; err == nil {
			poem.Author = &author
			// Load author's dynasty
			if author.DynastyID != nil {
				var dynasty Dynasty
				if err := r.db.Table(r.dynastiesTable()).First(&dynasty, *author.DynastyID).Error; err == nil {
					poem.Author.Dynasty = &dynasty
				}
			}
		}
	}

	// Load dynasty
	if poem.DynastyID != nil {
		var dynasty Dynasty
		if err := r.db.Table(r.dynastiesTable()).First(&dynasty, *poem.DynastyID).Error; err == nil {
			poem.Dynasty = &dynasty
		}
	}

	// Load type
	if poem.TypeID != nil {
		var ptype PoetryType
		if err := r.db.Table(r.poetryTypesTable()).First(&ptype, *poem.TypeID).Error; err == nil {
			poem.Type = &ptype
		}
	}

	return &poem, nil
}

// CountPoems returns the total number of poems
func (r *Repository) CountPoems() (int, error) {
	var count int64
	err := r.db.Table(r.poemsTable()).Count(&count).Error
	return int(count), err
}

// CountAuthors returns the total number of authors
func (r *Repository) CountAuthors() (int, error) {
	var count int64
	err := r.db.Table(r.authorsTable()).Count(&count).Error
	return int(count), err
}

// loadPoemRelations loads Author, Dynasty, and Type for a slice of poems
func (r *Repository) loadPoemRelations(poems []Poem) {
	if len(poems) == 0 {
		return
	}

	// Collect unique IDs
	authorIDs := make(map[int64]bool)
	dynastyIDs := make(map[int64]bool)
	typeIDs := make(map[int64]bool)

	for _, p := range poems {
		if p.AuthorID != nil {
			authorIDs[*p.AuthorID] = true
		}
		if p.DynastyID != nil {
			dynastyIDs[*p.DynastyID] = true
		}
		if p.TypeID != nil {
			typeIDs[*p.TypeID] = true
		}
	}

	// Load authors
	authors := make(map[int64]*Author)
	if len(authorIDs) > 0 {
		ids := make([]int64, 0, len(authorIDs))
		for id := range authorIDs {
			ids = append(ids, id)
		}
		var authorList []Author
		r.db.Table(r.authorsTable()).Where("id IN ?", ids).Find(&authorList)
		for i := range authorList {
			authors[authorList[i].ID] = &authorList[i]
			// Load author's dynasty
			if authorList[i].DynastyID != nil {
				dynastyIDs[*authorList[i].DynastyID] = true
			}
		}
	}

	// Load dynasties
	dynasties := make(map[int64]*Dynasty)
	if len(dynastyIDs) > 0 {
		ids := make([]int64, 0, len(dynastyIDs))
		for id := range dynastyIDs {
			ids = append(ids, id)
		}
		var dynastyList []Dynasty
		r.db.Table(r.dynastiesTable()).Where("id IN ?", ids).Find(&dynastyList)
		for i := range dynastyList {
			dynasties[dynastyList[i].ID] = &dynastyList[i]
		}
	}

	// Load types
	types := make(map[int64]*PoetryType)
	if len(typeIDs) > 0 {
		ids := make([]int64, 0, len(typeIDs))
		for id := range typeIDs {
			ids = append(ids, id)
		}
		var typeList []PoetryType
		r.db.Table(r.poetryTypesTable()).Where("id IN ?", ids).Find(&typeList)
		for i := range typeList {
			types[typeList[i].ID] = &typeList[i]
		}
	}

	// Assign relations to poems
	for i := range poems {
		if poems[i].AuthorID != nil {
			if author, ok := authors[*poems[i].AuthorID]; ok {
				poems[i].Author = author
				if author.DynastyID != nil {
					if d, ok := dynasties[*author.DynastyID]; ok {
						poems[i].Author.Dynasty = d
					}
				}
			}
		}
		if poems[i].DynastyID != nil {
			if dynasty, ok := dynasties[*poems[i].DynastyID]; ok {
				poems[i].Dynasty = dynasty
			}
		}
		if poems[i].TypeID != nil {
			if ptype, ok := types[*poems[i].TypeID]; ok {
				poems[i].Type = ptype
			}
		}
	}
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
	err = r.db.Table(r.dynastiesTable()).Where("name != ?", "其他").Count(&count).Error
	if err != nil {
		return nil, err
	}
	stats.TotalDynasties = int(count)

	// Poems by dynasty - use raw SQL with dynamic table names
	dynastyTable := r.dynastiesTable()
	poemTable := r.poemsTable()

	var dynastyStats []struct {
		Dynasty
		PoemCount int `gorm:"column:poem_count"`
	}

	err = r.db.Table(dynastyTable).
		Select(dynastyTable + ".*, COUNT(" + poemTable + ".id) as poem_count").
		Joins("LEFT JOIN " + poemTable + " ON " + dynastyTable + ".id = " + poemTable + ".dynasty_id").
		Group(dynastyTable + ".id").
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
	typeTable := r.poetryTypesTable()

	var typeStats []struct {
		PoetryType
		PoemCount int `gorm:"column:poem_count"`
	}

	err = r.db.Table(typeTable).
		Select(typeTable + ".*, COUNT(" + poemTable + ".id) as poem_count").
		Joins("LEFT JOIN " + poemTable + " ON " + typeTable + ".id = " + poemTable + ".type_id").
		Group(typeTable + ".id").
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

// ListPoems returns a paginated list of poems with relations loaded
func (r *Repository) ListPoems(limit, offset int) ([]Poem, error) {
	var poems []Poem
	err := r.db.Table(r.poemsTable()).
		Limit(limit).Offset(offset).
		Find(&poems).Error
	if err != nil {
		return nil, err
	}

	// Load relations for each poem
	r.loadPoemRelations(poems)
	return poems, nil
}

// ListPoemsWithFilter returns a paginated list of poems with optional filters
func (r *Repository) ListPoemsWithFilter(limit, offset int, dynastyID, authorID, typeID *int64) ([]Poem, int, error) {
	query := r.db.Table(r.poemsTable())

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
	err := query.
		Limit(limit).Offset(offset).
		Order("id DESC").
		Find(&poems).Error
	if err != nil {
		return nil, 0, err
	}

	// Load relations
	r.loadPoemRelations(poems)
	return poems, int(totalCount), nil
}

// ListAuthorPoems returns a paginated list of poems by a specific author
func (r *Repository) ListAuthorPoems(authorID int64, limit, offset int) ([]Poem, int, error) {
	var totalCount int64
	if err := r.db.Table(r.poemsTable()).Where("author_id = ?", authorID).Count(&totalCount).Error; err != nil {
		return nil, 0, err
	}

	var poems []Poem
	err := r.db.Table(r.poemsTable()).
		Where("author_id = ?", authorID).
		Limit(limit).Offset(offset).
		Order("id DESC").
		Find(&poems).Error
	if err != nil {
		return nil, 0, err
	}

	// Load relations
	r.loadPoemRelations(poems)
	return poems, int(totalCount), nil
}

// ListAuthorsWithFilter returns a paginated list of authors with optional dynasty filter
func (r *Repository) ListAuthorsWithFilter(limit, offset int, dynastyID *int64) ([]AuthorWithStats, int, error) {
	authorTable := r.authorsTable()
	poemTable := r.poemsTable()

	query := r.db.Table(authorTable)

	// Apply dynasty filter
	if dynastyID != nil {
		query = query.Where(authorTable+".dynasty_id = ?", *dynastyID)
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
		Select(authorTable + ".*, COUNT(" + poemTable + ".id) as poem_count").
		Joins("LEFT JOIN " + poemTable + " ON " + authorTable + ".id = " + poemTable + ".author_id").
		Group(authorTable + ".id").
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

	return authors, int(totalCount), nil
}

// SearchPoems searches for poems with full-text search support
// searchType can be: "all", "title", "content", "author"
func (r *Repository) SearchPoems(query string, searchType string, page, pageSize int) ([]Poem, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}

	offset := (page - 1) * pageSize
	pattern := "%" + query + "%"
	poemTable := r.poemsTable()
	authorTable := r.authorsTable()

	var poems []Poem
	var total int64

	switch searchType {
	case "title":
		// Search in title only
		r.db.Table(poemTable).Where("title LIKE ?", pattern).Count(&total)
		err := r.db.Table(poemTable).
			Where("title LIKE ?", pattern).
			Limit(pageSize).Offset(offset).
			Find(&poems).Error
		if err != nil {
			return nil, 0, err
		}

	case "content":
		// Search in content only
		r.db.Table(poemTable).Where("content LIKE ?", pattern).Count(&total)
		err := r.db.Table(poemTable).
			Where("content LIKE ?", pattern).
			Limit(pageSize).Offset(offset).
			Find(&poems).Error
		if err != nil {
			return nil, 0, err
		}

	case "author":
		// Search in author name
		r.db.Table(poemTable).
			Joins("JOIN "+authorTable+" ON "+poemTable+".author_id = "+authorTable+".id").
			Where(authorTable+".name LIKE ?", pattern).
			Count(&total)
		err := r.db.Table(poemTable).
			Joins("JOIN "+authorTable+" ON "+poemTable+".author_id = "+authorTable+".id").
			Where(authorTable+".name LIKE ?", pattern).
			Limit(pageSize).Offset(offset).
			Find(&poems).Error
		if err != nil {
			return nil, 0, err
		}

	default: // "all"
		// Search in title, content, and author name
		r.db.Table(poemTable).
			Joins("LEFT JOIN "+authorTable+" ON "+poemTable+".author_id = "+authorTable+".id").
			Where(poemTable+".title LIKE ? OR "+poemTable+".content LIKE ? OR "+authorTable+".name LIKE ?",
				pattern, pattern, pattern).
			Count(&total)
		err := r.db.Table(poemTable).
			Joins("LEFT JOIN "+authorTable+" ON "+poemTable+".author_id = "+authorTable+".id").
			Where(poemTable+".title LIKE ? OR "+poemTable+".content LIKE ? OR "+authorTable+".name LIKE ?",
				pattern, pattern, pattern).
			Limit(pageSize).Offset(offset).
			Find(&poems).Error
		if err != nil {
			return nil, 0, err
		}
	}

	// Load relations
	r.loadPoemRelations(poems)
	return poems, total, nil
}
