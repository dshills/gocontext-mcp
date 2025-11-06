# GoContext MCP Server

[![Go Version](https://img.shields.io/badge/Go-1.25.4-blue.svg)](https://golang.org)
[![License](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE)

GoContext is a Model Context Protocol (MCP) server that provides symbol-aware semantic search for Go codebases. It leverages Go's native AST parsing capabilities to understand code structure, types, and domain relationships, particularly for large-scale projects using domain-driven design (DDD) patterns.

## Features

- **AST-Native Parsing**: Uses Go's standard library (`go/parser`, `go/ast`, `go/types`) for accurate symbol extraction
- **Semantic Search**: Vector embeddings enable natural language code search ("authentication logic")
- **Hybrid Search**: Combines vector similarity with BM25 keyword search for optimal results
- **Incremental Indexing**: SHA-256 file hashing tracks changes, re-indexing only modified files
- **DDD Pattern Detection**: Automatically identifies aggregates, entities, repositories, services, CQRS patterns
- **Offline Operation**: Works without network access using local embeddings
- **Single Binary**: No external dependencies, easy deployment

## Quick Start

### Installation

#### Option 1: Download Pre-built Binary (Recommended)

Download the latest release for your platform from the [releases page](https://github.com/dshills/gocontext-mcp/releases):

```bash
# macOS Apple Silicon
curl -LO https://github.com/dshills/gocontext-mcp/releases/download/v1.0.0/gocontext-darwin-arm64
chmod +x gocontext-darwin-arm64
sudo mv gocontext-darwin-arm64 /usr/local/bin/gocontext

# macOS Intel
curl -LO https://github.com/dshills/gocontext-mcp/releases/download/v1.0.0/gocontext-darwin-amd64
chmod +x gocontext-darwin-amd64
sudo mv gocontext-darwin-amd64 /usr/local/bin/gocontext

# Linux x86_64
curl -LO https://github.com/dshills/gocontext-mcp/releases/download/v1.0.0/gocontext-linux-amd64
chmod +x gocontext-linux-amd64
sudo mv gocontext-linux-amd64 /usr/local/bin/gocontext

# Verify installation
gocontext --version
```

#### Option 2: Install with Go

```bash
go install github.com/dshills/gocontext-mcp/cmd/gocontext@latest
```

**Note**: Requires Go 1.25.4+. The build mode (CGO vs Pure Go) depends on your `CGO_ENABLED` environment variable.

#### Option 3: Build from Source

```bash
# Clone the repository
git clone https://github.com/dshills/gocontext-mcp.git
cd gocontext-mcp

# Build with CGO (includes sqlite-vec extension for fast vector search)
make build

# Or build pure Go version (no C compiler needed)
make build-purego

# Binary available at bin/gocontext
```

For detailed platform-specific instructions, see [docs/installation.md](docs/installation.md).

### Build Requirements

**CGO Build (Recommended)**:
- Go 1.25.4 or later
- C compiler (gcc, clang)
- Provides faster vector search via sqlite-vec extension

**Pure Go Build**:
- Go 1.25.4 or later
- No C compiler needed
- Uses modernc.org/sqlite (pure Go SQLite implementation)
- Slightly slower vector operations

### Configuration

#### MCP Server Setup

Add GoContext to your MCP client configuration:

**For Claude Code** (`~/.config/claude-code/mcp_settings.json`):

```json
{
  "mcpServers": {
    "gocontext": {
      "command": "/path/to/gocontext-mcp/bin/gocontext",
      "args": ["serve"],
      "env": {
        "JINA_API_KEY": "your-jina-api-key"
      }
    }
  }
}
```

**For Codex CLI**:

```json
{
  "mcpServers": {
    "gocontext": {
      "command": "/path/to/gocontext-mcp/bin/gocontext",
      "args": ["serve"]
    }
  }
}
```

#### Embedding Provider Configuration

GoContext supports multiple embedding providers:

1. **Jina AI** (Default, recommended for code):
   ```bash
   export JINA_API_KEY="your-api-key"
   ```
   Get your API key at: https://jina.ai/embeddings/

2. **OpenAI**:
   ```bash
   export OPENAI_API_KEY="your-api-key"
   export GOCONTEXT_EMBEDDING_PROVIDER="openai"
   ```

3. **Local (Offline)**:
   ```bash
   export GOCONTEXT_EMBEDDING_PROVIDER="local"
   ```
   Uses bundled local model, no API key required.

## Usage

### MCP Tools

GoContext provides three MCP tools:

#### 1. `index_codebase`

Index a Go codebase for semantic search:

```json
{
  "path": "/path/to/your/go/project",
  "force_reindex": false,
  "include_tests": true,
  "include_vendor": false
}
```

**Response**:
```json
{
  "status": "success",
  "files_indexed": 245,
  "files_skipped": 12,
  "files_failed": 0,
  "chunks_created": 1834,
  "embeddings_generated": 1834,
  "duration_ms": 45230
}
```

#### 2. `search_code`

Search indexed code semantically or by keywords:

```json
{
  "path": "/path/to/your/go/project",
  "query": "authentication middleware handlers",
  "limit": 10,
  "search_mode": "hybrid",
  "filters": {
    "symbol_types": ["function", "method"],
    "packages": ["internal/auth"],
    "ddd_patterns": ["service"]
  }
}
```

**Response**:
```json
{
  "results": [
    {
      "rank": 1,
      "relevance_score": 0.89,
      "symbol": {
        "name": "AuthMiddleware",
        "kind": "function",
        "package": "internal/auth",
        "signature": "func AuthMiddleware(next http.Handler) http.Handler"
      },
      "file": "internal/auth/middleware.go",
      "content": "func AuthMiddleware(next http.Handler) http.Handler { ... }",
      "context": {
        "before": "package auth\n\nimport \"net/http\"",
        "after": "func ValidateToken(token string) bool { ... }"
      }
    }
  ],
  "total_results": 8,
  "search_duration_ms": 234,
  "cache_hit": false
}
```

#### 3. `get_status`

Check indexing status:

```json
{
  "path": "/path/to/your/go/project"
}
```

**Response**:
```json
{
  "indexed": true,
  "project": {
    "root_path": "/path/to/your/go/project",
    "module_name": "github.com/yourorg/yourproject",
    "total_files": 245,
    "total_chunks": 1834,
    "last_indexed_at": "2025-11-06T10:30:00Z"
  },
  "health": {
    "database_accessible": true,
    "fts_indexes_built": true
  }
}
```

## Development

### Project Structure

```
gocontext-mcp/
├── cmd/gocontext/          # Main entry point
├── internal/               # Internal packages
│   ├── parser/            # AST parsing and symbol extraction
│   ├── chunker/           # Code chunking for embeddings
│   ├── embedder/          # Embedding generation (Jina/OpenAI/local)
│   ├── indexer/           # Indexing coordinator
│   ├── searcher/          # Hybrid search (vector + BM25)
│   ├── storage/           # SQLite + vector extension
│   └── mcp/               # MCP protocol handlers
├── pkg/types/             # Shared types and interfaces
└── tests/                 # Unit and integration tests
    ├── unit/
    ├── integration/
    └── testdata/
```

### Build Commands

```bash
# Development build (format, lint, test, build)
make dev

# Run all tests
make test

# Run tests with race detector
make test-race

# Generate coverage report
make test-coverage

# Run linters
make lint

# Run benchmarks
make bench

# Profile CPU usage
make bench-cpu

# Profile memory usage
make bench-mem

# Full CI pipeline
make ci

# Clean build artifacts
make clean
```

### Build Tags Explained

#### CGO Build

Uses `sqlite_vec` build tag to include the sqlite-vec extension:

```bash
CGO_ENABLED=1 go build -tags "sqlite_vec" -o bin/gocontext ./cmd/gocontext
```

**Pros**:
- Fast vector similarity search (native C implementation)
- Better performance for large codebases
- Recommended for production use

**Cons**:
- Requires C compiler at build time
- Binary is platform-specific

#### Pure Go Build

Uses `purego` build tag for pure Go SQLite driver:

```bash
CGO_ENABLED=0 go build -tags "purego" -o bin/gocontext-purego ./cmd/gocontext
```

**Pros**:
- No C compiler needed
- Cross-compile to any platform
- Single static binary

**Cons**:
- Slower vector operations (pure Go implementation)
- Higher memory usage for vector search

### Testing

```bash
# Run unit tests
go test ./pkg/...
go test ./internal/...

# Run integration tests
go test ./tests/integration/...

# Run specific test
go test -v ./internal/parser -run TestParseFile

# Run with race detector
go test -race ./...

# Generate coverage
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

### Linting

```bash
# Run linters
golangci-lint run

# Auto-fix issues
golangci-lint run --fix
```

## Performance

### Targets

- **Indexing**: < 5 minutes for 100k LOC
- **Search latency**: p95 < 500ms
- **Re-indexing**: < 30 seconds for 10 file changes
- **Memory**: < 500MB for 100k LOC codebase
- **Parsing**: 100 files in < 1 second

### Benchmarking

```bash
# Run all benchmarks
make bench

# Profile specific component
go test -bench=BenchmarkParsing -benchmem ./internal/parser

# CPU profiling
make bench-cpu
go tool pprof cpu.prof

# Memory profiling
make bench-mem
go tool pprof mem.prof
```

## Architecture

### Core Components

- **Parser**: Extracts symbols, types, and signatures from Go source using AST
- **Chunker**: Divides code into semantic chunks at function/type boundaries
- **Embedder**: Generates vector embeddings (Jina AI, OpenAI, or local models)
- **Indexer**: Coordinates parsing, chunking, embedding with concurrent worker pool
- **Searcher**: Hybrid search combining vector similarity + BM25 text search
- **Storage**: SQLite database with vector extension for embeddings

### Data Flow

**Indexing Pipeline:**
```
Go Files → Parser (AST extraction) → Chunker (semantic boundaries) →
Embedder (vectors) → Storage (SQLite)
```

**Search Pipeline:**
```
Query → Embedder (vectorize) → Hybrid Search (vector + BM25) →
Optional Reranker → Top-K Results
```

## Contributing

Contributions are welcome! Please see [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## License

This project is licensed under the MIT License - see [LICENSE](LICENSE) file for details.

## Acknowledgments

- Built with [mcp-go](https://github.com/mark3labs/mcp-go) for MCP protocol support
- Uses [sqlite-vec](https://github.com/asg017/sqlite-vec) for vector search
- Embedding providers: [Jina AI](https://jina.ai), [OpenAI](https://openai.com)

## Support

- Issues: [GitHub Issues](https://github.com/dshills/gocontext-mcp/issues)
- Documentation: [docs/](docs/)
- Discussions: [GitHub Discussions](https://github.com/dshills/gocontext-mcp/discussions)
