package database

import "gorm.io/gorm"

// Poem query methods for the Repository

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

// GetRandomPoem returns a random poem with optional filters
// This is much more efficient than using COUNT + OFFSET approach
// Uses SQLite's ORDER BY RANDOM() LIMIT 1 for optimal performance
func (r *Repository) GetRandomPoem(dynastyID, authorID, typeID *int64) (*Poem, error) {
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

	// Get a random poem using SQLite's RANDOM()
	var poems []Poem
	err := query.Order("RANDOM()").Limit(1).Find(&poems).Error
	if err != nil {
		return nil, err
	}

	// Check if we found a poem
	if len(poems) == 0 {
		return nil, gorm.ErrRecordNotFound
	}

	// Load relations for the poem
	r.loadPoemRelations(poems)
	return &poems[0], nil
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
