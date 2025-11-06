// Package embedder generates vector embeddings for code chunks using various providers.
//
// The embedder supports multiple embedding providers (Jina AI, OpenAI, local models)
// and provides batching, caching, and error handling for production use.
//
// # Basic Usage
//
//	// Create embedder (auto-detects provider from environment)
//	emb, err := embedder.New(context.Background())
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer emb.Close()
//
//	// Generate single embedding
//	result, err := emb.GenerateEmbedding(ctx, embedder.EmbeddingRequest{
//	    Text: "func ParseFile(path string) error { ... }",
//	})
//	fmt.Printf("Vector dimension: %d\n", len(result.Vector))
//
// # Batch Processing
//
// For efficiency, use batch processing:
//
//	texts := []string{
//	    chunk1.FullContent(),
//	    chunk2.FullContent(),
//	    chunk3.FullContent(),
//	}
//
//	resp, err := emb.GenerateBatch(ctx, embedder.BatchEmbeddingRequest{
//	    Texts: texts,
//	})
//
//	for i, embedding := range resp.Embeddings {
//	    // Store embedding for chunk i
//	}
//
// Batching reduces API calls and improves throughput significantly
// (e.g., 20x faster than sequential single requests).
//
// # Provider Selection
//
// The embedder selects a provider based on environment variables:
//
//  1. If GOCONTEXT_EMBEDDING_PROVIDER is set → use specified provider
//  2. Else if JINA_API_KEY is set → use Jina AI
//  3. Else if OPENAI_API_KEY is set → use OpenAI
//  4. Else → fallback to local provider (offline mode)
//
// Provider configuration:
//
//	// Explicit provider selection
//	os.Setenv("GOCONTEXT_EMBEDDING_PROVIDER", "jina")
//	os.Setenv("JINA_API_KEY", "your-api-key")
//
//	// Or use factory
//	config := embedder.Config{
//	    Provider: "jina",
//	    APIKey:   "your-api-key",
//	    BatchSize: 20,
//	}
//	emb := embedder.NewFromConfig(config)
//
// # Provider Comparison
//
// Jina AI (recommended for code):
//   - Dimensions: 1024
//   - Quality: Excellent (code-optimized)
//   - Speed: Fast
//   - Cost: Free tier available
//
// OpenAI:
//   - Dimensions: 1536
//   - Quality: Excellent (general purpose)
//   - Speed: Fast
//   - Cost: Pay per token
//
// Local (offline):
//   - Dimensions: 384
//   - Quality: Good
//   - Speed: Medium
//   - Cost: Free (CPU-based)
//
// # Caching
//
// The embedder includes an in-memory cache:
//
//	cache := embedder.NewCache(10000) // cache 10k embeddings
//
//	// Hash-based lookup
//	hash := computeHash(text)
//	if emb, ok := cache.Get(hash); ok {
//	    return emb // cache hit
//	}
//
//	// Generate and cache
//	emb := generateEmbedding(text)
//	cache.Set(hash, emb)
//
// # Error Handling
//
// The embedder handles transient failures with retry logic:
//
//	emb, err := embedder.GenerateBatch(ctx, req)
//	if errors.Is(err, embedder.ErrProviderFailed) {
//	    // API temporarily unavailable, retry later
//	}
//
// For offline scenarios, fallback to local provider:
//
//	emb, err := embedder.New(ctx)
//	if err != nil {
//	    // Fallback to local provider
//	    emb = embedder.NewLocal()
//	}
//
// # Performance
//
// Typical throughput (Jina AI, batch size 20):
//   - Single request: ~200ms (network latency)
//   - Batch of 20: ~400ms (2x slower, 20x more throughput)
//   - Concurrent batches (5 parallel): ~90 embeddings/sec
//
// For local provider:
//   - Single request: ~50ms (CPU-bound)
//   - No batching benefit (already local)
//   - Throughput: ~20 embeddings/sec
package embedder
