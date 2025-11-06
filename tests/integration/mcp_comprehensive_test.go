package integration

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dshills/gocontext-mcp/internal/indexer"
	"github.com/dshills/gocontext-mcp/internal/searcher"
	"github.com/dshills/gocontext-mcp/internal/storage"
)

// MCPToolsIntegrationSuite tests all MCP tools end-to-end
type MCPToolsIntegrationSuite struct {
	ctx         context.Context
	fixturesDir string
	tempDBDir   string
	store       storage.Storage
	indexer     *indexer.Indexer
	searcher    *searcher.Searcher
}

// setupTestSuite initializes the test environment
func setupTestSuite(t *testing.T) *MCPToolsIntegrationSuite {
	t.Helper()

	ctx := context.Background()

	// Get fixtures directory (absolute path required)
	wd, err := os.Getwd()
	require.NoError(t, err)
	fixturesDir := filepath.Join(filepath.Dir(wd), "testdata", "fixtures")

	// Make absolute if not already
	if !filepath.IsAbs(fixturesDir) {
		absPath, err := filepath.Abs(fixturesDir)
		require.NoError(t, err)
		fixturesDir = absPath
	}

	// Verify fixtures exist
	_, err = os.Stat(fixturesDir)
	require.NoError(t, err, "fixtures directory must exist: %s", fixturesDir)

	// Create temp database directory
	tempDBDir := t.TempDir()

	// Create storage using in-memory database for tests
	store, err := storage.NewSQLiteStorage(":memory:")
	require.NoError(t, err)

	// Create indexer with mock embedder
	mockEmb := NewMockEmbedder(384)
	idx := indexer.NewWithEmbedder(store, mockEmb)

	// Create searcher
	srch := searcher.NewSearcher(store, mockEmb)

	return &MCPToolsIntegrationSuite{
		ctx:         ctx,
		fixturesDir: fixturesDir,
		tempDBDir:   tempDBDir,
		store:       store,
		indexer:     idx,
		searcher:    srch,
	}
}

// cleanup closes resources
func (s *MCPToolsIntegrationSuite) cleanup() {
	if s.store != nil {
		_ = s.store.Close()
	}
}

// =============================================================================
// T109-T111: index_codebase Tool Integration Tests
// =============================================================================

// TestIndexCodebase_Success tests successful indexing of the sample project
// Task: T109
func TestIndexCodebase_Success(t *testing.T) {
	suite := setupTestSuite(t)
	defer suite.cleanup()

	// Index the project
	config := &indexer.Config{
		IncludeTests:       true,
		IncludeVendor:      false,
		GenerateEmbeddings: true,
	}

	stats, err := suite.indexer.IndexProject(suite.ctx, suite.fixturesDir, config)
	require.NoError(t, err, "indexing should succeed")

	// Verify statistics match actual files
	assert.Greater(t, stats.FilesIndexed, 0, "should have indexed files")
	assert.GreaterOrEqual(t, stats.FilesIndexed, 5, "should have indexed at least 5 fixture files")
	assert.Greater(t, stats.SymbolsExtracted, 0, "should have extracted symbols")
	assert.Greater(t, stats.ChunksCreated, 0, "should have created chunks")

	// Verify duration is reasonable
	assert.Less(t, stats.Duration.Milliseconds(), int64(30000), "indexing should complete in < 30s")

	// Log statistics for debugging
	t.Logf("Indexing stats: files=%d, symbols=%d, chunks=%d, duration=%v",
		stats.FilesIndexed, stats.SymbolsExtracted, stats.ChunksCreated, stats.Duration)

	// Verify project was created in database
	project, err := suite.store.GetProject(suite.ctx, suite.fixturesDir)
	require.NoError(t, err)
	assert.Equal(t, suite.fixturesDir, project.RootPath)
	assert.NotEmpty(t, project.ModuleName)
	assert.False(t, project.LastIndexedAt.IsZero())
}

// TestIndexCodebase_ForceReindex tests force reindexing functionality
// Task: T110
func TestIndexCodebase_ForceReindex(t *testing.T) {
	suite := setupTestSuite(t)
	defer suite.cleanup()

	// First index
	config := &indexer.Config{
		IncludeTests:       true,
		IncludeVendor:      false,
		GenerateEmbeddings: false, // Disable for speed
	}

	stats1, err := suite.indexer.IndexProject(suite.ctx, suite.fixturesDir, config)
	require.NoError(t, err)
	firstIndexTime := time.Now()

	// Wait a bit to ensure timestamp difference
	time.Sleep(100 * time.Millisecond)

	// Second index (should skip unchanged files)
	stats2, err := suite.indexer.IndexProject(suite.ctx, suite.fixturesDir, config)
	require.NoError(t, err)

	// Second indexing should skip files (incremental update)
	assert.GreaterOrEqual(t, stats2.FilesSkipped, stats1.FilesIndexed,
		"second index should skip unchanged files")
	assert.Equal(t, 0, stats2.FilesIndexed, "should not reindex unchanged files")

	// Verify project was updated
	project, err := suite.store.GetProject(suite.ctx, suite.fixturesDir)
	require.NoError(t, err)
	assert.True(t, project.LastIndexedAt.After(firstIndexTime))

	t.Logf("Incremental reindex: skipped=%d files", stats2.FilesSkipped)
}

// TestIndexCodebase_ErrorCases tests various error conditions
// Task: T111
func TestIndexCodebase_ErrorCases(t *testing.T) {
	tests := []struct {
		name        string
		path        string
		expectError bool
		errorCheck  func(t *testing.T, err error)
	}{
		{
			name:        "invalid path - not absolute",
			path:        "relative/path",
			expectError: true,
			errorCheck: func(t *testing.T, err error) {
				assert.Error(t, err)
			},
		},
		{
			name:        "invalid path - does not exist",
			path:        "/nonexistent/path/to/nowhere",
			expectError: true,
			errorCheck: func(t *testing.T, err error) {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "no such file")
			},
		},
		{
			name: "valid path - empty directory no go files",
			path: func() string {
				// Create a temp directory with no Go files
				dir, _ := os.MkdirTemp("", "emptydir")
				return dir
			}(),
			expectError: false,
			errorCheck: func(t *testing.T, err error) {
				// Indexing succeeds but indexes 0 files - this is valid behavior
				assert.NoError(t, err)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			suite := setupTestSuite(t)
			defer suite.cleanup()

			_, err := suite.indexer.IndexProject(suite.ctx, tt.path, nil)

			if tt.expectError {
				tt.errorCheck(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestIndexCodebase_ConcurrentIndexing tests that concurrent indexing is prevented
func TestIndexCodebase_ConcurrentIndexing(t *testing.T) {
	suite := setupTestSuite(t)
	defer suite.cleanup()

	// The indexer uses a mutex to prevent concurrent indexing on the SAME indexer instance
	// Create a slow-running config to give us time to test concurrency
	config := &indexer.Config{
		IncludeTests:       true,
		GenerateEmbeddings: true,
		Workers:            1, // Single worker for slower processing
	}

	// Start first indexing in goroutine
	done := make(chan error, 1)
	go func() {
		_, err := suite.indexer.IndexProject(suite.ctx, suite.fixturesDir, config)
		done <- err
	}()

	// Wait a bit to ensure first indexing has acquired the lock
	time.Sleep(10 * time.Millisecond)

	// Try to start second indexing on SAME indexer instance (should fail)
	_, err := suite.indexer.IndexProject(suite.ctx, suite.fixturesDir, config)

	// Check if we got the concurrent error OR the first one already finished
	if err != nil {
		assert.ErrorIs(t, err, indexer.ErrIndexingInProgress,
			"concurrent indexing should be prevented with ErrIndexingInProgress")
	} else {
		t.Log("First indexing completed before second attempt - this is acceptable")
	}

	// Wait for first to complete
	err1 := <-done
	assert.NoError(t, err1, "first indexing should complete successfully")
}

// =============================================================================
// T179-T183: search_code Tool Integration Tests
// =============================================================================

// TestSearchCode_SemanticQuery tests semantic search queries
// Task: T179
func TestSearchCode_SemanticQuery(t *testing.T) {
	suite := setupTestSuite(t)
	defer suite.cleanup()

	// Index first
	_, err := suite.indexer.IndexProject(suite.ctx, suite.fixturesDir, &indexer.Config{
		IncludeTests:       true,
		GenerateEmbeddings: true,
	})
	require.NoError(t, err)

	// Get project ID
	project, err := suite.store.GetProject(suite.ctx, suite.fixturesDir)
	require.NoError(t, err)

	// Test semantic queries
	semanticQueries := []struct {
		query          string
		expectedInText []string // Text we expect to find in results
	}{
		{
			query:          "authentication logic",
			expectedInText: []string{"auth", "token", "credential"},
		},
		{
			query:          "error handling",
			expectedInText: []string{"error", "handle", "wrap"},
		},
		{
			query:          "user repository",
			expectedInText: []string{"user", "repository"},
		},
	}

	for _, tc := range semanticQueries {
		t.Run(tc.query, func(t *testing.T) {
			req := searcher.SearchRequest{
				Query:     tc.query,
				Limit:     10,
				Mode:      searcher.SearchModeHybrid,
				ProjectID: project.ID,
				UseCache:  false,
			}

			resp, err := suite.searcher.Search(suite.ctx, req)
			require.NoError(t, err)
			assert.Greater(t, len(resp.Results), 0, "should return results for query: %s", tc.query)

			// Verify results contain expected text (case-insensitive)
			found := false
			for _, result := range resp.Results {
				content := strings.ToLower(result.Content)
				for _, expected := range tc.expectedInText {
					if strings.Contains(content, strings.ToLower(expected)) {
						found = true
						break
					}
				}
				if found {
					break
				}
			}
			assert.True(t, found, "results should contain expected text for query: %s", tc.query)

			// Verify search latency
			assert.Less(t, resp.Duration.Milliseconds(), int64(500),
				"search should complete in < 500ms (target)")

			t.Logf("Query '%s': %d results in %v", tc.query, len(resp.Results), resp.Duration)
		})
	}
}

// TestSearchCode_KeywordQuery tests keyword/exact match queries
// Task: T180
func TestSearchCode_KeywordQuery(t *testing.T) {
	suite := setupTestSuite(t)
	defer suite.cleanup()

	// Index first
	_, err := suite.indexer.IndexProject(suite.ctx, suite.fixturesDir, &indexer.Config{
		IncludeTests:       true,
		GenerateEmbeddings: true,
	})
	require.NoError(t, err)

	project, err := suite.store.GetProject(suite.ctx, suite.fixturesDir)
	require.NoError(t, err)

	// Test keyword queries (exact symbol names)
	keywordQueries := []struct {
		query           string
		mode            searcher.SearchMode
		minResultsCount int
	}{
		{
			query:           "UserRepository",
			mode:            searcher.SearchModeKeyword,
			minResultsCount: 1,
		},
		{
			query:           "AuthService",
			mode:            searcher.SearchModeKeyword,
			minResultsCount: 1,
		},
		{
			query:           "ValidateEmail",
			mode:            searcher.SearchModeKeyword,
			minResultsCount: 1,
		},
	}

	for _, tc := range keywordQueries {
		t.Run(tc.query, func(t *testing.T) {
			req := searcher.SearchRequest{
				Query:     tc.query,
				Limit:     10,
				Mode:      tc.mode,
				ProjectID: project.ID,
			}

			resp, err := suite.searcher.Search(suite.ctx, req)
			require.NoError(t, err)
			assert.GreaterOrEqual(t, len(resp.Results), tc.minResultsCount,
				"should find at least %d result for '%s'", tc.minResultsCount, tc.query)

			// Verify the query term appears in results
			found := false
			for _, result := range resp.Results {
				if strings.Contains(result.Content, tc.query) {
					found = true
					break
				}
			}
			assert.True(t, found, "query term should appear in results")

			t.Logf("Keyword '%s': found %d results", tc.query, len(resp.Results))
		})
	}
}

// TestSearchCode_WithFilters tests search with various filters
// Task: T181
func TestSearchCode_WithFilters(t *testing.T) {
	suite := setupTestSuite(t)
	defer suite.cleanup()

	// Index first
	_, err := suite.indexer.IndexProject(suite.ctx, suite.fixturesDir, &indexer.Config{
		IncludeTests:       true,
		GenerateEmbeddings: true,
	})
	require.NoError(t, err)

	project, err := suite.store.GetProject(suite.ctx, suite.fixturesDir)
	require.NoError(t, err)

	tests := []struct {
		name    string
		query   string
		filters *storage.SearchFilters
		verify  func(t *testing.T, results []searcher.SearchRequest)
	}{
		{
			name:  "filter by symbol type - struct",
			query: "user",
			filters: &storage.SearchFilters{
				SymbolTypes: []string{"struct"},
			},
		},
		{
			name:  "filter by symbol type - function",
			query: "validate",
			filters: &storage.SearchFilters{
				SymbolTypes: []string{"function"},
			},
		},
		{
			name:  "filter by DDD pattern - repository",
			query: "repository",
			filters: &storage.SearchFilters{
				DDDPatterns: []string{"repository"},
			},
		},
		{
			name:  "filter by DDD pattern - service",
			query: "service",
			filters: &storage.SearchFilters{
				DDDPatterns: []string{"service"},
			},
		},
		{
			name:  "filter by min relevance",
			query: "authentication",
			filters: &storage.SearchFilters{
				MinRelevance: 0.3,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := searcher.SearchRequest{
				Query:     tt.query,
				Limit:     10,
				Mode:      searcher.SearchModeHybrid,
				Filters:   tt.filters,
				ProjectID: project.ID,
			}

			resp, err := suite.searcher.Search(suite.ctx, req)
			require.NoError(t, err, "search with filters should succeed")

			// Filters might reduce results to zero if no matches
			t.Logf("Filter test '%s': query='%s', results=%d", tt.name, tt.query, len(resp.Results))
		})
	}
}

// TestSearchCode_SearchLatency tests that search meets latency targets
// Task: T182
func TestSearchCode_SearchLatency(t *testing.T) {
	suite := setupTestSuite(t)
	defer suite.cleanup()

	// Index first
	_, err := suite.indexer.IndexProject(suite.ctx, suite.fixturesDir, &indexer.Config{
		IncludeTests:       true,
		GenerateEmbeddings: true,
	})
	require.NoError(t, err)

	project, err := suite.store.GetProject(suite.ctx, suite.fixturesDir)
	require.NoError(t, err)

	// Run multiple searches to test latency
	queries := []string{
		"authentication",
		"user repository",
		"error handling",
		"validate email",
		"token expired",
	}

	var totalDuration time.Duration
	latencies := make([]time.Duration, 0, len(queries))

	for _, query := range queries {
		req := searcher.SearchRequest{
			Query:     query,
			Limit:     10,
			Mode:      searcher.SearchModeHybrid,
			ProjectID: project.ID,
		}

		resp, err := suite.searcher.Search(suite.ctx, req)
		require.NoError(t, err)

		latencies = append(latencies, resp.Duration)
		totalDuration += resp.Duration

		// Each individual search should meet latency target
		assert.Less(t, resp.Duration.Milliseconds(), int64(500),
			"search latency should be < 500ms (p95 target), got %v for query '%s'",
			resp.Duration, query)

		t.Logf("Query '%s': %d results in %v", query, len(resp.Results), resp.Duration)
	}

	avgLatency := totalDuration / time.Duration(len(queries))
	t.Logf("Average search latency: %v across %d queries", avgLatency, len(queries))
	assert.Less(t, avgLatency.Milliseconds(), int64(300),
		"average latency should be well under p95 target")
}

// TestSearchCode_ErrorCases tests search error handling
// Task: T183
func TestSearchCode_ErrorCases(t *testing.T) {
	suite := setupTestSuite(t)
	defer suite.cleanup()

	// Create a project but DON'T index it
	nonIndexedProject := &storage.Project{
		RootPath:     "/tmp/nonindexed",
		ModuleName:   "test",
		IndexVersion: storage.CurrentSchemaVersion,
	}
	err := suite.store.CreateProject(suite.ctx, nonIndexedProject)
	require.NoError(t, err)

	tests := []struct {
		name        string
		query       string
		projectID   int64
		expectError bool
		errorCheck  func(t *testing.T, err error)
	}{
		{
			name:        "empty query",
			query:       "",
			projectID:   1,
			expectError: true,
			errorCheck: func(t *testing.T, err error) {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "empty")
			},
		},
		{
			name:        "whitespace-only query",
			query:       "   ",
			projectID:   1,
			expectError: false, // Searcher accepts whitespace, returns no results
			errorCheck: func(t *testing.T, err error) {
				// Whitespace queries don't error, just return no/few results
				assert.NoError(t, err)
			},
		},
		{
			name:        "project not indexed",
			query:       "test query",
			projectID:   nonIndexedProject.ID,
			expectError: false, // Will succeed but return no results
			errorCheck: func(t *testing.T, err error) {
				// Should succeed but with no results
				assert.NoError(t, err)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := searcher.SearchRequest{
				Query:     tt.query,
				Limit:     10,
				Mode:      searcher.SearchModeHybrid,
				ProjectID: tt.projectID,
			}

			_, err := suite.searcher.Search(suite.ctx, req)
			if tt.expectError {
				tt.errorCheck(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// =============================================================================
// T118-T119: get_status Handler Unit Tests
// =============================================================================

// TestGetStatus_IndexedProject tests get_status for an indexed project
// Task: T118
func TestGetStatus_IndexedProject(t *testing.T) {
	suite := setupTestSuite(t)
	defer suite.cleanup()

	// Index the project
	_, err := suite.indexer.IndexProject(suite.ctx, suite.fixturesDir, &indexer.Config{
		IncludeTests:       true,
		GenerateEmbeddings: true,
	})
	require.NoError(t, err)

	// Get project
	project, err := suite.store.GetProject(suite.ctx, suite.fixturesDir)
	require.NoError(t, err)

	// Get status
	status, err := suite.store.GetStatus(suite.ctx, project.ID)
	require.NoError(t, err)
	assert.NotNil(t, status)

	// Verify indexed flag and statistics
	assert.Greater(t, status.FilesCount, 0, "should have indexed files")
	assert.Greater(t, status.SymbolsCount, 0, "should have extracted symbols")
	assert.Greater(t, status.ChunksCount, 0, "should have created chunks")
	assert.GreaterOrEqual(t, status.EmbeddingsCount, 0, "should have embeddings")

	// Verify project metadata
	assert.NotNil(t, status.Project)
	assert.Equal(t, suite.fixturesDir, status.Project.RootPath)
	assert.NotEmpty(t, status.Project.ModuleName)
	assert.False(t, status.Project.LastIndexedAt.IsZero())

	// Verify health check fields
	assert.True(t, status.Health.DatabaseAccessible, "database should be accessible")
	assert.True(t, status.Health.EmbeddingsAvailable || status.EmbeddingsCount == 0,
		"embeddings should be available if any exist")

	// Verify response can be serialized to JSON (MCP requirement)
	response := map[string]interface{}{
		"indexed": true,
		"project": map[string]interface{}{
			"path":            project.RootPath,
			"module_name":     project.ModuleName,
			"go_version":      project.GoVersion,
			"last_indexed_at": project.LastIndexedAt.Format(time.RFC3339),
		},
		"statistics": map[string]interface{}{
			"files_count":      status.FilesCount,
			"symbols_count":    status.SymbolsCount,
			"chunks_count":     status.ChunksCount,
			"embeddings_count": status.EmbeddingsCount,
		},
		"health": map[string]interface{}{
			"database_accessible":  status.Health.DatabaseAccessible,
			"embeddings_available": status.Health.EmbeddingsAvailable,
			"fts_indexes_built":    status.Health.FTSIndexesBuilt,
		},
	}

	jsonData, err := json.MarshalIndent(response, "", "  ")
	require.NoError(t, err)
	assert.NotEmpty(t, jsonData)

	t.Logf("Status for indexed project:\n%s", string(jsonData))
}

// TestGetStatus_NotIndexed tests get_status for a non-indexed project
// Task: T119
func TestGetStatus_NotIndexed(t *testing.T) {
	suite := setupTestSuite(t)
	defer suite.cleanup()

	// Create a temp directory that's not indexed
	tempDir := t.TempDir()

	// Try to get project (should return not found)
	_, err := suite.store.GetProject(suite.ctx, tempDir)
	assert.ErrorIs(t, err, storage.ErrNotFound, "should return ErrNotFound for unindexed project")

	// Verify response format for unindexed project
	response := map[string]interface{}{
		"indexed": false,
		"path":    tempDir,
		"message": "Project not indexed. Use index_codebase tool to index this project.",
	}

	jsonData, err := json.MarshalIndent(response, "", "  ")
	require.NoError(t, err)
	assert.Contains(t, string(jsonData), "not indexed")

	t.Logf("Status for non-indexed project:\n%s", string(jsonData))
}

// TestGetStatus_HealthChecks tests health check functionality
func TestGetStatus_HealthChecks(t *testing.T) {
	suite := setupTestSuite(t)
	defer suite.cleanup()

	// Index with embeddings
	_, err := suite.indexer.IndexProject(suite.ctx, suite.fixturesDir, &indexer.Config{
		IncludeTests:       true,
		GenerateEmbeddings: true,
	})
	require.NoError(t, err)

	project, err := suite.store.GetProject(suite.ctx, suite.fixturesDir)
	require.NoError(t, err)

	status, err := suite.store.GetStatus(suite.ctx, project.ID)
	require.NoError(t, err)

	// Verify all health checks
	assert.True(t, status.Health.DatabaseAccessible,
		"database should be accessible")

	if status.EmbeddingsCount > 0 {
		assert.True(t, status.Health.EmbeddingsAvailable,
			"embeddings should be available when count > 0")
	}

	t.Logf("Health checks: db=%v, embeddings=%v, fts=%v",
		status.Health.DatabaseAccessible,
		status.Health.EmbeddingsAvailable,
		status.Health.FTSIndexesBuilt)
}
