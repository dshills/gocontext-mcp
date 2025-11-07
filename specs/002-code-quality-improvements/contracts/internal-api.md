# Internal API Contracts: Code Quality Improvements

**Feature**: 002-code-quality-improvements
**Date**: 2025-11-06

## Overview

This document specifies the internal API contract changes required for code quality fixes. All changes maintain backward compatibility at the public API level while improving internal implementations.

---

## embedder.Cache

### Get (Modified - Returns Immutable Copy)

**Purpose**: Prevent cache pollution from caller mutations.

**Signature**:
```go
func (c *Cache) Get(hash [32]byte) *Embedding
```

**Old Behavior**:
- Returns pointer to cached embedding
- Caller can mutate Vector slice, affecting all subsequent cache hits
- Data race if multiple goroutines access same cached value

**New Behavior**:
- Returns deep copy of cached embedding
- Caller mutations don't affect cache
- Thread-safe without additional locking

**Contract**:
- **Input**: `hash` - SHA-256 hash of content
- **Output**: Deep copy of embedding if found, nil otherwise
- **Guarantees**:
  - Returned embedding is independent of cached value
  - Vector slice is newly allocated
  - Concurrent Gets don't race

**Test Requirements**:
```go
// Test caller mutation doesn't affect cache
emb1 := cache.Get(hash)
emb1.Vector[0] = 999.0  // Mutate returned value
emb2 := cache.Get(hash)
assert.NotEqual(t, emb1.Vector[0], emb2.Vector[0])  // Cache unchanged
```

---

### Set (Modified - LRU Eviction)

**Purpose**: Maintain cache capacity using LRU instead of clearing all entries.

**Signature**:
```go
func (c *Cache) Set(hash [32]byte, emb *Embedding)
```

**Old Behavior**:
- Clears entire cache when capacity reached
- All prior entries lost immediately
- Cache hit rate drops to 0% on eviction

**New Behavior**:
- Evicts least recently used entry only
- Maintains recent entries for cache hits
- Gradual eviction instead of complete flush

**Contract**:
- **Input**: `hash`, `emb` - Key and value to cache
- **Output**: None
- **Guarantees**:
  - Entry stored in cache
  - If at capacity, LRU entry evicted first
  - Access time tracked for LRU ordering

**Implementation Note**: Uses `github.com/hashicorp/golang-lru` internally.

---

## storage.SQLiteStorage

### ApplyMigrations (Modified - Semantic Versioning)

**Purpose**: Apply migrations in correct order using semantic version comparison.

**Signature**:
```go
func (s *SQLiteStorage) ApplyMigrations(ctx context.Context) error
```

**Old Behavior**:
- String comparison: `"1.10.0" < "1.2.0"` (incorrect)
- Migrations applied in wrong order
- Schema corruption possible

**New Behavior**:
- Semantic version comparison: `v1.10.0 > v1.2.0` (correct)
- Migrations applied in proper sequence
- Version ordering guaranteed

**Contract**:
- **Input**: `ctx` - Cancellation context
- **Output**: Error if migrations fail
- **Guarantees**:
  - All migrations with version > current applied
  - Applied in ascending semver order
  - Schema version updated after each migration
  - Atomic per-migration (transaction wrapped)

**Error Handling**:
```go
// Distinguish "no migrations" from database errors
if err == sql.ErrNoRows {
    currentVersion = "0.0.0"  // No schema yet
} else if err != nil {
    return fmt.Errorf("failed to read schema_version: %w", err)
}
```

---

### Querier Interface (New)

**Purpose**: Abstract database operations for transaction isolation.

**Interface Definition**:
```go
type querier interface {
    QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row
    QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error)
    ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
}
```

**Contract**:
- **Implementations**:
  - `*sql.DB` - Direct database queries
  - `*sql.Tx` - Transactional queries
- **Guarantees**:
  - All methods use context for cancellation
  - Transaction reads see uncommitted writes
  - Direct DB reads see committed state only

**Usage Pattern**:
```go
// Internal method
func (s *SQLiteStorage) getProjectWithQuerier(ctx context.Context, q querier, path string) (*types.Project, error) {
    row := q.QueryRowContext(ctx, "SELECT ...", path)
    // Single implementation for both DB and Tx
}

// Public API (uses DB)
func (s *SQLiteStorage) GetProject(ctx context.Context, path string) (*types.Project, error) {
    return s.getProjectWithQuerier(ctx, s.db, path)
}

// Transaction API (uses Tx)
func (tx *sqliteTx) GetProject(ctx context.Context, path string) (*types.Project, error) {
    return tx.storage.getProjectWithQuerier(ctx, tx.tx, path)
}
```

---

### UpsertSymbol / UpsertChunk (Modified - Atomic UPSERT)

**Purpose**: Prevent race conditions in concurrent upserts using SQLite's INSERT ... ON CONFLICT.

**Old Signature**:
```go
func (s *SQLiteStorage) upsertSymbolWithQuerier(ctx context.Context, q querier, symbol *types.Symbol) error
```

**Old Behavior** (Check-Then-Act):
1. SELECT to check if symbol exists
2. If exists, UPDATE
3. If not exists, INSERT
4. **Race condition**: Two goroutines both see "not exists", both INSERT → unique constraint violation

**New Behavior** (Atomic):
```sql
INSERT INTO symbols (file_id, name, kind, start_line, start_col, ...)
VALUES (?, ?, ?, ?, ?, ...)
ON CONFLICT(file_id, name, start_line, start_col)
DO UPDATE SET
    kind = excluded.kind,
    end_line = excluded.end_line,
    ...
WHERE symbols.file_id = excluded.file_id;
```

**Contract**:
- **Input**: `ctx`, `q`, `symbol` - Context, querier, symbol data
- **Output**: Error if database operation fails
- **Guarantees**:
  - Atomic operation (no race window)
  - Either inserts new or updates existing
  - Handles concurrent attempts correctly
  - Unique constraints enforced

**Schema Requirement**:
```sql
-- Must have unique constraint
CREATE UNIQUE INDEX idx_symbols_unique
ON symbols(file_id, name, start_line, start_col);
```

---

### GetProject / GetFileByID / etc. (Modified - Transaction-Aware)

**Purpose**: Ensure reads within transactions see uncommitted writes.

**Example Signature**:
```go
func (s *SQLiteStorage) getProjectWithQuerier(ctx context.Context, q querier, rootPath string) (*types.Project, error)
```

**Old Behavior**:
- Public method calls internal method with `s.db`
- Transaction method also calls internal method with `s.db`
- Transaction reads bypass transaction context → stale data

**New Behavior**:
- Internal method accepts `querier` interface
- Public method passes `s.db`
- Transaction method passes `t.tx`
- Reads use correct context → consistent view

**Contract**:
- **Input**: `ctx`, `q`, `params` - Context, querier, query parameters
- **Output**: Query results or error
- **Guarantees**:
  - Direct DB calls see committed state
  - Transaction calls see transactional state
  - Isolation level maintained (SQLite default: SERIALIZABLE)

---

## storage.sanitizeFTSQuery

### sanitizeFTSQuery (Modified - Escape Special Characters)

**Purpose**: Prevent FTS5 SQL injection by escaping operators.

**Signature**:
```go
func sanitizeFTSQuery(query string) string
```

**Old Behavior**:
```go
func sanitizeFTSQuery(query string) string {
    return query  // No-op, injection vulnerability
}
```

**New Behavior**:
```go
func sanitizeFTSQuery(query string) string {
    // Escape FTS5 special operators
    replacer := strings.NewReplacer(
        `"`, `\"`,    // Quote
        `*`, `\*`,    // Wildcard
        `(`, `\(`,    // Grouping
        `)`, `\)`,
    )
    escaped := replacer.Replace(query)

    // Replace Boolean operators with escaped versions
    escaped = escapeFTSKeywords(escaped)  // AND, OR, NOT, NEAR

    return escaped
}
```

**Contract**:
- **Input**: `query` - User-provided search string
- **Output**: Escaped query safe for FTS5 MATCH
- **Guarantees**:
  - No FTS5 operator injection possible
  - Alphanumeric queries unchanged
  - Whitespace preserved
  - Natural language queries still work

**Test Cases**:
```go
// Injection attempts
sanitize(`function OR DROP TABLE`) → `function \OR DROP TABLE`
sanitize(`search NEAR(attack)`) → `search \NEAR\(attack\)`
sanitize(`"quoted phrase"`) → `\"quoted phrase\"`

// Normal queries
sanitize(`getUserData`) → `getUserData`  // Unchanged
sanitize(`http client`) → `http client`  // Unchanged
```

---

## indexer.Indexer

### IndexLock (New Type)

**Purpose**: Provide non-blocking lock using atomic operations.

**Type Definition**:
```go
type IndexLock struct {
    state atomic.Int32  // 0=unlocked, 1=locked
}

func (l *IndexLock) TryAcquire() bool {
    return l.state.CompareAndSwap(0, 1)
}

func (l *IndexLock) Release() {
    l.state.Store(0)
}
```

**Contract**:
- **TryAcquire()**:
  - Returns `true` if lock acquired (was unlocked)
  - Returns `false` if lock already held
  - Atomic operation, no race conditions
- **Release()**:
  - Sets state to unlocked
  - Must only be called by lock holder
  - Should be called in defer after successful acquire

**Usage Pattern**:
```go
if !idx.indexLock.TryAcquire() {
    return nil, fmt.Errorf("indexing already in progress")
}
defer idx.indexLock.Release()

// ... perform indexing ...
```

**Test Requirements**:
```go
// Test concurrent acquisition
go func() { acquired1 := lock.TryAcquire() }()
go func() { acquired2 := lock.TryAcquire() }()
// Exactly one should succeed
assert.True(t, acquired1 XOR acquired2)
```

---

## embedder.Provider (Retry Logic)

### retryWithBackoff (New Utility)

**Purpose**: Centralize exponential backoff retry logic across all providers.

**Signature**:
```go
func retryWithBackoff(ctx context.Context, fn func() error, config RetryConfig) error
```

**Parameters**:
```go
type RetryConfig struct {
    MaxRetries int
    BaseDelay  time.Duration
    MaxDelay   time.Duration
    Multiplier float64
}
```

**Contract**:
- **Input**:
  - `ctx` - Cancellation context
  - `fn` - Function to retry
  - `config` - Retry parameters
- **Output**: Error if all retries exhausted or context cancelled
- **Guarantees**:
  - Retries up to MaxRetries times
  - Exponential backoff: delay *= Multiplier each attempt
  - Caps delay at MaxDelay
  - Respects context cancellation

**Usage**:
```go
// JinaProvider
err := retryWithBackoff(ctx, func() error {
    return j.callAPI(req)
}, RetryConfig{
    MaxRetries: 3,
    BaseDelay:  100 * time.Millisecond,
    MaxDelay:   10 * time.Second,
    Multiplier: 2.0,
})
```

**Eliminates Duplication**:
- JinaProvider retry logic
- OpenAIProvider retry logic
- Future provider retry logic

---

## searcher.Searcher

### runVectorSearch / runTextSearch (Modified - Timeout Handling)

**Purpose**: Prevent deadlocks and panics in concurrent search operations.

**Signature**:
```go
func (s *Searcher) runVectorSearch(ctx context.Context, req SearchRequest, resultChan chan<- searchResult)
```

**Old Behavior**:
```go
resultChan <- res  // Can panic if channel closed
```

**New Behavior**:
```go
select {
case resultChan <- res:
    // Successful send
case <-ctx.Done():
    // Context cancelled, don't send
    return
}
```

**Contract**:
- **Input**: `ctx`, `req`, `resultChan` - Search parameters and result channel
- **Output**: None (sends result to channel)
- **Guarantees**:
  - Respects context cancellation
  - Doesn't panic on closed channel
  - Non-blocking send with timeout
  - Proper cleanup on early return

---

## mcp.Server

### NewServer (Modified - Shared Embedder)

**Purpose**: Share single embedder instance between indexer and searcher for cache efficiency.

**Old Behavior**:
```go
embedder, _ := embedder.NewFromEnv()  // For searcher only
indexer := indexer.New(store)         // Creates own embedder internally
```

**New Behavior**:
```go
embedder, _ := embedder.NewFromEnv()
indexer := indexer.NewWithEmbedder(store, embedder)  // Share instance
searcher := searcher.NewSearcher(store, embedder)    // Same instance
```

**Contract**:
- **Guarantees**:
  - Single embedder cache shared across components
  - Cache hits benefit both indexing and searching
  - Reduced memory footprint
  - Consistent embedding configuration

---

## Test Contracts

### Regression Tests (Required Per Fix)

Each fixed issue must have regression test:

```go
func TestIssue001_TryLockCompiles(t *testing.T) {
    lock := &IndexLock{}
    acquired := lock.TryAcquire()
    assert.True(t, acquired)  // First acquire succeeds
    defer lock.Release()

    acquired2 := lock.TryAcquire()
    assert.False(t, acquired2)  // Second acquire fails
}

func TestIssue003_FTS5Injection(t *testing.T) {
    malicious := `function OR 1=1 -- `
    sanitized := sanitizeFTSQuery(malicious)
    assert.NotContains(t, sanitized, " OR ")  // Operator escaped
}
```

### Performance Benchmarks

```go
func BenchmarkSortCandidates(b *testing.B) {
    candidates := generateCandidates(1000)  // Large dataset

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        sortCandidates(candidates)
    }
    // Should be O(n log n), not O(n²)
}

func BenchmarkCacheLRU(b *testing.B) {
    cache := NewCache(100)  // Small capacity

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        cache.Set(randomHash(), randomEmbedding())
    }
    // Should evict LRU, not clear all
}
```

---

## Summary

| Component | Change | Breaking? | Test Required |
|-----------|--------|-----------|---------------|
| embedder.Cache.Get | Returns copy | No | Mutation isolation |
| embedder.Cache.Set | LRU eviction | No | Hit rate benchmark |
| storage.ApplyMigrations | Semver comparison | No | Version ordering |
| storage.querier | Interface abstraction | No | Transaction isolation |
| storage.UpsertSymbol | Atomic UPSERT | No | Concurrent upserts |
| storage.sanitizeFTSQuery | Escape operators | No | Injection attempts |
| indexer.IndexLock | Atomic lock | No | Concurrent acquire |
| embedder.retryWithBackoff | Centralized retry | No | Backoff timing |
| searcher.runVectorSearch | Timeout handling | No | Context cancellation |
| mcp.NewServer | Shared embedder | No | Cache sharing |

**All changes maintain backward compatibility at public API boundaries.**
