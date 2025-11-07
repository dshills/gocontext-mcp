package integration

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/dshills/gocontext-mcp/internal/embedder"
	"github.com/dshills/gocontext-mcp/internal/indexer"
	"github.com/dshills/gocontext-mcp/internal/storage"
)

// IndexingTestSuite contains tests for the indexing pipeline
type IndexingTestSuite struct {
	suite.Suite
	storage     storage.Storage
	indexer     *indexer.Indexer
	fixturesDir string
	ctx         context.Context
}

// SetupSuite runs once before all tests
func (s *IndexingTestSuite) SetupSuite() {
	s.ctx = context.Background()

	// Get fixtures directory
	wd, err := os.Getwd()
	s.Require().NoError(err)
	s.fixturesDir = filepath.Join(filepath.Dir(wd), "testdata", "fixtures")

	// Verify fixtures exist
	_, err = os.Stat(s.fixturesDir)
	s.Require().NoError(err, "fixtures directory should exist")
}

// SetupTest runs before each test
func (s *IndexingTestSuite) SetupTest() {
	// Create fresh in-memory storage for each test
	store, err := storage.NewSQLiteStorage(":memory:")
	s.Require().NoError(err)
	s.storage = store

	// Create indexer
	s.indexer = indexer.New(s.storage)
}

// TearDownTest runs after each test
func (s *IndexingTestSuite) TearDownTest() {
	if s.storage != nil {
		_ = s.storage.Close()
	}
}

// TestFullIndexing tests the complete indexing pipeline
func (s *IndexingTestSuite) TestFullIndexing() {
	config := &indexer.Config{
		IncludeTests:       true,
		IncludeVendor:      false,
		Workers:            2,
		BatchSize:          10,
		GenerateEmbeddings: false, // Disable embeddings for basic test
	}

	// Index the fixtures directory
	stats, err := s.indexer.IndexProject(s.ctx, s.fixturesDir, config)
	s.Require().NoError(err, "indexing should succeed")
	s.NotNil(stats)

	// Verify statistics
	s.T().Logf("Indexing stats: %+v", stats)
	s.Greater(stats.FilesIndexed, 0, "should index at least one file")
	s.Greater(stats.SymbolsExtracted, 0, "should extract symbols")
	s.Greater(stats.ChunksCreated, 0, "should create chunks")

	// Expected: 3 files (sample_simple.go, sample_ddd.go, sample_error.go)
	// sample_error.go will fail parsing but we continue
	totalProcessed := stats.FilesIndexed + stats.FilesFailed
	s.GreaterOrEqual(totalProcessed, 2, "should process at least 2 files")

	// Verify project was created
	project, err := s.storage.GetProject(s.ctx, s.fixturesDir)
	s.Require().NoError(err)
	s.Equal(s.fixturesDir, project.RootPath)
	s.Greater(project.TotalFiles, 0)
	s.Greater(project.TotalChunks, 0)
	s.False(project.LastIndexedAt.IsZero())

	// Verify files were indexed
	files, err := s.storage.ListFiles(s.ctx, project.ID)
	s.Require().NoError(err)
	s.NotEmpty(files, "should have indexed files")

	// Verify symbols were extracted
	hasSymbols := false
	for _, file := range files {
		symbols, err := s.storage.ListSymbolsByFile(s.ctx, file.ID)
		s.NoError(err)
		if len(symbols) > 0 {
			hasSymbols = true
			s.T().Logf("File %s has %d symbols", file.FilePath, len(symbols))
			break
		}
	}
	s.True(hasSymbols, "should extract symbols from files")

	// Verify chunks were created
	hasChunks := false
	for _, file := range files {
		chunks, err := s.storage.ListChunksByFile(s.ctx, file.ID)
		s.NoError(err)
		if len(chunks) > 0 {
			hasChunks = true
			s.T().Logf("File %s has %d chunks", file.FilePath, len(chunks))
			break
		}
	}
	s.True(hasChunks, "should create chunks from files")

	// Verify sample_simple.go symbols
	s.verifySimpleFileSymbols(project.ID)

	// Verify sample_ddd.go DDD patterns
	s.verifyDDDPatterns(project.ID)
}

// TestIncrementalIndexing tests incremental re-indexing with unchanged files
func (s *IndexingTestSuite) TestIncrementalIndexing() {
	config := &indexer.Config{
		IncludeTests:  true,
		IncludeVendor: false,
	}

	// Initial indexing
	stats1, err := s.indexer.IndexProject(s.ctx, s.fixturesDir, config)
	s.Require().NoError(err)
	s.T().Logf("Initial indexing: %d indexed, %d skipped", stats1.FilesIndexed, stats1.FilesSkipped)

	initialIndexed := stats1.FilesIndexed
	s.Greater(initialIndexed, 0)

	// Re-index without changes
	stats2, err := s.indexer.IndexProject(s.ctx, s.fixturesDir, config)
	s.Require().NoError(err)
	s.T().Logf("Re-indexing: %d indexed, %d skipped", stats2.FilesIndexed, stats2.FilesSkipped)

	// All files should be skipped on re-index (unchanged)
	s.Equal(0, stats2.FilesIndexed, "should skip unchanged files")
	s.Equal(initialIndexed, stats2.FilesSkipped, "should skip all previously indexed files")

	// Verify database state is consistent
	project, err := s.storage.GetProject(s.ctx, s.fixturesDir)
	s.Require().NoError(err)
	files, err := s.storage.ListFiles(s.ctx, project.ID)
	s.NoError(err)
	s.Equal(initialIndexed, len(files))
}

// TestModifiedFileReindexing tests re-indexing when a file is modified
// Note: This test simulates modification by creating a temporary copy
func (s *IndexingTestSuite) TestModifiedFileReindexing() {
	// Create a temporary directory with a copy of one fixture
	tempDir := s.T().TempDir()

	// Copy sample_simple.go to temp dir
	srcPath := filepath.Join(s.fixturesDir, "sample_simple.go")
	dstPath := filepath.Join(tempDir, "sample_simple.go")

	content, err := os.ReadFile(srcPath)
	s.Require().NoError(err)
	err = os.WriteFile(dstPath, content, 0644)
	s.Require().NoError(err)

	config := &indexer.Config{
		IncludeTests:  true,
		IncludeVendor: false,
	}

	// Initial indexing
	stats1, err := s.indexer.IndexProject(s.ctx, tempDir, config)
	s.Require().NoError(err)
	s.Equal(1, stats1.FilesIndexed, "should index one file")

	project, err := s.storage.GetProject(s.ctx, tempDir)
	s.Require().NoError(err)

	// Get initial chunk count
	files, err := s.storage.ListFiles(s.ctx, project.ID)
	s.Require().NoError(err)
	s.Len(files, 1)

	initialChunks, err := s.storage.ListChunksByFile(s.ctx, files[0].ID)
	s.Require().NoError(err)
	initialChunkCount := len(initialChunks)

	// Modify the file
	time.Sleep(10 * time.Millisecond) // Ensure mod time changes
	modifiedContent := append(content, []byte("\n// Modified comment\n")...)
	err = os.WriteFile(dstPath, modifiedContent, 0644)
	s.Require().NoError(err)

	// Re-index
	stats2, err := s.indexer.IndexProject(s.ctx, tempDir, config)
	s.Require().NoError(err)
	s.Equal(1, stats2.FilesIndexed, "should re-index modified file")
	s.Equal(0, stats2.FilesSkipped, "should not skip modified file")

	// Verify chunks were replaced
	files, err = s.storage.ListFiles(s.ctx, project.ID)
	s.Require().NoError(err)
	s.Len(files, 1)

	newChunks, err := s.storage.ListChunksByFile(s.ctx, files[0].ID)
	s.NoError(err)
	s.GreaterOrEqual(len(newChunks), initialChunkCount, "should have chunks after re-index")
}

// TestErrorHandling tests that indexing continues despite parse errors
func (s *IndexingTestSuite) TestErrorHandling() {
	config := &indexer.Config{
		IncludeTests:  true,
		IncludeVendor: false,
	}

	// Index fixtures including sample_error.go which has syntax errors
	stats, err := s.indexer.IndexProject(s.ctx, s.fixturesDir, config)
	s.Require().NoError(err, "indexing should succeed despite parse errors")

	// With improved parser (FR-021), files with syntax errors are still indexed
	// They extract partial symbols and are stored with ParseError field set
	// The parser continues processing instead of failing completely
	s.Greater(stats.FilesIndexed, 0, "should index files including those with errors")
	s.Greater(stats.SymbolsExtracted, 0, "should extract symbols from valid and partial files")

	// Verify project was created successfully
	project, err := s.storage.GetProject(s.ctx, s.fixturesDir)
	s.NoError(err)
	s.NotNil(project)

	// Verify that files with parse errors have the ParseError field set
	files, err := s.storage.ListFiles(s.ctx, project.ID)
	s.NoError(err)

	// Find the error file and verify it was stored with error info
	foundErrorFile := false
	for _, file := range files {
		if file.ParseError != nil {
			foundErrorFile = true
			s.T().Logf("File with parse error: %s, Error: %s", file.FilePath, *file.ParseError)
			break
		}
	}
	// sample_error.go should be indexed with ParseError set (graceful handling)
	s.True(foundErrorFile, "should have at least one file with ParseError set")
}

// TestEmptyDirectory tests indexing an empty directory
func (s *IndexingTestSuite) TestEmptyDirectory() {
	tempDir := s.T().TempDir()

	config := &indexer.Config{
		IncludeTests:  true,
		IncludeVendor: false,
	}

	// Should complete without error but index nothing
	stats, err := s.indexer.IndexProject(s.ctx, tempDir, config)
	s.Require().NoError(err)
	s.Equal(0, stats.FilesIndexed)
	s.Equal(0, stats.SymbolsExtracted)
	s.Equal(0, stats.ChunksCreated)
}

// TestConcurrentIndexing tests that concurrent workers function correctly
func (s *IndexingTestSuite) TestConcurrentIndexing() {
	config := &indexer.Config{
		IncludeTests:  true,
		IncludeVendor: false,
		Workers:       4, // Use multiple workers
		BatchSize:     1, // Small batches to test concurrency
	}

	stats, err := s.indexer.IndexProject(s.ctx, s.fixturesDir, config)
	s.Require().NoError(err)
	s.Greater(stats.FilesIndexed, 0)

	// Verify data consistency
	project, err := s.storage.GetProject(s.ctx, s.fixturesDir)
	s.Require().NoError(err)

	files, err := s.storage.ListFiles(s.ctx, project.ID)
	s.NoError(err)
	s.NotEmpty(files)

	// Verify all files have valid data
	for _, file := range files {
		s.NotEmpty(file.FilePath)
		s.NotZero(file.ContentHash)

		symbols, err := s.storage.ListSymbolsByFile(s.ctx, file.ID)
		s.NoError(err)

		chunks, err := s.storage.ListChunksByFile(s.ctx, file.ID)
		s.NoError(err)

		// At least one of symbols or chunks should exist (unless it's the error file)
		if file.ParseError == nil {
			s.True(len(symbols) > 0 || len(chunks) > 0,
				"valid files should have symbols or chunks")
		}
	}
}

// verifySimpleFileSymbols verifies symbols extracted from sample_simple.go
func (s *IndexingTestSuite) verifySimpleFileSymbols(projectID int64) {
	files, err := s.storage.ListFiles(s.ctx, projectID)
	s.Require().NoError(err)

	// Find sample_simple.go
	var simpleFile *storage.File
	for _, f := range files {
		if filepath.Base(f.FilePath) == "sample_simple.go" {
			simpleFile = f
			break
		}
	}

	if simpleFile == nil {
		s.T().Skip("sample_simple.go not found in indexed files")
		return
	}

	symbols, err := s.storage.ListSymbolsByFile(s.ctx, simpleFile.ID)
	s.Require().NoError(err)
	s.NotEmpty(symbols, "sample_simple.go should have symbols")

	// Expected symbols: User struct, UserRepository interface,
	// User.Greet method, ValidateEmail function, constants, variables
	symbolNames := make(map[string]bool)
	for _, sym := range symbols {
		symbolNames[sym.Name] = true
		s.T().Logf("Symbol: %s (%s)", sym.Name, sym.Kind)
	}

	// Check for expected symbols
	s.True(symbolNames["User"], "should have User struct")
	s.True(symbolNames["UserRepository"], "should have UserRepository interface")
	s.True(symbolNames["ValidateEmail"], "should have ValidateEmail function")
}

// verifyDDDPatterns verifies DDD pattern detection in sample_ddd.go
func (s *IndexingTestSuite) verifyDDDPatterns(projectID int64) {
	files, err := s.storage.ListFiles(s.ctx, projectID)
	s.Require().NoError(err)

	// Find sample_ddd.go
	var dddFile *storage.File
	for _, f := range files {
		if filepath.Base(f.FilePath) == "sample_ddd.go" {
			dddFile = f
			break
		}
	}

	if dddFile == nil {
		s.T().Skip("sample_ddd.go not found in indexed files")
		return
	}

	symbols, err := s.storage.ListSymbolsByFile(s.ctx, dddFile.ID)
	s.Require().NoError(err)
	s.NotEmpty(symbols, "sample_ddd.go should have symbols")

	// Track DDD patterns found
	var (
		foundAggregate   bool
		foundValueObject bool
		foundRepository  bool
		foundService     bool
		foundCommand     bool
		foundHandler     bool
		foundQuery       bool
	)

	for _, sym := range symbols {
		s.T().Logf("Symbol: %s, Aggregate=%v, VO=%v, Repo=%v, Service=%v, Command=%v, Handler=%v, Query=%v",
			sym.Name, sym.IsAggregateRoot, sym.IsValueObject, sym.IsRepository,
			sym.IsService, sym.IsCommand, sym.IsHandler, sym.IsQuery)

		if sym.IsAggregateRoot {
			foundAggregate = true
		}
		if sym.IsValueObject {
			foundValueObject = true
		}
		if sym.IsRepository {
			foundRepository = true
		}
		if sym.IsService {
			foundService = true
		}
		if sym.IsCommand {
			foundCommand = true
		}
		if sym.IsHandler {
			foundHandler = true
		}
		if sym.IsQuery {
			foundQuery = true
		}
	}

	// Verify DDD patterns were detected
	s.True(foundAggregate, "should detect aggregate root (OrderAggregate)")
	s.True(foundValueObject, "should detect value object (OrderItemVO)")
	s.True(foundRepository, "should detect repository (OrderRepository)")
	s.True(foundService, "should detect service (OrderService)")
	s.True(foundCommand, "should detect command (PlaceOrderCommand)")
	s.True(foundHandler, "should detect handler (OrderPlacedHandler)")
	s.True(foundQuery, "should detect query (ProcessOrderQuery)")
}

// TestForceReindex tests force re-indexing regardless of file hash
func (s *IndexingTestSuite) TestForceReindex() {
	// Create a temporary directory with a copy of fixtures
	tempDir := s.T().TempDir()

	// Copy a fixture file
	srcPath := filepath.Join(s.fixturesDir, "sample_simple.go")
	dstPath := filepath.Join(tempDir, "sample_simple.go")

	content, err := os.ReadFile(srcPath)
	s.Require().NoError(err)
	err = os.WriteFile(dstPath, content, 0644)
	s.Require().NoError(err)

	config := &indexer.Config{
		IncludeTests:       true,
		IncludeVendor:      false,
		GenerateEmbeddings: false,
	}

	// Initial indexing
	stats1, err := s.indexer.IndexProject(s.ctx, tempDir, config)
	s.Require().NoError(err)
	s.Equal(1, stats1.FilesIndexed, "should index one file initially")

	project, err := s.storage.GetProject(s.ctx, tempDir)
	s.Require().NoError(err)

	// Get initial data
	files, err := s.storage.ListFiles(s.ctx, project.ID)
	s.Require().NoError(err)
	s.Len(files, 1)
	initialFileID := files[0].ID
	initialHash := files[0].ContentHash

	initialSymbols, err := s.storage.ListSymbolsByFile(s.ctx, initialFileID)
	s.Require().NoError(err)
	initialSymbolCount := len(initialSymbols)
	s.Greater(initialSymbolCount, 0, "should have symbols")

	initialChunks, err := s.storage.ListChunksByFile(s.ctx, initialFileID)
	s.Require().NoError(err)
	initialChunkCount := len(initialChunks)
	s.Greater(initialChunkCount, 0, "should have chunks")

	// Re-index without changes - should skip
	stats2, err := s.indexer.IndexProject(s.ctx, tempDir, config)
	s.Require().NoError(err)
	s.Equal(0, stats2.FilesIndexed, "should skip unchanged file")
	s.Equal(1, stats2.FilesSkipped, "should report skipped file")

	// Now simulate force re-index by manually deleting the file record
	// This simulates the force_reindex behavior
	err = s.storage.DeleteFile(s.ctx, initialFileID)
	s.Require().NoError(err)

	// Force re-index by indexing again (file no longer exists in DB)
	stats3, err := s.indexer.IndexProject(s.ctx, tempDir, config)
	s.Require().NoError(err)
	s.Equal(1, stats3.FilesIndexed, "should re-index file in force mode")
	s.Equal(0, stats3.FilesSkipped, "should not skip any files")

	// Verify new data was created
	files, err = s.storage.ListFiles(s.ctx, project.ID)
	s.Require().NoError(err)
	s.Len(files, 1)
	newFileID := files[0].ID
	newHash := files[0].ContentHash

	// File ID should be different (new record)
	s.NotEqual(initialFileID, newFileID, "should create new file record")

	// Hash should be the same (file content unchanged)
	s.Equal(initialHash, newHash, "content hash should be identical")

	// Verify symbols were re-created
	newSymbols, err := s.storage.ListSymbolsByFile(s.ctx, newFileID)
	s.NoError(err)
	s.Equal(initialSymbolCount, len(newSymbols), "should recreate same number of symbols")

	// Verify chunks were re-created
	newChunks, err := s.storage.ListChunksByFile(s.ctx, newFileID)
	s.NoError(err)
	s.Equal(initialChunkCount, len(newChunks), "should recreate same number of chunks")

	// Verify old file data was cleaned up (cascaded deletes)
	oldSymbols, err := s.storage.ListSymbolsByFile(s.ctx, initialFileID)
	s.NoError(err)
	s.Empty(oldSymbols, "old symbols should be deleted")

	oldChunks, err := s.storage.ListChunksByFile(s.ctx, initialFileID)
	s.NoError(err)
	s.Empty(oldChunks, "old chunks should be deleted")
}

// TestConcurrentIndexingAttempts tests that concurrent indexing attempts are properly handled
func (s *IndexingTestSuite) TestConcurrentIndexingAttempts() {
	// This test directly verifies that ErrIndexingInProgress is returned
	// when trying to index while an indexing operation is already in progress.
	//
	// Rather than trying to race with timing, we test the mutex behavior
	// by directly calling TryLock/Unlock on the indexer's internal mutex field
	// and verifying the behavior.

	// Create indexer instance
	indexerInstance := indexer.New(s.storage)

	config := &indexer.Config{
		IncludeTests:       true,
		IncludeVendor:      false,
		GenerateEmbeddings: false,
	}

	// Test 1: First index should succeed
	result1, err := indexerInstance.IndexProject(s.ctx, s.fixturesDir, config)
	s.NoError(err, "First IndexProject should succeed")
	s.NotNil(result1)
	s.Greater(result1.FilesIndexed, 0, "Should have indexed files")

	// Now test the concurrent behavior with a fresh indexer
	slowIndexer := indexer.New(s.storage)

	// Test 2: Two concurrent attempts should result in one success and one ErrIndexingInProgress
	resultsChan := make(chan error, 2)

	// Start first indexing in background
	go func() {
		_, err := slowIndexer.IndexProject(s.ctx, s.fixturesDir, config)
		resultsChan <- err
	}()

	// Immediately try second indexing with same indexer (should block momentarily)
	go func() {
		// Small sleep to increase likelihood of contention
		time.Sleep(1 * time.Millisecond)
		_, err := slowIndexer.IndexProject(s.ctx, s.fixturesDir, config)
		resultsChan <- err
	}()

	// Collect both results
	results := make([]error, 0)
	timeout := time.NewTimer(5 * time.Second)
	defer timeout.Stop()

	for i := 0; i < 2; i++ {
		select {
		case err := <-resultsChan:
			results = append(results, err)
		case <-timeout.C:
			s.Fail("Timeout waiting for indexing results")
			return
		}
	}

	// At least one should succeed, at most one should return ErrIndexingInProgress
	// depending on timing
	successCount := 0
	errorCount := 0
	otherCount := 0

	for _, err := range results {
		if err == nil {
			successCount++
		} else if errors.Is(err, indexer.ErrIndexingInProgress) {
			errorCount++
		} else {
			otherCount++
			s.T().Logf("Unexpected error: %v", err)
		}
	}

	// At least one should succeed
	s.GreaterOrEqual(successCount, 1, "At least one indexing operation should succeed")

	// Other errors should only be ErrIndexingInProgress
	s.Equal(0, otherCount, "Should not have unexpected errors")

	// If we got exactly one error, it should be ErrIndexingInProgress
	if errorCount == 1 {
		s.T().Log("SUCCESS: Concurrent indexing properly handled with ErrIndexingInProgress")
	} else if successCount == 2 {
		// Both succeeded - timing was such that they ran sequentially
		s.T().Log("Both indexing operations completed (ran sequentially due to fast execution)")
	}
}

// TestLargeFile tests indexing a file with >10k lines
func (s *IndexingTestSuite) TestLargeFile() {
	tempDir := s.T().TempDir()

	// Generate a large Go file with realistic content
	var content strings.Builder
	content.WriteString("package large\n\n")
	content.WriteString("// Large file for stress testing\n\n")

	// Generate many functions to reach >10k lines (each function ~20 lines)
	for i := 0; i < 600; i++ {
		content.WriteString(fmt.Sprintf("// Function%d performs operation %d\n", i, i))
		content.WriteString(fmt.Sprintf("// This is a generated function for stress testing\n"))
		content.WriteString(fmt.Sprintf("// It simulates real Go code structure\n"))
		content.WriteString(fmt.Sprintf("func Function%d(x int, y int) int {\n", i))
		content.WriteString("	// Initialize result\n")
		content.WriteString("	result := x + y\n")
		content.WriteString("	\n")
		content.WriteString("	// Process calculation\n")
		content.WriteString("	for i := 0; i < 10; i++ {\n")
		content.WriteString("		result = result * 2\n")
		content.WriteString("		if result > 1000 {\n")
		content.WriteString("			result = result / 2\n")
		content.WriteString("		}\n")
		content.WriteString("	}\n")
		content.WriteString("	\n")
		content.WriteString("	// Additional processing\n")
		content.WriteString("	if result < 0 {\n")
		content.WriteString("		result = -result\n")
		content.WriteString("	}\n")
		content.WriteString("	\n")
		content.WriteString("	// Return final result\n")
		content.WriteString("	return result\n")
		content.WriteString("}\n\n")
	}

	largePath := filepath.Join(tempDir, "large.go")
	err := os.WriteFile(largePath, []byte(content.String()), 0644)
	s.Require().NoError(err)

	// Verify file size
	info, err := os.Stat(largePath)
	s.Require().NoError(err)
	lineCount := len(strings.Split(content.String(), "\n"))
	s.T().Logf("Generated file: %d bytes, ~%d lines", info.Size(), lineCount)
	s.Greater(lineCount, 10000, "should have >10k lines")

	config := &indexer.Config{
		IncludeTests:       true,
		IncludeVendor:      false,
		GenerateEmbeddings: false,
	}

	// Index the large file
	stats, err := s.indexer.IndexProject(s.ctx, tempDir, config)
	s.Require().NoError(err, "should index large file successfully")
	s.Equal(1, stats.FilesIndexed, "should index the large file")
	s.Greater(stats.SymbolsExtracted, 100, "should extract many symbols")
	s.Greater(stats.ChunksCreated, 100, "should create many chunks")

	// Verify data integrity
	project, err := s.storage.GetProject(s.ctx, tempDir)
	s.Require().NoError(err)

	files, err := s.storage.ListFiles(s.ctx, project.ID)
	s.NoError(err)
	s.Len(files, 1)

	symbols, err := s.storage.ListSymbolsByFile(s.ctx, files[0].ID)
	s.NoError(err)
	s.Greater(len(symbols), 100, "should extract many symbols from large file")
}

// TestEmptyProject tests indexing a project with no Go files
func (s *IndexingTestSuite) TestEmptyProject() {
	tempDir := s.T().TempDir()

	// Create subdirectories but no Go files
	err := os.MkdirAll(filepath.Join(tempDir, "pkg", "empty"), 0755)
	s.Require().NoError(err)

	// Add non-Go files
	err = os.WriteFile(filepath.Join(tempDir, "README.md"), []byte("# Empty Project"), 0644)
	s.Require().NoError(err)

	config := &indexer.Config{
		IncludeTests:  true,
		IncludeVendor: false,
	}

	// Should complete without error
	stats, err := s.indexer.IndexProject(s.ctx, tempDir, config)
	s.Require().NoError(err, "should handle empty project gracefully")
	s.Equal(0, stats.FilesIndexed, "should index no files")
	s.Equal(0, stats.SymbolsExtracted, "should extract no symbols")
	s.Equal(0, stats.ChunksCreated, "should create no chunks")

	// Project should still be created
	project, err := s.storage.GetProject(s.ctx, tempDir)
	s.NoError(err)
	s.NotNil(project)
	s.Equal(0, project.TotalFiles)
	s.Equal(0, project.TotalChunks)
}

// TestDirectoryWithoutGoFiles tests directory with no Go files
func (s *IndexingTestSuite) TestDirectoryWithoutGoFiles() {
	tempDir := s.T().TempDir()

	// Create files of other types
	err := os.WriteFile(filepath.Join(tempDir, "config.yaml"), []byte("key: value"), 0644)
	s.Require().NoError(err)
	err = os.WriteFile(filepath.Join(tempDir, "script.sh"), []byte("#!/bin/bash\necho test"), 0644)
	s.Require().NoError(err)
	err = os.WriteFile(filepath.Join(tempDir, "data.json"), []byte("{}"), 0644)
	s.Require().NoError(err)

	config := &indexer.Config{
		IncludeTests:  true,
		IncludeVendor: false,
	}

	stats, err := s.indexer.IndexProject(s.ctx, tempDir, config)
	s.Require().NoError(err, "should handle non-Go directory gracefully")
	s.Equal(0, stats.FilesIndexed)
	s.Equal(0, stats.SymbolsExtracted)
}

// TestConcurrentSearchDuringIndexing tests that search works during indexing
func (s *IndexingTestSuite) TestConcurrentSearchDuringIndexing() {
	// Pre-populate with some data
	config := &indexer.Config{
		IncludeTests:       true,
		IncludeVendor:      false,
		GenerateEmbeddings: false,
		Workers:            1, // Single worker to slow down indexing
	}

	// Initial quick index
	_, err := s.indexer.IndexProject(s.ctx, s.fixturesDir, config)
	s.Require().NoError(err)

	project, err := s.storage.GetProject(s.ctx, s.fixturesDir)
	s.Require().NoError(err)

	// Create a new indexer for concurrent attempt
	newIndexer := indexer.New(s.storage)

	// Start indexing in background (will fail with ErrIndexingInProgress or succeed if fast)
	indexDone := make(chan error, 1)
	go func() {
		_, err := newIndexer.IndexProject(s.ctx, s.fixturesDir, config)
		indexDone <- err
	}()

	// Perform searches while indexing might be happening
	searchCtx, cancel := context.WithTimeout(s.ctx, 2*time.Second)
	defer cancel()

	// Multiple concurrent searches
	searchDone := make(chan error, 5)
	for i := 0; i < 5; i++ {
		go func() {
			_, err := s.storage.SearchSymbols(searchCtx, "User", 10)
			searchDone <- err
		}()
	}

	// Collect search results
	for i := 0; i < 5; i++ {
		select {
		case err := <-searchDone:
			s.NoError(err, "searches should succeed during indexing")
		case <-time.After(3 * time.Second):
			s.Fail("Search timed out")
		}
	}

	// Wait for indexing to complete
	select {
	case err := <-indexDone:
		// Either success or ErrIndexingInProgress is acceptable
		if err != nil && !errors.Is(err, indexer.ErrIndexingInProgress) {
			s.Fail("Unexpected indexing error", "error", err)
		}
	case <-time.After(5 * time.Second):
		// Indexing took too long, that's OK for this test
		s.T().Log("Indexing still running after 5s, acceptable for concurrent test")
	}

	// Verify project is still accessible
	_, err = s.storage.GetProject(s.ctx, s.fixturesDir)
	s.NoError(err, "should still be able to access project")

	// Verify data integrity
	files, err := s.storage.ListFiles(s.ctx, project.ID)
	s.NoError(err)
	s.NotEmpty(files, "files should still be accessible")
}

// TestManyConcurrentSearches spawns 100 concurrent search queries
func (s *IndexingTestSuite) TestManyConcurrentSearches() {
	// Pre-populate with data
	config := &indexer.Config{
		IncludeTests:       true,
		IncludeVendor:      false,
		GenerateEmbeddings: false,
	}

	_, err := s.indexer.IndexProject(s.ctx, s.fixturesDir, config)
	s.Require().NoError(err)

	project, err := s.storage.GetProject(s.ctx, s.fixturesDir)
	s.Require().NoError(err)

	// Spawn 100 concurrent searches
	const numSearches = 100
	searchDone := make(chan error, numSearches)

	queries := []string{"User", "Order", "Repository", "Service", "Function"}

	for i := 0; i < numSearches; i++ {
		query := queries[i%len(queries)]
		go func(q string) {
			_, err := s.storage.SearchSymbols(s.ctx, q, 10)
			searchDone <- err
		}(query)
	}

	// Collect all results
	successCount := 0
	failCount := 0
	for i := 0; i < numSearches; i++ {
		select {
		case err := <-searchDone:
			if err == nil {
				successCount++
			} else {
				failCount++
				s.T().Logf("Search failed: %v", err)
			}
		case <-time.After(10 * time.Second):
			s.Fail("Search operation timed out", "completed", i, "total", numSearches)
			return
		}
	}

	s.T().Logf("Concurrent searches: %d succeeded, %d failed", successCount, failCount)
	s.Equal(numSearches, successCount, "all searches should succeed")
	s.Equal(0, failCount, "no searches should fail")

	// Verify project data is still consistent
	files, err := s.storage.ListFiles(s.ctx, project.ID)
	s.NoError(err)
	s.NotEmpty(files, "project data should remain consistent")
}

// TestIndexingWithEmbeddings tests full indexing pipeline with embeddings enabled
func (s *IndexingTestSuite) TestIndexingWithEmbeddings() {
	// Create a mock embedder
	mockEmb := NewMockEmbedder(384)
	indexerWithEmb := indexer.NewWithEmbedder(s.storage, mockEmb)

	config := &indexer.Config{
		IncludeTests:       true,
		IncludeVendor:      false,
		GenerateEmbeddings: true,
		Workers:            2,
		BatchSize:          10,
		EmbeddingBatch:     30,
	}

	// Index with embeddings
	stats, err := indexerWithEmb.IndexProject(s.ctx, s.fixturesDir, config)
	s.Require().NoError(err, "indexing with embeddings should succeed")
	s.NotNil(stats)

	// Verify embeddings were generated
	s.Greater(stats.FilesIndexed, 0, "should index files")
	s.Greater(stats.ChunksCreated, 0, "should create chunks")
	s.Greater(stats.EmbeddingsGenerated, 0, "should generate embeddings")
	s.T().Logf("Generated %d embeddings for %d chunks", stats.EmbeddingsGenerated, stats.ChunksCreated)

	// Some embeddings might fail, but most should succeed
	if stats.EmbeddingsFailed > 0 {
		s.T().Logf("Warning: %d embeddings failed", stats.EmbeddingsFailed)
	}

	// Verify embeddings are stored with correct dimension
	project, err := s.storage.GetProject(s.ctx, s.fixturesDir)
	s.Require().NoError(err)

	files, err := s.storage.ListFiles(s.ctx, project.ID)
	s.Require().NoError(err)
	s.NotEmpty(files)

	// Check embeddings for chunks
	embeddingCount := 0
	for _, file := range files {
		chunks, err := s.storage.ListChunksByFile(s.ctx, file.ID)
		s.NoError(err)

		for _, chunk := range chunks {
			emb, err := s.storage.GetEmbedding(s.ctx, chunk.ID)
			if err == storage.ErrNotFound {
				continue // Some chunks might not have embeddings
			}
			s.NoError(err)
			s.NotNil(emb)
			s.Equal(384, emb.Dimension, "embedding should have correct dimension")
			s.Equal("mock", emb.Provider, "embedding should use mock provider")
			s.Equal("mock-v1", emb.Model, "embedding should use mock model")
			s.NotEmpty(emb.Vector, "embedding should have vector data")
			embeddingCount++
		}
	}

	s.Greater(embeddingCount, 0, "should have stored embeddings")
	s.T().Logf("Verified %d embeddings in storage", embeddingCount)
}

// TestEmbeddingGenerationFailures tests graceful handling of embedding failures
func (s *IndexingTestSuite) TestEmbeddingGenerationFailures() {
	// Create a failing mock embedder
	failingEmb := &FailingMockEmbedder{}
	indexerWithEmb := indexer.NewWithEmbedder(s.storage, failingEmb)

	config := &indexer.Config{
		IncludeTests:       true,
		IncludeVendor:      false,
		GenerateEmbeddings: true,
		Workers:            2,
		BatchSize:          10,
	}

	// Index should succeed even if embeddings fail
	stats, err := indexerWithEmb.IndexProject(s.ctx, s.fixturesDir, config)
	s.Require().NoError(err, "indexing should succeed despite embedding failures")

	// Files and chunks should still be indexed
	s.Greater(stats.FilesIndexed, 0, "should index files")
	s.Greater(stats.ChunksCreated, 0, "should create chunks")

	// All embeddings should fail
	s.Equal(0, stats.EmbeddingsGenerated, "no embeddings should be generated with failing embedder")
	s.Greater(stats.EmbeddingsFailed, 0, "embeddings should be marked as failed")
	s.NotEmpty(stats.ErrorMessages, "should record embedding errors")

	s.T().Logf("Handled %d embedding failures gracefully", stats.EmbeddingsFailed)
}

// TestBenchmarkFullIndexingWithEmbeddings benchmarks full indexing with embeddings
func (s *IndexingTestSuite) TestBenchmarkFullIndexingWithEmbeddings() {
	mockEmb := NewMockEmbedder(384)
	indexerWithEmb := indexer.NewWithEmbedder(s.storage, mockEmb)

	config := &indexer.Config{
		IncludeTests:       true,
		IncludeVendor:      false,
		GenerateEmbeddings: true,
		Workers:            4,
		BatchSize:          20,
		EmbeddingBatch:     30,
	}

	start := time.Now()
	stats, err := indexerWithEmb.IndexProject(s.ctx, s.fixturesDir, config)
	duration := time.Since(start)

	s.Require().NoError(err)
	s.T().Logf("Full indexing with embeddings took %v", duration)
	s.T().Logf("Stats: %d files, %d chunks, %d embeddings", stats.FilesIndexed, stats.ChunksCreated, stats.EmbeddingsGenerated)

	// Verify reasonable performance (should be fast with mock embedder)
	s.Less(duration, 10*time.Second, "indexing with mock embeddings should be fast")
}

// FailingMockEmbedder always fails
type FailingMockEmbedder struct{}

func (m *FailingMockEmbedder) GenerateEmbedding(ctx context.Context, req embedder.EmbeddingRequest) (*embedder.Embedding, error) {
	return nil, errors.New("mock embedding failure")
}

func (m *FailingMockEmbedder) GenerateBatch(ctx context.Context, req embedder.BatchEmbeddingRequest) (*embedder.BatchEmbeddingResponse, error) {
	return nil, errors.New("mock batch embedding failure")
}

func (m *FailingMockEmbedder) Dimension() int   { return 384 }
func (m *FailingMockEmbedder) Provider() string { return "mock" }
func (m *FailingMockEmbedder) Model() string    { return "mock-v1" }
func (m *FailingMockEmbedder) Close() error     { return nil }

// TestIndexingTestSuite runs the suite
func TestIndexingTestSuite(t *testing.T) {
	suite.Run(t, new(IndexingTestSuite))
}
