package database

// Additional repository methods for REST API handlers

// GetAuthorsWithStats returns authors with their poem counts
func (r *Repository) GetAuthorsWithStats(limit, offset int) ([]AuthorWithStats, error) {
	var authors []AuthorWithStats

	// Use subquery for better performance on large datasets
	err := r.db.Table("authors").
		Select("authors.*, (SELECT COUNT(*) FROM poems WHERE poems.author_id = authors.id) as poem_count").
		Order("poem_count DESC").
		Limit(limit).
		Offset(offset).
		Find(&authors).Error

	return authors, err
}

// GetAuthorByID returns an author by ID
func (r *Repository) GetAuthorByID(id int64) (*Author, error) {
	var author Author
	err := r.db.Preload("Dynasty").First(&author, id).Error
	return &author, err
}

// GetPoemsByAuthor returns poems by a specific author
func (r *Repository) GetPoemsByAuthor(authorID int64, limit, offset int) ([]Poem, error) {
	var poems []Poem
	err := r.db.
		Preload("Author").
		Preload("Dynasty").
		Preload("Type").
		Where("author_id = ?", authorID).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&poems).Error
	return poems, err
}

// GetDynastiesWithStats returns dynasties with their poem and author counts
func (r *Repository) GetDynastiesWithStats() ([]DynastyWithStats, error) {
	var dynasties []DynastyWithStats

	// Use subqueries instead of JOINs for better performance on large datasets
	err := r.db.Table("dynasties").
		Select("dynasties.*, " +
			"(SELECT COUNT(*) FROM poems WHERE poems.dynasty_id = dynasties.id) as poem_count, " +
			"(SELECT COUNT(*) FROM authors WHERE authors.dynasty_id = dynasties.id) as author_count").
		Order("poem_count DESC").
		Find(&dynasties).Error

	return dynasties, err
}

// GetDynastyByID returns a dynasty by ID
func (r *Repository) GetDynastyByID(id int64) (*Dynasty, error) {
	var dynasty Dynasty
	err := r.db.First(&dynasty, id).Error
	return &dynasty, err
}

// GetPoemsByDynasty returns poems from a specific dynasty
func (r *Repository) GetPoemsByDynasty(dynastyID int64, limit, offset int) ([]Poem, error) {
	var poems []Poem
	err := r.db.
		Preload("Author").
		Preload("Dynasty").
		Preload("Type").
		Where("dynasty_id = ?", dynastyID).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&poems).Error
	return poems, err
}

// GetPoetryTypesWithStats returns poetry types with their poem counts
func (r *Repository) GetPoetryTypesWithStats() ([]PoetryTypeWithStats, error) {
	var types []PoetryTypeWithStats

	// Use subquery for better performance on large datasets
	err := r.db.Table("poetry_types").
		Select("poetry_types.*, (SELECT COUNT(*) FROM poems WHERE poems.type_id = poetry_types.id) as poem_count").
		Order("poem_count DESC").
		Find(&types).Error

	return types, err
}

// GetPoetryTypeByID returns a poetry type by ID
func (r *Repository) GetPoetryTypeByID(id int64) (*PoetryType, error) {
	var poetryType PoetryType
	err := r.db.First(&poetryType, id).Error
	return &poetryType, err
}

// GetPoemsByType returns poems of a specific type
func (r *Repository) GetPoemsByType(typeID int64, limit, offset int) ([]Poem, error) {
	var poems []Poem
	err := r.db.
		Preload("Author").
		Preload("Dynasty").
		Preload("Type").
		Where("type_id = ?", typeID).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&poems).Error
	return poems, err
}
