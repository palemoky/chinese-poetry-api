package database

import (
	"encoding/binary"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"
	"unicode"

	"gorm.io/gorm"
)

// 本文件实现 1～2 字短查询使用的单字倒排索引。
//
// 为什么需要它：FTS5 trigram 只能加速至少含 3 个字符的 LIKE，更短的模式会退化为
// 扫描整张 FTS 表，再用临时 B 树按 id 排序。在 37 万首的语料上，单字搜索哪怕
// 只取第 1 页也要约 0.16s，而且每翻一页、每次「随机一首含某字的诗」都要再扫一遍。
// 中文里单字、双字恰恰是最常见的查询（飞花令按单字出题）。
//
// 存储形态：每个字一行，记录它出现在哪些诗的标题、正文中。id 列表按升序做差值
// 编码后存成 uvarint 序列，37 万首的语料每个语言变体约 24MB；若每个（字, 诗）
// 存一行，同样的数据要约 300MB。
//
// 正确性不依赖索引是否最新：poems 表上的触发器在任何写入后清掉 char_index_state
// 中对应的行，查询见到索引无效就退回原有的 LIKE/GLOB 路径，结果与建索引前一致。
// 索引由 BuildCharIndex 全量重建：导入结束后由 processor 调用，老库则在 Migrate 中补建。

const (
	// charIndexStateTable 记录各语言变体的索引是否有效（有对应行且版本一致才有效）。
	charIndexStateTable = "char_index_state"

	// charIndexVersion 是索引内容的版本。修改 indexableRune 等影响索引内容的逻辑时
	// 必须递增，已有数据库的旧索引会因版本不符被视为无效并在下次迁移时重建。
	charIndexVersion = 1

	// charIndexVerifyChunk 是校验双字候选时每条 SQL 携带的 id 数，
	// 远低于 SQLite 的参数个数上限。
	charIndexVerifyChunk = 500
)

// charIndexTable 返回指定语言变体的单字倒排索引表名。
func charIndexTable(lang Lang) string {
	return "poem_char_index_" + langSuffix(lang)
}

// langSuffix 返回表名中的语言后缀，与 poemsTable 等保持一致。
func langSuffix(lang Lang) string {
	if lang == LangHant {
		return "zh_hant"
	}
	return "zh_hans"
}

// charField 标记一个字出现在标题还是正文中。
type charField uint8

const (
	fieldTitle charField = 1 << iota
	fieldContent
)

// indexableRune 判断一个字符是否收进倒排索引。
//
// 只收非 ASCII 的「文字」：标点、空白、控制字符不收，查询里带上它们就走原有路径。
// ASCII 也不收，因为 LIKE 对 ASCII 字母不区分大小写，按码点精确匹配的索引
// 给不出同样的结果；诗词正文里 ASCII 字符极少，不值得为此专门处理大小写。
func indexableRune(r rune) bool {
	if r < 0x80 {
		return false
	}
	return !unicode.In(r, unicode.P, unicode.Z, unicode.C)
}

// migrateCharIndex 创建倒排索引所需的表与失效触发器；索引无效时就地重建。
// 依赖 poems 表与 FTS 表，必须在两者之后执行。
func (db *DB) migrateCharIndex(lang Lang) error {
	stmts := []string{
		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
			lang TEXT PRIMARY KEY,
			version INTEGER NOT NULL
		)`, charIndexStateTable),

		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
			cp INTEGER PRIMARY KEY,
			title BLOB NOT NULL,
			content BLOB NOT NULL
		)`, charIndexTable(lang)),
	}

	// 只要标题或正文可能变化就让索引失效。失效只是删掉一行状态，
	// 导入时每插入一首诗多付这一点代价，换来的是索引永远不会给出过期的结果。
	poemTable := poemsTable(lang)
	invalidate := fmt.Sprintf("DELETE FROM %s WHERE lang = '%s';", charIndexStateTable, lang)
	stmts = append(stmts,
		fmt.Sprintf(`CREATE TRIGGER IF NOT EXISTS %[1]s_charidx_ai AFTER INSERT ON %[1]s BEGIN %[2]s END`, poemTable, invalidate),
		fmt.Sprintf(`CREATE TRIGGER IF NOT EXISTS %[1]s_charidx_ad AFTER DELETE ON %[1]s BEGIN %[2]s END`, poemTable, invalidate),
		fmt.Sprintf(`CREATE TRIGGER IF NOT EXISTS %[1]s_charidx_au AFTER UPDATE OF title, content ON %[1]s BEGIN %[2]s END`, poemTable, invalidate),
	)

	for _, stmt := range stmts {
		if err := db.Exec(stmt).Error; err != nil {
			return fmt.Errorf("failed to migrate char index for %s: %w", lang, err)
		}
	}

	ready, err := charIndexReady(db.DB, lang)
	if err != nil || ready {
		return err
	}
	return db.BuildCharIndex(lang)
}

// charIndexReady 判断指定语言变体的倒排索引当前是否有效。
func charIndexReady(tx *gorm.DB, lang Lang) (bool, error) {
	var versions []int
	err := tx.Raw(
		fmt.Sprintf("SELECT version FROM %s WHERE lang = ?", charIndexStateTable), string(lang),
	).Scan(&versions).Error
	if err != nil {
		return false, err
	}
	return len(versions) == 1 && versions[0] == charIndexVersion, nil
}

// postingWriter 以差值 uvarint 编码追加一个升序的 id 列表。
type postingWriter struct {
	last int64
	buf  []byte
}

func (w *postingWriter) add(id int64) {
	w.buf = binary.AppendUvarint(w.buf, uint64(id-w.last))
	w.last = id
}

// decodePostings 解码 postingWriter 写出的 id 列表。
func decodePostings(b []byte) ([]int64, error) {
	ids := make([]int64, 0, len(b)/2)
	var last int64
	for len(b) > 0 {
		delta, n := binary.Uvarint(b)
		if n <= 0 {
			return nil, errors.New("corrupt char index posting list")
		}
		last += int64(delta)
		ids = append(ids, last)
		b = b[n:]
	}
	return ids, nil
}

// BuildCharIndex 依据 FTS 表全量重建指定语言变体的单字倒排索引。
//
// 读的是 FTS 表而不是 poems 表：content_text 已是拼接好的正文，
// 与短查询退回 LIKE 时匹配的文本完全相同，两条路径才能给出一致的结果。
// 重建在一个事务内完成，过程中的查询看到的要么是旧状态，要么是新索引。
func (db *DB) BuildCharIndex(lang Lang) error {
	ftsTable := poemsFtsTable(lang)
	indexTable := charIndexTable(lang)

	err := db.Transaction(func(tx *gorm.DB) error {
		type postings struct{ title, content postingWriter }
		index := make(map[rune]*postings)

		rows, err := tx.Raw(fmt.Sprintf("SELECT rowid, title, content_text FROM %s ORDER BY rowid", ftsTable)).Rows()
		if err != nil {
			return err
		}

		seen := make(map[rune]charField)
		for rows.Next() {
			var id int64
			var title, content string
			if err := rows.Scan(&id, &title, &content); err != nil {
				_ = rows.Close()
				return err
			}

			clear(seen)
			for _, r := range title {
				if indexableRune(r) {
					seen[r] |= fieldTitle
				}
			}
			for _, r := range content {
				if indexableRune(r) {
					seen[r] |= fieldContent
				}
			}

			// rowid 升序遍历，每个字的 id 列表因此天然有序，可以直接差值编码
			for r, fields := range seen {
				p := index[r]
				if p == nil {
					p = &postings{}
					index[r] = p
				}
				if fields&fieldTitle != 0 {
					p.title.add(id)
				}
				if fields&fieldContent != 0 {
					p.content.add(id)
				}
			}
		}
		if err := rows.Close(); err != nil {
			return err
		}
		if err := rows.Err(); err != nil {
			return err
		}

		if err := tx.Exec("DELETE FROM " + indexTable).Error; err != nil {
			return err
		}
		insert := fmt.Sprintf("INSERT INTO %s (cp, title, content) VALUES (?, ?, ?)", indexTable)
		for r, p := range index {
			// 空列表存空 BLOB 而不是 NULL，免得读取时还要区分
			if err := tx.Exec(insert, int64(r), append([]byte{}, p.title.buf...), append([]byte{}, p.content.buf...)).Error; err != nil {
				return err
			}
		}

		return tx.Exec(
			fmt.Sprintf("INSERT OR REPLACE INTO %s (lang, version) VALUES (?, ?)", charIndexStateTable),
			string(lang), charIndexVersion,
		).Error
	})
	if err != nil {
		return fmt.Errorf("failed to build char index for %s: %w", lang, err)
	}

	db.invalidateCaches()
	return nil
}

// BuildCharIndexes 为简繁两个语言变体重建倒排索引，供导入结束后调用。
func (db *DB) BuildCharIndexes() error {
	for _, lang := range []Lang{LangHans, LangHant} {
		if err := db.BuildCharIndex(lang); err != nil {
			return err
		}
	}
	return nil
}

// charPostings 读取一个字在指定字段中的 id 列表（升序）。字不在索引中时返回空列表。
func (r *Repository) charPostings(ch rune, fields charField) ([]int64, error) {
	var rows []struct {
		Title   []byte
		Content []byte
	}
	err := r.db.Raw(
		fmt.Sprintf("SELECT title, content FROM %s WHERE cp = ?", charIndexTable(r.lang)), int64(ch),
	).Scan(&rows).Error
	if err != nil || len(rows) == 0 {
		return nil, err
	}

	var title, content []int64
	if fields&fieldTitle != 0 {
		if title, err = decodePostings(rows[0].Title); err != nil {
			return nil, err
		}
	}
	if fields&fieldContent != 0 {
		if content, err = decodePostings(rows[0].Content); err != nil {
			return nil, err
		}
	}
	return unionSorted(title, content), nil
}

// shortTermMatches 用倒排索引求出与 SearchPoems 语义一致的完整命中 id 列表（升序）。
// ok 为 false 表示此查询不适用索引（长度不是 1～2、含未收录的字符、索引无效等），
// 调用方应退回原有路径。
//
// searchType 取值与 SearchPoems 相同；"author" 只查作者表，本就很快，不走这里。
func (r *Repository) shortTermMatches(query, searchType string) (ids []int64, ok bool, err error) {
	var fields charField
	switch searchType {
	case "title":
		fields = fieldTitle
	case "content":
		fields = fieldContent
	case "all":
		fields = fieldTitle | fieldContent
	default:
		return nil, false, nil
	}

	runes := []rune(query)
	if len(runes) < 1 || len(runes) > 2 {
		return nil, false, nil
	}
	for _, ch := range runes {
		if !indexableRune(ch) {
			return nil, false, nil
		}
	}

	if ready, err := charIndexReady(r.db.DB, r.lang); err != nil || !ready {
		return nil, false, err
	}

	key := newCountKey("shortterm", r.lang).add(searchType).add(query).String()
	if ids, hit := r.db.idLists.get(key); hit {
		return ids, true, nil
	}

	ids, err = r.charPostings(runes[0], fields)
	if err != nil {
		return nil, false, err
	}

	// 单字：索引本身就是精确结果。双字：两个字都出现只是必要条件，
	// 还要回到 FTS 行上确认它们相邻，求交集先把候选压到足够小。
	if len(runes) == 2 {
		second, err := r.charPostings(runes[1], fields)
		if err != nil {
			return nil, false, err
		}
		if ids, err = r.verifySubstring(intersectSorted(ids, second), query, fields); err != nil {
			return nil, false, err
		}
	}

	if searchType == "all" {
		authorIDs, err := r.poemIDsByAuthorName(query)
		if err != nil {
			return nil, false, err
		}
		ids = unionSorted(ids, authorIDs)
	}

	r.db.idLists.set(key, ids)
	return ids, true, nil
}

// verifySubstring 从候选 id 中筛出标题或正文（按 fields）确实包含 query 的那些，结果升序。
func (r *Repository) verifySubstring(candidates []int64, query string, fields charField) ([]int64, error) {
	ftsTable := r.poemsFtsTable()
	titleCond, arg := substringMatch(ftsTable+".title", query)
	contentCond, _ := substringMatch(ftsTable+".content_text", query)

	var cond string
	var args []any
	switch fields {
	case fieldTitle:
		cond, args = titleCond, []any{arg}
	case fieldContent:
		cond, args = contentCond, []any{arg}
	default:
		cond, args = "("+titleCond+" OR "+contentCond+")", []any{arg, arg}
	}

	matched := make([]int64, 0, len(candidates)/4)
	for chunk := range slices.Chunk(candidates, charIndexVerifyChunk) {
		var ids []int64
		err := r.db.Table(ftsTable).
			Select("rowid").
			Where("rowid IN ?", chunk).
			Where(cond, args...).
			Pluck("rowid", &ids).Error
		if err != nil {
			return nil, err
		}
		matched = append(matched, ids...)
	}

	slices.Sort(matched)
	return matched, nil
}

// poemIDsByAuthorName 返回作者名包含 query 的全部诗词 id（升序），
// 对应 SearchPoems 在 "all" 模式下对作者名的匹配。
//
// 写成 author_id IN (子查询) 而不是 JOIN：JOIN 时 SQLite 选择扫描整张 poems 表
// 再逐行回查作者（实测 40ms），IN 则先在小小的作者表里筛出 id，再走 author_id 索引（1ms）。
func (r *Repository) poemIDsByAuthorName(query string) ([]int64, error) {
	cond, arg := substringMatch("name", query)
	authors := r.db.Table(r.authorsTable()).Select("id").Where(cond, arg)

	var ids []int64
	err := r.db.Table(r.poemsTable()).
		Where("author_id IN (?)", authors).
		Order("id").
		Pluck("id", &ids).Error
	return ids, err
}

// allModeMatches 求出 "all" 模式（标题、正文或作者名包含 query）的完整命中 id 列表，
// 三部分各自走索引后合并，结果进 idLists 缓存。
func (r *Repository) allModeMatches(query string) ([]int64, error) {
	key := newCountKey("all", r.lang).add(query).String()
	if ids, hit := r.db.idLists.get(key); hit {
		return ids, nil
	}

	ftsTable := r.poemsFtsTable()
	var ids []int64
	for _, column := range []string{"title", "content_text"} {
		cond, arg := substringMatch(column, query)
		var matched []int64
		if err := r.db.Table(ftsTable).Where(cond, arg).Order("rowid").Pluck("rowid", &matched).Error; err != nil {
			return nil, err
		}
		ids = unionSorted(ids, matched)
	}

	authorIDs, err := r.poemIDsByAuthorName(query)
	if err != nil {
		return nil, err
	}
	ids = unionSorted(ids, authorIDs)

	r.db.idLists.set(key, ids)
	return ids, nil
}

// pageOfIDs 取升序 id 列表中的一页诗词，并以列表长度作为总数。
func (r *Repository) pageOfIDs(ids []int64, offset, pageSize int) ([]Poem, int64, error) {
	start := min(offset, len(ids))
	end := min(offset+pageSize, len(ids))
	poems, err := r.poemsByIDs(ids[start:end])
	if err != nil {
		return nil, 0, err
	}
	return poems, int64(len(ids)), nil
}

// poemsByIDs 按 id 升序取出一批诗词并补齐关联数据。
func (r *Repository) poemsByIDs(ids []int64) ([]Poem, error) {
	poems := []Poem{}
	if len(ids) == 0 {
		return poems, nil
	}
	err := r.db.Table(r.poemsTable()).Where("id IN ?", ids).Order("id ASC").Find(&poems).Error
	if err != nil {
		return nil, err
	}
	if err := r.loadPoemRelations(poems); err != nil {
		return nil, err
	}
	return poems, nil
}

// intersectSorted 求两个升序列表的交集。
func intersectSorted(a, b []int64) []int64 {
	out := make([]int64, 0, min(len(a), len(b)))
	for i, j := 0, 0; i < len(a) && j < len(b); {
		switch {
		case a[i] < b[j]:
			i++
		case a[i] > b[j]:
			j++
		default:
			out = append(out, a[i])
			i++
			j++
		}
	}
	return out
}

// unionSorted 求两个升序列表的并集（去重）。
func unionSorted(a, b []int64) []int64 {
	if len(a) == 0 {
		return b
	}
	if len(b) == 0 {
		return a
	}
	out := make([]int64, 0, len(a)+len(b))
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		switch {
		case a[i] < b[j]:
			out = append(out, a[i])
			i++
		case a[i] > b[j]:
			out = append(out, b[j])
			j++
		default:
			out = append(out, a[i])
			i++
			j++
		}
	}
	out = append(out, a[i:]...)
	return append(out, b[j:]...)
}

// idListCache 缓存短查询的完整命中 id 列表，翻页时不必重新解码与校验。
//
// 与 countCache 一样用朴素的 TTL，超出上限时整体清空。上限按缓存的 id 总数计，
// 而不是条目数：一个常用字的列表就有数万个 id，按条目数限制挡不住内存增长。
type idListCache struct {
	mu      sync.Mutex
	entries map[string]idListEntry
	size    int
}

type idListEntry struct {
	ids       []int64
	expiresAt time.Time
}

const (
	idListCacheTTL = 5 * time.Minute

	// idListCacheMaxIDs 约合 32MB。最常用的单字也只有约 7 万个 id，足够容纳数十个热门查询。
	idListCacheMaxIDs = 4 << 20
)

func (c *idListCache) get(key string) ([]int64, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.entries[key]
	if !ok || time.Now().After(entry.expiresAt) {
		return nil, false
	}
	return entry.ids, true
}

func (c *idListCache) set(key string, ids []int64) {
	if len(ids) > idListCacheMaxIDs {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.entries == nil || c.size+len(ids) > idListCacheMaxIDs {
		c.entries = make(map[string]idListEntry)
		c.size = 0
	}
	if old, ok := c.entries[key]; ok {
		c.size -= len(old.ids)
	}
	c.entries[key] = idListEntry{ids: ids, expiresAt: time.Now().Add(idListCacheTTL)}
	c.size += len(ids)
}

func (c *idListCache) invalidate() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = nil
	c.size = 0
}
