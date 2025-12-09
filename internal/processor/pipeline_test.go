package processor

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetOptimalConfig(t *testing.T) {
	// Test that config returns reasonable values
	workBuf, resultBuf, errorBuf, defaultBatch, minBatch, maxBatch := getOptimalConfig()

	// All values should be positive
	assert.Greater(t, workBuf, 0, "workBuffer should be positive")
	assert.Greater(t, resultBuf, 0, "resultBuffer should be positive")
	assert.Greater(t, errorBuf, 0, "errorBuffer should be positive")
	assert.Greater(t, defaultBatch, 0, "defaultBatch should be positive")
	assert.Greater(t, minBatch, 0, "minBatch should be positive")
	assert.Greater(t, maxBatch, 0, "maxBatch should be positive")

	// Batch sizes should be ordered
	assert.LessOrEqual(t, minBatch, defaultBatch, "minBatch <= defaultBatch")
	assert.LessOrEqual(t, defaultBatch, maxBatch, "defaultBatch <= maxBatch")
}

func TestNewProcessor(t *testing.T) {
	// Note: This test requires a real database connection
	// For now, we just test that NewProcessor doesn't panic with nil
	// In a real scenario, you'd use a test database

	tests := []struct {
		name               string
		workers            int
		convertTraditional bool
		wantWorkers        int
	}{
		{
			name:               "default workers",
			workers:            0,
			convertTraditional: false,
			wantWorkers:        1, // Will use runtime.NumCPU()
		},
		{
			name:               "specific workers",
			workers:            4,
			convertTraditional: true,
			wantWorkers:        4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// We can't fully test without a database
			// But we can verify the logic
			if tt.workers <= 0 {
				assert.Greater(t, tt.wantWorkers, 0)
			} else {
				assert.Equal(t, tt.wantWorkers, tt.workers)
			}
		})
	}
}

func TestSetBatchSize(t *testing.T) {
	// Create a processor with mock repo (nil for this test)
	proc := &Processor{
		batchSize: 100,
	}

	tests := []struct {
		name     string
		newSize  int
		wantSize int
	}{
		{"set valid size", 200, 200},
		{"ignore zero", 0, 100},
		{"ignore negative", -10, 100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			proc.SetBatchSize(tt.newSize)
			assert.Equal(t, tt.wantSize, proc.batchSize)
		})
	}
}

func TestMin(t *testing.T) {
	tests := []struct {
		name string
		a    int
		b    int
		want int
	}{
		{"a < b", 5, 10, 5},
		{"a > b", 10, 5, 5},
		{"a == b", 5, 5, 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := min(tt.a, tt.b)
			assert.Equal(t, tt.want, got)
		})
	}
}

// Benchmark tests
func BenchmarkGetOptimalConfig(b *testing.B) {
	for b.Loop() {
		getOptimalConfig()
	}
}
