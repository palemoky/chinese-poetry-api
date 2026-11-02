package semantic

import (
	"context"
	"encoding/json"
	"errors"
	"math/rand/v2"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"

	"github.com/palemoky/chinese-poetry-api/internal/database"
	"github.com/palemoky/chinese-poetry-api/internal/testutil"
	"github.com/palemoky/chinese-poetry-api/internal/vectors"
)

func TestDot(t *testing.T) {
	a := []int8{1, -2, 3, 4, 5, -127, 127}
	b := []int8{7, 6, -5, 4, 3, -127, 127}
	assert.EqualValues(t, 7-12-15+16+15+16129+16129, dot(a, b))
}

// randomIndex 造一份 n 条 dims 维的随机向量，ID 从 1 开始。
func randomIndex(n, dims int, seed uint64) *vectors.Index {
	rng := rand.New(rand.NewPCG(seed, 1))
	idx := &vectors.Index{Meta: vectors.Meta{Dims: dims, Model: "test"}}
	for i := range n {
		idx.IDs = append(idx.IDs, int64(i+1))
		for range dims {
			idx.Data = append(idx.Data, int8(rng.IntN(255)-127))
		}
	}
	return idx
}

func TestTopRowsMatchesBruteForce(t *testing.T) {
	// 多于 1000 条才会分给多个协程；故意放进大量同分的行，检查同分时下标小的在前
	idx := randomIndex(5000, 8, 1)
	for row := 100; row < 5000; row += 7 {
		copy(idx.Data[row*8:(row+1)*8], idx.Data[0:8])
	}
	s := New(idx, nil, nil)
	q := []int8{3, -1, 4, 1, -5, 9, 2, -6}

	all := make([]candidate, 5000)
	for row := range all {
		all[row] = candidate{dot(q, idx.Data[row*8:(row+1)*8]), int32(row)}
	}
	slices.SortStableFunc(all, func(a, b candidate) int { return int(b.score - a.score) })

	for _, k := range []int{1, 10, 1000, 5000, 6000} {
		assert.Equal(t, all[:min(k, len(all))], s.topRows(q, k), "k=%d", k)
	}
}

func TestVectorRankingAddsAnthologyWeight(t *testing.T) {
	// 余弦：1 号 1.0，2 号 0.96，3 号 0.6。2 号入选后 1.01，越过 1 号；3 号入选后 0.65，仍在最后
	idx := &vectors.Index{
		Meta: vectors.Meta{Dims: 2},
		IDs:  []int64{1, 2, 3},
		Data: []int8{127, 0, 122, 35, 76, 102},
	}
	s := New(idx, []int64{2, 3}, nil)
	assert.Equal(t, []int64{2, 1, 3}, s.vectorRanking([]int8{127, 0}))
}

func TestFuse(t *testing.T) {
	// 1 号在两个列表里都第一，得分最高；2 号与 4 号同分（各在一个列表里排第二），
	// 先出现在关键词列表里的 2 号在前；3 号只在关键词列表第三
	got := fuse([]int64{1, 2, 3}, []int64{1, 4})
	assert.Equal(t, []int64{1, 2, 4, 3}, got)

	assert.Equal(t, []int64{5, 6}, fuse(nil, []int64{5, 6}), "no keyword matches")
}

// fakeEmbedder 返回预设的向量，并记下收到的文本。
type fakeEmbedder struct {
	mu      sync.Mutex
	vectors map[string][]float32
	err     error
	texts   []string
}

func (f *fakeEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	f.mu.Lock()
	f.texts = append(f.texts, texts...)
	f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	out := make([][]float32, len(texts))
	for i, text := range texts {
		v, ok := f.vectors[text]
		if !ok {
			return nil, errors.New("unexpected text " + text)
		}
		out[i] = v
	}
	return out, nil
}

// smallCorpus 建一个简繁两表都有的小库，返回仓储与对应的向量：
// 1 号与查询意思最近但不含查询词，2 号含查询词但意思远，3 号两者都不沾。
func smallCorpus(t *testing.T) (*database.Repository, *vectors.Index) {
	t.Helper()
	db, repo := testutil.SetupTestDB(t)
	for _, lang := range []database.Lang{database.LangHans, database.LangHant} {
		r := database.NewRepositoryWithLang(db, lang)
		dynastyID, err := r.GetOrCreateDynasty("唐")
		require.NoError(t, err)
		authorID, err := r.GetOrCreateAuthor("柳宗元", dynastyID)
		require.NoError(t, err)
		for _, p := range []struct {
			id      int64
			title   string
			content string
		}{{1, "江雪", "孤舟蓑笠翁，独钓寒江雪。"}, {2, "钓鱼", "闲来垂钓碧溪上。"}, {3, "春晓", "春眠不觉晓。"}} {
			content, err := json.Marshal([]string{p.content})
			require.NoError(t, err)
			require.NoError(t, r.InsertPoem(&database.Poem{
				ID: p.id, Title: p.title, Content: datatypes.JSON(content), ContentHash: p.title,
				AuthorID: &authorID, DynastyID: &dynastyID,
			}))
		}
	}
	idx := &vectors.Index{
		Meta: vectors.Meta{Dims: 2, Model: "test"},
		IDs:  []int64{1, 2, 3},
		Data: []int8{127, 0, 0, 127, -127, 0},
	}
	return repo, idx
}

func TestSearchFusesMeaningAndKeywords(t *testing.T) {
	repo, idx := smallCorpus(t)
	embedder := &fakeEmbedder{vectors: map[string][]float32{"钓鱼": {1, 0}}}
	s := New(idx, nil, embedder)

	// 1 号意思最近，2 号是关键词命中；两者各在一个列表里第一、另一个列表里靠后
	ids, err := s.Search(context.Background(), repo, "钓鱼", 10)
	require.NoError(t, err)
	assert.Equal(t, []int64{2, 1, 3}, ids, "2 leads both lists' combined score: first by keyword, second by meaning")

	ids, err = s.Search(context.Background(), repo, "钓鱼", 1)
	require.NoError(t, err)
	assert.Len(t, ids, 1)
}

func TestSearchEmbedsTraditionalQueriesAsSimplified(t *testing.T) {
	repo, idx := smallCorpus(t)
	embedder := &fakeEmbedder{vectors: map[string][]float32{"钓鱼": {1, 0}}}
	s := New(idx, nil, embedder)

	_, err := s.Search(context.Background(), repo.WithLang(database.LangHant), "釣魚", 10)
	require.NoError(t, err)
	assert.Equal(t, []string{"钓鱼"}, embedder.texts)
}

func TestSearchErrors(t *testing.T) {
	repo, idx := smallCorpus(t)

	s := New(idx, nil, &fakeEmbedder{vectors: map[string][]float32{}})
	_, err := s.Search(context.Background(), repo, strings.Repeat("月", MaxQueryRunes+1), 10)
	assert.ErrorIs(t, err, ErrQueryTooLong)

	s = New(idx, nil, &fakeEmbedder{err: errors.New("connection refused")})
	_, err = s.Search(context.Background(), repo, "钓鱼", 10)
	assert.ErrorIs(t, err, ErrEmbedding)

	// 接口换了模型：维度对不上，同样算嵌入失败，并说明原因
	s = New(idx, nil, &fakeEmbedder{vectors: map[string][]float32{"钓鱼": {1, 0, 0}}})
	_, err = s.Search(context.Background(), repo, "钓鱼", 10)
	assert.ErrorIs(t, err, ErrEmbedding)
	assert.ErrorContains(t, err, "3-dimensional")
}
