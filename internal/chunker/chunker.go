package chunker

import (
	"crypto/sha256"
	"fmt"
	"os"
	"strings"

	"github.com/dshills/gocontext-mcp/pkg/types"
)

const (
	// MaxTokensPerChunk is the target maximum token count per chunk
	MaxTokensPerChunk = 1000

	// TokensPerChar is the heuristic for estimating tokens (chars/4)
	TokensPerChar = 4
)

// Chunker creates semantic code chunks from parsed Go files
type Chunker struct{}

// New creates a new Chunker instance
func New() *Chunker {
	return &Chunker{}
}

// ChunkFile creates semantic chunks from a Go source file with its parse results
func (c *Chunker) ChunkFile(filePath string, parseResult *types.ParseResult, fileID int64) ([]*types.Chunk, error) {
	// Read file content
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	// Use the parseResult which already has symbol positions from the parser
	// Don't re-parse - the parser may have handled syntax errors gracefully
	// and we can still create chunks from the symbols that were extracted

	// Split content into lines for extraction
	lines := strings.Split(string(content), "\n")

	// Build context information
	contextBefore := c.buildPackageContext(parseResult, lines)

	chunks := make([]*types.Chunk, 0)

	// Create chunks for each symbol
	for i := range parseResult.Symbols {
		sym := &parseResult.Symbols[i]
		// Skip fields - they're included in their parent struct chunks
		if sym.Kind == types.KindField {
			continue
		}

		chunk := c.createChunkForSymbol(sym, lines, contextBefore, fileID)
		if chunk != nil {
			chunks = append(chunks, chunk)
		}
	}

	// If no symbols were found, create a package-level chunk
	if len(chunks) == 0 && len(lines) > 0 {
		chunk := c.createPackageChunk(parseResult.PackageName, lines, fileID)
		if chunk != nil {
			chunks = append(chunks, chunk)
		}
	}

	return chunks, nil
}

// createChunkForSymbol creates a chunk for a specific symbol
func (c *Chunker) createChunkForSymbol(sym *types.Symbol, lines []string, contextBefore string, fileID int64) *types.Chunk {
	// Extract the symbol's content based on line numbers
	if sym.Start.Line <= 0 || sym.End.Line <= 0 || sym.Start.Line > len(lines) {
		return nil
	}

	// Adjust for 0-based indexing
	startIdx := sym.Start.Line - 1
	endIdx := sym.End.Line
	if endIdx > len(lines) {
		endIdx = len(lines)
	}

	content := strings.Join(lines[startIdx:endIdx], "\n")

	// Determine chunk type
	chunkType := c.symbolKindToChunkType(sym.Kind)

	// Create the chunk
	chunk := &types.Chunk{
		FileID:        fileID,
		Content:       content,
		ContextBefore: contextBefore,
		StartLine:     sym.Start.Line,
		EndLine:       sym.End.Line,
		ChunkType:     chunkType,
	}

	// Compute token count and hash
	chunk.ComputeTokenCount()
	chunk.ComputeContentHash()

	// Note: Future enhancement - split oversized chunks (>MaxTokensPerChunk)
	// at logical boundaries within functions using more sophisticated AST analysis

	return chunk
}

// createPackageChunk creates a package-level chunk when no symbols are found
func (c *Chunker) createPackageChunk(packageName string, lines []string, fileID int64) *types.Chunk {
	if len(lines) == 0 {
		return nil
	}

	content := strings.Join(lines, "\n")

	chunk := &types.Chunk{
		FileID:    fileID,
		Content:   content,
		StartLine: 1,
		EndLine:   len(lines),
		ChunkType: types.ChunkPackage,
	}

	chunk.ComputeTokenCount()
	chunk.ComputeContentHash()

	return chunk
}

// buildPackageContext builds the context information (package + imports)
func (c *Chunker) buildPackageContext(parseResult *types.ParseResult, lines []string) string {
	var context strings.Builder

	// Add package declaration
	if parseResult.PackageName != "" {
		context.WriteString(fmt.Sprintf("package %s\n\n", parseResult.PackageName))
	}

	// Add imports if present
	if len(parseResult.Imports) > 0 {
		context.WriteString("import (\n")
		for _, imp := range parseResult.Imports {
			if imp.Alias != "" {
				context.WriteString(fmt.Sprintf("\t%s \"%s\"\n", imp.Alias, imp.Path))
			} else {
				context.WriteString(fmt.Sprintf("\t\"%s\"\n", imp.Path))
			}
		}
		context.WriteString(")\n")
	}

	return context.String()
}

// symbolKindToChunkType maps symbol kinds to chunk types
func (c *Chunker) symbolKindToChunkType(kind types.SymbolKind) types.ChunkType {
	switch kind {
	case types.KindFunction:
		return types.ChunkFunction
	case types.KindMethod:
		return types.ChunkMethod
	case types.KindStruct, types.KindInterface, types.KindType:
		return types.ChunkTypeDecl
	case types.KindConst:
		return types.ChunkConstGroup
	case types.KindVar:
		return types.ChunkVarGroup
	default:
		return types.ChunkPackage
	}
}

// ChunkFileWithStrategies provides more control over chunking strategies
type ChunkStrategy int

const (
	// StrategyFunctionLevel creates one chunk per function/method
	StrategyFunctionLevel ChunkStrategy = iota
	// StrategyTypeLevel creates chunks for types with their methods
	StrategyTypeLevel
	// StrategyPackageLevel creates a single chunk for the entire file
	StrategyPackageLevel
)

// ChunkFileWithStrategy creates chunks using a specific strategy
func (c *Chunker) ChunkFileWithStrategy(filePath string, parseResult *types.ParseResult, fileID int64, strategy ChunkStrategy) ([]*types.Chunk, error) {
	switch strategy {
	case StrategyFunctionLevel:
		return c.ChunkFile(filePath, parseResult, fileID)
	case StrategyPackageLevel:
		return c.chunkFilePackageLevel(filePath, parseResult, fileID)
	default:
		return c.ChunkFile(filePath, parseResult, fileID)
	}
}

// chunkFilePackageLevel creates a single chunk for the entire file
func (c *Chunker) chunkFilePackageLevel(filePath string, parseResult *types.ParseResult, fileID int64) ([]*types.Chunk, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	lines := strings.Split(string(content), "\n")
	chunk := c.createPackageChunk(parseResult.PackageName, lines, fileID)
	if chunk == nil {
		return nil, nil
	}

	return []*types.Chunk{chunk}, nil
}

// SplitOversizedChunk splits a chunk that exceeds MaxTokensPerChunk
// This is a placeholder for future enhancement with more sophisticated splitting
func (c *Chunker) SplitOversizedChunk(chunk *types.Chunk) ([]*types.Chunk, error) {
	if chunk.TokenCount <= MaxTokensPerChunk {
		return []*types.Chunk{chunk}, nil
	}

	// For now, we return the original chunk
	// Future enhancement: parse the chunk content and split at logical boundaries
	// (e.g., between statements, at the start of loops, etc.)
	return []*types.Chunk{chunk}, nil
}

// ExtractRelatedContext finds related symbols that provide context for a chunk
// This can be used to enhance ContextAfter field
func (c *Chunker) ExtractRelatedContext(sym *types.Symbol, allSymbols []types.Symbol) string {
	var related []string

	// For methods, find the receiver type
	if sym.Kind == types.KindMethod && sym.Receiver != "" {
		for i := range allSymbols {
			s := &allSymbols[i]
			if s.Kind == types.KindStruct && s.Name == sym.Receiver {
				related = append(related, fmt.Sprintf("// Receiver: %s", s.Signature))
				break
			}
		}
	}

	// For types, find related methods
	if sym.Kind == types.KindStruct {
		for i := range allSymbols {
			s := &allSymbols[i]
			if s.Kind == types.KindMethod && s.Receiver == sym.Name {
				related = append(related, fmt.Sprintf("// Method: %s", s.Signature))
			}
		}
	}

	if len(related) == 0 {
		return ""
	}

	return strings.Join(related, "\n")
}

// ComputeChunkHash computes the SHA-256 hash for a chunk's content
func ComputeChunkHash(content string) [32]byte {
	return sha256.Sum256([]byte(content))
}

// EstimateTokenCount estimates the number of tokens in a string
func EstimateTokenCount(text string) int {
	return len(text) / TokensPerChar
}
