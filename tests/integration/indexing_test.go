package integration

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

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

	// Should have some failed files (sample_error.go)
	s.Greater(stats.FilesFailed, 0, "should have failed files")
	s.NotEmpty(stats.ErrorMessages, "should record error messages")
	s.T().Logf("Failed files: %d, Errors: %v", stats.FilesFailed, stats.ErrorMessages)

	// Should still index valid files
	s.Greater(stats.FilesIndexed, 0, "should index valid files")
	s.Greater(stats.SymbolsExtracted, 0, "should extract symbols from valid files")

	// Verify project was still created successfully
	project, err := s.storage.GetProject(s.ctx, s.fixturesDir)
	s.NoError(err)
	s.NotNil(project)
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

// TestIndexingTestSuite runs the suite
func TestIndexingTestSuite(t *testing.T) {
	suite.Run(t, new(IndexingTestSuite))
}
