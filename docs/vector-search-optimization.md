# Vector Search Optimization Implementation

## Summary

Implemented SQL-based vector similarity search using the sqlite-vec extension, replacing the previous Go-based in-memory approach. This optimization significantly improves both performance and memory efficiency.

## Changes Made

### 1. Added sqlite-vec Go Bindings

**File**: `go.mod`
- Added dependency: `github.com/asg017/sqlite-vec-go-bindings v0.1.6`

### 2. Initialized sqlite-vec Extension

**File**: `internal/storage/build_cgo.go`
- Added automatic loading of sqlite-vec extension via `sqlite_vec.Auto()` in `init()` function
- Extension is now available for all database connections when built with CGO enabled

### 3. Refactored Vector Search Implementation

**File**: `internal/storage/vector_ops.go`

#### Main Changes:

**searchVector()** - Entry point with automatic fallback:
```go
func searchVector(...) ([]VectorResult, error) {
    if VectorExtensionAvailable {
        return searchVectorOptimized(...)
    }
    return searchVectorFallback(...)
}
```

**searchVectorOptimized()** - New SQL-based implementation:
- Uses `vec_distance_cosine()` SQL function for distance computation
- Pushes all filtering to SQL WHERE clause
- Applies ORDER BY and LIMIT at database level
- No in-memory loading of embeddings
- Streams results directly from query

**searchVectorFallback()** - Original Go-based implementation:
- Preserved for purego builds (without CGO)
- Maintains backward compatibility
- Used when sqlite-vec extension is not available

#### SQL Query Structure:

**Before (Fallback)**:
```sql
SELECT c.id, e.vector
FROM chunks c
JOIN embeddings e ON c.id = e.chunk_id
JOIN files f ON c.file_id = f.id
WHERE f.project_id = ?
-- All embeddings loaded into Go memory
-- Cosine similarity computed in Go
-- Sorting done in Go
```

**After (Optimized)**:
```sql
SELECT
    c.id as chunk_id,
    1.0 - vec_distance_cosine(e.vector, ?) as similarity
FROM chunks c
JOIN embeddings e ON c.id = e.chunk_id
JOIN files f ON c.file_id = f.id
WHERE f.project_id = ?
  AND (1.0 - vec_distance_cosine(e.vector, ?)) >= ? -- MinRelevance filter
  -- Additional filters applied here
ORDER BY similarity DESC
LIMIT ?
```

### 4. Comprehensive Testing

**File**: `internal/storage/vector_ops_test.go`

Created three categories of tests:

#### Integration Tests (TestVectorSearchOptimization)
- Compares optimized vs fallback implementations
- Tests 6 filter combinations:
  - Basic search (no filters)
  - Package filters
  - Symbol type filters
  - Minimum relevance threshold
  - File pattern matching
  - Combined filters
- Validates that results are equivalent (within floating-point precision)

#### Edge Case Tests (TestVectorSearchEdgeCases)
- Empty query vectors
- Zero/negative limits
- Non-existent projects
- Verifies graceful error handling

#### Debug Tests (vector_debug_test.go)
- Vector format compatibility verification
- Detailed score comparison logging
- Validates serialization format matches sqlite-vec expectations

### 5. Benchmarking

Created comprehensive benchmarks to measure performance improvements:

```go
BenchmarkVectorSearchOptimized
BenchmarkVectorSearchFallback
BenchmarkVectorSearchComparison
```

## Performance Results

### Benchmark Comparison (30 chunks, 384-dimensional vectors)

```
BenchmarkVectorSearchOptimized-14     42426    30640 ns/op      3120 B/op      43 allocs/op
BenchmarkVectorSearchFallback-14      23064    46976 ns/op    157161 B/op     204 allocs/op
```

### Performance Improvements

| Metric | Optimized | Fallback | Improvement |
|--------|-----------|----------|-------------|
| **Speed** | 30,640 ns/op | 46,976 ns/op | **1.53x faster** |
| **Memory** | 3,120 B/op | 157,161 B/op | **50.4x less memory** |
| **Allocations** | 43 allocs/op | 204 allocs/op | **4.7x fewer allocs** |

### Key Benefits

1. **No In-Memory Loading**: Embeddings stay in database, reducing memory footprint by 98%
2. **SQL-Level Filtering**: All filters applied in database, reducing data transfer
3. **Native SIMD Acceleration**: sqlite-vec uses AVX/NEON instructions for vector operations
4. **Streaming Results**: Results processed as they arrive, not batched in memory
5. **Database-Level Sorting**: SQLite's optimized ORDER BY instead of Go sort

## Accuracy Notes

### Floating-Point Precision

The optimized and fallback implementations produce **functionally equivalent** results, with minor differences due to floating-point precision:

- **Go implementation**: Uses `float64` for intermediate calculations
- **sqlite-vec**: Uses `float32` for storage and computation
- **Score differences**: Typically < 0.000003 (7th decimal place)
- **Impact**: May cause minor reordering when scores are very close, but both implementations return the same highly-relevant chunks

Example comparison:
```
Optimized: ChunkID=22, Score=0.8654629737
Fallback:  ChunkID=25, Score=0.8654606891
Difference: 0.0000022846 (0.0003%)
```

## Build Requirements

### CGO Build (Recommended for Production)
```bash
CGO_ENABLED=1 go build -tags "sqlite_vec sqlite_fts5" -o bin/gocontext ./cmd/gocontext
```

**Features**:
- sqlite-vec extension enabled
- 50x better memory efficiency
- 1.5x faster search
- Native SIMD acceleration

### Pure Go Build (Development/Cross-Platform)
```bash
CGO_ENABLED=0 go build -tags "purego" -o bin/gocontext ./cmd/gocontext
```

**Features**:
- No C compiler required
- Works everywhere
- Uses fallback Go implementation
- Suitable for smaller codebases

## Testing

### Run All Tests
```bash
CGO_ENABLED=1 go test -tags "sqlite_vec sqlite_fts5" ./internal/storage
```

### Run Benchmarks
```bash
CGO_ENABLED=1 go test -tags "sqlite_vec sqlite_fts5" \
  -bench=BenchmarkVectorSearch \
  -benchmem \
  ./internal/storage
```

### Run Specific Test Suites
```bash
# Integration tests
go test -tags "sqlite_vec sqlite_fts5" -run TestVectorSearchOptimization ./internal/storage

# Edge cases
go test -tags "sqlite_vec sqlite_fts5" -run TestVectorSearchEdgeCases ./internal/storage

# Debug/comparison
go test -tags "sqlite_vec sqlite_fts5" -run TestCompareVectorSearchResults ./internal/storage
```

## Migration Impact

### Backward Compatibility

✅ **Fully backward compatible** - no schema changes required
- Uses existing `embeddings` table with BLOB column
- Vector serialization format unchanged (little-endian float32)
- Existing embeddings work without modification

### API Compatibility

✅ **No API changes** - seamless drop-in replacement
- `SearchVector()` signature unchanged
- Filter options unchanged
- Result format unchanged
- Error handling unchanged

### Deployment

✅ **Zero downtime** - gradual rollout possible
1. Deploy new binary with sqlite-vec support
2. Extension auto-loads on startup
3. Existing queries automatically benefit
4. Fallback available if extension fails to load

## Future Optimizations

### Potential Enhancements

1. **Virtual Tables** (Optional)
   - Could use `vec0` virtual tables for even faster KNN queries
   - Requires schema migration
   - Estimated 2-3x additional speedup

2. **Index Structures** (Future)
   - sqlite-vec supports approximate nearest neighbor indexes
   - Would enable sub-millisecond search for large datasets
   - Trade-off: some accuracy for speed

3. **Batch Operations**
   - Vectorize multiple queries in single SQL statement
   - Useful for reranking scenarios
   - Could reduce overhead by 30-40%

## Monitoring

### Check Extension Status

Binary logs show extension availability at startup:
```
Build Mode: cgo, Driver: sqlite3, Vector Extension: true
```

### Runtime Verification

Query at runtime:
```sql
SELECT vec_version();
```

### Performance Metrics

Key metrics to track:
- Average query latency (target: <100ms for 100K chunks)
- Memory usage per search operation (target: <10KB)
- Allocation count (target: <100 allocs/op)

## References

- [sqlite-vec Documentation](https://alexgarcia.xyz/sqlite-vec/)
- [sqlite-vec GitHub](https://github.com/asg017/sqlite-vec)
- [Go Bindings](https://github.com/asg017/sqlite-vec-go-bindings)

## Tasks Completed

- [x] T043: Refactor searchVector to use sqlite-vec SQL-based filtering
- [x] T044: Remove in-memory loading of all embeddings
- [x] T045: Apply candidate filtering in SQL WHERE clause
- [x] Integration tests comparing old vs new implementation
- [x] Benchmark memory usage before/after optimization
- [x] Documentation and build instructions

## Delivery Date

November 7, 2025
