package database

import (
	"fmt"
	"strings"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/palemoky/chinese-poetry-api/internal/classifier"
)

// DB 是对 gorm.DB 连接的封装。
type DB struct {
	*gorm.DB

	// counts 缓存分页查询的 COUNT 结果。放在 DB 而不是 Repository 上，
	// 是因为 Repository.WithLang 每次都会返回一个新实例（每个请求至少一次），
	// 挂在 Repository 上的缓存永远不会被命中。
	counts countCache
}

// Open 打开 SQLite 数据库连接。
// maxOpenConns：最大连接数，传 0 则取保守默认值 1。
// maxIdleConns：最大空闲连接数，传 0 则取默认值 1。
func Open(path string, maxOpenConns, maxIdleConns int) (*DB, error) {
	config := &gorm.Config{
		Logger:      logger.Default.LogMode(logger.Silent), // 调试时可改为 logger.Info
		NowFunc:     time.Now,
		PrepareStmt: true, // 预编译语句以提升性能
	}

	// 针对并发写入优化的 SQLite 连接串：
	// _busy_timeout=5000  数据库被锁时最多等待 5 秒
	// _journal_mode=WAL   使用 WAL 日志以提升并发能力
	// _synchronous=NORMAL 在安全性与性能之间取平衡
	// cache=shared        多个连接共享缓存
	// _cache_size=-64000  64MB 页缓存（负值单位为 KB，正值为页数）
	// _temp_store=MEMORY  临时表与临时索引放在内存中
	dsn := path + "?_foreign_keys=on&_journal_mode=WAL&_busy_timeout=5000&_synchronous=NORMAL&cache=shared&_cache_size=-64000&_temp_store=MEMORY"

	db, err := gorm.Open(sqlite.Open(dsn), config)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// 取出底层 sql.DB 以配置连接池
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get underlying sql.DB: %w", err)
	}

	// 默认只用 1 条连接，对数据导入这类写密集场景更安全；
	// 以读为主的 API 服务可以调大。
	if maxOpenConns <= 0 {
		maxOpenConns = 1
	}
	if maxIdleConns <= 0 {
		maxIdleConns = 1
	}

	sqlDB.SetMaxOpenConns(maxOpenConns)
	sqlDB.SetMaxIdleConns(maxIdleConns)
	sqlDB.SetConnMaxLifetime(5 * time.Minute)

	// 探活，确认连接可用
	if err := sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &DB{DB: db}, nil
}

// NewDBFromGorm 包装一个已有的 gorm.DB 连接，便于测试时注入自定义配置。
func NewDBFromGorm(db *gorm.DB) *DB {
	return &DB{DB: db}
}

// Migrate 为简体、繁体两套表创建全部表结构、索引与初始数据。
func (db *DB) Migrate() error {
	// 先建元数据表
	if err := db.Exec(`CREATE TABLE IF NOT EXISTS metadata (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`).Error; err != nil {
		return fmt.Errorf("failed to create metadata table: %w", err)
	}

	// 简繁两套表分别建表
	for _, lang := range []Lang{LangHans, LangHant} {
		if err := db.migrateTablesForLang(lang); err != nil {
			return fmt.Errorf("failed to migrate tables for %s: %w", lang, err)
		}

		// 写入该语言变体的初始数据
		if err := db.insertInitialDataForLang(lang); err != nil {
			return fmt.Errorf("failed to insert initial data for %s: %w", lang, err)
		}

	}

	// 更新 schema 版本号
	if err := db.Exec(
		`INSERT OR REPLACE INTO metadata (key, value, updated_at) VALUES (?, ?, ?)`,
		"schema_version",
		fmt.Sprintf("%d", SchemaVersion),
		time.Now(),
	).Error; err != nil {
		return fmt.Errorf("failed to update schema version: %w", err)
	}

	return nil
}

// migrateTablesForLang 为指定语言变体创建全部表与索引。
func (db *DB) migrateTablesForLang(lang Lang) error {
	dynastyTable := dynastiesTable(lang)
	authorTable := authorsTable(lang)
	poetryTypeTable := poetryTypesTable(lang)
	poemTable := poemsTable(lang)

	// 朝代表
	dynastySQL := fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL UNIQUE,
		name_en TEXT,
		start_year INTEGER,
		end_year INTEGER,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`, dynastyTable)
	if err := db.Exec(dynastySQL).Error; err != nil {
		return fmt.Errorf("failed to create %s: %w", dynastyTable, err)
	}

	// 作者表。
	//
	// poem_count 是物化出来的作品数：作者列表按作品数倒序分页，若在查询时用
	// LEFT JOIN poems + GROUP BY 现算，代价是一次全表聚合（30 万首语料上约 15~100ms），
	// 且与页码无关——每页都要重来一遍，也无法借助索引排序。
	// 存成列后配合 idx_..._poem_count 索引，排序与分页都能走索引。
	// 该列由 RefreshAuthorPoemCounts 在导入结束后重算，见其文档说明。
	authorSQL := fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL UNIQUE,
		dynasty_id INTEGER,
		description TEXT,
		poem_count INTEGER NOT NULL DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (dynasty_id) REFERENCES %s(id)
	)`, authorTable, dynastyTable)
	if err := db.Exec(authorSQL).Error; err != nil {
		return fmt.Errorf("failed to create %s: %w", authorTable, err)
	}
	// dynasty_id 索引
	db.Exec(fmt.Sprintf("CREATE INDEX IF NOT EXISTS idx_%s_dynasty ON %s(dynasty_id)", authorTable, authorTable))

	// 早于 poem_count 的库里没有这一列，补上。CREATE TABLE IF NOT EXISTS 对已存在的表
	// 是空操作，不会带来新列，因此必须显式 ALTER。
	if err := db.addAuthorPoemCountColumn(authorTable); err != nil {
		return err
	}

	// 作者列表的排序键。SQLite 支持带 DESC 的索引列，排序方向与查询一致才能免去 B 树倒序扫描；
	// 带上 id 是为了在作品数相同时有稳定的 tiebreaker，否则分页会重复和遗漏。
	db.Exec(fmt.Sprintf("CREATE INDEX IF NOT EXISTS idx_%s_poem_count ON %s(poem_count DESC, id ASC)",
		authorTable, authorTable))
	// 按朝代过滤的作者列表走这条
	db.Exec(fmt.Sprintf("CREATE INDEX IF NOT EXISTS idx_%s_dynasty_poem_count ON %s(dynasty_id, poem_count DESC, id ASC)",
		authorTable, authorTable))

	// 诗词体裁表
	poetryTypeSQL := fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
		id INTEGER PRIMARY KEY,
		name TEXT NOT NULL UNIQUE,
		category TEXT NOT NULL,
		lines INTEGER,
		chars_per_line INTEGER,
		description TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`, poetryTypeTable)
	if err := db.Exec(poetryTypeSQL).Error; err != nil {
		return fmt.Errorf("failed to create %s: %w", poetryTypeTable, err)
	}

	// 诗词表
	poemSQL := fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
		id INTEGER PRIMARY KEY,
		type_id INTEGER,
		title TEXT NOT NULL,
		content TEXT NOT NULL,
		content_hash TEXT,
		author_id INTEGER,
		dynasty_id INTEGER,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (type_id) REFERENCES %s(id),
		FOREIGN KEY (author_id) REFERENCES %s(id),
		FOREIGN KEY (dynasty_id) REFERENCES %s(id)
	)`, poemTable, poetryTypeTable, authorTable, dynastyTable)
	if err := db.Exec(poemSQL).Error; err != nil {
		return fmt.Errorf("failed to create %s: %w", poemTable, err)
	}

	// 诗词表索引
	db.Exec(fmt.Sprintf("CREATE INDEX IF NOT EXISTS idx_%s_type ON %s(type_id)", poemTable, poemTable))
	db.Exec(fmt.Sprintf("CREATE INDEX IF NOT EXISTS idx_%s_title ON %s(title)", poemTable, poemTable))
	db.Exec(fmt.Sprintf("CREATE INDEX IF NOT EXISTS idx_%s_author ON %s(author_id)", poemTable, poemTable))
	db.Exec(fmt.Sprintf("CREATE INDEX IF NOT EXISTS idx_%s_dynasty ON %s(dynasty_id)", poemTable, poemTable))
	db.Exec(fmt.Sprintf("CREATE UNIQUE INDEX IF NOT EXISTS idx_%s_unique ON %s(title, content_hash)", poemTable, poemTable))
	// 复合索引，用于多体裁随机取词（type_id IN ... 叠加 id 范围查找）
	db.Exec(fmt.Sprintf("CREATE INDEX IF NOT EXISTS idx_%s_type_id ON %s(type_id, id)", poemTable, poemTable))

	if err := db.migrateFtsForLang(lang); err != nil {
		return err
	}

	// 以下两项都依赖 poems 表，必须放在建表之后
	if err := db.migrateAuthorPoemCountTriggers(lang); err != nil {
		return err
	}

	if err := db.migratePoemCounter(lang); err != nil {
		return err
	}

	return nil
}

// countersTable 保存物化的行数计数器，每行一个计数。
const countersTable = "counters"

// poemCounterName 返回某个语言变体下诗词总数计数器的名字。
func poemCounterName(lang Lang) string {
	return poemsTable(lang)
}

// migratePoemCounter 创建诗词总数的计数器及其同步触发器，并回填当前值。
//
// SQLite 没有 O(1) 的行数：COUNT(*) 必须扫完一整棵索引，代价随表大小增长，
// 而每个分页请求都要靠它算出 total/total_pages——第 1 页也照收。
// 30 万首的语料上实测：索引在 page cache 中时约 1ms，冷缓存时约 79ms，
// 而读计数器恒为 0.08ms 量级。
//
// 与之相对，带过滤的计数（按朝代/作者/体裁）走的是索引区间扫描，
// 代价随命中数而非表大小增长（实测 0.2~7ms），不值得为它们各自再维护一个计数器。
//
// 同步同样交给触发器而非由调用方在写完后重算：计数器与 poems 表在同一个数据库里，
// 导入进程与 API 进程各跑各的，只有把维护放进数据库本身，两边看到的才始终一致。
func (db *DB) migratePoemCounter(lang Lang) error {
	poemTable := poemsTable(lang)
	counter := poemCounterName(lang)

	createSQL := fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
		name TEXT PRIMARY KEY,
		value INTEGER NOT NULL DEFAULT 0
	)`, countersTable)
	if err := db.Exec(createSQL).Error; err != nil {
		return fmt.Errorf("failed to create %s: %w", countersTable, err)
	}

	triggers := []string{
		fmt.Sprintf(`CREATE TRIGGER IF NOT EXISTS %[1]s_counter_ai AFTER INSERT ON %[1]s BEGIN
			UPDATE %[2]s SET value = value + 1 WHERE name = %[3]q;
		END`, poemTable, countersTable, counter),

		fmt.Sprintf(`CREATE TRIGGER IF NOT EXISTS %[1]s_counter_ad AFTER DELETE ON %[1]s BEGIN
			UPDATE %[2]s SET value = value - 1 WHERE name = %[3]q;
		END`, poemTable, countersTable, counter),
	}

	for _, trigger := range triggers {
		if err := db.Exec(trigger).Error; err != nil {
			return fmt.Errorf("failed to create counter trigger on %s: %w", poemTable, err)
		}
	}

	return db.RefreshPoemCounter(lang)
}

// RefreshPoemCounter 依据 poems 表重算诗词总数计数器。
//
// 日常同步由 migratePoemCounter 建立的触发器负责，这里是全量重算：
// 供迁移时回填使用——此前的写入没有触发器可依。
func (db *DB) RefreshPoemCounter(lang Lang) error {
	err := db.Exec(fmt.Sprintf(
		`INSERT OR REPLACE INTO %s (name, value) VALUES (?, (SELECT COUNT(*) FROM %s))`,
		countersTable, poemsTable(lang),
	), poemCounterName(lang)).Error
	if err != nil {
		return fmt.Errorf("failed to refresh poem counter for %s: %w", lang, err)
	}

	db.counts.invalidate()
	return nil
}

// addAuthorPoemCountColumn 为早于该字段的数据库补上 authors.poem_count 列。
//
// 列补上时取值全为默认的 0，需要由 RefreshAuthorPoemCounts 回填；
// Migrate 会在建表完成后统一调用一次，因此这里只管加列。
func (db *DB) addAuthorPoemCountColumn(authorTable string) error {
	var columnCount int64
	if err := db.Raw(fmt.Sprintf(
		`SELECT count(*) FROM pragma_table_info(%q) WHERE name = 'poem_count'`, authorTable,
	)).Scan(&columnCount).Error; err != nil {
		return fmt.Errorf("failed to inspect columns of %s: %w", authorTable, err)
	}
	if columnCount > 0 {
		return nil
	}

	if err := db.Exec(fmt.Sprintf(
		"ALTER TABLE %s ADD COLUMN poem_count INTEGER NOT NULL DEFAULT 0", authorTable,
	)).Error; err != nil {
		return fmt.Errorf("failed to add poem_count to %s: %w", authorTable, err)
	}
	return nil
}

// migrateAuthorPoemCountTriggers 创建让 authors.poem_count 与 poems 保持同步的触发器，
// 并回填一次已有数据。
//
// 与 FTS 索引一样，同步交给触发器而不是由调用方记得在写完后重算：物化值一旦可能过期，
// 每个写入路径（含测试与将来新增的导入方式）都得背上这个负担，漏一处就是静默错误的数据。
// 增量维护的代价是每次 poems 写入多一条走主键的 UPDATE，相比同一批触发器里
// FTS trigram 索引的写入可以忽略。
func (db *DB) migrateAuthorPoemCountTriggers(lang Lang) error {
	authorTable := authorsTable(lang)
	poemTable := poemsTable(lang)

	// author_id 可为 NULL（佚名等），此时 WHERE id = NULL 匹配不到任何行，
	// 正是想要的行为：无归属的诗词不计入任何作者。
	triggers := []string{
		fmt.Sprintf(`CREATE TRIGGER IF NOT EXISTS %[2]s_author_count_ai AFTER INSERT ON %[2]s BEGIN
			UPDATE %[1]s SET poem_count = poem_count + 1 WHERE id = new.author_id;
		END`, authorTable, poemTable),

		fmt.Sprintf(`CREATE TRIGGER IF NOT EXISTS %[2]s_author_count_ad AFTER DELETE ON %[2]s BEGIN
			UPDATE %[1]s SET poem_count = poem_count - 1 WHERE id = old.author_id;
		END`, authorTable, poemTable),

		// 仅在归属发生变化时才动计数，避免每次改标题都白白更新两行
		fmt.Sprintf(`CREATE TRIGGER IF NOT EXISTS %[2]s_author_count_au AFTER UPDATE ON %[2]s
		WHEN old.author_id IS NOT new.author_id BEGIN
			UPDATE %[1]s SET poem_count = poem_count - 1 WHERE id = old.author_id;
			UPDATE %[1]s SET poem_count = poem_count + 1 WHERE id = new.author_id;
		END`, authorTable, poemTable),
	}

	for _, trigger := range triggers {
		if err := db.Exec(trigger).Error; err != nil {
			return fmt.Errorf("failed to create poem_count trigger on %s: %w", poemTable, err)
		}
	}

	return db.RefreshAuthorPoemCounts(lang)
}

// RefreshAuthorPoemCounts 依据 poems 表重算 authors.poem_count。
//
// 日常同步由 migrateAuthorPoemCountTriggers 建立的触发器负责，这里是全量重算：
// 供迁移时回填使用——刚补上该列的旧库取值全为 0，且此前的写入没有触发器可依。
func (db *DB) RefreshAuthorPoemCounts(lang Lang) error {
	authorTable := authorsTable(lang)
	poemTable := poemsTable(lang)

	err := db.Exec(fmt.Sprintf(`UPDATE %[1]s SET poem_count = (
		SELECT COUNT(*) FROM %[2]s WHERE %[2]s.author_id = %[1]s.id
	)`, authorTable, poemTable)).Error
	if err != nil {
		return fmt.Errorf("failed to refresh poem_count on %s: %w", authorTable, err)
	}

	db.counts.invalidate()
	return nil
}

// migrateFtsForLang 创建支撑全文检索的 FTS5 虚拟表及其同步触发器，
// 若表是本次新建的，还会做一次数据回填。
//
// 这里用的是自包含（而非 external-content）的 FTS5 索引：content_text 是派生值，
// 由以 JSON 数组形式存放的 poems.content 拍平而来，并非 poems 表上真实存在的列，
// 而 FTS5 的 external-content 模式要求能从关联表的同名列读回取值。
// 复制一份被索引的文本会多占些磁盘空间，但换来索引实现的简单与正确。
// 下面的触发器负责在 poems 表每次 INSERT/UPDATE/DELETE（含 loader 使用的
// ON CONFLICT upsert）时同步索引。
//
// 分词器选用 trigram 而非默认的 unicode61：中文没有空格作词边界，
// 标准分词器无法有效切分。trigram 索引让 `col LIKE '%...%'` 这类任意子串查询
// 也能走 FTS 索引加速（含单字、双字中文词），同时完全保持 SearchPoems
// 对外暴露的子串匹配语义。
func (db *DB) migrateFtsForLang(lang Lang) error {
	poemTable := poemsTable(lang)
	ftsTable := poemsFtsTable(lang)

	// 判断该表是首次创建（需要一次性回填），还是已经存在（已由触发器增量同步，每次 Migrate 都重建纯属浪费）
	var existingCount int64
	if err := db.Raw(
		`SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, ftsTable,
	).Scan(&existingCount).Error; err != nil {
		return fmt.Errorf("failed to check for existing %s: %w", ftsTable, err)
	}

	ftsSQL := fmt.Sprintf(`CREATE VIRTUAL TABLE IF NOT EXISTS %s USING fts5(
		title,
		content_text,
		tokenize='trigram'
	)`, ftsTable)
	if err := db.Exec(ftsSQL).Error; err != nil {
		return fmt.Errorf(
			"failed to create %s (requires SQLite built with FTS5 support, e.g. the sqlite_fts5 build tag): %w",
			ftsTable, err,
		)
	}

	// content_text 是从 poems.content 这个 JSON 数组中取出各段落拼接而成的纯文本，
	// 这样索引（以及针对它的 LIKE 查询）匹配的是可读正文，而非 JSON 里的原始标点。
	const contentTextExpr = `(SELECT COALESCE(group_concat(value, ''), '') FROM json_each(%s.content))`

	insertTrigger := fmt.Sprintf(`CREATE TRIGGER IF NOT EXISTS %[2]s_fts_ai AFTER INSERT ON %[2]s BEGIN
		INSERT INTO %[1]s(rowid, title, content_text)
		VALUES (new.id, new.title, `+fmt.Sprintf(contentTextExpr, "new")+`);
	END`, ftsTable, poemTable)
	if err := db.Exec(insertTrigger).Error; err != nil {
		return fmt.Errorf("failed to create insert trigger for %s: %w", ftsTable, err)
	}

	// 注意：FTS5 中 "INSERT INTO fts(fts, rowid, ...) VALUES ('delete', ...)" 这种
	// 特殊删除语法只适用于 external-content 表；本表是自包含的（见上文说明），
	// 因此直接按 rowid 执行普通 DELETE 即可。
	deleteTrigger := fmt.Sprintf(`CREATE TRIGGER IF NOT EXISTS %[2]s_fts_ad AFTER DELETE ON %[2]s BEGIN
		DELETE FROM %[1]s WHERE rowid = old.id;
	END`, ftsTable, poemTable)
	if err := db.Exec(deleteTrigger).Error; err != nil {
		return fmt.Errorf("failed to create delete trigger for %s: %w", ftsTable, err)
	}

	updateTrigger := fmt.Sprintf(`CREATE TRIGGER IF NOT EXISTS %[2]s_fts_au AFTER UPDATE ON %[2]s BEGIN
		DELETE FROM %[1]s WHERE rowid = old.id;
		INSERT INTO %[1]s(rowid, title, content_text)
		VALUES (new.id, new.title, `+fmt.Sprintf(contentTextExpr, "new")+`);
	END`, ftsTable, poemTable)
	if err := db.Exec(updateTrigger).Error; err != nil {
		return fmt.Errorf("failed to create update trigger for %s: %w", ftsTable, err)
	}

	// 仅在首次创建时回填：已有数据早于触发器存在，需要补进索引；
	// 而此前已存在的表本身就是最新的，没必要在每次迁移时都做一遍全量重建。
	//
	// FTS5 内置的 'rebuild' 命令要求 fts5 表的列名与内容表的真实列一一对应，
	// 而 content_text 是派生表达式而非存储列，因此这里用触发器里相同的表达式手动回填。
	if existingCount == 0 {
		backfillSQL := fmt.Sprintf(`INSERT INTO %[1]s(rowid, title, content_text)
			SELECT id, title, `+fmt.Sprintf(contentTextExpr, poemTable)+`
			FROM %[2]s`, ftsTable, poemTable)
		if err := db.Exec(backfillSQL).Error; err != nil {
			return fmt.Errorf("failed to backfill %s: %w", ftsTable, err)
		}
	}

	return nil
}

// insertInitialDataForLang 为指定语言变体写入朝代、体裁等初始数据。
func (db *DB) insertInitialDataForLang(lang Lang) error {
	dynastyTable := dynastiesTable(lang)
	poetryTypeTable := poetryTypesTable(lang)

	// 把 SQL 中的表名替换为该变体的表名，繁体库还需做简转繁
	dynastiesSQL := strings.ReplaceAll(InitialDynastiesSQL, "dynasties", dynastyTable)
	poetryTypesSQL := strings.ReplaceAll(InitialPoetryTypesSQL, "poetry_types", poetryTypeTable)

	if lang == LangHant {
		var err error
		dynastiesSQL, err = convertSQLToTraditional(dynastiesSQL)
		if err != nil {
			return fmt.Errorf("failed to convert dynasties SQL: %w", err)
		}
		poetryTypesSQL, err = convertSQLToTraditional(poetryTypesSQL)
		if err != nil {
			return fmt.Errorf("failed to convert poetry types SQL: %w", err)
		}
	}

	// 写入朝代数据
	if err := db.Exec(dynastiesSQL).Error; err != nil {
		return fmt.Errorf("failed to insert dynasties: %w", err)
	}

	// 写入体裁数据
	if err := db.Exec(poetryTypesSQL).Error; err != nil {
		return fmt.Errorf("failed to insert poetry types: %w", err)
	}

	return nil
}

// convertSQLToTraditional 把 SQL 语句中的中文转为繁体，
// 只转换单引号内的字符串字面量，保证 SQL 语法本身不受影响。
func convertSQLToTraditional(sql string) (string, error) {
	// 以单引号切分，奇数下标的片段即字符串字面量内部
	parts := strings.Split(sql, "'")

	for i := range parts {
		if i%2 == 1 {
			converted, err := classifier.ToTraditional(parts[i])
			if err != nil {
				return "", err
			}
			parts[i] = converted
		}
	}

	return strings.Join(parts, "'"), nil
}

// GetSchemaVersion 返回当前的 schema 版本号，未记录时返回 0。
func (db *DB) GetSchemaVersion() (int, error) {
	var version int
	err := db.Raw(`SELECT value FROM metadata WHERE key = ?`, "schema_version").Scan(&version).Error
	if err == gorm.ErrRecordNotFound {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return version, nil
}

// Close 关闭数据库连接。
func (db *DB) Close() error {
	sqlDB, err := db.DB.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}
