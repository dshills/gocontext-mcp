// Package storage provides SQLite-based persistence for indexed code data.
//
// The storage layer manages:
//   - Project metadata
//   - File information and content hashes
//   - Extracted symbols
//   - Code chunks
//   - Vector embeddings
//   - Full-text search indexes
//
// # Database Schema
//
// Tables:
//   - projects: Project metadata (root path, module name)
//   - files: File paths and SHA-256 hashes
//   - symbols: Extracted symbols (functions, types, etc.)
//   - chunks: Semantic code chunks
//   - embeddings: Vector embeddings for chunks
//   - chunks_fts: FTS5 full-text search index
//
// # Basic Usage
//
//	db, err := storage.NewSQLite("~/.gocontext/indices/project.db")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer db.Close()
//
//	// Store a file
//	fileID, err := db.StoreFile(ctx, storage.File{
//	    ProjectID:   projectID,
//	    Path:        "internal/parser/parser.go",
//	    PackageName: "parser",
//	    ContentHash: hash,
//	})
//
// # Transactions
//
// Use transactions for atomic operations:
//
//	tx, err := db.Begin(ctx)
//	if err != nil {
//	    return err
//	}
//	defer tx.Rollback()
//
//	// Multiple operations in transaction
//	fileID, _ := tx.StoreFile(ctx, file)
//	symbolIDs, _ := tx.StoreSymbols(ctx, symbols)
//	chunkIDs, _ := tx.StoreChunks(ctx, chunks)
//
//	if err := tx.Commit(); err != nil {
//	    return err
//	}
//
// # Incremental Updates
//
// Check file hashes to detect changes:
//
//	storedHash, found := db.GetFileHash(ctx, filePath)
//	currentHash := sha256.Sum256(content)
//
//	if found && storedHash == currentHash {
//	    // File unchanged, skip re-indexing
//	    return nil
//	}
//
//	// File changed, delete old data
//	db.DeleteFileData(ctx, fileID)
//
//	// Re-index file
//	// ...
//
// # Vector Operations
//
// Store and query vector embeddings:
//
//	// Store embedding
//	err := db.StoreEmbedding(ctx, chunkID, embedding)
//
//	// Vector similarity search
//	results, err := db.VectorSearch(ctx, queryVector, limit)
//	for _, result := range results {
//	    fmt.Printf("Chunk %d: distance %.3f\n",
//	        result.ChunkID, result.Distance)
//	}
//
// Vector search uses cosine similarity via sqlite-vec extension (CGO build)
// or pure Go implementation (purego build).
//
// # Full-Text Search
//
// Query using BM25 ranking:
//
//	results, err := db.KeywordSearch(ctx, "user authentication", limit)
//	for _, result := range results {
//	    fmt.Printf("Chunk %d: score %.3f\n",
//	        result.ChunkID, result.Score)
//	}
//
// FTS5 indexes are automatically updated when chunks are inserted.
//
// # Query Patterns
//
// Common query patterns:
//
//	// Get project by path
//	project, err := db.GetProjectByPath(ctx, "/path/to/project")
//
//	// Get all files in project
//	files, err := db.GetProjectFiles(ctx, projectID)
//
//	// Get symbols for file
//	symbols, err := db.GetFileSymbols(ctx, fileID)
//
//	// Get chunks for file
//	chunks, err := db.GetFileChunks(ctx, fileID)
//
//	// Search with filters
//	results, err := db.SearchWithFilters(ctx, storage.SearchFilters{
//	    Query:       "validation",
//	    SymbolTypes: []string{"function", "method"},
//	    Packages:    []string{"internal/auth"},
//	    Limit:       10,
//	})
//
// # Build Tags
//
// The storage package supports two build configurations:
//
// CGO Build (sqlite_vec tag):
//
//   - Uses github.com/mattn/go-sqlite3 driver
//
//   - Includes sqlite-vec extension for fast vector operations
//
//   - Requires C compiler
//
//     CGO_ENABLED=1 go build -tags "sqlite_vec"
//
// Pure Go Build (purego tag):
//
//   - Uses modernc.org/sqlite driver
//
//   - Pure Go vector operations (slower)
//
//   - No C compiler needed
//
//     CGO_ENABLED=0 go build -tags "purego"
//
// # Performance
//
// Typical operations:
//   - Store file: <1ms
//   - Store symbols (batch): <5ms for 100 symbols
//   - Store chunks (batch): <10ms for 100 chunks
//   - Store embeddings (batch): <20ms for 100 embeddings
//   - Vector search (1k chunks): <100ms
//   - Keyword search (1k chunks): <50ms
//
// Database size: ~10-20% of codebase size.
//
// # Example: Complete Workflow
//
//	// Open database
//	db, err := storage.NewSQLite("project.db")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer db.Close()
//
//	// Create or get project
//	project, err := db.GetOrCreateProject(ctx, storage.Project{
//	    RootPath:   "/Users/dev/myproject",
//	    ModuleName: "github.com/dev/myproject",
//	    GoVersion:  "1.21",
//	})
//
//	// Begin transaction
//	tx, _ := db.Begin(ctx)
//	defer tx.Rollback()
//
//	// Store file
//	file := storage.File{
//	    ProjectID:   project.ID,
//	    Path:        "main.go",
//	    PackageName: "main",
//	    ContentHash: hash,
//	}
//	fileID, _ := tx.StoreFile(ctx, file)
//
//	// Store symbols
//	symbols := []storage.Symbol{...}
//	symbolIDs, _ := tx.StoreSymbols(ctx, fileID, symbols)
//
//	// Store chunks
//	chunks := []storage.Chunk{...}
//	chunkIDs, _ := tx.StoreChunks(ctx, fileID, chunks)
//
//	// Store embeddings
//	for i, chunkID := range chunkIDs {
//	    tx.StoreEmbedding(ctx, chunkID, embeddings[i])
//	}
//
//	// Commit transaction
//	if err := tx.Commit(); err != nil {
//	    log.Fatal(err)
//	}
//
//	// Query
//	results, _ := db.HybridSearch(ctx, storage.SearchRequest{
//	    ProjectID:   project.ID,
//	    QueryVector: queryEmbedding,
//	    QueryText:   "authentication",
//	    Limit:       10,
//	})
package storage
