# Research: GoContext MCP Server

**Date**: 2025-11-06
**Feature**: GoContext MCP Server
**Phase**: Phase 0 - Technology Research and Decision Documentation

## Overview

This document consolidates research findings and technology decisions for the GoContext MCP Server implementation. All critical technology choices have been evaluated based on performance targets, offline operation requirements, and Go ecosystem best practices.

## Technology Decisions

### 1. AST Parsing Library

**Decision**: Use Go standard library (`go/parser`, `go/ast`, `go/types`, `go/token`)

**Rationale**:
- **Type-aware parsing**: go/types provides full type information for accurate symbol extraction
- **Battle-tested**: Used by go tooling (gofmt, gopls, staticcheck) proven at scale
- **Zero dependencies**: No external libraries required, aligns with single-binary goal
- **Complete coverage**: Handles all Go language features including generics (Go 1.18+)
- **Performance**: Optimized C implementation via standard library

**Alternatives Considered**:
- **tree-sitter-go**: Fast but lacks type information, requires CGO
- **regex-based parsing**: Insufficient for accurate symbol extraction, violates AST-Native principle
- **go/importer**: Considered but go/types provides more complete type information

**Best Practices**:
- Use `go/packages` for module-aware parsing
- Leverage `token.FileSet` for accurate position tracking
- Cache `types.Config` to improve performance across files
- Parse with `parser.ParseComments` to extract documentation

**References**:
- https://pkg.go.dev/go/parser
- https://pkg.go.dev/go/types
- https://go.dev/blog/ast (official AST usage guide)

---

### 2. Vector Storage Solution

**Decision**: SQLite with sqlite-vec extension

**Rationale**:
- **Single file storage**: Aligns with local-first architecture, easy backup/migration
- **Vector extension**: sqlite-vec provides cosine similarity search in SQLite
- **Offline operation**: No external database server required
- **ACID transactions**: Safe concurrent access for indexing
- **Cross-platform**: Works on Linux, macOS, Windows
- **Dual build support**: CGO build (sqlite-vec) + pure Go fallback (modernc.org/sqlite)

**Alternatives Considered**:
- **PostgreSQL + pgvector**: Requires external server, violates single-binary requirement
- **Milvus/Weaviate**: Standalone vector DBs, too heavy for local deployment
- **In-memory only**: Loses data on restart, unacceptable for indexing tool
- **Custom file format**: Significant engineering effort, no transactional guarantees

**Build Configuration**:
```bash
# CGO build with vector extension (recommended)
CGO_ENABLED=1 go build -tags "sqlite_vec"

# Pure Go fallback (slower vector ops)
CGO_ENABLED=0 go build -tags "purego"
```

**Best Practices**:
- Use prepared statements for all queries
- Enable WAL mode for concurrent reads during indexing
- Create indexes on file_path, symbol_name for fast lookups
- Store embeddings as BLOB type with float32 encoding
- Implement connection pooling (max 1 writer, multiple readers)

**References**:
- https://github.com/asg017/sqlite-vec
- https://github.com/mattn/go-sqlite3
- https://modernc.org/sqlite

---

### 3. Embedding Generation

**Decision**: Pluggable provider system (Jina AI default, OpenAI optional, local fallback)

**Rationale**:
- **Jina AI v3**: Optimized for code, 8k context, good performance/cost ratio
- **OpenAI text-embedding-3-small**: High quality but costlier, wider adoption
- **Local models**: Enable fully offline operation (spago/embeddings)
- **Provider abstraction**: Easy to swap implementations

**Embedding Specifications**:
| Provider | Model | Dimensions | Context | Cost/1M tokens |
|----------|-------|------------|---------|----------------|
| Jina AI  | jina-embeddings-v3 | 1024 | 8192 | $0.02 |
| OpenAI   | text-embedding-3-small | 1536 | 8191 | $0.02 |
| Local    | sentence-transformers | 384-768 | 512 | Free |

**Best Practices**:
- Batch embed chunks (10-50 at a time) to reduce API latency
- Cache embeddings by content hash to avoid re-generating
- Implement retry logic with exponential backoff for API calls
- Store raw text alongside embeddings for re-embedding if model changes
- Use context window wisely: chunk size target 500-1000 tokens

**Alternatives Considered**:
- **Single provider (lock-in)**: Reduces flexibility, breaks offline requirement
- **Always-local**: Lower quality results, worse search accuracy
- **Cloud-only**: Violates compliance/offline requirements

**References**:
- https://jina.ai/embeddings/
- https://platform.openai.com/docs/guides/embeddings
- https://github.com/nlpodyssey/spago

---

### 4. MCP Protocol Implementation

**Decision**: Use `github.com/mark3labs/mcp-go` SDK

**Rationale**:
- **Official Go implementation**: Maintained by MCP community
- **stdio transport**: Required for Claude Code/Codex CLI integration
- **Tool definition support**: JSON schema for tool parameters
- **Active development**: Regular updates, good documentation

**MCP Tools to Implement**:
1. **index_codebase**: Trigger indexing of a Go project
   - Parameters: `path` (string), `force_reindex` (bool)
   - Returns: indexing statistics, status

2. **search_code**: Semantic/keyword search
   - Parameters: `query` (string), `limit` (int), `filters` (object)
   - Returns: ranked code chunks with context

3. **get_status**: Query indexing status
   - Parameters: none
   - Returns: indexed files count, last update time, storage size

**Best Practices**:
- Use context.Context for cancellation support
- Stream progress updates for long indexing operations
- Return structured errors with actionable messages
- Validate all tool parameters against JSON schemas
- Log MCP protocol messages for debugging

**Alternatives Considered**:
- **Custom protocol**: Significant effort, no ecosystem compatibility
- **REST API**: Requires HTTP server, heavier than stdio
- **gRPC**: Overkill for local tool, stdio simpler

**References**:
- https://github.com/mark3labs/mcp-go
- https://modelcontextprotocol.io/introduction
- https://github.com/anthropics/claude-code

---

### 5. Chunking Strategy

**Decision**: AST-aware semantic chunking

**Rationale**:
- **Function-level chunks**: Each function/method is a natural semantic unit
- **Type definition chunks**: Struct/interface with fields and methods
- **Include context**: Package declaration, imports, related types
- **Size constraints**: Target 500-1000 tokens, split large functions if needed

**Chunking Rules**:
1. **Primary**: Function or method + its doc comment
2. **Type definitions**: Struct/interface + fields + doc comment
3. **Constants/Variables**: Group related declarations together
4. **Package context**: Always include package name and relevant imports
5. **Oversized handling**: Split large functions at logical boundaries (loops, conditionals)

**Best Practices**:
- Preserve go/ast node boundaries (don't split mid-expression)
- Include surrounding context for better search results
- Store chunk metadata: start/end positions, symbol names, type
- Hash chunk content for incremental updates

**Alternatives Considered**:
- **Fixed-size chunks**: Breaks code mid-function, poor semantic boundaries
- **File-level chunks**: Too large, reduces search precision
- **Line-based**: Ignores code structure, breaks function boundaries

**References**:
- https://docs.anthropic.com/claude/docs/embeddings-guide (chunking best practices)
- https://python.langchain.com/docs/modules/data_connection/document_transformers/code_splitter

---

### 6. Hybrid Search Strategy

**Decision**: Combine vector similarity + BM25 text search with Reciprocal Rank Fusion

**Rationale**:
- **Vector search**: Handles semantic queries ("authentication logic")
- **BM25**: Handles exact/keyword queries ("func ParseFile")
- **RRF fusion**: Combines rankings without manual weight tuning
- **Complementary strengths**: Vector for meaning, BM25 for precision

**Search Pipeline**:
```
Query → [Vector Embedding] → Vector Search (cosine similarity)
     ↓
     → BM25 Text Search (keyword matching)
     ↓
     → Reciprocal Rank Fusion (merge results)
     ↓
     → Optional Reranking (Jina reranker API)
     ↓
     → Top-K Results
```

**RRF Formula**: `RRF(d) = Σ 1 / (k + rank_i(d))` where k=60

**Best Practices**:
- Run vector and BM25 searches in parallel (goroutines)
- Use FTS5 in SQLite for BM25 implementation
- Normalize scores before fusion
- Apply filters (file path, symbol type) to both searches
- Return at least 2x requested results before reranking

**Alternatives Considered**:
- **Vector-only**: Poor performance on exact matches
- **BM25-only**: Misses semantic similarity
- **Learned weights**: Requires training data, complex
- **Cohere rerank API**: Good but requires API key, Jina sufficient

**References**:
- https://plg.uwaterloo.ca/~gvcormac/cormacksigir09-rrf.pdf (RRF paper)
- https://www.sqlite.org/fts5.html (SQLite FTS5)
- https://jina.ai/reranker/

---

### 7. Concurrency Architecture

**Decision**: Worker pool pattern with semaphore and errgroup

**Rationale**:
- **Worker pool**: Limit concurrent goroutines to runtime.NumCPU()
- **Semaphore**: Buffered channel prevents goroutine explosion
- **errgroup**: Proper error propagation and context cancellation
- **File-level parallelism**: Each file parsed independently

**Concurrency Patterns**:
```go
// Worker pool for file processing
semaphore := make(chan struct{}, runtime.NumCPU())
eg, ctx := errgroup.WithContext(context.Background())

for _, file := range files {
    file := file // capture loop variable
    eg.Go(func() error {
        semaphore <- struct{}{}        // acquire
        defer func() { <-semaphore }() // release

        result, err := parseFile(ctx, file)
        if err != nil {
            return err
        }
        return storeResult(ctx, result)
    })
}

if err := eg.Wait(); err != nil {
    return err
}
```

**Best Practices**:
- Use `sync.RWMutex` for read-heavy caches (file hashes)
- Avoid shared mutable state; prefer message passing (channels)
- Test with `go test -race` to catch data races
- Use atomic operations for counters (indexed files, progress)
- Cancel context on first error to stop unnecessary work

**Alternatives Considered**:
- **Unlimited goroutines**: Risk of OOM on large codebases
- **Single-threaded**: Too slow for 100k LOC target
- **sync.WaitGroup only**: No error propagation, manual context handling
- **Worker queue library**: Adds dependency, errgroup sufficient

**References**:
- https://pkg.go.dev/golang.org/x/sync/errgroup
- https://go.dev/blog/pipelines (concurrency patterns)
- https://dave.cheney.net/2018/01/06/if-aligned-memory-writes-are-atomic-why-do-we-need-the-sync-atomic-package

---

### 8. Incremental Indexing

**Decision**: SHA-256 file content hashing with modification time fallback

**Rationale**:
- **Content hash**: Detects actual changes (not just timestamp updates)
- **SHA-256**: Fast, collision-resistant, standard library support
- **In-memory cache**: Keep hash map in memory during indexing session
- **Persistent storage**: Store hashes in SQLite for next session

**Incremental Index Flow**:
1. Load previous file hashes from SQLite
2. Walk filesystem to discover all Go files
3. Check modification time first (fast filter)
4. Compute SHA-256 for files with changed mtime
5. Compare hashes: skip unchanged files
6. Parse/chunk/embed only changed files
7. Update hashes in SQLite

**Best Practices**:
- Hash file content, not metadata
- Use `io.ReadFull` with fixed buffer for consistent hashing
- Store hash alongside file path and mtime
- Invalidate dependent chunks when imported file changes
- Prune stale entries (deleted files) from database

**Alternatives Considered**:
- **Modification time only**: False positives (touch), false negatives (git checkout)
- **MD5 hashing**: Considered cryptographically weak, no advantage
- **File-level diff**: Complex, unnecessary for index use case
- **Git-based**: Not all projects use git, breaks portability

**References**:
- https://pkg.go.dev/crypto/sha256
- https://pkg.go.dev/io/fs (filesystem walking)

---

## Performance Benchmarks (Target vs. Actual)

These benchmarks will be measured during implementation:

| Operation | Target | Measurement Method |
|-----------|--------|-------------------|
| Parse 100 files | <1s | `go test -bench BenchmarkParsing` |
| Index 100k LOC | <5min | Integration test with real codebases |
| Re-index 10 files | <30s | Incremental indexing test |
| Search latency p95 | <500ms | Load test with concurrent queries |
| Memory 100k LOC | <500MB | `runtime.ReadMemStats()` during indexing |

**Action Items**:
- Create benchmark suite in `tests/benchmarks/`
- Test with real-world codebases (Kubernetes, Docker, standard library)
- Profile with `pprof` to identify bottlenecks
- Optimize hot paths based on profiling data

---

## Compliance and Security

### Data Privacy

**Considerations**:
- All data stored locally (SQLite file)
- No telemetry or analytics by default
- Embedding API calls: only code chunks sent (not full codebase)
- User controls embedding provider (can use local models)

**Recommendations**:
- Document data flow in quickstart.md
- Provide offline-only mode (local embeddings required)
- Clear error messages when API keys missing
- Support air-gapped environments (bundled local model)

### Build Security

**Considerations**:
- CGO dependency (go-sqlite3) requires C compiler
- Pure Go build available as fallback
- Supply chain: vet all dependencies

**Recommendations**:
- Pin dependency versions in go.mod
- Use `go mod verify` in CI
- Provide checksums for released binaries
- Document CGO vs. pure Go trade-offs

---

## Open Questions (Resolved)

All initial unknowns from Technical Context have been researched and resolved:

1. ✅ **SQLite vector extension**: sqlite-vec selected, CGO + pure Go builds supported
2. ✅ **Embedding provider**: Jina AI default, OpenAI optional, local fallback available
3. ✅ **Chunking strategy**: AST-aware semantic chunking at function/type boundaries
4. ✅ **Search algorithm**: Hybrid (vector + BM25) with RRF fusion
5. ✅ **Concurrency model**: Worker pool with semaphore and errgroup
6. ✅ **Incremental updates**: SHA-256 file hashing with mtime optimization

No clarifications needed - ready to proceed to Phase 1 (Design & Contracts).

---

## Next Steps

1. Generate data-model.md with entity schemas
2. Define MCP tool contracts in contracts/
3. Create quickstart.md for user onboarding
4. Update agent context with technology choices
5. Proceed to Phase 2 (task breakdown)

**Status**: Phase 0 Research Complete ✅
