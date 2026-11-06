package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/palemoky/chinese-poetry-api/internal/database"
)

func writeFixture(t *testing.T, path string, v any) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	data, err := json.Marshal(v)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0o644))
}

// writePoetryData 按 poetry-data 的目录结构写一份极小的语料：
// 一个繁体书写的全唐诗目录、一个宋词目录，外加作者小传与三部选本。
func writePoetryData(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	writeFixture(t, filepath.Join(root, "全唐诗", "poet.tang.0.json"), []map[string]any{
		{"title": "靜夜思", "author": "李白", "paragraphs": []string{"牀前明月光，疑是地上霜。", "舉頭望明月，低頭思故鄉。"}},
		{"title": "登鸛雀樓", "author": "王之渙", "paragraphs": []string{"白日依山盡，黃河入海流。", "欲窮千里目，更上一層樓。"}},
	})
	writeFixture(t, filepath.Join(root, "全唐诗", "authors.tang.json"), []map[string]any{
		{"name": "李白", "desc": "字太白。"},
	})
	writeFixture(t, filepath.Join(root, "宋词", "ci.song.0.json"), []map[string]any{
		{"rhythmic": "水调歌头", "author": "苏轼", "paragraphs": []string{"明月几时有？把酒问青天。"}},
	})

	// 选本：唐诗三百首、千家诗放在蒙学目录，宋词三百首放在宋词目录（不当作诗词加载）
	writeFixture(t, filepath.Join(root, "蒙学", "tangshisanbaishou.json"), map[string]any{
		"content": []map[string]any{{"title": "静夜思", "paragraphs": []string{"床前明月光，疑是地上霜。", "举头望明月，低头思故乡。"}}},
	})
	writeFixture(t, filepath.Join(root, "蒙学", "qianjiashi.json"), map[string]any{
		"content": []map[string]any{{"title": "登鹳雀楼", "paragraphs": []string{"白日依山尽，黄河入海流。", "欲穷千里目，更上一层楼。"}}},
	})
	writeFixture(t, filepath.Join(root, "宋词", "宋词三百首.json"), []map[string]any{
		{"rhythmic": "水调歌头", "paragraphs": []string{"明月几时有？把酒问青天。"}},
	})

	writeFixture(t, filepath.Join(root, "loader", "datas.json"), map[string]any{
		"cp_path": "./",
		"datasets": map[string]any{
			"tangsong": map[string]any{
				"name": "全唐诗全宋诗", "id": 1, "path": "全唐诗/", "tag": "paragraphs",
				"excludes": []string{"authors.tang.json"},
			},
			"songci": map[string]any{
				"name": "宋词", "id": 2, "path": "宋词/", "tag": "paragraphs",
				"excludes": []string{"宋词三百首.json"},
			},
		},
	})
	return root
}

// setFlags 设置命令行参数对应的全局变量，测试结束后恢复。
func setFlags(t *testing.T, input, output string, poems, anthology int64) {
	t.Helper()
	old := []any{inputDir, outputDB, workers, configPath, minPoems, minAnthology}
	t.Cleanup(func() {
		inputDir, outputDB, workers = old[0].(string), old[1].(string), old[2].(int)
		configPath, minPoems, minAnthology = old[3].(string), old[4].(int64), old[5].(int64)
	})
	inputDir, outputDB, workers, configPath = input, output, 2, ""
	minPoems, minAnthology = poems, anthology
}

func TestRunBuildsVerifiedDatabase(t *testing.T) {
	root := writePoetryData(t)
	out := filepath.Join(t.TempDir(), "poetry.db")
	summary := filepath.Join(t.TempDir(), "summary.md")
	t.Setenv("GITHUB_STEP_SUMMARY", summary)
	setFlags(t, root, out, 3, 3)

	require.NoError(t, run(nil, nil))

	db, err := database.OpenReadOnly(out, 1, 1)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	for _, lang := range []database.Lang{database.LangHans, database.LangHant} {
		repo := database.NewRepositoryWithLang(db, lang)
		count, err := repo.CountPoems()
		require.NoError(t, err)
		assert.Equal(t, 3, count, lang)
	}

	// 导入后的库能直接用于检索：单字倒排索引与全文索引都已建好
	repo := database.NewRepositoryWithLang(db, database.LangHans)
	poems, total, err := repo.SearchPoems("明月", "content", 1, 10)
	require.NoError(t, err)
	assert.EqualValues(t, 2, total)
	assert.Len(t, poems, 2)

	author, err := repo.GetAuthorByName("李白", nil)
	require.NoError(t, err)
	require.NotNil(t, author.Description)
	assert.Equal(t, "字太白。", *author.Description)

	report, err := os.ReadFile(summary)
	require.NoError(t, err, "the report goes to the GitHub job summary")
	assert.Contains(t, string(report), "### Release verification")
	assert.Contains(t, string(report), "| 唐 | 2 |")
	assert.NotContains(t, string(report), "❌")
}

// 校验不通过时返回错误，发布流程据此中止。
func TestRunFailsVerificationBelowMinimum(t *testing.T) {
	root := writePoetryData(t)
	out := filepath.Join(t.TempDir(), "poetry.db")
	t.Setenv("GITHUB_STEP_SUMMARY", "")
	setFlags(t, root, out, 1000, 0)

	err := run(nil, nil)
	require.ErrorContains(t, err, "failed release verification")

	// verify 子命令对同一个库给出同样的结论，降低门槛后通过
	require.Error(t, verifyDatabase(out))
	minPoems = 3
	require.NoError(t, verifyDatabase(out))
}

func TestRunReportsLoadErrors(t *testing.T) {
	setFlags(t, t.TempDir(), filepath.Join(t.TempDir(), "poetry.db"), 1, 0)
	assert.ErrorContains(t, run(nil, nil), "failed to create loader")

	// 缺少选本文件
	root := writePoetryData(t)
	require.NoError(t, os.Remove(filepath.Join(root, "蒙学", "qianjiashi.json")))
	setFlags(t, root, filepath.Join(t.TempDir(), "poetry.db"), 1, 0)
	assert.ErrorContains(t, run(nil, nil), "failed to load anthologies")
}

func TestRenderReport(t *testing.T) {
	md := renderReport(&database.VerifyReport{
		Checks: []database.Check{
			{Name: "poem count", Passed: true, Detail: "ok"},
			{Name: "author ids aligned", Passed: false, Detail: "2 authors missing"},
		},
		Dynasties: []database.DynastyCount{{Name: "唐", Count: 2}},
	})
	assert.Contains(t, md, "| poem count | ✅ | ok |")
	assert.Contains(t, md, "| author ids aligned | ❌ | 2 authors missing |")
	assert.Contains(t, md, "| 唐 | 2 |")
}

func TestAppendFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "summary.md")
	require.NoError(t, appendFile(path, "a"))
	require.NoError(t, appendFile(path, "b"))
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "ab", string(data))

	assert.Error(t, appendFile(filepath.Join(t.TempDir(), "missing", "summary.md"), "a"))
}
