package integration

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/dshills/gocontext-mcp/internal/indexer"
	"github.com/dshills/gocontext-mcp/internal/searcher"
	"github.com/dshills/gocontext-mcp/internal/storage"
)

// setupSearchBenchmark sets up indexed data for search benchmarks
func setupSearchBenchmark(b *testing.B) (storage.Storage, *searcher.Searcher, int64) {
	// Get fixtures directory
	wd, err := os.Getwd()
	if err != nil {
		b.Fatal(err)
	}
	fixturesDir := filepath.Join(filepath.Dir(wd), "testdata", "fixtures")

	// Create storage and index
	store, err := storage.NewSQLiteStorage(":memory:")
	if err != nil {
		b.Fatal(err)
	}

	idx := indexer.New(store)
	config := &indexer.Config{
		IncludeTests:  true,
		IncludeVendor: false,
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

	// Create mock embedder and searcher
	embedder := NewMockEmbedder(384)
	srch := searcher.NewSearcher(store, embedder)

	return store, srch, project.ID
}

// BenchmarkVectorSearch benchmarks vector similarity search
func BenchmarkVectorSearch(b *testing.B) {
	store, srch, projectID := setupSearchBenchmark(b)
	defer store.Close()

	req := searcher.SearchRequest{
		Query:     "user repository interface",
		Limit:     10,
		Mode:      searcher.SearchModeVector,
		ProjectID: projectID,
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := srch.Search(context.Background(), req)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkKeywordSearch benchmarks BM25 text search
func BenchmarkKeywordSearch(b *testing.B) {
	store, srch, projectID := setupSearchBenchmark(b)
	defer store.Close()

	req := searcher.SearchRequest{
		Query:     "ValidateEmail function",
		Limit:     10,
		Mode:      searcher.SearchModeKeyword,
		ProjectID: projectID,
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := srch.Search(context.Background(), req)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkHybridSearch benchmarks hybrid search with RRF
func BenchmarkHybridSearch(b *testing.B) {
	store, srch, projectID := setupSearchBenchmark(b)
	defer store.Close()

	req := searcher.SearchRequest{
		Query:       "order service business logic",
		Limit:       10,
		Mode:        searcher.SearchModeHybrid,
		ProjectID:   projectID,
		RRFConstant: 60,
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := srch.Search(context.Background(), req)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkSearchWithFilters benchmarks search with various filters
func BenchmarkSearchWithFilters(b *testing.B) {
	store, srch, projectID := setupSearchBenchmark(b)
	defer store.Close()

	req := searcher.SearchRequest{
		Query:     "repository",
		Limit:     10,
		Mode:      searcher.SearchModeHybrid,
		ProjectID: projectID,
		Filters: &storage.SearchFilters{
			SymbolTypes: []string{"interface", "struct"},
			DDDPatterns: []string{"repository"},
		},
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := srch.Search(context.Background(), req)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkSearchLimits benchmarks different result limits
func BenchmarkSearchLimits(b *testing.B) {
	store, srch, projectID := setupSearchBenchmark(b)
	defer store.Close()

	limits := []int{1, 5, 10, 20, 50}

	for _, limit := range limits {
		b.Run(string(rune('0'+limit/10))+"_limit_"+string(rune('0'+limit%10)), func(b *testing.B) {
			req := searcher.SearchRequest{
				Query:     "user order service",
				Limit:     limit,
				Mode:      searcher.SearchModeHybrid,
				ProjectID: projectID,
			}

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				_, err := srch.Search(context.Background(), req)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
