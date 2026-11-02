package vectors

import (
	"errors"
	"fmt"
	"math"
	"strconv"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// FormatVersion 是向量文件结构的版本。
const FormatVersion = 1

// QuantizationInt8 表示向量先归一化，再乘 127 取整存成 int8。
// 评测中它与 float32 的命中率相差在噪声以内，体积只有四分之一。
const QuantizationInt8 = "int8"

// Meta 是向量文件的元数据。
type Meta struct {
	FormatVersion int
	TextVersion   int
	Model         string
	Dims          int
	Quantization  string
	// Fingerprint 是生成时主库的 Fingerprint，PoemCount 是其中的诗词数。
	Fingerprint string
	PoemCount   int
	// Complete 在每首诗都写入向量后才置为 true。中断的文件可以续写，但不能加载。
	Complete bool
}

const schema = `
CREATE TABLE IF NOT EXISTS meta (key TEXT PRIMARY KEY, value TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS vectors (
	id INTEGER PRIMARY KEY,
	text_hash BLOB NOT NULL,
	vector BLOB NOT NULL
);`

// File 是打开的向量文件。
type File struct {
	db *gorm.DB
}

func open(path string, readOnly bool) (*File, error) {
	dsn := "file:" + path
	if readOnly {
		dsn += "?mode=ro"
	}
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		return nil, fmt.Errorf("failed to open vector file %s: %w", path, err)
	}
	return &File{db: db}, nil
}

// Create 打开或新建一个可写的向量文件。
func Create(path string) (*File, error) {
	f, err := open(path, false)
	if err != nil {
		return nil, err
	}
	// 每条向量约 1KB，默认的 4KB 页只放得下 3 条，浪费约四分之一；64KB 页的浪费不到 2%。
	// 页大小只在建表前设置才生效，对已有的文件是空操作
	if err := f.db.Exec("PRAGMA page_size = 65536").Error; err != nil {
		_ = f.Close()
		return nil, err
	}
	if err := f.db.Exec(schema).Error; err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("failed to create vector file schema: %w", err)
	}
	return f, nil
}

// OpenReadOnly 以只读方式打开已有的向量文件。
func OpenReadOnly(path string) (*File, error) {
	return open(path, true)
}

// Close 关闭文件。
func (f *File) Close() error {
	sqlDB, err := f.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

// ErrNoMeta 表示文件里还没有元数据，即一个新建的空文件。
var ErrNoMeta = errors.New("vector file has no metadata")

// ReadMeta 读出元数据。
func (f *File) ReadMeta() (Meta, error) {
	var rows []struct{ Key, Value string }
	if err := f.db.Raw("SELECT key, value FROM meta").Scan(&rows).Error; err != nil {
		return Meta{}, fmt.Errorf("failed to read vector file metadata: %w", err)
	}
	if len(rows) == 0 {
		return Meta{}, ErrNoMeta
	}
	values := make(map[string]string, len(rows))
	for _, r := range rows {
		values[r.Key] = r.Value
	}

	var m Meta
	var err error
	atoi := func(key string) int {
		if err != nil {
			return 0
		}
		var n int
		if n, err = strconv.Atoi(values[key]); err != nil {
			err = fmt.Errorf("vector file metadata %s=%q is not a number", key, values[key])
		}
		return n
	}
	m.FormatVersion = atoi("format_version")
	m.TextVersion = atoi("text_version")
	m.Dims = atoi("dims")
	m.PoemCount = atoi("poem_count")
	m.Model = values["model"]
	m.Quantization = values["quantization"]
	m.Fingerprint = values["fingerprint"]
	m.Complete = values["complete"] == "1"
	return m, err
}

// WriteMeta 写入元数据。
func (f *File) WriteMeta(m Meta) error {
	complete := "0"
	if m.Complete {
		complete = "1"
	}
	values := map[string]string{
		"format_version": strconv.Itoa(m.FormatVersion),
		"text_version":   strconv.Itoa(m.TextVersion),
		"model":          m.Model,
		"dims":           strconv.Itoa(m.Dims),
		"quantization":   m.Quantization,
		"fingerprint":    m.Fingerprint,
		"poem_count":     strconv.Itoa(m.PoemCount),
		"complete":       complete,
	}
	return f.db.Transaction(func(tx *gorm.DB) error {
		for k, v := range values {
			if err := tx.Exec("INSERT OR REPLACE INTO meta (key, value) VALUES (?, ?)", k, v).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// Entry 是一首诗的向量。
type Entry struct {
	ID     int64
	Hash   TextHash
	Vector []int8
}

// Put 在一个事务里写入一批向量，已有的 ID 覆盖。
func (f *File) Put(entries []Entry) error {
	return f.db.Transaction(func(tx *gorm.DB) error {
		for _, e := range entries {
			if err := tx.Exec("INSERT OR REPLACE INTO vectors (id, text_hash, vector) VALUES (?, ?, ?)",
				e.ID, e.Hash[:], int8Bytes(e.Vector)).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// Hashes 返回已写入的每个 ID 及其文本哈希，用于续写时跳过已完成的诗。
func (f *File) Hashes() (map[int64]TextHash, error) {
	rows, err := f.db.Raw("SELECT id, text_hash FROM vectors").Rows()
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make(map[int64]TextHash)
	for rows.Next() {
		var id int64
		var hash []byte
		if err := rows.Scan(&id, &hash); err != nil {
			return nil, err
		}
		var h TextHash
		copy(h[:], hash)
		out[id] = h
	}
	return out, rows.Err()
}

// Compact 整理文件，去掉续写与覆盖留下的空闲页，供发布前调用。
func (f *File) Compact() error {
	return f.db.Exec("VACUUM").Error
}

// Count 返回已写入的向量数。
func (f *File) Count() (int, error) {
	var n int64
	err := f.db.Raw("SELECT COUNT(*) FROM vectors").Scan(&n).Error
	return int(n), err
}

// Reuse 是上一版向量文件的索引：按文本哈希找向量。
type Reuse struct {
	file   *File
	byHash map[TextHash]int64
}

// OpenReuse 打开上一版的向量文件供复用。文件必须完整，且模型、维度、量化与拼接规则
// 都与 want 相同，否则复用来的向量与新嵌入的不可比，返回错误。
func OpenReuse(path string, want Meta) (*Reuse, error) {
	f, err := OpenReadOnly(path)
	if err != nil {
		return nil, err
	}
	meta, err := f.ReadMeta()
	if err == nil {
		err = compatible(meta, want)
	}
	if err == nil && !meta.Complete {
		err = errors.New("it is incomplete")
	}
	if err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("cannot reuse %s: %w", path, err)
	}

	hashes, err := f.Hashes()
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	byHash := make(map[TextHash]int64, len(hashes))
	for id, h := range hashes {
		byHash[h] = id
	}
	return &Reuse{file: f, byHash: byHash}, nil
}

// Lookup 返回文本哈希为 hash 的向量，没有时 ok 为 false。
func (r *Reuse) Lookup(hash TextHash) (vector []int8, ok bool, err error) {
	id, ok := r.byHash[hash]
	if !ok {
		return nil, false, nil
	}
	var blob []byte
	if err := r.file.db.Raw("SELECT vector FROM vectors WHERE id = ?", id).Row().Scan(&blob); err != nil {
		return nil, false, err
	}
	return bytesInt8(blob), true, nil
}

// Close 关闭上一版的文件。
func (r *Reuse) Close() error { return r.file.Close() }

// compatible 检查两份元数据描述的向量能否混用或比较。
func compatible(got, want Meta) error {
	switch {
	case got.FormatVersion != want.FormatVersion:
		return fmt.Errorf("format version %d, want %d", got.FormatVersion, want.FormatVersion)
	case got.TextVersion != want.TextVersion:
		return fmt.Errorf("text version %d, want %d", got.TextVersion, want.TextVersion)
	case got.Model != want.Model:
		return fmt.Errorf("model %q, want %q", got.Model, want.Model)
	case want.Dims != 0 && got.Dims != want.Dims:
		return fmt.Errorf("%d dimensions, want %d", got.Dims, want.Dims)
	case got.Quantization != want.Quantization:
		return fmt.Errorf("quantization %q, want %q", got.Quantization, want.Quantization)
	}
	return nil
}

// Quantize 把向量归一化后量化为 int8。全零向量无法归一化，返回错误。
func Quantize(v []float32) ([]int8, error) {
	var sum float64
	for _, x := range v {
		sum += float64(x) * float64(x)
	}
	norm := math.Sqrt(sum)
	if norm == 0 || math.IsNaN(norm) || math.IsInf(norm, 0) {
		return nil, errors.New("cannot normalize a zero or non-finite vector")
	}
	out := make([]int8, len(v))
	for i, x := range v {
		out[i] = int8(max(-127, min(127, math.Round(float64(x)/norm*127))))
	}
	return out, nil
}

func int8Bytes(v []int8) []byte {
	b := make([]byte, len(v))
	for i, x := range v {
		b[i] = byte(x)
	}
	return b
}

func bytesInt8(b []byte) []int8 {
	v := make([]int8, len(b))
	for i, x := range b {
		v[i] = int8(x)
	}
	return v
}
