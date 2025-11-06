# Quickstart: Code Quality Improvements Implementation

**Feature**: 002-code-quality-improvements
**Branch**: `002-code-quality-improvements`
**Date**: 2025-11-06

## Overview

This guide provides developers with step-by-step instructions for implementing all 94 code quality fixes organized by priority. Follow the priority order to address critical issues first.

## Prerequisites

### Required Dependencies

Add to `go.mod`:
```bash
go get github.com/Masterminds/semver/v3@v3.2.1
go get github.com/hashicorp/golang-lru/v2@v2.0.7
# golang.org/x/crypto already available as indirect dependency
```

### Development Tools

```bash
# Linter (must pass with zero issues)
golangci-lint --version  # Should be v1.50+

# Test with race detector
go test -race ./...

# Coverage target
go test -cover ./...  # Must maintain >80%
```

---

## P1: Critical Fixes (4 Issues)

### Issue #1: Fix TryLock Compilation Error

**File**: `internal/indexer/indexer.go:85`

**Problem**: `sync.Mutex` has no `TryLock` method.

**Solution**: Replace with atomic-based lock.

```go
// Add new type
type IndexLock struct {
    state atomic.Int32  // 0=unlocked, 1=locked
}

func (l *IndexLock) TryAcquire() bool {
    return l.state.CompareAndSwap(0, 1)
}

func (l *IndexLock) Release() {
    l.state.Store(0)
}

// In Indexer struct, replace:
// indexMutex sync.Mutex
indexLock IndexLock

// Usage in IndexProject:
if !idx.indexLock.TryAcquire() {
    return nil, fmt.Errorf("indexing already in progress for %s", config.RootPath)
}
defer idx.indexLock.Release()
```

**Test**:
```go
func TestIndexLock_ConcurrentAcquisition(t *testing.T) {
    lock := &IndexLock{}

    acquired1 := lock.TryAcquire()
    assert.True(t, acquired1, "First acquisition should succeed")
    defer lock.Release()

    acquired2 := lock.TryAcquire()
    assert.False(t, acquired2, "Second acquisition should fail while locked")
}
```

---

### Issue #2: Fix Database Connection Leak

**File**: `internal/storage/sqlite.go:39`

**Problem**: DB not closed if PRAGMA statements fail.

**Solution**: Use defer immediately after opening.

```go
func NewSQLiteStorage(dbPath string) (Storage, error) {
    db, err := sql.Open("sqlite3", dbPath)
    if err != nil {
        return nil, fmt.Errorf("failed to open database: %w", err)
    }
    defer func() {
        if err != nil {
            db.Close()  // Close on any error
        }
    }()

    // Apply PRAGMAs (failures now properly clean up)
    if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
        return nil, fmt.Errorf("failed to enable foreign keys: %w", err)
    }
    // ... more PRAGMAs ...

    storage := &SQLiteStorage{db: db, /* ... */}
    return storage, nil
}
```

**Test**:
```go
func TestSQLiteStorage_ConnectionLeakOnPragmaFailure(t *testing.T) {
    // Test with invalid database path or corrupted DB
    _, err := NewSQLiteStorage("/invalid/path/db.sqlite")
    assert.Error(t, err)
    // Verify no leaked file descriptors (check with lsof)
}
```

---

### Issue #3: Fix FTS5 Injection Vulnerability

**File**: `internal/storage/vector_ops.go:45`

**Problem**: `sanitizeFTSQuery` is a no-op, allowing SQL injection.

**Solution**: Implement proper escaping.

```go
// Precompile regex patterns (package level)
var (
    ftsOperatorPattern = regexp.MustCompile(`\b(AND|OR|NOT|NEAR)\b`)
    ftsSpecialChars    = strings.NewReplacer(
        `"`, `\"`,
        `*`, `\*`,
        `(`, `\(`,
        `)`, `\)`,
    )
)

func sanitizeFTSQuery(query string) string {
    // Escape special characters
    escaped := ftsSpecialChars.Replace(query)

    // Escape Boolean operators
    escaped = ftsOperatorPattern.ReplaceAllStringFunc(escaped, func(match string) string {
        return `\` + match
    })

    return escaped
}
```

**Test**:
```go
func TestSanitizeFTSQuery_PreventInjection(t *testing.T) {
    tests := []struct {
        name     string
        input    string
        expected string
    }{
        {"Boolean injection", "test OR DROP TABLE", `test \OR DROP TABLE`},
        {"NEAR injection", "search NEAR(10)", `search \NEAR\(10\)`},
        {"Wildcard", "user*", `user\*`},
        {"Quoted phrase", `"exact match"`, `\"exact match\"`},
        {"Normal query", "getUserData", "getUserData"},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result := sanitizeFTSQuery(tt.input)
            assert.Equal(t, tt.expected, result)
        })
    }
}

func TestSearchText_NoInjection(t *testing.T) {
    store := setupTestStorage(t)
    // Attempt injection
    malicious := `test" OR 1=1 --`
    results, err := store.SearchText(ctx, projectID, malicious, 10, nil)
    assert.NoError(t, err)
    // Should return legitimate results only, not all rows
}
```

---

### Issue #4: Fix Insecure Password Hashing

**File**: `tests/testdata/fixtures/authentication.go:118`

**Problem**: Uses SHA-256 without salt.

**Solution**: Use bcrypt with salt.

```go
import "golang.org/x/crypto/bcrypt"

func hashPassword(password string) (string, error) {
    // Use bcrypt with cost factor 10 (default)
    hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
    if err != nil {
        return "", fmt.Errorf("failed to hash password: %w", err)
    }
    return string(hashed), nil
}

func verifyPassword(hashedPassword, password string) bool {
    err := bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(password))
    return err == nil
}
```

**Test**:
```go
func TestHashPassword_UsesBcrypt(t *testing.T) {
    password := "secure_password_123"

    hashed, err := hashPassword(password)
    assert.NoError(t, err)
    assert.NotEqual(t, password, hashed)

    // Verify bcrypt format ($2a$10$...)
    assert.True(t, strings.HasPrefix(hashed, "$2a$"))

    // Verify password
    assert.True(t, verifyPassword(hashed, password))
    assert.False(t, verifyPassword(hashed, "wrong_password"))
}

func TestHashPassword_UniqueSalts(t *testing.T) {
    password := "same_password"

    hash1, _ := hashPassword(password)
    hash2, _ := hashPassword(password)

    // Different salts → different hashes
    assert.NotEqual(t, hash1, hash2)
}
```

---

## P2: Performance & Data Integrity (18 Issues)

### Issue #5: Validate Embedder Before Use

**File**: `internal/searcher/searcher.go:58`

**Problem**: Nil pointer dereference if embedder not configured.

**Solution**: Add nil check with descriptive error.

```go
func (s *Searcher) runVectorSearch(ctx context.Context, req SearchRequest, resultChan chan<- searchResult) {
    var res searchResult

    if s.embedder == nil {
        res.err = fmt.Errorf("vector search requires embedder to be configured")
        select {
        case resultChan <- res:
        case <-ctx.Done():
        }
        return
    }

    embReq := embedder.EmbeddingRequest{Text: req.Query}
    embedding, err := s.embedder.GenerateEmbedding(ctx, embReq)
    // ... rest of implementation
}
```

---

### Issue #7: Replace Bubble Sort with sort.Slice

**File**: `internal/storage/vector_ops.go:204`

**Problem**: O(n²) bubble sort inefficient for large datasets.

**Solution**: Use stdlib sort.Slice (O(n log n)).

```go
func sortCandidates(candidates []candidate) {
    sort.Slice(candidates, func(i, j int) bool {
        return candidates[i].score > candidates[j].score  // Descending order
    })
}
```

**Benchmark**:
```go
func BenchmarkSortCandidates(b *testing.B) {
    sizes := []int{100, 1000, 10000}

    for _, size := range sizes {
        b.Run(fmt.Sprintf("N=%d", size), func(b *testing.B) {
            candidates := make([]candidate, size)
            for i := range candidates {
                candidates[i] = candidate{
                    chunkID: int64(i),
                    score:   rand.Float64(),
                }
            }

            b.ResetTimer()
            for i := 0; i < b.N; i++ {
                sortCandidates(candidates)
            }
        })
    }
}
```

---

### Issue #9, #12: Semantic Version Comparison

**Files**: `internal/storage/migrations.go:131,390`

**Problem**: String comparison fails for semver (1.10.0 < 1.2.0 lexicographically).

**Solution**: Use semver library.

```go
import "github.com/Masterminds/semver/v3"

type Migration struct {
    Version     *semver.Version
    SQL         string
    Description string
}

func ApplyMigrations(ctx context.Context) error {
    // Parse current version
    var currentVersionStr string
    err := s.db.QueryRowContext(ctx, "SELECT version FROM schema_version ORDER BY applied_at DESC LIMIT 1").Scan(&currentVersionStr)

    var currentVersion *semver.Version
    if err == sql.ErrNoRows {
        currentVersion = semver.MustParse("0.0.0")
    } else if err != nil {
        return fmt.Errorf("failed to read schema_version: %w", err)
    } else {
        currentVersion, err = semver.NewVersion(currentVersionStr)
        if err != nil {
            return fmt.Errorf("invalid current version %q: %w", currentVersionStr, err)
        }
    }

    // Apply migrations in order
    for _, migration := range migrations {
        if migration.Version.LessThanOrEqual(currentVersion) {
            continue  // Already applied
        }

        // Apply migration in transaction
        if err := s.applyMigration(ctx, migration); err != nil {
            return err
        }
    }

    return nil
}
```

**Test**:
```go
func TestMigrationVersionComparison(t *testing.T) {
    v1_2_0 := semver.MustParse("1.2.0")
    v1_10_0 := semver.MustParse("1.10.0")

    assert.True(t, v1_10_0.GreaterThan(v1_2_0), "1.10.0 should be > 1.2.0")
}
```

---

### Issue #14: Transaction Isolation with Querier Interface

**Files**: `internal/storage/sqlite.go:150,498,643`

**Problem**: Reads within transactions use s.db instead of transaction context.

**Solution**: Refactor methods to accept querier interface.

```go
// Internal method accepts querier
func (s *SQLiteStorage) getProjectWithQuerier(ctx context.Context, q querier, rootPath string) (*types.Project, error) {
    var project types.Project
    row := q.QueryRowContext(ctx, "SELECT id, root_path, ... FROM projects WHERE root_path = ?", rootPath)
    if err := row.Scan(&project.ID, &project.RootPath, ...); err != nil {
        return nil, err
    }
    return &project, nil
}

// Public method uses s.db
func (s *SQLiteStorage) GetProject(ctx context.Context, rootPath string) (*types.Project, error) {
    return s.getProjectWithQuerier(ctx, s.db, rootPath)
}

// Transaction method uses t.tx
func (t *sqliteTx) GetProject(ctx context.Context, rootPath string) (*types.Project, error) {
    return t.storage.getProjectWithQuerier(ctx, t.tx, rootPath)
}
```

**Test**:
```go
func TestTransaction_ReadIsolation(t *testing.T) {
    store := setupTestStorage(t)

    tx, _ := store.BeginTx(ctx)

    // Write within transaction (not committed)
    project := &types.Project{RootPath: "/test", Name: "test"}
    tx.CreateProject(ctx, project)

    // Read within same transaction should see uncommitted write
    retrieved, err := tx.GetProject(ctx, "/test")
    assert.NoError(t, err)
    assert.Equal(t, "test", retrieved.Name)

    // Read outside transaction should NOT see uncommitted write
    _, err = store.GetProject(ctx, "/test")
    assert.Error(t, err)  // Not found

    tx.Commit()

    // After commit, both should see it
    retrieved, err = store.GetProject(ctx, "/test")
    assert.NoError(t, err)
}
```

---

### Issue #20, #24: LRU Cache Implementation

**Files**: `internal/embedder/embedder.go:77,65`

**Problem**: Entire cache cleared when capacity reached.

**Solution**: Use hashicorp/golang-lru.

```go
import lru "github.com/hashicorp/golang-lru/v2"

type Cache struct {
    cache *lru.Cache[string, *Embedding]
    mu    sync.RWMutex
}

func NewCache(maxEntries int) *Cache {
    cache, _ := lru.New[string, *Embedding](maxEntries)
    return &Cache{cache: cache}
}

func (c *Cache) Get(hash [32]byte) *Embedding {
    key := hex.EncodeToString(hash[:])

    if emb, ok := c.cache.Get(key); ok {
        // Return deep copy to prevent mutations
        return &Embedding{
            Vector:    append([]float32{}, emb.Vector...),
            Dimension: emb.Dimension,
            Provider:  emb.Provider,
            Model:     emb.Model,
            Hash:      emb.Hash,
        }
    }
    return nil
}

func (c *Cache) Set(hash [32]byte, emb *Embedding) {
    key := hex.EncodeToString(hash[:])
    c.cache.Add(key, emb)  // Automatically evicts LRU if at capacity
}
```

**Test**:
```go
func TestCache_LRUEviction(t *testing.T) {
    cache := NewCache(3)  // Capacity 3

    // Fill cache
    hash1 := sha256.Sum256([]byte("text1"))
    hash2 := sha256.Sum256([]byte("text2"))
    hash3 := sha256.Sum256([]byte("text3"))

    cache.Set(hash1, &Embedding{Vector: []float32{1.0}})
    cache.Set(hash2, &Embedding{Vector: []float32{2.0}})
    cache.Set(hash3, &Embedding{Vector: []float32{3.0}})

    // Access hash1 (makes it most recently used)
    cache.Get(hash1)

    // Add 4th item (should evict hash2, the LRU)
    hash4 := sha256.Sum256([]byte("text4"))
    cache.Set(hash4, &Embedding{Vector: []float32{4.0}})

    // Verify hash2 evicted, others remain
    assert.Nil(t, cache.Get(hash2))
    assert.NotNil(t, cache.Get(hash1))
    assert.NotNil(t, cache.Get(hash3))
    assert.NotNil(t, cache.Get(hash4))
}
```

---

## P3: Maintainability (46 Issues)

### Issue #27: Extract Common Retry Logic

**File**: `internal/embedder/providers.go:104`

**Problem**: Retry logic duplicated across Jina and OpenAI providers.

**Solution**: Extract reusable function.

```go
type RetryConfig struct {
    MaxRetries int
    BaseDelay  time.Duration
    MaxDelay   time.Duration
    Multiplier float64
}

func retryWithBackoff(ctx context.Context, fn func() error, config RetryConfig) error {
    var lastErr error
    delay := config.BaseDelay

    for attempt := 0; attempt <= config.MaxRetries; attempt++ {
        if attempt > 0 {
            // Wait with exponential backoff
            select {
            case <-time.After(delay):
            case <-ctx.Done():
                return ctx.Err()
            }

            delay = time.Duration(float64(delay) * config.Multiplier)
            if delay > config.MaxDelay {
                delay = config.MaxDelay
            }
        }

        if err := fn(); err != nil {
            lastErr = err
            continue
        }

        return nil  // Success
    }

    return fmt.Errorf("failed after %d attempts: %w", config.MaxRetries+1, lastErr)
}

// Usage in providers
func (j *JinaProvider) GenerateBatch(ctx context.Context, req BatchEmbeddingRequest) (*BatchEmbeddingResponse, error) {
    var response *BatchEmbeddingResponse

    err := retryWithBackoff(ctx, func() error {
        resp, err := j.callAPI(req)
        if err != nil {
            return err
        }
        response = resp
        return nil
    }, RetryConfig{
        MaxRetries: 3,
        BaseDelay:  100 * time.Millisecond,
        MaxDelay:   10 * time.Second,
        Multiplier: 2.0,
    })

    return response, err
}
```

---

### Issue #28: Share Embedder Between Components

**File**: `internal/mcp/server.go:40`

**Problem**: Indexer and searcher create separate embedder instances with separate caches.

**Solution**: Share single instance.

```go
func NewServer() (*Server, error) {
    store, err := storage.NewSQLiteStorage(dbPath)
    if err != nil {
        return nil, err
    }

    // Create single embedder instance
    emb, err := embedder.NewFromEnv()
    if err != nil {
        return nil, err
    }

    // Share embedder across components
    idx := indexer.NewWithEmbedder(store, emb)
    searcher := searcher.NewSearcher(store, emb)

    return &Server{
        storage:  store,
        indexer:  idx,
        searcher: searcher,
        embedder: emb,  // Shared instance
    }, nil
}
```

---

## Running Tests

### Unit Tests

```bash
# All tests
go test ./...

# With race detection
go test -race ./...

# Specific package
go test -v ./internal/storage/...

# Specific test
go test -v -run TestSanitizeFTSQuery ./internal/storage/...
```

### Integration Tests

```bash
# Full integration suite
go test -v ./tests/integration/...

# With timeout
go test -v -timeout 5m ./tests/integration/...
```

### Benchmarks

```bash
# Run all benchmarks
go test -bench=. ./...

# Benchmark specific component
go test -bench=BenchmarkSort ./internal/storage/...

# With memory profiling
go test -bench=. -benchmem ./internal/searcher/...
```

### Coverage

```bash
# Generate coverage report
go test -cover ./... -coverprofile=coverage.out

# View HTML report
go tool cover -html=coverage.out

# Check coverage percentage
go tool cover -func=coverage.out | grep total
```

---

## Verification Checklist

After implementing all fixes, verify:

- [ ] All tests pass: `go test ./...`
- [ ] No race conditions: `go test -race ./...`
- [ ] Linter passes: `golangci-lint run`
- [ ] Coverage >80%: `go test -cover ./...`
- [ ] Benchmarks show improvements (sorting, caching)
- [ ] All 94 issues have regression tests
- [ ] Documentation updated for API changes
- [ ] CLAUDE.md reflects any new patterns

---

## Priority Order Summary

1. **P1 (Critical)**: Issues #1-4 - Security and compilation blockers
2. **P2 (High)**: Issues #5-22 - Performance and data integrity
3. **P3 (Medium)**: Issues #23-68 - Maintainability and patterns
4. **P4 (Low)**: Issues #69-94 - Style and consistency

Each priority level can be implemented independently and deployed incrementally.

---

## Next Steps

After completing Phase 1 (this quickstart):

1. Run `/speckit.tasks` to generate detailed task breakdown
2. Implement fixes in priority order
3. Run tests after each fix
4. Commit when linter passes and tests green
5. Final integration test with full suite

See `tasks.md` (generated by `/speckit.tasks`) for detailed implementation tasks.
