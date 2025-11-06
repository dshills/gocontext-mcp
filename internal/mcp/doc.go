// Package mcp implements the Model Context Protocol (MCP) server for GoContext.
//
// The MCP server exposes three tools to AI coding assistants (Claude Code, Codex CLI):
//   - index_codebase: Index a Go project for semantic search
//   - search_code: Search indexed code with natural language queries
//   - get_status: Check indexing status and statistics
//
// # Protocol Overview
//
// MCP is a JSON-RPC 2.0 protocol over stdio transport:
//
//	Client → Server: {"method": "tools/call", "params": {...}}
//	Server → Client: {"result": {...}}
//
// The server communicates with MCP clients via standard input/output,
// making it simple to integrate with any MCP-compatible client.
//
// # Basic Usage
//
// The MCP server is typically started via the serve command:
//
//	gocontext serve
//
// It then listens on stdin for MCP protocol messages and writes responses to stdout.
//
// # Tool: index_codebase
//
// Index a Go codebase to make it searchable:
//
//	Request:
//	{
//	  "name": "index_codebase",
//	  "arguments": {
//	    "path": "/path/to/project",
//	    "force_reindex": false,
//	    "include_tests": true,
//	    "include_vendor": false
//	  }
//	}
//
//	Response:
//	{
//	  "success": true,
//	  "statistics": {
//	    "files_indexed": 247,
//	    "files_skipped": 89,
//	    "symbols_extracted": 8432,
//	    "duration_seconds": 35.2
//	  }
//	}
//
// # Tool: search_code
//
// Search indexed code semantically or by keywords:
//
//	Request:
//	{
//	  "name": "search_code",
//	  "arguments": {
//	    "path": "/path/to/project",
//	    "query": "user authentication logic",
//	    "limit": 10,
//	    "search_mode": "hybrid",
//	    "filters": {
//	      "symbol_types": ["function", "method"],
//	      "packages": ["internal/auth"]
//	    }
//	  }
//	}
//
//	Response:
//	{
//	  "results": [
//	    {
//	      "rank": 1,
//	      "relevance_score": 0.92,
//	      "symbol": {
//	        "name": "AuthenticateUser",
//	        "kind": "function",
//	        "signature": "func(...) error"
//	      },
//	      "file": {
//	        "path": "internal/auth/service.go",
//	        "start_line": 45,
//	        "end_line": 72
//	      },
//	      "content": "func AuthenticateUser(...) { ... }"
//	    }
//	  ]
//	}
//
// # Tool: get_status
//
// Check indexing status:
//
//	Request:
//	{
//	  "name": "get_status",
//	  "arguments": {
//	    "path": "/path/to/project"
//	  }
//	}
//
//	Response:
//	{
//	  "indexed": true,
//	  "project": {
//	    "root_path": "/path/to/project",
//	    "module_name": "github.com/user/project",
//	    "total_files": 247
//	  },
//	  "health": {
//	    "database_accessible": true,
//	    "embeddings_available": true
//	  }
//	}
//
// # MCP Client Configuration
//
// Configure in Claude Code's MCP settings:
//
//	{
//	  "mcpServers": {
//	    "gocontext": {
//	      "command": "/usr/local/bin/gocontext",
//	      "args": ["serve"],
//	      "env": {
//	        "JINA_API_KEY": "your-api-key"
//	      }
//	    }
//	  }
//	}
//
// # Error Handling
//
// The MCP server returns standard JSON-RPC error responses:
//
//	{
//	  "error": {
//	    "code": -32602,
//	    "message": "Invalid params",
//	    "data": {
//	      "param": "path",
//	      "reason": "Path does not exist"
//	    }
//	  }
//	}
//
// Error codes:
//   - -32602: Invalid params (missing/invalid arguments)
//   - -32603: Internal error (database, filesystem, etc.)
//   - -32001: Project not found
//   - -32002: Indexing in progress
//   - -32003: Project not indexed
//
// # Implementation Details
//
// The MCP package uses github.com/mark3labs/mcp-go for protocol implementation:
//
//	server := mcp.NewServer()
//	server.AddTool("index_codebase", handleIndexCodebase)
//	server.AddTool("search_code", handleSearchCode)
//	server.AddTool("get_status", handleGetStatus)
//	server.Run()
//
// Tool handlers receive structured requests and return structured responses:
//
//	func handleIndexCodebase(req IndexRequest) (IndexResponse, error) {
//	    // Validate request
//	    if req.Path == "" {
//	        return nil, ErrInvalidParams
//	    }
//
//	    // Call indexer
//	    stats, err := indexer.IndexProject(ctx, req)
//	    if err != nil {
//	        return nil, err
//	    }
//
//	    // Return response
//	    return IndexResponse{Success: true, Statistics: stats}, nil
//	}
//
// # Logging
//
// The MCP server logs to stderr (stdout is reserved for MCP protocol):
//
//	log.SetOutput(os.Stderr)
//	log.Printf("MCP server started")
//
// Set log level via environment:
//
//	GOCONTEXT_LOG_LEVEL=debug gocontext serve
//
// # Testing
//
// Test MCP tools with mock clients:
//
//	client := mcp.NewTestClient(server)
//
//	resp, err := client.Call("index_codebase", map[string]interface{}{
//	    "path": "/test/project",
//	})
//
//	assert.NoError(t, err)
//	assert.True(t, resp.Success)
package mcp
