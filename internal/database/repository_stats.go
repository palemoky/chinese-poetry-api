package database

// Statistics and counting methods

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
