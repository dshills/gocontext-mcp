package mcp

import (
	"github.com/mark3labs/mcp-go/mcp"
)

// indexCodebaseTool returns the tool definition for index_codebase
func indexCodebaseTool() mcp.Tool {
	return mcp.Tool{
		Name:        "index_codebase",
		Description: "Index a Go codebase to make it searchable",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]interface{}{
				"path": map[string]interface{}{
					"type":        "string",
					"description": "Absolute path to Go project root (must contain go.mod or .go files)",
				},
				"force_reindex": map[string]interface{}{
					"type":        "boolean",
					"description": "If true, re-index all files ignoring file hashes (full rebuild)",
					"default":     false,
				},
				"include_tests": map[string]interface{}{
					"type":        "boolean",
					"description": "If true, index *_test.go files",
					"default":     true,
				},
				"include_vendor": map[string]interface{}{
					"type":        "boolean",
					"description": "If true, index vendor/ directory",
					"default":     false,
				},
			},
			Required: []string{"path"},
		},
	}
}

// searchCodeTool returns the tool definition for search_code
func searchCodeTool() mcp.Tool {
	return mcp.Tool{
		Name:        "search_code",
		Description: "Search indexed Go codebase with natural language or keyword queries",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]interface{}{
				"path": map[string]interface{}{
					"type":        "string",
					"description": "Absolute path to indexed Go project",
				},
				"query": map[string]interface{}{
					"type":        "string",
					"description": "Search query (natural language or keywords)",
				},
				"limit": map[string]interface{}{
					"type":        "integer",
					"description": "Maximum number of results to return (1-100)",
					"default":     10,
					"minimum":     1,
					"maximum":     100,
				},
				"filters": map[string]interface{}{
					"type":        "object",
					"description": "Optional filters to narrow search",
					"properties": map[string]interface{}{
						"symbol_types": map[string]interface{}{
							"type":        "array",
							"description": "Filter by symbol kind (function, method, struct, interface, type)",
							"items": map[string]interface{}{
								"type": "string",
								"enum": []string{"function", "method", "struct", "interface", "type", "const", "var"},
							},
						},
						"file_pattern": map[string]interface{}{
							"type":        "string",
							"description": "Glob pattern for file paths (e.g., 'internal/**')",
						},
						"ddd_patterns": map[string]interface{}{
							"type":        "array",
							"description": "Filter by DDD pattern types",
							"items": map[string]interface{}{
								"type": "string",
								"enum": []string{"aggregate", "entity", "value_object", "repository", "service", "command", "query", "handler"},
							},
						},
						"packages": map[string]interface{}{
							"type":        "array",
							"description": "Filter by package names",
							"items": map[string]interface{}{
								"type": "string",
							},
						},
						"min_relevance": map[string]interface{}{
							"type":        "number",
							"description": "Minimum relevance score threshold (0.0-1.0)",
							"minimum":     0.0,
							"maximum":     1.0,
						},
					},
				},
				"search_mode": map[string]interface{}{
					"type":        "string",
					"description": "Search strategy: hybrid (vector + keyword), vector (semantic only), or keyword (BM25 only)",
					"enum":        []string{"hybrid", "vector", "keyword"},
					"default":     "hybrid",
				},
			},
			Required: []string{"path", "query"},
		},
	}
}

// getStatusTool returns the tool definition for get_status
func getStatusTool() mcp.Tool {
	return mcp.Tool{
		Name:        "get_status",
		Description: "Query indexing status and statistics for a Go project",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]interface{}{
				"path": map[string]interface{}{
					"type":        "string",
					"description": "Absolute path to Go project",
				},
			},
			Required: []string{"path"},
		},
	}
}
