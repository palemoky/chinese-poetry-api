package processor

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/palemoky/chinese-poetry-api/internal/database"
	"github.com/palemoky/chinese-poetry-api/internal/loader"
)

// importedPoem 是从诗词表读回的一行，作者、体裁已连表取出名称。
type importedPoem struct {
	ID         int64
	Title      string
	Content    []string
	AuthorID   int64
	AuthorName string
	Dynasty    string
	TypeName   string
	Anthology  int
}

// importedAuthor 是从作者表读回的一行。
type importedAuthor struct {
	ID          int64
	Name        string
	Dynasty     string
	Description string
}

// openImportDB 与 cmd/processor 一样用单连接打开文件数据库并建表。
// 不能用内存库：导入时多个 worker 并发查询，连接池一开新连接，
// 内存库的新连接就是另一个空库，看不到已建好的表。
func openImportDB(t *testing.T) *database.DB {
	t.Helper()

	db, err := database.Open(filepath.Join(t.TempDir(), "poetry.db"), 1, 1)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, db.Migrate())
	return db
}

// runImport 与 cmd/processor 的 processUnifiedDatabase 一样，先后导入简体、繁体两套表。
func runImport(t *testing.T, poems []loader.PoemWithMeta, bios []loader.AuthorBio, anthologies []loader.AnthologyPoem) *database.DB {
	t.Helper()

	db := openImportDB(t)
	for _, lang := range []database.Lang{database.LangHans, database.LangHant} {
		p := NewProcessor(database.NewRepositoryWithLang(db, lang), 2, lang == database.LangHant)
		p.SetBatchSize(2) // 小于诗词数，覆盖分批写入
		p.SetAuthorBios(bios)
		require.NoError(t, p.SetAnthologies(anthologies))
		require.NoError(t, p.Process(poems), "import %s", lang)
	}
	return db
}

// readPoems 按 ID 顺序读出某个语言变体的全部诗词，以标题为键。
func readPoems(t *testing.T, db *database.DB, lang database.Lang) map[string]importedPoem {
	t.Helper()

	var rows []struct {
		ID         int64
		Title      string
		Content    string
		AuthorID   int64
		AuthorName string
		Dynasty    string
		TypeName   string
		Anthology  int
	}
	require.NoError(t, db.Table(database.PoemsTable(lang)+" AS p").
		Select("p.id, p.title, p.content, p.author_id, a.name AS author_name, d.name AS dynasty, t.name AS type_name, p.anthology").
		Joins("JOIN "+database.AuthorsTable(lang)+" AS a ON a.id = p.author_id").
		Joins("JOIN "+database.DynastiesTable(lang)+" AS d ON d.id = p.dynasty_id").
		Joins("JOIN "+database.PoetryTypesTable(lang)+" AS t ON t.id = p.type_id").
		Order("p.id").Scan(&rows).Error)

	poems := make(map[string]importedPoem, len(rows))
	for _, r := range rows {
		var content []string
		require.NoError(t, json.Unmarshal([]byte(r.Content), &content))
		_, dup := poems[r.Title]
		require.False(t, dup, "duplicate title %q in %s", r.Title, lang)
		poems[r.Title] = importedPoem{
			ID: r.ID, Title: r.Title, Content: content, AuthorID: r.AuthorID, AuthorName: r.AuthorName,
			Dynasty: r.Dynasty, TypeName: r.TypeName, Anthology: r.Anthology,
		}
	}
	return poems
}

func readAuthors(t *testing.T, db *database.DB, lang database.Lang) []importedAuthor {
	t.Helper()

	var authors []importedAuthor
	require.NoError(t, db.Table(database.AuthorsTable(lang)+" AS a").
		Select("a.id, a.name, d.name AS dynasty, COALESCE(a.description, '') AS description").
		Joins("JOIN "+database.DynastiesTable(lang)+" AS d ON d.id = a.dynasty_id").
		Order("a.id").Scan(&authors).Error)
	return authors
}

func poem(dataset, dynasty, author, title string, paragraphs ...string) loader.PoemWithMeta {
	return loader.PoemWithMeta{
		PoemData:   loader.PoemData{Title: title, Author: author, Paragraphs: paragraphs},
		Dynasty:    dynasty,
		DatasetKey: dataset,
	}
}

func ci(author, rhythmic, title string, paragraphs ...string) loader.PoemWithMeta {
	p := poem("songci", "宋", author, title, paragraphs...)
	p.Rhythmic = rhythmic
	return p
}

// importCorpus 是覆盖导入各条规则的一小份语料，简体、繁体来源混杂，与真实数据一致。
func importCorpus() ([]loader.PoemWithMeta, []loader.AuthorBio, []loader.AnthologyPoem) {
	lunyu := poem("lunyu", "先秦", "孔子", "", "学而时习之，不亦说乎？")
	lunyu.Chapter = "学而篇"

	poems := []loader.PoemWithMeta{
		poem("tang", "唐", "李白", "静夜思", "床前明月光，疑是地上霜。", "举头望明月，低头思故乡。"),
		// 同一作者、同一正文，只是断句与标点不同：去重后只留第一首
		poem("tang", "唐", "李白", "静夜思（重出）", "床前明月光", "疑是地上霜", "举头望明月", "低头思故乡"),
		// 不同作者的同文作品（重出诗）各自保留
		poem("tang", "唐", "王维", "静夜思（王维）", "床前明月光，疑是地上霜。", "举头望明月，低头思故乡。"),
		// 繁体书写的全唐诗：简体表转为简体，繁体表保留原文（不能把「十里」再转成「十裏」）
		poem("tang", "唐", "杜牧", "贈別", "春風十里揚州路，卷上珠簾總不如。"),
		// 没有作者的归入佚名
		poem("tang", "唐", "", "无名氏诗", "白日依山尽，黄河入海流。", "欲穷千里目，更上一层楼。"),
		// 只有被跳过作品的作者不应建出来
		poem("tang", "唐", "无作品者", "空诗"),
		poem("tang", "唐", "无作品者", "占位诗", "无正文。"),
		// 词以词牌为题，有副标题时拼在后面；同一作者同一词牌的多首以首句区分
		ci("苏轼", "水调歌头", "丙辰中秋", "明月几时有？把酒问青天。"),
		ci("晏殊", "浣溪沙", "", "一曲新词酒一杯，去年天气旧亭台。"),
		ci("晏殊", "浣溪沙", "", "一向年光有限身，等闲离别易销魂。"),
		// 论语取章节名为题
		lunyu,
	}
	bios := []loader.AuthorBio{
		{Name: "李白", Dynasty: "唐", Description: "字太白，號青蓮居士。"},
		{Name: "李白", Dynasty: "唐", Description: "字太白，号青莲居士。"}, // 同一段小传的简体版，按简体判重
		{Name: "李白", Dynasty: "宋", Description: "另一位同名者"},     // 朝代不同，不挂到唐代李白
		{Name: "没有作品的人", Dynasty: "唐", Description: "不会被建出"},
	}
	anthologies := []loader.AnthologyPoem{
		{Source: "tangshi300", Paragraphs: []string{"床前明月光，疑是地上霜。", "举头望明月，低头思故乡。"}},
	}
	return poems, bios, anthologies
}

func TestProcessImportsSimplifiedTable(t *testing.T) {
	poems, bios, anthologies := importCorpus()
	db := runImport(t, poems, bios, anthologies)
	got := readPoems(t, db, database.LangHans)

	titles := make([]string, 0, len(got))
	for title := range got {
		titles = append(titles, title)
	}
	assert.ElementsMatch(t, []string{
		"静夜思", "静夜思（王维）", "赠别", "无名氏诗",
		"水调歌头·丙辰中秋", "浣溪沙·一曲新词酒一杯", "浣溪沙·一向年光有限身", "学而篇",
	}, titles)

	jys := got["静夜思"]
	assert.Equal(t, "李白", jys.AuthorName)
	assert.Equal(t, "唐", jys.Dynasty)
	assert.Equal(t, "五言绝句", jys.TypeName)
	assert.Equal(t, database.AnthologyTangShi300, jys.Anthology)
	assert.Equal(t, []string{"床前明月光，疑是地上霜。", "举头望明月，低头思故乡。"}, jys.Content)
	assert.Equal(t, int64(1), jys.ID, "poem IDs follow input order")

	assert.Equal(t, "王维", got["静夜思（王维）"].AuthorName)
	assert.Equal(t, database.AnthologyTangShi300, got["静夜思（王维）"].Anthology, "another author's copy is marked too")

	zb := got["赠别"]
	assert.Equal(t, "杜牧", zb.AuthorName)
	assert.Equal(t, []string{"春风十里扬州路，卷上珠帘总不如。"}, zb.Content)
	assert.Zero(t, zb.Anthology)

	assert.Equal(t, "佚名", got["无名氏诗"].AuthorName)
	assert.Equal(t, "宋词", got["水调歌头·丙辰中秋"].TypeName)
	assert.Equal(t, "宋", got["水调歌头·丙辰中秋"].Dynasty)
	assert.Equal(t, "论语", got["学而篇"].TypeName)
	assert.Equal(t, "先秦", got["学而篇"].Dynasty)
}

func TestProcessImportsTraditionalTable(t *testing.T) {
	poems, bios, anthologies := importCorpus()
	db := runImport(t, poems, bios, anthologies)
	got := readPoems(t, db, database.LangHant)

	jys, ok := got["靜夜思"]
	require.True(t, ok, "simplified sources are converted: %v", got)
	assert.Equal(t, []string{"牀前明月光，疑是地上霜。", "舉頭望明月，低頭思故鄉。"}, jys.Content)
	assert.Equal(t, "五言絕句", jys.TypeName)
	assert.Equal(t, database.AnthologyTangShi300, jys.Anthology, "anthology marks match in both tables")

	zb, ok := got["贈別"]
	require.True(t, ok)
	assert.Equal(t, []string{"春風十里揚州路，卷上珠簾總不如。"}, zb.Content,
		"traditional sources are kept as-is, not converted again")

	assert.Contains(t, got, "水調歌頭·丙辰中秋")
	assert.Contains(t, got, "浣溪沙·一曲新詞酒一杯")
	assert.Equal(t, "宋詞", got["水調歌頭·丙辰中秋"].TypeName)
	assert.Equal(t, "論語", got["學而篇"].TypeName)

	assert.Len(t, got, len(readPoems(t, db, database.LangHans)), "both tables keep the same poems")
}

// 简繁两表的作者 ID 与诗词 ID 一一对应，跨表关联才不会查到别人。
func TestProcessKeepsIDsAlignedAcrossVariants(t *testing.T) {
	poems, bios, anthologies := importCorpus()
	db := runImport(t, poems, bios, anthologies)

	hans, hant := readPoems(t, db, database.LangHans), readPoems(t, db, database.LangHant)
	byID := func(poems map[string]importedPoem) map[int64]int64 {
		m := make(map[int64]int64, len(poems))
		for _, p := range poems {
			m[p.ID] = p.AuthorID
		}
		return m
	}
	assert.Equal(t, byID(hans), byID(hant), "same poem IDs with the same author IDs")

	authorsHans, authorsHant := readAuthors(t, db, database.LangHans), readAuthors(t, db, database.LangHant)
	require.Len(t, authorsHant, len(authorsHans))
	for i := range authorsHans {
		assert.Equal(t, authorsHans[i].ID, authorsHant[i].ID)
	}
}

func TestProcessAttachesAuthorBios(t *testing.T) {
	poems, bios, anthologies := importCorpus()
	db := runImport(t, poems, bios, anthologies)

	names := func(authors []importedAuthor) []string {
		out := make([]string, len(authors))
		for i, a := range authors {
			out[i] = a.Name
		}
		return out
	}

	hans := readAuthors(t, db, database.LangHans)
	assert.Equal(t, []string{"李白", "王维", "杜牧", "佚名", "苏轼", "晏殊", "孔子"}, names(hans),
		"authors are numbered in order of first appearance; ones with only skipped poems are left out")
	assert.Equal(t, "字太白，号青莲居士。", hans[0].Description,
		"bios are converted, and the same bio in both scripts is kept once")
	assert.Empty(t, hans[1].Description)

	hant := readAuthors(t, db, database.LangHant)
	assert.Equal(t, "字太白，號青蓮居士。", hant[0].Description)
	assert.Equal(t, "杜牧", hant[2].Name)
	assert.Equal(t, "蘇軾", hant[4].Name)
}

func TestProcessEmptyCorpus(t *testing.T) {
	db := openImportDB(t)
	p := NewProcessor(database.NewRepository(db), 1, false)
	require.NoError(t, p.Process([]loader.PoemWithMeta{poem("tang", "唐", "李白", "空诗")}))

	var count int64
	require.NoError(t, db.Table(database.PoemsTable(database.LangHans)).Count(&count).Error)
	assert.Zero(t, count)
}

func TestResolveTitleByCategory(t *testing.T) {
	tests := []struct {
		name     string
		poem     loader.PoemData
		category string
		want     string
	}{
		{"ci with subtitle", loader.PoemData{Title: "丙辰中秋", Rhythmic: "水调歌头"}, "宋词", "水调歌头·丙辰中秋"},
		{"ci title same as rhythmic", loader.PoemData{Title: "浣溪沙", Rhythmic: "浣溪沙"}, "词", "浣溪沙"},
		{"ci without rhythmic", loader.PoemData{Title: "无题"}, "词", "无题"},
		{"lunyu chapter", loader.PoemData{Title: "论语", Chapter: "学而篇"}, "论语", "学而篇"},
		{"mengzi without chapter", loader.PoemData{Title: "孟子"}, "四书五经", "孟子"},
		{"shi ignores rhythmic", loader.PoemData{Title: "静夜思", Rhythmic: "x"}, "唐诗", "静夜思"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, resolveTitleByCategory(tt.poem, tt.category))
		})
	}
}
