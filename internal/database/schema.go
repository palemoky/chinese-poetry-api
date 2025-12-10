package database

const (
	// Schema version for migrations
	SchemaVersion = 1
)

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
