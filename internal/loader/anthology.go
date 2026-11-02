package loader

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// AnthologySource 是一个选本文件，Key 用来对应数据库里的选本标记。
type AnthologySource struct {
	Key  string
	Path string // 相对于数据根目录
}

// AnthologySources 是用来标记名篇的选本。
//
// 它们不作为诗词导入：蒙学下的文件结构与其他数据集不同，也不在 datas.json 里；
// 《宋词三百首》虽然随宋词目录导入，重复的会在去重时去掉。这里只读出正文，
// 用来在全集里找到对应的诗。
var AnthologySources = []AnthologySource{
	{Key: "tangshi300", Path: "蒙学/tangshisanbaishou.json"},
	{Key: "qianjiashi", Path: "蒙学/qianjiashi.json"},
	{Key: "songci300", Path: "宋词/宋词三百首.json"},
}

// AnthologyPoem 是选本中的一首诗，只保留匹配需要的正文。
type AnthologyPoem struct {
	Source     string // AnthologySource.Key
	Paragraphs []string
}

// LoadAnthologies 读取全部选本中的诗词。选本按体裁层层嵌套，
// 凡是带 paragraphs 的对象都算一首。
func (l *JSONLoader) LoadAnthologies() ([]AnthologyPoem, error) {
	var poems []AnthologyPoem
	for _, src := range AnthologySources {
		path := filepath.Join(l.basePath, src.Path)
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("failed to read anthology %s: %w", path, err)
		}
		var root any
		if err := json.Unmarshal(data, &root); err != nil {
			return nil, fmt.Errorf("failed to parse anthology %s: %w", path, err)
		}

		n := len(poems)
		walkPoems(root, func(paragraphs []string) {
			poems = append(poems, AnthologyPoem{Source: src.Key, Paragraphs: paragraphs})
		})
		if len(poems) == n {
			return nil, fmt.Errorf("anthology %s contains no poems", path)
		}
	}
	return poems, nil
}

// walkPoems 对 node 中每首诗的正文调用 visit。
//
// paragraphs 是字符串数组的对象算一首诗。组诗（如《千家诗》里杜甫的“其一”“其二”）
// 的 paragraphs 是一组带各自 paragraphs 的对象，每一首单独算，所以要继续往下找。
func walkPoems(node any, visit func([]string)) {
	switch v := node.(type) {
	case map[string]any:
		if paragraphs := getStringArray(v, "paragraphs"); len(paragraphs) > 0 {
			visit(paragraphs)
			return
		}
		for _, child := range v {
			walkPoems(child, visit)
		}
	case []any:
		for _, child := range v {
			walkPoems(child, visit)
		}
	}
}
