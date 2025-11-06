package embedder

import (
	"context"
	"fmt"
	"testing"
)

func BenchmarkComputeHash(b *testing.B) {
	texts := []string{
		"short",
		"medium length text for hashing",
		"this is a longer text that represents a typical code chunk that might be embedded for semantic search in a codebase",
	}

	for _, text := range texts {
		b.Run(fmt.Sprintf("len=%d", len(text)), func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = ComputeHash(text)
			}
		})
	}
}

func BenchmarkCache(b *testing.B) {
	cache := NewCache(10000)
	emb := &Embedding{
		Vector:    make([]float32, 1024),
		Dimension: 1024,
		Provider:  ProviderJina,
		Model:     "test",
		Hash:      "test-hash",
	}

	b.Run("set", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			hash := fmt.Sprintf("hash-%d", i%1000)
			cache.Set(hash, emb)
		}
	})

	// Populate cache for get benchmark
	for i := 0; i < 1000; i++ {
		cache.Set(fmt.Sprintf("hash-%d", i), emb)
	}

	b.Run("get-hit", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			hash := fmt.Sprintf("hash-%d", i%1000)
			_, _ = cache.Get(hash)
		}
	})

	b.Run("get-miss", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			hash := fmt.Sprintf("nonexistent-%d", i)
			_, _ = cache.Get(hash)
		}
	})
}

func BenchmarkLocalProvider(b *testing.B) {
	cache := NewCache(10000)
	provider, err := NewLocalProvider(cache)
	if err != nil {
		b.Fatalf("NewLocalProvider() error = %v", err)
	}
	defer provider.Close()

	ctx := context.Background()

	b.Run("single-embedding", func(b *testing.B) {
		req := EmbeddingRequest{
			Text: "func ProcessData(input []byte) (string, error) { return string(input), nil }",
		}
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := provider.GenerateEmbedding(ctx, req)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("single-embedding-cached", func(b *testing.B) {
		req := EmbeddingRequest{
			Text: "cached function code",
		}
		// Prime cache
		_, _ = provider.GenerateEmbedding(ctx, req)

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := provider.GenerateEmbedding(ctx, req)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("batch-10", func(b *testing.B) {
		texts := make([]string, 10)
		for i := range texts {
			texts[i] = fmt.Sprintf("code chunk %d", i)
		}
		req := BatchEmbeddingRequest{Texts: texts}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := provider.GenerateBatch(ctx, req)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("batch-50", func(b *testing.B) {
		texts := make([]string, 50)
		for i := range texts {
			texts[i] = fmt.Sprintf("code chunk %d with more content", i)
		}
		req := BatchEmbeddingRequest{Texts: texts}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := provider.GenerateBatch(ctx, req)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkNormalizeVector(b *testing.B) {
	sizes := []int{128, 384, 768, 1024, 1536}

	for _, size := range sizes {
		b.Run(fmt.Sprintf("dim=%d", size), func(b *testing.B) {
			vec := make([]float32, size)
			for i := range vec {
				vec[i] = float32(i) / float32(size)
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = NormalizeVector(vec)
			}
		})
	}
}

func BenchmarkValidation(b *testing.B) {
	b.Run("validate-request", func(b *testing.B) {
		req := EmbeddingRequest{Text: "sample text"}
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = ValidateRequest(req)
		}
	})

	b.Run("validate-batch", func(b *testing.B) {
		req := BatchEmbeddingRequest{
			Texts: []string{"text1", "text2", "text3", "text4", "text5"},
		}
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = ValidateBatchRequest(req)
		}
	})
}

// BenchmarkConcurrentCache tests cache performance under concurrent load
func BenchmarkConcurrentCache(b *testing.B) {
	cache := NewCache(10000)
	emb := &Embedding{
		Vector:    make([]float32, 1024),
		Dimension: 1024,
		Provider:  ProviderJina,
		Model:     "test",
	}

	// Pre-populate
	for i := 0; i < 1000; i++ {
		cache.Set(fmt.Sprintf("hash-%d", i), emb)
	}

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			// Mix of reads and writes
			if i%3 == 0 {
				cache.Set(fmt.Sprintf("hash-%d", i%2000), emb)
			} else {
				_, _ = cache.Get(fmt.Sprintf("hash-%d", i%2000))
			}
			i++
		}
	})
}
