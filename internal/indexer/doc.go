// Package indexer coordinates the end-to-end indexing pipeline for Go codebases.
//
// The indexer orchestrates parsing, chunking, embedding, and storage operations,
// managing concurrency and error handling for production-scale code indexing.
//
// # Basic Usage
//
//	idx := indexer.New(storage, embedder, parser, chunker)
//
//	stats, err := idx.IndexProject(ctx, indexer.IndexOptions{
//	    Path:         "/path/to/project",
//	    ForceReindex: false,
//	    IncludeTests: true,
//	})
//
//	fmt.Printf("Indexed %d files in %v\n", stats.FilesIndexed, stats.Duration)
//
// # Indexing Pipeline
//
// The indexer executes a multi-stage pipeline:
//
//  1. Project Discovery: Find all .go files, apply exclusion filters
//  2. Incremental Decision: Compare file hashes, skip unchanged files
//  3. Parse & Chunk: Extract symbols and create semantic chunks (parallel)
//  4. Embed: Generate vector embeddings in batches
//  5. Store: Persist to SQLite database in transactions
//
// # Incremental Indexing
//
// By default, the indexer only processes changed files:
//
//	// First index: processes all files
//	stats1, _ := idx.IndexProject(ctx, opts)
//	// Files: 247 indexed, 0 skipped
//
//	// Subsequent index: only changed files
//	stats2, _ := idx.IndexProject(ctx, opts)
//	// Files: 3 indexed, 244 skipped
//
// File change detection uses SHA-256 content hashing:
//
//	currentHash := sha256.Sum256(fileContent)
//	storedHash := db.GetFileHash(filePath)
//	if currentHash == storedHash {
//	    skip(file) // unchanged
//	}
//
// Force full re-index with:
//
//	opts.ForceReindex = true
//
// # Concurrent Processing
//
// The indexer uses a worker pool for parallel file processing:
//
//	workers := runtime.NumCPU()
//	semaphore := make(chan struct{}, workers)
//
//	for _, file := range files {
//	    semaphore <- struct{}{} // acquire
//	    go func(f string) {
//	        defer func() { <-semaphore }() // release
//	        processFile(f)
//	    }(file)
//	}
//
// Default: NumCPU() workers (typically 4-16 on modern machines).
//
// # Embedding Batching
//
// Chunks are collected and embedded in batches for efficiency:
//
//	batchSize := 20 // default
//	for i := 0; i < len(chunks); i += batchSize {
//	    batch := chunks[i:min(i+batchSize, len(chunks))]
//	    embeddings, _ := embedder.GenerateBatch(ctx, batch)
//	}
//
// This reduces API calls by 20x and significantly improves throughput.
//
// # Error Handling
//
// The indexer handles errors gracefully:
//
//	stats, err := idx.IndexProject(ctx, opts)
//	// err only returned for fatal errors (e.g., storage failure)
//
//	// Check partial failures
//	if stats.FilesFailed > 0 {
//	    for _, fileErr := range stats.FailedFiles {
//	        log.Printf("Failed to index %s: %v", fileErr.Path, fileErr.Error)
//	    }
//	}
//
// Parse errors are non-fatal:
//   - Syntax errors: Continue with partial results
//   - Read errors: Skip file, log error
//   - Embedding errors: Skip chunks, continue indexing
//
// # Progress Tracking
//
// Monitor progress with callbacks:
//
//	opts.OnProgress = func(progress indexer.Progress) {
//	    fmt.Printf("Progress: %d/%d files\n",
//	        progress.FilesProcessed, progress.TotalFiles)
//	}
//
// Or channel-based updates:
//
//	progressCh := make(chan indexer.Progress)
//	go func() {
//	    for p := range progressCh {
//	        updateUI(p)
//	    }
//	}()
//	stats, err := idx.IndexProjectWithProgress(ctx, opts, progressCh)
//
// # Performance
//
// Target performance (100k LOC codebase, M1 Mac):
//   - First index: <5 minutes
//   - Incremental update (10 files): <30 seconds
//   - Memory usage: <500MB peak
//
// Bottleneck: Embedding API calls (network latency).
//
// # Example: Complete Workflow
//
//	// Setup
//	db := storage.NewSQLite("~/.gocontext/indices/project.db")
//	emb := embedder.NewJina(apiKey)
//	prs := parser.New()
//	chnk := chunker.New()
//	idx := indexer.New(db, emb, prs, chnk)
//
//	// Index with options
//	stats, err := idx.IndexProject(ctx, indexer.IndexOptions{
//	    Path:          "/Users/dev/myproject",
//	    ForceReindex:  false,
//	    IncludeTests:  true,
//	    IncludeVendor: false,
//	    Workers:       8,
//	    BatchSize:     20,
//	})
//
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	fmt.Printf(`
//	Indexing complete:
//	  Files indexed: %d
//	  Files skipped: %d
//	  Files failed:  %d
//	  Symbols extracted: %d
//	  Chunks created: %d
//	  Embeddings generated: %d
//	  Duration: %v
//	  Index size: %.2f MB
//	`, stats.FilesIndexed, stats.FilesSkipped, stats.FilesFailed,
//	   stats.SymbolsExtracted, stats.ChunksCreated,
//	   stats.EmbeddingsGenerated, stats.Duration, stats.IndexSizeMB)
package indexer
