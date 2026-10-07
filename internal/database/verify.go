package database

import (
	"fmt"
)

// Check 是一项发布前检查的结果。
type Check struct {
	Name   string
	Passed bool
	Detail string
}

// DynastyCount 是某个朝代的作品数，供人工核对朝代分布。
type DynastyCount struct {
	Name  string
	Count int64
}

// VerifyReport 汇总发布前检查的结果。
type VerifyReport struct {
	Checks    []Check
	Dynasties []DynastyCount
}

// Passed 表示全部检查都通过。
func (r *VerifyReport) Passed() bool {
	for _, c := range r.Checks {
		if !c.Passed {
			return false
		}
	}
	return true
}

// VerifyRelease 检查导入完成的数据库能否发布，minPoems 是诗词总数的下限。
//
// 这些都是曾经出过错、肉眼又很难从统计数字上看出来的地方：
// 简繁两表的诗词与作者 ID 必须一一对应（API 用同一个 ID 在两表间切换），
// 关联不能缺失，FTS、倒排索引与物化计数必须与诗词表一致。
// 朝代分布无法自动判定对错，随报告一并列出供人工核对。
func (db *DB) VerifyRelease(minPoems int64) (*VerifyReport, error) {
	report := &VerifyReport{}
	add := func(name string, passed bool, format string, args ...any) {
		report.Checks = append(report.Checks, Check{Name: name, Passed: passed, Detail: fmt.Sprintf(format, args...)})
	}
	count := func(query string, args ...any) (int64, error) {
		var n int64
		err := db.Raw(query, args...).Scan(&n).Error
		return n, err
	}

	hans, hant := PoemsTable(LangHans), PoemsTable(LangHant)
	hansAuthors, hantAuthors := AuthorsTable(LangHans), AuthorsTable(LangHant)

	// 诗词数
	hansCount, err := count("SELECT COUNT(*) FROM " + hans)
	if err != nil {
		return nil, err
	}
	hantCount, err := count("SELECT COUNT(*) FROM " + hant)
	if err != nil {
		return nil, err
	}
	add("poem count", hansCount == hantCount && hansCount >= minPoems,
		"zh-Hans %d, zh-Hant %d, minimum %d", hansCount, hantCount, minPoems)

	// 诗词 ID 与其作者、朝代、体裁在两表间一致
	onlyOne, err := count(fmt.Sprintf(`SELECT
		(SELECT COUNT(*) FROM %[1]s h WHERE NOT EXISTS (SELECT 1 FROM %[2]s t WHERE t.id = h.id)) +
		(SELECT COUNT(*) FROM %[2]s t WHERE NOT EXISTS (SELECT 1 FROM %[1]s h WHERE h.id = t.id))`, hans, hant))
	if err != nil {
		return nil, err
	}
	add("poem ids aligned", onlyOne == 0, "%d poems exist in only one variant", onlyOne)

	misaligned, err := count(fmt.Sprintf(`SELECT COUNT(*) FROM %s h JOIN %s t ON t.id = h.id
		WHERE h.author_id IS NOT t.author_id OR h.dynasty_id IS NOT t.dynasty_id OR h.type_id IS NOT t.type_id`, hans, hant))
	if err != nil {
		return nil, err
	}
	add("poem relations aligned", misaligned == 0, "%d poems differ in author, dynasty or type between variants", misaligned)

	authorsMisaligned, err := count(fmt.Sprintf(`SELECT
		(SELECT COUNT(*) FROM %[1]s h LEFT JOIN %[2]s t ON t.id = h.id WHERE t.id IS NULL OR h.dynasty_id IS NOT t.dynasty_id) +
		(SELECT COUNT(*) FROM %[2]s t WHERE NOT EXISTS (SELECT 1 FROM %[1]s h WHERE h.id = t.id))`, hansAuthors, hantAuthors))
	if err != nil {
		return nil, err
	}
	add("author ids aligned", authorsMisaligned == 0, "%d authors missing from a variant or in a different dynasty", authorsMisaligned)

	for _, lang := range []Lang{LangHans, LangHant} {
		poems, authors := PoemsTable(lang), AuthorsTable(lang)

		// 关联缺失或悬空
		dangling, err := count(fmt.Sprintf(`SELECT COUNT(*) FROM %s p WHERE
			p.author_id IS NULL OR p.dynasty_id IS NULL OR p.type_id IS NULL
			OR NOT EXISTS (SELECT 1 FROM %s a WHERE a.id = p.author_id)
			OR NOT EXISTS (SELECT 1 FROM %s d WHERE d.id = p.dynasty_id)
			OR NOT EXISTS (SELECT 1 FROM %s y WHERE y.id = p.type_id)`,
			poems, authors, DynastiesTable(lang), PoetryTypesTable(lang)))
		if err != nil {
			return nil, err
		}
		add(fmt.Sprintf("%s relations present", lang), dangling == 0,
			"%d poems with a missing or dangling author, dynasty or type", dangling)

		// FTS 与诗词表行数一致
		ftsCount, err := count("SELECT COUNT(*) FROM " + PoemsFtsTable(lang))
		if err != nil {
			return nil, err
		}
		poemCount, err := count("SELECT COUNT(*) FROM " + poems)
		if err != nil {
			return nil, err
		}
		add(fmt.Sprintf("%s full-text index", lang), ftsCount == poemCount, "%d indexed of %d poems", ftsCount, poemCount)

		// 物化的作品数与实际一致，且没有无作品的作者
		badCounts, err := count(fmt.Sprintf(`SELECT COUNT(*) FROM %[1]s a
			WHERE a.poem_count != (SELECT COUNT(*) FROM %[2]s p WHERE p.author_id = a.id) OR a.poem_count = 0`, authors, poems))
		if err != nil {
			return nil, err
		}
		add(fmt.Sprintf("%s author poem counts", lang), badCounts == 0,
			"%d authors with a stale poem_count or no poems", badCounts)

		ready, err := charIndexReady(db.DB, lang)
		if err != nil {
			return nil, err
		}
		add(fmt.Sprintf("%s char index", lang), ready, "ready: %t", ready)
	}

	add("import finalized", db.importFinalized(), "import-only indexes and columns removed: %t", db.importFinalized())

	if err := db.Raw(fmt.Sprintf(`SELECT d.name AS name, COUNT(*) AS count FROM %s p
		JOIN %s d ON d.id = p.dynasty_id GROUP BY d.id ORDER BY count DESC`, hans, DynastiesTable(LangHans))).
		Scan(&report.Dynasties).Error; err != nil {
		return nil, err
	}

	return report, nil
}
