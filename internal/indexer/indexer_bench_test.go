package indexer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

// BenchmarkRealCodebase benchmarks indexing the gocontext-mcp codebase itself
func BenchmarkRealCodebase(b *testing.B) {
	// Get the project root (go up from internal/indexer to project root)
	wd, err := os.Getwd()
	if err != nil {
		b.Fatal(err)
	}
	projectRoot := filepath.Join(filepath.Dir(filepath.Dir(wd)))

	// Verify go.mod exists to confirm we're in the right place
	if _, err := os.Stat(filepath.Join(projectRoot, "go.mod")); os.IsNotExist(err) {
		b.Skipf("Project root not found at %s", projectRoot)
	}

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

		stats, err := idx.IndexProject(context.Background(), projectRoot, config)
		if err != nil {
			b.Fatal(err)
		}

		b.StopTimer()
		b.Logf("Indexed %d files, %d symbols, %d chunks, %d embeddings in %v",
			stats.FilesIndexed, stats.SymbolsExtracted, stats.ChunksCreated,
			stats.EmbeddingsGenerated, stats.Duration)

		// Calculate LOC and extrapolate to 100k LOC
		if stats.FilesIndexed > 0 {
			// Rough estimate: average Go file is ~300-500 LOC
			estimatedLOC := stats.FilesIndexed * 350
			scaleFactor := float64(100000) / float64(estimatedLOC)
			projectedTime := time.Duration(float64(stats.Duration) * scaleFactor)

			b.Logf("Estimated LOC: ~%d", estimatedLOC)
			b.Logf("Projected time for 100k LOC: %v", projectedTime)

			// Verify performance target: < 5 minutes for 100k LOC
			if projectedTime > 5*time.Minute {
				b.Logf("WARNING: Projected time %v exceeds 5 minute target", projectedTime)
			}
		}

		_ = store.Close()
		b.StartTimer()
	}
}

// BenchmarkRealCodebaseNoEmbeddings benchmarks indexing without embeddings
func BenchmarkRealCodebaseNoEmbeddings(b *testing.B) {
	wd, err := os.Getwd()
	if err != nil {
		b.Fatal(err)
	}
	projectRoot := filepath.Join(filepath.Dir(filepath.Dir(wd)))

	if _, err := os.Stat(filepath.Join(projectRoot, "go.mod")); os.IsNotExist(err) {
		b.Skipf("Project root not found at %s", projectRoot)
	}

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

		stats, err := idx.IndexProject(context.Background(), projectRoot, config)
		if err != nil {
			b.Fatal(err)
		}

		b.StopTimer()
		b.Logf("Indexed %d files, %d symbols, %d chunks in %v",
			stats.FilesIndexed, stats.SymbolsExtracted, stats.ChunksCreated, stats.Duration)

		// Performance comparison
		if stats.FilesIndexed > 0 {
			avgTimePerFile := stats.Duration / time.Duration(stats.FilesIndexed)
			b.Logf("Average time per file: %v", avgTimePerFile)

			// Estimate for 100k LOC (assuming ~350 files for 100k LOC)
			estimatedFiles := 100000 / 300 // ~333 files
			projectedTime := avgTimePerFile * time.Duration(estimatedFiles)
			b.Logf("Projected time for 100k LOC: %v", projectedTime)
		}

		_ = store.Close()
		b.StartTimer()
	}
}

// BenchmarkLargeScaleIndexing simulates indexing a 500k LOC codebase
func BenchmarkLargeScaleIndexing(b *testing.B) {
	// Create a temporary directory with generated Go files
	tempDir := b.TempDir()

	// Generate 100 files with ~5k LOC each to simulate 500k LOC
	b.Logf("Generating 100 test files with ~5k LOC each...")
	for fileNum := 0; fileNum < 100; fileNum++ {
		var content strings.Builder
		content.WriteString("package generated\n\n")
		content.WriteString("// Generated file for large-scale benchmarking\n\n")

		// Generate ~250 functions (~20 LOC each = 5k LOC per file)
		for i := 0; i < 250; i++ {
			funcNum := fileNum*250 + i
			content.WriteString(fmt.Sprintf("// Function%d performs operation %d\n", funcNum, funcNum))
			content.WriteString(fmt.Sprintf("func Function%d(x int, y int) int {\n", funcNum))
			content.WriteString("	result := x + y\n")
			content.WriteString("	for i := 0; i < 10; i++ {\n")
			content.WriteString("		result = result * 2\n")
			content.WriteString("		if result > 1000 {\n")
			content.WriteString("			result = result / 2\n")
			content.WriteString("		}\n")
			content.WriteString("	}\n")
			content.WriteString("	return result\n")
			content.WriteString("}\n\n")
		}

		fileName := fmt.Sprintf("generated_%03d.go", fileNum)
		filePath := filepath.Join(tempDir, fileName)
		if err := os.WriteFile(filePath, []byte(content.String()), 0644); err != nil {
			b.Fatal(err)
		}
	}

	b.Logf("Generated test corpus at %s", tempDir)

	config := &Config{
		Workers:            8, // More workers for large project
		BatchSize:          50,
		EmbeddingBatch:     50,
		IncludeTests:       false,
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

		stats, err := idx.IndexProject(context.Background(), tempDir, config)
		if err != nil {
			b.Fatal(err)
		}

		b.StopTimer()
		b.Logf("Large-scale indexing: %d files, %d symbols, %d chunks, %d embeddings in %v",
			stats.FilesIndexed, stats.SymbolsExtracted, stats.ChunksCreated,
			stats.EmbeddingsGenerated, stats.Duration)

		// Verify performance target
		if stats.Duration > 5*time.Minute {
			b.Logf("WARNING: Large-scale indexing took %v, exceeds 5 minute target", stats.Duration)
		} else {
			b.Logf("SUCCESS: Large-scale indexing completed in %v (within 5 minute target)", stats.Duration)
		}

		_ = store.Close()
		b.StartTimer()
	}
}
