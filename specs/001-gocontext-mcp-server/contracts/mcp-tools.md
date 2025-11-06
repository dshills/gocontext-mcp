# MCP Tools Contract: GoContext MCP Server

**Date**: 2025-11-06
**Feature**: GoContext MCP Server
**Phase**: Phase 1 - API Contract Definition
**Protocol**: Model Context Protocol (MCP) via stdio transport

## Overview

GoContext MCP Server exposes three tools via the MCP protocol for AI coding assistants (Claude Code, Codex CLI) to interact with Go codebase indexing and search functionality.

**Transport**: stdio (standard input/output)
**Format**: JSON-RPC 2.0
**Authentication**: None (local tool, trusted environment)

---

## Tool 1: index_codebase

**Purpose**: Index or re-index a Go codebase to make it searchable

**Method**: `tools/call`
**Tool Name**: `index_codebase`

### Request Schema

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "tools/call",
  "params": {
    "name": "index_codebase",
    "arguments": {
      "path": "/absolute/path/to/go/project",
      "force_reindex": false,
      "include_tests": true,
      "include_vendor": false
    }
  }
}
```

### Parameters

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `path` | string | Yes | - | Absolute path to Go project root (must contain go.mod or .go files) |
| `force_reindex` | boolean | No | false | If true, re-index all files ignoring file hashes (full rebuild) |
| `include_tests` | boolean | No | true | If true, index *_test.go files |
| `include_vendor` | boolean | No | false | If true, index vendor/ directory |

### Response Schema (Success)

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "success": true,
    "project_id": 42,
    "statistics": {
      "files_indexed": 1247,
      "files_skipped": 89,
      "files_failed": 2,
      "symbols_extracted": 8432,
      "chunks_created": 7891,
      "embeddings_generated": 7891,
      "duration_seconds": 127.3,
      "index_size_mb": 68.5
    },
    "errors": [
      {
        "file": "internal/broken/parser.go",
        "error": "syntax error: unexpected '}', expecting expression"
      }
    ],
    "message": "Successfully indexed 1247 files in 127.3 seconds"
  }
}
```

### Response Schema (Error)

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "error": {
    "code": -32602,
    "message": "Invalid params",
    "data": {
      "param": "path",
      "reason": "Path does not exist or is not accessible"
    }
  }
}
```

### Error Codes

| Code | Message | Description |
|------|---------|-------------|
| -32602 | Invalid params | Missing required parameter or invalid value |
| -32603 | Internal error | Database error, filesystem error, or unexpected failure |
| -32001 | Project not found | Specified path does not contain a Go project |
| -32002 | Indexing in progress | Another indexing operation is already running for this project |

### Behavior

**Incremental Indexing** (default):
1. Check file hashes against previously indexed files
2. Skip unchanged files
3. Re-index only changed/new files
4. Update embeddings only for modified chunks

**Force Re-index** (`force_reindex: true`):
1. Delete all existing index data for project
2. Re-parse all files
3. Re-generate all chunks and embeddings
4. Rebuild all indexes

**Progress Indication**:
- Long-running operation (may take minutes)
- Consider streaming progress via separate MCP notification (future enhancement)
- Current implementation: blocks until complete

### Performance Targets

- 100k LOC: complete in <5 minutes
- 10 file changes: complete in <30 seconds (incremental)
- Memory usage: <500MB during indexing

---

## Tool 2: search_code

**Purpose**: Search indexed codebase with natural language or keyword queries

**Method**: `tools/call`
**Tool Name**: `search_code`

### Request Schema

```json
{
  "jsonrpc": "2.0",
  "id": 2,
  "method": "tools/call",
  "params": {
    "name": "search_code",
    "arguments": {
      "path": "/absolute/path/to/go/project",
      "query": "user authentication logic",
      "limit": 10,
      "filters": {
        "symbol_types": ["function", "method"],
        "file_pattern": "internal/auth/**",
        "ddd_patterns": ["repository", "service"]
      },
      "search_mode": "hybrid"
    }
  }
}
```

### Parameters

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `path` | string | Yes | - | Absolute path to indexed Go project |
| `query` | string | Yes | - | Search query (natural language or keywords) |
| `limit` | integer | No | 10 | Maximum number of results to return (1-100) |
| `filters` | object | No | {} | Optional filters to narrow search |
| `search_mode` | string | No | "hybrid" | Search strategy: "hybrid", "vector", or "keyword" |

### Filters Object

| Filter | Type | Values | Description |
|--------|------|--------|-------------|
| `symbol_types` | array | ["function", "method", "struct", "interface", "type"] | Limit to specific symbol kinds |
| `file_pattern` | string | Glob pattern | Limit to files matching pattern (e.g., "internal/**") |
| `ddd_patterns` | array | ["aggregate", "entity", "value_object", "repository", "service"] | Limit to DDD pattern types |
| `packages` | array | Package names | Limit to specific packages |
| `min_relevance` | float | 0.0-1.0 | Minimum relevance score threshold |

### Response Schema (Success)

```json
{
  "jsonrpc": "2.0",
  "id": 2,
  "result": {
    "success": true,
    "query": "user authentication logic",
    "results": [
      {
        "chunk_id": 1523,
        "rank": 1,
        "relevance_score": 0.92,
        "symbol": {
          "name": "AuthenticateUser",
          "kind": "function",
          "package": "internal/auth",
          "signature": "func AuthenticateUser(username, password string) (*User, error)",
          "doc_comment": "AuthenticateUser verifies user credentials and returns authenticated user."
        },
        "file": {
          "path": "internal/auth/service.go",
          "package": "auth",
          "start_line": 45,
          "end_line": 72
        },
        "content": "// AuthenticateUser verifies user credentials and returns authenticated user.\nfunc AuthenticateUser(username, password string) (*User, error) {\n\tif username == \"\" || password == \"\" {\n\t\treturn nil, errors.New(\"username and password required\")\n\t}\n\t// ... implementation ...\n}",
        "context_before": "package auth\n\nimport (\n\t\"errors\"\n\t\"internal/models\"\n)",
        "context_after": "// ValidateToken checks if authentication token is valid\nfunc ValidateToken(token string) bool { ... }"
      }
    ],
    "statistics": {
      "total_results": 47,
      "returned_results": 10,
      "search_duration_ms": 234,
      "cache_hit": false
    },
    "message": "Found 47 results in 234ms"
  }
}
```

### Response Schema (Error)

```json
{
  "jsonrpc": "2.0",
  "id": 2,
  "error": {
    "code": -32603,
    "message": "Internal error",
    "data": {
      "reason": "Project not indexed. Run index_codebase first."
    }
  }
}
```

### Error Codes

| Code | Message | Description |
|------|---------|-------------|
| -32602 | Invalid params | Invalid query, limit out of range, or malformed filters |
| -32603 | Internal error | Database error or search engine failure |
| -32003 | Project not indexed | No index found for specified path |
| -32004 | Empty query | Query parameter is empty or whitespace-only |

### Search Modes

**hybrid** (default, recommended):
- Combines vector similarity search + BM25 keyword search
- Best for most queries (semantic + exact matching)
- Uses Reciprocal Rank Fusion to merge results

**vector**:
- Pure semantic search using embeddings
- Best for conceptual queries ("error handling patterns")
- Requires embedding model available

**keyword**:
- BM25 full-text search only
- Best for exact symbol name queries ("func ParseFile")
- Faster, no embedding required

### Performance Targets

- p95 latency: <500ms
- p99 latency: <1000ms
- Concurrent queries supported (read-only operations)

---

## Tool 3: get_status

**Purpose**: Query indexing status and statistics for a Go project

**Method**: `tools/call`
**Tool Name**: `get_status`

### Request Schema

```json
{
  "jsonrpc": "2.0",
  "id": 3,
  "method": "tools/call",
  "params": {
    "name": "get_status",
    "arguments": {
      "path": "/absolute/path/to/go/project"
    }
  }
}
```

### Parameters

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `path` | string | Yes | - | Absolute path to Go project |

### Response Schema (Indexed Project)

```json
{
  "jsonrpc": "2.0",
  "id": 3,
  "result": {
    "success": true,
    "indexed": true,
    "project": {
      "id": 42,
      "root_path": "/Users/dev/myproject",
      "module_name": "github.com/dev/myproject",
      "go_version": "1.21",
      "index_version": "1.0.0"
    },
    "statistics": {
      "total_files": 1247,
      "total_symbols": 8432,
      "total_chunks": 7891,
      "total_embeddings": 7891,
      "index_size_mb": 68.5,
      "last_indexed_at": "2025-11-06T10:30:45Z",
      "indexing_duration_seconds": 127.3
    },
    "health": {
      "database_accessible": true,
      "embeddings_available": true,
      "fts_indexes_built": true
    },
    "message": "Project indexed successfully with 1247 files"
  }
}
```

### Response Schema (Not Indexed)

```json
{
  "jsonrpc": "2.0",
  "id": 3,
  "result": {
    "success": true,
    "indexed": false,
    "project": null,
    "statistics": null,
    "health": null,
    "message": "Project not indexed. Run index_codebase to create index."
  }
}
```

### Error Codes

| Code | Message | Description |
|------|---------|-------------|
| -32602 | Invalid params | Path parameter missing or invalid |
| -32603 | Internal error | Database error or filesystem error |

### Use Cases

- Check if project needs indexing
- Verify index health after indexing
- Display statistics to user
- Troubleshoot search issues (missing embeddings, corrupted index)

---

## MCP Protocol Details

### Server Information

Returned in response to `initialize` method:

```json
{
  "protocolVersion": "2024-11-05",
  "capabilities": {
    "tools": {}
  },
  "serverInfo": {
    "name": "gocontext-mcp",
    "version": "1.0.0"
  }
}
```

### Tool Listing

Returned in response to `tools/list` method:

```json
{
  "tools": [
    {
      "name": "index_codebase",
      "description": "Index a Go codebase to make it searchable",
      "inputSchema": {
        "type": "object",
        "properties": {
          "path": {
            "type": "string",
            "description": "Absolute path to Go project root"
          },
          "force_reindex": {
            "type": "boolean",
            "description": "Force full re-index ignoring file hashes"
          },
          "include_tests": {
            "type": "boolean",
            "description": "Include *_test.go files in index"
          },
          "include_vendor": {
            "type": "boolean",
            "description": "Include vendor/ directory in index"
          }
        },
        "required": ["path"]
      }
    },
    {
      "name": "search_code",
      "description": "Search indexed Go codebase with natural language or keyword queries",
      "inputSchema": {
        "type": "object",
        "properties": {
          "path": {
            "type": "string",
            "description": "Absolute path to indexed Go project"
          },
          "query": {
            "type": "string",
            "description": "Search query (natural language or keywords)"
          },
          "limit": {
            "type": "integer",
            "description": "Maximum number of results (1-100)",
            "minimum": 1,
            "maximum": 100
          },
          "filters": {
            "type": "object",
            "description": "Optional filters to narrow search"
          },
          "search_mode": {
            "type": "string",
            "enum": ["hybrid", "vector", "keyword"],
            "description": "Search strategy to use"
          }
        },
        "required": ["path", "query"]
      }
    },
    {
      "name": "get_status",
      "description": "Query indexing status and statistics for a Go project",
      "inputSchema": {
        "type": "object",
        "properties": {
          "path": {
            "type": "string",
            "description": "Absolute path to Go project"
          }
        },
        "required": ["path"]
      }
    }
  ]
}
```

---

## Security Considerations

### Local Tool Trust Model

- No authentication required (local trusted environment)
- File system access limited to paths provided in requests
- No network access except optional embedding API calls
- SQLite database stored in user's home directory

### Input Validation

**Path validation**:
- Must be absolute path
- Must exist on filesystem
- Must be readable by current user
- Sanitize against path traversal attacks

**Query validation**:
- Limit query length (max 1000 chars)
- Sanitize special characters for SQL FTS queries
- Validate limit parameter (1-100 range)

**Filter validation**:
- Validate enum values (symbol_types, ddd_patterns)
- Validate glob patterns (file_pattern)
- Prevent SQL injection in filters

---

## Versioning

**Current Version**: 1.0.0

**Breaking Changes** (require major version bump):
- Removing a tool
- Changing required parameters
- Changing response structure (removing fields)

**Non-Breaking Changes** (minor version bump):
- Adding new tools
- Adding optional parameters
- Adding new response fields

**Compatibility**:
- MCP protocol version: 2024-11-05
- Clients should check `serverInfo.version` for compatibility

---

## Testing

### Contract Tests

Verify each tool with:
1. Valid request → expected response
2. Missing required params → error -32602
3. Invalid param values → error -32602
4. Project not found → appropriate error code
5. Concurrent requests → no data corruption

### Integration Tests

End-to-end flows:
1. Index project → verify statistics
2. Search indexed project → verify results
3. Re-index with changes → verify incremental update
4. Search non-indexed project → error -32003

---

**Status**: Phase 1 MCP Tools Contract Complete ✅
