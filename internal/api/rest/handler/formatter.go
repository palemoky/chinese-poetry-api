package handler

import "github.com/palemoky/chinese-poetry-api/internal/database"

// formatDynasty 把朝代整理成 API 响应结构，不含 created_at。
func formatDynasty(d *database.Dynasty) map[string]any {
	result := map[string]any{
		"id":   d.ID,
		"name": d.Name,
	}
	if d.NameEn != nil {
		result["name_en"] = *d.NameEn
	}
	if d.StartYear != nil {
		result["start_year"] = *d.StartYear
	}
	if d.EndYear != nil {
		result["end_year"] = *d.EndYear
	}
	return result
}

// formatDynastyWithStats 在朝代基础上附加统计数据。
func formatDynastyWithStats(d *database.DynastyWithStats) map[string]any {
	result := formatDynasty(&d.Dynasty)
	result["poem_count"] = d.PoemCount
	result["author_count"] = d.AuthorCount
	return result
}

// formatAuthor 把作者整理成 API 响应结构，不含 created_at。
func formatAuthor(a *database.Author) map[string]any {
	result := map[string]any{
		"id":   a.ID,
		"name": a.Name,
	}
	if a.Dynasty != nil {
		result["dynasty"] = a.Dynasty.Name
	}
	return result
}

// formatAuthorDetail 在作者基础上附加小传，只用于作者详情接口：
// 小传动辄数百字，放进列表和每首诗内嵌的作者信息里会让响应体积成倍增长。
func formatAuthorDetail(a *database.Author) map[string]any {
	result := formatAuthor(a)
	if a.Description != nil && *a.Description != "" {
		result["description"] = *a.Description
	}
	return result
}

// formatAuthorWithStats 在作者基础上附加统计数据。
func formatAuthorWithStats(a *database.AuthorWithStats) map[string]any {
	result := formatAuthor(&a.Author)
	result["poem_count"] = a.PoemCount
	return result
}

// formatPoetryType 把体裁整理成 API 响应结构，不含 created_at。
func formatPoetryType(t *database.PoetryType) map[string]any {
	result := map[string]any{
		"id":       t.ID,
		"name":     t.Name,
		"category": t.Category,
	}
	if t.Lines != nil {
		result["lines"] = *t.Lines
	}
	if t.CharsPerLine != nil {
		result["chars_per_line"] = *t.CharsPerLine
	}
	if t.Description != nil {
		result["description"] = *t.Description
	}
	return result
}

// formatPoetryTypeWithStats 在体裁基础上附加统计数据。
func formatPoetryTypeWithStats(t *database.PoetryTypeWithStats) map[string]any {
	result := formatPoetryType(&t.PoetryType)
	result["poem_count"] = t.PoemCount
	return result
}

// formatPoem 把诗词整理成 API 响应结构，关联对象以嵌套形式呈现。
func formatPoem(poem *database.Poem) map[string]any {
	var typeData map[string]any
	if poem.Type != nil {
		typeData = map[string]any{
			"id":       poem.Type.ID,
			"name":     poem.Type.Name,
			"category": poem.Type.Category,
		}
		if poem.Type.Description != nil {
			typeData["description"] = *poem.Type.Description
		}
	}

	var authorData map[string]any
	if poem.Author != nil {
		authorData = map[string]any{
			"id":   poem.Author.ID,
			"name": poem.Author.Name,
		}
	}

	var dynastyData map[string]any
	if poem.Dynasty != nil {
		dynastyData = formatDynasty(poem.Dynasty)
	}

	return map[string]any{
		"id":      poem.ID,
		"type":    typeData,
		"title":   poem.Title,
		"content": poem.Content,
		"author":  authorData,
		"dynasty": dynastyData,
	}
}
