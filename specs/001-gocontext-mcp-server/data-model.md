# Data Model: GoContext MCP Server

**Date**: 2025-11-06
**Feature**: GoContext MCP Server
**Phase**: Phase 1 - Data Model Design

## Overview

This document defines the core data entities for the GoContext MCP Server. The data model supports:
- Semantic code search with vector embeddings
- Incremental indexing with file hash tracking
- Symbol-aware search with type information
- Domain-driven design pattern detection

## Storage Layer

**Primary Storage**: SQLite database with sqlite-vec extension
**Location**: `~/.gocontext/indices/<project-hash>.db`
**Concurrent Access**: WAL mode (1 writer, N readers)

---

## Core Entities

### 1. Project

**Purpose**: Represents an indexed Go codebase

**Schema**:
```sql
CREATE TABLE projects (
    id INTEGER PRIMARY KEY,
    root_path TEXT NOT NULL UNIQUE,
    module_name TEXT,           -- From go.mod
    go_version TEXT,            -- Min Go version
    total_files INTEGER,
    total_chunks INTEGER,
    index_version TEXT,         -- Schema version for migrations
    last_indexed_at TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_projects_root_path ON projects(root_path);
```

**Validation Rules**:
- `root_path` must be absolute path
- `module_name` extracted from go.mod if present
- `go_version` parsed from go.mod or detected from source
- `index_version` tracks schema for breaking changes

**Relationships**:
- One project has many files (1:N)
- One project has many chunks (1:N through files)

---

### 2. File

**Purpose**: Tracks indexed Go source files with content hashing for incremental updates

**Schema**:
```sql
CREATE TABLE files (
    id INTEGER PRIMARY KEY,
    project_id INTEGER NOT NULL,
    file_path TEXT NOT NULL,    -- Relative to project root
    package_name TEXT,
    content_hash BLOB NOT NULL,  -- SHA-256 (32 bytes)
    mod_time TIMESTAMP,
    size_bytes INTEGER,
    parse_error TEXT,            -- NULL if parsed successfully
    last_indexed_at TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE,
    UNIQUE(project_id, file_path)
);

CREATE INDEX idx_files_project ON files(project_id);
CREATE INDEX idx_files_hash ON files(content_hash);
CREATE INDEX idx_files_package ON files(package_name);
CREATE INDEX idx_files_mod_time ON files(mod_time);
```

**Validation Rules**:
- `file_path` relative to project root, normalized (forward slashes)
- `content_hash` must be SHA-256 (32 bytes)
- `mod_time` from filesystem metadata
- `parse_error` populated on AST parse failures (file still tracked)

**Relationships**:
- One file belongs to one project (N:1)
- One file has many chunks (1:N)
- One file has many symbols (1:N)

---

### 3. Symbol

**Purpose**: Represents code symbols (functions, methods, types, interfaces) extracted via AST parsing

**Schema**:
```sql
CREATE TABLE symbols (
    id INTEGER PRIMARY KEY,
    file_id INTEGER NOT NULL,
    name TEXT NOT NULL,
    kind TEXT NOT NULL,          -- function, method, struct, interface, type, const, var
    package_name TEXT NOT NULL,
    signature TEXT,              -- Function signature or type definition
    doc_comment TEXT,
    scope TEXT,                  -- exported, unexported, package_local
    receiver TEXT,               -- For methods: receiver type name
    start_line INTEGER,
    start_col INTEGER,
    end_line INTEGER,
    end_col INTEGER,
    -- DDD pattern detection
    is_aggregate_root BOOLEAN DEFAULT 0,
    is_entity BOOLEAN DEFAULT 0,
    is_value_object BOOLEAN DEFAULT 0,
    is_repository BOOLEAN DEFAULT 0,
    is_service BOOLEAN DEFAULT 0,
    is_command BOOLEAN DEFAULT 0,
    is_query BOOLEAN DEFAULT 0,
    is_handler BOOLEAN DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (file_id) REFERENCES files(id) ON DELETE CASCADE
);

CREATE INDEX idx_symbols_file ON symbols(file_id);
CREATE INDEX idx_symbols_name ON symbols(name);
CREATE INDEX idx_symbols_kind ON symbols(kind);
CREATE INDEX idx_symbols_package ON symbols(package_name);
CREATE INDEX idx_symbols_ddd ON symbols(is_aggregate_root, is_entity, is_value_object);
CREATE INDEX idx_symbols_cqrs ON symbols(is_command, is_query, is_handler);

-- Full-text search on symbol names and signatures
CREATE VIRTUAL TABLE symbols_fts USING fts5(
    name, signature, doc_comment,
    content='symbols',
    content_rowid='id'
);
```

**Validation Rules**:
- `kind` must be one of: function, method, struct, interface, type, const, var, field
- `scope` must be one of: exported, unexported, package_local
- `receiver` only populated for methods
- `start_line/col` and `end_line/col` define symbol location
- DDD flags detected by naming conventions (e.g., "Repository" suffix)

**Relationships**:
- One symbol belongs to one file (N:1)
- One symbol may have one chunk (1:1 optional)

---

### 4. Chunk

**Purpose**: Semantically meaningful code sections for embedding and search

**Schema**:
```sql
CREATE TABLE chunks (
    id INTEGER PRIMARY KEY,
    file_id INTEGER NOT NULL,
    symbol_id INTEGER,           -- NULL for package-level chunks
    content TEXT NOT NULL,
    content_hash BLOB NOT NULL,  -- SHA-256 for deduplication
    token_count INTEGER,
    start_line INTEGER NOT NULL,
    end_line INTEGER NOT NULL,
    context_before TEXT,         -- Package, imports, related declarations
    context_after TEXT,          -- Related functions, types
    chunk_type TEXT NOT NULL,    -- function, type, method, package, const_group
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (file_id) REFERENCES files(id) ON DELETE CASCADE,
    FOREIGN KEY (symbol_id) REFERENCES symbols(id) ON DELETE CASCADE
);

CREATE INDEX idx_chunks_file ON chunks(file_id);
CREATE INDEX idx_chunks_symbol ON chunks(symbol_id);
CREATE INDEX idx_chunks_hash ON chunks(content_hash);
CREATE INDEX idx_chunks_type ON chunks(chunk_type);

-- Full-text search on chunk content
CREATE VIRTUAL TABLE chunks_fts USING fts5(
    content, context_before, context_after,
    content='chunks',
    content_rowid='id'
);
```

**Validation Rules**:
- `content` must not be empty
- `content_hash` must be SHA-256 (deduplication across files)
- `token_count` approximate (chars/4 heuristic or tiktoken)
- `chunk_type` must be: function, type, method, package, const_group, var_group
- `context_before/after` provide surrounding code for better search results

**Relationships**:
- One chunk belongs to one file (N:1)
- One chunk may represent one symbol (N:1 optional)
- One chunk has one embedding (1:1)

---

### 5. Embedding

**Purpose**: Vector embeddings for semantic search

**Schema**:
```sql
CREATE TABLE embeddings (
    id INTEGER PRIMARY KEY,
    chunk_id INTEGER NOT NULL UNIQUE,
    vector BLOB NOT NULL,        -- float32 array, dimension from provider
    dimension INTEGER NOT NULL,
    provider TEXT NOT NULL,      -- jina, openai, local
    model TEXT NOT NULL,         -- Model version for re-embedding
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (chunk_id) REFERENCES chunks(id) ON DELETE CASCADE
);

CREATE INDEX idx_embeddings_chunk ON embeddings(chunk_id);
CREATE INDEX idx_embeddings_provider ON embeddings(provider, model);

-- Vector similarity search (sqlite-vec extension)
-- Virtual table created at runtime based on embedding dimension
```

**Vector Storage**:
- Stored as BLOB (array of float32 values)
- Dimension varies by provider:
  - Jina AI v3: 1024
  - OpenAI text-embedding-3-small: 1536
  - Local models: 384-768
- Binary format: little-endian float32 array

**Validation Rules**:
- `vector` length must equal `dimension * sizeof(float32)`
- `provider` must be: jina, openai, local
- `model` tracks version for re-embedding when models update
- One embedding per chunk (enforce unique constraint)

**Relationships**:
- One embedding belongs to one chunk (1:1)

---

### 6. Import

**Purpose**: Track import relationships between Go packages

**Schema**:
```sql
CREATE TABLE imports (
    id INTEGER PRIMARY KEY,
    file_id INTEGER NOT NULL,
    import_path TEXT NOT NULL,
    alias TEXT,                  -- Import alias (if any)
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (file_id) REFERENCES files(id) ON DELETE CASCADE
);

CREATE INDEX idx_imports_file ON imports(file_id);
CREATE INDEX idx_imports_path ON imports(import_path);
```

**Validation Rules**:
- `import_path` follows Go import path format
- `alias` populated if import has explicit alias (`import alias "path"`)

**Relationships**:
- One import belongs to one file (N:1)
- Imports enable dependency graph analysis

---

### 7. SearchQuery (Cache)

**Purpose**: Cache search results for performance

**Schema**:
```sql
CREATE TABLE search_queries (
    id INTEGER PRIMARY KEY,
    query_text TEXT NOT NULL,
    query_hash BLOB NOT NULL UNIQUE,  -- SHA-256(query_text + filters)
    result_chunk_ids TEXT NOT NULL,   -- JSON array of chunk IDs
    result_count INTEGER NOT NULL,
    search_duration_ms INTEGER,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMP,             -- TTL for cache invalidation
    hit_count INTEGER DEFAULT 0
);

CREATE INDEX idx_search_hash ON search_queries(query_hash);
CREATE INDEX idx_search_expires ON search_queries(expires_at);
```

**Validation Rules**:
- `query_hash` includes query text and filters for uniqueness
- `result_chunk_ids` stored as JSON array for fast deserialization
- `expires_at` typically set to 1 hour after creation
- Cache invalidated when underlying chunks change

**Cache Strategy**:
- Write-through: populate on first query
- TTL-based expiration: 1 hour default
- LRU eviction: track hit_count, prune least-used entries
- Invalidate on re-indexing affected files

---

## Relationships Diagram

```
Project (1) ──────┬────────→ (N) File
                  │
                  └────────→ (N) Chunk (via File)

File (1) ─────────┬────────→ (N) Symbol
                  │
                  ├────────→ (N) Chunk
                  │
                  └────────→ (N) Import

Symbol (1) ───────────────→ (1) Chunk (optional)

Chunk (1) ────────────────→ (1) Embedding

SearchQuery (N) ───────────→ (N) Chunk (via result_chunk_ids JSON)
```

---

## Data Flow

### Indexing Flow

1. **Project Discovery**
   - Insert/update project record
   - Walk filesystem for .go files

2. **File Processing**
   - Check file hash against existing record
   - Skip if unchanged (incremental indexing)
   - Parse AST if changed or new

3. **Symbol Extraction**
   - Parse file with go/parser
   - Extract symbols via go/ast traversal
   - Detect DDD patterns by naming conventions
   - Insert symbols into database

4. **Chunking**
   - Create chunks for each symbol (function, type, method)
   - Include context (imports, package declaration)
   - Hash chunk content for deduplication
   - Insert chunks into database

5. **Embedding Generation**
   - Batch chunks for embedding API call
   - Generate vector embeddings
   - Store embeddings with provider metadata
   - Build FTS indexes for keyword search

### Search Flow

1. **Query Processing**
   - Check cache by query_hash
   - If cached and not expired, return cached results

2. **Parallel Search**
   - Vector search: embed query, compute cosine similarity
   - BM25 search: query chunks_fts full-text index
   - Run both searches concurrently

3. **Result Fusion**
   - Apply Reciprocal Rank Fusion (RRF)
   - Combine and re-rank results
   - Fetch chunk content and metadata

4. **Cache Update**
   - Store results in search_queries table
   - Set expires_at timestamp

---

## Migrations

**Version**: 1.0.0 (initial schema)

**Migration Strategy**:
- Store `index_version` in projects table
- Check version on server start
- Run SQL migrations if version mismatch
- Preserve existing data where possible

**Breaking Changes**:
- Embedding dimension change → re-embed all chunks
- Schema changes → migrate with SQL ALTER TABLE
- Provider removal → re-embed with new provider

---

## Performance Considerations

### Indexes

All foreign keys have indexes for join performance:
- `idx_files_project`: Fast file lookup by project
- `idx_chunks_file`: Fast chunk lookup by file
- `idx_embeddings_chunk`: Fast embedding lookup

Search-critical indexes:
- `idx_symbols_name`: Fast symbol name search
- `idx_chunks_hash`: Deduplication during chunking
- FTS5 indexes: Full-text search on content

### Query Patterns

**Hot paths** (optimize first):
1. Vector similarity search (most frequent, latency-sensitive)
2. BM25 keyword search (frequent, must be <100ms)
3. Symbol lookup by name (developer queries)
4. File hash check (every indexing run)

**Cold paths** (optimize later):
- Import graph analysis
- DDD pattern queries
- Statistics aggregation

### Storage Estimates

For 100k LOC codebase:
- Files: ~1,000 files × 200 bytes = 0.2 MB
- Symbols: ~10,000 symbols × 500 bytes = 5 MB
- Chunks: ~8,000 chunks × 2 KB = 16 MB
- Embeddings: 8,000 × (1024 dim × 4 bytes) = 32 MB
- FTS indexes: ~10 MB
- **Total**: ~65 MB per 100k LOC

---

## Data Integrity

### Constraints

- Foreign keys with CASCADE DELETE (cleanup on file removal)
- UNIQUE constraints on (project_id, file_path) prevent duplicates
- NOT NULL on critical fields (content_hash, vectors)

### Transactions

- File batch indexing in single transaction (atomic updates)
- Rollback on any error during indexing
- WAL mode enables concurrent reads during writes

### Backup Strategy

- SQLite database is single file → easy backup
- Backup before re-indexing (destructive operation)
- Export command: copy database file
- Import command: replace database file

---

## Evolution and Versioning

### Schema Version

Current: `1.0.0`

Track in projects table:
```sql
UPDATE projects SET index_version = '1.0.0' WHERE id = ?;
```

### Future Considerations

Potential additions (not in v1.0):
- `types` table: Detailed type information beyond symbols
- `comments` table: Inline comments for context search
- `test_coverage` table: Link symbols to test coverage
- `call_graph` table: Function call relationships
- `complexity_metrics` table: Cyclomatic complexity per function

---

**Status**: Phase 1 Data Model Complete ✅
