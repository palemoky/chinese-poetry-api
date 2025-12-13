package helpers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/palemoky/chinese-poetry-api/internal/database"
)

func TestParseOptionalInt64(t *testing.T) {
	tests := []struct {
		name    string
		input   *string
		want    *int64
		wantErr bool
	}{
		{
			name:  "nil input",
			input: nil,
			want:  nil,
		},
		{
			name:  "empty string",
			input: stringPtr(""),
			want:  nil,
		},
		{
			name:  "valid number",
			input: stringPtr("123"),
			want:  int64Ptr(123),
		},
		{
			name:    "invalid number",
			input:   stringPtr("abc"),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseOptionalInt64(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			if tt.want == nil {
				assert.Nil(t, got)
			} else {
				require.NotNil(t, got)
				assert.Equal(t, *tt.want, *got)
			}
		})
	}
}

func TestParseLangString(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  database.Lang
	}{
		{"simplified", "zh-Hans", database.LangHans},
		{"traditional", "zh-Hant", database.LangHant},
		{"empty defaults to simplified", "", database.LangHans},
		{"invalid defaults to simplified", "en", database.LangHans},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseLangString(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseLangPointer(t *testing.T) {
	hans := database.LangHans
	hant := database.LangHant

	tests := []struct {
		name  string
		input *database.Lang
		want  database.Lang
	}{
		{"nil defaults to simplified", nil, database.LangHans},
		{"simplified", &hans, database.LangHans},
		{"traditional", &hant, database.LangHant},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseLangPointer(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestNewPagination(t *testing.T) {
	tests := []struct {
		name         string
		page         int
		pageSize     int
		wantPage     int
		wantPageSize int
	}{
		{"valid values", 2, 50, 2, 50},
		{"page < 1 defaults to 1", 0, 20, 1, 20},
		{"pageSize < 1 defaults to 20", 1, 0, 1, 20},
		{"pageSize > 100 caps at 100", 1, 200, 1, 100},
		{"negative values", -1, -1, 1, 20},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewPagination(tt.page, tt.pageSize)
			assert.Equal(t, tt.wantPage, got.Page)
			assert.Equal(t, tt.wantPageSize, got.PageSize)
		})
	}
}

func TestPaginationOffset(t *testing.T) {
	tests := []struct {
		name       string
		page       int
		pageSize   int
		wantOffset int
	}{
		{"first page", 1, 20, 0},
		{"second page", 2, 20, 20},
		{"third page", 3, 50, 100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewPagination(tt.page, tt.pageSize)
			assert.Equal(t, tt.wantOffset, p.Offset())
		})
	}
}

// Helper functions
func stringPtr(s string) *string {
	return &s
}

func int64Ptr(i int64) *int64 {
	return &i
}
