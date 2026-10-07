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
		poemBy("劉禹錫", "唐"),
		poemBy("苏轼", "宋"), // 宋词（简体）里的苏轼，应与上面归为一人
		poemBy("", "唐"),
		poemBy("陸游", "宋"), // 全唐诗（繁体）
		poemBy("陆游", "宋"), // 宋词（简体）：简转繁会得到「陸遊」，必须仍归为同一人
		poemBy("張潮", "唐"), // 唐代诗人张潮
		poemBy("张潮", "清"), // 清代《幽梦影》作者张潮：同名异人
		poemBy("", "宋"),   // 各朝代的佚名各是一条
		{PoemData: loader.PoemData{Author: "只有空正文", Paragraphs: []string{"  "}}, Dynasty: "唐"},
	}
	bios := []loader.AuthorBio{
		{Name: "蘇軾", Dynasty: "宋", Description: "蘇軾，字子瞻。"},
		{Name: "苏轼", Dynasty: "宋", Description: "苏轼，字子瞻。"}, // 同一段小传的简体版，应去重
		{Name: "苏轼", Dynasty: "宋", Description: "北宋文学家。"},
		{Name: "張潮", Dynasty: "唐", Description: "張潮，曲阿人。"},
		{Name: "无人", Dynasty: "唐", Description: "没有作品的作者不建记录。"},
	}

	scripts, err := detectSourceScripts(poems)
	require.NoError(t, err)
	hans, err := (&Processor{convertToTraditional: false}).planAuthors(poems, scripts, bios)
	require.NoError(t, err)
	hant, err := (&Processor{convertToTraditional: true}).planAuthors(poems, scripts, bios)
	require.NoError(t, err)

	type row struct {
		id                  int64
		name, dynasty, desc string
	}
	rows := func(as []plannedAuthor) []row {
		out := make([]row, len(as))
		for i, a := range as {
			out[i] = row{a.id, a.name, a.dynasty, a.description}
		}
		return out
	}
	assert.Equal(t, []row{
		{1, "苏轼", "宋", "苏轼，字子瞻。\n\n北宋文学家。"},
		{2, "刘禹锡", "唐", ""},
		{3, "佚名", "唐", ""},
		{4, "陆游", "宋", ""},
		{5, "张潮", "唐", "张潮，曲阿人。"},
		{6, "张潮", "清", ""},
		{7, "佚名", "宋", ""},
	}, rows(hans))

	require.Len(t, hant, len(hans))
	for i := range hans {
		assert.Equal(t, hans[i].id, hant[i].id, "both variants must share author IDs")
		assert.Equal(t, hans[i].identity, hant[i].identity)
	}
	assert.Equal(t, "蘇軾", hant[0].name)
	assert.Equal(t, "蘇軾，字子瞻。\n\n北宋文學家。", hant[0].description, "a traditional bio is kept, a simplified one converted")
	assert.Equal(t, "陸游", hant[3].name, "the traditional table keeps the first-seen spelling")
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
