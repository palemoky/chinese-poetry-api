package semantic

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"

	"github.com/palemoky/chinese-poetry-api/internal/database"
	"github.com/palemoky/chinese-poetry-api/internal/testutil"
	"github.com/palemoky/chinese-poetry-api/internal/vectors"
)

// openFixture 建一个两首诗的库，以及为它（fingerprint 为空时）或为别的库生成的向量文件。
func openFixture(t *testing.T, fingerprint string) (*database.DB, string) {
	t.Helper()
	db, repo := testutil.SetupTestDB(t)
	dynastyID, err := repo.GetOrCreateDynasty("唐")
	require.NoError(t, err)
	authorID, err := repo.GetOrCreateAuthor("李白", dynastyID)
	require.NoError(t, err)
	for id, text := range map[int64]string{1: "床前明月光", 2: "春眠不觉晓"} {
		content, err := json.Marshal([]string{text})
		require.NoError(t, err)
		require.NoError(t, repo.InsertPoem(&database.Poem{
			ID: id, Title: "题", Content: datatypes.JSON(content), ContentHash: text, AuthorID: &authorID, DynastyID: &dynastyID,
		}))
	}

	docs, err := vectors.LoadCorpus(db)
	require.NoError(t, err)
	if fingerprint == "" {
		fingerprint = vectors.Fingerprint(docs)
	}

	path := filepath.Join(t.TempDir(), "poetry-vectors.db")
	f, err := vectors.Create(path)
	require.NoError(t, err)
	var entries []vectors.Entry
	for _, d := range docs {
		entries = append(entries, vectors.Entry{ID: d.ID, Hash: d.Hash, Vector: []int8{127, 0}})
	}
	require.NoError(t, f.Put(entries))
	require.NoError(t, f.WriteMeta(vectors.Meta{
		FormatVersion: vectors.FormatVersion, TextVersion: vectors.TextVersion, Model: "bge-m3", Dims: 2,
		Quantization: vectors.QuantizationInt8, Fingerprint: fingerprint, PoemCount: len(docs), Complete: true,
	}))
	require.NoError(t, f.Close())
	return db, path
}

// embeddingServer 返回 dims 维的向量，并检查请求的模型名。
func embeddingServer(t *testing.T, dims int, wantModel string) string {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Model string
			Input []string
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		assert.Equal(t, wantModel, req.Model)
		vec := make([]float32, dims)
		vec[0] = 1
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{{"index": 0, "embedding": vec}}})
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func TestOpen(t *testing.T) {
	t.Run("enabled, model taken from the vector file", func(t *testing.T) {
		db, path := openFixture(t, "")
		s, warning, err := Open(context.Background(), Config{VectorsPath: path, EmbeddingURL: embeddingServer(t, 2, "bge-m3")}, db)
		require.NoError(t, err)
		assert.Empty(t, warning)
		assert.Equal(t, 2, s.Size())
	})

	t.Run("model name overridden", func(t *testing.T) {
		db, path := openFixture(t, "")
		_, _, err := Open(context.Background(), Config{
			VectorsPath: path, EmbeddingURL: embeddingServer(t, 2, "BAAI/bge-m3"), EmbeddingModel: "BAAI/bge-m3",
		}, db)
		require.NoError(t, err)
	})

	t.Run("no embedding API", func(t *testing.T) {
		db, path := openFixture(t, "")
		_, _, err := Open(context.Background(), Config{VectorsPath: path}, db)
		assert.True(t, IsDisabled(err))
	})

	t.Run("no vector file", func(t *testing.T) {
		db, _ := openFixture(t, "")
		_, _, err := Open(context.Background(), Config{
			VectorsPath: filepath.Join(t.TempDir(), "missing.db"), EmbeddingURL: "http://127.0.0.1:9/v1",
		}, db)
		assert.True(t, IsDisabled(err))
	})

	t.Run("vector file of another database", func(t *testing.T) {
		db, path := openFixture(t, "another")
		_, _, err := Open(context.Background(), Config{VectorsPath: path, EmbeddingURL: embeddingServer(t, 2, "bge-m3")}, db)
		assert.ErrorContains(t, err, "different poetry database")
		assert.False(t, IsDisabled(err), "a mismatch is a configuration error, not a missing feature")
	})

	t.Run("API serves another model", func(t *testing.T) {
		db, path := openFixture(t, "")
		_, _, err := Open(context.Background(), Config{VectorsPath: path, EmbeddingURL: embeddingServer(t, 3, "bge-m3")}, db)
		assert.ErrorContains(t, err, "3-dimensional")
	})

	t.Run("API not reachable yet", func(t *testing.T) {
		db, path := openFixture(t, "")
		srv := httptest.NewServer(http.NotFoundHandler())
		url := srv.URL
		srv.Close()

		s, warning, err := Open(context.Background(), Config{VectorsPath: path, EmbeddingURL: url}, db)
		require.NoError(t, err)
		assert.NotNil(t, s, "stays enabled; queries retry the API")
		assert.Contains(t, warning, "not reachable")
	})
}
