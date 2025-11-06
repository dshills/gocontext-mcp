// Package searcher implements hybrid code search combining vector similarity and keyword matching.
//
// The searcher provides three search modes:
//   - Hybrid: Combines vector + BM25 keyword search (recommended)
//   - Vector: Pure semantic search using embeddings
//   - Keyword: BM25 full-text search only
//
// # Basic Usage
//
//	s := searcher.New(storage, embedder)
//
//	results, err := s.Search(ctx, searcher.SearchRequest{
//	    ProjectPath: "/path/to/project",
//	    Query:       "user authentication logic",
//	    Limit:       10,
//	    Mode:        searcher.ModeHybrid,
//	})
//
//	for _, result := range results {
//	    fmt.Printf("[%d] %s (score: %.2f)\n",
//	        result.Rank, result.Symbol.Name, result.RelevanceScore)
//	}
//
// # Search Modes
//
// Hybrid Mode (default, recommended):
//
//   - Combines vector similarity + BM25 keyword search
//
//   - Uses Reciprocal Rank Fusion (RRF) to merge results
//
//   - Best for most queries (semantic + exact matching)
//
//     results, _ := s.Search(ctx, searcher.SearchRequest{
//     Query: "authentication logic",
//     Mode:  searcher.ModeHybrid,
//     })
//
// Vector Mode:
//
//   - Pure semantic search using embeddings
//
//   - Best for conceptual queries ("error handling patterns")
//
//   - Requires embedding model available
//
//     results, _ := s.Search(ctx, searcher.SearchRequest{
//     Query: "async error handling patterns",
//     Mode:  searcher.ModeVector,
//     })
//
// Keyword Mode:
//
//   - BM25 full-text search only
//
//   - Best for exact symbol names ("func ParseFile")
//
//   - Faster, no embedding required (works offline)
//
//     results, _ := s.Search(ctx, searcher.SearchRequest{
//     Query: "ParseFile",
//     Mode:  searcher.ModeKeyword,
//     })
//
// # Reciprocal Rank Fusion (RRF)
//
// Hybrid mode uses RRF to combine vector and keyword results:
//
//	For each result r in vector_results:
//	    rrf_score[r.chunk_id] += 1 / (k + r.rank)
//
//	For each result r in keyword_results:
//	    rrf_score[r.chunk_id] += 1 / (k + r.rank)
//
//	Sort by rrf_score descending
//
// Where k = 60 (standard RRF constant).
//
// # Filtering
//
// Apply filters to narrow search:
//
//	results, _ := s.Search(ctx, searcher.SearchRequest{
//	    Query: "validation",
//	    Filters: searcher.Filters{
//	        SymbolTypes: []string{"function", "method"},
//	        Packages:    []string{"internal/auth"},
//	        DDDPatterns: []string{"service", "repository"},
//	        MinScore:    0.7,
//	    },
//	})
//
// Available filters:
//   - SymbolTypes: function, method, struct, interface, type
//   - Packages: Package names to include
//   - DDDPatterns: repository, service, entity, aggregate, etc.
//   - MinScore: Minimum relevance score (0.0-1.0)
//
// # Relevance Scoring
//
// Relevance scores are normalized to [0, 1]:
//   - 1.0: Perfect match
//   - 0.8-1.0: Highly relevant
//   - 0.6-0.8: Moderately relevant
//   - 0.4-0.6: Somewhat relevant
//   - <0.4: Low relevance
//
// Scores combine multiple factors:
//   - Vector similarity (cosine distance)
//   - BM25 text relevance
//   - Query term coverage
//   - Symbol name match bonus
//
// # Context Enrichment
//
// Results include surrounding context:
//
//	result.Content       // Main chunk content
//	result.Symbol        // Symbol metadata (if available)
//	result.File          // File path, package, line numbers
//	result.Context       // Context before/after for understanding
//
// Example:
//
//	fmt.Printf("Found in %s:%d\n", result.File.Path, result.File.StartLine)
//	fmt.Printf("Symbol: %s %s\n", result.Symbol.Kind, result.Symbol.Name)
//	fmt.Printf("Code:\n%s\n", result.Content)
//
// # Performance
//
// Target latency (1834 chunks indexed):
//   - Hybrid: p50=180ms, p95=420ms, p99=580ms
//   - Vector: p50=120ms, p95=350ms, p99=480ms
//   - Keyword: p50=45ms, p95=95ms, p99=130ms
//
// Latency breakdown (hybrid mode, p95):
//   - Query embedding: 80ms (API call)
//   - Vector search: 120ms (cosine similarity)
//   - BM25 search: 60ms (FTS5 index)
//   - RRF merge: 15ms (in-memory)
//   - Context loading: 145ms (SQL joins)
//
// # Caching
//
// Query embeddings are cached for repeated searches:
//
//	// First search: generates embedding (180ms)
//	results1, _ := s.Search(ctx, req)
//
//	// Repeat search: uses cached embedding (100ms)
//	results2, _ := s.Search(ctx, req)
//
// Cache expires after 1 hour or 10k entries.
//
// # Example: Advanced Search
//
//	searcher := searcher.New(storage, embedder)
//
//	results, err := searcher.Search(ctx, searcher.SearchRequest{
//	    ProjectPath: "/Users/dev/myproject",
//	    Query:       "user authentication with JWT tokens",
//	    Limit:       20,
//	    Mode:        searcher.ModeHybrid,
//	    Filters: searcher.Filters{
//	        SymbolTypes: []string{"function", "method"},
//	        Packages:    []string{"internal/auth", "pkg/jwt"},
//	        MinScore:    0.7,
//	    },
//	})
//
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	fmt.Printf("Found %d results:\n\n", len(results))
//	for _, result := range results {
//	    fmt.Printf("[%d] %.2f - %s.%s\n",
//	        result.Rank,
//	        result.RelevanceScore,
//	        result.Symbol.Package,
//	        result.Symbol.Name,
//	    )
//	    fmt.Printf("     %s:%d-%d\n",
//	        result.File.Path,
//	        result.File.StartLine,
//	        result.File.EndLine,
//	    )
//	    fmt.Printf("     %s\n\n", result.Symbol.Signature)
//	}
package searcher
