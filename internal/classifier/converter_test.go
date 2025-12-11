package classifier

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToTraditional(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"simple conversion", "中国", "中國"},
		{"poetry text", "春眠不觉晓", "春眠不覺曉"},
		{"already traditional", "詩詞", "詩詞"},
		{"empty string", "", ""},
		{"mixed text", "李白的诗歌很优美", "李白的詩歌很優美"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ToTraditional(tt.input)
			require.NoError(t, err, "ToTraditional should not error")
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestToSimplified(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"simple conversion", "中國", "中国"},
		{"poetry text", "春眠不覺曉", "春眠不觉晓"},
		{"already simplified", "诗词", "诗词"},
		{"empty string", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ToSimplified(tt.input)
			require.NoError(t, err, "ToSimplified should not error")
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestToTraditionalArray(t *testing.T) {
	tests := []struct {
		name  string
		input []string
		want  []string
	}{
		{"convert array", []string{"中国", "诗歌"}, []string{"中國", "詩歌"}},
		{"empty array", []string{}, []string{}},
		{"single element", []string{"春眠不觉晓"}, []string{"春眠不覺曉"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ToTraditionalArray(tt.input)
			require.NoError(t, err, "ToTraditionalArray should not error")
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestToSimplifiedArray(t *testing.T) {
	tests := []struct {
		name  string
		input []string
		want  []string
	}{
		{
			name:  "convert array",
			input: []string{"中國", "詩歌"},
			want:  []string{"中国", "诗歌"},
		},
		{
			name:  "empty array",
			input: []string{},
			want:  []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ToSimplifiedArray(tt.input)
			require.NoError(t, err, "ToSimplifiedArray should not error")
			assert.Equal(t, tt.want, got)
		})
	}
}
