package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"

	"github.com/palemoky/chinese-poetry-api/internal/database"
	"github.com/palemoky/chinese-poetry-api/internal/vectors"
)

// buildPoetryDB 建一个含 poems 的主库文件，poems 的键是 ID，值是正文。
func buildPoetryDB(t *testing.T, path string, poems map[int64]string) {
	t.Helper()
	db, err := database.Open(path, 1, 1)
	require.NoError(t, err)
	require.NoError(t, db.Migrate())
	repo := database.NewRepository(db)
	dynastyID, err := repo.GetOrCreateDynasty("唐")
	require.NoError(t, err)
	authorID, err := repo.GetOrCreateAuthor("李白", dynastyID)
	require.NoError(t, err)
	for id, text := range poems {
		content, err := json.Marshal([]string{text})
		require.NoError(t, err)
		require.NoError(t, repo.InsertPoem(&database.Poem{
			ID: id, Title: "题", Content: datatypes.JSON(content), ContentHash: text,
			AuthorID: &authorID, DynastyID: &dynastyID,
		}))
	}
	require.NoError(t, db.Close())
}

// fakeEmbedder 为每条文本返回一个由长度决定的向量，并记下嵌入过哪些文本。
type fakeEmbedder struct {
	mu    sync.Mutex
	texts []string
}

func (f *fakeEmbedder) server(t *testing.T) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct{ Input []string }
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		f.mu.Lock()
		f.texts = append(f.texts, req.Input...)
		f.mu.Unlock()
		type item struct {
			Index     int       `json:"index"`
			Embedding []float32 `json:"embedding"`
		}
		var resp struct {
			Data []item `json:"data"`
		}
		for i, text := range req.Input {
			resp.Data = append(resp.Data, item{i, []float32{float32(len([]rune(text))), 1, 0}})
		}
		require.NoError(t, json.NewEncoder(w).Encode(resp))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestEmbedBuildsLoadableFileAndReusesUnchangedPoems(t *testing.T) {
	dir := t.TempDir()
	fake := &fakeEmbedder{}
	api := fake.server(t).URL

	// 第一版：三首诗
	v1 := filepath.Join(dir, "v1.db")
	buildPoetryDB(t, v1, map[int64]string{1: "床前明月光", 2: "春眠不觉晓", 3: "白日依山尽"})
	out1 := filepath.Join(dir, "vectors-v1.db")
	require.NoError(t, runEmbed(context.Background(), embedOptions{db: v1, out: out1, api: api, model: "m", batch: 2}))
	assert.Len(t, fake.texts, 3)

	idx := loadFor(t, v1, out1)
	assert.Equal(t, []int64{1, 2, 3}, idx.IDs)
	assert.Equal(t, 3, idx.Meta.Dims)

	// 第二版：ID 全部变了，一首改了字，一首是新的。只有这两首需要重新嵌入
	fake.texts = nil
	v2 := filepath.Join(dir, "v2.db")
	buildPoetryDB(t, v2, map[int64]string{10: "床前明月光", 11: "春眠不觉晓。", 12: "白日依山尽", 13: "千山鸟飞绝"})
	out2 := filepath.Join(dir, "vectors-v2.db")
	require.NoError(t, runEmbed(context.Background(), embedOptions{db: v2, out: out2, reuse: out1, api: api, model: "m", batch: 2}))
	assert.ElementsMatch(t, []string{
		vectors.Document("题", "李白", []string{"春眠不觉晓。"}),
		vectors.Document("题", "李白", []string{"千山鸟飞绝"}),
	}, fake.texts)

	idx2 := loadFor(t, v2, out2)
	assert.Equal(t, []int64{10, 11, 12, 13}, idx2.IDs)
	assert.Equal(t, idx.Data[0:3], idx2.Data[0:3], "the reused vector follows its text to the new ID")

	// 第一版的向量文件不能配第二版的主库
	_, err := vectors.Load(out1, fingerprintOf(t, v2))
	assert.ErrorContains(t, err, "different poetry database")
}

func TestEmbedResumesAndRefusesForeignOutput(t *testing.T) {
	dir := t.TempDir()
	fake := &fakeEmbedder{}
	api := fake.server(t).URL

	db := filepath.Join(dir, "poetry.db")
	buildPoetryDB(t, db, map[int64]string{1: "床前明月光", 2: "春眠不觉晓"})
	out := filepath.Join(dir, "vectors.db")
	require.NoError(t, runEmbed(context.Background(), embedOptions{db: db, out: out, api: api, model: "m", batch: 1}))

	// 再跑一次：全部续用，不再请求接口
	fake.texts = nil
	require.NoError(t, runEmbed(context.Background(), embedOptions{db: db, out: out, api: api, model: "m", batch: 1}))
	assert.Empty(t, fake.texts)

	// 换了模型就不能续写同一个文件
	err := runEmbed(context.Background(), embedOptions{db: db, out: out, api: api, model: "other", batch: 1})
	assert.ErrorContains(t, err, "different database or model")
}

func TestEmbedNeedsTheNamedKey(t *testing.T) {
	err := runEmbed(context.Background(), embedOptions{apiKeyEnv: "POETRY_TEST_UNSET_KEY", batch: 1})
	assert.ErrorContains(t, err, "POETRY_TEST_UNSET_KEY is empty")
}

func fingerprintOf(t *testing.T, path string) string {
	t.Helper()
	db, err := database.OpenReadOnly(path, 1, 1)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	docs, err := vectors.LoadCorpus(db)
	require.NoError(t, err)
	return vectors.Fingerprint(docs)
}

func loadFor(t *testing.T, dbPath, vectorPath string) *vectors.Index {
	t.Helper()
	idx, err := vectors.Load(vectorPath, fingerprintOf(t, dbPath))
	require.NoError(t, err)
	return idx
}

func TestCheckEmbeddings(t *testing.T) {
	dir := t.TempDir()
	fake := &fakeEmbedder{}
	api := fake.server(t).URL

	db := filepath.Join(dir, "poetry.db")
	buildPoetryDB(t, db, map[int64]string{1: "床前明月光", 2: "春眠不觉晓", 3: "白日依山尽"})
	out := filepath.Join(dir, "vectors.db")
	require.NoError(t, runEmbed(context.Background(), embedOptions{db: db, out: out, api: api, model: "m", batch: 8}))

	// 同一个接口：向量一致
	require.NoError(t, runCheckEmbeddings(context.Background(), checkOptions{db: db, vectors: out, api: api, sample: 3}))

	// 另一个接口给出不同的向量
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct{ Input []string }
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		data := make([]map[string]any, len(req.Input))
		for i := range req.Input {
			data[i] = map[string]any{"index": i, "embedding": []float32{0, 0, 1}}
		}
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{"data": data}))
	}))
	defer other.Close()
	err := runCheckEmbeddings(context.Background(), checkOptions{db: db, vectors: out, api: other.URL, sample: 3})
	assert.ErrorContains(t, err, "does not produce the vectors")
}

// 同一个模型在不同接口上叫法不同：文件记录统一的名字，复用照常生效
func TestEmbedAPIModelNameIsSeparateFromRecordedModel(t *testing.T) {
	dir := t.TempDir()
	var gotModels []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Model string
			Input []string
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		gotModels = append(gotModels, req.Model)
		data := make([]map[string]any, len(req.Input))
		for i := range req.Input {
			data[i] = map[string]any{"index": i, "embedding": []float32{1, 0, 0}}
		}
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{"data": data}))
	}))
	defer srv.Close()

	v1 := filepath.Join(dir, "v1.db")
	buildPoetryDB(t, v1, map[int64]string{1: "床前明月光"})
	out1 := filepath.Join(dir, "vectors-v1.db")
	require.NoError(t, runEmbed(context.Background(), embedOptions{db: v1, out: out1, api: srv.URL, model: "bge-m3", batch: 8}))

	v2 := filepath.Join(dir, "v2.db")
	buildPoetryDB(t, v2, map[int64]string{1: "床前明月光", 2: "春眠不觉晓"})
	out2 := filepath.Join(dir, "vectors-v2.db")
	require.NoError(t, runEmbed(context.Background(), embedOptions{
		db: v2, out: out2, reuse: out1, api: srv.URL, model: "bge-m3", apiModel: "Pro/BAAI/bge-m3", batch: 8,
	}))

	assert.Equal(t, []string{"bge-m3", "Pro/BAAI/bge-m3"}, gotModels, "only the new poem is embedded, under the API's name")
	assert.Equal(t, "bge-m3", loadFor(t, v2, out2).Meta.Model)
}
