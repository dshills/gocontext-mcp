package indexer

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/dshills/gocontext-mcp/internal/chunker"
	"github.com/dshills/gocontext-mcp/internal/embedder"
	"github.com/dshills/gocontext-mcp/internal/parser"
	"github.com/dshills/gocontext-mcp/internal/storage"
)

// ErrIndexingInProgress indicates that an indexing operation is already running
var ErrIndexingInProgress = errors.New("indexing already in progress for this project")

// Indexer coordinates the indexing pipeline: parse -> chunk -> embed -> store
type Indexer struct {
	parser   *parser.Parser
	chunker  *chunker.Chunker
	embedder embedder.Embedder
	storage  storage.Storage

	// Worker pool configuration
	workers int

	// Concurrency control for indexing operations
	indexLock IndexLock
}

// Config contains configuration for the indexer
type Config struct {
	Workers            int  // Number of concurrent workers (default: runtime.NumCPU())
	BatchSize          int  // Number of files to commit per transaction (default: 20)
	EmbeddingBatch     int  // Number of chunks per embedding batch (default: 30)
	IncludeTests       bool // Whether to index test files (default: true)
	IncludeVendor      bool // Whether to index vendor directory (default: false)
	GenerateEmbeddings bool // Whether to generate embeddings (default: true)
	ForceReindex       bool // Whether to force reindex all files ignoring hashes (default: false)
}

// Progress tracks indexing progress
type Progress struct {
	TotalFiles   int32
	IndexedFiles int32
	SkippedFiles int32
	FailedFiles  int32
	TotalSymbols int32
	TotalChunks  int32
	StartTime    time.Time
	EndTime      time.Time
}

// Statistics contains statistics about the indexing operation
type Statistics struct {
	FilesIndexed        int
	FilesSkipped        int
	FilesFailed         int
	SymbolsExtracted    int
	ChunksCreated       int
	EmbeddingsGenerated int
	EmbeddingsFailed    int
	Duration            time.Duration
	ErrorMessages       []string
}

// New creates a new Indexer instance
func New(storage storage.Storage) *Indexer {
	return &Indexer{
		parser:   parser.New(),
		chunker:  chunker.New(),
		embedder: nil, // Lazy initialization on first use
		storage:  storage,
		workers:  runtime.NumCPU(),
	}
}

// NewWithEmbedder creates a new Indexer with a pre-configured embedder
func NewWithEmbedder(storage storage.Storage, emb embedder.Embedder) *Indexer {
	return &Indexer{
		parser:   parser.New(),
		chunker:  chunker.New(),
		embedder: emb,
		storage:  storage,
		workers:  runtime.NumCPU(),
	}
}

// IndexProject indexes an entire Go project
func (idx *Indexer) IndexProject(ctx context.Context, rootPath string, config *Config) (*Statistics, error) {
	// Attempt to acquire lock for exclusive indexing access
	if !idx.indexLock.TryAcquire() {
		return nil, ErrIndexingInProgress
	}
	defer idx.indexLock.Release()

	if config == nil {
		config = &Config{
			Workers:            runtime.NumCPU(),
			BatchSize:          20,
			EmbeddingBatch:     30,
			IncludeTests:       true,
			IncludeVendor:      false,
			GenerateEmbeddings: true,
		}
	}

	// Initialize embedder if needed and embeddings are requested
	if config.GenerateEmbeddings && idx.embedder == nil {
		emb, err := embedder.NewFromEnv()
		if err != nil {
			// Log warning but continue without embeddings
			log.Printf("Warning: Failed to initialize embedder: %v. Continuing without embeddings.", err)
			config.GenerateEmbeddings = false
		} else {
			idx.embedder = emb
		}
	}

	if config.Workers <= 0 {
		config.Workers = runtime.NumCPU()
	}
	idx.workers = config.Workers

	startTime := time.Now()
	stats := &Statistics{
		ErrorMessages: make([]string, 0),
	}

	// Get or create project
	project, err := idx.getOrCreateProject(ctx, rootPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get or create project: %w", err)
	}

	// Discover Go files
	files, err := idx.discoverFiles(rootPath, config)
	if err != nil {
		return nil, fmt.Errorf("failed to discover files: %w", err)
	}

	// Index files concurrently
	err = idx.indexFiles(ctx, project, files, config, stats)
	if err != nil {
		return nil, fmt.Errorf("failed to index files: %w", err)
	}

	// Update project statistics
	if err := idx.updateProjectStats(ctx, project); err != nil {
		return nil, fmt.Errorf("failed to update project stats: %w", err)
	}

	stats.Duration = time.Since(startTime)
	return stats, nil
}

// getOrCreateProject retrieves an existing project or creates a new one
func (idx *Indexer) getOrCreateProject(ctx context.Context, rootPath string) (*storage.Project, error) {
	// Try to get existing project
	project, err := idx.storage.GetProject(ctx, rootPath)
	if err == nil {
		return project, nil
	}

	if err != storage.ErrNotFound {
		return nil, err
	}

	// Create new project
	project = &storage.Project{
		RootPath:     rootPath,
		IndexVersion: storage.CurrentSchemaVersion,
	}

	// Try to extract module info from go.mod
	goModPath := filepath.Join(rootPath, "go.mod")
	if modInfo, err := parseGoMod(goModPath); err == nil {
		project.ModuleName = modInfo.Module
		project.GoVersion = modInfo.GoVersion
	}

	if err := idx.storage.CreateProject(ctx, project); err != nil {
		return nil, err
	}

	return project, nil
}

// discoverFiles finds all Go files in the project
func (idx *Indexer) discoverFiles(rootPath string, config *Config) ([]string, error) {
	var files []string

	err := filepath.Walk(rootPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if info.IsDir() {
			// Skip vendor unless explicitly included
			if !config.IncludeVendor && info.Name() == "vendor" {
				return filepath.SkipDir
			}
			// Skip hidden directories
			if strings.HasPrefix(info.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}

		// Check if it's a Go file
		if !strings.HasSuffix(path, ".go") {
			return nil
		}

		// Skip test files unless explicitly included
		if !config.IncludeTests && strings.HasSuffix(path, "_test.go") {
			return nil
		}

		files = append(files, path)
		return nil
	})

	return files, err
}

// indexFiles indexes a batch of files concurrently
func (idx *Indexer) indexFiles(ctx context.Context, project *storage.Project, files []string, config *Config, stats *Statistics) error {
	// Create worker pool with semaphore
	semaphore := make(chan struct{}, idx.workers)

	// Track progress with atomic counters
	var (
		indexed        int32
		skipped        int32
		failed         int32
		symbols        int32
		chunks         int32
		embeddings     int32
		embeddingsFail int32
	)

	// Process files in batches for transaction efficiency
	batchSize := config.BatchSize
	if batchSize <= 0 {
		batchSize = 20
	}

	// Use errgroup for concurrent processing with error propagation
	g, gctx := errgroup.WithContext(ctx)
	var mu sync.Mutex // Protect stats.ErrorMessages

	for i := 0; i < len(files); i += batchSize {
		end := i + batchSize
		if end > len(files) {
			end = len(files)
		}
		batch := files[i:end]

		g.Go(func() error {
			return idx.indexBatch(gctx, project, batch, config, semaphore, &indexed, &skipped, &failed, &symbols, &chunks, &embeddings, &embeddingsFail, &mu, stats)
		})
	}

	// Wait for all goroutines to complete
	if err := g.Wait(); err != nil {
		return err
	}

	// Update statistics
	stats.FilesIndexed = int(indexed)
	stats.FilesSkipped = int(skipped)
	stats.FilesFailed = int(failed)
	stats.SymbolsExtracted = int(symbols)
	stats.ChunksCreated = int(chunks)
	stats.EmbeddingsGenerated = int(embeddings)
	stats.EmbeddingsFailed = int(embeddingsFail)

	return nil
}

// chunkWithID pairs a chunk with its content for embedding
type chunkWithID struct {
	chunk   *storage.Chunk
	content string
}

// indexBatch indexes a batch of files within a transaction
func (idx *Indexer) indexBatch(ctx context.Context, project *storage.Project, files []string, config *Config,
	semaphore chan struct{}, indexed, skipped, failed, symbols, chunks, embeddings, embeddingsFail *int32,
	mu *sync.Mutex, stats *Statistics) error {

	// Start a transaction for this batch
	tx, err := idx.storage.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Collect chunks from all files in this batch for batch embedding
	var allChunks []chunkWithID

	// Process each file in the batch
	for _, filePath := range files {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case semaphore <- struct{}{}:
			// Acquire semaphore
		}

		fileChunks, err := idx.indexFile(ctx, tx, project, filePath, config, indexed, skipped, failed, symbols, chunks)
		<-semaphore // Release semaphore

		if err != nil {
			atomic.AddInt32(failed, 1)
			mu.Lock()
			stats.ErrorMessages = append(stats.ErrorMessages, fmt.Sprintf("%s: %v", filePath, err))
			mu.Unlock()
			// Continue with other files
			continue
		}

		// Collect chunks for embedding
		if config.GenerateEmbeddings && len(fileChunks) > 0 {
			for _, chunk := range fileChunks {
				allChunks = append(allChunks, chunkWithID{
					chunk:   chunk,
					content: chunk.Content,
				})
			}
		}
	}

	// Commit the batch (must happen before embeddings to have chunk IDs)
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Generate embeddings for all chunks in this batch
	if config.GenerateEmbeddings && len(allChunks) > 0 && idx.embedder != nil {
		// Track embedding results for cleanup
		// embeddingResults maps chunkID -> success status for each chunk that was processed
		embeddingResults := idx.generateEmbeddingsForChunks(ctx, allChunks, config.EmbeddingBatch, embeddings, embeddingsFail, mu, stats)

		// Clean up orphaned chunks (chunks without embeddings)
		// This maintains consistency: with embeddings enabled, all stored chunks should have embeddings.
		// Only chunks where embedding generation/storage failed are deleted (embeddingResults[id]=false or missing).
		// Chunks with successful embeddings (embeddingResults[id]=true) are kept regardless of which file they came from.
		// Design decision: We intentionally don't fail the batch on cleanup errors to avoid data loss,
		// but log the error for monitoring. Orphaned chunks can be cleaned up later via maintenance jobs.
		if err := idx.cleanupOrphanedChunks(ctx, allChunks, embeddingResults, mu, stats); err != nil {
			// Log cleanup error but don't fail the batch
			mu.Lock()
			stats.ErrorMessages = append(stats.ErrorMessages, fmt.Sprintf("cleanup orphaned chunks: %v", err))
			mu.Unlock()
		}
	}

	return nil
}

// indexFile indexes a single file and returns the stored chunks
func (idx *Indexer) indexFile(ctx context.Context, store storage.Storage, project *storage.Project,
	filePath string, config *Config, indexed, skipped, failed, symbols, chunks *int32) ([]*storage.Chunk, error) {

	// Compute relative path
	relPath, err := filepath.Rel(project.RootPath, filePath)
	if err != nil {
		return nil, err
	}

	// Compute file hash
	hash, modTime, sizeBytes, err := computeFileHash(filePath)
	if err != nil {
		return nil, err
	}

	// Check if file has changed and handle incremental update (unless force reindex)
	if !config.ForceReindex {
		shouldSkip, err := idx.checkFileChanged(ctx, store, project.ID, relPath, hash, skipped)
		if err != nil {
			return nil, err
		}
		if shouldSkip {
			return nil, nil
		}
	} else {
		// Force reindex: delete existing file and all related data (cascades to chunks, symbols, imports)
		existingFile, err := store.GetFile(ctx, project.ID, relPath)
		if err == nil {
			// File exists - delete it (ON DELETE CASCADE will handle related data)
			if err := store.DeleteFile(ctx, existingFile.ID); err != nil {
				return nil, fmt.Errorf("failed to delete existing file for force reindex: %w", err)
			}
		} else if err != storage.ErrNotFound {
			return nil, fmt.Errorf("failed to check existing file: %w", err)
		}
	}

	// Parse the file
	parseResult, err := idx.parser.ParseFile(filePath)
	if err != nil {
		return nil, err
	}

	// Create or update file record
	file := &storage.File{
		ProjectID:   project.ID,
		FilePath:    relPath,
		PackageName: parseResult.PackageName,
		ContentHash: hash,
		ModTime:     modTime,
		SizeBytes:   sizeBytes,
	}

	// Check for parse errors
	if len(parseResult.Errors) > 0 {
		errMsg := parseResult.Errors[0].Message
		file.ParseError = &errMsg
	}

	if err := store.UpsertFile(ctx, file); err != nil {
		return nil, err
	}

	// Store imports
	for _, imp := range parseResult.Imports {
		impRecord := &storage.Import{
			FileID:     file.ID,
			ImportPath: imp.Path,
			Alias:      imp.Alias,
		}
		if err := store.UpsertImport(ctx, impRecord); err != nil {
			return nil, fmt.Errorf("failed to store import: %w", err)
		}
	}

	// Store symbols
	symbolCount := 0
	for i := range parseResult.Symbols {
		sym := storage.FromTypesSymbol(parseResult.Symbols[i], file.ID)
		if err := store.UpsertSymbol(ctx, sym); err != nil {
			return nil, fmt.Errorf("failed to store symbol: %w", err)
		}
		symbolCount++
	}

	// Create chunks
	fileChunks, err := idx.chunker.ChunkFile(filePath, parseResult, file.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to chunk file: %w", err)
	}

	// Store chunks and collect them for return
	var storedChunks []*storage.Chunk
	chunkCount := 0
	for _, chunk := range fileChunks {
		storageChunk := &storage.Chunk{
			FileID:        file.ID,
			SymbolID:      chunk.SymbolID,
			Content:       chunk.Content,
			ContentHash:   chunk.ContentHash,
			TokenCount:    chunk.TokenCount,
			StartLine:     chunk.StartLine,
			EndLine:       chunk.EndLine,
			ContextBefore: chunk.ContextBefore,
			ContextAfter:  chunk.ContextAfter,
			ChunkType:     string(chunk.ChunkType),
		}
		if err := store.UpsertChunk(ctx, storageChunk); err != nil {
			return nil, fmt.Errorf("failed to store chunk: %w", err)
		}
		storedChunks = append(storedChunks, storageChunk)
		chunkCount++
	}

	// Update counters
	atomic.AddInt32(indexed, 1)
	atomic.AddInt32(symbols, int32(symbolCount))
	atomic.AddInt32(chunks, int32(chunkCount))

	return storedChunks, nil
}

// checkFileChanged checks if a file has changed and needs re-indexing
func (idx *Indexer) checkFileChanged(ctx context.Context, store storage.Storage, projectID int64,
	relPath string, hash [32]byte, skipped *int32) (bool, error) {

	existingFile, err := store.GetFile(ctx, projectID, relPath)
	if err == storage.ErrNotFound {
		// New file, needs indexing
		return false, nil
	}
	if err != nil {
		return false, err
	}

	// File exists - check if it has changed
	if existingFile.ContentHash == hash {
		// File unchanged, skip
		atomic.AddInt32(skipped, 1)
		return true, nil
	}

	// File changed - delete old data before re-indexing
	// Delete chunks (this will cascade to embeddings via FK constraint)
	if err := store.DeleteChunksByFile(ctx, existingFile.ID); err != nil {
		return false, fmt.Errorf("failed to delete old chunks: %w", err)
	}

	// Delete symbols (this will cascade to FTS via triggers)
	if err := store.DeleteSymbolsByFile(ctx, existingFile.ID); err != nil {
		return false, fmt.Errorf("failed to delete old symbols: %w", err)
	}

	// Delete imports
	if err := store.DeleteImportsByFile(ctx, existingFile.ID); err != nil {
		return false, fmt.Errorf("failed to delete old imports: %w", err)
	}

	return false, nil
}

// updateProjectStats updates the project's file and chunk counts
func (idx *Indexer) updateProjectStats(ctx context.Context, project *storage.Project) error {
	// Get file count
	files, err := idx.storage.ListFiles(ctx, project.ID)
	if err != nil {
		return err
	}

	// Count chunks across all files
	totalChunks := 0
	for _, file := range files {
		chunks, err := idx.storage.ListChunksByFile(ctx, file.ID)
		if err != nil {
			return err
		}
		totalChunks += len(chunks)
	}

	project.TotalFiles = len(files)
	project.TotalChunks = totalChunks
	project.LastIndexedAt = time.Now()

	return idx.storage.UpdateProject(ctx, project)
}

// generateEmbeddingsForChunks generates embeddings for a batch of chunks and returns results
func (idx *Indexer) generateEmbeddingsForChunks(ctx context.Context, chunks []chunkWithID, batchSize int, embeddings, embeddingsFail *int32, mu *sync.Mutex, stats *Statistics) map[int64]bool {
	if batchSize <= 0 {
		batchSize = 30
	}

	// Track which chunks successfully got embeddings
	results := make(map[int64]bool)

	// Process chunks in batches
	for i := 0; i < len(chunks); i += batchSize {
		end := i + batchSize
		if end > len(chunks) {
			end = len(chunks)
		}
		batch := chunks[i:end]

		// Prepare batch request
		texts := make([]string, len(batch))
		for j, c := range batch {
			texts[j] = c.content
		}

		// Generate embeddings for this batch
		resp, err := idx.embedder.GenerateBatch(ctx, embedder.BatchEmbeddingRequest{
			Texts: texts,
		})

		if err != nil {
			// Log error and track failure
			mu.Lock()
			stats.ErrorMessages = append(stats.ErrorMessages, fmt.Sprintf("embedding batch %d-%d: %v", i, end, err))
			mu.Unlock()
			atomic.AddInt32(embeddingsFail, int32(len(batch)))

			// Mark all chunks in this batch as failed
			for _, c := range batch {
				if c.chunk.ID != 0 {
					results[c.chunk.ID] = false
				}
			}
			continue
		}

		// Store embeddings
		for j, emb := range resp.Embeddings {
			if j >= len(batch) {
				break
			}

			chunkID := batch[j].chunk.ID
			if chunkID == 0 {
				// Chunk wasn't stored successfully, skip
				// Don't add to results map - we only track successfully stored chunks
				atomic.AddInt32(embeddingsFail, 1)
				continue
			}

			// Serialize vector
			vectorBlob := storage.SerializeVector(emb.Vector)

			storageEmb := &storage.Embedding{
				ChunkID:   chunkID,
				Vector:    vectorBlob,
				Dimension: emb.Dimension,
				Provider:  emb.Provider,
				Model:     emb.Model,
			}

			if err := idx.storage.UpsertEmbedding(ctx, storageEmb); err != nil {
				mu.Lock()
				stats.ErrorMessages = append(stats.ErrorMessages, fmt.Sprintf("store embedding chunk %d: %v", chunkID, err))
				mu.Unlock()
				atomic.AddInt32(embeddingsFail, 1)
				results[chunkID] = false
				continue
			}

			atomic.AddInt32(embeddings, 1)
			results[chunkID] = true
		}
	}

	return results
}

// cleanupOrphanedChunks removes chunks that failed to get embeddings
func (idx *Indexer) cleanupOrphanedChunks(ctx context.Context, chunks []chunkWithID, embeddingResults map[int64]bool, mu *sync.Mutex, stats *Statistics) error {
	// Collect chunk IDs that need to be deleted
	var orphanedChunkIDs []int64
	for _, c := range chunks {
		chunkID := c.chunk.ID
		if chunkID == 0 {
			continue // Skip chunks that weren't stored
		}

		// Check if embedding generation succeeded for this chunk
		success, exists := embeddingResults[chunkID]
		if !exists || !success {
			orphanedChunkIDs = append(orphanedChunkIDs, chunkID)
		}
	}

	// No orphaned chunks to clean up
	if len(orphanedChunkIDs) == 0 {
		return nil
	}

	// Log cleanup operation
	mu.Lock()
	stats.ErrorMessages = append(stats.ErrorMessages, fmt.Sprintf("cleaning up %d orphaned chunks", len(orphanedChunkIDs)))
	mu.Unlock()

	// Delete orphaned chunks in a transaction for atomicity
	tx, err := idx.storage.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin cleanup transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Delete orphaned chunks in batch for performance
	deletedCount, err := tx.DeleteChunksBatch(ctx, orphanedChunkIDs)
	if err != nil {
		return fmt.Errorf("failed to batch delete %d orphaned chunks: %w", len(orphanedChunkIDs), err)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit cleanup transaction: %w", err)
	}

	// Log successful cleanup
	if deletedCount > 0 {
		mu.Lock()
		stats.ErrorMessages = append(stats.ErrorMessages, fmt.Sprintf("successfully deleted %d orphaned chunks", deletedCount))
		mu.Unlock()
	}

	return nil
}

// computeFileHash computes SHA-256 hash of a file
func computeFileHash(filePath string) ([32]byte, time.Time, int64, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return [32]byte{}, time.Time{}, 0, err
	}
	defer func() { _ = file.Close() }()

	// Get file info
	info, err := file.Stat()
	if err != nil {
		return [32]byte{}, time.Time{}, 0, err
	}

	// Compute hash
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return [32]byte{}, time.Time{}, 0, err
	}

	var result [32]byte
	copy(result[:], hash.Sum(nil))

	return result, info.ModTime(), info.Size(), nil
}

// goModInfo contains parsed go.mod information
type goModInfo struct {
	Module    string
	GoVersion string
}

// parseGoMod extracts basic info from go.mod file
func parseGoMod(goModPath string) (*goModInfo, error) {
	content, err := os.ReadFile(goModPath)
	if err != nil {
		return nil, err
	}

	info := &goModInfo{}
	lines := strings.Split(string(content), "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			info.Module = strings.TrimSpace(strings.TrimPrefix(line, "module"))
		} else if strings.HasPrefix(line, "go ") {
			info.GoVersion = strings.TrimSpace(strings.TrimPrefix(line, "go"))
		}
	}

	return info, nil
}
