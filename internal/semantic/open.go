package semantic

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/palemoky/chinese-poetry-api/internal/database"
	"github.com/palemoky/chinese-poetry-api/internal/embedding"
	"github.com/palemoky/chinese-poetry-api/internal/vectors"
)

// Config 是语义检索的配置。
type Config struct {
	// VectorsPath 是向量文件（poetry-vectors.db）的路径，文件不存在时不启用。
	VectorsPath string
	// EmbeddingURL 是 OpenAI 兼容的嵌入接口地址，如 http://localhost:11434/v1，为空时不启用。
	EmbeddingURL string
	// EmbeddingModel 是查询所用的模型名，为空时取向量文件记录的模型。
	// 必须与生成向量文件的是同一个模型，只是不同服务的叫法可能不同（如 bge-m3 与 BAAI/bge-m3）。
	EmbeddingModel string
	// APIKey 是嵌入接口的密钥，本地 Ollama 不需要。
	APIKey string
}

// queryRetries 是查询嵌入失败后的重试次数。用户在等结果，只重试一次。
const queryRetries = 1

// probeTimeout 是启动时试嵌入一条文本的时限。
const probeTimeout = 15 * time.Second

// Open 载入向量文件并连上嵌入接口。没有配置或缺少向量文件时返回 ErrDisabled；
// 向量文件与主库不匹配、接口返回的维度不对等配置错误返回其他错误。两种情况服务都应照常启动，
// 只是不提供语义检索。
//
// 接口暂时连不上不算错误：它可能比本服务启动得晚，查询时会再试。返回的 warning 说明这种情况。
func Open(ctx context.Context, cfg Config, db *database.DB) (s *Searcher, warning string, err error) {
	if cfg.EmbeddingURL == "" {
		return nil, "", &ErrDisabled{Reason: "no embedding API configured (EMBEDDING_API_URL)"}
	}
	if _, err := os.Stat(cfg.VectorsPath); errors.Is(err, os.ErrNotExist) {
		return nil, "", &ErrDisabled{Reason: "vector file not found at " + cfg.VectorsPath}
	}

	docs, err := vectors.LoadCorpus(db)
	if err != nil {
		return nil, "", err
	}
	idx, err := vectors.Load(cfg.VectorsPath, vectors.Fingerprint(docs))
	if err != nil {
		return nil, "", err
	}

	anthology, err := database.NewRepository(db).AnthologyIDs()
	if err != nil {
		return nil, "", err
	}

	model := cfg.EmbeddingModel
	if model == "" {
		model = idx.Meta.Model
	}
	client := embedding.New(cfg.EmbeddingURL, model, cfg.APIKey)
	client.Retries = queryRetries
	s = New(idx, anthology, client)

	probeCtx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	if _, err := s.embedQuery(probeCtx, "床前明月光"); err != nil {
		var dims *dimensionError
		if errors.As(err, &dims) {
			return nil, "", err
		}
		warning = fmt.Sprintf("embedding API is not reachable yet, semantic search will fail until it is: %v", err)
	}
	return s, warning, nil
}
