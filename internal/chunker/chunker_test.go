package chunker

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dshills/gocontext-mcp/internal/parser"
	"github.com/dshills/gocontext-mcp/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	c := New()
	assert.NotNil(t, c)
}

func TestChunkFile_SimpleFunction(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.go")

	content := `package testpkg

import "fmt"

// Greet prints a greeting message
func Greet(name string) {
	fmt.Println("Hello, " + name)
}
`

	err := os.WriteFile(testFile, []byte(content), 0644)
	require.NoError(t, err)

	// Parse the file first
	p := parser.New()
	parseResult, err := p.ParseFile(testFile)
	require.NoError(t, err)

	// Chunk the file
	c := New()
	chunks, err := c.ChunkFile(testFile, parseResult, 1)

	require.NoError(t, err)
	assert.NotEmpty(t, chunks)

	// Find the Greet function chunk
	var greetChunk *types.Chunk
	for _, chunk := range chunks {
		if chunk.ChunkType == types.ChunkFunction {
			greetChunk = chunk
			break
		}
	}

	require.NotNil(t, greetChunk)
	assert.Contains(t, greetChunk.Content, "Greet")
	assert.Contains(t, greetChunk.Content, "fmt.Println")
	assert.Contains(t, greetChunk.ContextBefore, "package testpkg")
	assert.Greater(t, greetChunk.TokenCount, 0)
	assert.NotEmpty(t, greetChunk.ContentHash)
}

func TestChunkFile_StructWithMethods(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "struct.go")

	content := `package testpkg

type User struct {
	ID   int
	Name string
}

func (u *User) GetID() int {
	return u.ID
}

func (u *User) SetName(name string) {
	u.Name = name
}
`

	err := os.WriteFile(testFile, []byte(content), 0644)
	require.NoError(t, err)

	p := parser.New()
	parseResult, err := p.ParseFile(testFile)
	require.NoError(t, err)

	c := New()
	chunks, err := c.ChunkFile(testFile, parseResult, 1)

	require.NoError(t, err)
	assert.NotEmpty(t, chunks)

	// Should have chunks for: User struct, GetID method, SetName method
	// Fields are skipped
	var typeChunks, methodChunks int
	for _, chunk := range chunks {
		if chunk.ChunkType == types.ChunkTypeDecl {
			typeChunks++
		}
		if chunk.ChunkType == types.ChunkMethod {
			methodChunks++
		}
	}

	assert.Equal(t, 1, typeChunks)   // User struct
	assert.Equal(t, 2, methodChunks) // GetID and SetName
}

func TestChunkFile_EmptyFile(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "empty.go")

	err := os.WriteFile(testFile, []byte(""), 0644)
	require.NoError(t, err)

	p := parser.New()
	parseResult, err := p.ParseFile(testFile)
	require.NoError(t, err)

	c := New()
	chunks, err := c.ChunkFile(testFile, parseResult, 1)

	require.NoError(t, err)
	// Empty file creates a package chunk (has at least "")
	// Parser returns error so we get an empty parse result which creates a package chunk
	assert.NotEmpty(t, chunks)
}

func TestChunkFile_OnlyPackageDeclaration(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "pkgonly.go")

	content := `package main
`

	err := os.WriteFile(testFile, []byte(content), 0644)
	require.NoError(t, err)

	p := parser.New()
	parseResult, err := p.ParseFile(testFile)
	require.NoError(t, err)

	c := New()
	chunks, err := c.ChunkFile(testFile, parseResult, 1)

	require.NoError(t, err)
	// Should create a package-level chunk
	assert.Len(t, chunks, 1)
	assert.Equal(t, types.ChunkPackage, chunks[0].ChunkType)
}

func TestChunkFile_Constants(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "consts.go")

	content := `package testpkg

const (
	MaxSize = 100
	MinSize = 10
)

const Single = 42
`

	err := os.WriteFile(testFile, []byte(content), 0644)
	require.NoError(t, err)

	p := parser.New()
	parseResult, err := p.ParseFile(testFile)
	require.NoError(t, err)

	c := New()
	chunks, err := c.ChunkFile(testFile, parseResult, 1)

	require.NoError(t, err)
	assert.NotEmpty(t, chunks)

	// Count const chunks
	constChunks := 0
	for _, chunk := range chunks {
		if chunk.ChunkType == types.ChunkConstGroup {
			constChunks++
		}
	}
	assert.Greater(t, constChunks, 0)
}

func TestChunkFile_Variables(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "vars.go")

	content := `package testpkg

var (
	DefaultName = "test"
	DefaultAge  = 25
)

var Single = "value"
`

	err := os.WriteFile(testFile, []byte(content), 0644)
	require.NoError(t, err)

	p := parser.New()
	parseResult, err := p.ParseFile(testFile)
	require.NoError(t, err)

	c := New()
	chunks, err := c.ChunkFile(testFile, parseResult, 1)

	require.NoError(t, err)
	assert.NotEmpty(t, chunks)

	// Count var chunks
	varChunks := 0
	for _, chunk := range chunks {
		if chunk.ChunkType == types.ChunkVarGroup {
			varChunks++
		}
	}
	assert.Greater(t, varChunks, 0)
}

func TestChunkFile_Interface(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "interface.go")

	content := `package testpkg

type Reader interface {
	Read(p []byte) (n int, err error)
	Close() error
}
`

	err := os.WriteFile(testFile, []byte(content), 0644)
	require.NoError(t, err)

	p := parser.New()
	parseResult, err := p.ParseFile(testFile)
	require.NoError(t, err)

	c := New()
	chunks, err := c.ChunkFile(testFile, parseResult, 1)

	require.NoError(t, err)
	assert.NotEmpty(t, chunks)

	// Find the interface chunk
	var interfaceChunk *types.Chunk
	for _, chunk := range chunks {
		if chunk.ChunkType == types.ChunkTypeDecl {
			interfaceChunk = chunk
			break
		}
	}

	require.NotNil(t, interfaceChunk)
	assert.Contains(t, interfaceChunk.Content, "Reader interface")
}

func TestChunkFile_NonExistentFile(t *testing.T) {
	c := New()

	// Create an empty parse result
	parseResult := &types.ParseResult{PackageName: "test"}

	_, err := c.ChunkFile("/nonexistent/file.go", parseResult, 1)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read file")
}

func TestBuildPackageContext(t *testing.T) {
	c := New()

	tests := []struct {
		name          string
		parseResult   *types.ParseResult
		expectedInCtx []string
	}{
		{
			name: "package with imports",
			parseResult: &types.ParseResult{
				PackageName: "main",
				Imports: []types.Import{
					{Path: "fmt", Alias: ""},
					{Path: "strings", Alias: "str"},
				},
			},
			expectedInCtx: []string{"package main", `"fmt"`, `str "strings"`},
		},
		{
			name: "package without imports",
			parseResult: &types.ParseResult{
				PackageName: "test",
				Imports:     []types.Import{},
			},
			expectedInCtx: []string{"package test"},
		},
		{
			name: "empty package name",
			parseResult: &types.ParseResult{
				PackageName: "",
				Imports:     []types.Import{},
			},
			expectedInCtx: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			context := c.buildPackageContext(tt.parseResult, []string{})
			for _, expected := range tt.expectedInCtx {
				assert.Contains(t, context, expected)
			}
		})
	}
}

func TestSymbolKindToChunkType(t *testing.T) {
	c := New()

	tests := []struct {
		kind     types.SymbolKind
		expected types.ChunkType
	}{
		{types.KindFunction, types.ChunkFunction},
		{types.KindMethod, types.ChunkMethod},
		{types.KindStruct, types.ChunkTypeDecl},
		{types.KindInterface, types.ChunkTypeDecl},
		{types.KindType, types.ChunkTypeDecl},
		{types.KindConst, types.ChunkConstGroup},
		{types.KindVar, types.ChunkVarGroup},
		{types.KindField, types.ChunkPackage}, // default case
	}

	for _, tt := range tests {
		t.Run(string(tt.kind), func(t *testing.T) {
			result := c.symbolKindToChunkType(tt.kind)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCreateChunkForSymbol_InvalidPositions(t *testing.T) {
	c := New()

	tests := []struct {
		name   string
		symbol types.Symbol
	}{
		{
			name: "zero start line",
			symbol: types.Symbol{
				Name:  "TestFunc",
				Kind:  types.KindFunction,
				Start: types.Position{Line: 0, Column: 0},
				End:   types.Position{Line: 5, Column: 1},
			},
		},
		{
			name: "zero end line",
			symbol: types.Symbol{
				Name:  "TestFunc",
				Kind:  types.KindFunction,
				Start: types.Position{Line: 1, Column: 0},
				End:   types.Position{Line: 0, Column: 1},
			},
		},
		{
			name: "start line beyond file",
			symbol: types.Symbol{
				Name:  "TestFunc",
				Kind:  types.KindFunction,
				Start: types.Position{Line: 100, Column: 0},
				End:   types.Position{Line: 105, Column: 1},
			},
		},
	}

	lines := []string{"line 1", "line 2", "line 3"}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chunk := c.createChunkForSymbol(&tt.symbol, lines, "", 1)
			assert.Nil(t, chunk)
		})
	}
}

func TestCreatePackageChunk_EmptyLines(t *testing.T) {
	c := New()
	chunk := c.createPackageChunk("test", []string{}, 1)
	assert.Nil(t, chunk)
}

func TestChunkFileWithStrategy_PackageLevel(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.go")

	content := `package testpkg

func A() {}
func B() {}
`

	err := os.WriteFile(testFile, []byte(content), 0644)
	require.NoError(t, err)

	p := parser.New()
	parseResult, err := p.ParseFile(testFile)
	require.NoError(t, err)

	c := New()
	chunks, err := c.ChunkFileWithStrategy(testFile, parseResult, 1, StrategyPackageLevel)

	require.NoError(t, err)
	// Package level strategy should create one chunk for the whole file
	assert.Len(t, chunks, 1)
	assert.Equal(t, types.ChunkPackage, chunks[0].ChunkType)
}

func TestChunkFileWithStrategy_FunctionLevel(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.go")

	content := `package testpkg

func A() {}
func B() {}
`

	err := os.WriteFile(testFile, []byte(content), 0644)
	require.NoError(t, err)

	p := parser.New()
	parseResult, err := p.ParseFile(testFile)
	require.NoError(t, err)

	c := New()
	chunks, err := c.ChunkFileWithStrategy(testFile, parseResult, 1, StrategyFunctionLevel)

	require.NoError(t, err)
	// Function level should create separate chunks
	assert.Len(t, chunks, 2)
}
