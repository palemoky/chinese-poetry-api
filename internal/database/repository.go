package database

import (
	"database/sql"
	"encoding/json"
	"fmt"
)

// Repository handles database operations
type Repository struct {
	db *DB
}

// NewRepository creates a new repository
func NewRepository(db *DB) *Repository {
	return &Repository{db: db}
}

// GetOrCreateDynasty gets or creates a dynasty by name
func (r *Repository) GetOrCreateDynasty(name string) (int64, error) {
	var id int64
	err := r.db.QueryRow(`SELECT id FROM dynasties WHERE name = ?`, name).Scan(&id)
	if err == nil {
		return id, nil
	}

	if err != sql.ErrNoRows {
		return 0, err
	}

	// Dynasty doesn't exist, create it
	result, err := r.db.Exec(`INSERT INTO dynasties (name) VALUES (?)`, name)
	if err != nil {
		return 0, err
	}

	return result.LastInsertId()
}

// GetOrCreateAuthor gets or creates an author
func (r *Repository) GetOrCreateAuthor(name, namePinyin, namePinyinAbbr string, dynastyID int64) (int64, error) {
	var id int64
	err := r.db.QueryRow(
		`SELECT id FROM authors WHERE name = ? AND dynasty_id = ?`,
		name, dynastyID,
	).Scan(&id)

	if err == nil {
		return id, nil
	}

	if err != sql.ErrNoRows {
		return 0, err
	}

	// Author doesn't exist, create it
	result, err := r.db.Exec(
		`INSERT INTO authors (name, name_pinyin, name_pinyin_abbr, dynasty_id) VALUES (?, ?, ?, ?)`,
		name, namePinyin, namePinyinAbbr, dynastyID,
	)
	if err != nil {
		return 0, err
	}

	return result.LastInsertId()
}

// GetPoetryTypeID gets the ID of a poetry type by name
func (r *Repository) GetPoetryTypeID(name string) (int64, error) {
	var id int64
	err := r.db.QueryRow(`SELECT id FROM poetry_types WHERE name = ?`, name).Scan(&id)
	return id, err
}

// InsertPoem inserts a poem into the database
func (r *Repository) InsertPoem(poem *Poem) error {
	// Convert paragraphs to JSON
	contentJSON, err := json.Marshal(poem.Content)
	if err != nil {
		return fmt.Errorf("failed to marshal content: %w", err)
	}

	_, err = r.db.Exec(`
		INSERT INTO poems (
			id, title, title_pinyin, title_pinyin_abbr,
			author_id, dynasty_id, type_id,
			content, rhythmic, rhythmic_pinyin
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		poem.ID,
		poem.Title,
		poem.TitlePinyin,
		poem.TitlePinyinAbbr,
		poem.AuthorID,
		poem.DynastyID,
		poem.TypeID,
		string(contentJSON),
		poem.Rhythmic,
		poem.RhythmicPinyin,
	)

	return err
}

// GetPoemByID retrieves a poem by ID
func (r *Repository) GetPoemByID(id string) (*PoemWithRelations, error) {
	var poem PoemWithRelations
	var contentJSON string
	var author Author
	var dynasty Dynasty
	var poetryType PoetryType

	err := r.db.QueryRow(`
		SELECT 
			p.id, p.title, p.title_pinyin, p.title_pinyin_abbr,
			p.content, p.rhythmic, p.rhythmic_pinyin, p.created_at,
			a.id, a.name, a.name_pinyin, a.name_pinyin_abbr, a.created_at,
			d.id, d.name, d.name_en, d.start_year, d.end_year, d.created_at,
			t.id, t.name, t.category, t.lines, t.chars_per_line, t.created_at
		FROM poems p
		LEFT JOIN authors a ON p.author_id = a.id
		LEFT JOIN dynasties d ON p.dynasty_id = d.id
		LEFT JOIN poetry_types t ON p.type_id = t.id
		WHERE p.id = ?
	`, id).Scan(
		&poem.ID, &poem.Title, &poem.TitlePinyin, &poem.TitlePinyinAbbr,
		&contentJSON, &poem.Rhythmic, &poem.RhythmicPinyin, &poem.CreatedAt,
		&author.ID, &author.Name, &author.NamePinyin, &author.NamePinyinAbbr, &author.CreatedAt,
		&dynasty.ID, &dynasty.Name, &dynasty.NameEn, &dynasty.StartYear, &dynasty.EndYear, &dynasty.CreatedAt,
		&poetryType.ID, &poetryType.Name, &poetryType.Category, &poetryType.Lines, &poetryType.CharsPerLine, &poetryType.CreatedAt,
	)

	if err != nil {
		return nil, err
	}

	// Parse content JSON
	if err := json.Unmarshal([]byte(contentJSON), &poem.Content); err != nil {
		return nil, fmt.Errorf("failed to unmarshal content: %w", err)
	}

	poem.Author = &author
	poem.Dynasty = &dynasty
	poem.Type = &poetryType

	return &poem, nil
}

// CountPoems returns the total number of poems
func (r *Repository) CountPoems() (int, error) {
	var count int
	err := r.db.QueryRow(`SELECT COUNT(*) FROM poems`).Scan(&count)
	return count, err
}

// CountAuthors returns the total number of authors
func (r *Repository) CountAuthors() (int, error) {
	var count int
	err := r.db.QueryRow(`SELECT COUNT(*) FROM authors`).Scan(&count)
	return count, err
}

// GetStatistics returns overall statistics
func (r *Repository) GetStatistics() (*Statistics, error) {
	stats := &Statistics{}

	// Total counts
	var err error
	stats.TotalPoems, err = r.CountPoems()
	if err != nil {
		return nil, err
	}

	stats.TotalAuthors, err = r.CountAuthors()
	if err != nil {
		return nil, err
	}

	err = r.db.QueryRow(`SELECT COUNT(*) FROM dynasties WHERE name != '其他'`).Scan(&stats.TotalDynasties)
	if err != nil {
		return nil, err
	}

	// Poems by dynasty
	rows, err := r.db.Query(`
		SELECT d.id, d.name, d.name_en, d.start_year, d.end_year, COUNT(p.id) as count
		FROM dynasties d
		LEFT JOIN poems p ON d.id = p.dynasty_id
		GROUP BY d.id
		ORDER BY count DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var ds DynastyWithStats
		err := rows.Scan(
			&ds.ID, &ds.Name, &ds.NameEn, &ds.StartYear, &ds.EndYear, &ds.PoemCount,
		)
		if err != nil {
			return nil, err
		}
		stats.PoemsByDynasty = append(stats.PoemsByDynasty, ds)
	}

	// Poems by type
	rows, err = r.db.Query(`
		SELECT t.id, t.name, t.category, t.lines, t.chars_per_line, COUNT(p.id) as count
		FROM poetry_types t
		LEFT JOIN poems p ON t.id = p.type_id
		GROUP BY t.id
		ORDER BY count DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var ts PoetryTypeWithStats
		err := rows.Scan(
			&ts.ID, &ts.Name, &ts.Category, &ts.Lines, &ts.CharsPerLine, &ts.PoemCount,
		)
		if err != nil {
			return nil, err
		}
		stats.PoemsByType = append(stats.PoemsByType, ts)
	}

	return stats, nil
}
