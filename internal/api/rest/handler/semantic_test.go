package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/palemoky/chinese-poetry-api/internal/semantic"
	"github.com/palemoky/chinese-poetry-api/internal/vectors"
)

type stubEmbedder struct {
	vector []float32
	err    error
}

func (s stubEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	if s.err != nil {
		return nil, s.err
	}
	out := make([][]float32, len(texts))
	for i := range out {
		out[i] = s.vector
	}
	return out, nil
}

func TestSemanticSearchHandler(t *testing.T) {
	router, repo := setupPoemTestRouter(t)
	createTestPoem(t, repo, 1, "春晓", "春眠不觉晓")
	createTestPoem(t, repo, 2, "江雪", "孤舟蓑笠翁，独钓寒江雪。")
	idx := &vectors.Index{Meta: vectors.Meta{Dims: 2}, IDs: []int64{1, 2}, Data: []int8{0, 127, 127, 0}}

	ok := semantic.New(idx, nil, stubEmbedder{vector: []float32{1, 0}})
	failing := semantic.New(idx, nil, stubEmbedder{err: errors.New("connection refused")})
	router.GET("/semantic", SemanticSearchHandler(repo, ok))
	router.GET("/failing", SemanticSearchHandler(repo, failing))
	router.GET("/disabled", SemanticSearchHandler(repo, nil))

	get := func(path string) (int, map[string]any) {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		var body map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		return w.Code, body
	}

	code, body := get("/semantic?q=" + "%E9%9B%AA%E5%A4%A9%E9%92%93%E9%B1%BC")
	require.Equal(t, http.StatusOK, code)
	data := body["data"].([]any)
	require.Len(t, data, 2)
	assert.Equal(t, "江雪", data[0].(map[string]any)["title"])

	code, body = get("/semantic?q=x&limit=1")
	assert.Equal(t, http.StatusOK, code)
	assert.Len(t, body["data"], 1)

	for _, path := range []string{"/semantic", "/semantic?q=x&limit=0", "/semantic?q=x&limit=51", "/semantic?q=x&page=2"} {
		code, _ = get(path)
		assert.Equal(t, http.StatusBadRequest, code, path)
	}

	code, body = get("/failing?q=x")
	assert.Equal(t, http.StatusBadGateway, code)
	assert.Equal(t, "embedding service unavailable", body["error"])

	code, _ = get("/disabled?q=x")
	assert.Equal(t, http.StatusServiceUnavailable, code)
}
