# Data Model: Code Quality Improvements

**Feature**: 002-code-quality-improvements
**Date**: 2025-11-06

## Overview

This document describes data model changes required for fixing 94 code quality issues. Most changes involve internal type modifications to support correct implementations (LRU caching, semantic versioning, transaction isolation).

## Modified Entities

### CacheEntry (New)

**Purpose**: Support LRU eviction policy for embedding cache instead of clearing entire cache.

**Fields**:
- `key`: `[32]byte` - SHA-256 hash of cached content
- `value`: `*Embedding` - Cached embedding vector and metadata
- `accessTime`: `time.Time` - Last access timestamp for LRU ordering
- `accessCount`: `uint64` - Access frequency counter (optional for LFU hybrid)

**Relationships**:
- Managed by `embedder.Cache` instance
- Multiple CacheEntry instances form LRU queue

**Validation Rules**:
- Key must be non-zero (computed via SHA-256)
- Value must be non-nil when stored
- AccessTime updated on every Get operation

**State Transitions**:
1. **New** → Entry created on cache miss, embedding generated
2. **Accessed** → AccessTime updated on cache hit
3. **Evicted** → Removed when cache full and entry is least recently used

**Implementation Note**: Will use `github.com/hashicorp/golang-lru` which handles LRU queue internally.

---

### Migration (Modified)

**Purpose**: Enable correct semantic version comparison for database migrations instead of broken string comparison.

**Fields** (changed):
- `Version`: `*semver.Version` (was `string`) - Semantic version for ordering
- `SQL`: `string` - Migration SQL statements (unchanged)
- `Description`: `string` - Human-readable description (unchanged)

**Validation Rules**:
- Version must parse as valid semantic version (MAJOR.MINOR.PATCH)
- SQL must be non-empty
- Version must be unique across all migrations

**Comparison Logic**:
```go
// OLD (broken):
if migration.Version <= currentVersion { skip }

// NEW (correct):
if migration.Version.LessThanOrEqual(currentVersion) { skip }
```

**Migration Path**:
- Existing versions stored as strings in schema_version table
- Parse strings to semver.Version at runtime
- No database schema changes required

---

### Querier (New Interface)

**Purpose**: Abstract database operations to support both direct DB access and transactional access with proper isolation.

**Interface Definition**:
```go
type querier interface {
    QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row
    QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error)
    ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
}
```

**Implementations**:
- `*sql.DB` - Implements querier for direct database access
- `*sql.Tx` - Implements querier for transactional access

**Usage Pattern**:
```go
// Internal method accepts querier
func (s *SQLiteStorage) getProjectWithQuerier(ctx context.Context, q querier, rootPath string) (*types.Project, error) {
    row := q.QueryRowContext(ctx, "SELECT ...", rootPath)
    // ...
}

// Public method uses s.db
func (s *SQLiteStorage) GetProject(ctx context.Context, rootPath string) (*types.Project, error) {
    return s.getProjectWithQuerier(ctx, s.db, rootPath)
}

// Transaction method uses tx
func (t *sqliteTx) GetProject(ctx context.Context, rootPath string) (*types.Project, error) {
    return t.storage.getProjectWithQuerier(ctx, t.tx, rootPath)
}
```

---

### IndexLock (New)

**Purpose**: Provide non-blocking lock semantics using atomic operations to replace non-existent sync.Mutex.TryLock.

**Fields**:
- `state`: `atomic.Int32` - Lock state (0=unlocked, 1=locked)

**Methods**:
```go
func (l *IndexLock) TryAcquire() bool {
    return l.state.CompareAndSwap(0, 1)
}

func (l *IndexLock) Release() {
    l.state.Store(0)
}
```

**Validation Rules**:
- TryAcquire returns true only if lock was previously unlocked
- Release must only be called by lock holder
- Lock must be released in defer after successful acquisition

**State Transitions**:
1. **Unlocked (0)** → TryAcquire succeeds, transitions to Locked
2. **Locked (1)** → TryAcquire fails, remains Locked
3. **Locked (1)** → Release transitions to Unlocked

**Usage Pattern**:
```go
if !idx.indexLock.TryAcquire() {
    return nil, fmt.Errorf("indexing already in progress")
}
defer idx.indexLock.Release()
```

---

### Embedding (Modified - Immutability)

**Purpose**: Prevent cache pollution by returning immutable copies instead of pointers to mutable cached values.

**Fields** (unchanged):
- `Vector`: `[]float32` - Embedding vector
- `Dimension`: `int` - Vector dimension
- `Provider`: `string` - Embedder provider name
- `Model`: `string` - Model identifier
- `Hash`: `[32]byte` - Content hash

**Behavior Change**:
- `Cache.Get()` now returns deep copy instead of pointer to cached value
- Callers can mutate returned embedding without affecting cache

**Implementation**:
```go
// OLD (unsafe):
func (c *Cache) Get(hash [32]byte) *Embedding {
    c.mu.RLock()
    defer c.mu.RUnlock()
    return c.embeddings[hash] // Pointer to cached value
}

// NEW (safe):
func (c *Cache) Get(hash [32]byte) *Embedding {
    c.mu.RLock()
    defer c.mu.RUnlock()
    cached := c.embeddings[hash]
    if cached == nil {
        return nil
    }
    // Deep copy
    return &Embedding{
        Vector:    append([]float32{}, cached.Vector...),
        Dimension: cached.Dimension,
        Provider:  cached.Provider,
        Model:     cached.Model,
        Hash:      cached.Hash,
    }
}
```

---

## Internal Type Changes

### RetryConfig (New)

**Purpose**: Centralize retry logic parameters for embedder providers.

**Fields**:
- `maxRetries`: `int` - Maximum retry attempts
- `baseDelay`: `time.Duration` - Initial backoff delay
- `maxDelay`: `time.Duration` - Maximum backoff delay
- `multiplier`: `float64` - Exponential backoff multiplier

**Default Configuration**:
```go
defaultRetryConfig := RetryConfig{
    maxRetries: 3,
    baseDelay:  100 * time.Millisecond,
    maxDelay:   10 * time.Second,
    multiplier: 2.0,
}
```

---

### SearchCandidate (Modified Sorting)

**Purpose**: Sort search candidates efficiently using stdlib sort.Slice instead of O(n²) bubble sort.

**Fields** (unchanged):
- `chunkID`: `int64`
- `score`: `float64`

**Sorting Change**:
```go
// OLD (bubble sort - O(n²)):
func sortCandidates(candidates []candidate) {
    for i := 0; i < len(candidates); i++ {
        for j := i + 1; j < len(candidates); j++ {
            if candidates[j].score > candidates[i].score {
                candidates[i], candidates[j] = candidates[j], candidates[i]
            }
        }
    }
}

// NEW (sort.Slice - O(n log n)):
func sortCandidates(candidates []candidate) {
    sort.Slice(candidates, func(i, j int) bool {
        return candidates[i].score > candidates[j].score
    })
}
```

---

## Schema Changes

**No database schema migrations required**. All changes are in-memory data structures or internal type representations.

### Explanation:
- **Migrations**: String → semver.Version conversion happens at runtime, schema_version table unchanged
- **Cache**: LRU entries managed in memory, no persistent storage
- **Querier**: Interface abstraction, no new database tables
- **IndexLock**: In-memory atomic state, no persistence

### Future Considerations:
If query result caching (FR-006) is implemented with persistent storage:
```sql
CREATE TABLE IF NOT EXISTS query_cache (
    query_hash BLOB PRIMARY KEY,
    results BLOB,  -- Serialized SearchResponse
    created_at INTEGER,
    expires_at INTEGER
);

CREATE INDEX idx_query_cache_expiry ON query_cache(expires_at);
```

---

## Type Safety Improvements

### FTS5 Query Sanitization

**Purpose**: Prevent SQL injection by escaping special FTS5 operators.

**Type**:
```go
type FTS5Query struct {
    raw      string
    sanitized string
}

func NewFTS5Query(raw string) FTS5Query {
    return FTS5Query{
        raw:       raw,
        sanitized: sanitizeFTSQuery(raw),
    }
}
```

**Validation**:
- Escapes characters: `(`, `)`, `"`, `*`, `AND`, `OR`, `NOT`, `NEAR`
- Preserves whitespace and basic alphanumeric queries
- Returns escaped string safe for MATCH clause

---

## Relationships

### Cache ↔ Embedder
- One Embedder has one Cache instance
- Cache stores multiple CacheEntry instances
- LRU eviction maintains cache capacity limits

### Storage ↔ Querier
- Storage methods accept querier interface
- DB operations route through querier (direct or transactional)
- Ensures consistent view within transactions

### Indexer ↔ IndexLock
- One Indexer has one IndexLock instance
- Lock prevents concurrent indexing attempts
- Non-blocking TryAcquire allows graceful rejection

### Migration ↔ Semver
- Each Migration has one semver.Version
- Versions compared using semver semantics
- Ordering determines migration application sequence

---

## Validation Summary

| Entity | Key Validation |
|--------|----------------|
| CacheEntry | Non-zero key, non-nil value, updated accessTime |
| Migration | Valid semver format, unique version |
| Querier | Implements required interface methods |
| IndexLock | Atomic state transitions, proper release |
| Embedding | Deep copy on Get, immutable to callers |
| FTS5Query | Escaped special operators, safe for SQL |

---

## Testing Considerations

### Unit Tests Required:
- CacheEntry LRU eviction ordering
- Migration semver comparison (1.10.0 > 1.2.0)
- Querier interface implementations (DB and Tx)
- IndexLock concurrent acquisition attempts
- Embedding deep copy isolation
- FTS5Query sanitization edge cases

### Integration Tests Required:
- Transaction reads see uncommitted writes (querier isolation)
- Cache hit rate with LRU eviction
- Migration application in correct order
- Concurrent indexing with IndexLock
- Search with sanitized FTS5 queries

---

## Implementation Priority

**P1 (Critical)**:
- IndexLock (FR-001) - Compilation blocker
- FTS5Query sanitization (FR-003) - Security vulnerability
- Querier interface (FR-002, FR-014) - Transaction isolation

**P2 (High)**:
- Migration semver (FR-012) - Data integrity
- CacheEntry LRU (FR-009) - Performance
- Embedding immutability (FR-022) - Concurrency safety

**P3 (Medium)**:
- RetryConfig (FR-018) - Code maintainability
- SearchCandidate sorting (FR-007) - Performance optimization

---

**Notes**:
- All type changes maintain backward compatibility at API level
- Internal representations change but external interfaces stable
- No breaking changes to MCP protocol or public types
- Migration to semver is transparent to database consumers
