package database

// 本文件包含供 REST API handler 使用的补充查询方法。

// GetAuthorsWithStats 返回作者列表及各自的作品数量。
func (r *Repository) GetAuthorsWithStats(limit, offset int) ([]AuthorWithStats, error) {
	authorTable := r.authorsTable()
	dynastyTable := r.dynastiesTable()

	var authors []AuthorWithStats

	// 作品数取自物化列 authors.poem_count，理由见 ListAuthorsWithFilter。
	//
	// 排序里加上 id 是为了在 poem_count 相同时打破并列：
	// 否则大量作品数相同的作者之间顺序不确定，执行计划一变，
	// LIMIT/OFFSET 分页就会出现重复和遗漏。
	err := r.db.Table(authorTable).
		Select(authorTable + ".*").
		Order("poem_count DESC, " + authorTable + ".id ASC").
		Limit(limit).
		Offset(offset).
		Find(&authors).Error
	if err != nil {
		return nil, err
	}

	// 为每位作者补上所属朝代
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

// GetAuthorByID 按 ID 查询作者。
func (r *Repository) GetAuthorByID(id int64) (*Author, error) {
	var author Author
	err := r.db.Table(r.authorsTable()).First(&author, id).Error
	if err != nil {
		return nil, err
	}

	// 加载所属朝代
	if author.DynastyID != nil {
		var dynasty Dynasty
		if err := r.db.Table(r.dynastiesTable()).First(&dynasty, *author.DynastyID).Error; err == nil {
			author.Dynasty = &dynasty
		}
	}

	return &author, nil
}

// GetAuthorByName 按姓名查询作者。
func (r *Repository) GetAuthorByName(name string) (*Author, error) {
	var author Author
	err := r.db.Table(r.authorsTable()).Where("name = ?", name).First(&author).Error
	if err != nil {
		return nil, err
	}

	// 加载所属朝代
	if author.DynastyID != nil {
		var dynasty Dynasty
		if err := r.db.Table(r.dynastiesTable()).First(&dynasty, *author.DynastyID).Error; err == nil {
			author.Dynasty = &dynasty
		}
	}

	return &author, nil
}

// GetPoemsByAuthor 查询指定作者的诗词。
//
// 排序用 id ASC 而非 created_at：created_at 取的是批量导入时的 CURRENT_TIMESTAMP，
// 同一批诗词的取值完全相同，作为唯一排序键没有 tiebreaker，翻页时会重复和遗漏。
// id 唯一且单调，同时与 ListPoemsWithFilter 的排序保持一致。
func (r *Repository) GetPoemsByAuthor(authorID int64, limit, offset int) ([]Poem, error) {
	var poems []Poem
	err := r.db.Table(r.poemsTable()).
		Where("author_id = ?", authorID).
		Order("id ASC").
		Limit(limit).
		Offset(offset).
		Find(&poems).Error
	if err != nil {
		return nil, err
	}

	r.loadPoemRelations(poems)
	return poems, nil
}

// GetDynastiesWithStats 返回朝代列表及各自的作品数与作者数。
func (r *Repository) GetDynastiesWithStats() ([]DynastyWithStats, error) {
	dynastyTable := r.dynastiesTable()
	poemTable := r.poemsTable()
	authorTable := r.authorsTable()

	var dynasties []DynastyWithStats

	// 数据量大时子查询比 JOIN 更快，故此处用子查询统计
	err := r.db.Table(dynastyTable).
		Select(dynastyTable + ".*, " +
			"(SELECT COUNT(*) FROM " + poemTable + " WHERE " + poemTable + ".dynasty_id = " + dynastyTable + ".id) as poem_count, " +
			"(SELECT COUNT(*) FROM " + authorTable + " WHERE " + authorTable + ".dynasty_id = " + dynastyTable + ".id) as author_count").
		Order("poem_count DESC, " + dynastyTable + ".id ASC").
		Find(&dynasties).Error

	return dynasties, err
}

// GetDynastyByID 按 ID 查询朝代。
func (r *Repository) GetDynastyByID(id int64) (*Dynasty, error) {
	var dynasty Dynasty
	err := r.db.Table(r.dynastiesTable()).First(&dynasty, id).Error
	return &dynasty, err
}

// GetDynastyByName 按名称查询朝代。
func (r *Repository) GetDynastyByName(name string) (*Dynasty, error) {
	var dynasty Dynasty
	err := r.db.Table(r.dynastiesTable()).Where("name = ?", name).First(&dynasty).Error
	return &dynasty, err
}

// GetPoemsByDynasty 查询指定朝代的诗词。排序理由同 GetPoemsByAuthor。
func (r *Repository) GetPoemsByDynasty(dynastyID int64, limit, offset int) ([]Poem, error) {
	var poems []Poem
	err := r.db.Table(r.poemsTable()).
		Where("dynasty_id = ?", dynastyID).
		Order("id ASC").
		Limit(limit).
		Offset(offset).
		Find(&poems).Error
	if err != nil {
		return nil, err
	}

	r.loadPoemRelations(poems)
	return poems, nil
}

// GetPoetryTypesWithStats 返回体裁列表及各自的作品数量。
func (r *Repository) GetPoetryTypesWithStats() ([]PoetryTypeWithStats, error) {
	typeTable := r.poetryTypesTable()
	poemTable := r.poemsTable()

	var types []PoetryTypeWithStats

	// 数据量大时子查询比 JOIN 更快
	err := r.db.Table(typeTable).
		Select(typeTable + ".*, (SELECT COUNT(*) FROM " + poemTable + " WHERE " + poemTable + ".type_id = " + typeTable + ".id) as poem_count").
		Order("poem_count DESC, " + typeTable + ".id ASC").
		Find(&types).Error

	return types, err
}

// GetPoetryTypeByID 按 ID 查询体裁。
func (r *Repository) GetPoetryTypeByID(id int64) (*PoetryType, error) {
	var poetryType PoetryType
	err := r.db.Table(r.poetryTypesTable()).First(&poetryType, id).Error
	return &poetryType, err
}

// GetPoemsByType 查询指定体裁的诗词。排序理由同 GetPoemsByAuthor。
func (r *Repository) GetPoemsByType(typeID int64, limit, offset int) ([]Poem, error) {
	var poems []Poem
	err := r.db.Table(r.poemsTable()).
		Where("type_id = ?", typeID).
		Order("id ASC").
		Limit(limit).
		Offset(offset).
		Find(&poems).Error
	if err != nil {
		return nil, err
	}

	r.loadPoemRelations(poems)
	return poems, nil
}
