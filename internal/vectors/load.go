package vectors

import (
	"fmt"
)

// Index 是载入内存的全部向量，按 ID 升序连续存放：第 i 首诗的向量是 Data[i*Dims : (i+1)*Dims]。
type Index struct {
	Meta Meta
	IDs  []int64
	Data []int8
}

// Load 读入整份向量文件。fingerprint 是当前主库的 Fingerprint：
// 向量文件必须完整、是本程序能读的格式，并且正是为这份主库生成的，否则返回错误，
// 调用方应当关闭语义检索，而不是用错位的向量返回错误的结果。
func Load(path, fingerprint string) (*Index, error) {
	f, err := OpenReadOnly(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	meta, err := f.ReadMeta()
	if err != nil {
		return nil, err
	}
	switch {
	case meta.FormatVersion != FormatVersion:
		return nil, fmt.Errorf("vector file format %d is not supported (want %d)", meta.FormatVersion, FormatVersion)
	case meta.TextVersion != TextVersion:
		return nil, fmt.Errorf("vector file text version %d is not supported (want %d)", meta.TextVersion, TextVersion)
	case meta.Quantization != QuantizationInt8:
		return nil, fmt.Errorf("vector file quantization %q is not supported", meta.Quantization)
	case !meta.Complete:
		return nil, fmt.Errorf("vector file is incomplete")
	case meta.Dims <= 0:
		return nil, fmt.Errorf("vector file has %d dimensions", meta.Dims)
	case meta.Fingerprint != fingerprint:
		return nil, fmt.Errorf("vector file was built for a different poetry database; download the one released with it")
	}

	idx := &Index{
		Meta: meta,
		IDs:  make([]int64, 0, meta.PoemCount),
		Data: make([]int8, 0, meta.PoemCount*meta.Dims),
	}
	rows, err := f.db.Raw("SELECT id, vector FROM vectors ORDER BY id").Rows()
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var blob []byte
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id, &blob); err != nil {
			return nil, err
		}
		if len(blob) != meta.Dims {
			return nil, fmt.Errorf("vector of poem %d has %d dimensions, want %d", id, len(blob), meta.Dims)
		}
		idx.IDs = append(idx.IDs, id)
		for _, b := range blob {
			idx.Data = append(idx.Data, int8(b))
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(idx.IDs) != meta.PoemCount {
		return nil, fmt.Errorf("vector file has %d vectors, want %d", len(idx.IDs), meta.PoemCount)
	}
	return idx, nil
}
