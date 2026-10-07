package processor

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/palemoky/chinese-poetry-api/internal/classifier"
	"github.com/palemoky/chinese-poetry-api/internal/loader"
)

// poemBy 构造一首诗，正文的字形跟随作者名（繁体名配繁体正文），
// 与源数据中全唐诗、宋词各自的书写方式一致。
func poemBy(author, dynasty string) loader.PoemWithMeta {
	content := "牀前明月光，疑是地上霜。舉頭望明月，低頭思故鄉。"
	if simp, _ := classifier.ToSimplified(author); simp == author {
		content = "床前明月光，疑是地上霜。举头望明月，低头思故乡。"
	}
	return loader.PoemWithMeta{
		PoemData: loader.PoemData{Author: author, Paragraphs: []string{content}},
		Dynasty:  dynasty,
	}
}

func TestPlanAuthors(t *testing.T) {
	poems := []loader.PoemWithMeta{
		poemBy("蘇軾", "宋"), // 全唐诗（繁体）里的苏轼
		poemBy("劉禹錫", "宋"),
		poemBy("劉禹錫", "唐"),
		poemBy("苏轼", "宋"), // 宋词（简体）里的苏轼，应与上面归为一人
		poemBy("劉禹錫", "唐"),
		poemBy("", "唐"),
		poemBy("陸游", "宋"), // 全唐诗（繁体）
		poemBy("陆游", "宋"), // 宋词（简体）：简转繁会得到「陸遊」，必须仍归为同一人
		{PoemData: loader.PoemData{Author: "只有空正文", Paragraphs: []string{"  "}}, Dynasty: "唐"},
	}

	scripts, err := detectSourceScripts(poems)
	require.NoError(t, err)
	hans, err := (&Processor{convertToTraditional: false}).planAuthors(poems, scripts)
	require.NoError(t, err)
	hant, err := (&Processor{convertToTraditional: true}).planAuthors(poems, scripts)
	require.NoError(t, err)

	assert.Equal(t, []plannedAuthor{
		{id: 1, name: "苏轼", dynasty: "宋"},
		// 首次出现在宋诗文件里，但唐代作品更多，判为唐
		{id: 2, name: "刘禹锡", dynasty: "唐"},
		{id: 3, name: "佚名", dynasty: "唐"},
		{id: 4, name: "陆游", dynasty: "宋"},
	}, ignoreIdentity(hans), "the author with only skipped poems gets no record")

	require.Len(t, hant, len(hans))
	for i := range hans {
		assert.Equal(t, hans[i].id, hant[i].id, "both variants must share author IDs")
		assert.Equal(t, hans[i].dynasty, hant[i].dynasty)
	}
	assert.Equal(t, "蘇軾", hant[0].name)
	assert.Equal(t, "劉禹錫", hant[1].name)
	assert.Equal(t, "陸游", hant[3].name, "the traditional table keeps the first-seen spelling")
	assert.Equal(t, "陆游", hant[3].identity)
}

func ignoreIdentity(authors []plannedAuthor) []plannedAuthor {
	out := make([]plannedAuthor, len(authors))
	for i, a := range authors {
		a.identity = ""
		out[i] = a
	}
	return out
}

// 作者数相同时以先出现的朝代为准，结果不随运行而变。
func TestPlanAuthorsDynastyTieKeepsFirst(t *testing.T) {
	poems := []loader.PoemWithMeta{poemBy("某甲", "五代"), poemBy("某甲", "宋")}
	for range 5 {
		got, err := (&Processor{}).planAuthors(poems, make([]bool, len(poems)))
		require.NoError(t, err)
		assert.Equal(t, "五代", got[0].dynasty)
	}
}

// 繁体源文本生成繁体库时必须原样保留：简转繁会改掉简繁同形的字。
func TestToVariantKeepsSourceScript(t *testing.T) {
	hant := &Processor{convertToTraditional: true}
	hans := &Processor{convertToTraditional: false}

	for _, src := range []string{"陸游", "十里芰荷香", "咸陽", "子云"} {
		got, err := hant.toVariant(src, true)
		require.NoError(t, err)
		assert.Equal(t, src, got, "traditional source must not be re-converted")
	}

	got, err := hant.toVariant("陆游", false)
	require.NoError(t, err)
	assert.NotEqual(t, "陆游", got, "simplified sources are still converted")

	got, err = hans.toVariant("陸游", true)
	require.NoError(t, err)
	assert.Equal(t, "陆游", got)
}

func TestDetectSourceScripts(t *testing.T) {
	got, err := detectSourceScripts([]loader.PoemWithMeta{
		poemBy("蘇軾", "宋"), poemBy("苏轼", "宋"),
	})
	require.NoError(t, err)
	assert.Equal(t, []bool{true, false}, got)
}
