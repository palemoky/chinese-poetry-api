package database

// Additional repository methods for REST API handlers

// GetAuthorsWithStats returns authors with their poem counts
func (r *Repository) GetAuthorsWithStats(limit, offset int) ([]AuthorWithStats, error) {
	authorTable := r.authorsTable()
	poemTable := r.poemsTable()
	dynastyTable := r.dynastiesTable()

	var authors []AuthorWithStats

	// Use subquery for better performance on large datasets
	err := r.db.Table(authorTable).
		Select(authorTable + ".*, (SELECT COUNT(*) FROM " + poemTable + " WHERE " + poemTable + ".author_id = " + authorTable + ".id) as poem_count").
		Order("poem_count DESC").
		Limit(limit).
		Offset(offset).
		Find(&authors).Error
	if err != nil {
		return nil, err
	}

	// Load dynasty for each author
	dynastyIDs := make(map[int64]bool)
	for _, a := range authors {
		if a.DynastyID != nil {
			dynastyIDs[*a.DynastyID] = true
		}
	}

	if len(dynastyIDs) > 0 {
		ids := make([]int64, 0, len(dynastyIDs))
		for id := range dynastyIDs {
			ids = append(ids, id)
		}
		var dynasties []Dynasty
		r.db.Table(dynastyTable).Where("id IN ?", ids).Find(&dynasties)

		dynastyMap := make(map[int64]*Dynasty)
		for i := range dynasties {
			dynastyMap[dynasties[i].ID] = &dynasties[i]
		}

		for i := range authors {
			if authors[i].DynastyID != nil {
				if d, ok := dynastyMap[*authors[i].DynastyID]; ok {
					authors[i].Dynasty = d
				}
			}
		}
	}

	return authors, nil
}

// GetAuthorByID returns an author by ID
func (r *Repository) GetAuthorByID(id int64) (*Author, error) {
	var author Author
	err := r.db.Table(r.authorsTable()).First(&author, id).Error
	if err != nil {
		return nil, err
	}

	// Load dynasty
	if author.DynastyID != nil {
		var dynasty Dynasty
		if err := r.db.Table(r.dynastiesTable()).First(&dynasty, *author.DynastyID).Error; err == nil {
			author.Dynasty = &dynasty
		}
	}

	return &author, nil
}

// GetPoemsByAuthor returns poems by a specific author
func (r *Repository) GetPoemsByAuthor(authorID int64, limit, offset int) ([]Poem, error) {
	var poems []Poem
	err := r.db.Table(r.poemsTable()).
		Where("author_id = ?", authorID).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&poems).Error
	if err != nil {
		return nil, err
	}

	r.loadPoemRelations(poems)
	return poems, nil
}

// GetDynastiesWithStats returns dynasties with their poem and author counts
func (r *Repository) GetDynastiesWithStats() ([]DynastyWithStats, error) {
	dynastyTable := r.dynastiesTable()
	poemTable := r.poemsTable()
	authorTable := r.authorsTable()

	var dynasties []DynastyWithStats

	// Use subqueries instead of JOINs for better performance on large datasets
	err := r.db.Table(dynastyTable).
		Select(dynastyTable + ".*, " +
			"(SELECT COUNT(*) FROM " + poemTable + " WHERE " + poemTable + ".dynasty_id = " + dynastyTable + ".id) as poem_count, " +
			"(SELECT COUNT(*) FROM " + authorTable + " WHERE " + authorTable + ".dynasty_id = " + dynastyTable + ".id) as author_count").
		Order("poem_count DESC").
		Find(&dynasties).Error

	return dynasties, err
}

// GetDynastyByID returns a dynasty by ID
func (r *Repository) GetDynastyByID(id int64) (*Dynasty, error) {
	var dynasty Dynasty
	err := r.db.Table(r.dynastiesTable()).First(&dynasty, id).Error
	return &dynasty, err
}

// GetPoemsByDynasty returns poems from a specific dynasty
func (r *Repository) GetPoemsByDynasty(dynastyID int64, limit, offset int) ([]Poem, error) {
	var poems []Poem
	err := r.db.Table(r.poemsTable()).
		Where("dynasty_id = ?", dynastyID).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&poems).Error
	if err != nil {
		return nil, err
	}

	r.loadPoemRelations(poems)
	return poems, nil
}

// GetPoetryTypesWithStats returns poetry types with their poem counts
func (r *Repository) GetPoetryTypesWithStats() ([]PoetryTypeWithStats, error) {
	typeTable := r.poetryTypesTable()
	poemTable := r.poemsTable()

	var types []PoetryTypeWithStats

	// Use subquery for better performance on large datasets
	err := r.db.Table(typeTable).
		Select(typeTable + ".*, (SELECT COUNT(*) FROM " + poemTable + " WHERE " + poemTable + ".type_id = " + typeTable + ".id) as poem_count").
		Order("poem_count DESC").
		Find(&types).Error

	return types, err
}

// GetPoetryTypeByID returns a poetry type by ID
func (r *Repository) GetPoetryTypeByID(id int64) (*PoetryType, error) {
	var poetryType PoetryType
	err := r.db.Table(r.poetryTypesTable()).First(&poetryType, id).Error
	return &poetryType, err
}

// GetPoemsByType returns poems of a specific type
func (r *Repository) GetPoemsByType(typeID int64, limit, offset int) ([]Poem, error) {
	var poems []Poem
	err := r.db.Table(r.poemsTable()).
		Where("type_id = ?", typeID).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&poems).Error
	if err != nil {
		return nil, err
	}

	r.loadPoemRelations(poems)
	return poems, nil
}
