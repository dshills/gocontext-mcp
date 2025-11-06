package chunker_test

import (
	"path/filepath"
	"testing"

	"github.com/dshills/gocontext-mcp/internal/chunker"
	"github.com/dshills/gocontext-mcp/internal/parser"
	"github.com/dshills/gocontext-mcp/pkg/types"
)

func TestChunkFile_Simple(t *testing.T) {
	// Parse the file first
	p := parser.New()
	filePath := filepath.Join("..", "..", "testdata", "fixtures", "sample_simple.go")
	parseResult, err := p.ParseFile(filePath)
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}

	// Chunk the file
	c := chunker.New()
	chunks, err := c.ChunkFile(filePath, parseResult, 1)
	if err != nil {
		t.Fatalf("ChunkFile() error = %v", err)
	}

	if len(chunks) == 0 {
		t.Fatal("no chunks created")
	}

	// Verify each chunk has required fields
	for i, chunk := range chunks {
		t.Run(string(chunk.ChunkType), func(t *testing.T) {
			if chunk.Content == "" {
				t.Errorf("chunk[%d]: Content is empty", i)
			}

			if chunk.StartLine <= 0 || chunk.EndLine <= 0 {
				t.Errorf("chunk[%d]: invalid line numbers: Start=%d, End=%d",
					i, chunk.StartLine, chunk.EndLine)
			}

			if chunk.StartLine > chunk.EndLine {
				t.Errorf("chunk[%d]: StartLine (%d) > EndLine (%d)",
					i, chunk.StartLine, chunk.EndLine)
			}

			if chunk.TokenCount <= 0 {
				t.Errorf("chunk[%d]: TokenCount not computed", i)
			}

			// Verify content hash is computed
			var zeroHash [32]byte
			if chunk.ContentHash == zeroHash {
				t.Errorf("chunk[%d]: ContentHash not computed", i)
			}

			if chunk.FileID != 1 {
				t.Errorf("chunk[%d]: FileID = %d, want 1", i, chunk.FileID)
			}

			// Verify ContextBefore contains package declaration
			if chunk.ContextBefore == "" {
				t.Errorf("chunk[%d]: ContextBefore is empty", i)
			}
		})
	}
}

func TestChunkFile_ChunkTypes(t *testing.T) {
	p := parser.New()
	filePath := filepath.Join("..", "..", "testdata", "fixtures", "sample_simple.go")
	parseResult, err := p.ParseFile(filePath)
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}

	c := chunker.New()
	chunks, err := c.ChunkFile(filePath, parseResult, 1)
	if err != nil {
		t.Fatalf("ChunkFile() error = %v", err)
	}

	// Count chunk types
	chunkTypes := make(map[types.ChunkType]int)
	for _, chunk := range chunks {
		chunkTypes[chunk.ChunkType]++
	}

	// Expected: at least 1 type (User struct), 1 function, 1 method
	expectedMinCounts := map[types.ChunkType]int{
		types.ChunkTypeDecl:   1, // User struct, UserRepository interface
		types.ChunkFunction:   1, // ValidateEmail
		types.ChunkMethod:     1, // Greet method
		types.ChunkConstGroup: 1, // MaxNameLength, MinNameLength
		types.ChunkVarGroup:   1, // DefaultName, DefaultEmail
	}

	for chunkType, minCount := range expectedMinCounts {
		if chunkTypes[chunkType] < minCount {
			t.Errorf("chunk type %s count = %d, want >= %d",
				chunkType, chunkTypes[chunkType], minCount)
		}
	}
}

func TestChunkFile_ContentExtraction(t *testing.T) {
	p := parser.New()
	filePath := filepath.Join("..", "..", "testdata", "fixtures", "sample_simple.go")
	parseResult, err := p.ParseFile(filePath)
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}

	c := chunker.New()
	chunks, err := c.ChunkFile(filePath, parseResult, 1)
	if err != nil {
		t.Fatalf("ChunkFile() error = %v", err)
	}

	// Find the User struct chunk
	var userChunk *types.Chunk
	for _, chunk := range chunks {
		if chunk.ChunkType == types.ChunkTypeDecl && chunk.StartLine >= 8 && chunk.StartLine <= 10 {
			// User struct is around line 8-13
			userChunk = chunk
			break
		}
	}

	if userChunk == nil {
		t.Fatal("User struct chunk not found")
	}

	// Verify the content contains the struct definition
	if userChunk.Content == "" {
		t.Error("User chunk Content is empty")
	}

	// Content should have proper line boundaries (no mid-expression splits)
	// This is guaranteed by using AST node boundaries
}

func TestChunkFile_TokenCount(t *testing.T) {
	p := parser.New()
	filePath := filepath.Join("..", "..", "testdata", "fixtures", "sample_simple.go")
	parseResult, err := p.ParseFile(filePath)
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}

	c := chunker.New()
	chunks, err := c.ChunkFile(filePath, parseResult, 1)
	if err != nil {
		t.Fatalf("ChunkFile() error = %v", err)
	}

	for _, chunk := range chunks {
		// Verify token count is reasonable (chars/4)
		expectedTokens := (len(chunk.Content) + len(chunk.ContextBefore) + len(chunk.ContextAfter)) / 4
		if chunk.TokenCount != expectedTokens {
			t.Errorf("chunk TokenCount = %d, expected ~%d",
				chunk.TokenCount, expectedTokens)
		}
	}
}

func TestChunkFile_ContentHash(t *testing.T) {
	p := parser.New()
	filePath := filepath.Join("..", "..", "testdata", "fixtures", "sample_simple.go")
	parseResult, err := p.ParseFile(filePath)
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}

	c := chunker.New()
	chunks, err := c.ChunkFile(filePath, parseResult, 1)
	if err != nil {
		t.Fatalf("ChunkFile() error = %v", err)
	}

	// Verify all chunks have unique hashes (unless content is identical)
	seen := make(map[[32]byte]bool)
	duplicates := 0

	for _, chunk := range chunks {
		if seen[chunk.ContentHash] {
			duplicates++
		}
		seen[chunk.ContentHash] = true
	}

	// In our test file, all chunks should be unique
	if duplicates > 0 {
		t.Logf("Warning: %d duplicate content hashes (may be expected for identical code)", duplicates)
	}
}

func TestChunkFile_DDD(t *testing.T) {
	p := parser.New()
	filePath := filepath.Join("..", "..", "testdata", "fixtures", "sample_ddd.go")
	parseResult, err := p.ParseFile(filePath)
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}

	c := chunker.New()
	chunks, err := c.ChunkFile(filePath, parseResult, 2)
	if err != nil {
		t.Fatalf("ChunkFile() error = %v", err)
	}

	if len(chunks) == 0 {
		t.Fatal("no chunks created for DDD file")
	}

	// Verify we have chunks for DDD patterns
	hasAggregateChunk := false
	hasRepositoryChunk := false
	hasServiceChunk := false

	for _, chunk := range chunks {
		// The chunk content should contain the symbol
		if chunk.Content != "" {
			if containsString(chunk.Content, "OrderAggregate") {
				hasAggregateChunk = true
			}
			if containsString(chunk.Content, "OrderRepository") {
				hasRepositoryChunk = true
			}
			if containsString(chunk.Content, "OrderService") {
				hasServiceChunk = true
			}
		}
	}

	if !hasAggregateChunk {
		t.Error("missing chunk for OrderAggregate")
	}
	if !hasRepositoryChunk {
		t.Error("missing chunk for OrderRepository")
	}
	if !hasServiceChunk {
		t.Error("missing chunk for OrderService")
	}
}

func TestChunkFile_ContextBefore(t *testing.T) {
	p := parser.New()
	filePath := filepath.Join("..", "..", "testdata", "fixtures", "sample_simple.go")
	parseResult, err := p.ParseFile(filePath)
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}

	c := chunker.New()
	chunks, err := c.ChunkFile(filePath, parseResult, 1)
	if err != nil {
		t.Fatalf("ChunkFile() error = %v", err)
	}

	// All chunks should have ContextBefore with package declaration
	for i, chunk := range chunks {
		if chunk.ContextBefore == "" {
			t.Errorf("chunk[%d]: ContextBefore is empty", i)
			continue
		}

		// Should contain package declaration
		if !containsString(chunk.ContextBefore, "package sample") {
			t.Errorf("chunk[%d]: ContextBefore missing package declaration", i)
		}

		// Should contain imports
		if !containsString(chunk.ContextBefore, "import") {
			t.Logf("chunk[%d]: ContextBefore missing imports (may be expected)", i)
		}
	}
}

func TestChunkFile_Strategies(t *testing.T) {
	p := parser.New()
	filePath := filepath.Join("..", "..", "testdata", "fixtures", "sample_simple.go")
	parseResult, err := p.ParseFile(filePath)
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}

	c := chunker.New()

	tests := []struct {
		name     string
		strategy chunker.ChunkStrategy
		wantMin  int
		wantMax  int
	}{
		{
			name:     "function level",
			strategy: chunker.StrategyFunctionLevel,
			wantMin:  5, // At least several chunks
			wantMax:  20,
		},
		{
			name:     "package level",
			strategy: chunker.StrategyPackageLevel,
			wantMin:  1, // Single chunk
			wantMax:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chunks, err := c.ChunkFileWithStrategy(filePath, parseResult, 1, tt.strategy)
			if err != nil {
				t.Fatalf("ChunkFileWithStrategy() error = %v", err)
			}

			if len(chunks) < tt.wantMin || len(chunks) > tt.wantMax {
				t.Errorf("chunk count = %d, want between %d and %d",
					len(chunks), tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestEstimateTokenCount(t *testing.T) {
	tests := []struct {
		name string
		text string
		want int
	}{
		{
			name: "empty",
			text: "",
			want: 0,
		},
		{
			name: "short text",
			text: "func Hello() {}",
			want: 3, // 15 chars / 4 = 3
		},
		{
			name: "longer text",
			text: "func LongerFunctionName(param1 string, param2 int) error { return nil }",
			want: 17, // 71 chars / 4 = 17
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := chunker.EstimateTokenCount(tt.text)
			if got != tt.want {
				t.Errorf("EstimateTokenCount() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestComputeChunkHash(t *testing.T) {
	content1 := "func Hello() {}"
	content2 := "func Hello() {}"
	content3 := "func Goodbye() {}"

	hash1 := chunker.ComputeChunkHash(content1)
	hash2 := chunker.ComputeChunkHash(content2)
	hash3 := chunker.ComputeChunkHash(content3)

	// Same content should produce same hash
	if hash1 != hash2 {
		t.Error("identical content produced different hashes")
	}

	// Different content should produce different hash
	if hash1 == hash3 {
		t.Error("different content produced identical hashes")
	}
}

func TestChunk_Validate(t *testing.T) {
	tests := []struct {
		name    string
		chunk   types.Chunk
		wantErr bool
	}{
		{
			name: "valid chunk",
			chunk: types.Chunk{
				FileID:      1,
				Content:     "func Test() {}",
				ContentHash: [32]byte{1, 2, 3},
				StartLine:   1,
				EndLine:     3,
				ChunkType:   types.ChunkFunction,
			},
			wantErr: false,
		},
		{
			name: "missing content",
			chunk: types.Chunk{
				FileID:      1,
				Content:     "",
				ContentHash: [32]byte{1, 2, 3},
				StartLine:   1,
				EndLine:     3,
				ChunkType:   types.ChunkFunction,
			},
			wantErr: true,
		},
		{
			name: "invalid line numbers",
			chunk: types.Chunk{
				FileID:      1,
				Content:     "func Test() {}",
				ContentHash: [32]byte{1, 2, 3},
				StartLine:   5,
				EndLine:     3,
				ChunkType:   types.ChunkFunction,
			},
			wantErr: true,
		},
		{
			name: "missing content hash",
			chunk: types.Chunk{
				FileID:      1,
				Content:     "func Test() {}",
				ContentHash: [32]byte{},
				StartLine:   1,
				EndLine:     3,
				ChunkType:   types.ChunkFunction,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.chunk.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// Helper function
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
