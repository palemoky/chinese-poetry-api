package graph

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/99designs/gqlgen/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"

	"github.com/palemoky/chinese-poetry-api/internal/database"
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

// semanticClient 写入两首诗并返回启用了语义检索的客户端：查询向量与 2 号（江雪）最近。
func semanticClient(t *testing.T, embedder semantic.Embedder) *client.Client {
	t.Helper()
	resolver, repo := setupTestResolver(t)
	dynastyID, err := repo.GetOrCreateDynasty("唐")
	require.NoError(t, err)
	authorID, err := repo.GetOrCreateAuthor("柳宗元", dynastyID)
	require.NoError(t, err)
	for id, p := range map[int64][2]string{1: {"春晓", "春眠不觉晓"}, 2: {"江雪", "孤舟蓑笠翁，独钓寒江雪。"}} {
		content, err := json.Marshal([]string{p[1]})
		require.NoError(t, err)
		require.NoError(t, repo.InsertPoem(&database.Poem{
			ID: id, Title: p[0], Content: datatypes.JSON(content), ContentHash: p[0], AuthorID: &authorID, DynastyID: &dynastyID,
		}))
	}
	idx := &vectors.Index{Meta: vectors.Meta{Dims: 2}, IDs: []int64{1, 2}, Data: []int8{0, 127, 127, 0}}
	resolver.Searcher = semantic.New(idx, nil, embedder)
	return createTestClient(t, resolver)
}

func TestSemanticSearchPoems(t *testing.T) {
	c := semanticClient(t, stubEmbedder{vector: []float32{1, 0}})

	var resp struct {
		SemanticSearchPoems []struct {
			Title  string
			Author struct{ Name string }
		}
	}
	require.NoError(t, c.Post(`{ semanticSearchPoems(query: "老人雪天独自钓鱼") { title author { name } } }`, &resp))
	require.Len(t, resp.SemanticSearchPoems, 2)
	assert.Equal(t, "江雪", resp.SemanticSearchPoems[0].Title)
	assert.Equal(t, "柳宗元", resp.SemanticSearchPoems[0].Author.Name)

	require.NoError(t, c.Post(`{ semanticSearchPoems(query: "雪", limit: 1) { title } }`, &resp))
	assert.Len(t, resp.SemanticSearchPoems, 1)

	err := c.Post(`{ semanticSearchPoems(query: "雪", limit: 51) { title } }`, &resp)
	assert.ErrorContains(t, err, "limit must be between 1 and 50")
}

func TestSemanticSearchPoemsErrors(t *testing.T) {
	var resp map[string]any

	resolver, _ := setupTestResolver(t)
	err := createTestClient(t, resolver).Post(`{ semanticSearchPoems(query: "雪") { title } }`, &resp)
	assert.ErrorContains(t, err, "semantic search is not enabled on this server")

	c := semanticClient(t, stubEmbedder{err: errors.New("connection refused")})
	err = c.Post(`{ semanticSearchPoems(query: "雪") { title } }`, &resp)
	assert.ErrorContains(t, err, "embedding service unavailable")
	assert.NotContains(t, err.Error(), "connection refused")
}

func TestSemanticSearchPoemsComplexityCountsEachPoem(t *testing.T) {
	c := semanticClient(t, stubEmbedder{vector: []float32{1, 0}})
	var resp map[string]any
	// 50 首诗各展开作者的 100 首诗，远超上限 5000
	err := c.Post(`{ semanticSearchPoems(query: "雪", limit: 50) {
		author { poems(pageSize: 100) { edges { node { title } } } } } }`, &resp)
	assert.ErrorContains(t, err, "complexity")
}
