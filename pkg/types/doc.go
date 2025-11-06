// Package types provides shared type definitions for the GoContext MCP server.
//
// This package defines domain types used across multiple components of GoContext,
// including symbols, chunks, parse results, and search results.
//
// # Core Types
//
// Symbol represents a Go language construct (function, method, type, etc.)
// extracted from source code via AST parsing:
//
//	symbol := &types.Symbol{
//	    Name:      "ParseFile",
//	    Kind:      types.KindFunction,
//	    Package:   "parser",
//	    Signature: "func ParseFile(path string) (*ParseResult, error)",
//	}
//
// Chunk represents a semantic code section for embedding and search:
//
//	chunk := &types.Chunk{
//	    Content:       functionBody,
//	    ContextBefore: imports,
//	    ContextAfter:  relatedFunctions,
//	    ChunkType:     types.ChunkFunction,
//	}
//
// # Domain-Driven Design (DDD) Pattern Detection
//
// Symbol types include flags for detecting DDD patterns based on naming conventions:
//
//	symbol.IsRepository  // "*Repository" suffix
//	symbol.IsService     // "*Service" suffix
//	symbol.IsEntity      // "*Entity" suffix or has "ID" field
//	symbol.IsAggregate   // "*Aggregate" suffix
//
// These flags enable architectural pattern queries:
//
//	// Find all repository implementations
//	SELECT * FROM symbols WHERE is_repository = 1
//
// # Validation
//
// All domain types implement validation methods to ensure data integrity:
//
//	if err := symbol.Validate(); err != nil {
//	    log.Fatal(err)
//	}
//
//	if err := chunk.Validate(); err != nil {
//	    log.Fatal(err)
//	}
//
// # Search Results
//
// SearchResult combines symbol metadata with relevance scoring:
//
//	result := &types.SearchResult{
//	    ChunkID:        123,
//	    Rank:           1,
//	    RelevanceScore: 0.92,
//	    Symbol:         symbol,
//	    Content:        chunkContent,
//	}
//
// Relevance scores are normalized to [0, 1] range, with higher values indicating
// better matches.
package types
