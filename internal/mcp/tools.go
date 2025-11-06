package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/dshills/gocontext-mcp/internal/indexer"
	"github.com/dshills/gocontext-mcp/internal/storage"
)

// MCP error codes
const (
	ErrorCodeInvalidParams      = -32602 // Invalid method parameters
	ErrorCodeInternalError      = -32603 // Internal JSON-RPC error
	ErrorCodeProjectNotFound    = -32001 // Specified path does not contain a Go project
	ErrorCodeIndexingInProgress = -32002 // Another indexing operation is already running
	ErrorCodeNotIndexed         = -32003 // Project not indexed
	ErrorCodeEmptyQuery         = -32004 // Query parameter is empty
)

// handleIndexCodebase handles the index_codebase tool invocation
func (s *Server) handleIndexCodebase(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Extract and validate parameters
	args, ok := request.Params.Arguments.(map[string]interface{})
	if !ok {
		return nil, newMCPError(ErrorCodeInvalidParams, "invalid arguments", nil)
	}

	path, ok := args["path"].(string)
	if !ok || path == "" {
		return nil, newMCPError(ErrorCodeInvalidParams, "path parameter is required", map[string]interface{}{
			"param":  "path",
			"reason": "missing or empty",
		})
	}

	// Validate path exists and is accessible
	if err := validatePath(path); err != nil {
		return nil, newMCPError(ErrorCodeInvalidParams, "invalid path", map[string]interface{}{
			"param":  "path",
			"reason": err.Error(),
		})
	}

	// Parse optional parameters
	forceReindex, _ := args["force_reindex"].(bool)
	includeTests := getBoolDefault(args, "include_tests", true)
	includeVendor := getBoolDefault(args, "include_vendor", false)

	// Create indexer config
	config := &indexer.Config{
		IncludeTests:  includeTests,
		IncludeVendor: includeVendor,
	}

	// Run indexing
	stats, err := s.indexer.IndexProject(ctx, path, config)
	if err != nil {
		return nil, newMCPError(ErrorCodeInternalError, "indexing failed", map[string]interface{}{
			"error": err.Error(),
		})
	}

	// Format response
	response := map[string]interface{}{
		"indexed":           true,
		"files_indexed":     stats.FilesIndexed,
		"files_skipped":     stats.FilesSkipped,
		"files_failed":      stats.FilesFailed,
		"symbols_extracted": stats.SymbolsExtracted,
		"chunks_created":    stats.ChunksCreated,
		"duration_ms":       stats.Duration.Milliseconds(),
	}

	if len(stats.ErrorMessages) > 0 {
		// Include first few errors
		errorCount := len(stats.ErrorMessages)
		if errorCount > 5 {
			response["errors"] = stats.ErrorMessages[:5]
			response["error_count"] = errorCount
		} else {
			response["errors"] = stats.ErrorMessages
		}
	}

	_ = forceReindex // TODO: Implement force reindex logic

	return mcp.NewToolResultText(formatJSON(response)), nil
}

// handleSearchCode handles the search_code tool invocation
func (s *Server) handleSearchCode(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Extract and validate parameters
	args, ok := request.Params.Arguments.(map[string]interface{})
	if !ok {
		return nil, newMCPError(ErrorCodeInvalidParams, "invalid arguments", nil)
	}

	path, ok := args["path"].(string)
	if !ok || path == "" {
		return nil, newMCPError(ErrorCodeInvalidParams, "path parameter is required", map[string]interface{}{
			"param":  "path",
			"reason": "missing or empty",
		})
	}

	query, ok := args["query"].(string)
	if !ok || query == "" {
		return nil, newMCPError(ErrorCodeEmptyQuery, "query parameter is required and cannot be empty", map[string]interface{}{
			"param":  "query",
			"reason": "missing or empty",
		})
	}

	// Validate path exists and is accessible
	if err := validatePath(path); err != nil {
		return nil, newMCPError(ErrorCodeInvalidParams, "invalid path", map[string]interface{}{
			"param":  "path",
			"reason": err.Error(),
		})
	}

	// Parse optional parameters
	limit := getIntDefault(args, "limit", 10)
	if limit < 1 || limit > 100 {
		return nil, newMCPError(ErrorCodeInvalidParams, "limit must be between 1 and 100", map[string]interface{}{
			"param": "limit",
			"value": limit,
		})
	}

	searchMode := getStringDefault(args, "search_mode", "hybrid")
	if searchMode != "hybrid" && searchMode != "vector" && searchMode != "keyword" {
		return nil, newMCPError(ErrorCodeInvalidParams, "invalid search_mode", map[string]interface{}{
			"param":   "search_mode",
			"value":   searchMode,
			"allowed": []string{"hybrid", "vector", "keyword"},
		})
	}

	// Parse filters
	filters, _ := args["filters"].(map[string]interface{})
	_ = filters

	// TODO: Implement actual search logic

	// Return stub response
	return mcp.NewToolResultText(fmt.Sprintf("Search operation not yet implemented for query: %s", query)), nil
}

// handleGetStatus handles the get_status tool invocation
func (s *Server) handleGetStatus(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Extract and validate parameters
	args, ok := request.Params.Arguments.(map[string]interface{})
	if !ok {
		return nil, newMCPError(ErrorCodeInvalidParams, "invalid arguments", nil)
	}

	path, ok := args["path"].(string)
	if !ok || path == "" {
		return nil, newMCPError(ErrorCodeInvalidParams, "path parameter is required", map[string]interface{}{
			"param":  "path",
			"reason": "missing or empty",
		})
	}

	// Validate path exists and is accessible
	if err := validatePath(path); err != nil {
		return nil, newMCPError(ErrorCodeInvalidParams, "invalid path", map[string]interface{}{
			"param":  "path",
			"reason": err.Error(),
		})
	}

	// Try to get project
	project, err := s.storage.GetProject(ctx, path)
	if err == storage.ErrNotFound {
		// Project not indexed
		response := map[string]interface{}{
			"indexed": false,
			"path":    path,
			"message": "Project not indexed. Use index_codebase tool to index this project.",
		}
		return mcp.NewToolResultText(formatJSON(response)), nil
	}
	if err != nil {
		return nil, newMCPError(ErrorCodeInternalError, "failed to get project status", map[string]interface{}{
			"error": err.Error(),
		})
	}

	// Get detailed status
	status, err := s.storage.GetStatus(ctx, project.ID)
	if err != nil {
		return nil, newMCPError(ErrorCodeInternalError, "failed to get status", map[string]interface{}{
			"error": err.Error(),
		})
	}

	// Format response
	response := map[string]interface{}{
		"indexed": true,
		"project": map[string]interface{}{
			"path":            project.RootPath,
			"module_name":     project.ModuleName,
			"go_version":      project.GoVersion,
			"last_indexed_at": project.LastIndexedAt.Format("2006-01-02T15:04:05Z07:00"),
		},
		"statistics": map[string]interface{}{
			"files_count":      status.FilesCount,
			"symbols_count":    status.SymbolsCount,
			"chunks_count":     status.ChunksCount,
			"embeddings_count": status.EmbeddingsCount,
			"index_size_mb":    fmt.Sprintf("%.2f", status.IndexSizeMB),
		},
		"health": map[string]interface{}{
			"database_accessible":  status.Health.DatabaseAccessible,
			"embeddings_available": status.Health.EmbeddingsAvailable,
			"fts_indexes_built":    status.Health.FTSIndexesBuilt,
		},
	}

	return mcp.NewToolResultText(formatJSON(response)), nil
}

// Helper functions

// newMCPError creates a properly formatted MCP error
func newMCPError(code int, message string, data interface{}) error {
	// MCP errors are returned as regular errors, the framework handles encoding
	return &MCPError{
		Code:    code,
		Message: message,
		Data:    data,
	}
}

// MCPError represents an MCP protocol error
type MCPError struct {
	Code    int
	Message string
	Data    interface{}
}

func (e *MCPError) Error() string {
	return fmt.Sprintf("MCP error %d: %s", e.Code, e.Message)
}

// validatePath checks if a path exists and is accessible
func validatePath(path string) error {
	if path == "" {
		return ErrPathRequired
	}

	// Check if path is absolute
	if !filepath.IsAbs(path) {
		return ErrPathNotAbsolute
	}

	// Check if path exists
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return ErrPathNotFound
	}
	if err != nil {
		return ErrPathNotReadable
	}

	// Check if it's a directory
	if !info.IsDir() {
		return ErrNotDirectory
	}

	// Check if directory is readable
	f, err := os.Open(path)
	if err != nil {
		return ErrPathNotReadable
	}
	_ = f.Close()

	// Check for Go files
	hasGoFiles := false
	_ = filepath.Walk(path, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() && strings.HasSuffix(p, ".go") {
			hasGoFiles = true
			// Continue walking - we just need to know if at least one Go file exists
		}
		return nil
	})

	if !hasGoFiles {
		return ErrNoGoFiles
	}

	return nil
}

// formatJSON formats a map as indented JSON
func formatJSON(data map[string]interface{}) string {
	bytes, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Sprintf("%v", data)
	}
	return string(bytes)
}

// getBoolDefault extracts a boolean parameter with a default value
func getBoolDefault(args map[string]interface{}, key string, defaultValue bool) bool {
	if val, ok := args[key].(bool); ok {
		return val
	}
	return defaultValue
}

// getIntDefault extracts an integer parameter with a default value
func getIntDefault(args map[string]interface{}, key string, defaultValue int) int {
	if val, ok := args[key].(float64); ok {
		return int(val)
	}
	if val, ok := args[key].(int); ok {
		return val
	}
	return defaultValue
}

// getStringDefault extracts a string parameter with a default value
func getStringDefault(args map[string]interface{}, key string, defaultValue string) string {
	if val, ok := args[key].(string); ok {
		return val
	}
	return defaultValue
}

// Validation helpers

var (
	ErrPathRequired    = errors.New("path is required")
	ErrPathNotAbsolute = errors.New("path must be absolute")
	ErrPathNotFound    = errors.New("path does not exist")
	ErrPathNotReadable = errors.New("path is not readable")
	ErrNotDirectory    = errors.New("path is not a directory")
	ErrNoGoFiles       = errors.New("directory does not contain Go files")
)
