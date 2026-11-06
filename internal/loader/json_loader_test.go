package loader

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFileDynasty(t *testing.T) {
	assert.Equal(t, "宋", fileDynasty("poet.song.12000.json", "唐"))
	assert.Equal(t, "唐", fileDynasty("poet.tang.0.json", "唐"))
	assert.Equal(t, "宋", fileDynasty("ci.song.0.json", "宋"))
	assert.Equal(t, "唐", fileDynasty("唐诗三百首.json", "唐"), "files without a dynasty token keep the dataset's")
}

// writeJSON 把 v 写成 JSON 文件，目录不存在时一并创建。
func writeJSON(t *testing.T, path string, v any) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	data, err := json.Marshal(v)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0o644))
}

func TestLoadAllAssignsDynastyPerFileInStableOrder(t *testing.T) {
	root := t.TempDir()
	poem := func(title string) []map[string]any {
		return []map[string]any{{"title": title, "author": "某", "paragraphs": []string{"一二三四五。"}}}
	}
	writeJSON(t, filepath.Join(root, "全唐诗", "poet.song.0.json"), poem("宋诗"))
	writeJSON(t, filepath.Join(root, "全唐诗", "poet.tang.0.json"), poem("唐诗"))
	writeJSON(t, filepath.Join(root, "元曲", "yuanqu.json"), poem("元曲"))
	writeJSON(t, filepath.Join(root, "宋词", "ci.song.0.json"), poem("宋词"))

	// 数据集 ID 的顺序与 map 键的字典序、书写顺序都不同，以确认按 ID 排序
	writeJSON(t, filepath.Join(root, "loader", "datas.json"), map[string]any{
		"cp_path": "./",
		"datasets": map[string]any{
			"songci":   map[string]any{"name": "宋词", "id": 5, "path": "宋词/", "tag": "paragraphs"},
			"tangsong": map[string]any{"name": "全唐诗全宋诗", "id": 3, "path": "全唐诗/", "tag": "paragraphs"},
			"yuanqu":   map[string]any{"name": "元曲", "id": 2, "path": "元曲/yuanqu.json", "tag": "paragraphs"},
		},
	})

	l, err := NewJSONLoader(filepath.Join(root, "loader", "datas.json"))
	require.NoError(t, err)

	for range 5 { // map 遍历顺序随机，多跑几次
		poems, err := l.LoadAll()
		require.NoError(t, err)

		var got [][2]string
		for _, p := range poems {
			got = append(got, [2]string{p.Title, p.Dynasty})
		}
		assert.Equal(t, [][2]string{
			{"元曲", "元"},
			{"宋诗", "宋"}, // 全唐诗目录下的 poet.song.* 不再被判为唐
			{"唐诗", "唐"},
			{"宋词", "宋"},
		}, got)
	}
}

func TestLoadAuthorBios(t *testing.T) {
	root := t.TempDir()
	poem := []map[string]any{{"title": "t", "author": "某", "paragraphs": []string{"一二三。"}}}
	writeJSON(t, filepath.Join(root, "全唐诗", "poet.tang.0.json"), poem)
	writeJSON(t, filepath.Join(root, "全唐诗", "authors.tang.json"), []map[string]any{{"name": "李白", "desc": "李白，字太白。"}})
	writeJSON(t, filepath.Join(root, "全唐诗", "authors.song.json"), []map[string]any{{"name": "蘇軾", "desc": "蘇軾，字子瞻。"}, {"name": "無傳", "desc": ""}})
	writeJSON(t, filepath.Join(root, "全唐诗", "error", "authors.tang.json"), []map[string]any{{"name": "错", "desc": "子目录不读"}})
	writeJSON(t, filepath.Join(root, "宋词", "ci.song.0.json"), poem)
	writeJSON(t, filepath.Join(root, "宋词", "author.song.json"), []map[string]any{{"name": "苏轼", "description": "北宋文学家。", "short_description": "略"}})
	writeJSON(t, filepath.Join(root, "五代诗词", "nantang", "poetrys.json"), poem)
	writeJSON(t, filepath.Join(root, "五代诗词", "nantang", "authors.json"), []map[string]any{{"name": "李璟", "desc": "南唐元宗。"}})
	writeJSON(t, filepath.Join(root, "loader", "datas.json"), map[string]any{
		"cp_path": "./",
		"datasets": map[string]any{
			"wudai-nantang": map[string]any{"name": "五代-南唐", "id": 1, "path": "五代诗词/nantang/poetrys.json", "tag": "paragraphs"},
			"tangsong":      map[string]any{"name": "全唐诗全宋诗", "id": 3, "path": "全唐诗/", "tag": "paragraphs"},
			"songci":        map[string]any{"name": "宋词", "id": 5, "path": "宋词/", "tag": "paragraphs"},
		},
	})

	l, err := NewJSONLoader(filepath.Join(root, "loader", "datas.json"))
	require.NoError(t, err)
	bios, err := l.LoadAuthorBios()
	require.NoError(t, err)

	assert.Equal(t, []AuthorBio{
		{Name: "李璟", Description: "南唐元宗。", Dynasty: "五代"},
		{Name: "蘇軾", Description: "蘇軾，字子瞻。", Dynasty: "宋"}, // 全唐诗目录下的 authors.song.json 属于宋
		{Name: "李白", Description: "李白，字太白。", Dynasty: "唐"},
		{Name: "苏轼", Description: "北宋文学家。", Dynasty: "宋"},
	}, bios, "empty bios and subdirectories are skipped")
}

func TestInferDynasty(t *testing.T) {
	tests := []struct {
		key, name, want string
	}{
		{"songci", "宋词", "宋"},
		{"tangsong", "全唐诗全宋诗", "唐"},
		{"youmengying", "幽梦影-张潮文集", "清"},
		{"caocao", "曹操诗集", "魏晋"},
		// 映射表之外的数据集按名称推断
		{"quantangshibubian", "全唐诗补编", "唐"},
		{"songshi", "全宋诗", "宋"},
		{"yuanzaju", "元杂剧选", "元"},
		{"unknown", "唐", "唐"},
		{"unknown", "明清小品", "其他"},
		{"unknown", "", "其他"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, inferDynasty(tt.key, tt.name), "%s/%s", tt.key, tt.name)
	}
}

// 各数据集的正文字段名不同（paragraphs、content、para），有的还没有作者字段。
func TestLoadAllReadsEveryContentLayout(t *testing.T) {
	root := t.TempDir()
	writeJSON(t, filepath.Join(root, "宋词", "ci.song.0.json"), []map[string]any{
		{"id": "c1", "title": "", "rhythmic": "浣溪沙", "author": "晏殊", "paragraphs": []string{"一曲新词酒一杯。"}},
		{"title": "无正文", "author": "某", "paragraphs": []string{}}, // 没有正文的条目丢弃
	})
	// 目录型数据集中排除的文件与非 JSON 文件不加载，解析失败的文件跳过
	writeJSON(t, filepath.Join(root, "宋词", "author.song.json"), []map[string]any{
		{"name": "晏殊", "description": "不是诗词", "paragraphs": []string{"不应出现"}},
	})
	require.NoError(t, os.WriteFile(filepath.Join(root, "宋词", "README.md"), []byte("x"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "宋词", "broken.json"), []byte("{"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "宋词", "subdir"), 0o755))

	writeJSON(t, filepath.Join(root, "论语", "lunyu.json"), []map[string]any{
		{"chapter": "学而篇", "paragraphs": []string{"学而时习之，不亦说乎？"}},
	})
	writeJSON(t, filepath.Join(root, "诗经", "shijing.json"), []map[string]any{
		{"title": "关雎", "content": []string{"关关雎鸠，在河之洲。"}},
	})
	writeJSON(t, filepath.Join(root, "幽梦影", "youmengying.json"), []map[string]any{
		{"content": "读经宜冬，其神专也。", "comment": []string{"评语"}},
	})
	writeJSON(t, filepath.Join(root, "纳兰性德", "nalan.json"), []map[string]any{
		{"title": "长相思", "para": []string{"山一程，水一程。"}},
	})
	// 未指定 tag 时依次尝试 paragraphs、para、content
	writeJSON(t, filepath.Join(root, "曹操诗集", "caocao.json"), []map[string]any{
		{"title": "观沧海", "paragraphs": []string{"东临碣石，以观沧海。"}},
		{"title": "短歌行", "para": []string{"对酒当歌，人生几何！"}},
		{"title": "龟虽寿", "content": "神龟虽寿，犹有竟时。"},
		{"title": "无正文"},
	})

	writeJSON(t, filepath.Join(root, "loader", "datas.json"), map[string]any{
		"cp_path": "./",
		"datasets": map[string]any{
			"songci":      map[string]any{"name": "宋词", "id": 1, "path": "宋词/", "tag": "paragraphs", "excludes": []string{"author.song.json"}},
			"lunyu":       map[string]any{"name": "论语", "id": 2, "path": "论语/lunyu.json", "tag": "paragraphs"},
			"shijing":     map[string]any{"name": "诗经", "id": 3, "path": "诗经/shijing.json", "tag": "content"},
			"youmengying": map[string]any{"name": "幽梦影", "id": 4, "path": "幽梦影/youmengying.json", "tag": "content"},
			"nalanxingde": map[string]any{"name": "纳兰性德", "id": 5, "path": "纳兰性德/nalan.json", "tag": "para"},
			"caocao":      map[string]any{"name": "曹操诗集", "id": 6, "path": "曹操诗集/caocao.json"},
		},
	})

	l, err := NewJSONLoader(filepath.Join(root, "loader", "datas.json"))
	require.NoError(t, err)
	poems, err := l.LoadAll()
	require.NoError(t, err)

	type row struct{ ID, Title, Chapter, Rhythmic, Author, Dynasty, Key, First string }
	var got []row
	for _, p := range poems {
		require.NotEmpty(t, p.Paragraphs, p.Title)
		got = append(got, row{p.ID, p.Title, p.Chapter, p.Rhythmic, p.Author, p.Dynasty, p.DatasetKey, p.Paragraphs[0]})
	}
	assert.Equal(t, []row{
		{"c1", "", "", "浣溪沙", "晏殊", "宋", "songci", "一曲新词酒一杯。"},
		{"", "", "学而篇", "", "", "先秦", "lunyu", "学而时习之，不亦说乎？"},
		{"", "关雎", "", "", "", "先秦", "shijing", "关关雎鸠，在河之洲。"},
		{"", "", "", "", "张潮", "清", "youmengying", "读经宜冬，其神专也。"}, // 没有作者字段时填数据集的默认作者
		{"", "长相思", "", "", "纳兰性德", "清", "nalanxingde", "山一程，水一程。"},
		{"", "观沧海", "", "", "曹操", "魏晋", "caocao", "东临碣石，以观沧海。"},
		{"", "短歌行", "", "", "曹操", "魏晋", "caocao", "对酒当歌，人生几何！"},
		{"", "龟虽寿", "", "", "曹操", "魏晋", "caocao", "神龟虽寿，犹有竟时。"},
	}, got)
}

func TestLoadAllFailsOnMissingDatasetFile(t *testing.T) {
	root := t.TempDir()
	writeJSON(t, filepath.Join(root, "loader", "datas.json"), map[string]any{
		"cp_path": "./",
		"datasets": map[string]any{
			"yuanqu": map[string]any{"name": "元曲", "id": 1, "path": "元曲/yuanqu.json", "tag": "paragraphs"},
		},
	})
	l, err := NewJSONLoader(filepath.Join(root, "loader", "datas.json"))
	require.NoError(t, err)

	_, err = l.LoadAll()
	assert.ErrorContains(t, err, "yuanqu")
}

func TestNewJSONLoaderErrors(t *testing.T) {
	_, err := NewJSONLoader(filepath.Join(t.TempDir(), "missing.json"))
	require.Error(t, err)

	bad := filepath.Join(t.TempDir(), "datas.json")
	require.NoError(t, os.WriteFile(bad, []byte("not json"), 0o644))
	_, err = NewJSONLoader(bad)
	require.Error(t, err)
}
