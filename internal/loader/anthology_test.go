package loader

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadAnthologies(t *testing.T) {
	root := t.TempDir()
	// 蒙学下的选本按体裁嵌套
	writeJSON(t, filepath.Join(root, "蒙学", "tangshisanbaishou.json"), map[string]any{
		"title": "唐詩三百首",
		"content": []any{
			map[string]any{"type": "五言絕句", "content": []any{
				map[string]any{"chapter": "登鸛雀樓", "author": "王之渙", "paragraphs": []string{"白日依山盡，黃河入海流。"}},
				map[string]any{"chapter": "缺正文", "author": "某"},
			}},
		},
	})
	writeJSON(t, filepath.Join(root, "蒙学", "qianjiashi.json"), map[string]any{
		"content": []any{map[string]any{"type": "七言絕句", "content": []any{
			map[string]any{"chapter": "春日", "author": "（宋）朱熹", "paragraphs": []string{"勝日尋芳泗水濱，無邊光景一時新。"}},
			// 组诗：每一首各有 paragraphs
			map[string]any{"chapter": "攜妓納涼晚際遇雨", "author": "（唐）杜甫", "paragraphs": []any{
				map[string]any{"subchapter": "其一", "paragraphs": []string{"落日放船好，輕風生浪遲。"}},
				map[string]any{"subchapter": "其二", "paragraphs": []string{"雨來霑席上，風急打船頭。"}},
			}},
		}}},
	})
	// 宋词三百首是扁平的列表
	writeJSON(t, filepath.Join(root, "宋词", "宋词三百首.json"), []map[string]any{
		{"author": "苏轼", "rhythmic": "水调歌头", "paragraphs": []string{"明月几时有？把酒问青天。"}},
	})
	writeJSON(t, filepath.Join(root, "loader", "datas.json"), map[string]any{"cp_path": "./", "datasets": map[string]any{}})

	l, err := NewJSONLoader(filepath.Join(root, "loader", "datas.json"))
	require.NoError(t, err)
	poems, err := l.LoadAnthologies()
	require.NoError(t, err)

	assert.Equal(t, []AnthologyPoem{
		{Source: "tangshi300", Paragraphs: []string{"白日依山盡，黃河入海流。"}},
		{Source: "qianjiashi", Paragraphs: []string{"勝日尋芳泗水濱，無邊光景一時新。"}},
		{Source: "qianjiashi", Paragraphs: []string{"落日放船好，輕風生浪遲。"}},
		{Source: "qianjiashi", Paragraphs: []string{"雨來霑席上，風急打船頭。"}},
		{Source: "songci300", Paragraphs: []string{"明月几时有？把酒问青天。"}},
	}, poems, "entries without paragraphs are skipped")
}

func TestLoadAnthologiesFailsWhenFileMissingOrEmpty(t *testing.T) {
	root := t.TempDir()
	writeJSON(t, filepath.Join(root, "loader", "datas.json"), map[string]any{"cp_path": "./", "datasets": map[string]any{}})
	l, err := NewJSONLoader(filepath.Join(root, "loader", "datas.json"))
	require.NoError(t, err)

	_, err = l.LoadAnthologies()
	assert.Error(t, err, "a missing anthology must fail the build instead of silently tagging nothing")

	writeJSON(t, filepath.Join(root, "蒙学", "tangshisanbaishou.json"), map[string]any{"content": []any{}})
	_, err = l.LoadAnthologies()
	assert.ErrorContains(t, err, "contains no poems")
}
