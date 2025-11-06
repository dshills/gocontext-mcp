package integration

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/dshills/gocontext-mcp/internal/indexer"
	"github.com/dshills/gocontext-mcp/internal/storage"
)

// BenchmarkFullIndexing benchmarks the complete indexing pipeline
func BenchmarkFullIndexing(b *testing.B) {
	// Get fixtures directory
	wd, err := os.Getwd()
	if err != nil {
		b.Fatal(err)
	}
	fixturesDir := filepath.Join(filepath.Dir(wd), "testdata", "fixtures")

	config := &indexer.Config{
		IncludeTests:  true,
		IncludeVendor: false,
		Workers:       4,
		BatchSize:     10,
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Create fresh storage for each iteration
		store, err := storage.NewSQLiteStorage(":memory:")
		if err != nil {
			b.Fatal(err)
		}

		idx := indexer.New(store)
		_, err = idx.IndexProject(context.Background(), fixturesDir, config)
		if err != nil {
			b.Fatal(err)
		}

		_ = store.Close()
	}
}

// BenchmarkIndexingWorkers benchmarks different worker counts
func BenchmarkIndexingWorkers(b *testing.B) {
	wd, err := os.Getwd()
	if err != nil {
		b.Fatal(err)
	}
	fixturesDir := filepath.Join(filepath.Dir(wd), "testdata", "fixtures")

	workerCounts := []int{1, 2, 4, 8}

	for _, workers := range workerCounts {
		b.Run(string(rune('0'+workers))+"_workers", func(b *testing.B) {
			config := &indexer.Config{
				IncludeTests:  true,
				IncludeVendor: false,
				Workers:       workers,
				BatchSize:     10,
			}

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				store, err := storage.NewSQLiteStorage(":memory:")
				if err != nil {
					b.Fatal(err)
				}

				idx := indexer.New(store)
				_, err = idx.IndexProject(context.Background(), fixturesDir, config)
				if err != nil {
					b.Fatal(err)
				}

				_ = store.Close()
			}
		})
	}
}

// BenchmarkIncrementalIndexing benchmarks re-indexing with no changes
func BenchmarkIncrementalIndexing(b *testing.B) {
	wd, err := os.Getwd()
	if err != nil {
		b.Fatal(err)
	}
	fixturesDir := filepath.Join(filepath.Dir(wd), "testdata", "fixtures")

	// Setup: do initial indexing once
	store, err := storage.NewSQLiteStorage(":memory:")
	if err != nil {
		b.Fatal(err)
	}
	defer store.Close()

	idx := indexer.New(store)
	config := &indexer.Config{
		IncludeTests:  true,
		IncludeVendor: false,
	}

	// Initial indexing
	_, err = idx.IndexProject(context.Background(), fixturesDir, config)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()

	// Benchmark re-indexing (should skip all files)
	for i := 0; i < b.N; i++ {
		_, err := idx.IndexProject(context.Background(), fixturesDir, config)
		if err != nil {
			b.Fatal(err)
		}
	}
}
