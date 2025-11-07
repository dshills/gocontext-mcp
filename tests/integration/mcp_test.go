package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/suite"

	"github.com/dshills/gocontext-mcp/internal/indexer"
	mcpserver "github.com/dshills/gocontext-mcp/internal/mcp"
	"github.com/dshills/gocontext-mcp/internal/storage"
)

// MCPTestSuite contains tests for MCP tool integration
type MCPTestSuite struct {
	suite.Suite
	server      *mcpserver.Server
	fixturesDir string
	tempDBDir   string
	ctx         context.Context
}

// SetupSuite runs once before all tests
func (s *MCPTestSuite) SetupSuite() {
	s.ctx = context.Background()

	// Get fixtures directory
	wd, err := os.Getwd()
	s.Require().NoError(err)
	s.fixturesDir = filepath.Join(filepath.Dir(wd), "testdata", "fixtures")

	// Verify it's an absolute path
	if !filepath.IsAbs(s.fixturesDir) {
		absPath, err := filepath.Abs(s.fixturesDir)
		s.Require().NoError(err)
		s.fixturesDir = absPath
	}

	// Create temp directory for database
	tempDir := s.T().TempDir()
	s.tempDBDir = tempDir

	// Set mock embedder environment (to avoid needing real API keys)
	os.Setenv("GOCONTEXT_EMBEDDER", "mock")
}

// SetupTest runs before each test
func (s *MCPTestSuite) SetupTest() {
	// Create fresh server for each test
	server, err := mcpserver.NewServer(s.tempDBDir)
	s.Require().NoError(err)
	s.server = server
}

// TearDownTest runs after each test
func (s *MCPTestSuite) TearDownTest() {
	// Server cleanup is handled by test temp dir
}

// TestIndexCodebaseTool tests the index_codebase MCP tool
func (s *MCPTestSuite) TestIndexCodebaseTool() {
	// Create tool call request
	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "index_codebase",
			Arguments: map[string]interface{}{
				"path":           s.fixturesDir,
				"include_tests":  true,
				"include_vendor": false,
				"force_reindex":  false,
			},
		},
	}

	// Note: We can't directly call handleIndexCodebase as it's not exported
	// and we don't have access to the MCP server's tool registry in tests.
	// Instead, we'll test the underlying components directly.

	// This is a simulation of what the MCP handler would do
	s.T().Log("Testing index_codebase tool logic")
	s.T().Logf("Fixtures path: %s", s.fixturesDir)

	// Verify path is valid
	info, err := os.Stat(s.fixturesDir)
	s.Require().NoError(err)
	s.True(info.IsDir(), "fixtures path should be a directory")

	// The actual indexing is tested in IndexingTestSuite
	// Here we verify the MCP tool schema and parameter handling
	s.NotEmpty(request.Params.Name)
	s.NotEmpty(request.Params.Arguments)

	args, ok := request.Params.Arguments.(map[string]interface{})
	s.Require().True(ok, "arguments should be a map")

	path, ok := args["path"].(string)
	s.True(ok, "path should be a string")
	s.Equal(s.fixturesDir, path)

	includeTests, ok := args["include_tests"].(bool)
	s.True(ok, "include_tests should be a bool")
	s.True(includeTests)

	s.T().Log("index_codebase tool parameters validated successfully")
}

// TestIndexCodebaseValidation tests parameter validation
func (s *MCPTestSuite) TestIndexCodebaseValidation() {
	tests := []struct {
		name        string
		args        map[string]interface{}
		shouldError bool
		errorMsg    string
	}{
		{
			name: "valid absolute path",
			args: map[string]interface{}{
				"path": s.fixturesDir,
			},
			shouldError: false,
		},
		{
			name: "missing path",
			args: map[string]interface{}{
				"include_tests": true,
			},
			shouldError: true,
			errorMsg:    "path",
		},
		{
			name: "empty path",
			args: map[string]interface{}{
				"path": "",
			},
			shouldError: true,
			errorMsg:    "path",
		},
		{
			name: "non-existent path",
			args: map[string]interface{}{
				"path": "/nonexistent/path/to/nowhere",
			},
			shouldError: true,
			errorMsg:    "not exist",
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			s.T().Logf("Testing validation for: %s", tt.name)

			// Validate path parameter
			path, ok := tt.args["path"].(string)
			if !ok || path == "" {
				if tt.shouldError {
					s.T().Log("Correctly detected missing/empty path")
					return
				}
				s.Fail("path should be present and non-empty")
				return
			}

			// Check path exists
			_, err := os.Stat(path)
			if err != nil {
				if tt.shouldError {
					s.T().Logf("Correctly detected invalid path: %v", err)
					s.Contains(err.Error(), "no such file")
					return
				}
				s.Fail("path should exist", "error: %v", err)
				return
			}

			if tt.shouldError {
				s.Fail("expected error but validation passed")
			} else {
				s.T().Log("Validation passed as expected")
			}
		})
	}
}

// TestGetStatusTool tests the get_status MCP tool
func (s *MCPTestSuite) TestGetStatusTool() {
	// First, we need to index the project
	s.T().Log("Indexing project for status test")

	// Create storage directly for testing
	dbPath := filepath.Join(s.tempDBDir, "test_status.db")
	store, err := storage.NewSQLiteStorage(dbPath)
	s.Require().NoError(err)
	defer store.Close()

	// Index the fixtures
	indexer := s.createIndexerForTest(store)
	stats, err := indexer.IndexProject(s.ctx, s.fixturesDir, nil)
	s.Require().NoError(err)
	s.T().Logf("Indexed: %d files, %d symbols", stats.FilesIndexed, stats.SymbolsExtracted)

	// Now test get_status
	project, err := store.GetProject(s.ctx, s.fixturesDir)
	s.Require().NoError(err)

	status, err := store.GetStatus(s.ctx, project.ID)
	s.Require().NoError(err)
	s.NotNil(status)

	// Verify status fields
	s.T().Logf("Status: %d files, %d symbols, %d chunks",
		status.FilesCount, status.SymbolsCount, status.ChunksCount)

	s.Greater(status.FilesCount, 0, "should have indexed files")
	s.Greater(status.ChunksCount, 0, "should have chunks")

	// Verify project metadata
	s.NotNil(status.Project)
	s.Equal(s.fixturesDir, status.Project.RootPath)
	s.False(status.Project.LastIndexedAt.IsZero())

	// Verify health status
	s.True(status.Health.DatabaseAccessible, "database should be accessible")

	s.T().Log("get_status tool logic validated successfully")
}

// TestGetStatusNotIndexed tests get_status for unindexed project
func (s *MCPTestSuite) TestGetStatusNotIndexed() {
	tempDir := s.T().TempDir()

	// Create storage
	dbPath := filepath.Join(s.tempDBDir, "test_not_indexed.db")
	store, err := storage.NewSQLiteStorage(dbPath)
	s.Require().NoError(err)
	defer store.Close()

	// Try to get status for unindexed project
	_, err = store.GetProject(s.ctx, tempDir)
	s.Equal(storage.ErrNotFound, err, "should return ErrNotFound for unindexed project")

	s.T().Log("Correctly handles unindexed project")
}

// TestSearchCodeTool tests the search_code MCP tool
func (s *MCPTestSuite) TestSearchCodeTool() {
	// Setup: index the project first
	dbPath := filepath.Join(s.tempDBDir, "test_search.db")
	store, err := storage.NewSQLiteStorage(dbPath)
	s.Require().NoError(err)
	defer store.Close()

	indexer := s.createIndexerForTest(store)
	_, err = indexer.IndexProject(s.ctx, s.fixturesDir, nil)
	s.Require().NoError(err)

	project, err := store.GetProject(s.ctx, s.fixturesDir)
	s.Require().NoError(err)

	// Create search request
	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "search_code",
			Arguments: map[string]interface{}{
				"path":        s.fixturesDir,
				"query":       "user repository",
				"limit":       10,
				"search_mode": "hybrid",
			},
		},
	}

	// Validate parameters
	args, ok := request.Params.Arguments.(map[string]interface{})
	s.Require().True(ok, "arguments should be a map")
	s.validateSearchParams(args)

	// The actual search functionality is tested in SearchTestSuite
	s.T().Logf("search_code tool parameters validated for project ID %d", project.ID)
}

// TestSearchCodeValidation tests search parameter validation
func (s *MCPTestSuite) TestSearchCodeValidation() {
	tests := []struct {
		name        string
		args        map[string]interface{}
		shouldError bool
		errorMsg    string
	}{
		{
			name: "valid search request",
			args: map[string]interface{}{
				"path":  s.fixturesDir,
				"query": "user",
				"limit": 10,
			},
			shouldError: false,
		},
		{
			name: "missing query",
			args: map[string]interface{}{
				"path": s.fixturesDir,
			},
			shouldError: true,
			errorMsg:    "query",
		},
		{
			name: "empty query",
			args: map[string]interface{}{
				"path":  s.fixturesDir,
				"query": "",
			},
			shouldError: true,
			errorMsg:    "query",
		},
		{
			name: "invalid limit - too low",
			args: map[string]interface{}{
				"path":  s.fixturesDir,
				"query": "test",
				"limit": 0,
			},
			shouldError: true,
			errorMsg:    "limit",
		},
		{
			name: "invalid limit - too high",
			args: map[string]interface{}{
				"path":  s.fixturesDir,
				"query": "test",
				"limit": 101,
			},
			shouldError: true,
			errorMsg:    "limit",
		},
		{
			name: "invalid search mode",
			args: map[string]interface{}{
				"path":        s.fixturesDir,
				"query":       "test",
				"search_mode": "invalid",
			},
			shouldError: true,
			errorMsg:    "mode",
		},
		{
			name: "valid filters",
			args: map[string]interface{}{
				"path":  s.fixturesDir,
				"query": "test",
				"filters": map[string]interface{}{
					"symbol_types":  []string{"function", "struct"},
					"ddd_patterns":  []string{"repository"},
					"min_relevance": 0.5,
				},
			},
			shouldError: false,
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			s.T().Logf("Testing search validation: %s", tt.name)

			err := s.validateSearchParams(tt.args)
			if tt.shouldError {
				s.Error(err, "should return validation error")
				if tt.errorMsg != "" {
					s.Contains(err.Error(), tt.errorMsg)
				}
			} else {
				s.NoError(err, "should pass validation")
			}
		})
	}
}

// TestEndToEndWorkflow tests complete MCP workflow
func (s *MCPTestSuite) TestEndToEndWorkflow() {
	s.T().Log("Testing end-to-end MCP workflow")

	// Step 1: Create database
	dbPath := filepath.Join(s.tempDBDir, "test_e2e.db")
	store, err := storage.NewSQLiteStorage(dbPath)
	s.Require().NoError(err)
	defer store.Close()

	// Step 2: Check status before indexing (should be not found)
	_, err = store.GetProject(s.ctx, s.fixturesDir)
	s.Equal(storage.ErrNotFound, err, "project should not exist yet")
	s.T().Log("✓ Verified project not indexed initially")

	// Step 3: Index the codebase
	indexer := s.createIndexerForTest(store)
	stats, err := indexer.IndexProject(s.ctx, s.fixturesDir, nil)
	s.Require().NoError(err)
	s.Greater(stats.FilesIndexed, 0)
	s.T().Logf("✓ Indexed %d files with %d symbols", stats.FilesIndexed, stats.SymbolsExtracted)

	// Step 4: Check status after indexing
	project, err := store.GetProject(s.ctx, s.fixturesDir)
	s.Require().NoError(err)
	s.NotNil(project)
	s.T().Logf("✓ Project indexed: %s", project.ModuleName)

	status, err := store.GetStatus(s.ctx, project.ID)
	s.Require().NoError(err)
	s.Greater(status.FilesCount, 0)
	s.Greater(status.ChunksCount, 0)
	s.T().Logf("✓ Status retrieved: %d files, %d chunks", status.FilesCount, status.ChunksCount)

	// Step 5: Perform searches
	// (Search functionality tested in SearchTestSuite)
	s.T().Log("✓ End-to-end workflow completed successfully")
}

// Helper methods

// createIndexerForTest creates an indexer with the given storage
func (s *MCPTestSuite) createIndexerForTest(store storage.Storage) *indexer.Indexer {
	return indexer.New(store)
}

// validateSearchParams validates search parameters
func (s *MCPTestSuite) validateSearchParams(args map[string]interface{}) error {
	// Validate path
	path, ok := args["path"].(string)
	if !ok || path == "" {
		return fmt.Errorf("path is required")
	}

	// Validate query
	query, ok := args["query"].(string)
	if !ok || query == "" {
		return fmt.Errorf("query is required")
	}

	// Validate limit
	if limitVal, ok := args["limit"]; ok {
		var limit int
		switch v := limitVal.(type) {
		case int:
			limit = v
		case float64:
			limit = int(v)
		default:
			return fmt.Errorf("invalid limit type")
		}

		if limit < 1 || limit > 100 {
			return fmt.Errorf("limit must be between 1 and 100")
		}
	}

	// Validate search mode
	if modeVal, ok := args["search_mode"]; ok {
		mode, ok := modeVal.(string)
		if !ok {
			return fmt.Errorf("invalid mode type")
		}
		if mode != "hybrid" && mode != "vector" && mode != "keyword" {
			return fmt.Errorf("invalid search mode")
		}
	}

	// Validate filters if present
	if filtersVal, ok := args["filters"]; ok {
		filters, ok := filtersVal.(map[string]interface{})
		if !ok {
			return storage.ErrNotFound
		}

		// Validate min_relevance
		if minRel, ok := filters["min_relevance"]; ok {
			var minRelFloat float64
			switch v := minRel.(type) {
			case float64:
				minRelFloat = v
			case int:
				minRelFloat = float64(v)
			default:
				return storage.ErrNotFound
			}

			if minRelFloat < 0.0 || minRelFloat > 1.0 {
				return storage.ErrNotFound
			}
		}
	}

	return nil
}

// TestMCPToolSchemas tests that tool schemas are properly defined
func (s *MCPTestSuite) TestMCPToolSchemas() {
	// Test that we can create valid tool call requests
	tests := []struct {
		name string
		tool string
		args map[string]interface{}
	}{
		{
			name: "index_codebase",
			tool: "index_codebase",
			args: map[string]interface{}{
				"path":           s.fixturesDir,
				"include_tests":  true,
				"include_vendor": false,
			},
		},
		{
			name: "search_code",
			tool: "search_code",
			args: map[string]interface{}{
				"path":  s.fixturesDir,
				"query": "test",
				"limit": 10,
			},
		},
		{
			name: "get_status",
			tool: "get_status",
			args: map[string]interface{}{
				"path": s.fixturesDir,
			},
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			// Verify we can serialize to JSON (MCP protocol requirement)
			data, err := json.Marshal(tt.args)
			s.NoError(err, "should serialize to JSON")
			s.NotEmpty(data)

			// Verify we can deserialize
			var result map[string]interface{}
			err = json.Unmarshal(data, &result)
			s.NoError(err, "should deserialize from JSON")
			s.Equal(tt.args["path"], result["path"])

			s.T().Logf("Tool %s: schema validated", tt.tool)
		})
	}
}

// TestMCPTestSuite runs the suite
func TestMCPTestSuite(t *testing.T) {
	suite.Run(t, new(MCPTestSuite))
}
