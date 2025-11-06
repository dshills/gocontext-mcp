package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/dshills/gocontext-mcp/internal/embedder"
	"github.com/dshills/gocontext-mcp/internal/indexer"
	"github.com/dshills/gocontext-mcp/internal/storage"
)

// MockEmbedder provides a simple test embedder
type MockEmbedder struct {
	dimension int
}

func NewMockEmbedder(dimension int) *MockEmbedder {
	return &MockEmbedder{dimension: dimension}
}

func (m *MockEmbedder) GenerateEmbedding(ctx context.Context, req embedder.EmbeddingRequest) (*embedder.Embedding, error) {
	vector := make([]float32, m.dimension)
	for i := range vector {
		vector[i] = 0.1 * float32(i)
	}
	return &embedder.Embedding{
		Vector:    vector,
		Dimension: m.dimension,
		Provider:  "mock",
		Model:     "mock-v1",
	}, nil
}

func (m *MockEmbedder) GenerateBatch(ctx context.Context, req embedder.BatchEmbeddingRequest) (*embedder.BatchEmbeddingResponse, error) {
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
		Provider:   "mock",
		Model:      "mock-v1",
	}, nil
}

func (m *MockEmbedder) Dimension() int   { return m.dimension }
func (m *MockEmbedder) Provider() string { return "mock" }
func (m *MockEmbedder) Model() string    { return "mock-v1" }
func (m *MockEmbedder) Close() error     { return nil }

func main() {
	fmt.Println("Testing embedding integration...")

	// Create temp directory for test
	tmpDir, err := os.MkdirTemp("", "gocontext-test-*")
	if err != nil {
		log.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a simple test Go file
	testFile := tmpDir + "/test.go"
	testCode := `package main

// Add adds two numbers
func Add(a, b int) int {
	return a + b
}

func main() {
	result := Add(1, 2)
	println(result)
}
`
	if err := os.WriteFile(testFile, []byte(testCode), 0644); err != nil {
		log.Fatalf("Failed to write test file: %v", err)
	}

	// Create in-memory storage
	store, err := storage.NewSQLiteStorage(":memory:")
	if err != nil {
		log.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	// Create mock embedder
	mockEmb := NewMockEmbedder(384)

	// Create indexer with embedder
	idx := indexer.NewWithEmbedder(store, mockEmb)

	// Index with embeddings enabled
	config := &indexer.Config{
		Workers:            2,
		BatchSize:          10,
		EmbeddingBatch:     30,
		IncludeTests:       false,
		IncludeVendor:      false,
		GenerateEmbeddings: true,
	}

	ctx := context.Background()
	stats, err := idx.IndexProject(ctx, tmpDir, config)
	if err != nil {
		log.Fatalf("Failed to index project: %v", err)
	}

	// Print statistics
	fmt.Printf("\nIndexing Statistics:\n")
	fmt.Printf("  Files Indexed: %d\n", stats.FilesIndexed)
	fmt.Printf("  Files Skipped: %d\n", stats.FilesSkipped)
	fmt.Printf("  Files Failed: %d\n", stats.FilesFailed)
	fmt.Printf("  Symbols Extracted: %d\n", stats.SymbolsExtracted)
	fmt.Printf("  Chunks Created: %d\n", stats.ChunksCreated)
	fmt.Printf("  Embeddings Generated: %d\n", stats.EmbeddingsGenerated)
	fmt.Printf("  Embeddings Failed: %d\n", stats.EmbeddingsFailed)
	fmt.Printf("  Duration: %v\n", stats.Duration)

	if len(stats.ErrorMessages) > 0 {
		fmt.Printf("\nErrors:\n")
		for _, msg := range stats.ErrorMessages {
			fmt.Printf("  - %s\n", msg)
		}
	}

	// Verify embeddings were stored
	project, err := store.GetProject(ctx, tmpDir)
	if err != nil {
		log.Fatalf("Failed to get project: %v", err)
	}

	files, err := store.ListFiles(ctx, project.ID)
	if err != nil {
		log.Fatalf("Failed to list files: %v", err)
	}

	embeddingCount := 0
	for _, file := range files {
		chunks, err := store.ListChunksByFile(ctx, file.ID)
		if err != nil {
			log.Fatalf("Failed to list chunks: %v", err)
		}

		for _, chunk := range chunks {
			_, err := store.GetEmbedding(ctx, chunk.ID)
			if err == nil {
				embeddingCount++
			}
		}
	}

	fmt.Printf("\nVerification:\n")
	fmt.Printf("  Embeddings in DB: %d\n", embeddingCount)

	if embeddingCount > 0 {
		fmt.Println("\n✓ SUCCESS: Embeddings were generated and stored!")
	} else {
		fmt.Println("\n✗ FAILURE: No embeddings were stored!")
		os.Exit(1)
	}
}
