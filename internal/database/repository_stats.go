package database

import "fmt"

// 本文件包含各类统计与计数方法。

// CountPoems 返回诗词总数。
//
// 读的是物化计数器而非 COUNT(*)：后者要扫完一整棵索引，
// 而这里是一次主键查找。计数器由触发器维护，见 DB.migratePoemCounter。
func (r *Repository) CountPoems() (int, error) {
	var count int64
	err := r.db.Raw(
		fmt.Sprintf("SELECT value FROM %s WHERE name = ?", countersTable),
		poemCounterName(r.lang),
	).Scan(&count).Error
	return int(count), err
}

// CountAuthors 返回作者总数。
func (r *Repository) CountAuthors() (int, error) {
	var count int64
	err := r.db.Table(r.authorsTable()).Count(&count).Error
	return int(count), err
}

// CountPoemsByAuthor 返回某位作者名下的作品数。
//
// 读的是物化列 authors.poem_count（见 DB.migrateAuthorPoemCountTriggers），
// 而不是去 poems 表数一遍：GraphQL 的 Author.poemCount 是逐行解析的字段，
// 一页 20 位作者就会调用 20 次，每次都数一遍属于纯粹的浪费。
func (r *Repository) CountPoemsByAuthor(authorID int64) (int, error) {
	var count int64
	err := r.db.Table(r.authorsTable()).
		Select("poem_count").
		Where("id = ?", authorID).
		Scan(&count).Error
	return int(count), err
}

// CountPoemsByDynasty 返回某个朝代的作品数。
func (r *Repository) CountPoemsByDynasty(dynastyID int64) (int, error) {
	return r.countPoemsWhere("dynasty_id = ?", dynastyID)
}

// CountPoemsByType 返回某种体裁的作品数。
func (r *Repository) CountPoemsByType(typeID int64) (int, error) {
	return r.countPoemsWhere("type_id = ?", typeID)
}

// CountAuthorsByDynasty 返回某朝代下至少有一首作品的作者数（去重）。
func (r *Repository) CountAuthorsByDynasty(dynastyID int64) (int, error) {
	var count int64
	err := r.db.Table(r.poemsTable()).
		Where("dynasty_id = ?", dynastyID).
		Distinct("author_id").
		Count(&count).Error
	return int(count), err
}

// countPoemsWhere 按单个条件统计诗词数量。
// 它的存在是为了让这类计数和其他查询一样统一走 poemsTable()：
// GraphQL resolver 早先用 db.Model(&Poem{}) 计数，会解析到 Poem.TableName()，
// 也就是没有语言后缀的旧表名 "poems"；简繁分表之后该表已不存在，
// 导致这些字段在运行时全部报错。
func (r *Repository) countPoemsWhere(query string, args ...any) (int, error) {
	var count int64
	err := r.db.Table(r.poemsTable()).Where(query, args...).Count(&count).Error
	return int(count), err
}

// GetStatistics 返回全库的整体统计数据。
func (r *Repository) GetStatistics() (*Statistics, error) {
	stats := &Statistics{}

	// 各项总数
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

	// 按朝代统计作品数。表名是动态的，故手写 SQL 片段
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
		Order("poem_count DESC, " + dynastyTable + ".id ASC").
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

	// 按体裁统计作品数
	typeTable := r.poetryTypesTable()

	var typeStats []struct {
		PoetryType
		PoemCount int `gorm:"column:poem_count"`
	}

	err = r.db.Table(typeTable).
		Select(typeTable + ".*, COUNT(" + poemTable + ".id) as poem_count").
		Joins("LEFT JOIN " + poemTable + " ON " + typeTable + ".id = " + poemTable + ".type_id").
		Group(typeTable + ".id").
		Order("poem_count DESC, " + typeTable + ".id ASC").
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

// ListAuthorsWithFilter 分页查询作者列表，可按朝代过滤。
//
// 作品数取自物化列 authors.poem_count（由 RefreshAuthorPoemCounts 维护），
// 而非 JOIN poems + GROUP BY 现算：后者是一次全表聚合，代价与页码无关，
// 每页都要重来，且排序无法走索引。改用物化列后排序直接吃 idx_..._poem_count。
func (r *Repository) ListAuthorsWithFilter(limit, offset int, dynastyID *int64) ([]AuthorWithStats, int, error) {
	authorTable := r.authorsTable()

	query := r.db.Table(authorTable)

	// 应用朝代过滤
	if dynastyID != nil {
		query = query.Where(authorTable+".dynasty_id = ?", *dynastyID)
	}

	// 先取满足条件的总数。与页码无关，每页都会重算，故走缓存
	key := newCountKey("authors", r.lang).addOptionalID(dynastyID).String()
	totalCount, err := r.db.counts.getOrLoad(key, func() (int64, error) {
		var n int64
		err := query.Count(&n).Error
		return n, err
	})
	if err != nil {
		return nil, 0, err
	}

	// 再取作者及其作品数。排序里带上 id 是为了在作品数相同时打破并列，
	// 否则顺序不确定，分页会出现重复和遗漏。
	var authors []AuthorWithStats
	err = query.
		Select(authorTable + ".*").
		Order("poem_count DESC, " + authorTable + ".id ASC").
		Limit(limit).Offset(offset).
		Scan(&authors).Error
	if err != nil {
		return nil, 0, err
	}

	return authors, int(totalCount), nil
}
