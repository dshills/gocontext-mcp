package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/dshills/gocontext-mcp/internal/indexer"
	"github.com/dshills/gocontext-mcp/internal/searcher"
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

	// Validate required parameters
	path, query, err := validateSearchParams(args)
	if err != nil {
		return nil, err
	}

	// Check if project is indexed
	project, err := s.storage.GetProject(ctx, path)
	if err == storage.ErrNotFound {
		return nil, newMCPError(ErrorCodeNotIndexed, "project not indexed", map[string]interface{}{
			"path":    path,
			"message": "Run index_codebase tool first to index this project",
		})
	}
	if err != nil {
		return nil, newMCPError(ErrorCodeInternalError, "failed to get project", map[string]interface{}{
			"error": err.Error(),
		})
	}

	// Parse and validate optional parameters
	limit, searchMode, searchFilters, err := parseSearchOptions(args)
	if err != nil {
		return nil, err
	}

	// Sanitize query for SQL FTS to prevent injection
	sanitizedQuery := sanitizeQueryForFTS(query)

	// Build search request
	searchReq := searcher.SearchRequest{
		Query:     sanitizedQuery,
		Limit:     limit,
		Mode:      searcher.SearchMode(searchMode),
		Filters:   searchFilters,
		ProjectID: project.ID,
		UseCache:  true, // Enable caching for performance
	}

	// Perform search
	searchResp, err := s.searcher.Search(ctx, searchReq)
	if err != nil {
		return nil, newMCPError(ErrorCodeInternalError, "search failed", map[string]interface{}{
			"error": err.Error(),
		})
	}

	// Format response
	response := formatSearchResponse(query, searchResp)

	return mcp.NewToolResultText(formatJSON(response)), nil
}

// validateSearchParams validates required search parameters (path and query)
func validateSearchParams(args map[string]interface{}) (path string, query string, err error) {
	path, ok := args["path"].(string)
	if !ok || path == "" {
		return "", "", newMCPError(ErrorCodeInvalidParams, "path parameter is required", map[string]interface{}{
			"param":  "path",
			"reason": "missing or empty",
		})
	}

	query, ok = args["query"].(string)
	if !ok || query == "" {
		return "", "", newMCPError(ErrorCodeEmptyQuery, "query parameter is required and cannot be empty", map[string]interface{}{
			"param":  "query",
			"reason": "missing or empty",
		})
	}

	// Trim whitespace and check if query is empty
	query = strings.TrimSpace(query)
	if query == "" {
		return "", "", newMCPError(ErrorCodeEmptyQuery, "query parameter cannot be whitespace-only", map[string]interface{}{
			"param":  "query",
			"reason": "empty after trimming whitespace",
		})
	}

	// Validate path exists and is accessible
	if err := validatePath(path); err != nil {
		return "", "", newMCPError(ErrorCodeInvalidParams, "invalid path", map[string]interface{}{
			"param":  "path",
			"reason": err.Error(),
		})
	}

	return path, query, nil
}

// parseSearchOptions parses and validates optional search parameters
func parseSearchOptions(args map[string]interface{}) (limit int, searchMode string, filters *storage.SearchFilters, err error) {
	// Parse limit
	limit = getIntDefault(args, "limit", 10)
	if limit < 1 || limit > 100 {
		return 0, "", nil, newMCPError(ErrorCodeInvalidParams, "limit must be between 1 and 100", map[string]interface{}{
			"param": "limit",
			"value": limit,
		})
	}

	// Parse search mode
	searchMode = getStringDefault(args, "search_mode", "hybrid")
	if searchMode != "hybrid" && searchMode != "vector" && searchMode != "keyword" {
		return 0, "", nil, newMCPError(ErrorCodeInvalidParams, "invalid search_mode", map[string]interface{}{
			"param":   "search_mode",
			"value":   searchMode,
			"allowed": []string{"hybrid", "vector", "keyword"},
		})
	}

	// Parse and validate filters
	filters, err = parseSearchFilters(args)
	if err != nil {
		return 0, "", nil, newMCPError(ErrorCodeInvalidParams, "invalid filters", map[string]interface{}{
			"error": err.Error(),
		})
	}

	return limit, searchMode, filters, nil
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

// parseSearchFilters extracts and validates search filters from request arguments
//
//nolint:gocyclo // Complexity from thorough validation of multiple filter types
func parseSearchFilters(args map[string]interface{}) (*storage.SearchFilters, error) {
	filtersArg, ok := args["filters"].(map[string]interface{})
	if !ok || len(filtersArg) == 0 {
		return nil, nil // No filters specified
	}

	filters := &storage.SearchFilters{}

	// Parse symbol_types
	if symbolTypes, ok := filtersArg["symbol_types"].([]interface{}); ok {
		filters.SymbolTypes = make([]string, 0, len(symbolTypes))
		for _, st := range symbolTypes {
			if s, ok := st.(string); ok {
				if !isValidSymbolType(s) {
					return nil, fmt.Errorf("invalid symbol_type: %s", s)
				}
				filters.SymbolTypes = append(filters.SymbolTypes, s)
			}
		}
	}

	// Parse file_pattern
	if filePattern, ok := filtersArg["file_pattern"].(string); ok {
		filters.FilePattern = filePattern
	}

	// Parse ddd_patterns
	if dddPatterns, ok := filtersArg["ddd_patterns"].([]interface{}); ok {
		filters.DDDPatterns = make([]string, 0, len(dddPatterns))
		for _, dp := range dddPatterns {
			if s, ok := dp.(string); ok {
				if !isValidDDDPattern(s) {
					return nil, fmt.Errorf("invalid ddd_pattern: %s", s)
				}
				filters.DDDPatterns = append(filters.DDDPatterns, s)
			}
		}
	}

	// Parse packages
	if packages, ok := filtersArg["packages"].([]interface{}); ok {
		filters.Packages = make([]string, 0, len(packages))
		for _, pkg := range packages {
			if s, ok := pkg.(string); ok {
				filters.Packages = append(filters.Packages, s)
			}
		}
	}

	// Parse min_relevance
	if minRel, ok := filtersArg["min_relevance"].(float64); ok {
		if minRel < 0.0 || minRel > 1.0 {
			return nil, fmt.Errorf("min_relevance must be between 0.0 and 1.0, got %f", minRel)
		}
		filters.MinRelevance = minRel
	}

	return filters, nil
}

// isValidSymbolType checks if a symbol type is valid
func isValidSymbolType(st string) bool {
	validTypes := map[string]bool{
		"function":  true,
		"method":    true,
		"struct":    true,
		"interface": true,
		"type":      true,
		"const":     true,
		"var":       true,
	}
	return validTypes[st]
}

// isValidDDDPattern checks if a DDD pattern is valid
func isValidDDDPattern(pattern string) bool {
	validPatterns := map[string]bool{
		"aggregate":    true,
		"entity":       true,
		"value_object": true,
		"repository":   true,
		"service":      true,
		"command":      true,
		"query":        true,
		"handler":      true,
	}
	return validPatterns[pattern]
}

// sanitizeQueryForFTS sanitizes a query string for SQL FTS to prevent injection
// This removes special FTS operators and characters that could cause issues
func sanitizeQueryForFTS(query string) string {
	// Remove characters that have special meaning in SQLite FTS5
	// Keep alphanumeric, spaces, and basic punctuation
	re := regexp.MustCompile(`[^\w\s\-_.]`)
	sanitized := re.ReplaceAllString(query, " ")

	// Collapse multiple spaces
	sanitized = regexp.MustCompile(`\s+`).ReplaceAllString(sanitized, " ")

	return strings.TrimSpace(sanitized)
}

// formatSearchResponse formats a searcher.SearchResponse into the MCP response format
func formatSearchResponse(query string, resp *searcher.SearchResponse) map[string]interface{} {
	results := make([]map[string]interface{}, len(resp.Results))

	for i, result := range resp.Results {
		resultMap := map[string]interface{}{
			"chunk_id":        result.ChunkID,
			"rank":            result.Rank,
			"relevance_score": result.RelevanceScore,
			"file": map[string]interface{}{
				"path":       result.File.Path,
				"package":    result.File.Package,
				"start_line": result.File.StartLine,
				"end_line":   result.File.EndLine,
			},
			"content": result.Content,
			"context": result.Context,
		}

		// Include symbol if present
		if result.Symbol != nil {
			resultMap["symbol"] = map[string]interface{}{
				"name":        result.Symbol.Name,
				"kind":        result.Symbol.Kind,
				"package":     result.Symbol.Package,
				"signature":   result.Symbol.Signature,
				"doc_comment": result.Symbol.DocComment,
			}
		}

		results[i] = resultMap
	}

	return map[string]interface{}{
		"results": results,
		"statistics": map[string]interface{}{
			"total_results":      resp.TotalResults,
			"returned_results":   len(resp.Results),
			"search_duration_ms": resp.Duration.Milliseconds(),
			"cache_hit":          resp.CacheHit,
		},
	}
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
