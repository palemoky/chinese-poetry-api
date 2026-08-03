package database

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPoemCursorRoundTrip(t *testing.T) {
	for _, id := range []int64{0, 1, 300000, 1 << 40} {
		cursor := EncodePoemCursor(id)
		decoded, err := DecodePoemCursor(cursor)
		require.NoError(t, err)
		assert.Equal(t, id, decoded)
	}
}

func TestDecodePoemCursorRejectsGarbage(t *testing.T) {
	tests := []struct {
		name   string
		cursor string
	}{
		{"empty", ""},
		{"not base64", "!!!"},
		// 客户端最容易犯的错：把游标当成可以自己构造的行号
		{"a bare number", "12"},
		{"base64 of a bare number", encodeCursor("", 12)},
		// 另一类游标误传到诗词接口，前缀能挡住
		{"wrong prefix", encodeCursor("author", 12)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := DecodePoemCursor(tt.cursor)
			assert.Error(t, err)
		})
	}
}
