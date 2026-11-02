package database

import (
	"cmp"
	"slices"
)

// 搜索结果的相关度分数。命中集合不变，只决定先后：标题与查询完全相同的
// 最可能是用户要找的那首，其次是标题包含查询、作者名、正文；收在选本里的名篇再加分，
// 足以越过“标题包含”，但越不过“标题完全相同”。
// 权重来自 eval/semantic 的关键词排序评测（那里是 4、2、2、1、1 与 1.5，这里整体乘 2），
// 同分时保持 ID 升序。
const (
	scoreTitleExact  = 8
	scoreTitlePart   = 4
	scoreAuthorExact = 4
	scoreAuthorPart  = 2
	scoreContent     = 2
	scoreAnthology   = 3
)

// titleSeparators 是标题中“词牌·首句”“宫调・曲牌”之类的分隔符。
// 搜词牌名、剧名时，分隔符前的部分与查询相同也算标题完全相同。
var titleSeparators = []string{"·", "・"}

// rankedMatches 把 searchType 下的完整命中列表 ids（升序）按相关度重排，结果进 idLists 缓存。
// "content" 模式下所有命中同分，调用方无需排序。
func (r *Repository) rankedMatches(query, searchType string, ids []int64) ([]int64, error) {
	if len(ids) < 2 {
		return ids, nil
	}

	key := r.rankedKey(query, searchType)
	if ranked, hit := r.db.idLists.get(key); hit {
		return ranked, nil
	}

	var title, content, author []int64
	var err error
	switch searchType {
	case "title":
		title = ids
	case "author":
		author = ids
	default:
		if title, err = r.fieldMatches(query, "title"); err != nil {
			return nil, err
		}
		if content, err = r.fieldMatches(query, "content"); err != nil {
			return nil, err
		}
		if author, err = r.poemIDsByAuthorName(query); err != nil {
			return nil, err
		}
	}

	exactTitle, err := r.exactTitleIDs(title, query)
	if err != nil {
		return nil, err
	}
	var exactAuthor []int64
	if len(author) > 0 {
		if exactAuthor, err = r.poemIDsByExactAuthorName(query); err != nil {
			return nil, err
		}
	}

	anthology, err := r.AnthologyIDs()
	if err != nil {
		return nil, err
	}

	ranked := rankByScore(ids, func(id int64) int {
		score := 0
		if sortedContains(anthology, id) {
			score += scoreAnthology
		}
		switch {
		case sortedContains(exactTitle, id):
			score += scoreTitleExact
		case sortedContains(title, id):
			score += scoreTitlePart
		}
		switch {
		case sortedContains(exactAuthor, id):
			score += scoreAuthorExact
		case sortedContains(author, id):
			score += scoreAuthorPart
		}
		if sortedContains(content, id) {
			score += scoreContent
		}
		return score
	})

	r.db.idLists.set(key, ranked)
	return ranked, nil
}

// rankedKey 是排好序的命中列表在 idLists 中的键。
func (r *Repository) rankedKey(query, searchType string) string {
	return newCountKey("ranked", r.lang).add(searchType).add(query).String()
}

// rankByScore 按分数降序重排升序的 ids，同分保持原顺序。
func rankByScore(ids []int64, score func(int64) int) []int64 {
	type scored struct {
		id    int64
		score int
	}
	items := make([]scored, len(ids))
	for i, id := range ids {
		items[i] = scored{id, score(id)}
	}
	slices.SortStableFunc(items, func(a, b scored) int { return cmp.Compare(b.score, a.score) })

	out := make([]int64, len(items))
	for i, item := range items {
		out[i] = item.id
	}
	return out
}

func sortedContains(ids []int64, id int64) bool {
	_, ok := slices.BinarySearch(ids, id)
	return ok
}

// fieldMatches 返回标题或正文（field 取 "title" 或 "content"）包含 query 的全部 id（升序）。
// 短查询走倒排索引，其余走 FTS trigram 索引，两者结果一致。
func (r *Repository) fieldMatches(query, field string) ([]int64, error) {
	if ids, ok, err := r.shortTermMatches(query, field); err != nil || ok {
		return ids, err
	}
	column := "title"
	if field == "content" {
		column = "content_text"
	}
	return r.ftsColumnMatches(column, query)
}

// ftsColumnMatches 返回 FTS 表中 column 包含 query 的全部 rowid（升序）。
func (r *Repository) ftsColumnMatches(column, query string) ([]int64, error) {
	cond, arg := substringMatch(column, query)
	var ids []int64
	err := r.conn().Table(r.poemsFtsTable()).Where(cond, arg).Order("rowid").Pluck("rowid", &ids).Error
	return ids, err
}

// exactTitleIDs 从 candidates（升序）中筛出标题与 query 完全相同，或分隔符前的部分与 query 相同的 id。
//
// 发布的数据库没有标题索引，所以只在已知标题包含 query 的候选里查，而不是扫描全表。
// 与 LIKE 一致，完全相同的比较不区分 ASCII 大小写。
func (r *Repository) exactTitleIDs(candidates []int64, query string) ([]int64, error) {
	if len(candidates) == 0 {
		return nil, nil
	}

	cond := "title = ? COLLATE NOCASE"
	args := []any{query}
	prefixLen := len([]rune(query)) + 1
	for _, sep := range titleSeparators {
		cond += " OR substr(title, 1, ?) = ?"
		args = append(args, prefixLen, query+sep)
	}

	var matched []int64
	for chunk := range slices.Chunk(candidates, charIndexVerifyChunk) {
		var ids []int64
		err := r.conn().Table(r.poemsFtsTable()).
			Where("rowid IN ?", chunk).
			Where("("+cond+")", args...).
			Pluck("rowid", &ids).Error
		if err != nil {
			return nil, err
		}
		matched = append(matched, ids...)
	}
	slices.Sort(matched)
	return matched, nil
}

// poemIDsByExactAuthorName 返回作者名与 query 相同（不区分 ASCII 大小写）的全部诗词 id（升序）。
func (r *Repository) poemIDsByExactAuthorName(query string) ([]int64, error) {
	authors := r.conn().Table(r.authorsTable()).Select("id").Where("name = ? COLLATE NOCASE", query)

	var ids []int64
	err := r.conn().Table(r.poemsTable()).
		Where("author_id IN (?)", authors).
		Order("id").
		Pluck("id", &ids).Error
	return ids, err
}
