package graph

import (
	"context"
	"errors"
	"fmt"

	"go.uber.org/zap"

	"github.com/palemoky/chinese-poetry-api/internal/database"
	"github.com/palemoky/chinese-poetry-api/internal/logger"
	"github.com/palemoky/chinese-poetry-api/internal/semantic"
)

// semanticSearch 实现 semanticSearchPoems。放在手写文件里，gqlgen 重新生成
// schema.resolvers.go 时不会改动这里的 import 与逻辑。
func (r *Resolver) semanticSearch(ctx context.Context, query string, lang database.Lang, limit *int) ([]*database.Poem, error) {
	if r.Searcher == nil {
		return nil, errors.New("semantic search is not enabled on this server")
	}
	if query == "" {
		return nil, errors.New("query must not be empty")
	}
	n := defaultSemanticLimit
	if limit != nil {
		n = *limit
	}
	if n < 1 || n > semantic.MaxLimit {
		return nil, fmt.Errorf("limit must be between 1 and %d", semantic.MaxLimit)
	}

	repo := r.repo(ctx, lang)
	ids, err := r.Searcher.Search(ctx, repo, query, n)
	switch {
	case errors.Is(err, semantic.ErrQueryTooLong):
		return nil, err
	case errors.Is(err, semantic.ErrEmbedding):
		// 内部细节只记日志，调用方只需知道该稍后再试或改用 searchPoems
		logger.Warn("Semantic search embedding failed", zap.Error(err))
		return nil, errors.New("embedding service unavailable")
	case err != nil:
		return nil, err
	}

	poems, err := repo.PoemsByIDs(ids)
	if err != nil {
		return nil, err
	}
	out := make([]*database.Poem, len(poems))
	for i := range poems {
		out[i] = &poems[i]
	}
	return out, nil
}
