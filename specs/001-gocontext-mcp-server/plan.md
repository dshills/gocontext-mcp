# Implementation Plan: GoContext MCP Server

**Branch**: `001-gocontext-mcp-server` | **Date**: 2025-11-06 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `/specs/001-gocontext-mcp-server/spec.md`

**Note**: This template is filled in by the `/speckit.plan` command. See `.specify/templates/commands/plan.md` for the execution workflow.

## Summary

GoContext MCP Server provides semantic code search for Go codebases through AST-based parsing and vector embeddings. The system indexes Go projects to extract symbols, types, and domain patterns, then enables natural language search via the Model Context Protocol. Core value: developers can search large codebases (100k+ LOC) with sub-500ms response times while maintaining offline operation and compliance requirements.

## Technical Context

**Language/Version**: Go 1.25.4
**Primary Dependencies**:
- `go/parser`, `go/ast`, `go/types` (standard library - AST parsing)
- `github.com/mark3labs/mcp-go` (MCP protocol implementation)
- `github.com/mattn/go-sqlite3` or `modernc.org/sqlite` (storage with vector extension)
- `golang.org/x/sync/errgroup` (concurrent processing)

**Storage**: SQLite with vector extension (sqlite-vec) for embeddings and full-text search indexes

**Testing**: Go standard testing (`go test`), golangci-lint for linting, race detector for concurrency

**Target Platform**: Cross-platform (Linux, macOS, Windows) - single binary deployment

**Project Type**: Single project (command-line tool + MCP server)

**Performance Goals**:
- Index 100k LOC in <5 minutes
- Search p95 latency <500ms
- Re-index 10 files in <30 seconds
- Memory <500MB for 100k LOC

**Constraints**:
- Offline-capable after initial setup
- Single binary with no external runtime dependencies
- Must support both CGO (for sqlite-vec) and pure Go builds
- Thread-safe for concurrent indexing and search operations

**Scale/Scope**:
- Target codebases: 10k-500k lines of Go code
- Concurrent operations: runtime.NumCPU() goroutines
- Storage: ~10-20% of codebase size for index

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

### I. Code Quality (NON-NEGOTIABLE)
- ✅ **PASS**: golangci-lint configured in `.golangci.yml` with comprehensive checks
- ✅ **PASS**: All code will pass linting before commit
- ✅ **PASS**: No linter violations tolerated in codebase

### II. Concurrent Execution
- ✅ **PASS**: Architecture designed for concurrent operation
- ✅ **PASS**: Parser, Indexer, Embedder use goroutines and worker pools
- ✅ **PASS**: errgroup used for concurrent error handling
- **Action Required**: Design all components thread-safe from start

### III. Test-Driven Quality
- ✅ **PASS**: Target >80% code coverage established
- ✅ **PASS**: Tests run frequently during development
- ✅ **PASS**: Root cause analysis for failures required
- **Action Required**: Write tests before implementation for critical paths

### IV. Performance First
- ✅ **PASS**: Explicit performance targets defined (<5min indexing, <500ms search)
- ✅ **PASS**: Targets align with constitution requirements
- **Action Required**: Benchmark critical paths (parsing, embedding, search)
- **Action Required**: Profile memory usage during indexing

### V. AST-Native Design
- ✅ **PASS**: go/parser and go/ast used for all code analysis
- ✅ **PASS**: No regex or text-based parsing for Go structures
- ✅ **PASS**: Type-aware symbol extraction via go/types

### Quality Gates
- ✅ **Pre-Commit**: Linting, tests, formatting, builds configured
- ✅ **Pre-Push**: Coverage requirements, no debug code
- ✅ **Definition of Done**: Tests, documentation, concurrency review, performance validation

**Overall Status**: ✅ PASS - All constitution requirements aligned

### Post-Phase 1 Design Review

After completing Phase 1 design (data model, contracts, quickstart), constitution compliance re-evaluated:

**I. Code Quality** - ✅ PASS (no changes)
- Architecture maintains simplicity and clarity
- No complex frameworks introduced
- Standard Go patterns throughout

**II. Concurrent Execution** - ✅ PASS (validated)
- Worker pool pattern documented in research.md
- File-level parallelism with semaphore and errgroup
- All storage operations thread-safe (SQLite WAL mode)
- Concurrent search supported (read-only operations)

**III. Test-Driven Quality** - ✅ PASS (validated)
- Test structure defined in project layout (tests/unit/, tests/integration/)
- Contract tests specified in mcp-tools.md
- Benchmark targets documented in research.md
- Performance targets measurable and testable

**IV. Performance First** - ✅ PASS (validated)
- Data model optimized for fast lookups (indexes on all FK and search fields)
- FTS5 for keyword search, vector similarity for semantic search
- Caching strategy documented (search_queries table)
- Incremental indexing via file hashing minimizes reprocessing

**V. AST-Native Design** - ✅ PASS (validated)
- Parser component uses only go/parser, go/ast, go/types
- Symbol extraction via AST traversal (no regex)
- Type information from go/types package
- DDD pattern detection uses AST attributes, not text matching

**Final Status**: ✅ PASS - All constitution requirements validated through design phase. Ready to proceed to implementation.

## Project Structure

### Documentation (this feature)

```text
specs/001-gocontext-mcp-server/
├── plan.md              # This file (/speckit.plan command output)
├── research.md          # Phase 0 output (/speckit.plan command)
├── data-model.md        # Phase 1 output (/speckit.plan command)
├── quickstart.md        # Phase 1 output (/speckit.plan command)
├── contracts/           # Phase 1 output (/speckit.plan command)
│   ├── mcp-tools.md     # MCP protocol tool definitions
│   └── storage-api.md   # Internal storage interface contracts
└── tasks.md             # Phase 2 output (/speckit.tasks command - NOT created by /speckit.plan)
```

### Source Code (repository root)

```text
cmd/
└── gocontext/
    └── main.go          # Entry point, MCP server initialization

internal/
├── parser/              # AST parsing and symbol extraction
│   ├── parser.go
│   ├── symbols.go
│   └── ddd.go           # DDD pattern detection
├── chunker/             # Code chunking for embeddings
│   ├── chunker.go
│   └── strategies.go
├── embedder/            # Embedding generation (Jina/OpenAI/local)
│   ├── embedder.go
│   ├── providers.go
│   └── cache.go
├── indexer/             # Coordination of parse+chunk+embed+store
│   ├── indexer.go
│   ├── worker_pool.go
│   └── incremental.go   # File hash tracking
├── searcher/            # Hybrid search (vector + BM25)
│   ├── searcher.go
│   ├── vector.go
│   ├── text.go
│   └── reranker.go
├── storage/             # SQLite + vector extension
│   ├── storage.go
│   ├── sqlite.go
│   ├── migrations.go
│   └── vector_ops.go
└── mcp/                 # MCP protocol handlers
    ├── server.go
    ├── tools.go         # index_codebase, search_code, get_status
    └── schemas.go

pkg/
└── types/               # Shared types and interfaces
    ├── symbol.go
    ├── chunk.go
    └── result.go

tests/
├── integration/         # Full pipeline tests
│   ├── indexing_test.go
│   ├── search_test.go
│   └── mcp_test.go
├── unit/                # Component unit tests
│   ├── parser/
│   ├── chunker/
│   ├── embedder/
│   ├── indexer/
│   ├── searcher/
│   └── storage/
└── testdata/            # Sample Go codebases for testing
    └── fixtures/
```

**Structure Decision**: Single project structure selected because:
- Single binary deployment model
- All components are part of one cohesive MCP server
- No web frontend or separate API server needed
- Internal packages prevent external API surface exposure
- cmd/ contains single entry point
- pkg/ exposes shared types (minimal public API)

## Complexity Tracking

**No complexity violations** - This plan complies with all constitution requirements and maintains simplicity:

- Single project structure (not multi-repo or microservices)
- Standard Go project layout (cmd/, internal/, pkg/, tests/)
- Direct SQLite storage (no separate database server)
- goroutines and errgroup (standard Go concurrency)
- No complex frameworks or patterns beyond MCP protocol requirement

All design choices prioritize simplicity, performance, and Go idiomatic patterns per Constitution Principle V.
