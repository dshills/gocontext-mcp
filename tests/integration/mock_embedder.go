package integration

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"

	"github.com/dshills/gocontext-mcp/internal/embedder"
)

// MockEmbedder provides a fake embedder for testing
// It generates deterministic vectors based on text hash
type MockEmbedder struct {
	dimension int
	provider  string
	model     string
}

// NewMockEmbedder creates a new mock embedder
func NewMockEmbedder(dimension int) *MockEmbedder {
	return &MockEmbedder{
		dimension: dimension,
		provider:  "mock",
		model:     "mock-v1",
	}
}

// GenerateEmbedding generates a deterministic fake embedding
func (m *MockEmbedder) GenerateEmbedding(ctx context.Context, req embedder.EmbeddingRequest) (*embedder.Embedding, error) {
	if req.Text == "" {
		return nil, embedder.ErrEmptyText
	}

	// Generate deterministic vector from text hash
	hash := sha256.Sum256([]byte(req.Text))
	vector := make([]float32, m.dimension)

	// Use hash bytes to generate pseudo-random but deterministic floats
	for i := 0; i < m.dimension; i++ {
		idx := (i * 4) % 32
		val := binary.BigEndian.Uint32(hash[idx : idx+4])
		// Normalize to [-1, 1]
		vector[i] = (float32(val)/float32(1<<32))*2 - 1
	}

	// Normalize vector to unit length
	var sum float32
	for _, v := range vector {
		sum += v * v
	}
	magnitude := float32(1.0)
	if sum > 0 {
		magnitude = float32(1.0) / float32(sum)
		for i := range vector {
			vector[i] *= magnitude
		}
	}

	return &embedder.Embedding{
		Vector:    vector,
		Dimension: m.dimension,
		Provider:  m.provider,
		Model:     m.model,
		Hash:      embedder.ComputeHash(req.Text),
	}, nil
}

// GenerateBatch generates embeddings for multiple texts
func (m *MockEmbedder) GenerateBatch(ctx context.Context, req embedder.BatchEmbeddingRequest) (*embedder.BatchEmbeddingResponse, error) {
	if len(req.Texts) == 0 {
		return nil, fmt.Errorf("no texts provided")
	}

	embeddings := make([]*embedder.Embedding, len(req.Texts))
	for i, text := range req.Texts {
		emb, err := m.GenerateEmbedding(ctx, embedder.EmbeddingRequest{Text: text})
		if err != nil {
			return nil, err
		}
		embeddings[i] = emb
	}

	return &embedder.BatchEmbeddingResponse{
		Embeddings: embeddings,
		Provider:   m.provider,
		Model:      m.model,
	}, nil
}

// Dimension returns the embedding dimension
func (m *MockEmbedder) Dimension() int {
	return m.dimension
}

// Provider returns the provider name
func (m *MockEmbedder) Provider() string {
	return m.provider
}

// Model returns the model name
func (m *MockEmbedder) Model() string {
	return m.model
}

// Close releases resources (no-op for mock)
func (m *MockEmbedder) Close() error {
	return nil
}
