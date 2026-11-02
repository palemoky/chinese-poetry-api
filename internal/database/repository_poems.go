package database

import (
	"crypto/rand"
	"math/big"
	"slices"
	"strings"

	"gorm.io/gorm"
)

// 本文件包含 Repository 的诗词查询方法。

// GetPoemByID 按 ID 查询单首诗词，并加载全部关联数据。
func (r *Repository) GetPoemByID(id string) (*Poem, error) {
	var poem Poem
	// 注意：表名是动态的（简繁两套表），GORM 的 Preload 无法正确处理，因此关联数据统一改为手动查询
	err := r.conn().Table(r.poemsTable()).
		Where("id = ?", id).
		First(&poem).Error
	if err != nil {
		return nil, err
	}

	// 加载作者
	if poem.AuthorID != nil {
		var author Author
		if err := r.conn().Table(r.authorsTable()).First(&author, *poem.AuthorID).Error; err == nil {
			poem.Author = &author
			// 加载作者所属朝代
			if author.DynastyID != nil {
				var dynasty Dynasty
				if err := r.conn().Table(r.dynastiesTable()).First(&dynasty, *author.DynastyID).Error; err == nil {
					poem.Author.Dynasty = &dynasty
				}
			}
		}
	}

	// 加载诗词所属朝代
	if poem.DynastyID != nil {
		var dynasty Dynasty
		if err := r.conn().Table(r.dynastiesTable()).First(&dynasty, *poem.DynastyID).Error; err == nil {
			poem.Dynasty = &dynasty
		}
	}

	// 加载体裁
	if poem.TypeID != nil {
		var ptype PoetryType
		if err := r.conn().Table(r.poetryTypesTable()).First(&ptype, *poem.TypeID).Error; err == nil {
			poem.Type = &ptype
		}
	}

	return &poem, nil
}

// loadPoemRelations 为一批诗词批量加载作者、朝代与体裁，
// 通过先收集 ID 再按 IN 查询的方式避免 N+1 查询。
//
// 查询失败时返回错误，而不是带着缺失的关联数据照常返回：
// 否则一次数据库故障在调用方看来只是「这批诗恰好没有作者」，无从察觉。
func (r *Repository) loadPoemRelations(poems []Poem) error {
	if len(poems) == 0 {
		return nil
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
		if err := r.conn().Table(r.authorsTable()).Where("id IN ?", ids).Find(&authorList).Error; err != nil {
			return err
		}
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
		if err := r.conn().Table(r.dynastiesTable()).Where("id IN ?", ids).Find(&dynastyList).Error; err != nil {
			return err
		}
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
		if err := r.conn().Table(r.poetryTypesTable()).Where("id IN ?", ids).Find(&typeList).Error; err != nil {
			return err
		}
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

	return nil
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
	typeIDs, err := r.expandCategoryTypes(typeIDs)
	if err != nil {
		return nil, 0, err
	}

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
			err := applyFilters(r.conn().Table(r.poemsTable())).Count(&n).Error
			return n, err
		})
	}
	if err != nil {
		return nil, 0, err
	}

	// 再取当前分页数据
	query := applyFilters(r.conn().Table(r.poemsTable()))
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

	if err := r.loadPoemRelations(poems); err != nil {
		return nil, 0, err
	}
	return poems, int(totalCount), nil
}

// GetRandomPoem 按可选条件随机返回一首诗词，多个体裁之间为 OR 关系。
// 采用「先 COUNT 再随机 OFFSET」的方式，保证结果在过滤集合内均匀分布。
// 过滤集合为空时返回 gorm.ErrRecordNotFound，其余错误原样返回。
func (r *Repository) GetRandomPoem(dynastyID, authorID *int64, typeIDs []int64) (*Poem, error) {
	typeIDs, err := r.expandCategoryTypes(typeIDs)
	if err != nil {
		return nil, err
	}

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

	// 计数与 listPoems 共用同一套来源与缓存键：无过滤读计数器，带过滤走 TTL 缓存。
	// 随机接口是被反复刷新的那一类，每次都现数一遍很不划算。
	var count int64
	if dynastyID == nil && authorID == nil && len(typeIDs) == 0 {
		var n int
		n, err = r.CountPoems()
		count = int64(n)
	} else {
		key := newCountKey("poems", r.lang).
			addOptionalID(dynastyID).
			addOptionalID(authorID).
			addIDs(typeIDs).
			String()
		count, err = r.db.counts.getOrLoad(key, func() (int64, error) {
			var n int64
			err := applyFilters(r.conn().Table(r.poemsTable())).Count(&n).Error
			return n, err
		})
	}
	if err != nil {
		return nil, err
	}

	return r.pickRandomPoem(count, func() *gorm.DB {
		return applyFilters(r.conn().Table(r.poemsTable())).Order("id ASC")
	})
}

// expandCategoryTypes 把 typeIDs 中代表整个分类的体裁展开为该分类下的全部体裁，返回升序去重的 ID。
//
// 「唐诗」既是一个体裁，也是五言绝句、七言律诗等体裁所属的分类。按格律能判定的诗都归入
// 具体的体裁，「唐诗」体裁本身没有作品，按它过滤因此什么也查不到（#120）。
// 名称与所属分类相同的体裁视为分类本身，按它过滤即按整个分类过滤。
// 体裁表只有十几行，这条查询的代价可以忽略。
func (r *Repository) expandCategoryTypes(typeIDs []int64) ([]int64, error) {
	if len(typeIDs) == 0 {
		return typeIDs, nil
	}
	table := r.poetryTypesTable()
	var ids []int64
	err := r.conn().Table(table).
		Where("id IN ?", typeIDs).
		Or("category IN (?)", r.conn().Table(table).
			Select("category").
			Where("id IN ? AND name = category", typeIDs)).
		Order("id").
		Pluck("id", &ids).Error
	if err != nil {
		return nil, err
	}
	// 不存在的 ID 查不到行，原样保留，使其照旧匹配不到任何诗词
	for _, id := range typeIDs {
		if !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)
	return slices.Compact(ids), nil
}

// GetRandomPoemByChar 随机返回一首正文包含指定汉字的诗词（用于飞花令等玩法）。
// 与 GetRandomPoem 不同，此方法有意不支持叠加作者/体裁/朝代过滤：
// 它依赖 FTS 联表来定位候选，与其他方法使用的 id/dynasty/author/type 过滤属于
// 两种不同的查询形态，混用会让「除汉字外不接受其他过滤条件」这一 API 约定
// （由 handler 层强制）在不知不觉中被破坏。
// 同样采用「先 COUNT 再随机 OFFSET」保证均匀分布。
func (r *Repository) GetRandomPoemByChar(char string) (*Poem, error) {
	// 有倒排索引时直接在命中列表里随机取，省掉 COUNT 与 OFFSET 两次全表扫描
	if ids, ok, err := r.shortTermMatches(char, "content"); err != nil {
		return nil, err
	} else if ok {
		if len(ids) == 0 {
			return nil, gorm.ErrRecordNotFound
		}
		i, err := rand.Int(rand.Reader, big.NewInt(int64(len(ids))))
		if err != nil {
			return nil, err
		}
		poems, err := r.PoemsByIDs(ids[i.Int64() : i.Int64()+1])
		if err != nil {
			return nil, err
		}
		if len(poems) == 0 {
			return nil, gorm.ErrRecordNotFound
		}
		return &poems[0], nil
	}

	poemTable := r.poemsTable()
	ftsTable := r.poemsFtsTable()
	cond, arg := substringMatch(ftsTable+".content_text", char)

	matches := func() *gorm.DB {
		return r.conn().Table(poemTable).
			Joins("JOIN "+ftsTable+" ON "+ftsTable+".rowid = "+poemTable+".id").
			Where(cond, arg)
	}

	// 单字不足 trigram 的三字符，用不上 FTS 索引，COUNT 要扫一遍整张 FTS 表，
	// 因此计数走缓存；汉字的取值空间有限，缓存很快就能覆盖常用字。
	key := newCountKey("char", r.lang).add(char).String()
	count, err := r.db.counts.getOrLoad(key, func() (int64, error) {
		var n int64
		err := matches().Count(&n).Error
		return n, err
	})
	if err != nil {
		return nil, err
	}

	return r.pickRandomPoem(count, func() *gorm.DB {
		return matches().Order(poemTable + ".id ASC")
	})
}

// pickRandomPoem 在 query 的 count 条有序结果里均匀随机取一首，并补齐关联数据。
// count 为 0 时返回 gorm.ErrRecordNotFound。
//
// 关联数据用 loadPoemRelations 补齐。先前的写法是取出整行后再按 ID 调一次
// GetPoemByID：同一行查两遍，外加逐个查作者、作者朝代、诗词朝代与体裁，
// 一首诗 6 次查询；现在是取数 1 次加关联数据至多 3 次 IN 查询。
func (r *Repository) pickRandomPoem(count int64, query func() *gorm.DB) (*Poem, error) {
	if count == 0 {
		return nil, gorm.ErrRecordNotFound
	}

	randomBig, err := rand.Int(rand.Reader, big.NewInt(count))
	if err != nil {
		return nil, err
	}

	// 计数可能来自缓存，与当前数据存在短暂偏差；偏移落空时按「没有结果」处理，
	// 而不是报 500——语料在运行期是只读的，这只会发生在导入刚改动数据之后。
	var poems []Poem
	err = query().
		Select(r.poemsTable() + ".*").
		Offset(int(randomBig.Int64())).Limit(1).
		Find(&poems).Error
	if err != nil {
		return nil, err
	}
	if len(poems) == 0 {
		return nil, gorm.ErrRecordNotFound
	}

	if err := r.loadPoemRelations(poems); err != nil {
		return nil, err
	}
	return &poems[0], nil
}

// ListAuthorPoems 分页查询指定作者的诗词。
func (r *Repository) ListAuthorPoems(authorID int64, limit, offset int) ([]Poem, int, error) {
	key := newCountKey("author_poems", r.lang).addID(authorID).String()
	totalCount, err := r.db.counts.getOrLoad(key, func() (int64, error) {
		var n int64
		err := r.conn().Table(r.poemsTable()).Where("author_id = ?", authorID).Count(&n).Error
		return n, err
	})
	if err != nil {
		return nil, 0, err
	}

	var poems []Poem
	err = r.conn().Table(r.poemsTable()).
		Where("author_id = ?", authorID).
		Limit(limit).Offset(offset).
		Order("id ASC").
		Find(&poems).Error
	if err != nil {
		return nil, 0, err
	}

	if err := r.loadPoemRelations(poems); err != nil {
		return nil, 0, err
	}
	return poems, int(totalCount), nil
}

// SearchPoems 基于建立在标题与正文上的 FTS5 trigram 索引搜索诗词，
// 索引的创建见 migrateFtsForLang。trigram 分词器使得 LIKE '%...%' 可以走 FTS 索引，
// 无需全表扫描 poems，同时保留子串匹配语义——包括 FTS5 经典 MATCH 无法处理的
// 单字、双字中文查询。
// searchType 可取："all"、"title"、"content"、"author"。
func (r *Repository) SearchPoems(query string, searchType string, page, pageSize int) ([]Poem, int64, error) {
	return r.SearchPoemsInDynasty(query, searchType, nil, page, pageSize)
}

// SearchPoemsInDynasty 与 SearchPoems 相同，dynastyID 不为 nil 时只返回该朝代的诗词，
// 排序与不限朝代时一致。
func (r *Repository) SearchPoemsInDynasty(query string, searchType string, dynastyID *int64, page, pageSize int) ([]Poem, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}

	offset := (page - 1) * pageSize

	if searchType == "content" {
		// 只搜正文时所有命中同分，保持 ID 顺序。1～2 字的查询走倒排索引，
		// 其余直接在 SQL 里分页，不必取出完整列表
		if ids, ok, err := r.shortTermMatches(query, searchType); err != nil {
			return nil, 0, err
		} else if ok {
			ids, err = r.inDynasty(ids, dynastyID, searchType, query)
			if err != nil {
				return nil, 0, err
			}
			return r.pageOfIDs(ids, offset, pageSize)
		}
		return r.searchContent(query, dynastyID, offset, pageSize)
	}

	ranked, err := r.rankedIDs(query, searchType)
	if err != nil {
		return nil, 0, err
	}
	ranked, err = r.inDynasty(ranked, dynastyID, searchType, query)
	if err != nil {
		return nil, 0, err
	}
	return r.pageOfIDs(ranked, offset, pageSize)
}

// dynastyFilterChunk 是 inDynasty 每条查询带的 ID 个数，低于 SQLite 默认的 32766 个参数上限。
const dynastyFilterChunk = 10000

// inDynasty 保持顺序地筛出 ids 中属于 dynastyID 的诗词，dynastyID 为 nil 时原样返回。
// ids 须是 searchType 下 query 的完整命中列表，结果按这三者与朝代进 idLists 缓存：
// 常用字的命中多达数万首，每翻一页都重筛一遍很浪费。
func (r *Repository) inDynasty(ids []int64, dynastyID *int64, searchType, query string) ([]int64, error) {
	if dynastyID == nil || len(ids) == 0 {
		return ids, nil
	}
	cacheKey := newCountKey("in-dynasty", r.lang).addID(*dynastyID).add(searchType).add(query).String()
	if filtered, hit := r.db.idLists.get(cacheKey); hit {
		return filtered, nil
	}

	keep := make(map[int64]bool)
	for chunk := range slices.Chunk(ids, dynastyFilterChunk) {
		var matched []int64
		err := r.conn().Table(r.poemsTable()).
			Where("dynasty_id = ? AND id IN ?", *dynastyID, chunk).
			Pluck("id", &matched).Error
		if err != nil {
			return nil, err
		}
		for _, id := range matched {
			keep[id] = true
		}
	}
	filtered := make([]int64, 0, len(keep))
	for _, id := range ids {
		if keep[id] {
			filtered = append(filtered, id)
		}
	}

	r.db.idLists.set(cacheKey, filtered)
	return filtered, nil
}

// SearchPoemIDs 返回 "all" 模式下按相关度排序的前 limit 个命中 id，供语义检索与关键词结果融合。
func (r *Repository) SearchPoemIDs(query string, limit int) ([]int64, error) {
	ranked, err := r.rankedIDs(query, "all")
	if err != nil {
		return nil, err
	}
	return ranked[:min(limit, len(ranked))], nil
}

// rankedIDs 返回 searchType（"content" 以外）下按相关度排好的完整命中列表。
func (r *Repository) rankedIDs(query, searchType string) ([]int64, error) {
	// 1～2 字的查询用不上 trigram 索引，改走单字倒排索引；不适用时（如索引失效）
	// 继续走下面的原有路径，两者结果一致。
	if ids, ok, err := r.shortTermMatches(query, searchType); err != nil {
		return nil, err
	} else if ok {
		return r.rankedMatches(query, searchType, ids)
	}

	// 排好的列表已在缓存里时，不必再求一遍命中集合
	if ranked, hit := r.db.idLists.get(r.rankedKey(query, searchType)); hit {
		return ranked, nil
	}

	var ids []int64
	var err error
	switch searchType {
	case "title":
		ids, err = r.ftsColumnMatches("title", query)
	case "author":
		// 作者表很小，普通 LIKE 足够快
		ids, err = r.poemIDsByAuthorName(query)
	default:
		// "all" 不能一条 SQL 搞定：标题、正文与作者名之间的 OR 跨了 FTS 表与作者表，
		// SQLite 无法把它下推给 FTS5，整条查询退化为全表扫描
		// （实测 3 字查询 0.49s，而单搜正文仅 6ms）。拆成三条各自走索引的查询再合并。
		ids, err = r.allModeMatches(query)
	}
	if err != nil {
		return nil, err
	}
	return r.rankedMatches(query, searchType, ids)
}

// searchContent 是只搜正文的长查询路径：走 FTS trigram 索引，按 ID 升序分页。
func (r *Repository) searchContent(query string, dynastyID *int64, offset, pageSize int) ([]Poem, int64, error) {
	poemTable := r.poemsTable()
	ftsTable := r.poemsFtsTable()
	ftsJoin := "JOIN " + ftsTable + " ON " + ftsTable + ".rowid = " + poemTable + ".id"
	applyMatch := func(q *gorm.DB) *gorm.DB {
		cond, arg := substringMatch(ftsTable+".content_text", query)
		q = q.Joins(ftsJoin).Where(cond, arg)
		if dynastyID != nil {
			q = q.Where(poemTable+".dynasty_id = ?", *dynastyID)
		}
		return q
	}

	// 计数与页码无关，且要把同一个 FTS join 再跑一遍，代价与取数相当，故走缓存。
	// 键含用户输入的关键词，缓存的条目数上限见 countCacheMaxEntries。
	key := newCountKey("search", r.lang).add("content").add(query).addOptionalID(dynastyID).String()
	total, err := r.db.counts.getOrLoad(key, func() (int64, error) {
		var n int64
		err := applyMatch(r.conn().Table(poemTable)).Count(&n).Error
		return n, err
	})
	if err != nil {
		return nil, 0, err
	}

	var poems []Poem
	err = applyMatch(r.conn().Table(poemTable)).
		Select(poemTable + ".*").
		Order(poemTable + ".id").
		Limit(pageSize).Offset(offset).
		Find(&poems).Error
	if err != nil {
		return nil, 0, err
	}

	if err := r.loadPoemRelations(poems); err != nil {
		return nil, 0, err
	}
	return poems, total, nil
}

// globEscaper 把 GLOB 的元字符放进字符类，使其按字面匹配。
var globEscaper = strings.NewReplacer("*", "[*]", "?", "[?]", "[", "[[]")

// substringMatch 构造「column 包含 s」的条件及其参数，s 按字面匹配。
//
// 默认用 LIKE '%s%'，这是 FTS5 trigram 索引能加速的写法。但 s 里的 % 与 _
// 会被 LIKE 当成通配符（搜 "%" 等于匹配全部），而转义它们所需的 ESCAPE 子句
// 恰恰会让 SQLite 不再把条件下推给 FTS5，查询退化为全表扫描。
// 因此只在 s 含有这两个字符时改用 GLOB：% 与 _ 在 GLOB 里本就是普通字符，
// trigram 同样能用 GLOB 走索引。GLOB 区分 ASCII 大小写，而 LIKE 不区分，
// 这点差别只影响那些本来就带着 % 或 _ 的查询，可以接受。
func substringMatch(column, s string) (cond string, arg string) {
	if !strings.ContainsAny(s, "%_") {
		return column + " LIKE ?", "%" + s + "%"
	}
	return column + " GLOB ?", "*" + globEscaper.Replace(s) + "*"
}
