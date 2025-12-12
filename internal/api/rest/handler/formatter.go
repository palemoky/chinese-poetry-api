package handler

import "github.com/palemoky/chinese-poetry-api/internal/database"

// formatDynasty formats a dynasty for API response, excluding created_at.
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

// formatDynastyWithStats formats a dynasty with statistics for API response.
func formatDynastyWithStats(d *database.DynastyWithStats) map[string]any {
	result := formatDynasty(&d.Dynasty)
	result["poem_count"] = d.PoemCount
	result["author_count"] = d.AuthorCount
	return result
}

// formatAuthor formats an author for API response, excluding created_at.
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

// formatAuthorWithStats formats an author with statistics for API response.
func formatAuthorWithStats(a *database.AuthorWithStats) map[string]any {
	result := formatAuthor(&a.Author)
	result["poem_count"] = a.PoemCount
	return result
}

// formatPoetryType formats a poetry type for API response, excluding created_at.
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

// formatPoetryTypeWithStats formats a poetry type with statistics for API response.
func formatPoetryTypeWithStats(t *database.PoetryTypeWithStats) map[string]any {
	result := formatPoetryType(&t.PoetryType)
	result["poem_count"] = t.PoemCount
	return result
}

// formatPoem formats a poem for API response with nested objects.
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
