package database

import (
	"crypto/rand"
	"math/big"
	"strconv"

	"gorm.io/gorm"
)

// 本文件包含 Repository 的诗词查询方法。

// GetPoemByID 按 ID 查询单首诗词，并加载全部关联数据。
func (r *Repository) GetPoemByID(id string) (*Poem, error) {
	var poem Poem
	// 注意：表名是动态的（简繁两套表），GORM 的 Preload 无法正确处理，因此关联数据统一改为手动查询
	err := r.db.Table(r.poemsTable()).
		Where("id = ?", id).
		First(&poem).Error
	if err != nil {
		return nil, err
	}

	// 加载作者
	if poem.AuthorID != nil {
		var author Author
		if err := r.db.Table(r.authorsTable()).First(&author, *poem.AuthorID).Error; err == nil {
			poem.Author = &author
			// 加载作者所属朝代
			if author.DynastyID != nil {
				var dynasty Dynasty
				if err := r.db.Table(r.dynastiesTable()).First(&dynasty, *author.DynastyID).Error; err == nil {
					poem.Author.Dynasty = &dynasty
				}
			}
		}
	}

	// 加载诗词所属朝代
	if poem.DynastyID != nil {
		var dynasty Dynasty
		if err := r.db.Table(r.dynastiesTable()).First(&dynasty, *poem.DynastyID).Error; err == nil {
			poem.Dynasty = &dynasty
		}
	}

	// 加载体裁
	if poem.TypeID != nil {
		var ptype PoetryType
		if err := r.db.Table(r.poetryTypesTable()).First(&ptype, *poem.TypeID).Error; err == nil {
			poem.Type = &ptype
		}
	}

	return &poem, nil
}

// loadPoemRelations 为一批诗词批量加载作者、朝代与体裁，
// 通过先收集 ID 再按 IN 查询的方式避免 N+1 查询。
func (r *Repository) loadPoemRelations(poems []Poem) {
	if len(poems) == 0 {
		return
	}

	// 收集去重后的关联 ID
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

	// 批量加载作者
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
			// 作者的朝代也一并纳入待查集合
			if authorList[i].DynastyID != nil {
				dynastyIDs[*authorList[i].DynastyID] = true
			}
		}
	}

	// 批量加载朝代
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

	// 批量加载体裁
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

	// 回填关联对象
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

// ListPoemsWithFilter 按可选条件分页查询诗词列表。
// 多个 typeID 之间是 OR 关系，与 GetRandomPoem 的行为保持一致。
//
// 结果固定按 id 升序排列：诗词 ID 是导入时顺序分配的（见 processor.Pipeline），
// 因此升序即语料本身的顺序，与「最新」无关。本仓储的所有分页查询都用同一排序，
// 以保证 REST 与 GraphQL 对「第 N 页」的理解一致。
func (r *Repository) ListPoemsWithFilter(limit, offset int, dynastyID, authorID *int64, typeIDs []int64) ([]Poem, int, error) {
	return r.listPoems(limit, offset, nil, dynastyID, authorID, typeIDs)
}

// ListPoemsAfter 与 ListPoemsWithFilter 返回相同的列表与排序，但用游标而非 OFFSET
// 定位起点：after 为上一页最后一条诗词的 id，只返回 id 大于它的记录。
// after 为 nil 时等价于取第一页。
//
// 相比 OFFSET，这里是一次 id 索引上的区间扫描，代价与翻到第几页无关，
// 因此不受 handler.MaxOffset 的深度限制。代价是只能顺序前进，无法直接跳到第 N 页。
func (r *Repository) ListPoemsAfter(limit int, after *int64, dynastyID, authorID *int64, typeIDs []int64) ([]Poem, int, error) {
	return r.listPoems(limit, 0, after, dynastyID, authorID, typeIDs)
}

// listPoems 是 OFFSET 与游标两种分页方式的共同实现：两者只在如何定位起点上有区别，
// 过滤条件、计数与排序必须完全一致，否则同一份数据在两种翻页方式下会呈现出不同的顺序。
func (r *Repository) listPoems(limit, offset int, after *int64, dynastyID, authorID *int64, typeIDs []int64) ([]Poem, int, error) {
	applyFilters := func(q *gorm.DB) *gorm.DB {
		if dynastyID != nil {
			q = q.Where("dynasty_id = ?", *dynastyID)
		}
		if authorID != nil {
			q = q.Where("author_id = ?", *authorID)
		}
		if len(typeIDs) > 0 {
			q = q.Where("type_id IN ?", typeIDs)
		}
		return q
	}

	// 先取满足条件的总数。这一步与页码无关，每页都会重算。
	// 注意 after 不参与计数：totalCount 是整个结果集的大小，不是剩余条数。
	//
	// 无过滤时读物化计数器：COUNT(*) 要扫完一整棵索引，是唯一一个代价随表大小
	// 增长的计数。带过滤时走的是索引区间扫描，代价随命中数增长，交给 TTL 缓存即可。
	var totalCount int64
	var err error
	if dynastyID == nil && authorID == nil && len(typeIDs) == 0 {
		var n int
		n, err = r.CountPoems()
		totalCount = int64(n)
	} else {
		key := newCountKey("poems", r.lang).
			addOptionalID(dynastyID).
			addOptionalID(authorID).
			addIDs(typeIDs).
			String()
		totalCount, err = r.db.counts.getOrLoad(key, func() (int64, error) {
			var n int64
			err := applyFilters(r.db.Table(r.poemsTable())).Count(&n).Error
			return n, err
		})
	}
	if err != nil {
		return nil, 0, err
	}

	// 再取当前分页数据
	query := applyFilters(r.db.Table(r.poemsTable()))
	if after != nil {
		query = query.Where("id > ?", *after)
	}

	var poems []Poem
	err = query.
		Limit(limit).Offset(offset).
		Order("id ASC").
		Find(&poems).Error
	if err != nil {
		return nil, 0, err
	}

	r.loadPoemRelations(poems)
	return poems, int(totalCount), nil
}

// GetRandomPoem 按可选条件随机返回一首诗词，多个体裁之间为 OR 关系。
// 采用「先 COUNT 再随机 OFFSET」的方式，保证结果在过滤集合内均匀分布。
func (r *Repository) GetRandomPoem(dynastyID, authorID *int64, typeIDs []int64) (*Poem, error) {
	poemTable := r.poemsTable()

	// 把过滤条件统一附加到查询上
	applyFilters := func(q *gorm.DB) *gorm.DB {
		if dynastyID != nil {
			q = q.Where("dynasty_id = ?", *dynastyID)
		}
		if authorID != nil {
			q = q.Where("author_id = ?", *authorID)
		}
		if len(typeIDs) > 0 {
			q = q.Where("type_id IN ?", typeIDs)
		}
		return q
	}

	// 统计命中数量
	var count int64
	if err := applyFilters(r.db.Table(poemTable)).Count(&count).Error; err != nil || count == 0 {
		return nil, gorm.ErrRecordNotFound
	}

	// 在 [0, count) 内取随机偏移
	randomBig, err := rand.Int(rand.Reader, big.NewInt(count))
	if err != nil {
		return nil, err
	}
	offset := int(randomBig.Int64())

	// 取该偏移位置上的诗词
	var poem Poem
	err = applyFilters(r.db.Table(poemTable)).Order("id ASC").Offset(offset).Limit(1).First(&poem).Error
	if err != nil {
		return nil, err
	}

	// 再按 ID 完整加载一次，补齐关联数据
	return r.GetPoemByID(strconv.FormatInt(poem.ID, 10))
}

// GetRandomPoemByChar 随机返回一首正文包含指定汉字的诗词（用于飞花令等玩法）。
// 与 GetRandomPoem 不同，此方法有意不支持叠加作者/体裁/朝代过滤：
// 它依赖 FTS 联表来定位候选，与其他方法使用的 id/dynasty/author/type 过滤属于
// 两种不同的查询形态，混用会让「除汉字外不接受其他过滤条件」这一 API 约定
// （由 handler 层强制）在不知不觉中被破坏。
// 同样采用「先 COUNT 再随机 OFFSET」保证均匀分布。
func (r *Repository) GetRandomPoemByChar(char string) (*Poem, error) {
	poemTable := r.poemsTable()
	ftsTable := r.poemsFtsTable()
	pattern := "%" + char + "%"

	matches := func(q *gorm.DB) *gorm.DB {
		return q.Joins("JOIN "+ftsTable+" ON "+ftsTable+".rowid = "+poemTable+".id").
			Where(ftsTable+".content_text LIKE ?", pattern)
	}

	// 统计命中数量
	var count int64
	if err := matches(r.db.Table(poemTable)).Count(&count).Error; err != nil || count == 0 {
		return nil, gorm.ErrRecordNotFound
	}

	// 在 [0, count) 内取随机偏移
	randomBig, err := rand.Int(rand.Reader, big.NewInt(count))
	if err != nil {
		return nil, err
	}
	offset := int(randomBig.Int64())

	// 取该偏移位置上的诗词
	var poem Poem
	err = matches(r.db.Table(poemTable)).
		Select(poemTable + ".*").
		Order(poemTable + ".id ASC").
		Offset(offset).Limit(1).First(&poem).Error
	if err != nil {
		return nil, err
	}

	// 再按 ID 完整加载一次，补齐关联数据
	return r.GetPoemByID(strconv.FormatInt(poem.ID, 10))
}

// ListAuthorPoems 分页查询指定作者的诗词。
func (r *Repository) ListAuthorPoems(authorID int64, limit, offset int) ([]Poem, int, error) {
	key := newCountKey("author_poems", r.lang).addID(authorID).String()
	totalCount, err := r.db.counts.getOrLoad(key, func() (int64, error) {
		var n int64
		err := r.db.Table(r.poemsTable()).Where("author_id = ?", authorID).Count(&n).Error
		return n, err
	})
	if err != nil {
		return nil, 0, err
	}

	var poems []Poem
	err = r.db.Table(r.poemsTable()).
		Where("author_id = ?", authorID).
		Limit(limit).Offset(offset).
		Order("id ASC").
		Find(&poems).Error
	if err != nil {
		return nil, 0, err
	}

	r.loadPoemRelations(poems)
	return poems, int(totalCount), nil
}

// SearchPoems 基于建立在标题与正文上的 FTS5 trigram 索引搜索诗词，
// 索引的创建见 migrateFtsForLang。trigram 分词器使得 LIKE '%...%' 可以走 FTS 索引，
// 无需全表扫描 poems，同时保留子串匹配语义——包括 FTS5 经典 MATCH 无法处理的
// 单字、双字中文查询。
// searchType 可取："all"、"title"、"content"、"author"。
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
	ftsTable := r.poemsFtsTable()
	ftsJoin := "JOIN " + ftsTable + " ON " + ftsTable + ".rowid = " + poemTable + ".id"

	authorJoin := "JOIN " + authorTable + " ON " + poemTable + ".author_id = " + authorTable + ".id"

	// 每种搜索模式只在「附加哪些 join 与 where」上有区别，把这部分抽成一个函数，
	// 计数与取数就能共用同一份条件，不必两处各写一遍、各自漏改。
	var applyMatch func(*gorm.DB) *gorm.DB
	switch searchType {
	case "title":
		// 仅搜标题，走 FTS trigram 索引
		applyMatch = func(q *gorm.DB) *gorm.DB {
			return q.Joins(ftsJoin).Where(ftsTable+".title LIKE ?", pattern)
		}

	case "content":
		// 仅搜正文，走 FTS trigram 索引
		applyMatch = func(q *gorm.DB) *gorm.DB {
			return q.Joins(ftsJoin).Where(ftsTable+".content_text LIKE ?", pattern)
		}

	case "author":
		// 仅搜作者名。作者表很小，普通 LIKE 足够快
		applyMatch = func(q *gorm.DB) *gorm.DB {
			return q.Joins(authorJoin).Where(authorTable+".name LIKE ?", pattern)
		}

	default: // "all"
		// 标题、正文（走 FTS）与作者名一并搜索
		applyMatch = func(q *gorm.DB) *gorm.DB {
			return q.Joins(ftsJoin).
				Joins("LEFT JOIN "+authorTable+" ON "+poemTable+".author_id = "+authorTable+".id").
				Where(ftsTable+".title LIKE ? OR "+ftsTable+".content_text LIKE ? OR "+authorTable+".name LIKE ?",
					pattern, pattern, pattern)
		}
	}

	// 计数与页码无关，且要把同一个 FTS join 再跑一遍，代价与取数相当，故走缓存。
	// 键含用户输入的关键词，缓存的条目数上限见 countCacheMaxEntries。
	key := newCountKey("search", r.lang).add(searchType).add(query).String()
	total, err := r.db.counts.getOrLoad(key, func() (int64, error) {
		var n int64
		err := applyMatch(r.db.Table(poemTable)).Count(&n).Error
		return n, err
	})
	if err != nil {
		return nil, 0, err
	}

	var poems []Poem
	err = applyMatch(r.db.Table(poemTable)).
		Select(poemTable + ".*").
		Order(poemTable + ".id").
		Limit(pageSize).Offset(offset).
		Find(&poems).Error
	if err != nil {
		return nil, 0, err
	}

	r.loadPoemRelations(poems)
	return poems, total, nil
}
