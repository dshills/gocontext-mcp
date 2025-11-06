# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

GoContext is a Model Context Protocol (MCP) server that provides symbol-aware semantic search for Go codebases. It leverages Go's native AST parsing capabilities to understand code structure, types, and domain relationships, particularly for large-scale projects using domain-driven design (DDD) patterns.

### Key Characteristics
- **Go-native AST parsing** using `go/parser`, `go/ast`, `go/types`
- **Local-first architecture** with SQLite + vector extensions
- **Concurrent indexing** using goroutines for performance
- **Single binary deployment** with no external runtime dependencies

## Build & Test Commands

### Building
```bash
# SQLite with vector extension (requires CGO)
CGO_ENABLED=1 go build -tags "sqlite_vec"

# Pure Go build (no CGO, alternative driver)
CGO_ENABLED=0 go build -tags "purego"
```

### Testing
```bash
# Run all tests
go test ./...

# Run tests with coverage (target: >80%)
go test -cover ./...

# Run tests with race detector
go test -race ./...

# Run specific component tests
go test ./pkg/parser/...
go test ./pkg/chunker/...
go test ./pkg/embedder/...
```

### Linting
```bash
# Run golangci-lint with project configuration
golangci-lint run

# Configuration is in .golangci.yml
# Key enabled linters: gofmt, govet, staticcheck, errcheck, gosimple, ineffassign, unused, gocyclo, misspell, goconst
```

## Architecture

### Component Layers

**MCP Handler Layer** (uses github.com/mark3labs/mcp-go)
- Implements MCP protocol tools: `index_codebase`, `search_code`, `get_status`
- Communicates via stdio with MCP clients (Claude Code, Codex CLI)

**Core Engine Layer**
- **Parser**: Extracts symbols, types, signatures from Go source using AST
- **Chunker**: Divides code into semantic chunks at function/type boundaries
- **Embedder**: Generates vector embeddings (Jina AI, OpenAI, or local models)
- **Indexer**: Coordinates parsing, chunking, embedding with concurrent worker pool
- **Searcher**: Hybrid search combining vector similarity + BM25 text search

**Storage Layer**
- SQLite database with vector extension for embeddings
- In-memory file hash cache for incremental updates
- Text search indexes for keyword matching

### Data Flow

**Indexing Pipeline:**
Go Files → Parser (AST extraction) → Chunker (semantic boundaries) → Embedder (vectors) → Storage (SQLite)

**Search Pipeline:**
Query → Embedder (vectorize) → Hybrid Search (vector + BM25) → Optional Reranker → Top-K Results

### Domain-Driven Design (DDD) Pattern Detection

The parser identifies DDD patterns by naming conventions:
- Aggregates: Types with "Aggregate" suffix
- Entities: Types with "Entity" suffix or "ID" field
- Value Objects: Immutable types with value semantics
- Repositories: Types/interfaces with "Repository" suffix
- Services: Types with "Service" suffix
- CQRS: Types with "Command", "Query", "Handler" suffixes

## Performance Targets

- **Indexing**: < 5 minutes for 100k LOC
- **Search latency**: p95 < 500ms
- **Re-indexing**: < 30 seconds for 10 file changes
- **Memory**: < 500MB for 100k LOC codebase
- **Parsing**: 100 files in < 1 second
- **Search accuracy**: >90% recall, >80% precision

## Code Organization Principles

### Symbol Extraction
When parsing Go code, extract:
- Functions and methods (with receiver types for methods)
- Structs and interfaces
- Type aliases and custom types
- Doc comments
- Scope (exported vs unexported via `token.IsExported()`)
- Signatures (parameters, return types)

### Chunking Strategy
Create chunks that:
- Respect function/type boundaries
- Include surrounding context (imports, package declaration)
- Preserve relationships between symbols
- Hash content using `crypto/sha256` for incremental updates
- Target semantic completeness over arbitrary size limits

### Concurrent Processing
Use worker pools with bounded concurrency:
- Semaphore channel pattern: `semaphore := make(chan struct{}, workers)`
- `errgroup` for error propagation in concurrent operations
- Max workers: `runtime.NumCPU()` goroutines

## Dependencies

### Core Go Packages
- `go/parser`, `go/ast`, `go/token`, `go/types`, `go/doc` for AST analysis
- `crypto/sha256` for file hashing
- `database/sql` for SQLite interaction

### External Dependencies
- `github.com/mark3labs/mcp-go` - MCP protocol implementation
- `github.com/mattn/go-sqlite3` - SQLite driver (CGO required for vector extension)
- `modernc.org/sqlite` - Pure Go SQLite alternative
- `golang.org/x/sync/errgroup` - Concurrent processing helpers

## Testing Strategy

### Unit Tests
- Coverage target: >80%
- Test structure: table-driven tests with `name`, `input`, `expected`, `wantErr` fields
- Focus areas: Parser (AST extraction), Chunker (boundary detection), Embedder (API mocking), Storage (CRUD), Searcher (ranking)

### Integration Tests
- Full pipeline: Parse → Chunk → Embed → Store
- Search pipeline: Query → Embed → Search → Rerank
- Incremental reindexing
- MCP tool integration
- Use `:memory:` SQLite databases for test isolation

### Test File Conventions
- Test files can skip error checking for brevity (see `.golangci.yml`)
- Higher complexity allowed in test files for comprehensive cases

## SpecKit Workflow

This project uses SpecKit for specification-driven development:
- Specifications in `specs/` directory
- SpecKit commands available in `.claude/commands/`
- Constitution and templates in `.specify/` directory
- Use `/speckit.*` slash commands for specification workflow

## Project Status

**Version:** 1.0.0
**Status:** Design Phase
**Go Version:** 1.25.4
**Target:** Production-ready binary for 100k+ LOC codebases

## Active Technologies
- Go 1.25.4 (001-gocontext-mcp-server)
- SQLite with vector extension (sqlite-vec) for embeddings and FTS5 for text search
- github.com/Masterminds/semver/v3 for semantic versioning (002-code-quality-improvements)
- github.com/hashicorp/golang-lru/v2 for LRU caching (002-code-quality-improvements)
- golang.org/x/crypto/bcrypt for password hashing (002-code-quality-improvements)

## Recent Changes
- 002-code-quality-improvements: Implemented 42 critical code quality fixes including security hardening (FTS5 injection protection, bcrypt hashing), performance optimizations (O(n log n) sorting, LRU cache), data integrity (atomic UPSERT, semantic versioning), and maintainability improvements (retry logic extraction, shared embedder instance)
- 001-gocontext-mcp-server: Added Go 1.25.4
- when creating binaries put them in the ./bin directory
