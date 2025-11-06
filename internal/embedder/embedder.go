package embedder

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	lru "github.com/hashicorp/golang-lru/v2"
)

// Common errors
var (
	ErrInvalidInput      = errors.New("invalid input")
	ErrProviderFailed    = errors.New("embedding provider failed")
	ErrUnsupportedModel  = errors.New("unsupported model")
	ErrEmptyText         = errors.New("text cannot be empty")
	ErrBatchTooLarge     = errors.New("batch size exceeds limit")
	ErrNoProviderEnabled = errors.New("no embedding provider configured")
)

// Embedding represents a vector embedding with metadata
type Embedding struct {
	Vector    []float32
	Dimension int
	Provider  string
	Model     string
	Hash      string // Content hash for caching
}

// EmbeddingRequest represents a request to generate embeddings
type EmbeddingRequest struct {
	Text  string
	Model string // Optional: override default model
}

// BatchEmbeddingRequest represents a batch request
type BatchEmbeddingRequest struct {
	Texts []string
	Model string // Optional: override default model
}

// BatchEmbeddingResponse represents a batch response
type BatchEmbeddingResponse struct {
	Embeddings []*Embedding
	Provider   string
	Model      string
}

// Embedder interface defines methods for generating embeddings
type Embedder interface {
	// GenerateEmbedding generates a single embedding for the given text
	GenerateEmbedding(ctx context.Context, req EmbeddingRequest) (*Embedding, error)

	// GenerateBatch generates embeddings for multiple texts efficiently
	GenerateBatch(ctx context.Context, req BatchEmbeddingRequest) (*BatchEmbeddingResponse, error)

	// Dimension returns the embedding dimension for this provider
	Dimension() int

	// Provider returns the provider name
	Provider() string

	// Model returns the model name
	Model() string

	// Close releases any resources held by the embedder
	Close() error
}

// Cache provides in-memory LRU caching of embeddings by content hash
type Cache struct {
	cache *lru.Cache[string, *Embedding]
}

// NewCache creates a new embedding cache with LRU eviction
func NewCache(maxLen int) *Cache {
	if maxLen <= 0 {
		maxLen = 10000 // Default: cache 10k embeddings
	}
	cache, err := lru.New[string, *Embedding](maxLen)
	if err != nil {
		// Should never happen with positive size, but fallback to default
		cache, _ = lru.New[string, *Embedding](10000)
	}
	return &Cache{
		cache: cache,
	}
}

// Get retrieves a deep copy of an embedding from cache
// Returns a copy to prevent caller mutations from affecting cached values
func (c *Cache) Get(hash string) (*Embedding, bool) {
	emb, ok := c.cache.Get(hash)
	if !ok {
		return nil, false
	}

	// Return deep copy to prevent cache pollution from mutations
	vectorCopy := make([]float32, len(emb.Vector))
	copy(vectorCopy, emb.Vector)

	return &Embedding{
		Vector:    vectorCopy,
		Dimension: emb.Dimension,
		Provider:  emb.Provider,
		Model:     emb.Model,
		Hash:      emb.Hash,
	}, true
}

// Set stores an embedding in cache with automatic LRU eviction
func (c *Cache) Set(hash string, emb *Embedding) {
	// LRU cache handles eviction automatically when at capacity
	c.cache.Add(hash, emb)
}

// Size returns the current cache size
func (c *Cache) Size() int {
	return c.cache.Len()
}

// Clear empties the cache
func (c *Cache) Clear() {
	c.cache.Purge()
}

// ComputeHash computes SHA-256 hash of text for caching
func ComputeHash(text string) string {
	h := sha256.Sum256([]byte(text))
	return hex.EncodeToString(h[:])
}

// ValidateRequest validates an embedding request
func ValidateRequest(req EmbeddingRequest) error {
	if req.Text == "" {
		return ErrEmptyText
	}
	return nil
}

// ValidateBatchRequest validates a batch embedding request
func ValidateBatchRequest(req BatchEmbeddingRequest) error {
	if len(req.Texts) == 0 {
		return fmt.Errorf("%w: no texts provided", ErrInvalidInput)
	}

	for i, text := range req.Texts {
		if text == "" {
			return fmt.Errorf("%w: text at index %d is empty", ErrInvalidInput, i)
		}
	}

	return nil
}
