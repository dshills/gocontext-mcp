package indexer

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/dshills/gocontext-mcp/internal/embedder"
	"github.com/dshills/gocontext-mcp/internal/storage"
)

// MockEmbedder provides a fast, fake embedder for benchmarking
type MockEmbedder struct {
	dimension int
}

func NewMockEmbedder(dimension int) *MockEmbedder {
	return &MockEmbedder{dimension: dimension}
}

func (m *MockEmbedder) GenerateEmbedding(ctx context.Context, req embedder.EmbeddingRequest) (*embedder.Embedding, error) {
	return &embedder.Embedding{
		Vector:    make([]float32, m.dimension),
		Dimension: m.dimension,
		Provider:  "mock",
		Model:     "mock-v1",
		Hash:      embedder.ComputeHash(req.Text),
	}, nil
}

func (m *MockEmbedder) GenerateBatch(ctx context.Context, req embedder.BatchEmbeddingRequest) (*embedder.BatchEmbeddingResponse, error) {
	embeddings := make([]*embedder.Embedding, len(req.Texts))
	for i, text := range req.Texts {
		embeddings[i] = &embedder.Embedding{
			Vector:    make([]float32, m.dimension),
			Dimension: m.dimension,
			Provider:  "mock",
			Model:     "mock-v1",
			Hash:      embedder.ComputeHash(text),
		}
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

// getTestFixtures returns the path to test fixtures
func getTestFixtures(b *testing.B) string {
	wd, err := os.Getwd()
	if err != nil {
		b.Fatal(err)
	}
	// Navigate from internal/indexer to tests/testdata/fixtures
	fixturesDir := filepath.Join(filepath.Dir(filepath.Dir(wd)), "tests", "testdata", "fixtures")
	if _, err := os.Stat(fixturesDir); os.IsNotExist(err) {
		b.Skipf("Fixtures directory not found: %s", fixturesDir)
	}
	return fixturesDir
}

// BenchmarkIndexProject benchmarks full project indexing
func BenchmarkIndexProject(b *testing.B) {
	fixturesDir := getTestFixtures(b)

	config := &Config{
		Workers:            4,
		BatchSize:          20,
		EmbeddingBatch:     30,
		IncludeTests:       true,
		IncludeVendor:      false,
		GenerateEmbeddings: true,
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		b.StopTimer()
		store, err := storage.NewSQLiteStorage(":memory:")
		if err != nil {
			b.Fatal(err)
		}
		mockEmb := NewMockEmbedder(384)
		idx := NewWithEmbedder(store, mockEmb)
		b.StartTimer()

		_, err = idx.IndexProject(context.Background(), fixturesDir, config)
		if err != nil {
			b.Fatal(err)
		}

		b.StopTimer()
		_ = store.Close()
		b.StartTimer()
	}
}

// BenchmarkIndexProjectNoEmbeddings benchmarks indexing without embeddings
func BenchmarkIndexProjectNoEmbeddings(b *testing.B) {
	fixturesDir := getTestFixtures(b)

	config := &Config{
		Workers:            4,
		BatchSize:          20,
		IncludeTests:       true,
		IncludeVendor:      false,
		GenerateEmbeddings: false,
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		b.StopTimer()
		store, err := storage.NewSQLiteStorage(":memory:")
		if err != nil {
			b.Fatal(err)
		}
		idx := New(store)
		b.StartTimer()

		_, err = idx.IndexProject(context.Background(), fixturesDir, config)
		if err != nil {
			b.Fatal(err)
		}

		b.StopTimer()
		_ = store.Close()
		b.StartTimer()
	}
}

// BenchmarkIncrementalIndex benchmarks re-indexing with few changes
func BenchmarkIncrementalIndex(b *testing.B) {
	fixturesDir := getTestFixtures(b)

	// Setup: do initial indexing once
	store, err := storage.NewSQLiteStorage(":memory:")
	if err != nil {
		b.Fatal(err)
	}
	defer store.Close()

	mockEmb := NewMockEmbedder(384)
	idx := NewWithEmbedder(store, mockEmb)
	config := &Config{
		Workers:            4,
		BatchSize:          20,
		IncludeTests:       true,
		IncludeVendor:      false,
		GenerateEmbeddings: false,
	}

	// Initial indexing
	_, err = idx.IndexProject(context.Background(), fixturesDir, config)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	b.ReportAllocs()

	// Benchmark re-indexing (should skip unchanged files)
	for i := 0; i < b.N; i++ {
		_, err := idx.IndexProject(context.Background(), fixturesDir, config)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkFileDiscovery benchmarks file discovery only
func BenchmarkFileDiscovery(b *testing.B) {
	fixturesDir := getTestFixtures(b)

	store, err := storage.NewSQLiteStorage(":memory:")
	if err != nil {
		b.Fatal(err)
	}
	defer store.Close()

	idx := New(store)
	config := &Config{
		IncludeTests:  true,
		IncludeVendor: false,
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := idx.discoverFiles(fixturesDir, config)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkParseFile benchmarks individual file parsing
func BenchmarkParseFile(b *testing.B) {
	fixturesDir := getTestFixtures(b)

	// Find a representative Go file
	var testFile string
	err := filepath.Walk(fixturesDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		if filepath.Ext(path) == ".go" && testFile == "" {
			testFile = path
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil || testFile == "" {
		b.Skip("No Go files found in fixtures")
	}

	store, err := storage.NewSQLiteStorage(":memory:")
	if err != nil {
		b.Fatal(err)
	}
	defer store.Close()

	idx := New(store)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := idx.parser.ParseFile(testFile)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkChunkFile benchmarks individual file chunking
func BenchmarkChunkFile(b *testing.B) {
	fixturesDir := getTestFixtures(b)

	// Find a representative Go file
	var testFile string
	err := filepath.Walk(fixturesDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		if filepath.Ext(path) == ".go" && testFile == "" {
			testFile = path
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil || testFile == "" {
		b.Skip("No Go files found in fixtures")
	}

	store, err := storage.NewSQLiteStorage(":memory:")
	if err != nil {
		b.Fatal(err)
	}
	defer store.Close()

	idx := New(store)

	// Parse once to get parse result
	parseResult, err := idx.parser.ParseFile(testFile)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := idx.chunker.ChunkFile(testFile, parseResult, 1)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkEmbeddingGeneration benchmarks batch embedding generation
func BenchmarkEmbeddingGeneration(b *testing.B) {
	mockEmb := NewMockEmbedder(384)
	ctx := context.Background()

	// Test different batch sizes
	batchSizes := []int{1, 10, 30, 50, 100}

	for _, size := range batchSizes {
		b.Run(string(rune('0'+size/100))+string(rune('0'+(size/10)%10))+string(rune('0'+size%10))+"_chunks", func(b *testing.B) {
			texts := make([]string, size)
			for i := range texts {
				texts[i] = "func Example() { return nil }"
			}

			req := embedder.BatchEmbeddingRequest{Texts: texts}

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				_, err := mockEmb.GenerateBatch(ctx, req)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkWorkerCounts benchmarks different worker pool sizes
func BenchmarkWorkerCounts(b *testing.B) {
	fixturesDir := getTestFixtures(b)
	workerCounts := []int{1, 2, 4, 8, 16}

	for _, workers := range workerCounts {
		b.Run(string(rune('0'+workers/10))+string(rune('0'+workers%10))+"_workers", func(b *testing.B) {
			config := &Config{
				Workers:            workers,
				BatchSize:          20,
				IncludeTests:       true,
				IncludeVendor:      false,
				GenerateEmbeddings: false,
			}

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				b.StopTimer()
				store, err := storage.NewSQLiteStorage(":memory:")
				if err != nil {
					b.Fatal(err)
				}
				idx := New(store)
				b.StartTimer()

				_, err = idx.IndexProject(context.Background(), fixturesDir, config)
				if err != nil {
					b.Fatal(err)
				}

				b.StopTimer()
				_ = store.Close()
				b.StartTimer()
			}
		})
	}
}

// BenchmarkBatchSizes benchmarks different transaction batch sizes
func BenchmarkBatchSizes(b *testing.B) {
	fixturesDir := getTestFixtures(b)
	batchSizes := []int{5, 10, 20, 50, 100}

	for _, batchSize := range batchSizes {
		b.Run(string(rune('0'+batchSize/100))+string(rune('0'+(batchSize/10)%10))+string(rune('0'+batchSize%10))+"_batch", func(b *testing.B) {
			config := &Config{
				Workers:            4,
				BatchSize:          batchSize,
				IncludeTests:       true,
				IncludeVendor:      false,
				GenerateEmbeddings: false,
			}

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				b.StopTimer()
				store, err := storage.NewSQLiteStorage(":memory:")
				if err != nil {
					b.Fatal(err)
				}
				idx := New(store)
				b.StartTimer()

				_, err = idx.IndexProject(context.Background(), fixturesDir, config)
				if err != nil {
					b.Fatal(err)
				}

				b.StopTimer()
				_ = store.Close()
				b.StartTimer()
			}
		})
	}
}

// BenchmarkFileHashing benchmarks file hash computation
func BenchmarkFileHashing(b *testing.B) {
	fixturesDir := getTestFixtures(b)

	// Find a representative Go file
	var testFile string
	err := filepath.Walk(fixturesDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		if filepath.Ext(path) == ".go" && testFile == "" {
			testFile = path
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil || testFile == "" {
		b.Skip("No Go files found in fixtures")
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _, _, err := computeFileHash(testFile)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkGoModParsing benchmarks go.mod parsing
func BenchmarkGoModParsing(b *testing.B) {
	fixturesDir := getTestFixtures(b)
	goModPath := filepath.Join(fixturesDir, "go.mod")

	// Check if go.mod exists
	if _, err := os.Stat(goModPath); os.IsNotExist(err) {
		b.Skip("go.mod not found in fixtures")
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := parseGoMod(goModPath)
		if err != nil {
			b.Fatal(err)
		}
	}
}
