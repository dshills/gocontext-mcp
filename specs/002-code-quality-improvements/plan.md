# Implementation Plan: Comprehensive Code Quality Improvements

**Branch**: `002-code-quality-improvements` | **Date**: 2025-11-06 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `/specs/002-code-quality-improvements/spec.md`

**Note**: This template is filled in by the `/speckit.plan` command. See `.specify/templates/commands/plan.md` for the execution workflow.

## Summary

Fix 94 code quality issues identified by multi-LLM code review (Anthropic, Google, OpenAI) including 4 critical security/reliability bugs, 18 high-priority performance/safety issues, 46 medium maintainability concerns, and 26 low-priority style improvements. The technical approach prioritizes critical issues (P1) that prevent safe deployment, followed by performance/data integrity issues (P2), maintainability improvements (P3), and style consistency (P4).

## Technical Context

**Language/Version**: Go 1.25.4
**Primary Dependencies**:
- golang.org/x/crypto/bcrypt (secure password hashing)
- github.com/Masterminds/semver (semantic version comparison)
- github.com/hashicorp/golang-lru (LRU cache implementation)
- Existing: go/parser, go/ast, database/sql, github.com/mark3labs/mcp-go

**Storage**: SQLite with vector extension (sqlite-vec) for embeddings and FTS5 for text search
**Testing**: go test with race detector, >80% coverage target, integration tests use :memory: SQLite
**Target Platform**: Cross-platform (macOS, Linux, Windows) with both CGO-enabled and pure Go builds
**Project Type**: Single project (MCP server as CLI tool)
**Performance Goals**:
- Initial indexing: <5 minutes for 100k LOC
- Search latency: p95 <500ms
- Re-indexing: <30 seconds for 10 file changes
- Memory usage: <500MB for 100k LOC

**Constraints**:
- Must maintain backward compatibility with existing SQLite schemas
- All changes must pass golangci-lint with zero issues
- Must work with existing MCP protocol implementation
- Support both CGO-enabled (sqlite-vec) and pure Go (purego) builds

**Scale/Scope**: 94 issues across 38 files spanning entire codebase (internal/*, pkg/*, tests/*)

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

### I. Code Quality (NON-NEGOTIABLE)
✅ **PASS** - This feature's entire purpose is resolving linter violations and code quality issues to achieve zero golangci-lint warnings

### II. Concurrent Execution
✅ **PASS** - Fixes include timeout handling for concurrent search operations (FR-010), proper channel handling, and race condition prevention. Implementation will use concurrent testing with `-race` flag.

### III. Test-Driven Quality
✅ **PASS** - All fixes require regression tests (Success Criteria SC-004, SC-009). Each fixed issue will have corresponding test demonstrating the problem and validating the fix. Root cause fixes mandated over symptom patches.

### IV. Performance First
✅ **PASS** - Multiple fixes directly address performance targets:
- FR-007: Replace O(n²) bubble sort with O(n log n)
- FR-008: Push vector operations to database layer
- FR-009: Implement LRU cache eviction
- Success Criteria SC-002 & SC-003 validate performance maintained

### V. AST-Native Design
✅ **PASS** - FR-021 improves parser to extract partial AST results even with syntax errors, maintaining AST-native approach. No regex or text parsing introduced.

**Gate Status**: ✅ ALL GATES PASS - Feature aligns with all constitutional principles

## Project Structure

### Documentation (this feature)

```text
specs/002-code-quality-improvements/
├── plan.md              # This file (/speckit.plan command output)
├── research.md          # Phase 0 output (/speckit.plan command)
├── data-model.md        # Phase 1 output (/speckit.plan command)
├── quickstart.md        # Phase 1 output (/speckit.plan command)
├── contracts/           # Phase 1 output (/speckit.plan command)
└── tasks.md             # Phase 2 output (/speckit.tasks command - NOT created by /speckit.plan)
```

### Source Code (repository root)

```text
# Single project structure (existing)
internal/
├── chunker/          # FR-017: ContextAfter population fix
├── embedder/         # FR-009, FR-022: LRU cache, immutable copies
│   ├── embedder.go   # Cache eviction improvements
│   └── providers.go  # FR-018: Extract common retry logic
├── indexer/          # FR-001: Fix TryLock, FR-011: Embedding cleanup
│   ├── indexer.go
│   └── indexer_bench_test.go
├── mcp/              # FR-019: Share embedder instance
│   ├── server.go
│   └── tools.go      # FR-020: Precompile regex patterns
├── parser/           # FR-021: Extract partial results on syntax errors
│   └── parser.go
├── searcher/         # FR-005, FR-006, FR-010: Nil checks, caching, timeouts
│   ├── searcher.go
│   └── searcher_bench_test.go
└── storage/          # Critical fixes: FR-002, FR-003, FR-012-FR-016
    ├── migrations.go # FR-012, FR-013: Semantic versioning
    ├── sqlite.go     # FR-002, FR-014, FR-015: Connection leaks, transactions, upserts
    └── vector_ops.go # FR-003, FR-007, FR-008: FTS5 sanitization, sorting, vector ops

pkg/types/            # FR-017: Chunk.ContextAfter evaluation

tests/
├── integration/      # Add regression tests for all fixes
│   ├── indexing_test.go
│   ├── mcp_comprehensive_test.go
│   └── mock_embedder.go
└── testdata/
    └── fixtures/
        └── authentication.go  # FR-004, FR-023, FR-024: Secure auth (test fixture)

cmd/test_embedding/   # Utility maintained for testing embedder providers

.golangci.yml         # Linter configuration (already compliant)
```

**Structure Decision**: Single project structure maintained as GoContext MCP is a unified server binary. All fixes applied in-place to existing modules without architectural reorganization. Test files co-located with implementation per Go conventions.

## Complexity Tracking

> **Fill ONLY if Constitution Check has violations that must be justified**

*No violations - all constitutional gates pass.*

---

## Phase 0: Research & Technology Decisions

**Status**: ✅ COMPLETE

All technical decisions resolved during specification phase:

### Decision 1: Semantic Versioning Library
**Chosen**: `github.com/Masterminds/semver`
**Rationale**: Industry-standard library with 8k+ stars, handles semantic version comparison correctly (solves Issues #7, #9, #32)
**Alternatives Considered**:
- Custom implementation - Rejected: reinventing wheel, error-prone
- String comparison - Current broken approach

### Decision 2: LRU Cache Implementation
**Chosen**: `github.com/hashicorp/golang-lru`
**Rationale**: Production-tested by HashiCorp, thread-safe, handles eviction correctly (solves Issues #20, #24, #25, #26)
**Alternatives Considered**:
- Custom LRU - Rejected: complex to implement correctly, testing burden
- Standard map - Current broken approach (clears entire cache)

### Decision 3: Password Hashing Algorithm
**Chosen**: `golang.org/x/crypto/bcrypt`
**Rationale**: Go standard library recommended approach, adaptive cost factor, per-password salt built-in (solves Issue #4)
**Alternatives Considered**:
- scrypt - More complex, less Go-idiomatic
- Argon2id - Newer but requires more configuration
- SHA-256 - Current insecure approach

### Decision 4: Non-Blocking Lock Mechanism
**Chosen**: `sync/atomic` with `CompareAndSwap`
**Rationale**: Standard library, no allocation overhead, correct implementation of try-lock semantics (solves Issue #1)
**Alternatives Considered**:
- Channel-based semaphore - More allocations, less idiomatic
- sync.Mutex.TryLock - Does not exist in Go

### Decision 5: FTS5 Query Sanitization Strategy
**Chosen**: Escape special FTS5 operators: `AND OR NOT NEAR * " ( )`
**Rationale**: SQLite FTS5 documentation recommends escaping, prevents injection (solves Issue #3)
**Alternatives Considered**:
- Parameterized queries - Not applicable to FTS5 MATCH syntax
- Allow-list keywords - Too restrictive for natural language queries

### Decision 6: Vector Search Optimization
**Chosen**: Use sqlite-vec extension with SQL-based filtering
**Rationale**: Already available in codebase, pushes computation to database (solves Issue #16)
**Alternatives Considered**:
- In-memory Go computation - Current broken approach, memory inefficient
- External vector DB - Adds deployment complexity

### Decision 7: Transaction Isolation Pattern
**Chosen**: Refactor all storage methods to accept `querier` interface
**Rationale**: Ensures reads within transactions use transaction context (solves Issues #13, #14, #19)
**Alternatives Considered**:
- Duplicate methods - Code duplication, maintenance burden
- Context-based switching - Error-prone, implicit behavior

### Decision 8: Retry Logic Abstraction
**Chosen**: Extract common `retryWithBackoff(fn func() error, maxRetries int, baseDelay time.Duration)` function
**Rationale**: DRY principle, centralizes retry policy (solves Issue #27)
**Alternatives Considered**:
- Keep duplicated - Current approach, maintenance burden
- Third-party retry library - Overkill for simple exponential backoff

**No further research required** - all technical unknowns resolved.

---

## Phase 1: Design Artifacts

### Data Model Changes

**File**: `data-model.md`

#### Modified Entities

**CacheEntry** (new for LRU implementation)
```
CacheEntry:
  - key: [32]byte (hash)
  - value: *Embedding
  - accessTime: time.Time
  - accessCount: uint64
```

**Migration** (modified for semantic versioning)
```
Migration:
  - Version: semver.Version (changed from string)
  - SQL: string
  - Description: string
```

**Querier Interface** (new abstraction for transactions)
```
type querier interface {
  QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row
  QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error)
  ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
}
```

**IndexLock** (replace TryLock usage)
```
IndexLock:
  - state: atomic.Int32 (0=unlocked, 1=locked)

Methods:
  - TryAcquire() bool
  - Release()
```

### API Contracts

**File**: `contracts/internal-api.md`

#### Modified Internal APIs

**embedder.Cache**
```go
// OLD: Clears entire cache when full
Set(hash [32]byte, emb *Embedding)

// NEW: LRU eviction
Set(hash [32]byte, emb *Embedding)
Get(hash [32]byte) *Embedding // Returns immutable copy
```

**storage.Storage**
```go
// OLD: String version comparison
ApplyMigrations(ctx context.Context) error

// NEW: Semantic version comparison
ApplyMigrations(ctx context.Context) error // Uses semver internally

// OLD: Methods use s.db directly
GetProject(ctx context.Context, rootPath string) (*types.Project, error)

// NEW: Methods accept querier
getProjectWithQuerier(ctx context.Context, q querier, rootPath string) (*types.Project, error)
```

**storage.sanitizeFTSQuery**
```go
// OLD: No-op function
func sanitizeFTSQuery(query string) string { return query }

// NEW: Escapes FTS5 operators
func sanitizeFTSQuery(query string) string // Returns escaped query
```

**indexer.Indexer**
```go
// OLD: Uses sync.Mutex.TryLock (doesn't exist)
idx.indexMutex.TryLock()

// NEW: Uses atomic-based lock
type IndexLock struct { state atomic.Int32 }
func (l *IndexLock) TryAcquire() bool
```

### Testing Strategy

**File**: `quickstart.md`

#### Testing Approach

**Unit Tests** (per component):
- Test each fix in isolation with focused test cases
- Use table-driven tests for multiple scenarios
- Mock external dependencies (embedders, database)

**Integration Tests**:
- Full pipeline tests with :memory: SQLite
- Concurrent operation tests with `-race` flag
- Transaction isolation verification
- Cache behavior validation

**Regression Tests** (one per fixed issue):
- Test demonstrates original bug
- Test validates fix resolves issue
- Test ensures fix doesn't break existing functionality

**Performance Validation**:
- Benchmark sorting with 1000+ candidates (verify O(n log n))
- Memory profiling for vector operations
- Cache hit rate measurement
- Search latency p95 measurement

#### Running Tests

```bash
# All tests with race detection
go test -race ./...

# Coverage report
go test -cover ./... -coverprofile=coverage.out

# Specific component
go test ./internal/storage/... -v

# Benchmarks
go test -bench=. ./internal/searcher/...
```

### Quickstart for Developers

**File**: `quickstart.md` (implementation guide)

See generated quickstart.md for detailed implementation steps.

---

## Phase 2: Task Breakdown

**Status**: ⏳ PENDING (generated by `/speckit.tasks` command)

Task breakdown deferred to `/speckit.tasks` command which will:
1. Parse all functional requirements (FR-001 through FR-027)
2. Generate dependency-ordered tasks
3. Identify parallel execution opportunities
4. Create tasks.md with actionable implementation steps

---

## Constitution Check (Post-Design)

*Re-evaluation after Phase 1 design completion*

### I. Code Quality (NON-NEGOTIABLE)
✅ **PASS** - Design includes linter compliance verification, no new warnings introduced

### II. Concurrent Execution
✅ **PASS** - Atomic-based locks, proper channel handling, transaction safety maintained

### III. Test-Driven Quality
✅ **PASS** - Comprehensive test strategy defined with regression tests for each issue

### IV. Performance First
✅ **PASS** - Algorithmic improvements (O(n log n) sorting), LRU caching, database-layer vector ops

### V. AST-Native Design
✅ **PASS** - No changes to AST parsing approach, improvements to error handling only

**Final Gate Status**: ✅ ALL GATES PASS

---

## Notes

**Review Report Source**: `review-results/review-report-20251106-160257.md`
- 38 files analyzed by 3 LLM providers
- 94 issues categorized by severity and domain
- Each issue mapped to functional requirement in spec

**Implementation Priority**:
1. P1 (Critical): Issues #1-4 - Security and reliability
2. P2 (High): Issues #5-22 - Performance and data integrity
3. P3 (Medium): Issues #23-68 - Maintainability
4. P4 (Low): Issues #69-94 - Style consistency

**Dependencies Added**:
- `github.com/Masterminds/semver` v3.2.1
- `github.com/hashicorp/golang-lru` v2.0.7
- `golang.org/x/crypto` (already indirect dependency)

**Backward Compatibility**:
- SQLite schema unchanged (migrations use semver internally only)
- Public APIs unchanged (internal implementations improved)
- MCP protocol unchanged (fixes are implementation details)

**Risk Mitigation**:
- Extensive test coverage prevents regressions
- Incremental rollout by priority allows early detection
- Atomic changes per fix simplify debugging
- Race detector validates concurrency correctness
