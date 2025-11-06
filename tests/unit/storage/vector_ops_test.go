package storage

import (
	"fmt"
	"math"
	"testing"

	"github.com/dshills/gocontext-mcp/internal/storage"
)

func TestSerializeDeserializeVector(t *testing.T) {
	tests := []struct {
		name   string
		vector []float32
	}{
		{
			name:   "simple vector",
			vector: []float32{1.0, 2.0, 3.0, 4.0},
		},
		{
			name:   "normalized vector",
			vector: []float32{0.5, 0.5, 0.5, 0.5},
		},
		{
			name:   "negative values",
			vector: []float32{-1.0, -0.5, 0.5, 1.0},
		},
		{
			name:   "large dimension",
			vector: make([]float32, 1024),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Serialize
			blob := storage.SerializeVector(tt.vector)

			// Check size
			expectedSize := len(tt.vector) * 4 // float32 is 4 bytes
			if len(blob) != expectedSize {
				t.Errorf("blob size = %d, expected %d", len(blob), expectedSize)
			}

			// Deserialize
			deserialized := storage.DeserializeVector(blob)

			// Check length
			if len(deserialized) != len(tt.vector) {
				t.Errorf("deserialized length = %d, expected %d", len(deserialized), len(tt.vector))
			}

			// Check values
			for i := range tt.vector {
				if math.Abs(float64(deserialized[i]-tt.vector[i])) > 1e-6 {
					t.Errorf("deserialized[%d] = %f, expected %f", i, deserialized[i], tt.vector[i])
				}
			}
		})
	}
}

func TestCosineSimilarity(t *testing.T) {
	tests := []struct {
		name     string
		a        []float32
		b        []float32
		expected float64
		delta    float64
	}{
		{
			name:     "identical vectors",
			a:        []float32{1.0, 2.0, 3.0},
			b:        []float32{1.0, 2.0, 3.0},
			expected: 1.0,
			delta:    0.0001,
		},
		{
			name:     "orthogonal vectors",
			a:        []float32{1.0, 0.0},
			b:        []float32{0.0, 1.0},
			expected: 0.0,
			delta:    0.0001,
		},
		{
			name:     "opposite vectors",
			a:        []float32{1.0, 2.0, 3.0},
			b:        []float32{-1.0, -2.0, -3.0},
			expected: -1.0,
			delta:    0.0001,
		},
		{
			name:     "normalized vectors at 45 degrees",
			a:        []float32{1.0, 0.0},
			b:        []float32{0.707, 0.707},
			expected: 0.707,
			delta:    0.01,
		},
		{
			name:     "zero vector",
			a:        []float32{0.0, 0.0, 0.0},
			b:        []float32{1.0, 2.0, 3.0},
			expected: 0.0,
			delta:    0.0001,
		},
		{
			name:     "different dimensions",
			a:        []float32{1.0, 2.0},
			b:        []float32{1.0, 2.0, 3.0},
			expected: 0.0, // Should return 0 for mismatched dimensions
			delta:    0.0001,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := storage.CosineSimilarity(tt.a, tt.b)
			diff := math.Abs(result - tt.expected)
			if diff > tt.delta {
				t.Errorf("CosineSimilarity() = %f, expected %f (delta %f)", result, tt.expected, diff)
			}
		})
	}
}

func BenchmarkCosineSimilarity(b *testing.B) {
	dimensions := []int{384, 768, 1024, 1536}

	for _, dim := range dimensions {
		b.Run(fmt.Sprintf("dim_%d", dim), func(b *testing.B) {
			// Create test vectors
			vec1 := make([]float32, dim)
			vec2 := make([]float32, dim)
			for i := range vec1 {
				vec1[i] = float32(i) / float32(dim)
				vec2[i] = float32(dim-i) / float32(dim)
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = storage.CosineSimilarity(vec1, vec2)
			}
		})
	}
}

func BenchmarkSerializeVector(b *testing.B) {
	dimensions := []int{384, 768, 1024, 1536}

	for _, dim := range dimensions {
		b.Run(fmt.Sprintf("dim_%d", dim), func(b *testing.B) {
			vec := make([]float32, dim)
			for i := range vec {
				vec[i] = float32(i) / float32(dim)
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = storage.SerializeVector(vec)
			}
		})
	}
}

func BenchmarkDeserializeVector(b *testing.B) {
	dimensions := []int{384, 768, 1024, 1536}

	for _, dim := range dimensions {
		b.Run(fmt.Sprintf("dim_%d", dim), func(b *testing.B) {
			vec := make([]float32, dim)
			for i := range vec {
				vec[i] = float32(i) / float32(dim)
			}
			blob := storage.SerializeVector(vec)

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = storage.DeserializeVector(blob)
			}
		})
	}
}
