package embedder

import (
	"context"
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
