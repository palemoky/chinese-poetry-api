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
		{
			name:  "simple conversion",
			input: "中国",
			want:  "中國",
		},
		{
			name:  "poetry text",
			input: "春眠不觉晓",
			want:  "春眠不覺曉",
		},
		{
			name:  "already traditional",
			input: "詩詞",
			want:  "詩詞",
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
		{
			name:  "mixed text",
			input: "李白的诗歌很优美",
			want:  "李白的詩歌很優美",
		},
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
		{
			name:  "simple conversion",
			input: "中國",
			want:  "中国",
		},
		{
			name:  "poetry text",
			input: "春眠不覺曉",
			want:  "春眠不觉晓",
		},
		{
			name:  "already simplified",
			input: "诗词",
			want:  "诗词",
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
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
		{
			name:  "convert array",
			input: []string{"中国", "诗歌"},
			want:  []string{"中國", "詩歌"},
		},
		{
			name:  "empty array",
			input: []string{},
			want:  []string{},
		},
		{
			name:  "single element",
			input: []string{"春眠不觉晓"},
			want:  []string{"春眠不覺曉"},
		},
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

func TestToPinyinNoTone(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "simple Chinese",
			input: "李白",
			want:  "li bai",
		},
		{
			name:  "with spaces",
			input: "李 白",
			want:  "li bai", // Spaces are normalized
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
		{
			name:  "English text",
			input: "Hello",
			want:  "", // English returns empty string
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ToPinyinNoTone(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestToPinyinAbbr(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "simple Chinese",
			input: "李白",
			want:  "lb",
		},
		{
			name:  "longer name",
			input: "白居易",
			want:  "bjy",
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ToPinyinAbbr(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

// Benchmark tests
func BenchmarkToTraditional(b *testing.B) {
	text := "春眠不觉晓，处处闻啼鸟。夜来风雨声，花落知多少。"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ToTraditional(text)
	}
}

func BenchmarkToPinyinNoTone(b *testing.B) {
	text := "李白"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ToPinyinNoTone(text)
	}
}
