package loader

import (
	"cmp"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// DataConfig 对应 datas.json 的结构。
type DataConfig struct {
	CPPath   string                 `json:"cp_path"`
	Datasets map[string]DatasetInfo `json:"datasets"`
}

// DatasetInfo 描述单个数据集的配置。
type DatasetInfo struct {
	Name     string   `json:"name"`
	ID       int      `json:"id"`
	Path     string   `json:"path"`
	Tag      string   `json:"tag"`
	Excludes []string `json:"excludes"`
	Comments string   `json:"comments,omitempty"`
}

// PoemData 是 JSON 文件中的一首诗词。
type PoemData struct {
	ID         string   `json:"id"`
	Title      string   `json:"title"`
	Chapter    string   `json:"chapter,omitempty"` // 用于论语、四书五经
	Author     string   `json:"author"`
	Paragraphs []string `json:"paragraphs"`
	Rhythmic   string   `json:"rhythmic,omitempty"` // 词牌名，用于词
	Content    string   `json:"content,omitempty"`  // 正文的备用字段
	Para       []string `json:"para,omitempty"`     // 正文的备用字段
}

// JSONLoader 从 JSON 文件中加载诗词数据。
type JSONLoader struct {
	config      *DataConfig
	basePath    string
	idToDynasty map[int]string
}

// NewJSONLoader 读取配置文件并创建加载器。
func NewJSONLoader(configPath string) (*JSONLoader, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config DataConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// 确定数据文件的根目录
	configDir := filepath.Dir(configPath)
	var basePath string

	// cp_path 指定了非当前目录时，与配置文件所在目录拼接
	if config.CPPath != "" && config.CPPath != "./" && config.CPPath != "." {
		basePath = filepath.Join(configDir, config.CPPath)
	} else {
		// cp_path 为 "./" 或 "." 时，根目录取 loader 目录的上一级
		basePath = filepath.Dir(configDir)
	}

	// 建立数据集 ID 到朝代的映射
	idToDynasty := make(map[int]string)
	for key, dataset := range config.Datasets {
		idToDynasty[dataset.ID] = inferDynasty(key, dataset.Name)
	}

	return &JSONLoader{
		config:      &config,
		basePath:    basePath,
		idToDynasty: idToDynasty,
	}, nil
}

// LoadAll 加载全部数据集中的诗词数据。
//
// 数据集按 ID 顺序加载，而不是遍历 map：诗词 ID 按加载顺序分配（见 processor），
// map 的遍历顺序每次运行都不同，此前同一份数据每次发布都会得到一套不同的诗词 ID，
// 外部保存的 /poems/:id 链接随之失效。
func (l *JSONLoader) LoadAll() ([]PoemWithMeta, error) {
	var allPoems []PoemWithMeta

	keys := make([]string, 0, len(l.config.Datasets))
	for key := range l.config.Datasets {
		keys = append(keys, key)
	}
	slices.SortFunc(keys, func(a, b string) int {
		return cmp.Or(cmp.Compare(l.config.Datasets[a].ID, l.config.Datasets[b].ID), cmp.Compare(a, b))
	})

	for _, key := range keys {
		dataset := l.config.Datasets[key]
		poems, err := l.loadDataset(key, dataset)
		if err != nil {
			return nil, fmt.Errorf("failed to load dataset %s: %w", key, err)
		}
		allPoems = append(allPoems, poems...)
	}

	return allPoems, nil
}

// PoemWithMeta 是诗词数据加上其来源信息。
type PoemWithMeta struct {
	PoemData
	Dynasty     string
	DatasetName string
	DatasetKey  string
}

func (l *JSONLoader) loadDataset(key string, dataset DatasetInfo) ([]PoemWithMeta, error) {
	fullPath := filepath.Join(l.basePath, dataset.Path)
	dynasty := l.idToDynasty[dataset.ID]

	info, err := os.Stat(fullPath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat path %s: %w", fullPath, err)
	}

	var poems []PoemWithMeta

	if info.IsDir() {
		// 目录：逐个加载其中的 JSON 文件
		entries, err := os.ReadDir(fullPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read directory %s: %w", fullPath, err)
		}

		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}

			// 跳过配置中排除的文件
			if contains(dataset.Excludes, entry.Name()) {
				continue
			}

			if filepath.Ext(entry.Name()) != ".json" {
				continue
			}

			filePath := filepath.Join(fullPath, entry.Name())
			filePoems, err := l.loadJSONFile(filePath, dataset.Tag)
			if err != nil {
				fmt.Printf("Warning: failed to load %s: %v\n", filePath, err)
				continue
			}

			fileDyn := fileDynasty(entry.Name(), dynasty)
			for _, poem := range filePoems {
				poemWithMeta := PoemWithMeta{
					PoemData:    poem,
					Dynasty:     fileDyn,
					DatasetName: dataset.Name,
					DatasetKey:  key,
				}

				// 数据中没有作者时填入该数据集的默认作者
				if poemWithMeta.Author == "" {
					if defaultAuthor := getDefaultAuthorFromDataset(key); defaultAuthor != "" {
						poemWithMeta.Author = defaultAuthor
					}
				}

				poems = append(poems, poemWithMeta)
			}
		}
	} else {
		// 单个文件：直接加载
		filePoems, err := l.loadJSONFile(fullPath, dataset.Tag)
		if err != nil {
			return nil, err
		}

		for _, poem := range filePoems {
			poemWithMeta := PoemWithMeta{
				PoemData:    poem,
				Dynasty:     dynasty,
				DatasetName: dataset.Name,
				DatasetKey:  key,
			}

			// 数据中没有作者时填入该数据集的默认作者
			if poemWithMeta.Author == "" {
				if defaultAuthor := getDefaultAuthorFromDataset(key); defaultAuthor != "" {
					poemWithMeta.Author = defaultAuthor
				}
			}

			poems = append(poems, poemWithMeta)
		}
	}

	return poems, nil
}

func (l *JSONLoader) loadJSONFile(path string, tag string) ([]PoemData, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	var rawPoems []map[string]any
	if err := json.Unmarshal(data, &rawPoems); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	var poems []PoemData
	for _, raw := range rawPoems {
		poem := PoemData{
			Title:  getString(raw, "title"),
			Author: getString(raw, "author"),
		}

		// ID 字段
		if id, ok := raw["id"].(string); ok {
			poem.ID = id
		}

		// 词牌名，用于词
		if rhythmic, ok := raw["rhythmic"].(string); ok {
			poem.Rhythmic = rhythmic
		}

		// 章节名，用于论语、四书五经
		if chapter, ok := raw["chapter"].(string); ok {
			poem.Chapter = chapter
		}

		// 按配置的 tag 提取正文
		switch tag {
		case "paragraphs":
			poem.Paragraphs = getStringArray(raw, "paragraphs")
		case "content":
			if content, ok := raw["content"].(string); ok {
				poem.Content = content
				poem.Paragraphs = []string{content}
			} else {
				poem.Paragraphs = getStringArray(raw, "content")
			}
		case "para":
			poem.Paragraphs = getStringArray(raw, "para")
		default:
			// 未指定则依次尝试各个可能的字段
			if paras := getStringArray(raw, "paragraphs"); len(paras) > 0 {
				poem.Paragraphs = paras
			} else if paras := getStringArray(raw, "para"); len(paras) > 0 {
				poem.Paragraphs = paras
			} else if content, ok := raw["content"].(string); ok {
				poem.Paragraphs = []string{content}
			}
		}

		if len(poem.Paragraphs) > 0 {
			poems = append(poems, poem)
		}
	}

	return poems, nil
}

func inferDynasty(key, name string) string {
	// 数据集 key 到朝代的映射
	dynastyMap := map[string]string{
		"tangsong":          "唐",
		"songci":            "宋",
		"yuanqu":            "元",
		"wudai-huajianji":   "五代",
		"wudai-nantang":     "五代",
		"yudingquantangshi": "唐",
		"shuimotangshi":     "唐",
		"shijing":           "先秦",
		"chuci":             "先秦",
		"lunyu":             "先秦",
		"mengzi":            "先秦",
		"caocao":            "魏晋",
		"nalanxingde":       "清",
		"youmengying":       "清", // 张潮《幽梦影》
	}

	if dynasty, ok := dynastyMap[key]; ok {
		return dynasty
	}

	// 映射表中没有则尝试从名称推断
	if contains([]string{"唐"}, name) {
		return "唐"
	}
	if contains([]string{"宋"}, name) {
		return "宋"
	}
	if contains([]string{"元"}, name) {
		return "元"
	}

	return "其他"
}

// fileDynasty 依据文件名修正数据集级别的朝代。
//
// 「全唐诗」目录（数据集 tangsong，「全唐诗全宋诗」）同时收了唐诗与宋诗，
// 两者只靠文件名区分：poet.tang.N.json 与 poet.song.N.json。按数据集整体
// 判为唐时，约 25 万首宋诗都被标成了唐诗。
func fileDynasty(fileName, datasetDynasty string) string {
	switch {
	case strings.Contains(fileName, ".song."):
		return "宋"
	case strings.Contains(fileName, ".tang."):
		return "唐"
	default:
		return datasetDynasty
	}
}

// getDefaultAuthorFromDataset 返回数据集的默认作者，用于数据中缺少 author 字段的情况。
func getDefaultAuthorFromDataset(datasetKey string) string {
	authorMap := map[string]string{
		"caocao":      "曹操",
		"nalanxingde": "纳兰性德",
	}

	if author, ok := authorMap[datasetKey]; ok {
		return author
	}
	return ""
}

func getString(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func getStringArray(m map[string]any, key string) []string {
	if arr, ok := m[key].([]any); ok {
		result := make([]string, 0, len(arr))
		for _, item := range arr {
			if str, ok := item.(string); ok {
				result = append(result, str)
			}
		}
		return result
	}
	return nil
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
