package types

import (
	"crypto/sha256"
	"errors"
)

// ChunkType represents the type of code chunk
type ChunkType string

const (
	ChunkFunction   ChunkType = "function"
	ChunkTypeDecl   ChunkType = "type"
	ChunkMethod     ChunkType = "method"
	ChunkPackage    ChunkType = "package"
	ChunkConstGroup ChunkType = "const_group"
	ChunkVarGroup   ChunkType = "var_group"
)

// Chunk represents a semantically meaningful code section for embedding and search
type Chunk struct {
	// Identification
	ID       int64
	FileID   int64
	SymbolID *int64 // Nullable - package-level chunks may not have a symbol

	// Content
	Content       string
	ContentHash   [32]byte // SHA-256 hash for deduplication
	TokenCount    int
	ContextBefore string // Package, imports, related declarations
	ContextAfter  string // Related functions, types

	// Location
	StartLine int
	EndLine   int

	// Metadata
	ChunkType ChunkType
}

// ValidateContent checks if the chunk content is valid
func (c *Chunk) ValidateContent() error {
	if c.Content == "" {
		return errors.New("chunk content cannot be empty")
	}

	if c.StartLine <= 0 || c.EndLine <= 0 {
		return errors.New("line numbers must be positive")
	}

	if c.StartLine > c.EndLine {
		return errors.New("start line must be before or equal to end line")
	}

	return nil
}

// ComputeTokenCount estimates the number of tokens in the chunk
// Uses a simple heuristic: characters / 4
func (c *Chunk) ComputeTokenCount() int {
	// Simple heuristic: average English word is ~4 chars, code tokens similar
	// For more accuracy, could use tiktoken library
	totalChars := len(c.Content) + len(c.ContextBefore) + len(c.ContextAfter)
	c.TokenCount = totalChars / 4
	return c.TokenCount
}

// ComputeContentHash computes the SHA-256 hash of the chunk content
func (c *Chunk) ComputeContentHash() {
	c.ContentHash = sha256.Sum256([]byte(c.Content))
}

// ValidateChunkType checks if the chunk type is valid
func (c *Chunk) ValidateChunkType() error {
	switch c.ChunkType {
	case ChunkFunction, ChunkTypeDecl, ChunkMethod, ChunkPackage, ChunkConstGroup, ChunkVarGroup:
		return nil
	default:
		return errors.New("invalid chunk type")
	}
}

// Validate performs comprehensive validation of the chunk
func (c *Chunk) Validate() error {
	if err := c.ValidateContent(); err != nil {
		return err
	}

	if err := c.ValidateChunkType(); err != nil {
		return err
	}

	if c.FileID == 0 {
		return errors.New("file ID is required")
	}

	// Verify content hash is computed
	var zeroHash [32]byte
	if c.ContentHash == zeroHash {
		return errors.New("content hash must be computed")
	}

	return nil
}

// FullContent returns the complete content including context
func (c *Chunk) FullContent() string {
	result := ""
	if c.ContextBefore != "" {
		result += c.ContextBefore + "\n\n"
	}
	result += c.Content
	if c.ContextAfter != "" {
		result += "\n\n" + c.ContextAfter
	}
	return result
}
