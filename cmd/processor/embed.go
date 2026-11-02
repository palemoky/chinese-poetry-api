package main

import (
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
	cmd.Flags().StringVar(&o.model, "model", "bge-m3", "Embedding model; it must be the model queries are embedded with")
	cmd.Flags().StringVar(&o.apiKeyEnv, "api-key-env", "", "Name of the environment variable holding the API key, if the API needs one")
	cmd.Flags().IntVar(&o.batch, "batch", 64, "Poems per embedding request")
	return cmd
}

func runEmbed(ctx context.Context, o embedOptions) error {
	if o.batch < 1 {
		return errors.New("--batch must be positive")
	}
	apiKey := ""
	if o.apiKeyEnv != "" {
		if apiKey = os.Getenv(o.apiKeyEnv); apiKey == "" {
			return fmt.Errorf("environment variable %s is empty", o.apiKeyEnv)
		}
	}

	db, err := database.OpenReadOnly(o.db, 1, 1)
	if err != nil {
		return err
	}
	docs, err := vectors.LoadCorpus(db)
	_ = db.Close()
	if err != nil {
		return err
	}
	if len(docs) == 0 {
		return fmt.Errorf("%s has no poems", o.db)
	}

	want := vectors.Meta{
		FormatVersion: vectors.FormatVersion,
		TextVersion:   vectors.TextVersion,
		Model:         o.model,
		Quantization:  vectors.QuantizationInt8,
		Fingerprint:   vectors.Fingerprint(docs),
		PoemCount:     len(docs),
	}

	out, err := vectors.Create(o.out)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()

	// 续写只接受为同一份主库、同一个模型生成的文件，否则从头生成会更安全
	switch meta, err := out.ReadMeta(); {
	case errors.Is(err, vectors.ErrNoMeta):
	case err != nil:
		return err
	case meta.Fingerprint != want.Fingerprint || meta.Model != want.Model || meta.TextVersion != want.TextVersion:
		return fmt.Errorf("%s was built for a different database or model; remove it or choose another --output", o.out)
	default:
		want.Dims = meta.Dims
	}
	if err := out.WriteMeta(want); err != nil {
		return err
	}

	done, err := out.Hashes()
	if err != nil {
		return err
	}

	var reuse *vectors.Reuse
	if o.reuse != "" {
		if reuse, err = vectors.OpenReuse(o.reuse, want); err != nil {
			return err
		}
		defer func() { _ = reuse.Close() }()
	}

	client := embedding.New(o.api, o.model, apiKey)
	var stats struct{ resumed, reused, embedded int }
	var pending []vectors.Doc
	var copied []vectors.Entry
	start := time.Now()
	lastLog := start

	flush := func() error {
		if len(copied) > 0 {
			if err := out.Put(copied); err != nil {
				return err
			}
			copied = copied[:0]
		}
		if len(pending) == 0 {
			return nil
		}
		texts := make([]string, len(pending))
		for i, d := range pending {
			texts[i] = d.Text
		}
		raw, err := client.Embed(ctx, texts)
		if err != nil {
			return err
		}
		entries := make([]vectors.Entry, len(pending))
		for i, d := range pending {
			if want.Dims == 0 {
				want.Dims = len(raw[i])
				if err := out.WriteMeta(want); err != nil {
					return err
				}
			}
			if len(raw[i]) != want.Dims {
				return fmt.Errorf("poem %d embedded to %d dimensions, want %d", d.ID, len(raw[i]), want.Dims)
			}
			q, err := vectors.Quantize(raw[i])
			if err != nil {
				return fmt.Errorf("poem %d: %w", d.ID, err)
			}
			entries[i] = vectors.Entry{ID: d.ID, Hash: d.Hash, Vector: q}
		}
		if err := out.Put(entries); err != nil {
			return err
		}
		stats.embedded += len(pending)
		pending = pending[:0]

		if time.Since(lastLog) > 30*time.Second {
			lastLog = time.Now()
			total := stats.resumed + stats.reused + stats.embedded
			logger.Info("Embedding progress", zap.Int("done", total), zap.Int("total", len(docs)),
				zap.Int("embedded", stats.embedded), zap.Duration("elapsed", time.Since(start).Round(time.Second)))
		}
		return nil
	}

	for _, d := range docs {
		if h, ok := done[d.ID]; ok && h == d.Hash {
			stats.resumed++
			continue
		}
		if reuse != nil {
			v, ok, err := reuse.Lookup(d.Hash)
			if err != nil {
				return err
			}
			if ok && (want.Dims == 0 || len(v) == want.Dims) {
				if want.Dims == 0 {
					want.Dims = len(v)
					if err := out.WriteMeta(want); err != nil {
						return err
					}
				}
				copied = append(copied, vectors.Entry{ID: d.ID, Hash: d.Hash, Vector: v})
				stats.reused++
				if len(copied) >= 1000 {
					if err := flush(); err != nil {
						return err
					}
				}
				continue
			}
		}
		pending = append(pending, d)
		if len(pending) >= o.batch {
			if err := flush(); err != nil {
				return err
			}
		}
	}
	if err := flush(); err != nil {
		return err
	}

	// 文件里不应有主库已经没有的 ID（续写一个旧文件时可能残留），数量必须正好相等
	count, err := out.Count()
	if err != nil {
		return err
	}
	if count != len(docs) {
		return fmt.Errorf("%s has %d vectors for %d poems; remove it and run again", o.out, count, len(docs))
	}
	want.Complete = true
	if err := out.WriteMeta(want); err != nil {
		return err
	}
	if err := out.Compact(); err != nil {
		return fmt.Errorf("failed to compact %s: %w", o.out, err)
	}

	logger.Info("Vector file complete",
		zap.String("file", o.out), zap.Int("poems", len(docs)), zap.Int("dims", want.Dims),
		zap.Int("resumed", stats.resumed), zap.Int("reused", stats.reused), zap.Int("embedded", stats.embedded),
		zap.Duration("elapsed", time.Since(start).Round(time.Second)))
	return nil
}
