package mcpserver

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/palemoky/chinese-poetry-api/internal/database"
	"github.com/palemoky/chinese-poetry-api/internal/semantic"
	"github.com/palemoky/chinese-poetry-api/internal/vectors"
)

// stubEmbedder 把每条文本映射到同一个向量，或返回 err。
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

// semanticFixture 写入两首诗：查询向量与 2 号（江雪）最近，但两首都不含查询词。
func semanticFixture(t *testing.T, embedder semantic.Embedder) (*database.Repository, *semantic.Searcher) {
	t.Helper()
	_, repo := setupRepo(t)
	seedPoem(t, repo, 1, "春晓", "孟浩然", "唐", "春眠不觉晓，处处闻啼鸟。")
	seedPoem(t, repo, 2, "江雪", "柳宗元", "唐", "孤舟蓑笠翁，独钓寒江雪。")
	idx := &vectors.Index{
		Meta: vectors.Meta{Dims: 2, Model: "test"},
		IDs:  []int64{1, 2},
		Data: []int8{0, 127, 127, 0},
	}
	return repo, semantic.New(idx, nil, embedder)
}

func TestSemanticToolOnlyWhenEnabled(t *testing.T) {
	repo, searcher := semanticFixture(t, stubEmbedder{vector: []float32{1, 0}})

	for _, enabled := range []bool{false, true} {
		var s *semantic.Searcher
		if enabled {
			s = searcher
		}
		session := connectWith(t, repo, s)
		result, err := session.ListTools(context.Background(), nil)
		require.NoError(t, err)
		var names []string
		for _, tool := range result.Tools {
			names = append(names, tool.Name)
		}
		assert.Equal(t, enabled, contains(names, "search_poems_by_meaning"), "enabled=%t", enabled)
		assert.Equal(t, enabled, strings.Contains(session.InitializeResult().Instructions, "search_poems_by_meaning"))
	}
}

func TestSearchPoemsByMeaning(t *testing.T) {
	repo, searcher := semanticFixture(t, stubEmbedder{vector: []float32{1, 0}})
	session := connectWith(t, repo, searcher)

	out := callInto[CandidatesOutput](t, session, "search_poems_by_meaning", map[string]any{"query": "老人雪天独自钓鱼"})
	require.Len(t, out.Poems, 2)
	assert.Equal(t, "江雪", out.Poems[0].Title)
	assert.Equal(t, "柳宗元", out.Poems[0].Author)

	out = callInto[CandidatesOutput](t, session, "search_poems_by_meaning", map[string]any{"query": "雪", "limit": 1})
	assert.Len(t, out.Poems, 1)

	text, isError := call(t, session, "search_poems_by_meaning", map[string]any{"query": "雪", "limit": 51})
	assert.True(t, isError)
	assert.Contains(t, text, "limit must be between 1 and 50")

	text, isError = call(t, session, "search_poems_by_meaning", map[string]any{"query": "  "})
	assert.True(t, isError)
	assert.Contains(t, text, "query must not be empty")
}

func TestSearchPoemsByMeaningPointsToKeywordSearchWhenEmbeddingFails(t *testing.T) {
	repo, searcher := semanticFixture(t, stubEmbedder{err: errors.New("connection refused")})
	session := connectWith(t, repo, searcher)

	text, isError := call(t, session, "search_poems_by_meaning", map[string]any{"query": "雪"})
	assert.True(t, isError)
	assert.Contains(t, text, "use search_poems instead")
	assert.NotContains(t, text, "connection refused", "internal details are not leaked")
}

func contains(list []string, item string) bool {
	for _, s := range list {
		if s == item {
			return true
		}
	}
	return false
}
