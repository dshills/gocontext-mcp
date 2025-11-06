package searcher

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/dshills/gocontext-mcp/internal/embedder"
	"github.com/dshills/gocontext-mcp/internal/indexer"
	"github.com/dshills/gocontext-mcp/internal/storage"
)

// MockEmbedder provides a fast, deterministic embedder for benchmarking
type MockEmbedder struct {
	dimension int
}

func NewMockEmbedder(dimension int) *MockEmbedder {
	return &MockEmbedder{dimension: dimension}
}

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
		Provider:  "mock",
		Model:     "mock-v1",
		Hash:      embedder.ComputeHash(req.Text),
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

// setupSearchBenchmark sets up indexed data for search benchmarks
func setupSearchBenchmark(b *testing.B) (storage.Storage, *Searcher, int64) {
	// Get fixtures directory
	wd, err := os.Getwd()
	if err != nil {
		b.Fatal(err)
	}
	fixturesDir := filepath.Join(filepath.Dir(filepath.Dir(wd)), "tests", "testdata", "fixtures")

	// Check if fixtures exist
	if _, err := os.Stat(fixturesDir); os.IsNotExist(err) {
		b.Skipf("Fixtures directory not found: %s", fixturesDir)
	}

	// Create storage and index
	store, err := storage.NewSQLiteStorage(":memory:")
	if err != nil {
		b.Fatal(err)
	}

	mockEmb := NewMockEmbedder(384)
	idx := indexer.NewWithEmbedder(store, mockEmb)
	config := &indexer.Config{
		IncludeTests:       true,
		IncludeVendor:      false,
		GenerateEmbeddings: true,
		Workers:            4,
		BatchSize:          20,
	}

	_, err = idx.IndexProject(context.Background(), fixturesDir, config)
	if err != nil {
		store.Close()
		b.Fatal(err)
	}

	project, err := store.GetProject(context.Background(), fixturesDir)
	if err != nil {
		store.Close()
		b.Fatal(err)
	}

	srch := NewSearcher(store, mockEmb)

	return store, srch, project.ID
}

// BenchmarkHybridSearch benchmarks full hybrid search (vector + BM25 + RRF)
func BenchmarkHybridSearch(b *testing.B) {
	store, srch, projectID := setupSearchBenchmark(b)
	defer store.Close()

	req := SearchRequest{
		Query:       "order service business logic",
		Limit:       10,
		Mode:        SearchModeHybrid,
		ProjectID:   projectID,
		RRFConstant: 60,
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := srch.Search(context.Background(), req)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkVectorSearch benchmarks vector similarity search only
func BenchmarkVectorSearch(b *testing.B) {
	store, srch, projectID := setupSearchBenchmark(b)
	defer store.Close()

	req := SearchRequest{
		Query:     "user repository interface",
		Limit:     10,
		Mode:      SearchModeVector,
		ProjectID: projectID,
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := srch.Search(context.Background(), req)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkKeywordSearch benchmarks BM25 text search only
func BenchmarkKeywordSearch(b *testing.B) {
	store, srch, projectID := setupSearchBenchmark(b)
	defer store.Close()

	req := SearchRequest{
		Query:     "ValidateEmail function",
		Limit:     10,
		Mode:      SearchModeKeyword,
		ProjectID: projectID,
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := srch.Search(context.Background(), req)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkRRF benchmarks Reciprocal Rank Fusion algorithm
func BenchmarkRRF(b *testing.B) {
	// Create synthetic search results
	vectorResults := make([]storage.VectorResult, 20)
	for i := range vectorResults {
		vectorResults[i] = storage.VectorResult{
			ChunkID:         int64(i + 1),
			SimilarityScore: float64(20-i) / 20.0,
		}
	}

	textResults := make([]storage.TextResult, 20)
	for i := range textResults {
		textResults[i] = storage.TextResult{
			ChunkID:   int64(i + 10),
			BM25Score: float64(20-i) / 10.0,
		}
	}

	store, err := storage.NewSQLiteStorage(":memory:")
	if err != nil {
		b.Fatal(err)
	}
	defer store.Close()

	mockEmb := NewMockEmbedder(384)
	srch := NewSearcher(store, mockEmb)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = srch.applyRRF(vectorResults, textResults, 60)
	}
}

// BenchmarkFilterApplication benchmarks filter processing
func BenchmarkFilterApplication(b *testing.B) {
	store, srch, projectID := setupSearchBenchmark(b)
	defer store.Close()

	filters := &storage.SearchFilters{
		SymbolTypes:  []string{"function", "struct", "interface"},
		DDDPatterns:  []string{"repository", "service", "aggregate"},
		FilePattern:  "*.go",
		MinRelevance: 0.5,
	}

	req := SearchRequest{
		Query:     "user order repository",
		Limit:     10,
		Mode:      SearchModeHybrid,
		ProjectID: projectID,
		Filters:   filters,
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := srch.Search(context.Background(), req)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkQueryValidation benchmarks request validation
func BenchmarkQueryValidation(b *testing.B) {
	store, err := storage.NewSQLiteStorage(":memory:")
	if err != nil {
		b.Fatal(err)
	}
	defer store.Close()

	mockEmb := NewMockEmbedder(384)
	srch := NewSearcher(store, mockEmb)

	req := SearchRequest{
		Query:     "test query",
		Limit:     10,
		Mode:      SearchModeHybrid,
		ProjectID: 1,
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		err := srch.validateRequest(&req)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkQueryHashing benchmarks query hash computation
func BenchmarkQueryHashing(b *testing.B) {
	req := SearchRequest{
		Query:     "test query with filters",
		Limit:     10,
		Mode:      SearchModeHybrid,
		ProjectID: 1,
		Filters: &storage.SearchFilters{
			SymbolTypes: []string{"function", "struct"},
			DDDPatterns: []string{"repository"},
		},
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = computeQueryHash(req)
	}
}

// BenchmarkResultsFetching benchmarks fetching full result details
func BenchmarkResultsFetching(b *testing.B) {
	store, srch, projectID := setupSearchBenchmark(b)
	defer store.Close()

	// First do a search to get ranked results
	req := SearchRequest{
		Query:     "repository",
		Limit:     20,
		Mode:      SearchModeHybrid,
		ProjectID: projectID,
	}

	resp, err := srch.Search(context.Background(), req)
	if err != nil {
		b.Fatal(err)
	}

	if len(resp.Results) == 0 {
		b.Skip("No results to fetch")
	}

	// Create ranked results from the response
	ranked := make([]rankedResult, len(resp.Results))
	for i, r := range resp.Results {
		ranked[i] = rankedResult{
			chunkID: r.ChunkID,
			score:   r.RelevanceScore,
			rank:    r.Rank,
		}
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := srch.fetchResults(context.Background(), ranked, 10)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkSearchLimits benchmarks different result limits
func BenchmarkSearchLimits(b *testing.B) {
	store, srch, projectID := setupSearchBenchmark(b)
	defer store.Close()

	limits := []int{1, 5, 10, 20, 50, 100}

	for _, limit := range limits {
		b.Run(fmt.Sprintf("%03d_results", limit), func(b *testing.B) {
			req := SearchRequest{
				Query:     "user order service",
				Limit:     limit,
				Mode:      SearchModeHybrid,
				ProjectID: projectID,
			}

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				_, err := srch.Search(context.Background(), req)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkSearchQueries benchmarks various query types
func BenchmarkSearchQueries(b *testing.B) {
	store, srch, projectID := setupSearchBenchmark(b)
	defer store.Close()

	queries := []struct {
		name  string
		query string
	}{
		{"short", "user"},
		{"medium", "user repository"},
		{"long", "user repository interface with methods"},
		{"domain", "order aggregate with business rules"},
		{"technical", "func ValidateEmail returns error"},
	}

	for _, q := range queries {
		b.Run(q.name, func(b *testing.B) {
			req := SearchRequest{
				Query:     q.query,
				Limit:     10,
				Mode:      SearchModeHybrid,
				ProjectID: projectID,
			}

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				_, err := srch.Search(context.Background(), req)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkSearchModes benchmarks different search modes
func BenchmarkSearchModes(b *testing.B) {
	store, srch, projectID := setupSearchBenchmark(b)
	defer store.Close()

	modes := []SearchMode{
		SearchModeVector,
		SearchModeKeyword,
		SearchModeHybrid,
	}

	for _, mode := range modes {
		b.Run(string(mode), func(b *testing.B) {
			req := SearchRequest{
				Query:     "repository interface",
				Limit:     10,
				Mode:      mode,
				ProjectID: projectID,
			}

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				_, err := srch.Search(context.Background(), req)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkEmbeddingGeneration benchmarks query embedding generation
func BenchmarkEmbeddingGeneration(b *testing.B) {
	mockEmb := NewMockEmbedder(384)
	ctx := context.Background()

	queries := []string{
		"user",
		"user repository",
		"user repository interface with methods",
		"order aggregate with business validation rules and domain events",
	}

	for _, query := range queries {
		b.Run(fmt.Sprintf("len_%d", len(query)), func(b *testing.B) {
			req := embedder.EmbeddingRequest{Text: query}

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				_, err := mockEmb.GenerateEmbedding(ctx, req)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkSortRankedResults benchmarks result sorting
func BenchmarkSortRankedResults(b *testing.B) {
	// Create various sizes of ranked results
	sizes := []int{10, 20, 50, 100, 200}

	for _, size := range sizes {
		b.Run(fmt.Sprintf("%03d_results", size), func(b *testing.B) {
			results := make([]rankedResult, size)
			for i := range results {
				results[i] = rankedResult{
					chunkID: int64(i),
					score:   float64(size-i) / float64(size),
					rank:    i,
				}
			}

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				// Make a copy to sort
				toSort := make([]rankedResult, len(results))
				copy(toSort, results)
				sortRankedResults(toSort)
			}
		})
	}
}

// BenchmarkConcurrentSearch benchmarks concurrent search operations
func BenchmarkConcurrentSearch(b *testing.B) {
	store, srch, projectID := setupSearchBenchmark(b)
	defer store.Close()

	req := SearchRequest{
		Query:     "repository interface",
		Limit:     10,
		Mode:      SearchModeHybrid,
		ProjectID: projectID,
	}

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, err := srch.Search(context.Background(), req)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}
