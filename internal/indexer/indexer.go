package indexer

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/dshills/gocontext-mcp/internal/chunker"
	"github.com/dshills/gocontext-mcp/internal/parser"
	"github.com/dshills/gocontext-mcp/internal/storage"
)

// Indexer coordinates the indexing pipeline: parse -> chunk -> store
type Indexer struct {
	parser  *parser.Parser
	chunker *chunker.Chunker
	storage storage.Storage

	// Worker pool configuration
	workers int
}

// Config contains configuration for the indexer
type Config struct {
	Workers       int  // Number of concurrent workers (default: runtime.NumCPU())
	BatchSize     int  // Number of files to commit per transaction (default: 20)
	IncludeTests  bool // Whether to index test files (default: true)
	IncludeVendor bool // Whether to index vendor directory (default: false)
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
	FilesIndexed     int
	FilesSkipped     int
	FilesFailed      int
	SymbolsExtracted int
	ChunksCreated    int
	Duration         time.Duration
	ErrorMessages    []string
}

// New creates a new Indexer instance
func New(storage storage.Storage) *Indexer {
	return &Indexer{
		parser:  parser.New(),
		chunker: chunker.New(),
		storage: storage,
		workers: runtime.NumCPU(),
	}
}

// IndexProject indexes an entire Go project
func (idx *Indexer) IndexProject(ctx context.Context, rootPath string, config *Config) (*Statistics, error) {
	if config == nil {
		config = &Config{
			Workers:       runtime.NumCPU(),
			BatchSize:     20,
			IncludeTests:  true,
			IncludeVendor: false,
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
		indexed int32
		skipped int32
		failed  int32
		symbols int32
		chunks  int32
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
			return idx.indexBatch(gctx, project, batch, semaphore, &indexed, &skipped, &failed, &symbols, &chunks, &mu, stats)
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

	return nil
}

// indexBatch indexes a batch of files within a transaction
func (idx *Indexer) indexBatch(ctx context.Context, project *storage.Project, files []string,
	semaphore chan struct{}, indexed, skipped, failed, symbols, chunks *int32,
	mu *sync.Mutex, stats *Statistics) error {

	// Start a transaction for this batch
	tx, err := idx.storage.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Process each file in the batch
	for _, filePath := range files {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case semaphore <- struct{}{}:
			// Acquire semaphore
		}

		err := idx.indexFile(ctx, tx, project, filePath, indexed, skipped, failed, symbols, chunks)
		<-semaphore // Release semaphore

		if err != nil {
			atomic.AddInt32(failed, 1)
			mu.Lock()
			stats.ErrorMessages = append(stats.ErrorMessages, fmt.Sprintf("%s: %v", filePath, err))
			mu.Unlock()
			// Continue with other files
			continue
		}
	}

	// Commit the batch
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// indexFile indexes a single file
func (idx *Indexer) indexFile(ctx context.Context, store storage.Storage, project *storage.Project,
	filePath string, indexed, skipped, failed, symbols, chunks *int32) error {

	// Compute relative path
	relPath, err := filepath.Rel(project.RootPath, filePath)
	if err != nil {
		return err
	}

	// Compute file hash
	hash, modTime, sizeBytes, err := computeFileHash(filePath)
	if err != nil {
		return err
	}

	// Check if file has changed and handle incremental update
	shouldSkip, err := idx.checkFileChanged(ctx, store, project.ID, relPath, hash, skipped)
	if err != nil {
		return err
	}
	if shouldSkip {
		return nil
	}

	// Parse the file
	parseResult, err := idx.parser.ParseFile(filePath)
	if err != nil {
		return err
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
		return err
	}

	// Store imports
	for _, imp := range parseResult.Imports {
		impRecord := &storage.Import{
			FileID:     file.ID,
			ImportPath: imp.Path,
			Alias:      imp.Alias,
		}
		if err := store.UpsertImport(ctx, impRecord); err != nil {
			return fmt.Errorf("failed to store import: %w", err)
		}
	}

	// Store symbols
	symbolCount := 0
	for i := range parseResult.Symbols {
		sym := storage.FromTypesSymbol(parseResult.Symbols[i], file.ID)
		if err := store.UpsertSymbol(ctx, sym); err != nil {
			return fmt.Errorf("failed to store symbol: %w", err)
		}
		symbolCount++
	}

	// Create chunks
	fileChunks, err := idx.chunker.ChunkFile(filePath, parseResult, file.ID)
	if err != nil {
		return fmt.Errorf("failed to chunk file: %w", err)
	}

	// Store chunks
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
			return fmt.Errorf("failed to store chunk: %w", err)
		}
		chunkCount++
	}

	// Update counters
	atomic.AddInt32(indexed, 1)
	atomic.AddInt32(symbols, int32(symbolCount))
	atomic.AddInt32(chunks, int32(chunkCount))

	return nil
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

	// File changed - delete old chunks before re-indexing
	if err := store.DeleteChunksByFile(ctx, existingFile.ID); err != nil {
		return false, fmt.Errorf("failed to delete old chunks: %w", err)
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
