package loader

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// DataConfig represents the structure of datas.json
type DataConfig struct {
	CPPath   string                 `json:"cp_path"`
	Datasets map[string]DatasetInfo `json:"datasets"`
}

// DatasetInfo contains information about a dataset
type DatasetInfo struct {
	Name     string   `json:"name"`
	ID       int      `json:"id"`
	Path     string   `json:"path"`
	Tag      string   `json:"tag"`
	Excludes []string `json:"excludes"`
	Comments string   `json:"comments,omitempty"`
}

// PoemData represents a poem from JSON
type PoemData struct {
	ID         string   `json:"id"`
	Title      string   `json:"title"`
	Author     string   `json:"author"`
	Paragraphs []string `json:"paragraphs"`
	Rhythmic   string   `json:"rhythmic,omitempty"` // For ci (词)
	Content    string   `json:"content,omitempty"`  // Alternative field
	Para       []string `json:"para,omitempty"`     // Alternative field
}

// JSONLoader loads poetry data from JSON files
type JSONLoader struct {
	config      *DataConfig
	basePath    string
	idToDynasty map[int]string
}

// NewJSONLoader creates a new JSON loader
func NewJSONLoader(configPath string) (*JSONLoader, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config DataConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// Determine base path
	configDir := filepath.Dir(configPath)
	basePath := configDir

	// If cp_path is specified and not current directory, join it
	if config.CPPath != "" && config.CPPath != "./" && config.CPPath != "." {
		basePath = filepath.Join(configDir, config.CPPath)
	} else {
		// cp_path is "./" or ".", so base path is parent of loader directory
		basePath = filepath.Dir(configDir)
	}

	// Build ID to dynasty mapping
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

// LoadAll loads all poetry data from all datasets
func (l *JSONLoader) LoadAll() ([]PoemWithMeta, error) {
	var allPoems []PoemWithMeta

	for key, dataset := range l.config.Datasets {
		poems, err := l.loadDataset(key, dataset)
		if err != nil {
			return nil, fmt.Errorf("failed to load dataset %s: %w", key, err)
		}
		allPoems = append(allPoems, poems...)
	}

	return allPoems, nil
}

// PoemWithMeta includes metadata about the poem's source
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
		// Load from directory
		entries, err := os.ReadDir(fullPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read directory %s: %w", fullPath, err)
		}

		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}

			// Check if file should be excluded
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

			for _, poem := range filePoems {
				poems = append(poems, PoemWithMeta{
					PoemData:    poem,
					Dynasty:     dynasty,
					DatasetName: dataset.Name,
					DatasetKey:  key,
				})
			}
		}
	} else {
		// Load single file
		filePoems, err := l.loadJSONFile(fullPath, dataset.Tag)
		if err != nil {
			return nil, err
		}

		for _, poem := range filePoems {
			poems = append(poems, PoemWithMeta{
				PoemData:    poem,
				Dynasty:     dynasty,
				DatasetName: dataset.Name,
				DatasetKey:  key,
			})
		}
	}

	return poems, nil
}

func (l *JSONLoader) loadJSONFile(path string, tag string) ([]PoemData, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	var rawPoems []map[string]interface{}
	if err := json.Unmarshal(data, &rawPoems); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	var poems []PoemData
	for _, raw := range rawPoems {
		poem := PoemData{
			Title:  getString(raw, "title"),
			Author: getString(raw, "author"),
		}

		// Handle ID field
		if id, ok := raw["id"].(string); ok {
			poem.ID = id
		}

		// Handle rhythmic (for ci/词)
		if rhythmic, ok := raw["rhythmic"].(string); ok {
			poem.Rhythmic = rhythmic
		}

		// Extract paragraphs based on tag
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
			// Try all possible fields
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
	// Map dataset keys to dynasties
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
	}

	if dynasty, ok := dynastyMap[key]; ok {
		return dynasty
	}

	// Try to infer from name
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

func getString(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func getStringArray(m map[string]interface{}, key string) []string {
	if arr, ok := m[key].([]interface{}); ok {
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
