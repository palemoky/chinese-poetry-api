package vectors

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDocumentMatchesEvalRecipe(t *testing.T) {
	// 与 eval/semantic/embed_eval.py 的 f"{title}\n{author}\n{''.join(content)}"[:1500] 一致
	assert.Equal(t, "静夜思\n李白\n床前明月光，疑是地上霜。举头望明月，低头思故乡。",
		Document("静夜思", "李白", []string{"床前明月光，疑是地上霜。", "举头望明月，低头思故乡。"}))

	long := Document("长", "某", []string{strings.Repeat("字", 2000)})
	assert.Equal(t, maxDocumentRunes, len([]rune(long)), "truncated by characters, not bytes")
	assert.True(t, strings.HasPrefix(long, "长\n某\n字"))
}

func TestFingerprintDependsOnIDsAndText(t *testing.T) {
	docs := []Doc{{ID: 1, Hash: HashText("a")}, {ID: 2, Hash: HashText("b")}}
	base := Fingerprint(docs)
	assert.Equal(t, base, Fingerprint([]Doc{{ID: 1, Hash: HashText("a")}, {ID: 2, Hash: HashText("b")}}))
	assert.NotEqual(t, base, Fingerprint([]Doc{{ID: 1, Hash: HashText("a")}, {ID: 3, Hash: HashText("b")}}), "an ID changed")
	assert.NotEqual(t, base, Fingerprint([]Doc{{ID: 1, Hash: HashText("a")}, {ID: 2, Hash: HashText("c")}}), "a text changed")
	assert.NotEqual(t, base, Fingerprint(docs[:1]), "a poem removed")
}

func TestQuantize(t *testing.T) {
	q, err := Quantize([]float32{3, 0, -4})
	require.NoError(t, err)
	// 归一化为 (0.6, 0, -0.8) 后乘 127
	assert.Equal(t, []int8{76, 0, -102}, q)

	q, err = Quantize([]float32{1e30, 0})
	require.NoError(t, err)
	assert.Equal(t, []int8{127, 0}, q)

	_, err = Quantize([]float32{0, 0})
	assert.Error(t, err)
}

func testMeta(fingerprint string, n int) Meta {
	return Meta{
		FormatVersion: FormatVersion, TextVersion: TextVersion, Model: "bge-m3", Dims: 3,
		Quantization: QuantizationInt8, Fingerprint: fingerprint, PoemCount: n,
	}
}

// writeFile 写一个含 entries 的向量文件，complete 决定是否标记为完整。
func writeFile(t *testing.T, path string, meta Meta, entries []Entry, complete bool) {
	t.Helper()
	f, err := Create(path)
	require.NoError(t, err)
	defer func() { require.NoError(t, f.Close()) }()
	require.NoError(t, f.Put(entries))
	meta.Complete = complete
	require.NoError(t, f.WriteMeta(meta))
}

func TestWriteAndLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v.db")
	entries := []Entry{
		{ID: 2, Hash: HashText("b"), Vector: []int8{4, 5, 6}},
		{ID: 1, Hash: HashText("a"), Vector: []int8{1, -2, 3}},
	}
	writeFile(t, path, testMeta("fp", 2), entries, true)

	f, err := OpenReadOnly(path)
	require.NoError(t, err)
	meta, err := f.ReadMeta()
	require.NoError(t, err)
	assert.Equal(t, Meta{
		FormatVersion: FormatVersion, TextVersion: TextVersion, Model: "bge-m3", Dims: 3,
		Quantization: QuantizationInt8, Fingerprint: "fp", PoemCount: 2, Complete: true,
	}, meta)
	hashes, err := f.Hashes()
	require.NoError(t, err)
	assert.Equal(t, map[int64]TextHash{1: HashText("a"), 2: HashText("b")}, hashes)
	require.NoError(t, f.Close())

	idx, err := Load(path, "fp")
	require.NoError(t, err)
	assert.Equal(t, []int64{1, 2}, idx.IDs, "sorted by ID")
	assert.Equal(t, []int8{1, -2, 3, 4, 5, 6}, idx.Data)
}

func TestLoadRejects(t *testing.T) {
	good := []Entry{{ID: 1, Hash: HashText("a"), Vector: []int8{1, 2, 3}}}
	cases := []struct {
		name     string
		meta     func(Meta) Meta
		entries  []Entry
		complete bool
		want     string
	}{
		{"incomplete file", nil, good, false, "incomplete"},
		{"other database", nil, good, true, "different poetry database"},
		{"missing vectors", func(m Meta) Meta { m.PoemCount = 2; return m }, good, true, "has 1 vectors, want 2"},
		{"wrong size", nil, []Entry{{ID: 1, Hash: HashText("a"), Vector: []int8{1, 2}}}, true, "has 2 dimensions"},
		{"newer format", func(m Meta) Meta { m.FormatVersion++; return m }, good, true, "format"},
		{"other text recipe", func(m Meta) Meta { m.TextVersion++; return m }, good, true, "text version"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "v.db")
			meta := testMeta("fp", 1)
			if c.meta != nil {
				meta = c.meta(meta)
			}
			writeFile(t, path, meta, c.entries, c.complete)

			fingerprint := "fp"
			if c.name == "other database" {
				fingerprint = "another"
			}
			_, err := Load(path, fingerprint)
			assert.ErrorContains(t, err, c.want)
		})
	}

	empty := filepath.Join(t.TempDir(), "empty.db")
	f, err := Create(empty)
	require.NoError(t, err)
	require.NoError(t, f.Close())
	_, err = Load(empty, "fp")
	assert.ErrorIs(t, err, ErrNoMeta)

	_, err = Load(filepath.Join(t.TempDir(), "missing.db"), "fp")
	assert.Error(t, err, "a missing file is never created by Load")
}

func TestReuse(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	writeFile(t, path, testMeta("old", 1), []Entry{{ID: 7, Hash: HashText("a"), Vector: []int8{1, 2, 3}}}, true)

	r, err := OpenReuse(path, testMeta("new", 0))
	require.NoError(t, err)
	defer func() { _ = r.Close() }()

	v, ok, err := r.Lookup(HashText("a"))
	require.NoError(t, err)
	assert.True(t, ok, "found by text, whatever its old ID")
	assert.Equal(t, []int8{1, 2, 3}, v)

	_, ok, err = r.Lookup(HashText("changed"))
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestReuseRejectsIncompatibleFiles(t *testing.T) {
	entries := []Entry{{ID: 1, Hash: HashText("a"), Vector: []int8{1, 2, 3}}}
	cases := []struct {
		name     string
		meta     func(Meta) Meta
		complete bool
		want     string
	}{
		{"incomplete", func(m Meta) Meta { return m }, false, "incomplete"},
		{"other model", func(m Meta) Meta { m.Model = "other"; return m }, true, "model"},
		{"other text recipe", func(m Meta) Meta { m.TextVersion++; return m }, true, "text version"},
		{"other quantization", func(m Meta) Meta { m.Quantization = "float32"; return m }, true, "quantization"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "old.db")
			writeFile(t, path, c.meta(testMeta("old", 1)), entries, c.complete)
			_, err := OpenReuse(path, testMeta("new", 0))
			assert.ErrorContains(t, err, c.want)
		})
	}
}
