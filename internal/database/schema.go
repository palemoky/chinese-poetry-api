package database

const (
	// Schema version for migrations
	SchemaVersion = 1
)

// InitialDynastiesSQL contains initial data for dynasties
// Ordered chronologically by start_year for consistent ID assignment
var InitialDynastiesSQL = `INSERT OR IGNORE INTO dynasties (name, name_en, start_year, end_year) VALUES
	('先秦', 'Pre-Qin', -2070, -221),
	('两汉', 'Han', -206, 220),
	('魏晋', 'Wei-Jin', 220, 420),
	('南北朝', 'Northern and Southern', 420, 589),
	('隋', 'Sui', 581, 618),
	('唐', 'Tang', 618, 907),
	('五代', 'Five Dynasties', 907, 960),
	('宋', 'Song', 960, 1279),
	('元', 'Yuan', 1271, 1368),
	('清', 'Qing', 1644, 1912),
	('其他', 'Other', NULL, NULL)`

// InitialPoetryTypesSQL contains initial data for poetry types
// IDs use semantic ranges for easy categorization and extension:
//
//	10-19: 唐诗/诗 (Poetry)
//	20-29: 宋词/词 (Ci)
//	30-39: 元曲/曲 (Qu)
//	40-49: 蒙学 (Primer)
//	50-59: 诗经 (Book of Songs)
//	60-69: 论语 (Analects)
//	70-79: 楚辞 (Songs of Chu)
//	80-89: 四书五经 (Four Books and Five Classics)
//	99: 其他 (Other)
var InitialPoetryTypesSQL = `INSERT OR IGNORE INTO poetry_types (id, name, category, lines, chars_per_line, description) VALUES
	(10, '唐诗', '唐诗', NULL, NULL, '诗'),
	(11, '五言绝句', '诗', 4, 5, '四句，每句五字'),
	(12, '七言绝句', '诗', 4, 7, '四句，每句七字'),
	(13, '五言律诗', '诗', 8, 5, '八句，每句五字'),
	(14, '七言律诗', '诗', 8, 7, '八句，每句七字'),
	(15, '五言古诗', '诗', NULL, 5, '不限句数，每句五字'),
	(16, '七言古诗', '诗', NULL, 7, '不限句数，每句七字'),
	(20, '宋词', '词', NULL, NULL, '长短句'),
	(30, '元曲', '曲', NULL, NULL, '散曲'),
	(40, '蒙学', '蒙学', NULL, NULL, '蒙学'),
	(50, '诗经', '诗经', NULL, NULL, '诗经'),
	(60, '论语', '论语', NULL, NULL, '论语'),
	(70, '楚辞', '楚辞', NULL, NULL, '楚辞'),
	(80, '四书五经', '四书五经', NULL, NULL, '四书五经'),
	(99, '其他', '其他', NULL, NULL, '不规则或其他形式')`
