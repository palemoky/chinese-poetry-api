package main

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/palemoky/chinese-poetry-api/internal/database"
	"github.com/palemoky/chinese-poetry-api/internal/embedding"
	"github.com/palemoky/chinese-poetry-api/internal/logger"
	"github.com/palemoky/chinese-poetry-api/internal/vectors"
)

type embedOptions struct {
	db        string
	out       string
	reuse     string
	api       string
	model     string
	apiModel  string
	apiKeyEnv string
	batch     int
}

func embedCommand() *cobra.Command {
	var o embedOptions
	cmd := &cobra.Command{
		Use:   "embed",
		Short: "Build the optional vector file for semantic search",
		Long: `Embed every poem of a finished database into a vector file (poetry-vectors.db).

Poems whose text is unchanged since --reuse are copied from it instead of being
embedded again, so a release only embeds what changed. An interrupted run resumes
where it stopped when started again with the same --output.`,
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt)
			defer stop()
			err := runEmbed(ctx, o)
			if ctx.Err() != nil {
				return fmt.Errorf("interrupted; finished batches are saved in %s, run the same command again to resume", o.out)
			}
			return err
		},
	}
	cmd.Flags().StringVar(&o.db, "db", "poetry.db", "Finished poetry database")
	cmd.Flags().StringVarP(&o.out, "output", "o", "poetry-vectors.db", "Vector file to create or resume")
	cmd.Flags().StringVar(&o.reuse, "reuse", "", "Vector file of the previous release to copy unchanged poems from")
	cmd.Flags().StringVar(&o.api, "api", "http://localhost:11434/v1", "Base URL of an OpenAI-compatible embeddings API")
	cmd.Flags().StringVar(&o.model, "model", "bge-m3", "Embedding model recorded in the file; reuse requires the same name, and queries must use the same model")
	cmd.Flags().StringVar(&o.apiModel, "api-model", "", "Name of that model at --api if it differs, e.g. Pro/BAAI/bge-m3; defaults to --model")
	cmd.Flags().StringVar(&o.apiKeyEnv, "api-key-env", "", "Name of the environment variable holding the API key, if the API needs one")
	cmd.Flags().IntVar(&o.batch, "batch", 64, "Poems per embedding request")
	return cmd
}

func runEmbed(ctx context.Context, o embedOptions) error {
	apiKey, err := checkEmbedOptions(o)
	if err != nil {
		return err
	}
	docs, err := loadDocs(o.db)
	if err != nil {
		return err
	}

	r := &embedRun{
		ctx:    ctx,
		opts:   o,
		docs:   docs,
		client: newEmbedClient(o.api, cmp.Or(o.apiModel, o.model), apiKey),
		start:  time.Now(),
		want: vectors.Meta{
			FormatVersion: vectors.FormatVersion,
			TextVersion:   vectors.TextVersion,
			Model:         o.model,
			Quantization:  vectors.QuantizationInt8,
			Fingerprint:   vectors.Fingerprint(docs),
			PoemCount:     len(docs),
		},
	}
	r.lastLog = r.start
	defer r.close()

	done, err := r.openOutput()
	if err != nil {
		return err
	}
	if o.reuse != "" {
		if r.reuse, err = vectors.OpenReuse(o.reuse, r.want); err != nil {
			return err
		}
	}

	for _, d := range docs {
		if h, ok := done[d.ID]; ok && h == d.Hash {
			r.stats.resumed++
			continue
		}
		if err := r.add(d); err != nil {
			return err
		}
	}
	if err := r.flush(); err != nil {
		return err
	}
	return r.finish()
}

// newEmbedClient 返回生成向量文件用的客户端。全量生成要跑几个小时，云服务按分钟限流，
// 这里比服务端查询有耐心得多：遇到限流最多等十来分钟，而不是几秒后就退出。
func newEmbedClient(api, model, apiKey string) *embedding.Client {
	c := embedding.New(api, model, apiKey)
	c.Retries = 10
	c.MaxBackoff = 2 * time.Minute
	return c
}

func checkEmbedOptions(o embedOptions) (apiKey string, err error) {
	if o.batch < 1 {
		return "", errors.New("--batch must be positive")
	}
	if o.apiKeyEnv == "" {
		return "", nil
	}
	if apiKey = os.Getenv(o.apiKeyEnv); apiKey == "" {
		return "", fmt.Errorf("environment variable %s is empty", o.apiKeyEnv)
	}
	return apiKey, nil
}

func loadDocs(path string) ([]vectors.Doc, error) {
	db, err := database.OpenReadOnly(path, 1, 1)
	if err != nil {
		return nil, err
	}
	docs, err := vectors.LoadCorpus(db)
	_ = db.Close()
	if err != nil {
		return nil, err
	}
	if len(docs) == 0 {
		return nil, fmt.Errorf("%s has no poems", path)
	}
	return docs, nil
}

// embedRun 是一次 embed 的状态：已复制、待嵌入的诗攒成批写入输出文件。
type embedRun struct {
	ctx    context.Context
	opts   embedOptions
	docs   []vectors.Doc
	client *embedding.Client
	out    *vectors.File
	reuse  *vectors.Reuse
	want   vectors.Meta

	pending []vectors.Doc   // 待嵌入
	copied  []vectors.Entry // 从上一版复制来、待写入

	stats   struct{ resumed, reused, embedded int }
	start   time.Time
	lastLog time.Time
}

func (r *embedRun) close() {
	if r.reuse != nil {
		_ = r.reuse.Close()
	}
	if r.out != nil {
		_ = r.out.Close()
	}
}

// openOutput 打开或新建输出文件，返回其中已写入的向量，供续写时跳过。
// 续写只接受为同一份主库、同一个模型生成的文件，否则从头生成会更安全。
func (r *embedRun) openOutput() (map[int64]vectors.TextHash, error) {
	out, err := vectors.Create(r.opts.out)
	if err != nil {
		return nil, err
	}
	r.out = out

	switch meta, err := out.ReadMeta(); {
	case errors.Is(err, vectors.ErrNoMeta):
	case err != nil:
		return nil, err
	case meta.Fingerprint != r.want.Fingerprint || meta.Model != r.want.Model || meta.TextVersion != r.want.TextVersion:
		return nil, fmt.Errorf("%s was built for a different database or model; remove it or choose another --output", r.opts.out)
	default:
		r.want.Dims = meta.Dims
	}
	if err := out.WriteMeta(r.want); err != nil {
		return nil, err
	}
	return out.Hashes()
}

// add 安排一首诗：上一版有同样文本的向量就复制，否则排进待嵌入的批次。
func (r *embedRun) add(d vectors.Doc) error {
	if r.reuse != nil {
		v, ok, err := r.reuse.Lookup(d.Hash)
		if err != nil {
			return err
		}
		if ok && (r.want.Dims == 0 || len(v) == r.want.Dims) {
			if err := r.setDims(len(v)); err != nil {
				return err
			}
			r.copied = append(r.copied, vectors.Entry{ID: d.ID, Hash: d.Hash, Vector: v})
			r.stats.reused++
			if len(r.copied) >= 1000 {
				return r.flush()
			}
			return nil
		}
	}
	r.pending = append(r.pending, d)
	if len(r.pending) >= r.opts.batch {
		return r.flush()
	}
	return nil
}

// setDims 在第一次见到向量时记下维度。
func (r *embedRun) setDims(n int) error {
	if r.want.Dims != 0 {
		return nil
	}
	r.want.Dims = n
	return r.out.WriteMeta(r.want)
}

// flush 写入复制来的向量，并嵌入、写入待嵌入的一批。
func (r *embedRun) flush() error {
	if len(r.copied) > 0 {
		if err := r.out.Put(r.copied); err != nil {
			return err
		}
		r.copied = r.copied[:0]
	}
	if len(r.pending) == 0 {
		return nil
	}

	texts := make([]string, len(r.pending))
	for i, d := range r.pending {
		texts[i] = d.Text
	}
	raw, err := r.client.Embed(r.ctx, texts)
	if err != nil {
		return err
	}
	entries := make([]vectors.Entry, len(r.pending))
	for i, d := range r.pending {
		if entries[i], err = r.quantize(d, raw[i]); err != nil {
			return err
		}
	}
	if err := r.out.Put(entries); err != nil {
		return err
	}
	r.stats.embedded += len(r.pending)
	r.pending = r.pending[:0]
	r.logProgress()
	return nil
}

func (r *embedRun) quantize(d vectors.Doc, raw []float32) (vectors.Entry, error) {
	if err := r.setDims(len(raw)); err != nil {
		return vectors.Entry{}, err
	}
	if len(raw) != r.want.Dims {
		return vectors.Entry{}, fmt.Errorf("poem %d embedded to %d dimensions, want %d", d.ID, len(raw), r.want.Dims)
	}
	q, err := vectors.Quantize(raw)
	if err != nil {
		return vectors.Entry{}, fmt.Errorf("poem %d: %w", d.ID, err)
	}
	return vectors.Entry{ID: d.ID, Hash: d.Hash, Vector: q}, nil
}

func (r *embedRun) logProgress() {
	if time.Since(r.lastLog) <= 30*time.Second {
		return
	}
	r.lastLog = time.Now()
	total := r.stats.resumed + r.stats.reused + r.stats.embedded
	logger.Info("Embedding progress", zap.Int("done", total), zap.Int("total", len(r.docs)),
		zap.Int("embedded", r.stats.embedded), zap.Duration("elapsed", time.Since(r.start).Round(time.Second)))
}

// finish 确认每首诗都有向量后把文件标记为完整并整理。
// 文件里不应有主库已经没有的 ID（续写一个旧文件时可能残留），数量必须正好相等。
func (r *embedRun) finish() error {
	count, err := r.out.Count()
	if err != nil {
		return err
	}
	if count != len(r.docs) {
		return fmt.Errorf("%s has %d vectors for %d poems; remove it and run again", r.opts.out, count, len(r.docs))
	}
	r.want.Complete = true
	if err := r.out.WriteMeta(r.want); err != nil {
		return err
	}
	if err := r.out.Compact(); err != nil {
		return fmt.Errorf("failed to compact %s: %w", r.opts.out, err)
	}

	logger.Info("Vector file complete",
		zap.String("file", r.opts.out), zap.Int("poems", len(r.docs)), zap.Int("dims", r.want.Dims),
		zap.Int("resumed", r.stats.resumed), zap.Int("reused", r.stats.reused), zap.Int("embedded", r.stats.embedded),
		zap.Duration("elapsed", time.Since(r.start).Round(time.Second)))
	return nil
}
