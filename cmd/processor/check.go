package main

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"slices"

	"github.com/spf13/cobra"

	"github.com/palemoky/chinese-poetry-api/internal/database"
	"github.com/palemoky/chinese-poetry-api/internal/embedding"
	"github.com/palemoky/chinese-poetry-api/internal/vectors"
)

// minCheckCosine 是同一模型在不同运行环境下应达到的相似度。量化本身带来的误差远小于此，
// 低于它说明接口用的模型或预处理与生成向量文件时不同，查询向量与诗词向量就不可比。
const minCheckCosine = 0.99

type checkOptions struct {
	db, vectors, api, model, apiKeyEnv string
	sample                             int
}

func checkEmbeddingsCommand() *cobra.Command {
	var o checkOptions
	cmd := &cobra.Command{
		Use:   "check-embeddings",
		Short: "Check that an embeddings API produces the vectors in a vector file",
		Long: `Embed a sample of poems through an embeddings API and compare the results with
the vector file. Run it before pointing the server at an API other than the one the
file was built with: semantic search only works if both produce the same vectors.`,
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCheckEmbeddings(cmd.Context(), o)
		},
	}
	cmd.Flags().StringVar(&o.db, "db", "poetry.db", "Poetry database the vector file belongs to")
	cmd.Flags().StringVar(&o.vectors, "vectors", "poetry-vectors.db", "Vector file to compare against")
	cmd.Flags().StringVar(&o.api, "api", "http://localhost:11434/v1", "Base URL of an OpenAI-compatible embeddings API")
	cmd.Flags().StringVar(&o.model, "model", "", "Model name at that API; defaults to the model recorded in the vector file")
	cmd.Flags().StringVar(&o.apiKeyEnv, "api-key-env", "", "Name of the environment variable holding the API key, if the API needs one")
	cmd.Flags().IntVar(&o.sample, "sample", 64, "Number of poems to compare, spread evenly over the corpus")
	return cmd
}

func runCheckEmbeddings(ctx context.Context, o checkOptions) error {
	if o.sample < 1 {
		return errors.New("--sample must be positive")
	}
	apiKey, err := checkEmbedOptions(embedOptions{apiKeyEnv: o.apiKeyEnv, batch: 1})
	if err != nil {
		return err
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
	idx, err := vectors.Load(o.vectors, vectors.Fingerprint(docs))
	if err != nil {
		return err
	}

	model := o.model
	if model == "" {
		model = idx.Meta.Model
	}
	client := embedding.New(o.api, model, apiKey)

	// 均匀取样，覆盖各个朝代与体裁（诗词按数据集顺序编号）
	step := max(1, len(docs)/o.sample)
	var sample []vectors.Doc
	for i := 0; i < len(docs) && len(sample) < o.sample; i += step {
		sample = append(sample, docs[i])
	}

	texts := make([]string, len(sample))
	for i, d := range sample {
		texts[i] = d.Text
	}
	raw, err := client.Embed(ctx, texts)
	if err != nil {
		return err
	}

	worst, sum := math.Inf(1), 0.0
	worstID := int64(0)
	for i, d := range sample {
		if len(raw[i]) != idx.Meta.Dims {
			return fmt.Errorf("the API returned %d-dimensional vectors, the vector file has %d: it is a different model",
				len(raw[i]), idx.Meta.Dims)
		}
		got, err := vectors.Quantize(raw[i])
		if err != nil {
			return fmt.Errorf("poem %d: %w", d.ID, err)
		}
		row, ok := slices.BinarySearch(idx.IDs, d.ID)
		if !ok {
			return fmt.Errorf("poem %d is missing from the vector file", d.ID)
		}
		c := cosine(got, idx.Data[row*idx.Meta.Dims:(row+1)*idx.Meta.Dims])
		sum += c
		if c < worst {
			worst, worstID = c, d.ID
		}
	}

	_, _ = fmt.Fprintf(os.Stdout, "model %s at %s: %d poems, mean cosine %.4f, lowest %.4f (poem %d)\n",
		model, o.api, len(sample), sum/float64(len(sample)), worst, worstID)
	if worst < minCheckCosine {
		return fmt.Errorf("lowest cosine %.4f is below %.2f: this API does not produce the vectors in %s", worst, minCheckCosine, o.vectors)
	}
	_, _ = fmt.Fprintln(os.Stdout, "OK: queries embedded by this API can be compared with the vector file")
	return nil
}

func cosine(a, b []int8) float64 {
	var dot, na, nb float64
	for i := range a {
		x, y := float64(a[i]), float64(b[i])
		dot += x * y
		na += x * x
		nb += y * y
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / math.Sqrt(na*nb)
}
