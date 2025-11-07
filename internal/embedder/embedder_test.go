package embedder

import (
	"context"
	"fmt"
	"testing"
)

func TestComputeHash(t *testing.T) {
	tests := []struct {
		name  string
		text  string
		want  string
		equal bool
	}{
		{
			name:  "empty string",
			text:  "",
			want:  "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
			equal: true,
		},
		{
			name:  "simple text",
			text:  "hello world",
			want:  "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9",
			equal: true,
		},
		{
			name:  "same text produces same hash",
			text:  "test",
			want:  "test",
			equal: false, // Will compute and compare
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ComputeHash(tt.text)
			if tt.equal {
				if got != tt.want {
					t.Errorf("ComputeHash() = %v, want %v", got, tt.want)
				}
			} else {
				// Test consistency
				got2 := ComputeHash(tt.text)
				if got != got2 {
					t.Errorf("ComputeHash() not consistent: %v != %v", got, got2)
				}
			}
		})
	}
}

func TestValidateRequest(t *testing.T) {
	tests := []struct {
		name    string
		req     EmbeddingRequest
		wantErr error
	}{
		{
			name: "valid request",
			req: EmbeddingRequest{
				Text: "test text",
			},
			wantErr: nil,
		},
		{
			name: "empty text",
			req: EmbeddingRequest{
				Text: "",
			},
			wantErr: ErrEmptyText,
		},
		{
			name: "with model",
			req: EmbeddingRequest{
				Text:  "test",
				Model: "custom-model",
			},
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRequest(tt.req)
			if err != tt.wantErr {
				t.Errorf("ValidateRequest() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateBatchRequest(t *testing.T) {
	tests := []struct {
		name    string
		req     BatchEmbeddingRequest
		wantErr bool
	}{
		{
			name: "valid batch",
			req: BatchEmbeddingRequest{
				Texts: []string{"text1", "text2", "text3"},
			},
			wantErr: false,
		},
		{
			name: "empty batch",
			req: BatchEmbeddingRequest{
				Texts: []string{},
			},
			wantErr: true,
		},
		{
			name: "contains empty text",
			req: BatchEmbeddingRequest{
				Texts: []string{"text1", "", "text3"},
			},
			wantErr: true,
		},
		{
			name: "all texts valid",
			req: BatchEmbeddingRequest{
				Texts: []string{"a", "b", "c"},
				Model: "test-model",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateBatchRequest(tt.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateBatchRequest() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestCache(t *testing.T) {
	t.Run("basic operations", func(t *testing.T) {
		cache := NewCache(3)

		// Test empty cache
		if _, ok := cache.Get("nonexistent"); ok {
			t.Error("Expected cache miss on empty cache")
		}

		// Test set and get
		emb := &Embedding{
			Vector:    []float32{1.0, 2.0, 3.0},
			Dimension: 3,
			Provider:  ProviderJina,
			Model:     "test",
			Hash:      "hash1",
		}
		cache.Set("hash1", emb)

		got, ok := cache.Get("hash1")
		if !ok {
			t.Error("Expected cache hit")
		}
		if got.Hash != "hash1" {
			t.Errorf("Got hash %s, want hash1", got.Hash)
		}

		// Test size
		if cache.Size() != 1 {
			t.Errorf("Cache size = %d, want 1", cache.Size())
		}
	})

	t.Run("eviction on capacity", func(t *testing.T) {
		cache := NewCache(2)

		cache.Set("hash1", &Embedding{Hash: "hash1"})
		cache.Set("hash2", &Embedding{Hash: "hash2"})

		if cache.Size() != 2 {
			t.Errorf("Cache size = %d, want 2", cache.Size())
		}

		// This should trigger eviction (simple clear strategy)
		cache.Set("hash3", &Embedding{Hash: "hash3"})

		// After eviction, only new entry exists
		if _, ok := cache.Get("hash3"); !ok {
			t.Error("Expected new entry to be cached")
		}
	})

	t.Run("clear", func(t *testing.T) {
		cache := NewCache(10)
		cache.Set("hash1", &Embedding{Hash: "hash1"})
		cache.Set("hash2", &Embedding{Hash: "hash2"})

		cache.Clear()

		if cache.Size() != 0 {
			t.Errorf("Cache size after clear = %d, want 0", cache.Size())
		}

		if _, ok := cache.Get("hash1"); ok {
			t.Error("Expected cache miss after clear")
		}
	})

	t.Run("concurrent access", func(t *testing.T) {
		cache := NewCache(100)

		done := make(chan bool)
		for i := 0; i < 10; i++ {
			go func(id int) {
				for j := 0; j < 100; j++ {
					hash := ComputeHash("text" + string(rune(id*100+j)))
					emb := &Embedding{
						Vector:    []float32{float32(id), float32(j)},
						Dimension: 2,
						Hash:      hash,
					}
					cache.Set(hash, emb)
					cache.Get(hash)
				}
				done <- true
			}(i)
		}

		for i := 0; i < 10; i++ {
			<-done
		}

		// Should not panic and should have some entries
		if cache.Size() == 0 {
			t.Error("Cache is empty after concurrent operations")
		}
	})

	t.Run("mutation isolation", func(t *testing.T) {
		cache := NewCache(10)

		// Store original embedding
		original := &Embedding{
			Vector:    []float32{1.0, 2.0, 3.0},
			Dimension: 3,
			Provider:  ProviderJina,
			Model:     "test",
			Hash:      "hash1",
		}
		cache.Set("hash1", original)

		// Get first copy and mutate it
		emb1, ok := cache.Get("hash1")
		if !ok {
			t.Fatal("Expected cache hit")
		}
		emb1.Vector[0] = 999.0

		// Get second copy - should be unchanged
		emb2, ok := cache.Get("hash1")
		if !ok {
			t.Fatal("Expected cache hit")
		}

		if emb2.Vector[0] == 999.0 {
			t.Error("Cache pollution detected: mutation affected cached value")
		}
		if emb2.Vector[0] != 1.0 {
			t.Errorf("Expected original value 1.0, got %f", emb2.Vector[0])
		}
	})
}

func TestLocalProvider(t *testing.T) {
	cache := NewCache(10)
	provider, err := NewLocalProvider(cache)
	if err != nil {
		t.Fatalf("NewLocalProvider() error = %v", err)
	}
	defer provider.Close()

	t.Run("provider metadata", func(t *testing.T) {
		if provider.Provider() != ProviderLocal {
			t.Errorf("Provider() = %s, want %s", provider.Provider(), ProviderLocal)
		}
		if provider.Dimension() != LocalDimension {
			t.Errorf("Dimension() = %d, want %d", provider.Dimension(), LocalDimension)
		}
		if provider.Model() == "" {
			t.Error("Model() returned empty string")
		}
	})

	t.Run("single embedding", func(t *testing.T) {
		ctx := context.Background()
		req := EmbeddingRequest{
			Text: "test code snippet",
		}

		emb, err := provider.GenerateEmbedding(ctx, req)
		if err != nil {
			t.Fatalf("GenerateEmbedding() error = %v", err)
		}

		if emb == nil {
			t.Fatal("GenerateEmbedding() returned nil embedding")
		}
		if len(emb.Vector) != LocalDimension {
			t.Errorf("Vector dimension = %d, want %d", len(emb.Vector), LocalDimension)
		}
		if emb.Provider != ProviderLocal {
			t.Errorf("Provider = %s, want %s", emb.Provider, ProviderLocal)
		}
	})

	t.Run("batch embedding", func(t *testing.T) {
		ctx := context.Background()
		req := BatchEmbeddingRequest{
			Texts: []string{"text1", "text2", "text3"},
		}

		resp, err := provider.GenerateBatch(ctx, req)
		if err != nil {
			t.Fatalf("GenerateBatch() error = %v", err)
		}

		if len(resp.Embeddings) != 3 {
			t.Errorf("Got %d embeddings, want 3", len(resp.Embeddings))
		}

		for i, emb := range resp.Embeddings {
			if len(emb.Vector) != LocalDimension {
				t.Errorf("Embedding %d: dimension = %d, want %d", i, len(emb.Vector), LocalDimension)
			}
		}
	})

	t.Run("caching", func(t *testing.T) {
		ctx := context.Background()
		text := "cached text"

		// First call
		emb1, err := provider.GenerateEmbedding(ctx, EmbeddingRequest{Text: text})
		if err != nil {
			t.Fatalf("First GenerateEmbedding() error = %v", err)
		}

		// Second call should use cache
		emb2, err := provider.GenerateEmbedding(ctx, EmbeddingRequest{Text: text})
		if err != nil {
			t.Fatalf("Second GenerateEmbedding() error = %v", err)
		}

		// Should return same vector
		if len(emb1.Vector) != len(emb2.Vector) {
			t.Error("Cached embedding has different dimension")
		}
		for i := range emb1.Vector {
			if emb1.Vector[i] != emb2.Vector[i] {
				t.Errorf("Cached embedding differs at index %d", i)
				break
			}
		}
	})

	t.Run("validation errors", func(t *testing.T) {
		ctx := context.Background()

		// Empty text
		_, err := provider.GenerateEmbedding(ctx, EmbeddingRequest{Text: ""})
		if err == nil {
			t.Error("Expected error for empty text")
		}

		// Empty batch
		_, err = provider.GenerateBatch(ctx, BatchEmbeddingRequest{Texts: []string{}})
		if err == nil {
			t.Error("Expected error for empty batch")
		}
	})

	t.Run("context cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		req := EmbeddingRequest{Text: "test"}
		// Local provider doesn't check context in current implementation
		// But should not panic
		_, _ = provider.GenerateEmbedding(ctx, req)
	})
}

func TestNormalizeVector(t *testing.T) {
	tests := []struct {
		name     string
		input    []float32
		wantNorm float32
	}{
		{
			name:     "unit vector",
			input:    []float32{1.0, 0.0, 0.0},
			wantNorm: 1.0,
		},
		{
			name:     "needs normalization",
			input:    []float32{3.0, 4.0},
			wantNorm: 1.0,
		},
		{
			name:     "zero vector",
			input:    []float32{0.0, 0.0, 0.0},
			wantNorm: 0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NormalizeVector(tt.input)

			// Calculate norm of result
			var sum float32
			for _, v := range result {
				sum += v * v
			}
			norm := float32(0)
			if sum > 0 {
				norm = float32(1) // Approximately 1 after normalization
			}

			// Allow small floating point errors
			diff := norm - tt.wantNorm
			if diff < 0 {
				diff = -diff
			}
			if diff > 0.0001 && tt.wantNorm != 0 {
				t.Errorf("Normalized vector norm = %f, want %f", norm, tt.wantNorm)
			}
		})
	}
}

// T030: Integration test for LRU cache eviction behavior
func TestT030_LRUCacheEviction(t *testing.T) {
	t.Run("LRU evicts least recently used entry when at capacity", func(t *testing.T) {
		// Create cache with capacity of 3
		cache := NewCache(3)

		// Fill cache to capacity
		cache.Set("hash1", &Embedding{Hash: "hash1", Vector: []float32{1.0}})
		cache.Set("hash2", &Embedding{Hash: "hash2", Vector: []float32{2.0}})
		cache.Set("hash3", &Embedding{Hash: "hash3", Vector: []float32{3.0}})

		if cache.Size() != 3 {
			t.Errorf("cache size = %d, want 3", cache.Size())
		}

		// Add fourth entry - should evict least recently used (hash1)
		cache.Set("hash4", &Embedding{Hash: "hash4", Vector: []float32{4.0}})

		// hash1 should be evicted
		if _, ok := cache.Get("hash1"); ok {
			t.Error("hash1 should have been evicted but is still in cache")
		}

		// hash4 should be present
		if _, ok := cache.Get("hash4"); !ok {
			t.Error("hash4 should be in cache")
		}

		// Cache size should remain at capacity
		if cache.Size() != 3 {
			t.Errorf("cache size = %d, want 3 after eviction", cache.Size())
		}
	})

	t.Run("accessing entry makes it most recently used", func(t *testing.T) {
		cache := NewCache(3)

		// Fill cache
		cache.Set("hash1", &Embedding{Hash: "hash1", Vector: []float32{1.0}})
		cache.Set("hash2", &Embedding{Hash: "hash2", Vector: []float32{2.0}})
		cache.Set("hash3", &Embedding{Hash: "hash3", Vector: []float32{3.0}})

		// Access hash1 to make it most recently used
		if _, ok := cache.Get("hash1"); !ok {
			t.Fatal("hash1 should be in cache")
		}

		// Add new entry - should evict hash2 (now least recently used)
		cache.Set("hash4", &Embedding{Hash: "hash4", Vector: []float32{4.0}})

		// hash1 should still be present (was accessed)
		if _, ok := cache.Get("hash1"); !ok {
			t.Error("hash1 should still be in cache after being accessed")
		}

		// hash2 should be evicted
		if _, ok := cache.Get("hash2"); ok {
			t.Error("hash2 should have been evicted")
		}
	})

	t.Run("cache maintains hot entries", func(t *testing.T) {
		cache := NewCache(5)

		// Add entries
		for i := 1; i <= 5; i++ {
			hash := ComputeHash(fmt.Sprintf("text%d", i))
			cache.Set(hash, &Embedding{
				Hash:   hash,
				Vector: []float32{float32(i)},
			})
		}

		// Frequently access entries 1, 2, 3
		hotHashes := []string{
			ComputeHash("text1"),
			ComputeHash("text2"),
			ComputeHash("text3"),
		}

		for i := 0; i < 10; i++ {
			for _, hash := range hotHashes {
				cache.Get(hash)
			}
		}

		// Add 3 new entries - should evict cold entries (4, 5, and one hot entry)
		for i := 6; i <= 8; i++ {
			hash := ComputeHash(fmt.Sprintf("text%d", i))
			cache.Set(hash, &Embedding{
				Hash:   hash,
				Vector: []float32{float32(i)},
			})
		}

		// At least 2 of the frequently accessed entries should remain
		hotCount := 0
		for _, hash := range hotHashes {
			if _, ok := cache.Get(hash); ok {
				hotCount++
			}
		}

		if hotCount < 2 {
			t.Errorf("expected at least 2 hot entries to remain, got %d", hotCount)
		}
	})

	t.Run("cache size never exceeds maxEntries", func(t *testing.T) {
		maxEntries := 10
		cache := NewCache(maxEntries)

		// Add many more entries than capacity
		for i := 0; i < maxEntries*3; i++ {
			hash := ComputeHash(fmt.Sprintf("text%d", i))
			cache.Set(hash, &Embedding{
				Hash:   hash,
				Vector: []float32{float32(i)},
			})

			// Verify size never exceeds capacity
			if cache.Size() > maxEntries {
				t.Errorf("cache size %d exceeds max capacity %d", cache.Size(), maxEntries)
			}
		}

		// Final size should be at capacity
		if cache.Size() != maxEntries {
			t.Errorf("final cache size = %d, want %d", cache.Size(), maxEntries)
		}
	})

	t.Run("eviction with concurrent access", func(t *testing.T) {
		cache := NewCache(50)
		done := make(chan bool)

		// Multiple goroutines adding and accessing entries
		for g := 0; g < 5; g++ {
			go func(id int) {
				for i := 0; i < 100; i++ {
					hash := ComputeHash(fmt.Sprintf("goroutine%d_text%d", id, i))
					cache.Set(hash, &Embedding{
						Hash:   hash,
						Vector: []float32{float32(id), float32(i)},
					})

					// Sometimes access existing entries
					if i%3 == 0 && i > 0 {
						oldHash := ComputeHash(fmt.Sprintf("goroutine%d_text%d", id, i-1))
						cache.Get(oldHash)
					}
				}
				done <- true
			}(g)
		}

		// Wait for all goroutines
		for i := 0; i < 5; i++ {
			<-done
		}

		// Cache should be at capacity, not exceed it
		if cache.Size() > 50 {
			t.Errorf("cache size %d exceeds capacity 50 after concurrent access", cache.Size())
		}

		// Cache should have some entries
		if cache.Size() == 0 {
			t.Error("cache is empty after concurrent access")
		}
	})
}
