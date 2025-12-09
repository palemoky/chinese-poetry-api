package database

const (
	// Schema version for migrations
	SchemaVersion = 1
)

// CreateTablesSQL contains all table creation statements
var CreateTablesSQL = []string{
	// Dynasties table
	`CREATE TABLE IF NOT EXISTS dynasties (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL UNIQUE,
		name_en TEXT,
		start_year INTEGER,
		end_year INTEGER,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`,

	// Authors table
	`CREATE TABLE IF NOT EXISTS authors (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		name_pinyin TEXT,
		name_pinyin_abbr TEXT,
		dynasty_id INTEGER,
		description TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (dynasty_id) REFERENCES dynasties(id)
	)`,

	// Poetry types table
	`CREATE TABLE IF NOT EXISTS poetry_types (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL UNIQUE,
		category TEXT NOT NULL,
		lines INTEGER,
		chars_per_line INTEGER,
		description TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`,

	// Poems table
	`CREATE TABLE IF NOT EXISTS poems (
		id TEXT PRIMARY KEY,
		title TEXT NOT NULL,
		title_pinyin TEXT,
		title_pinyin_abbr TEXT,
		author_id INTEGER,
		dynasty_id INTEGER,
		type_id INTEGER,
		content TEXT NOT NULL,
		rhythmic TEXT,
		rhythmic_pinyin TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (author_id) REFERENCES authors(id),
		FOREIGN KEY (dynasty_id) REFERENCES dynasties(id),
		FOREIGN KEY (type_id) REFERENCES poetry_types(id)
	)`,

	// Full-text search virtual table
	`CREATE VIRTUAL TABLE IF NOT EXISTS poems_fts USING fts5(
		poem_id UNINDEXED,
		title,
		title_pinyin,
		content,
		author_name,
		author_pinyin,
		content='',
		tokenize='unicode61'
	)`,

	// Metadata table for schema version
	`CREATE TABLE IF NOT EXISTS metadata (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`,
}

// CreateIndexesSQL contains all index creation statements
var CreateIndexesSQL = []string{
	`CREATE INDEX IF NOT EXISTS idx_poems_author ON poems(author_id)`,
	`CREATE INDEX IF NOT EXISTS idx_poems_dynasty ON poems(dynasty_id)`,
	`CREATE INDEX IF NOT EXISTS idx_poems_type ON poems(type_id)`,
	`CREATE INDEX IF NOT EXISTS idx_poems_title_pinyin ON poems(title_pinyin)`,
	`CREATE INDEX IF NOT EXISTS idx_authors_dynasty ON authors(dynasty_id)`,
	`CREATE INDEX IF NOT EXISTS idx_authors_name ON authors(name)`,
	`CREATE INDEX IF NOT EXISTS idx_authors_pinyin ON authors(name_pinyin)`,
}

// InitialDataSQL contains initial data for dynasties and poetry types
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

// TriggerSQL contains triggers for FTS synchronization
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
