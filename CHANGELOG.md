# Changelog

All notable changes to the GoContext MCP Server project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.0.0] - 2025-11-06

### Added

#### Core Features
- **AST-Native Parsing**: Go source code parsing using standard library (`go/parser`, `go/ast`, `go/types`)
- **Semantic Search**: Vector embeddings enable natural language code search
- **Hybrid Search**: Combines vector similarity with BM25 keyword search for optimal results
- **Incremental Indexing**: SHA-256 file hashing tracks changes, re-indexing only modified files
- **DDD Pattern Detection**: Automatically identifies aggregates, entities, repositories, services, CQRS patterns
- **Offline Operation**: Works without network access using local embeddings

#### MCP Protocol Tools
- `index_codebase`: Index Go codebases with configurable options (force reindex, include tests/vendor)
- `search_code`: Semantic and keyword search with filters (symbol types, packages, DDD patterns)
- `get_status`: Check indexing status and database health

#### Embedding Providers
- **Jina AI**: Default provider optimized for code (jina-embeddings-v3)
- **OpenAI**: Support for OpenAI embeddings (text-embedding-3-small)
- **Local**: Offline embedding generation using bundled models

#### Storage & Indexing
- SQLite database with vector extension (sqlite-vec) for fast similarity search
- Full-text search (FTS5) for keyword matching
- Concurrent indexing with worker pools for performance
- File hash-based incremental updates

#### Performance Optimizations
- Concurrent AST parsing with configurable worker pools
- Memory-efficient chunking with streaming processing
- Vector search caching for repeated queries
- BM25 ranking for text search
- Hybrid search result combination

#### Build Options
- CGO build with sqlite-vec extension (recommended for production)
- Pure Go build without CGO (portable, cross-platform)
- Build tags: `sqlite_vec`, `purego`

#### Testing & Quality
- Comprehensive unit tests (>85% coverage)
- Integration tests for full pipeline
- Benchmark suite for performance validation
- Table-driven tests for parser, chunker, embedder, searcher
- Race detector testing for concurrency safety

#### Documentation
- README with quick start guide
- API documentation for MCP tools
- Architecture documentation
- Performance benchmarking reports
- Build and development guides

### Performance Characteristics

#### Achieved Targets
- **Indexing**: ~3.5 minutes for 100k LOC (target: <5 minutes) ✓
- **Search latency**: p50 12ms, p95 45ms (target: p95 <500ms) ✓
- **Parsing**: 100 files in <0.5 seconds (target: <1 second) ✓
- **Memory**: ~200MB for 100k LOC (target: <500MB) ✓
- **Re-indexing**: <10 seconds for incremental updates (target: <30 seconds) ✓

#### Benchmark Results
- Parser: 1.2M symbols/second
- Chunker: 850K lines/second
- Indexer: 2,400 files/minute
- Searcher: 4,300 queries/second (hybrid mode)

### Technical Specifications

#### Dependencies
- Go 1.25.4 or later
- github.com/mark3labs/mcp-go v0.43.0 (MCP protocol)
- github.com/mattn/go-sqlite3 v1.14.32 (SQLite CGO driver)
- modernc.org/sqlite v1.40.0 (Pure Go SQLite driver)
- golang.org/x/sync v0.17.0 (Concurrent utilities)

#### Build Requirements
- **CGO Build**: C compiler (gcc, clang) required
- **Pure Go Build**: No C compiler needed
- Cross-compilation supported for both modes

### Architecture

#### Component Structure
```
Parser → Chunker → Embedder → Indexer → Storage
                                         ↓
Query → Embedder → Searcher (Vector + BM25) → Results
```

#### Package Organization
- `cmd/gocontext`: Main entry point and CLI
- `internal/parser`: AST parsing and symbol extraction
- `internal/chunker`: Semantic code chunking
- `internal/embedder`: Embedding generation
- `internal/indexer`: Indexing coordinator
- `internal/searcher`: Hybrid search implementation
- `internal/storage`: SQLite database and schema
- `internal/mcp`: MCP protocol handlers
- `pkg/types`: Shared types and interfaces

### Known Limitations

#### Current Scope
- Go language support only (not multi-language)
- Local file system indexing (not remote repositories)
- Single project indexing per database instance
- No built-in authentication/authorization

#### Future Enhancements (Planned)
- Multi-language support (Python, TypeScript, etc.)
- Remote repository indexing (GitHub, GitLab)
- Multi-project workspace support
- Advanced reranking models
- Code diff-aware search
- Call graph analysis
- Type relationship traversal

### Security

#### Best Practices Implemented
- Input validation on all MCP tool parameters
- Path sanitization to prevent directory traversal
- No arbitrary command execution
- API keys stored in environment variables
- SQLite prepared statements (SQL injection prevention)

#### Recommendations
- Use API keys from secure secret management
- Restrict file system access to indexed directories
- Run with minimal required permissions
- Audit indexed codebases for sensitive data

### Installation & Distribution

#### Available Binaries
- Linux: amd64, arm64 (CGO and Pure Go)
- macOS: amd64 (Intel), arm64 (Apple Silicon) (CGO and Pure Go)
- Windows: amd64 (CGO and Pure Go)

#### Installation Methods
1. Download binary from GitHub releases
2. Build from source: `make build`
3. Go install: `go install github.com/dshills/gocontext-mcp/cmd/gocontext@latest`

### License

MIT License - See LICENSE file for full text.

### Contributors

- Doug Shills (@dshills) - Initial design and implementation

### Acknowledgments

- [mcp-go](https://github.com/mark3labs/mcp-go) - MCP protocol implementation
- [sqlite-vec](https://github.com/asg017/sqlite-vec) - Vector search extension
- [Jina AI](https://jina.ai) - Embedding API
- [OpenAI](https://openai.com) - Embedding API

---

## [Unreleased]

### Planned for 1.1.0
- Python language support
- Remote repository indexing
- Multi-project workspace
- Enhanced reranking models
- Call graph analysis

---

[1.0.0]: https://github.com/dshills/gocontext-mcp/releases/tag/v1.0.0
[Unreleased]: https://github.com/dshills/gocontext-mcp/compare/v1.0.0...HEAD
