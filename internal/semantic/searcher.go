// Package semantic 实现按意思检索诗词：查询向量与发布的 int8 向量比相似度，
// 再与关键词搜索的结果融合。
//
// 流程与参数照搬 eval/semantic 中不依赖 GPU 的最佳方案（client_rerank.py 的候选流程），
// 改动前应先在那里重新评测：
//  1. 查询向量与全部诗词向量求点积，取最相似的 pool 首；
//  2. 这些诗按“余弦相似度 + 选本加分”重排；
//  3. 与关键词搜索的前 keywordFetch 条用 RRF 融合。
package semantic

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"runtime"
	"slices"
	"sync"
	"unicode/utf8"

	"github.com/palemoky/chinese-poetry-api/internal/classifier"
	"github.com/palemoky/chinese-poetry-api/internal/database"
	"github.com/palemoky/chinese-poetry-api/internal/vectors"
)

const (
	// pool 是参与重排与融合的向量候选数，与评测中的 POOL 相同。
	pool = 1000
	// keywordFetch 是参与融合的关键词结果数，与评测中的 FETCH 相同。
	keywordFetch = 100
	// rrfC 是 RRF 的平滑常数，与评测中的 RRF_C 相同。
	rrfC = 60
	// anthologyWeight 是选本诗在余弦相似度上的加分，评测中 0.05 让名篇多命中 10 条，冷门诗不受影响。
	anthologyWeight = 0.05

	// MaxQueryRunes 是查询长度上限。描述一首诗用不了这么长，更长的多半是误用，也会拖慢嵌入。
	MaxQueryRunes = 200
	// MaxLimit 是一次最多返回的诗词数。
	MaxLimit = 50
)

// ErrQueryTooLong 表示查询超过 MaxQueryRunes。
var ErrQueryTooLong = fmt.Errorf("query must be at most %d characters", MaxQueryRunes)

// ErrEmbedding 表示嵌入接口出错（连不上、超时、模型不对），而不是本服务的问题。
var ErrEmbedding = errors.New("embedding service failed")

// Embedder 把文本转成向量，由 embedding.Client 实现。
type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}

// Searcher 持有载入内存的向量，可并发使用。
type Searcher struct {
	ids       []int64
	data      []int8
	dims      int
	anthology []bool // 与 ids 一一对应
	embedder  Embedder
	model     string
}

// New 用载入的向量创建 Searcher。anthologyIDs 是收在选本里的诗词 id，简繁两表一致。
func New(idx *vectors.Index, anthologyIDs []int64, embedder Embedder) *Searcher {
	anthology := make([]bool, len(idx.IDs))
	for i, id := range idx.IDs {
		_, anthology[i] = slices.BinarySearch(anthologyIDs, id)
	}
	return &Searcher{
		ids:       idx.IDs,
		data:      idx.Data,
		dims:      idx.Meta.Dims,
		anthology: anthology,
		embedder:  embedder,
		model:     idx.Meta.Model,
	}
}

// Size 返回向量数。
func (s *Searcher) Size() int { return len(s.ids) }

// Search 返回与 query 最相关的至多 limit 首诗的 id，按相关度排序。
// repo 决定关键词搜索查哪个语言变体；向量两种语言共用，因为两表 ID 一一对应。
func (s *Searcher) Search(ctx context.Context, repo *database.Repository, query string, limit int) ([]int64, error) {
	if utf8.RuneCountInString(query) > MaxQueryRunes {
		return nil, ErrQueryTooLong
	}
	limit = max(1, min(limit, MaxLimit))

	q, err := s.embedQuery(ctx, query)
	if err != nil {
		return nil, err
	}
	semantic := s.vectorRanking(q)

	keyword, err := repo.SearchPoemIDs(query, keywordFetch)
	if err != nil {
		return nil, err
	}

	fused := fuse(keyword, semantic)
	return fused[:min(limit, len(fused))], nil
}

func (s *Searcher) embedQuery(ctx context.Context, query string) ([]int8, error) {
	// 诗词向量按简体文本生成（vectors.LoadCorpus），繁体查询先转成简体再嵌入，
	// 评测中繁体查询前 30 条的命中因此从 40/64 回到与简体相同的 43/64。
	// 关键词搜索仍用原查询查对应的语言表
	simplified, err := classifier.ToSimplified(query)
	if err != nil {
		return nil, err
	}
	raw, err := s.embedder.Embed(ctx, []string{simplified})
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrEmbedding, err)
	}
	if len(raw) != 1 || len(raw[0]) != s.dims {
		return nil, fmt.Errorf("%w: %w", ErrEmbedding, &dimensionError{got: lenFirst(raw), want: s.dims, model: s.model})
	}
	q, err := vectors.Quantize(raw[0])
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrEmbedding, err)
	}
	return q, nil
}

// dimensionError 表示嵌入接口用的模型与生成向量文件的不是同一个。
type dimensionError struct {
	got, want int
	model     string
}

func (e *dimensionError) Error() string {
	return fmt.Sprintf("embedding API returned a %d-dimensional vector, the vector file has %d; "+
		"queries must be embedded with the model the file was built with (%s)", e.got, e.want, e.model)
}

func lenFirst(raw [][]float32) int {
	if len(raw) == 0 {
		return 0
	}
	return len(raw[0])
}

// candidate 是一首诗的向量得分，row 是它在 ids 中的下标。
type candidate struct {
	score int32
	row   int32
}

// vectorRanking 返回与 q 最相似的 pool 首诗的 id，按“余弦相似度 + 选本加分”降序。
func (s *Searcher) vectorRanking(q []int8) []int64 {
	top := s.topRows(q, pool)

	// int8 向量是归一化向量乘 127 取整，点积除以 127² 即余弦相似度
	const scale = 127 * 127
	type ranked struct {
		id    int64
		score float64
	}
	out := make([]ranked, len(top))
	for i, c := range top {
		score := float64(c.score) / scale
		if s.anthology[c.row] {
			score += anthologyWeight
		}
		out[i] = ranked{s.ids[c.row], score}
	}
	slices.SortStableFunc(out, func(a, b ranked) int { return cmp.Compare(b.score, a.score) })

	ids := make([]int64, len(out))
	for i, r := range out {
		ids[i] = r.id
	}
	return ids
}

// topRows 并行计算 q 与全部向量的点积，返回得分最高的 k 行（降序，同分时下标小的在前）。
func (s *Searcher) topRows(q []int8, k int) []candidate {
	n := len(s.ids)
	workers := max(1, min(runtime.GOMAXPROCS(0), n/1000))
	chunk := (n + workers - 1) / workers

	parts := make([][]candidate, workers)
	var wg sync.WaitGroup
	for w := range workers {
		wg.Go(func() {
			start, end := w*chunk, min((w+1)*chunk, n)
			h := make(minHeap, 0, k)
			for row := start; row < end; row++ {
				h.offer(candidate{dot(q, s.data[row*s.dims:(row+1)*s.dims]), int32(row)}, k)
			}
			parts[w] = h
		})
	}
	wg.Wait()

	all := slices.Concat(parts...)
	slices.SortFunc(all, func(a, b candidate) int {
		return cmp.Or(cmp.Compare(b.score, a.score), cmp.Compare(a.row, b.row))
	})
	return all[:min(k, len(all))]
}

// dot 是两个 int8 向量的点积。1024 维时最大值约 1650 万，int32 不会溢出。
func dot(a, b []int8) int32 {
	b = b[:len(a)]
	var s0, s1, s2, s3 int32
	i := 0
	for ; i+4 <= len(a); i += 4 {
		s0 += int32(a[i]) * int32(b[i])
		s1 += int32(a[i+1]) * int32(b[i+1])
		s2 += int32(a[i+2]) * int32(b[i+2])
		s3 += int32(a[i+3]) * int32(b[i+3])
	}
	for ; i < len(a); i++ {
		s0 += int32(a[i]) * int32(b[i])
	}
	return s0 + s1 + s2 + s3
}

// minHeap 保留得分最高的若干候选，堆顶是其中最差的一个。
type minHeap []candidate

// worse 判断 a 是否比 b 差：得分低，或同分时下标大。
func worse(a, b candidate) bool {
	return a.score < b.score || (a.score == b.score && a.row > b.row)
}

func (h *minHeap) offer(c candidate, k int) {
	if len(*h) < k {
		*h = append(*h, c)
		h.up(len(*h) - 1)
		return
	}
	if worse(c, (*h)[0]) || c == (*h)[0] {
		return
	}
	(*h)[0] = c
	h.down(0)
}

func (h minHeap) up(i int) {
	for i > 0 {
		parent := (i - 1) / 2
		if !worse(h[i], h[parent]) {
			return
		}
		h[i], h[parent] = h[parent], h[i]
		i = parent
	}
}

func (h minHeap) down(i int) {
	for {
		smallest := i
		for _, child := range []int{2*i + 1, 2*i + 2} {
			if child < len(h) && worse(h[child], h[smallest]) {
				smallest = child
			}
		}
		if smallest == i {
			return
		}
		h[i], h[smallest] = h[smallest], h[i]
		i = smallest
	}
}

// fuse 用 RRF 融合关键词与向量两个排序：每首诗得分为它在各列表中 1/(rrfC + 名次) 之和。
// 同分时先出现在关键词列表里的在前，与评测脚本一致。
func fuse(keyword, semantic []int64) []int64 {
	score := make(map[int64]float64, len(keyword)+len(semantic))
	var order []int64
	for _, list := range [][]int64{keyword, semantic} {
		for rank, id := range list[:min(pool, len(list))] {
			if _, seen := score[id]; !seen {
				order = append(order, id)
			}
			score[id] += 1 / float64(rrfC+rank+1)
		}
	}
	slices.SortStableFunc(order, func(a, b int64) int { return cmp.Compare(score[b], score[a]) })
	return order
}

// ErrDisabled 表示服务没有启用语义检索，Reason 说明原因。
type ErrDisabled struct{ Reason string }

func (e *ErrDisabled) Error() string { return "semantic search is disabled: " + e.Reason }

// IsDisabled 判断 err 是否是 ErrDisabled。
func IsDisabled(err error) bool {
	var d *ErrDisabled
	return errors.As(err, &d)
}
