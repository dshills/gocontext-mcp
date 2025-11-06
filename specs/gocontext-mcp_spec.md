# GoContext MCP Server - Technical Specification

**Version:** 1.0.0
**Status:** Design Phase
**Target Completion:** 5 weeks from kickoff
**Author:** Technical Specification
**Last Updated:** 2025-11-06

---

## Executive Summary

GoContext is a specialized Model Context Protocol (MCP) server that provides symbol-aware semantic search for Go codebases. Unlike general-purpose tools, GoContext leverages Go's native AST parsing capabilities to understand code structure, types, and domain relationships. It is designed for large-scale Go projects, particularly those using domain-driven design principles.

### Key Differentiators
- **Go-native AST parsing** using `go/parser`, `go/ast`, `go/types`
- **Domain-aware search** optimized for DDD patterns (aggregates, entities, value objects)
- **Local-first architecture** suitable for compliance-sensitive environments (HIPAA, FDA 21 CFR Part 11)
- **Concurrent indexing** leveraging Go's goroutines for fast processing
- **Single binary deployment** with no external runtime dependencies

---

## Table of Contents

1. [Project Goals & Success Criteria](#project-goals--success-criteria)
2. [Architecture Overview](#architecture-overview)
3. [Component Specifications](#component-specifications)
4. [Data Models](#data-models)
5. [API Definitions](#api-definitions)
6. [Implementation Phases](#implementation-phases)
7. [Testing Strategy](#testing-strategy)
8. [Deployment & Operations](#deployment--operations)
9. [Security & Compliance](#security--compliance)
10. [Open Questions & Decisions](#open-questions--decisions)

---

## Project Goals & Success Criteria

### Primary Goals (Working Backwards)

#### End State (Week 5)
A production-ready Go binary that:
- Indexes a 100k+ LOC Go codebase in < 5 minutes
- Returns semantically relevant code chunks in < 500ms
- Integrates seamlessly with Claude Code and Codex CLI
- Supports incremental re-indexing (only changed files)
- Provides domain-aware search queries

#### Success Criteria
1. **Performance**
   - Initial indexing: < 5 minutes for 100k LOC
   - Search latency: p95 < 500ms
   - Re-indexing: < 30 seconds for 10 file changes
   - Memory usage: < 500MB for 100k LOC codebase

2. **Accuracy**
   - Search recall: > 90% for symbol-based queries
   - Precision: > 80% for semantic queries
   - Zero false negatives for exact symbol matches

3. **Usability**
   - Single command installation
   - Zero configuration for basic use
   - Works offline after initial embedding generation
   - Clear error messages and progress indication

4. **Compatibility**
   - Claude Code integration via MCP
   - Codex CLI integration via MCP
   - Support for Go modules (go.mod)
   - Support for Go 1.21+ codebases

### Non-Goals (Scope Boundaries)

- Multi-language support (TypeScript, Python, etc.)
- Cloud-hosted SaaS offering
- Real-time collaboration features
- IDE plugin development
- Code modification or refactoring tools

---

## Architecture Overview

### System Context Diagram

```
┌─────────────────────────────────────────────────────────────┐
│                      MCP Clients                             │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐         │
│  │ Claude Code │  │  Codex CLI  │  │   Custom    │         │
│  └──────┬──────┘  └──────┬──────┘  └──────┬──────┘         │
│         │                 │                 │                │
│         └─────────────────┴─────────────────┘                │
│                           │                                  │
│                           │ MCP Protocol (stdio)             │
└───────────────────────────┼──────────────────────────────────┘
                            │
┌───────────────────────────┼──────────────────────────────────┐
│                    GoContext Server                          │
│                           │                                  │
│  ┌────────────────────────▼───────────────────────────────┐ │
│  │            MCP Handler (mcp-go SDK)                    │ │
│  │  ┌──────────────────────────────────────────────────┐ │ │
│  │  │ Tools: index_codebase, search_code, get_status   │ │ │
│  │  └──────────────────────────────────────────────────┘ │ │
│  └────────────────┬───────────────────────────────────────┘ │
│                   │                                          │
│  ┌────────────────▼───────────────────────────────────────┐ │
│  │               Core Engine                              │ │
│  │  ┌──────────┐  ┌──────────┐  ┌──────────┐           │ │
│  │  │  Parser  │  │ Indexer  │  │ Searcher │           │ │
│  │  │  (AST)   │  │ (Worker  │  │ (Hybrid) │           │ │
│  │  │          │  │  Pool)   │  │          │           │ │
│  │  └──────────┘  └──────────┘  └──────────┘           │ │
│  └────────────────┬───────────────────────────────────────┘ │
│                   │                                          │
│  ┌────────────────▼───────────────────────────────────────┐ │
│  │            Storage Layer                               │ │
│  │  ┌──────────────────┐  ┌──────────────────┐          │ │
│  │  │ SQLite + Vector  │  │  File Hash Cache │          │ │
│  │  │    Extension     │  │   (in-memory)    │          │ │
│  │  └──────────────────┘  └──────────────────┘          │ │
│  └────────────────────────────────────────────────────────┘ │
│                                                              │
│  ┌──────────────────────────────────────────────────────┐   │
│  │         External Services (Optional)                 │   │
│  │  ┌─────────────┐  ┌─────────────┐                  │   │
│  │  │ Jina AI API │  │  OpenAI API │  (Embeddings)    │   │
│  │  └─────────────┘  └─────────────┘                  │   │
│  └──────────────────────────────────────────────────────┘   │
└──────────────────────────────────────────────────────────────┘
```

### High-Level Data Flow

#### Indexing Flow
```
Go Source Files
    │
    ├─> Parser: go/parser + go/ast
    │     ├─> Extract symbols (funcs, methods, structs, interfaces)
    │     ├─> Extract types and signatures
    │     ├─> Extract doc comments
    │     └─> Identify scope (exported vs unexported)
    │
    ├─> Chunker: Semantic boundaries
    │     ├─> Create chunks at function/type boundaries
    │     ├─> Include surrounding context
    │     ├─> Preserve import relationships
    │     └─> Hash content for incremental updates
    │
    ├─> Embedder: Generate vector embeddings
    │     ├─> API call to Jina/OpenAI (or local model)
    │     ├─> Batch processing for efficiency
    │     └─> Cache results
    │
    └─> Storage: Persist to SQLite
          ├─> Insert/update chunks
          ├─> Store embeddings in vector extension
          ├─> Update file hash registry
          └─> Build text search indexes
```

#### Search Flow
```
User Query (natural language or keyword)
    │
    ├─> Query Embedder: Convert to vector
    │
    ├─> Hybrid Search:
    │     ├─> Vector Search (cosine similarity)
    │     ├─> BM25 Text Search (keyword matching)
    │     └─> Fusion: Combine results with weights
    │
    ├─> Reranker (Optional):
    │     └─> Use Jina reranker or local scoring
    │
    └─> Results: Top-K chunks with context
          ├─> Chunk content
          ├─> File path and line numbers
          ├─> Symbol information
          ├─> Relevance score
          └─> Surrounding context
```

### Technology Stack

#### Core Dependencies
```go
// Standard library
go/parser      // Parse Go source files
go/ast         // AST traversal and analysis
go/token       // Source position tracking
go/types       // Type information and checking
go/doc         // Documentation extraction
crypto/sha256  // File hashing for incremental updates
database/sql   // SQLite interaction

// External dependencies
github.com/mark3labs/mcp-go              // MCP protocol implementation
github.com/mattn/go-sqlite3              // SQLite driver with custom build
modernc.org/sqlite                       // Pure Go SQLite (alternative)
github.com/pgvector/pgvector-go          // Vector similarity (if using Postgres)

// Optional dependencies
github.com/nlpodyssey/spago/embeddings   // Local embeddings (alternative to API)
golang.org/x/mod/modfile                 // Go module parsing
golang.org/x/sync/errgroup               // Concurrent processing
```

#### Build Configuration
```bash
# SQLite with vector extension
CGO_ENABLED=1 go build -tags "sqlite_vec"

# Pure Go build (no CGO)
CGO_ENABLED=0 go build -tags "purego"
```

---

## Component Specifications

### 1. Parser Component

**Purpose:** Extract semantic information from Go source files using native AST parsing.

**Input:** Go source file paths
**Output:** Structured symbol information

#### Interface
```go
package parser

import (
    "go/ast"
    "go/token"
    "go/types"
)

// Parser extracts semantic information from Go source files
type Parser interface {
    // ParseFile parses a single Go file
    ParseFile(path string) (*ParseResult, error)

    // ParsePackage parses all files in a package
    ParsePackage(pkgPath string) (*PackageInfo, error)

    // ParseModule parses an entire Go module
    ParseModule(moduleRoot string) (*ModuleInfo, error)
}

// ParseResult contains extracted information from a single file
type ParseResult struct {
    FilePath    string
    Package     string
    Imports     []Import
    Symbols     []Symbol
    Types       []TypeInfo
    Comments    map[string]string // symbol name -> doc comment
    FileHash    [32]byte
    ModTime     time.Time
}

// Symbol represents a code symbol (function, method, type, etc.)
type Symbol struct {
    Name        string
    Kind        SymbolKind
    Package     string
    File        string
    StartPos    token.Position
    EndPos      token.Position
    Signature   string
    DocComment  string
    Scope       ScopeKind
    Receiver    string        // For methods: receiver type
    TypeParams  []string      // For generic types/functions
    Attributes  SymbolAttrs   // Domain-specific attributes
}

type SymbolKind int

const (
    SymbolFunction SymbolKind = iota
    SymbolMethod
    SymbolStruct
    SymbolInterface
    SymbolType
    SymbolConst
    SymbolVar
    SymbolField
)

type ScopeKind int

const (
    ScopeExported ScopeKind = iota
    ScopeUnexported
    ScopePackageLocal
)

// SymbolAttrs contains domain-specific metadata
type SymbolAttrs struct {
    IsAggregateRoot bool   // DDD: Aggregate root
    IsEntity        bool   // DDD: Entity
    IsValueObject   bool   // DDD: Value object
    IsRepository    bool   // DDD: Repository
    IsService       bool   // DDD: Domain service
    IsCommand       bool   // CQRS: Command
    IsQuery         bool   // CQRS: Query
    IsHandler       bool   // CQRS: Handler
    Tags            []string
}

// TypeInfo represents type information
type TypeInfo struct {
    Name       string
    Kind       TypeKind
    Underlying string
    Methods    []string
    Fields     []Field
    Implements []string // Interface names
}

type TypeKind int

const (
    TypeStruct TypeKind = iota
    TypeInterface
    TypeAlias
    TypePrimitive
    TypeSlice
    TypeMap
    TypeChannel
    TypePointer
    TypeFunction
)

// Import represents an import statement
type Import struct {
    Path  string
    Alias string
}

// PackageInfo contains information about a Go package
type PackageInfo struct {
    Path        string
    Name        string
    Files       []*ParseResult
    Symbols     []Symbol
    Types       []TypeInfo
    Imports     []Import
    Dependencies []string
}

// ModuleInfo contains information about a Go module
type ModuleInfo struct {
    Path        string
    Version     string
    Packages    []*PackageInfo
    GoVersion   string
    Requires    []Dependency
}

type Dependency struct {
    Path    string
    Version string
}
```

#### Implementation Details

**Symbol Detection Logic:**
```go
// Detect domain-driven design patterns
func detectDDDPatterns(symbol Symbol) SymbolAttrs {
    attrs := SymbolAttrs{}

    // Aggregate root detection
    if hasMethod(symbol, "AggregateRoot") {
        attrs.IsAggregateRoot = true
    }

    // Entity detection (has ID field and methods)
    if symbol.Kind == SymbolStruct {
        if hasField(symbol, "ID") && hasMethod(symbol, "Entity") {
            attrs.IsEntity = true
        }
    }

    // Value object detection (no ID, immutable)
    if symbol.Kind == SymbolStruct {
        if !hasField(symbol, "ID") && isImmutable(symbol) {
            attrs.IsValueObject = true
        }
    }

    // Repository detection
    if strings.HasSuffix(symbol.Name, "Repository") {
        attrs.IsRepository = true
    }

    // Service detection
    if strings.HasSuffix(symbol.Name, "Service") {
        attrs.IsService = true
    }

    // CQRS pattern detection
    if strings.HasSuffix(symbol.Name, "Command") {
        attrs.IsCommand = true
    }
    if strings.HasSuffix(symbol.Name, "Query") {
        attrs.IsQuery = true
    }
    if strings.HasSuffix(symbol.Name, "Handler") {
        attrs.IsHandler = true
    }

    return attrs
}
```

**AST Traversal:**
```go
// Visit implements ast.Visitor
func (p *parser) Visit(node ast.Node) ast.Visitor {
    switch n := node.(type) {
    case *ast.FuncDecl:
        p.extractFunction(n)
    case *ast.GenDecl:
        p.extractGenDecl(n)
    case *ast.TypeSpec:
        p.extractType(n)
    case *ast.ImportSpec:
        p.extractImport(n)
    }
    return p
}

func (p *parser) extractFunction(fn *ast.FuncDecl) {
    symbol := Symbol{
        Name:       fn.Name.Name,
        Package:    p.currentPackage,
        File:       p.currentFile,
        StartPos:   p.fset.Position(fn.Pos()),
        EndPos:     p.fset.Position(fn.End()),
        DocComment: fn.Doc.Text(),
    }

    // Determine if method or function
    if fn.Recv != nil {
        symbol.Kind = SymbolMethod
        symbol.Receiver = p.extractReceiver(fn.Recv)
    } else {
        symbol.Kind = SymbolFunction
    }

    // Extract signature
    symbol.Signature = p.extractSignature(fn)

    // Determine scope
    if token.IsExported(fn.Name.Name) {
        symbol.Scope = ScopeExported
    } else {
        symbol.Scope = ScopeUnexported
    }

    // Detect domain patterns
    symbol.Attributes = detectDDDPatterns(symbol)

    p.symbols = append(p.symbols, symbol)
}
```

#### Error Handling
- Invalid Go syntax: Return detailed parse errors with line/column
- Missing imports: Continue parsing, mark as incomplete
- Type resolution failures: Log warning, continue with partial info
- File access errors: Return error, don't crash entire indexing

#### Performance Targets
- Parse 100 files in < 1 second
- Memory: < 100MB for 100k LOC
- Concurrent parsing: Up to runtime.NumCPU() goroutines

---

### 2. Chunker Component

**Purpose:** Divide Go source code into semantically meaningful chunks for embedding and search.

**Input:** ParseResult from parser
**Output:** Chunks with embeddings

#### Interface
```go
package chunker

import (
    "gocontext/parser"
)

// Chunker creates semantic chunks from parsed code
type Chunker interface {
    // CreateChunks generates chunks from parsed results
    CreateChunks(result *parser.ParseResult, opts ChunkOptions) ([]Chunk, error)

    // CreateChunksForPackage generates chunks for entire package
    CreateChunksForPackage(pkg *parser.PackageInfo, opts ChunkOptions) ([]Chunk, error)
}

// Chunk represents a semantic unit of code
type Chunk struct {
    ID          string            // Unique identifier
    Content     string            // Actual code content
    File        string            // Source file path
    Package     string            // Package name
    StartLine   int               // Starting line number
    EndLine     int               // Ending line number
    StartCol    int               // Starting column
    EndCol      int               // Ending column
    Symbols     []string          // Symbol names in this chunk
    SymbolKind  parser.SymbolKind // Primary symbol kind
    Context     ChunkContext      // Surrounding context
    Metadata    ChunkMetadata     // Additional metadata
    Hash        [32]byte          // Content hash
}

// ChunkContext provides surrounding code context
type ChunkContext struct {
    Before      string   // Lines before chunk (for context)
    After       string   // Lines after chunk (for context)
    Imports     []string // Relevant imports
    PackageDoc  string   // Package-level documentation
    TypeContext string   // Parent type (for methods/fields)
}

// ChunkMetadata contains searchable metadata
type ChunkMetadata struct {
    Signature      string
    DocComment     string
    Scope          parser.ScopeKind
    Tags           []string
    Complexity     int      // Cyclomatic complexity
    LOC            int      // Lines of code
    Interfaces     []string // Implemented interfaces
    Dependencies   []string // Called functions/types
    CalledBy       []string // Functions that call this
    DomainConcepts []string // Extracted domain terms
}

// ChunkOptions configures chunking behavior
type ChunkOptions struct {
    MaxChunkSize    int  // Max lines per chunk
    ContextLines    int  // Lines of context before/after
    IncludeTests    bool // Include test functions
    IncludeInternal bool // Include internal packages
    SplitLargeFuncs bool // Split functions > MaxChunkSize
    GroupRelated    bool // Group related symbols
}
```

#### Chunking Strategies

**Strategy 1: Function-Level Chunking (Default)**
```go
// Each function/method becomes a chunk
// Includes:
// - Function signature
// - Function body
// - Doc comment
// - N lines of context before/after

func chunkByFunction(result *parser.ParseResult, opts ChunkOptions) []Chunk {
    var chunks []Chunk

    for _, symbol := range result.Symbols {
        if symbol.Kind == parser.SymbolFunction ||
           symbol.Kind == parser.SymbolMethod {

            chunk := Chunk{
                ID:         generateChunkID(symbol),
                Content:    extractContent(result.FilePath, symbol),
                File:       symbol.File,
                Package:    symbol.Package,
                StartLine:  symbol.StartPos.Line,
                EndLine:    symbol.EndPos.Line,
                Symbols:    []string{symbol.Name},
                SymbolKind: symbol.Kind,
            }

            // Add context
            chunk.Context = extractContext(result, symbol, opts.ContextLines)

            // Add metadata
            chunk.Metadata = extractMetadata(result, symbol)

            chunks = append(chunks, chunk)
        }
    }

    return chunks
}
```

**Strategy 2: Type-Level Chunking**
```go
// Group struct definition with its methods
// Useful for understanding types holistically

func chunkByType(result *parser.ParseResult, opts ChunkOptions) []Chunk {
    typeGroups := groupByReceiver(result.Symbols)

    var chunks []Chunk
    for typeName, symbols := range typeGroups {
        chunk := Chunk{
            ID:         generateTypeChunkID(typeName),
            Content:    combineSymbols(symbols),
            Symbols:    extractNames(symbols),
            SymbolKind: parser.SymbolStruct,
        }

        chunks = append(chunks, chunk)
    }

    return chunks
}
```

**Strategy 3: Domain Concept Chunking**
```go
// Group related domain concepts (aggregate + entities + value objects)
// Optimized for DDD codebases

func chunkByDomainConcept(result *parser.ParseResult, opts ChunkOptions) []Chunk {
    aggregates := findAggregates(result.Symbols)

    var chunks []Chunk
    for _, agg := range aggregates {
        // Find related entities and value objects
        related := findRelatedTypes(agg, result.Symbols)

        chunk := Chunk{
            ID:      generateAggregateChunkID(agg),
            Content: combineAggregate(agg, related),
            Symbols: append([]string{agg.Name}, extractNames(related)...),
            Metadata: ChunkMetadata{
                DomainConcepts: extractDomainTerms(agg, related),
            },
        }

        chunks = append(chunks, chunk)
    }

    return chunks
}
```

#### Content Extraction

**Code Extraction:**
```go
func extractContent(filePath string, symbol parser.Symbol) string {
    // Read file
    content, err := os.ReadFile(filePath)
    if err != nil {
        return ""
    }

    lines := strings.Split(string(content), "\n")

    // Extract relevant lines
    startLine := max(0, symbol.StartPos.Line-1)
    endLine := min(len(lines), symbol.EndPos.Line)

    return strings.Join(lines[startLine:endLine], "\n")
}
```

**Context Extraction:**
```go
func extractContext(result *parser.ParseResult, symbol parser.Symbol, contextLines int) ChunkContext {
    content, _ := os.ReadFile(result.FilePath)
    lines := strings.Split(string(content), "\n")

    // Before context
    beforeStart := max(0, symbol.StartPos.Line-contextLines-1)
    beforeEnd := max(0, symbol.StartPos.Line-1)
    before := strings.Join(lines[beforeStart:beforeEnd], "\n")

    // After context
    afterStart := min(len(lines), symbol.EndPos.Line)
    afterEnd := min(len(lines), symbol.EndPos.Line+contextLines)
    after := strings.Join(lines[afterStart:afterEnd], "\n")

    // Extract relevant imports
    imports := extractRelevantImports(result.Imports, symbol)

    // Get type context for methods
    typeContext := ""
    if symbol.Kind == parser.SymbolMethod {
        typeContext = findTypeDefinition(result, symbol.Receiver)
    }

    return ChunkContext{
        Before:      before,
        After:       after,
        Imports:     imports,
        TypeContext: typeContext,
    }
}
```

**Metadata Extraction:**
```go
func extractMetadata(result *parser.ParseResult, symbol parser.Symbol) ChunkMetadata {
    return ChunkMetadata{
        Signature:      symbol.Signature,
        DocComment:     symbol.DocComment,
        Scope:          symbol.Scope,
        Tags:           symbol.Attributes.Tags,
        Complexity:     calculateComplexity(result, symbol),
        LOC:            symbol.EndPos.Line - symbol.StartPos.Line + 1,
        Interfaces:     findImplementedInterfaces(result, symbol),
        Dependencies:   findDependencies(result, symbol),
        CalledBy:       findCallers(result, symbol),
        DomainConcepts: extractDomainTerms(symbol),
    }
}
```

#### Chunk ID Generation
```go
func generateChunkID(symbol parser.Symbol) string {
    // Format: <package>:<file>:<symbol>:<hash>
    // Example: github.com/user/pkg:file.go:FunctionName:a1b2c3d4

    h := sha256.New()
    h.Write([]byte(symbol.Package))
    h.Write([]byte(symbol.File))
    h.Write([]byte(symbol.Name))
    h.Write([]byte(symbol.Signature))

    hash := hex.EncodeToString(h.Sum(nil))[:8]

    return fmt.Sprintf("%s:%s:%s:%s",
        symbol.Package,
        filepath.Base(symbol.File),
        symbol.Name,
        hash,
    )
}
```

#### Performance Targets
- Process 1000 chunks in < 2 seconds
- Memory: < 200MB for 10k chunks
- Chunk size: 10-500 lines (configurable)

---

### 3. Embedder Component

**Purpose:** Generate vector embeddings for code chunks to enable semantic search.

**Input:** Chunks
**Output:** Chunks with embeddings

#### Interface
```go
package embedder

import (
    "gocontext/chunker"
)

// Embedder generates vector embeddings for code chunks
type Embedder interface {
    // Embed generates embeddings for a single chunk
    Embed(chunk *chunker.Chunk) ([]float32, error)

    // EmbedBatch generates embeddings for multiple chunks
    EmbedBatch(chunks []*chunker.Chunk) ([][]float32, error)

    // EmbedQuery generates embedding for a search query
    EmbedQuery(query string) ([]float32, error)
}

// EmbedderConfig configures the embedder
type EmbedderConfig struct {
    Provider     EmbedProvider
    APIKey       string
    Model        string
    Dimensions   int
    BatchSize    int
    Timeout      time.Duration
    RetryPolicy  RetryPolicy
    CacheEnabled bool
    CachePath    string
}

type EmbedProvider int

const (
    ProviderJina EmbedProvider = iota
    ProviderOpenAI
    ProviderLocal
    ProviderCohere
    ProviderVoyage
)

// RetryPolicy defines retry behavior
type RetryPolicy struct {
    MaxRetries     int
    InitialBackoff time.Duration
    MaxBackoff     time.Duration
    Multiplier     float64
}
```

#### Implementation: Jina AI (Primary)
```go
// Jina AI embeddings API
// Model: jina-embeddings-v3
// Dimensions: 1024
// Context: 8192 tokens

type jinaEmbedder struct {
    apiKey    string
    model     string
    client    *http.Client
    cache     *embedCache
}

func (e *jinaEmbedder) EmbedBatch(chunks []*chunker.Chunk) ([][]float32, error) {
    // Prepare texts
    texts := make([]string, len(chunks))
    for i, chunk := range chunks {
        texts[i] = formatChunkForEmbedding(chunk)
    }

    // Check cache
    uncachedIndices := []int{}
    results := make([][]float32, len(chunks))

    for i, text := range texts {
        if cached, ok := e.cache.Get(text); ok {
            results[i] = cached
        } else {
            uncachedIndices = append(uncachedIndices, i)
        }
    }

    // API call for uncached
    if len(uncachedIndices) > 0 {
        req := JinaEmbedRequest{
            Model: e.model,
            Input: collectUncached(texts, uncachedIndices),
        }

        resp, err := e.callJinaAPI(req)
        if err != nil {
            return nil, err
        }

        // Populate results and cache
        for i, idx := range uncachedIndices {
            results[idx] = resp.Data[i].Embedding
            e.cache.Set(texts[idx], resp.Data[i].Embedding)
        }
    }

    return results, nil
}

func formatChunkForEmbedding(chunk *chunker.Chunk) string {
    // Format: <doc><signature><content>
    // Prioritize documentation and signatures for better semantic matching

    var parts []string

    if chunk.Metadata.DocComment != "" {
        parts = append(parts, chunk.Metadata.DocComment)
    }

    if chunk.Metadata.Signature != "" {
        parts = append(parts, chunk.Metadata.Signature)
    }

    parts = append(parts, chunk.Content)

    // Add domain concepts for better semantic matching
    if len(chunk.Metadata.DomainConcepts) > 0 {
        concepts := strings.Join(chunk.Metadata.DomainConcepts, ", ")
        parts = append(parts, fmt.Sprintf("Domain: %s", concepts))
    }

    return strings.Join(parts, "\n\n")
}

type JinaEmbedRequest struct {
    Model string   `json:"model"`
    Input []string `json:"input"`
}

type JinaEmbedResponse struct {
    Data []struct {
        Embedding []float32 `json:"embedding"`
        Index     int       `json:"index"`
    } `json:"data"`
}
```

#### Implementation: OpenAI (Alternative)
```go
// OpenAI embeddings API
// Model: text-embedding-3-large
// Dimensions: 3072 (can be reduced to 1536, 768, or 256)
// Context: 8191 tokens

type openaiEmbedder struct {
    apiKey    string
    model     string
    dims      int
    client    *http.Client
    cache     *embedCache
}

func (e *openaiEmbedder) EmbedBatch(chunks []*chunker.Chunk) ([][]float32, error) {
    // Similar structure to Jina implementation
    // Use OpenAI API format

    req := OpenAIEmbedRequest{
        Model:      e.model,
        Input:      prepareTexts(chunks),
        Dimensions: e.dims,
    }

    // Make API call with retry logic
    resp, err := e.callOpenAIAPI(req)
    if err != nil {
        return nil, err
    }

    return extractEmbeddings(resp), nil
}
```

#### Implementation: Local (Future)
```go
// Local embeddings using sentence-transformers compatible models
// Model: all-MiniLM-L6-v2 or BGE-small-en
// Dimensions: 384 or 512
// Pros: Fast, private, no API costs
// Cons: Lower quality than API models

type localEmbedder struct {
    model     *transformers.Model
    tokenizer *transformers.Tokenizer
    cache     *embedCache
}

func (e *localEmbedder) EmbedBatch(chunks []*chunker.Chunk) ([][]float32, error) {
    // TODO: Implement using Go ML libraries
    // Options:
    // 1. Use ONNX runtime with exported models
    // 2. Call Python microservice
    // 3. Use pure Go implementation (limited models)

    return nil, errors.New("local embeddings not yet implemented")
}
```

#### Caching Strategy
```go
type embedCache struct {
    cache map[string][]float32
    mu    sync.RWMutex
    path  string
}

func (c *embedCache) Get(text string) ([]float32, bool) {
    c.mu.RLock()
    defer c.mu.RUnlock()

    key := hashText(text)
    embedding, ok := c.cache[key]
    return embedding, ok
}

func (c *embedCache) Set(text string, embedding []float32) {
    c.mu.Lock()
    defer c.mu.Unlock()

    key := hashText(text)
    c.cache[key] = embedding

    // Persist to disk periodically
    if len(c.cache)%100 == 0 {
        c.persist()
    }
}

func hashText(text string) string {
    h := sha256.Sum256([]byte(text))
    return hex.EncodeToString(h[:])
}
```

#### Error Handling & Retry Logic
```go
func (e *jinaEmbedder) callJinaAPI(req JinaEmbedRequest) (*JinaEmbedResponse, error) {
    policy := e.retryPolicy
    backoff := policy.InitialBackoff

    for attempt := 0; attempt <= policy.MaxRetries; attempt++ {
        resp, err := e.doAPICall(req)
        if err == nil {
            return resp, nil
        }

        // Check if retryable
        if !isRetryable(err) {
            return nil, err
        }

        // Exponential backoff
        if attempt < policy.MaxRetries {
            time.Sleep(backoff)
            backoff = time.Duration(float64(backoff) * policy.Multiplier)
            if backoff > policy.MaxBackoff {
                backoff = policy.MaxBackoff
            }
        }
    }

    return nil, fmt.Errorf("max retries exceeded")
}

func isRetryable(err error) bool {
    // Retry on rate limits, timeouts, server errors
    // Don't retry on auth errors, invalid input

    if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
        return true
    }

    // Check HTTP status codes
    // 429 (rate limit), 500, 502, 503, 504 -> retry
    // 400, 401, 403 -> don't retry

    return false
}
```

#### Performance Targets
- Batch size: 32-128 chunks per API call
- Throughput: 500 chunks/minute (API limited)
- Cache hit rate: > 80% for re-indexing
- Latency: < 2s per batch (API dependent)

---

### 4. Indexer Component

**Purpose:** Coordinate parsing, chunking, and embedding; manage incremental updates.

**Input:** Codebase path
**Output:** Indexed data in storage

#### Interface
```go
package indexer

import (
    "gocontext/parser"
    "gocontext/chunker"
    "gocontext/embedder"
    "gocontext/storage"
)

// Indexer orchestrates the indexing pipeline
type Indexer interface {
    // Index indexes a codebase
    Index(ctx context.Context, path string, opts IndexOptions) (*IndexResult, error)

    // Reindex performs incremental re-indexing
    Reindex(ctx context.Context, path string) (*IndexResult, error)

    // GetStatus returns current indexing status
    GetStatus(ctx context.Context) (*IndexStatus, error)

    // Cancel cancels ongoing indexing operation
    Cancel(ctx context.Context) error
}

// IndexOptions configures indexing behavior
type IndexOptions struct {
    // Parsing options
    IncludeTests     bool
    IncludeInternal  bool
    IncludeVendor    bool
    MaxFileSize      int64

    // Chunking options
    ChunkStrategy    chunker.ChunkStrategy
    MaxChunkSize     int
    ContextLines     int

    // Embedding options
    EmbedProvider    embedder.EmbedProvider
    EmbedModel       string
    BatchSize        int

    // Processing options
    Concurrency      int
    ProgressCallback func(IndexProgress)
}

// IndexResult contains indexing results
type IndexResult struct {
    StartTime       time.Time
    EndTime         time.Time
    Duration        time.Duration

    FilesProcessed  int
    FilesSkipped    int
    FilesErrored    int

    ChunksCreated   int
    ChunksUpdated   int
    ChunksDeleted   int

    BytesProcessed  int64
    EmbeddingsCached int
    EmbeddingsNew   int

    Errors          []IndexError
}

// IndexStatus represents current indexing status
type IndexStatus struct {
    State           IndexState
    Progress        float64 // 0.0 to 1.0
    CurrentFile     string
    FilesProcessed  int
    TotalFiles      int
    ChunksProcessed int
    StartTime       time.Time
    EstimatedRemaining time.Duration
}

type IndexState int

const (
    StateIdle IndexState = iota
    StateScanning
    StateParsing
    StateChunking
    StateEmbedding
    StateStoring
    StateCompleted
    StateError
    StateCancelled
)

// IndexProgress for callbacks
type IndexProgress struct {
    State       IndexState
    Phase       string
    File        string
    Processed   int
    Total       int
    Percentage  float64
    Message     string
}

// IndexError represents an indexing error
type IndexError struct {
    File    string
    Phase   string
    Error   error
    Context map[string]any
}
```

#### Implementation
```go
type indexer struct {
    parser   parser.Parser
    chunker  chunker.Chunker
    embedder embedder.Embedder
    storage  storage.Storage

    // State management
    mu         sync.RWMutex
    status     IndexStatus
    ctx        context.Context
    cancel     context.CancelFunc

    // File tracking
    fileHashes map[string][32]byte
    fileCache  *fileHashCache

    // Worker pool
    workers    int
    semaphore  chan struct{}
}

func New(
    p parser.Parser,
    c chunker.Chunker,
    e embedder.Embedder,
    s storage.Storage,
) Indexer {
    return &indexer{
        parser:   p,
        chunker:  c,
        embedder: e,
        storage:  s,
        fileHashes: make(map[string][32]byte),
    }
}

func (idx *indexer) Index(ctx context.Context, path string, opts IndexOptions) (*IndexResult, error) {
    // Initialize
    idx.mu.Lock()
    idx.ctx, idx.cancel = context.WithCancel(ctx)
    idx.status = IndexStatus{
        State:     StateScanning,
        StartTime: time.Now(),
    }
    idx.workers = opts.Concurrency
    if idx.workers == 0 {
        idx.workers = runtime.NumCPU()
    }
    idx.semaphore = make(chan struct{}, idx.workers)
    idx.mu.Unlock()

    result := &IndexResult{
        StartTime: time.Now(),
    }

    // Phase 1: Scan filesystem
    files, err := idx.scanFiles(path, opts)
    if err != nil {
        return nil, fmt.Errorf("scan failed: %w", err)
    }

    idx.updateStatus(StateScanning, len(files), 0)

    // Phase 2: Process files concurrently
    fileChan := make(chan string, len(files))
    for _, f := range files {
        fileChan <- f
    }
    close(fileChan)

    // Worker pool
    var wg sync.WaitGroup
    errChan := make(chan IndexError, len(files))

    for w := 0; w < idx.workers; w++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            idx.worker(fileChan, errChan, opts, result)
        }()
    }

    // Progress tracking
    go idx.trackProgress(opts.ProgressCallback, len(files))

    wg.Wait()
    close(errChan)

    // Collect errors
    for err := range errChan {
        result.Errors = append(result.Errors, err)
        result.FilesErrored++
    }

    result.EndTime = time.Now()
    result.Duration = result.EndTime.Sub(result.StartTime)

    idx.updateStatus(StateCompleted, 0, 0)

    return result, nil
}

func (idx *indexer) worker(
    fileChan <-chan string,
    errChan chan<- IndexError,
    opts IndexOptions,
    result *IndexResult,
) {
    for file := range fileChan {
        select {
        case <-idx.ctx.Done():
            return
        default:
        }

        // Acquire semaphore
        idx.semaphore <- struct{}{}

        // Process file
        err := idx.processFile(file, opts, result)
        if err != nil {
            errChan <- IndexError{
                File:  file,
                Phase: "process",
                Error: err,
            }
        }

        // Release semaphore
        <-idx.semaphore

        // Update progress
        idx.incrementProgress()
    }
}

func (idx *indexer) processFile(
    file string,
    opts IndexOptions,
    result *IndexResult,
) error {
    // Check if file changed
    hash, err := hashFile(file)
    if err != nil {
        return fmt.Errorf("hash file: %w", err)
    }

    if oldHash, ok := idx.fileHashes[file]; ok && oldHash == hash {
        atomic.AddInt32(&result.FilesSkipped, 1)
        return nil // File unchanged
    }

    // Parse
    idx.updateStatus(StateParsing, 0, 0)
    parseResult, err := idx.parser.ParseFile(file)
    if err != nil {
        return fmt.Errorf("parse: %w", err)
    }

    // Chunk
    idx.updateStatus(StateChunking, 0, 0)
    chunks, err := idx.chunker.CreateChunks(parseResult, chunker.ChunkOptions{
        MaxChunkSize: opts.MaxChunkSize,
        ContextLines: opts.ContextLines,
    })
    if err != nil {
        return fmt.Errorf("chunk: %w", err)
    }

    // Embed
    idx.updateStatus(StateEmbedding, 0, 0)
    embeddings, err := idx.embedder.EmbedBatch(chunks)
    if err != nil {
        return fmt.Errorf("embed: %w", err)
    }

    // Attach embeddings to chunks
    for i := range chunks {
        chunks[i].Embedding = embeddings[i]
    }

    // Store
    idx.updateStatus(StateStoring, 0, 0)
    err = idx.storage.UpsertChunks(idx.ctx, chunks)
    if err != nil {
        return fmt.Errorf("store: %w", err)
    }

    // Update file hash
    idx.fileHashes[file] = hash

    atomic.AddInt32(&result.FilesProcessed, 1)
    atomic.AddInt32(&result.ChunksCreated, int32(len(chunks)))
    atomic.AddInt64(&result.BytesProcessed, getFileSize(file))

    return nil
}
```

#### File Scanning
```go
func (idx *indexer) scanFiles(path string, opts IndexOptions) ([]string, error) {
    var files []string

    err := filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
        if err != nil {
            return err
        }

        // Skip directories
        if d.IsDir() {
            // Skip vendor if not included
            if !opts.IncludeVendor && d.Name() == "vendor" {
                return filepath.SkipDir
            }
            // Skip hidden directories
            if strings.HasPrefix(d.Name(), ".") {
                return filepath.SkipDir
            }
            return nil
        }

        // Only .go files
        if !strings.HasSuffix(p, ".go") {
            return nil
        }

        // Skip test files if not included
        if !opts.IncludeTests && strings.HasSuffix(p, "_test.go") {
            return nil
        }

        // Check file size
        info, err := d.Info()
        if err != nil {
            return err
        }
        if info.Size() > opts.MaxFileSize {
            return nil // Skip large files
        }

        files = append(files, p)
        return nil
    })

    return files, err
}
```

#### Incremental Reindexing
```go
func (idx *indexer) Reindex(ctx context.Context, path string) (*IndexResult, error) {
    // Load previous file hashes from storage
    oldHashes, err := idx.storage.GetFileHashes(ctx)
    if err != nil {
        return nil, fmt.Errorf("load hashes: %w", err)
    }
    idx.fileHashes = oldHashes

    // Scan current files
    files, err := idx.scanFiles(path, IndexOptions{})
    if err != nil {
        return nil, err
    }

    // Compute current hashes
    newHashes := make(map[string][32]byte)
    for _, file := range files {
        hash, err := hashFile(file)
        if err != nil {
            continue
        }
        newHashes[file] = hash
    }

    // Find changed/new files
    var changedFiles []string
    for file, newHash := range newHashes {
        if oldHash, ok := oldHashes[file]; !ok || oldHash != newHash {
            changedFiles = append(changedFiles, file)
        }
    }

    // Find deleted files
    var deletedFiles []string
    for file := range oldHashes {
        if _, ok := newHashes[file]; !ok {
            deletedFiles = append(deletedFiles, file)
        }
    }

    // Delete chunks for deleted files
    for _, file := range deletedFiles {
        err := idx.storage.DeleteChunksByFile(ctx, file)
        if err != nil {
            log.Printf("failed to delete chunks for %s: %v", file, err)
        }
    }

    // Index changed files
    if len(changedFiles) == 0 {
        return &IndexResult{
            StartTime: time.Now(),
            EndTime:   time.Now(),
        }, nil
    }

    // Use same indexing logic for changed files
    return idx.Index(ctx, path, IndexOptions{
        // Filter to only changed files internally
    })
}
```

#### File Hashing
```go
func hashFile(path string) ([32]byte, error) {
    f, err := os.Open(path)
    if err != nil {
        return [32]byte{}, err
    }
    defer f.Close()

    h := sha256.New()
    if _, err := io.Copy(h, f); err != nil {
        return [32]byte{}, err
    }

    var hash [32]byte
    copy(hash[:], h.Sum(nil))
    return hash, nil
}
```

#### Progress Tracking
```go
func (idx *indexer) updateStatus(state IndexState, total, processed int) {
    idx.mu.Lock()
    defer idx.mu.Unlock()

    idx.status.State = state
    if total > 0 {
        idx.status.TotalFiles = total
    }
    if processed > 0 {
        idx.status.FilesProcessed = processed
    }

    if idx.status.TotalFiles > 0 {
        idx.status.Progress = float64(idx.status.FilesProcessed) / float64(idx.status.TotalFiles)
    }
}

func (idx *indexer) trackProgress(callback func(IndexProgress), total int) {
    ticker := time.NewTicker(500 * time.Millisecond)
    defer ticker.Stop()

    for {
        select {
        case <-idx.ctx.Done():
            return
        case <-ticker.C:
            idx.mu.RLock()
            status := idx.status
            idx.mu.RUnlock()

            if callback != nil {
                callback(IndexProgress{
                    State:      status.State,
                    Processed:  status.FilesProcessed,
                    Total:      status.TotalFiles,
                    Percentage: status.Progress * 100,
                })
            }
        }
    }
}
```

#### Performance Targets
- Indexing speed: 100k LOC in < 5 minutes
- Incremental update: 10 files in < 30 seconds
- Concurrency: Up to NumCPU workers
- Memory: < 500MB peak for 100k LOC

---

### 5. Storage Component

**Purpose:** Persist indexed data with vector search capabilities.

**Input:** Chunks with embeddings
**Output:** Search results

#### Interface
```go
package storage

import (
    "context"
    "gocontext/chunker"
)

// Storage handles persistence and retrieval
type Storage interface {
    // Write operations
    UpsertChunks(ctx context.Context, chunks []chunker.Chunk) error
    DeleteChunksByFile(ctx context.Context, file string) error
    DeleteAll(ctx context.Context) error

    // Read operations
    GetChunk(ctx context.Context, id string) (*chunker.Chunk, error)
    GetChunksByFile(ctx context.Context, file string) ([]chunker.Chunk, error)
    GetFileHashes(ctx context.Context) (map[string][32]byte, error)

    // Search operations
    VectorSearch(ctx context.Context, query []float32, opts SearchOptions) ([]SearchResult, error)
    TextSearch(ctx context.Context, query string, opts SearchOptions) ([]SearchResult, error)
    HybridSearch(ctx context.Context, query string, embedding []float32, opts SearchOptions) ([]SearchResult, error)

    // Metadata operations
    GetStats(ctx context.Context) (*StorageStats, error)
    Vacuum(ctx context.Context) error

    // Lifecycle
    Close() error
}

// SearchOptions configures search behavior
type SearchOptions struct {
    Limit          int
    Offset         int
    MinScore       float64
    Filters        SearchFilters
    IncludeContext bool
}

// SearchFilters allows filtering search results
type SearchFilters struct {
    Packages   []string
    Files      []string
    SymbolKind []parser.SymbolKind
    Scope      []parser.ScopeKind
    Tags       []string

    // DDD filters
    AggregatesOnly bool
    EntitiesOnly   bool
    RepositoriesOnly bool

    // CQRS filters
    CommandsOnly bool
    QueriesOnly  bool
}

// SearchResult represents a search result
type SearchResult struct {
    Chunk    chunker.Chunk
    Score    float64
    Distance float64 // For vector search
    Rank     int
    Method   SearchMethod
}

type SearchMethod int

const (
    SearchMethodVector SearchMethod = iota
    SearchMethodText
    SearchMethodHybrid
)

// StorageStats contains storage statistics
type StorageStats struct {
    TotalChunks    int
    TotalFiles     int
    TotalPackages  int
    DatabaseSize   int64
    IndexSize      int64
    LastIndexed    time.Time
    LastVacuum     time.Time
}
```

#### Implementation: SQLite with Vector Extension
```go
type sqliteStorage struct {
    db     *sql.DB
    dbPath string
    mu     sync.RWMutex
}

func NewSQLiteStorage(dbPath string) (Storage, error) {
    // Initialize SQLite with vector extension
    db, err := sql.Open("sqlite3", dbPath)
    if err != nil {
        return nil, err
    }

    // Enable foreign keys
    _, err = db.Exec("PRAGMA foreign_keys = ON")
    if err != nil {
        return nil, err
    }

    // Load vector extension
    _, err = db.Exec("SELECT load_extension('vector0')")
    if err != nil {
        return nil, fmt.Errorf("load vector extension: %w", err)
    }

    s := &sqliteStorage{
        db:     db,
        dbPath: dbPath,
    }

    // Create schema
    if err := s.createSchema(); err != nil {
        return nil, err
    }

    return s, nil
}
```

#### Database Schema
```sql
-- Files table: Track indexed files
CREATE TABLE IF NOT EXISTS files (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    path TEXT NOT NULL UNIQUE,
    package TEXT NOT NULL,
    hash BLOB NOT NULL,
    size INTEGER NOT NULL,
    mod_time INTEGER NOT NULL,
    indexed_at INTEGER NOT NULL
);

CREATE INDEX idx_files_package ON files(package);
CREATE INDEX idx_files_path ON files(path);

-- Chunks table: Code chunks with metadata
CREATE TABLE IF NOT EXISTS chunks (
    id TEXT PRIMARY KEY,
    file_id INTEGER NOT NULL,
    content TEXT NOT NULL,
    package TEXT NOT NULL,
    start_line INTEGER NOT NULL,
    end_line INTEGER NOT NULL,
    start_col INTEGER NOT NULL,
    end_col INTEGER NOT NULL,
    hash BLOB NOT NULL,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,

    FOREIGN KEY (file_id) REFERENCES files(id) ON DELETE CASCADE
);

CREATE INDEX idx_chunks_file ON chunks(file_id);
CREATE INDEX idx_chunks_package ON chunks(package);

-- Symbols table: Symbol information
CREATE TABLE IF NOT EXISTS symbols (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    chunk_id TEXT NOT NULL,
    name TEXT NOT NULL,
    kind INTEGER NOT NULL,
    signature TEXT,
    doc_comment TEXT,
    scope INTEGER NOT NULL,
    receiver TEXT,

    FOREIGN KEY (chunk_id) REFERENCES chunks(id) ON DELETE CASCADE
);

CREATE INDEX idx_symbols_chunk ON symbols(chunk_id);
CREATE INDEX idx_symbols_name ON symbols(name);
CREATE INDEX idx_symbols_kind ON symbols(kind);

-- Symbol attributes: DDD/CQRS patterns
CREATE TABLE IF NOT EXISTS symbol_attributes (
    symbol_id INTEGER PRIMARY KEY,
    is_aggregate_root BOOLEAN NOT NULL DEFAULT 0,
    is_entity BOOLEAN NOT NULL DEFAULT 0,
    is_value_object BOOLEAN NOT NULL DEFAULT 0,
    is_repository BOOLEAN NOT NULL DEFAULT 0,
    is_service BOOLEAN NOT NULL DEFAULT 0,
    is_command BOOLEAN NOT NULL DEFAULT 0,
    is_query BOOLEAN NOT NULL DEFAULT 0,
    is_handler BOOLEAN NOT NULL DEFAULT 0,

    FOREIGN KEY (symbol_id) REFERENCES symbols(id) ON DELETE CASCADE
);

-- Tags: For custom tagging
CREATE TABLE IF NOT EXISTS tags (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    symbol_id INTEGER NOT NULL,
    tag TEXT NOT NULL,

    FOREIGN KEY (symbol_id) REFERENCES symbols(id) ON DELETE CASCADE
);

CREATE INDEX idx_tags_symbol ON tags(symbol_id);
CREATE INDEX idx_tags_tag ON tags(tag);

-- Embeddings table: Vector embeddings
CREATE TABLE IF NOT EXISTS embeddings (
    chunk_id TEXT PRIMARY KEY,
    embedding BLOB NOT NULL,  -- Vector stored as BLOB
    dimensions INTEGER NOT NULL,
    model TEXT NOT NULL,
    created_at INTEGER NOT NULL,

    FOREIGN KEY (chunk_id) REFERENCES chunks(id) ON DELETE CASCADE
);

-- Vector index for similarity search
CREATE INDEX idx_embeddings_vector ON embeddings(embedding);

-- Full-text search index
CREATE VIRTUAL TABLE IF NOT EXISTS chunks_fts USING fts5(
    chunk_id,
    content,
    signature,
    doc_comment,
    package,
    symbol_names
);

-- Metadata table: Store indexing metadata
CREATE TABLE IF NOT EXISTS metadata (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at INTEGER NOT NULL
);
```

#### Upsert Operations
```go
func (s *sqliteStorage) UpsertChunks(ctx context.Context, chunks []chunker.Chunk) error {
    tx, err := s.db.BeginTx(ctx, nil)
    if err != nil {
        return err
    }
    defer tx.Rollback()

    for _, chunk := range chunks {
        // Insert/update file
        fileID, err := s.upsertFile(tx, chunk.File, chunk.Package, chunk.Hash)
        if err != nil {
            return fmt.Errorf("upsert file: %w", err)
        }

        // Insert/update chunk
        err = s.upsertChunk(tx, &chunk, fileID)
        if err != nil {
            return fmt.Errorf("upsert chunk: %w", err)
        }

        // Insert symbols
        for _, symbol := range chunk.Symbols {
            err = s.insertSymbol(tx, chunk.ID, symbol)
            if err != nil {
                return fmt.Errorf("insert symbol: %w", err)
            }
        }

        // Insert embedding
        if len(chunk.Embedding) > 0 {
            err = s.upsertEmbedding(tx, chunk.ID, chunk.Embedding)
            if err != nil {
                return fmt.Errorf("upsert embedding: %w", err)
            }
        }

        // Update FTS index
        err = s.updateFTS(tx, &chunk)
        if err != nil {
            return fmt.Errorf("update FTS: %w", err)
        }
    }

    return tx.Commit()
}

func (s *sqliteStorage) upsertFile(tx *sql.Tx, path, pkg string, hash [32]byte) (int64, error) {
    query := `
        INSERT INTO files (path, package, hash, size, mod_time, indexed_at)
        VALUES (?, ?, ?, ?, ?, ?)
        ON CONFLICT(path) DO UPDATE SET
            package = excluded.package,
            hash = excluded.hash,
            size = excluded.size,
            mod_time = excluded.mod_time,
            indexed_at = excluded.indexed_at
        RETURNING id
    `

    info, err := os.Stat(path)
    if err != nil {
        return 0, err
    }

    var id int64
    err = tx.QueryRow(query,
        path,
        pkg,
        hash[:],
        info.Size(),
        info.ModTime().Unix(),
        time.Now().Unix(),
    ).Scan(&id)

    return id, err
}

func (s *sqliteStorage) upsertChunk(tx *sql.Tx, chunk *chunker.Chunk, fileID int64) error {
    query := `
        INSERT INTO chunks (
            id, file_id, content, package,
            start_line, end_line, start_col, end_col,
            hash, created_at, updated_at
        )
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
        ON CONFLICT(id) DO UPDATE SET
            content = excluded.content,
            start_line = excluded.start_line,
            end_line = excluded.end_line,
            start_col = excluded.start_col,
            end_col = excluded.end_col,
            hash = excluded.hash,
            updated_at = excluded.updated_at
    `

    now := time.Now().Unix()
    _, err := tx.Exec(query,
        chunk.ID,
        fileID,
        chunk.Content,
        chunk.Package,
        chunk.StartLine,
        chunk.EndLine,
        chunk.StartCol,
        chunk.EndCol,
        chunk.Hash[:],
        now,
        now,
    )

    return err
}

func (s *sqliteStorage) upsertEmbedding(tx *sql.Tx, chunkID string, embedding []float32) error {
    // Convert float32 slice to bytes
    buf := new(bytes.Buffer)
    err := binary.Write(buf, binary.LittleEndian, embedding)
    if err != nil {
        return err
    }

    query := `
        INSERT INTO embeddings (chunk_id, embedding, dimensions, model, created_at)
        VALUES (?, ?, ?, ?, ?)
        ON CONFLICT(chunk_id) DO UPDATE SET
            embedding = excluded.embedding,
            dimensions = excluded.dimensions,
            model = excluded.model,
            created_at = excluded.created_at
    `

    _, err = tx.Exec(query,
        chunkID,
        buf.Bytes(),
        len(embedding),
        "jina-embeddings-v3", // Or from config
        time.Now().Unix(),
    )

    return err
}

func (s *sqliteStorage) updateFTS(tx *sql.Tx, chunk *chunker.Chunk) error {
    query := `
        INSERT INTO chunks_fts (
            chunk_id, content, signature, doc_comment, package, symbol_names
        )
        VALUES (?, ?, ?, ?, ?, ?)
        ON CONFLICT(chunk_id) DO UPDATE SET
            content = excluded.content,
            signature = excluded.signature,
            doc_comment = excluded.doc_comment,
            package = excluded.package,
            symbol_names = excluded.symbol_names
    `

    symbolNames := strings.Join(chunk.Symbols, " ")

    _, err := tx.Exec(query,
        chunk.ID,
        chunk.Content,
        chunk.Metadata.Signature,
        chunk.Metadata.DocComment,
        chunk.Package,
        symbolNames,
    )

    return err
}
```

#### Vector Search
```go
func (s *sqliteStorage) VectorSearch(
    ctx context.Context,
    query []float32,
    opts SearchOptions,
) ([]SearchResult, error) {
    // Convert query to bytes
    queryBuf := new(bytes.Buffer)
    binary.Write(queryBuf, binary.LittleEndian, query)

    // Use vector similarity
    sql := `
        SELECT
            c.id,
            c.content,
            c.package,
            c.start_line,
            c.end_line,
            f.path,
            vec_distance_cosine(e.embedding, ?) as distance
        FROM chunks c
        JOIN embeddings e ON c.id = e.chunk_id
        JOIN files f ON c.file_id = f.id
    `

    // Add filters
    where, args := s.buildFilters(opts.Filters, []any{queryBuf.Bytes()})
    if where != "" {
        sql += " WHERE " + where
    }

    sql += `
        ORDER BY distance ASC
        LIMIT ? OFFSET ?
    `

    args = append(args, opts.Limit, opts.Offset)

    rows, err := s.db.QueryContext(ctx, sql, args...)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var results []SearchResult
    for rows.Next() {
        var chunk chunker.Chunk
        var distance float64

        err := rows.Scan(
            &chunk.ID,
            &chunk.Content,
            &chunk.Package,
            &chunk.StartLine,
            &chunk.EndLine,
            &chunk.File,
            &distance,
        )
        if err != nil {
            return nil, err
        }

        results = append(results, SearchResult{
            Chunk:    chunk,
            Distance: distance,
            Score:    1.0 - distance, // Convert distance to score
            Method:   SearchMethodVector,
        })
    }

    return results, nil
}
```

#### Text Search (BM25)
```go
func (s *sqliteStorage) TextSearch(
    ctx context.Context,
    query string,
    opts SearchOptions,
) ([]SearchResult, error) {
    sql := `
        SELECT
            c.id,
            c.content,
            c.package,
            c.start_line,
            c.end_line,
            f.path,
            bm25(chunks_fts) as score
        FROM chunks_fts
        JOIN chunks c ON chunks_fts.chunk_id = c.id
        JOIN files f ON c.file_id = f.id
        WHERE chunks_fts MATCH ?
    `

    // Add filters
    where, args := s.buildFilters(opts.Filters, []any{query})
    if where != "" {
        sql += " AND " + where
    }

    sql += `
        ORDER BY score DESC
        LIMIT ? OFFSET ?
    `

    args = append(args, opts.Limit, opts.Offset)

    rows, err := s.db.QueryContext(ctx, sql, args...)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var results []SearchResult
    for rows.Next() {
        var chunk chunker.Chunk
        var score float64

        err := rows.Scan(
            &chunk.ID,
            &chunk.Content,
            &chunk.Package,
            &chunk.StartLine,
            &chunk.EndLine,
            &chunk.File,
            &score,
        )
        if err != nil {
            return nil, err
        }

        results = append(results, SearchResult{
            Chunk:  chunk,
            Score:  score,
            Method: SearchMethodText,
        })
    }

    return results, nil
}
```

#### Hybrid Search
```go
func (s *sqliteStorage) HybridSearch(
    ctx context.Context,
    query string,
    embedding []float32,
    opts SearchOptions,
) ([]SearchResult, error) {
    // Perform both searches
    vectorResults, err := s.VectorSearch(ctx, embedding, opts)
    if err != nil {
        return nil, fmt.Errorf("vector search: %w", err)
    }

    textResults, err := s.TextSearch(ctx, query, opts)
    if err != nil {
        return nil, fmt.Errorf("text search: %w", err)
    }

    // Fusion: Reciprocal Rank Fusion (RRF)
    // Score = 1 / (k + rank)
    // k = 60 (standard constant)

    const k = 60
    scoreMap := make(map[string]float64)
    chunkMap := make(map[string]chunker.Chunk)

    // Vector results
    for i, result := range vectorResults {
        score := 1.0 / (k + float64(i+1))
        scoreMap[result.Chunk.ID] = score * 0.5 // Weight: 50%
        chunkMap[result.Chunk.ID] = result.Chunk
    }

    // Text results
    for i, result := range textResults {
        score := 1.0 / (k + float64(i+1))
        scoreMap[result.Chunk.ID] += score * 0.5 // Weight: 50%
        chunkMap[result.Chunk.ID] = result.Chunk
    }

    // Convert to results
    var results []SearchResult
    for id, score := range scoreMap {
        results = append(results, SearchResult{
            Chunk:  chunkMap[id],
            Score:  score,
            Method: SearchMethodHybrid,
        })
    }

    // Sort by score
    sort.Slice(results, func(i, j int) bool {
        return results[i].Score > results[j].Score
    })

    // Apply limit
    if len(results) > opts.Limit {
        results = results[:opts.Limit]
    }

    return results, nil
}
```

#### Filter Building
```go
func (s *sqliteStorage) buildFilters(filters SearchFilters, args []any) (string, []any) {
    var conditions []string

    if len(filters.Packages) > 0 {
        placeholders := strings.Repeat("?,", len(filters.Packages))
        placeholders = placeholders[:len(placeholders)-1]
        conditions = append(conditions, fmt.Sprintf("c.package IN (%s)", placeholders))
        for _, pkg := range filters.Packages {
            args = append(args, pkg)
        }
    }

    if len(filters.Files) > 0 {
        placeholders := strings.Repeat("?,", len(filters.Files))
        placeholders = placeholders[:len(placeholders)-1]
        conditions = append(conditions, fmt.Sprintf("f.path IN (%s)", placeholders))
        for _, file := range filters.Files {
            args = append(args, file)
        }
    }

    if len(filters.SymbolKind) > 0 {
        conditions = append(conditions, `
            c.id IN (
                SELECT chunk_id FROM symbols
                WHERE kind IN (?)
            )
        `)
        args = append(args, filters.SymbolKind)
    }

    if filters.AggregatesOnly {
        conditions = append(conditions, `
            c.id IN (
                SELECT s.chunk_id FROM symbols s
                JOIN symbol_attributes sa ON s.id = sa.symbol_id
                WHERE sa.is_aggregate_root = 1
            )
        `)
    }

    // Similar for other filters...

    if len(conditions) == 0 {
        return "", args
    }

    return strings.Join(conditions, " AND "), args
}
```

#### Performance Targets
- Vector search: < 100ms for 10k chunks
- Text search: < 50ms for 10k chunks
- Hybrid search: < 200ms for 10k chunks
- Insert throughput: 1000 chunks/second
- Database size: ~10MB per 1000 chunks

---

### 6. Searcher Component

**Purpose:** Provide high-level search interface with reranking.

**Input:** User query
**Output:** Ranked search results

#### Interface
```go
package searcher

import (
    "context"
    "gocontext/storage"
    "gocontext/embedder"
)

// Searcher provides search functionality
type Searcher interface {
    // Search performs semantic search
    Search(ctx context.Context, query string, opts SearchOptions) (*SearchResponse, error)

    // SearchSymbols finds specific symbols
    SearchSymbols(ctx context.Context, symbolName string, opts SearchOptions) (*SearchResponse, error)

    // SearchDomain finds domain concepts
    SearchDomain(ctx context.Context, concept string, opts SearchOptions) (*SearchResponse, error)
}

// SearchOptions configures search
type SearchOptions struct {
    Limit          int
    IncludeContext bool
    Rerank         bool
    RerankModel    string
    Filters        storage.SearchFilters
}

// SearchResponse contains search results
type SearchResponse struct {
    Results    []storage.SearchResult
    Query      string
    TotalResults int
    SearchTime time.Duration
    Method     string
}
```

#### Implementation
```go
type searcher struct {
    storage  storage.Storage
    embedder embedder.Embedder
    reranker Reranker
}

func New(
    s storage.Storage,
    e embedder.Embedder,
    r Reranker,
) Searcher {
    return &searcher{
        storage:  s,
        embedder: e,
        reranker: r,
    }
}

func (s *searcher) Search(
    ctx context.Context,
    query string,
    opts SearchOptions,
) (*SearchResponse, error) {
    start := time.Now()

    // Generate query embedding
    embedding, err := s.embedder.EmbedQuery(query)
    if err != nil {
        return nil, fmt.Errorf("embed query: %w", err)
    }

    // Perform hybrid search
    results, err := s.storage.HybridSearch(ctx, query, embedding, storage.SearchOptions{
        Limit:          opts.Limit * 2, // Get more for reranking
        Filters:        opts.Filters,
        IncludeContext: opts.IncludeContext,
    })
    if err != nil {
        return nil, fmt.Errorf("search: %w", err)
    }

    // Rerank if requested
    if opts.Rerank && s.reranker != nil {
        results, err = s.reranker.Rerank(ctx, query, results, opts.Limit)
        if err != nil {
            return nil, fmt.Errorf("rerank: %w", err)
        }
    } else if len(results) > opts.Limit {
        results = results[:opts.Limit]
    }

    return &SearchResponse{
        Results:      results,
        Query:        query,
        TotalResults: len(results),
        SearchTime:   time.Since(start),
        Method:       "hybrid",
    }, nil
}

func (s *searcher) SearchSymbols(
    ctx context.Context,
    symbolName string,
    opts SearchOptions,
) (*SearchResponse, error) {
    // Optimize for exact symbol matching
    // Use text search for better precision

    start := time.Now()

    results, err := s.storage.TextSearch(ctx, symbolName, storage.SearchOptions{
        Limit:          opts.Limit,
        Filters:        opts.Filters,
        IncludeContext: opts.IncludeContext,
    })
    if err != nil {
        return nil, err
    }

    return &SearchResponse{
        Results:      results,
        Query:        symbolName,
        TotalResults: len(results),
        SearchTime:   time.Since(start),
        Method:       "symbol",
    }, nil
}

func (s *searcher) SearchDomain(
    ctx context.Context,
    concept string,
    opts SearchOptions,
) (*SearchResponse, error) {
    // Add DDD-specific filters
    opts.Filters.AggregatesOnly = true

    return s.Search(ctx, concept, opts)
}
```

#### Reranking (Optional)
```go
// Reranker reorders search results
type Reranker interface {
    Rerank(ctx context.Context, query string, results []storage.SearchResult, limit int) ([]storage.SearchResult, error)
}

// Jina Reranker
type jinaReranker struct {
    apiKey string
    model  string
    client *http.Client
}

func (r *jinaReranker) Rerank(
    ctx context.Context,
    query string,
    results []storage.SearchResult,
    limit int,
) ([]storage.SearchResult, error) {
    // Prepare documents
    documents := make([]string, len(results))
    for i, result := range results {
        documents[i] = result.Chunk.Content
    }

    // Call Jina reranker API
    req := JinaRerankRequest{
        Model:     r.model,
        Query:     query,
        Documents: documents,
        TopN:      limit,
    }

    resp, err := r.callAPI(ctx, req)
    if err != nil {
        return nil, err
    }

    // Reorder results
    reranked := make([]storage.SearchResult, len(resp.Results))
    for i, rerankResult := range resp.Results {
        idx := rerankResult.Index
        reranked[i] = results[idx]
        reranked[i].Score = rerankResult.RelevanceScore
        reranked[i].Rank = i + 1
    }

    return reranked, nil
}

type JinaRerankRequest struct {
    Model     string   `json:"model"`
    Query     string   `json:"query"`
    Documents []string `json:"documents"`
    TopN      int      `json:"top_n"`
}

type JinaRerankResponse struct {
    Results []struct {
        Index          int     `json:"index"`
        RelevanceScore float64 `json:"relevance_score"`
    } `json:"results"`
}
```

---

### 7. MCP Server Component

**Purpose:** Expose functionality via Model Context Protocol for AI agents.

**Input:** MCP requests from Claude Code/Codex CLI
**Output:** MCP responses

#### Interface
```go
package mcp

import (
    "context"
    "github.com/mark3labs/mcp-go/server"
    "gocontext/indexer"
    "gocontext/searcher"
)

// Server implements MCP protocol
type Server struct {
    indexer  indexer.Indexer
    searcher searcher.Searcher
    mcpServer *server.MCPServer
}

func New(idx indexer.Indexer, srch searcher.Searcher) *Server {
    return &Server{
        indexer:  idx,
        searcher: srch,
    }
}

func (s *Server) Start(ctx context.Context) error {
    // Initialize MCP server
    s.mcpServer = server.NewMCPServer(
        "gocontext",
        "1.0.0",
        server.WithStdio(), // Use stdio transport
    )

    // Register tools
    s.registerTools()

    // Register resources
    s.registerResources()

    // Register prompts
    s.registerPrompts()

    // Start server
    return s.mcpServer.Serve(ctx)
}
```

#### MCP Tools
```go
func (s *Server) registerTools() {
    // Tool: index_codebase
    s.mcpServer.AddTool(server.Tool{
        Name:        "index_codebase",
        Description: "Index a Go codebase for semantic search",
        InputSchema: map[string]any{
            "type": "object",
            "properties": map[string]any{
                "path": map[string]any{
                    "type":        "string",
                    "description": "Path to the codebase root",
                },
                "include_tests": map[string]any{
                    "type":        "boolean",
                    "description": "Include test files",
                    "default":     false,
                },
                "include_vendor": map[string]any{
                    "type":        "boolean",
                    "description": "Include vendor directory",
                    "default":     false,
                },
            },
            "required": []string{"path"},
        },
    }, s.handleIndexCodebase)

    // Tool: search_code
    s.mcpServer.AddTool(server.Tool{
        Name:        "search_code",
        Description: "Search codebase using semantic search",
        InputSchema: map[string]any{
            "type": "object",
            "properties": map[string]any{
                "query": map[string]any{
                    "type":        "string",
                    "description": "Search query (natural language or keywords)",
                },
                "limit": map[string]any{
                    "type":        "integer",
                    "description": "Maximum results to return",
                    "default":     5,
                },
                "include_context": map[string]any{
                    "type":        "boolean",
                    "description": "Include surrounding context",
                    "default":     true,
                },
                "filters": map[string]any{
                    "type":        "object",
                    "description": "Optional filters",
                    "properties": map[string]any{
                        "packages": map[string]any{
                            "type":        "array",
                            "items":       map[string]string{"type": "string"},
                            "description": "Filter by package names",
                        },
                        "symbol_kind": map[string]any{
                            "type":        "string",
                            "enum":        []string{"function", "method", "struct", "interface", "type"},
                            "description": "Filter by symbol type",
                        },
                        "aggregates_only": map[string]any{
                            "type":        "boolean",
                            "description": "Only aggregate roots (DDD)",
                            "default":     false,
                        },
                    },
                },
            },
            "required": []string{"query"},
        },
    }, s.handleSearchCode)

    // Tool: search_symbol
    s.mcpServer.AddTool(server.Tool{
        Name:        "search_symbol",
        Description: "Find specific symbols (functions, types, etc.)",
        InputSchema: map[string]any{
            "type": "object",
            "properties": map[string]any{
                "symbol_name": map[string]any{
                    "type":        "string",
                    "description": "Symbol name to search for",
                },
                "symbol_kind": map[string]any{
                    "type":        "string",
                    "enum":        []string{"function", "method", "struct", "interface", "type"},
                    "description": "Type of symbol",
                },
            },
            "required": []string{"symbol_name"},
        },
    }, s.handleSearchSymbol)

    // Tool: find_domain_concept
    s.mcpServer.AddTool(server.Tool{
        Name:        "find_domain_concept",
        Description: "Find domain concepts (aggregates, entities, value objects)",
        InputSchema: map[string]any{
            "type": "object",
            "properties": map[string]any{
                "concept": map[string]any{
                    "type":        "string",
                    "description": "Domain concept to search for",
                },
                "concept_type": map[string]any{
                    "type":        "string",
                    "enum":        []string{"aggregate", "entity", "value_object", "repository", "service"},
                    "description": "Type of domain concept",
                },
            },
            "required": []string{"concept"},
        },
    }, s.handleFindDomainConcept)

    // Tool: get_status
    s.mcpServer.AddTool(server.Tool{
        Name:        "get_status",
        Description: "Get indexing status and statistics",
        InputSchema: map[string]any{
            "type":       "object",
            "properties": map[string]any{},
        },
    }, s.handleGetStatus)

    // Tool: reindex
    s.mcpServer.AddTool(server.Tool{
        Name:        "reindex",
        Description: "Incrementally re-index changed files",
        InputSchema: map[string]any{
            "type": "object",
            "properties": map[string]any{
                "path": map[string]any{
                    "type":        "string",
                    "description": "Path to the codebase root",
                },
            },
            "required": []string{"path"},
        },
    }, s.handleReindex)

    // Tool: clear_index
    s.mcpServer.AddTool(server.Tool{
        Name:        "clear_index",
        Description: "Clear all indexed data",
        InputSchema: map[string]any{
            "type": "object",
            "properties": map[string]any{
                "confirm": map[string]any{
                    "type":        "boolean",
                    "description": "Confirm deletion",
                    "default":     false,
                },
            },
            "required": []string{"confirm"},
        },
    }, s.handleClearIndex)
}
```

#### Tool Handlers
```go
func (s *Server) handleIndexCodebase(ctx context.Context, args map[string]any) (*server.ToolResponse, error) {
    path := args["path"].(string)
    includeTests := getBool(args, "include_tests", false)
    includeVendor := getBool(args, "include_vendor", false)

    // Start indexing
    result, err := s.indexer.Index(ctx, path, indexer.IndexOptions{
        IncludeTests:  includeTests,
        IncludeVendor: includeVendor,
    })
    if err != nil {
        return nil, fmt.Errorf("indexing failed: %w", err)
    }

    // Format response
    response := fmt.Sprintf(`Indexing completed successfully!

📊 Statistics:
- Files processed: %d
- Files skipped: %d (unchanged)
- Files errored: %d
- Chunks created: %d
- Duration: %s

✅ Your codebase is now indexed and ready for semantic search.`,
        result.FilesProcessed,
        result.FilesSkipped,
        result.FilesErrored,
        result.ChunksCreated,
        result.Duration,
    )

    if len(result.Errors) > 0 {
        response += fmt.Sprintf("\n\n⚠️  Errors encountered:\n")
        for i, err := range result.Errors {
            if i >= 5 {
                response += fmt.Sprintf("... and %d more\n", len(result.Errors)-5)
                break
            }
            response += fmt.Sprintf("- %s: %v\n", err.File, err.Error)
        }
    }

    return &server.ToolResponse{
        Content: []server.Content{
            {
                Type: "text",
                Text: response,
            },
        },
    }, nil
}

func (s *Server) handleSearchCode(ctx context.Context, args map[string]any) (*server.ToolResponse, error) {
    query := args["query"].(string)
    limit := getInt(args, "limit", 5)
    includeContext := getBool(args, "include_context", true)

    // Parse filters
    filters := parseFilters(args)

    // Perform search
    results, err := s.searcher.Search(ctx, query, searcher.SearchOptions{
        Limit:          limit,
        IncludeContext: includeContext,
        Rerank:         true,
        Filters:        filters,
    })
    if err != nil {
        return nil, fmt.Errorf("search failed: %w", err)
    }

    // Format response
    var response string
    response += fmt.Sprintf("🔍 Found %d results for: %q\n\n", results.TotalResults, query)

    for i, result := range results.Results {
        response += fmt.Sprintf("📄 Result %d (Score: %.2f)\n", i+1, result.Score)
        response += fmt.Sprintf("File: %s:%d-%d\n", result.Chunk.File, result.Chunk.StartLine, result.Chunk.EndLine)
        response += fmt.Sprintf("Package: %s\n", result.Chunk.Package)

        if len(result.Chunk.Symbols) > 0 {
            response += fmt.Sprintf("Symbols: %s\n", strings.Join(result.Chunk.Symbols, ", "))
        }

        response += "\n```go\n"
        response += result.Chunk.Content
        response += "\n```\n\n"

        if includeContext && result.Chunk.Context.Before != "" {
            response += "Context (before):\n```go\n"
            response += result.Chunk.Context.Before
            response += "\n```\n\n"
        }
    }

    response += fmt.Sprintf("Search completed in %s", results.SearchTime)

    return &server.ToolResponse{
        Content: []server.Content{
            {
                Type: "text",
                Text: response,
            },
        },
    }, nil
}

func (s *Server) handleGetStatus(ctx context.Context, args map[string]any) (*server.ToolResponse, error) {
    // Get indexing status
    status, err := s.indexer.GetStatus(ctx)
    if err != nil {
        return nil, err
    }

    // Get storage stats
    stats, err := s.storage.GetStats(ctx)
    if err != nil {
        return nil, err
    }

    response := fmt.Sprintf(`📊 GoContext Status

Indexing State: %s
Progress: %.1f%%

Statistics:
- Total Chunks: %d
- Total Files: %d
- Total Packages: %d
- Database Size: %s
- Last Indexed: %s

%s`,
        status.State,
        status.Progress*100,
        stats.TotalChunks,
        stats.TotalFiles,
        stats.TotalPackages,
        formatBytes(stats.DatabaseSize),
        stats.LastIndexed.Format(time.RFC3339),
        getStatusMessage(status),
    )

    return &server.ToolResponse{
        Content: []server.Content{
            {
                Type: "text",
                Text: response,
            },
        },
    }, nil
}
```

#### Helper Functions
```go
func getBool(args map[string]any, key string, def bool) bool {
    if v, ok := args[key]; ok {
        if b, ok := v.(bool); ok {
            return b
        }
    }
    return def
}

func getInt(args map[string]any, key string, def int) int {
    if v, ok := args[key]; ok {
        switch n := v.(type) {
        case int:
            return n
        case float64:
            return int(n)
        }
    }
    return def
}

func parseFilters(args map[string]any) storage.SearchFilters {
    filters := storage.SearchFilters{}

    if f, ok := args["filters"].(map[string]any); ok {
        if pkgs, ok := f["packages"].([]any); ok {
            for _, p := range pkgs {
                if pkg, ok := p.(string); ok {
                    filters.Packages = append(filters.Packages, pkg)
                }
            }
        }

        if kind, ok := f["symbol_kind"].(string); ok {
            filters.SymbolKind = []parser.SymbolKind{parseSymbolKind(kind)}
        }

        if agg, ok := f["aggregates_only"].(bool); ok {
            filters.AggregatesOnly = agg
        }
    }

    return filters
}

func formatBytes(bytes int64) string {
    const unit = 1024
    if bytes < unit {
        return fmt.Sprintf("%d B", bytes)
    }
    div, exp := int64(unit), 0
    for n := bytes / unit; n >= unit; n /= unit {
        div *= unit
        exp++
    }
    return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
```

---

## Data Models

### Core Types Summary

```go
// Symbol represents a code symbol
type Symbol struct {
    Name       string
    Kind       SymbolKind
    Package    string
    File       string
    Position   Position
    Signature  string
    DocComment string
    Scope      ScopeKind
    Attributes SymbolAttrs
}

// Chunk represents a code chunk
type Chunk struct {
    ID          string
    Content     string
    File        string
    Package     string
    Lines       Range
    Symbols     []string
    Context     ChunkContext
    Metadata    ChunkMetadata
    Embedding   []float32
    Hash        [32]byte
}

// SearchResult represents a search result
type SearchResult struct {
    Chunk    Chunk
    Score    float64
    Distance float64
    Rank     int
    Method   SearchMethod
}
```

---

## API Definitions

### MCP Tools

#### 1. index_codebase
**Purpose:** Index a Go codebase for semantic search

**Input:**
```json
{
  "path": "/path/to/codebase",
  "include_tests": false,
  "include_vendor": false,
  "include_internal": true
}
```

**Output:**
```json
{
  "files_processed": 150,
  "files_skipped": 25,
  "chunks_created": 1250,
  "duration": "2m15s",
  "errors": []
}
```

#### 2. search_code
**Purpose:** Semantic search for code

**Input:**
```json
{
  "query": "patient data validation",
  "limit": 5,
  "include_context": true,
  "filters": {
    "packages": ["domain/patient"],
    "aggregates_only": true
  }
}
```

**Output:**
```json
{
  "results": [
    {
      "file": "domain/patient/aggregate.go",
      "lines": "45-78",
      "score": 0.92,
      "content": "...",
      "symbols": ["ValidatePatientData"],
      "context": "..."
    }
  ],
  "total_results": 5,
  "search_time": "145ms"
}
```

#### 3. search_symbol
**Purpose:** Find specific symbols

**Input:**
```json
{
  "symbol_name": "CreatePatient",
  "symbol_kind": "function"
}
```

**Output:**
```json
{
  "results": [
    {
      "file": "domain/patient/service.go",
      "line": 23,
      "signature": "func CreatePatient(ctx context.Context, cmd CreatePatientCommand) (*Patient, error)",
      "package": "patient",
      "content": "..."
    }
  ]
}
```

#### 4. find_domain_concept
**Purpose:** Find DDD concepts

**Input:**
```json
{
  "concept": "trial protocol",
  "concept_type": "aggregate"
}
```

**Output:**
```json
{
  "results": [
    {
      "aggregate": "Protocol",
      "file": "domain/protocol/aggregate.go",
      "entities": ["ProtocolVersion", "Inclusion", "Exclusion"],
      "value_objects": ["ProtocolID", "ProtocolName"],
      "content": "..."
    }
  ]
}
```

#### 5. get_status
**Purpose:** Get indexing status

**Input:** `{}`

**Output:**
```json
{
  "state": "completed",
  "progress": 1.0,
  "total_chunks": 1250,
  "total_files": 150,
  "total_packages": 12,
  "database_size": "45.2 MB",
  "last_indexed": "2025-11-06T10:30:00Z"
}
```

#### 6. reindex
**Purpose:** Incremental re-index

**Input:**
```json
{
  "path": "/path/to/codebase"
}
```

**Output:**
```json
{
  "files_changed": 8,
  "chunks_updated": 65,
  "chunks_deleted": 12,
  "duration": "18s"
}
```

#### 7. clear_index
**Purpose:** Clear all indexed data

**Input:**
```json
{
  "confirm": true
}
```

**Output:**
```json
{
  "deleted": true,
  "chunks_removed": 1250,
  "database_reset": true
}
```

---

## Implementation Phases

### Phase 1: Core Foundation (Week 1-2)

**Goals:**
- Basic Go AST parsing
- Simple chunking strategy
- SQLite storage without vectors
- Basic CLI for testing

**Deliverables:**
1. Parser that extracts functions, types, and basic metadata
2. Function-level chunker
3. SQLite schema with full-text search
4. CLI command: `gocontext index <path>`
5. CLI command: `gocontext search <query>`

**Acceptance Criteria:**
- Can parse 10k LOC Go codebase
- Can create chunks for all functions
- Can search by text (BM25)
- Basic test coverage (>60%)

**Tasks:**
```
[ ] Set up Go project structure
[ ] Implement parser.Parser interface
[ ] Implement AST traversal
[ ] Extract functions and types
[ ] Implement chunker.Chunker interface
[ ] Create function-level chunks
[ ] Design SQLite schema
[ ] Implement storage.Storage interface (without vectors)
[ ] Create CLI with cobra
[ ] Add index command
[ ] Add search command (text only)
[ ] Write unit tests
[ ] Write integration tests
[ ] Documentation
```

### Phase 2: Embeddings & Vector Search (Week 2-3)

**Goals:**
- Integrate embedding API (Jina/OpenAI)
- Add vector similarity search
- Implement hybrid search

**Deliverables:**
1. Embedder implementation with API integration
2. Vector storage in SQLite
3. Vector similarity search
4. Hybrid search (vector + text)
5. Embedding cache

**Acceptance Criteria:**
- Can generate embeddings for chunks
- Vector search returns semantically relevant results
- Hybrid search combines both methods
- Cache reduces API calls by >80% on re-index

**Tasks:**
```
[ ] Implement embedder.Embedder interface
[ ] Integrate Jina AI API
[ ] Add API retry logic
[ ] Implement embedding cache
[ ] Add vector column to SQLite
[ ] Implement vector similarity search
[ ] Implement hybrid search with RRF
[ ] Add embedding to index command
[ ] Update search command to use hybrid search
[ ] Write tests for embedder
[ ] Write tests for vector search
[ ] Benchmark search performance
```

### Phase 3: MCP Integration (Week 3-4)

**Goals:**
- Implement MCP server
- Define and implement MCP tools
- Test with Claude Code

**Deliverables:**
1. MCP server implementation
2. All 7 MCP tools working
3. Integration with Claude Code
4. Progress reporting for indexing

**Acceptance Criteria:**
- MCP server starts and communicates via stdio
- Claude Code can index a codebase
- Claude Code can search code
- All tools return properly formatted responses

**Tasks:**
```
[ ] Integrate mcp-go SDK
[ ] Implement MCP server
[ ] Define tool schemas
[ ] Implement index_codebase handler
[ ] Implement search_code handler
[ ] Implement search_symbol handler
[ ] Implement find_domain_concept handler
[ ] Implement get_status handler
[ ] Implement reindex handler
[ ] Implement clear_index handler
[ ] Test with Claude Code
[ ] Test with Codex CLI
[ ] Add progress callbacks
[ ] Error handling and validation
[ ] Documentation for MCP integration
```

### Phase 4: Advanced Features (Week 4-5)

**Goals:**
- Incremental indexing
- Domain-aware search
- Reranking
- Performance optimization

**Deliverables:**
1. Incremental reindexing based on file hashes
2. DDD pattern detection
3. Domain concept search
4. Optional reranking with Jina
5. Performance optimizations

**Acceptance Criteria:**
- Reindex detects and processes only changed files
- Can identify aggregates, entities, value objects
- Domain search filters work correctly
- Reranking improves search relevance
- Meets all performance targets

**Tasks:**
```
[ ] Implement file hash tracking
[ ] Implement incremental reindex logic
[ ] Implement DDD pattern detection
[ ] Add SymbolAttrs to schema
[ ] Implement domain filters
[ ] Integrate Jina reranker
[ ] Add reranking option to search
[ ] Optimize parser for large files
[ ] Optimize chunker for large functions
[ ] Add concurrency to indexing
[ ] Profile and optimize bottlenecks
[ ] Add benchmarks
[ ] Load testing with large codebases
[ ] Documentation for advanced features
```

### Phase 5: Polish & Release (Week 5)

**Goals:**
- Production readiness
- Documentation
- Packaging
- Release

**Deliverables:**
1. Comprehensive documentation
2. Installation scripts
3. Binary releases for multiple platforms
4. Example configurations
5. Tutorial and guides

**Acceptance Criteria:**
- Full test coverage (>80%)
- All documentation complete
- Binaries work on macOS, Linux, Windows
- Installation is one command
- Users can successfully index and search

**Tasks:**
```
[ ] Complete all documentation
[ ] Write README
[ ] Write CONTRIBUTING guide
[ ] Write user guide
[ ] Create example projects
[ ] Set up GitHub Actions for CI/CD
[ ] Build cross-platform binaries
[ ] Create installation script
[ ] Test installation on all platforms
[ ] Create demo video
[ ] Prepare release notes
[ ] Tag v1.0.0 release
[ ] Publish binaries
[ ] Announce release
```

---

## Testing Strategy

### Unit Tests

**Coverage Target:** >80%

**Key Areas:**
- Parser: Test AST extraction for various Go constructs
- Chunker: Test chunking strategies and edge cases
- Embedder: Test API integration with mocking
- Storage: Test CRUD operations and search
- Searcher: Test search logic and ranking

**Test Structure:**
```go
func TestParser_ParseFile(t *testing.T) {
    tests := []struct {
        name     string
        input    string
        expected *ParseResult
        wantErr  bool
    }{
        {
            name: "simple function",
            input: `package main
func Add(a, b int) int {
    return a + b
}`,
            expected: &ParseResult{
                Symbols: []Symbol{
                    {
                        Name: "Add",
                        Kind: SymbolFunction,
                        // ...
                    },
                },
            },
            wantErr: false,
        },
        // More test cases...
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Test implementation
        })
    }
}
```

### Integration Tests

**Coverage Target:** Key user flows

**Test Scenarios:**
1. Full indexing pipeline: Parse → Chunk → Embed → Store
2. Search pipeline: Query → Embed → Search → Rerank
3. Incremental reindexing
4. MCP tool integration
5. Error handling and recovery

**Test Structure:**
```go
func TestIntegration_IndexAndSearch(t *testing.T) {
    // Setup test codebase
    tmpDir := setupTestCodebase(t)
    defer os.RemoveAll(tmpDir)

    // Initialize components
    parser := parser.New()
    chunker := chunker.New()
    embedder := embedder.New(embedder.Config{
        Provider: embedder.ProviderMock, // Use mock for testing
    })
    storage := storage.NewSQLite(":memory:")
    defer storage.Close()

    indexer := indexer.New(parser, chunker, embedder, storage)

    // Index codebase
    ctx := context.Background()
    result, err := indexer.Index(ctx, tmpDir, indexer.IndexOptions{})
    require.NoError(t, err)
    assert.Greater(t, result.ChunksCreated, 0)

    // Search
    searcher := searcher.New(storage, embedder, nil)
    results, err := searcher.Search(ctx, "test query", searcher.SearchOptions{
        Limit: 5,
    })
    require.NoError(t, err)
    assert.NotEmpty(t, results.Results)
}
```

### End-to-End Tests

**Test Scenarios:**
1. Real Go project indexing (e.g., a well-known OSS project)
2. Claude Code integration test
3. Performance benchmarks
4. Stress tests with large codebases

**Benchmark Example:**
```go
func BenchmarkIndexing(b *testing.B) {
    // Setup
    codebasePath := "testdata/large-project"

    for i := 0; i < b.N; i++ {
        // Index codebase
        // Measure time and memory
    }
}

func BenchmarkSearch(b *testing.B) {
    // Setup indexed database

    queries := []string{
        "user authentication",
        "database connection",
        "error handling",
    }

    for i := 0; i < b.N; i++ {
        query := queries[i%len(queries)]
        // Perform search
        // Measure latency
    }
}
```

### Test Data

**Create Test Codebases:**
- Small: 100 LOC, 5 files
- Medium: 10k LOC, 50 files
- Large: 100k LOC, 500 files
- Real: Clone popular Go projects (e.g., chi, viper, cobra)

---

## Deployment & Operations

### Installation

**Option 1: Binary Release (Recommended)**
```bash
# macOS/Linux
curl -L https://github.com/user/gocontext/releases/latest/download/gocontext-$(uname -s)-$(uname -m) -o gocontext
chmod +x gocontext
sudo mv gocontext /usr/local/bin/

# Windows (PowerShell)
Invoke-WebRequest -Uri https://github.com/user/gocontext/releases/latest/download/gocontext-windows-amd64.exe -OutFile gocontext.exe
```

**Option 2: Go Install**
```bash
go install github.com/user/gocontext@latest
```

**Option 3: Build from Source**
```bash
git clone https://github.com/user/gocontext.git
cd gocontext
make build
sudo make install
```

### Configuration

**Config File:** `~/.gocontext/config.yaml`
```yaml
# Embedding provider
embedding:
  provider: jina  # jina, openai, local
  api_key: ${JINA_API_KEY}
  model: jina-embeddings-v3
  dimensions: 1024

# Storage
storage:
  path: ~/.gocontext/data/index.db
  cache_size: 500MB

# Indexing
indexing:
  workers: 4
  include_tests: false
  include_vendor: false
  max_file_size: 1MB
  chunk_size: 500

# Search
search:
  default_limit: 5
  enable_reranking: true
  reranking_model: jina-reranker-v2-base-multilingual

# Logging
logging:
  level: info
  format: text
  output: stderr
```

### MCP Integration

**Claude Code:**
```bash
# Add to Claude Code configuration
gocontext mcp install
```

This adds to `~/.config/claude/mcp_config.json`:
```json
{
  "mcpServers": {
    "gocontext": {
      "command": "gocontext",
      "args": ["mcp", "serve"],
      "env": {
        "JINA_API_KEY": "your-api-key"
      }
    }
  }
}
```

**Codex CLI:**
Add to `~/.codex/config.toml`:
```toml
[mcp_servers.gocontext]
command = "gocontext"
args = ["mcp", "serve"]
env = { "JINA_API_KEY" = "your-api-key" }
```

### CLI Commands

```bash
# Initialize configuration
gocontext init

# Index a codebase
gocontext index /path/to/project

# Search code
gocontext search "query"

# Search symbols
gocontext symbol FunctionName

# Get status
gocontext status

# Reindex (incremental)
gocontext reindex

# Clear index
gocontext clear --confirm

# Start MCP server (for Claude Code/Codex)
gocontext mcp serve

# Install MCP integration
gocontext mcp install
```

### Monitoring & Logging

**Logs Location:** `~/.gocontext/logs/`

**Log Levels:**
- `debug`: Detailed debugging information
- `info`: General information (default)
- `warn`: Warning messages
- `error`: Error messages

**Metrics to Track:**
- Indexing time per file
- Chunks created per file
- Embedding API latency
- Search latency
- Cache hit rate
- Error rate

**Health Checks:**
```bash
# Check if index exists
gocontext status

# Validate index integrity
gocontext validate

# Rebuild index if corrupted
gocontext rebuild
```

---

## Security & Compliance

### Data Privacy

**Local-First Architecture:**
- All code stays on local machine
- Index stored in local SQLite database
- Only code embeddings sent to API (no raw code)

**API Privacy:**
- Use HTTPS for all API calls
- API keys stored securely in config/env
- Option to use local embeddings (no API calls)

**HIPAA/FDA Compliance Considerations:**
- For clinical trials: Use local embeddings
- Ensure no PHI in code comments
- Audit log for all search queries
- Encryption at rest option

### Security Best Practices

**API Key Management:**
```bash
# Use environment variable
export JINA_API_KEY=your-api-key

# Or use system keychain (macOS)
gocontext config set-key --keychain
```

**File Permissions:**
```bash
# Restrict config file permissions
chmod 600 ~/.gocontext/config.yaml

# Restrict database permissions
chmod 600 ~/.gocontext/data/index.db
```

**Input Validation:**
- Validate all file paths
- Sanitize search queries
- Limit file sizes
- Prevent directory traversal

### Audit Logging

**Log Format:**
```json
{
  "timestamp": "2025-11-06T10:30:00Z",
  "action": "search",
  "user": "username",
  "query": "patient data",
  "results_count": 5,
  "duration_ms": 145
}
```

**Audit Events:**
- Indexing started/completed
- Search queries
- Configuration changes
- API calls
- Errors

---

## Open Questions & Decisions

### 1. Embedding Provider

**Options:**
- **Jina AI** (Recommended)
  - Pros: High quality, reasonable pricing, purpose-built for code
  - Cons: API dependency, costs scale with usage
- **OpenAI**
  - Pros: Very high quality, well-documented
  - Cons: Higher cost, rate limits
- **Local (Sentence Transformers)**
  - Pros: Free, private, fast
  - Cons: Lower quality, need to bundle model or download

**Decision:** Start with Jina, add local option in Phase 4

### 2. Vector Storage

**Options:**
- **SQLite + vector extension** (Recommended)
  - Pros: Single file, no external dependencies, simple
  - Cons: Limited scalability (but fine for 100k LOC)
- **Qdrant**
  - Pros: Purpose-built for vectors, scalable
  - Cons: Additional service to run
- **Weaviate**
  - Pros: Powerful features, good performance
  - Cons: Complex setup

**Decision:** SQLite for MVP, consider Qdrant for v2.0

### 3. Chunking Strategy

**Options:**
- **Function-level** (Recommended for Phase 1)
  - Pros: Natural semantic boundaries, good for search
  - Cons: Large functions may exceed token limits
- **Type-level**
  - Pros: Groups related methods, good for understanding types
  - Cons: May create very large chunks
- **Domain concept-level**
  - Pros: Best for DDD codebases, groups aggregates
  - Cons: Complex to implement, requires pattern detection

**Decision:** Function-level for Phase 1, add other strategies in Phase 4

### 4. Reranking

**Options:**
- **Jina Reranker** (Recommended)
  - Pros: High quality, easy integration
  - Cons: API cost, latency
- **Local reranking** (e.g., cross-encoder)
  - Pros: Fast, free
  - Cons: Lower quality, need to bundle model
- **No reranking**
  - Pros: Simpler, faster
  - Cons: Lower search quality

**Decision:** Optional Jina reranker in Phase 4

### 5. CLI vs MCP-Only

**Options:**
- **CLI + MCP** (Recommended)
  - Pros: Useful for testing, standalone tool, flexible
  - Cons: More code to maintain
- **MCP-only**
  - Pros: Simpler, focused
  - Cons: Harder to test, less flexible

**Decision:** Both - CLI for testing/standalone, MCP as primary interface

### 6. Language Support

**Future Consideration:**
- Should we add TypeScript/JavaScript support?
- Or stay Go-only and focused?

**Recommendation:** Stay Go-only for v1.0, consider multi-language in v2.0

---

## Success Metrics

### Phase 1 (Weeks 1-2)
- [ ] Can parse 10k LOC Go codebase
- [ ] Text search returns relevant results
- [ ] Tests pass with >60% coverage

### Phase 2 (Week 3)
- [ ] Embeddings generated successfully
- [ ] Vector search works
- [ ] Hybrid search improves relevance

### Phase 3 (Week 4)
- [ ] Claude Code can index and search
- [ ] All MCP tools functional
- [ ] Smooth user experience

### Phase 4 (Week 5)
- [ ] Incremental reindex works
- [ ] DDD patterns detected
- [ ] Performance targets met

### Phase 5 (Week 5)
- [ ] All documentation complete
- [ ] v1.0.0 released
- [ ] Installable with one command

### Final Success Criteria
- [ ] Index 100k LOC in < 5 minutes
- [ ] Search latency p95 < 500ms
- [ ] Test coverage > 80%
- [ ] Works with Claude Code and Codex CLI
- [ ] Positive user feedback

---

## Timeline Summary

```
Week 1-2: Core Foundation
├─ Parser implementation
├─ Chunker implementation
├─ SQLite storage (text only)
└─ Basic CLI

Week 2-3: Embeddings & Vector Search
├─ Embedding API integration
├─ Vector storage
├─ Hybrid search
└─ Caching

Week 3-4: MCP Integration
├─ MCP server
├─ Tool implementations
├─ Claude Code testing
└─ Progress reporting

Week 4-5: Advanced Features
├─ Incremental indexing
├─ DDD pattern detection
├─ Reranking
└─ Performance optimization

Week 5: Polish & Release
├─ Documentation
├─ Testing
├─ Packaging
└─ Release v1.0.0
```

---

## Appendix A: Go Project Structure

```
gocontext/
├── cmd/
│   └── gocontext/
│       └── main.go              # CLI entry point
├── internal/
│   ├── parser/
│   │   ├── parser.go            # Parser implementation
│   │   ├── ast.go               # AST traversal
│   │   ├── types.go             # Type definitions
│   │   └── parser_test.go
│   ├── chunker/
│   │   ├── chunker.go           # Chunker implementation
│   │   ├── strategies.go        # Chunking strategies
│   │   ├── domain.go            # Domain-aware chunking
│   │   └── chunker_test.go
│   ├── embedder/
│   │   ├── embedder.go          # Embedder interface
│   │   ├── jina.go              # Jina implementation
│   │   ├── openai.go            # OpenAI implementation
│   │   ├── local.go             # Local embeddings
│   │   ├── cache.go             # Embedding cache
│   │   └── embedder_test.go
│   ├── storage/
│   │   ├── storage.go           # Storage interface
│   │   ├── sqlite.go            # SQLite implementation
│   │   ├── schema.sql           # Database schema
│   │   └── storage_test.go
│   ├── indexer/
│   │   ├── indexer.go           # Indexer implementation
│   │   ├── worker.go            # Worker pool
│   │   ├── incremental.go       # Incremental indexing
│   │   └── indexer_test.go
│   ├── searcher/
│   │   ├── searcher.go          # Searcher implementation
│   │   ├── reranker.go          # Reranking logic
│   │   └── searcher_test.go
│   ├── mcp/
│   │   ├── server.go            # MCP server
│   │   ├── tools.go             # Tool handlers
│   │   └── mcp_test.go
│   ├── config/
│   │   ├── config.go            # Configuration
│   │   └── config.yaml
│   └── cli/
│       ├── root.go              # CLI root command
│       ├── index.go             # Index command
│       ├── search.go            # Search command
│       └── mcp.go               # MCP commands
├── pkg/
│   └── api/
│       └── types.go             # Public API types
├── testdata/
│   ├── small/                   # Test codebases
│   ├── medium/
│   └── large/
├── docs/
│   ├── README.md
│   ├── ARCHITECTURE.md
│   ├── USER_GUIDE.md
│   └── API.md
├── scripts/
│   ├── install.sh
│   └── build.sh
├── .github/
│   └── workflows/
│       ├── ci.yml
│       └── release.yml
├── go.mod
├── go.sum
├── Makefile
├── README.md
└── LICENSE
```

---

## Appendix B: Database Schema

See [Component Specification #5 - Storage](#5-storage-component) for complete schema.

---

## Appendix C: API Examples

See [API Definitions](#api-definitions) for complete API documentation.

---

## Appendix D: Performance Benchmarks

**Target Benchmarks:**

| Metric | Small (100 LOC) | Medium (10k LOC) | Large (100k LOC) |
|--------|-----------------|------------------|------------------|
| Initial Index | < 5s | < 30s | < 5min |
| Incremental Index | < 1s | < 10s | < 30s |
| Vector Search | < 100ms | < 200ms | < 500ms |
| Text Search | < 50ms | < 100ms | < 200ms |
| Hybrid Search | < 150ms | < 300ms | < 500ms |
| Memory Usage | < 50MB | < 200MB | < 500MB |
| Database Size | < 1MB | < 10MB | < 100MB |

---

## Appendix E: References

**Documentation:**
- [Go AST Package](https://pkg.go.dev/go/ast)
- [Tree-sitter](https://tree-sitter.github.io/tree-sitter/)
- [Model Context Protocol](https://github.com/modelcontextprotocol)
- [mcp-go SDK](https://github.com/mark3labs/mcp-go)
- [SQLite Vector Extension](https://github.com/asg017/sqlite-vss)
- [Jina AI Embeddings](https://jina.ai/embeddings/)

**Related Projects:**
- [deepcontext-mcp](https://github.com/Wildcard-Official/deepcontext-mcp)
- [mcp-server-tree-sitter](https://github.com/wrale/mcp-server-tree-sitter)

---

## Change Log

| Version | Date | Changes |
|---------|------|---------|
| 1.0.0 | 2025-11-06 | Initial specification |

---

**End of Specification**
