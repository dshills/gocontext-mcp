# Embedder Component

The embedder component provides a pluggable system for generating vector embeddings from code chunks.

## Features

- **Multiple Providers**: Support for Jina AI, OpenAI, and local embeddings
- **Automatic Provider Selection**: Detects available API keys and selects provider
- **Batch Processing**: Efficiently process 10-50 chunks per API call
- **Caching**: In-memory cache by content hash to avoid regenerating embeddings
- **Retry Logic**: Exponential backoff for API failures (max 3 retries)
- **Provider Metadata**: Store provider/model information with embeddings

## Usage

### Basic Usage

```go
import "github.com/dshills/gocontext-mcp/internal/embedder"

// Auto-detect provider from environment
embedder, err := embedder.NewFromEnv()
if err != nil {
    log.Fatal(err)
}
defer embedder.Close()

// Generate single embedding
req := embedder.EmbeddingRequest{
    Text: "func ProcessData(input []byte) (string, error) { ... }",
}
emb, err := embedder.GenerateEmbedding(ctx, req)
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Generated %d-dimensional embedding\n", emb.Dimension)
```

### Batch Processing

```go
// Generate embeddings for multiple chunks efficiently
texts := []string{
    "chunk1 code",
    "chunk2 code",
    "chunk3 code",
}

req := embedder.BatchEmbeddingRequest{
    Texts: texts,
}

resp, err := embedder.GenerateBatch(ctx, req)
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Generated %d embeddings using %s\n",
    len(resp.Embeddings), resp.Provider)
```

### Explicit Provider Configuration

```go
// Use specific provider
cfg := embedder.Config{
    Provider:  embedder.ProviderJina,
    APIKey:    "your-api-key",
    CacheSize: 10000,
}

emb, err := embedder.New(cfg)
if err != nil {
    log.Fatal(err)
}
defer emb.Close()
```

## Environment Variables

The embedder supports the following environment variables:

### Provider Selection

- `GOCONTEXT_EMBEDDING_PROVIDER`: Explicitly set provider (`jina`, `openai`, or `local`)
- `JINA_API_KEY`: Jina AI API key (auto-selects Jina provider)
- `OPENAI_API_KEY`: OpenAI API key (auto-selects OpenAI provider)

### Selection Priority

1. If `GOCONTEXT_EMBEDDING_PROVIDER` is set, use that provider
2. Else if `JINA_API_KEY` is set, use Jina provider
3. Else if `OPENAI_API_KEY` is set, use OpenAI provider
4. Else fallback to local provider (stub implementation)

## Providers

### Jina AI (Default)

- **Model**: `jina-embeddings-v3`
- **Dimensions**: 1024
- **Context**: 8192 tokens
- **Cost**: $0.02 per 1M tokens
- **Best for**: Code embeddings with good quality/cost ratio

```bash
export JINA_API_KEY="your-jina-api-key"
```

### OpenAI

- **Model**: `text-embedding-3-small`
- **Dimensions**: 1536
- **Context**: 8191 tokens
- **Cost**: $0.02 per 1M tokens
- **Best for**: High-quality embeddings, wide adoption

```bash
export OPENAI_API_KEY="your-openai-api-key"
```

### Local (Placeholder)

- **Dimensions**: 384
- **Context**: N/A (deterministic hash-based)
- **Cost**: Free
- **Best for**: Offline operation, testing

**Note**: The local provider is currently a stub that generates deterministic vectors based on text hash. For production offline use, integrate with a local model like sentence-transformers.

## Caching

Embeddings are cached in-memory by content hash (SHA-256):

- **Default cache size**: 10,000 embeddings
- **Eviction policy**: Simple clear when capacity reached
- **Benefits**: Avoid re-embedding unchanged chunks during incremental indexing

```go
// Check cache size
cache := embedder.NewCache(5000) // Custom size
provider, _ := embedder.NewJinaProvider(apiKey, cache)

// Embeddings automatically cached on generation
```

## Error Handling

The embedder provides structured errors:

- `ErrEmptyText`: Text cannot be empty
- `ErrBatchTooLarge`: Batch exceeds max size (100 texts)
- `ErrProviderFailed`: API call failed after retries
- `ErrNoProviderEnabled`: No API key configured

```go
emb, err := provider.GenerateEmbedding(ctx, req)
if err != nil {
    switch {
    case errors.Is(err, embedder.ErrEmptyText):
        // Handle validation error
    case errors.Is(err, embedder.ErrProviderFailed):
        // Handle API failure
    default:
        // Handle other errors
    }
}
```

## Performance

Based on benchmarks:

- **Hash computation**: ~80-110 ns/op
- **Cache get/set**: ~45-50 ns/op
- **Single embedding (local)**: ~115 ns/op
- **Batch-10 (local)**: ~1 µs/op
- **Batch-50 (local)**: ~5 µs/op

For API-based providers (Jina, OpenAI):
- **Single embedding**: 100-300ms (network latency)
- **Batch-50**: 200-500ms (efficient batching)

**Recommendation**: Always use batch processing for multiple chunks.

## Testing

Run tests:
```bash
go test ./internal/embedder/... -v
```

Run benchmarks:
```bash
go test ./internal/embedder/... -bench=. -benchmem
```

Check coverage:
```bash
go test ./internal/embedder/... -cover
```

## Integration Example

```go
// Indexer integration
type Indexer struct {
    embedder embedder.Embedder
}

func (i *Indexer) IndexChunks(ctx context.Context, chunks []*Chunk) error {
    // Collect texts
    texts := make([]string, len(chunks))
    for i, chunk := range chunks {
        texts[i] = chunk.Content
    }

    // Batch embed
    resp, err := i.embedder.GenerateBatch(ctx, embedder.BatchEmbeddingRequest{
        Texts: texts,
    })
    if err != nil {
        return fmt.Errorf("generate embeddings: %w", err)
    }

    // Store embeddings
    for i, emb := range resp.Embeddings {
        chunks[i].Embedding = emb.Vector
        chunks[i].EmbeddingProvider = emb.Provider
        chunks[i].EmbeddingModel = emb.Model
    }

    return nil
}
```

## Future Enhancements

- [ ] Integrate with local sentence-transformers model
- [ ] Support for custom embedding dimensions
- [ ] LRU cache eviction policy
- [ ] Persistent cache (SQLite)
- [ ] Progress callbacks for batch operations
- [ ] Retry configuration (max attempts, backoff)
- [ ] Support for additional providers (Cohere, HuggingFace)
