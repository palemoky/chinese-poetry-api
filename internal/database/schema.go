package database

const (
	// Schema version for migrations
	SchemaVersion = 1
)

// CreateFTSTableSQL contains the FTS5 virtual table creation
// GORM doesn't support virtual tables, so we create it manually
var CreateFTSTableSQL = `CREATE VIRTUAL TABLE IF NOT EXISTS poems_fts USING fts5(
	poem_id UNINDEXED,
	title,
	title_pinyin,
	content,
	author_name,
	author_pinyin,
	content='',
	tokenize='unicode61'
)`

// InitialDynastiesSQL contains initial data for dynasties
var InitialDynastiesSQL = `INSERT OR IGNORE INTO dynasties (name, name_en, start_year, end_year) VALUES
	('唐', 'Tang', 618, 907),
	('宋', 'Song', 960, 1279),
	('元', 'Yuan', 1271, 1368),
	('五代', 'Five Dynasties', 907, 960),
	('先秦', 'Pre-Qin', -2070, -221),
	('两汉', 'Han', -206, 220),
	('魏晋', 'Wei-Jin', 220, 420),
	('南北朝', 'Northern and Southern', 420, 589),
	('隋', 'Sui', 581, 618),
	('清', 'Qing', 1644, 1912),
	('其他', 'Other', NULL, NULL)`

// InitialPoetryTypesSQL contains initial data for poetry types
var InitialPoetryTypesSQL = `INSERT OR IGNORE INTO poetry_types (name, category, lines, chars_per_line, description) VALUES
	('五言绝句', '诗', 4, 5, '四句，每句五字'),
	('七言绝句', '诗', 4, 7, '四句，每句七字'),
	('五言律诗', '诗', 8, 5, '八句，每句五字'),
	('七言律诗', '诗', 8, 7, '八句，每句七字'),
	('五言古诗', '诗', NULL, 5, '不限句数，每句五字'),
	('七言古诗', '诗', NULL, 7, '不限句数，每句七字'),
	('词', '词', NULL, NULL, '长短句'),
	('曲', '曲', NULL, NULL, '散曲'),
	('其他', '其他', NULL, NULL, '不规则或其他形式')`

// TriggersSQL contains triggers for FTS synchronization
var TriggersSQL = []string{
	// Trigger to update FTS when inserting poems
	`CREATE TRIGGER IF NOT EXISTS poems_ai AFTER INSERT ON poems BEGIN
		INSERT INTO poems_fts(poem_id, title, title_pinyin, content, author_name, author_pinyin)
		SELECT
			NEW.id,
			NEW.title,
			NEW.title_pinyin,
			NEW.content,
			a.name,
			a.name_pinyin
		FROM authors a WHERE a.id = NEW.author_id;
	END`,

	// Trigger to update FTS when updating poems
	`CREATE TRIGGER IF NOT EXISTS poems_au AFTER UPDATE ON poems BEGIN
		DELETE FROM poems_fts WHERE poem_id = OLD.id;
		INSERT INTO poems_fts(poem_id, title, title_pinyin, content, author_name, author_pinyin)
		SELECT
			NEW.id,
			NEW.title,
			NEW.title_pinyin,
			NEW.content,
			a.name,
			a.name_pinyin
		FROM authors a WHERE a.id = NEW.author_id;
	END`,

	// Trigger to update FTS when deleting poems
	`CREATE TRIGGER IF NOT EXISTS poems_ad AFTER DELETE ON poems BEGIN
		DELETE FROM poems_fts WHERE poem_id = OLD.id;
	END`,
}
