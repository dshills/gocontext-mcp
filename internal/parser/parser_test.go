package parser

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dshills/gocontext-mcp/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	p := New()
	assert.NotNil(t, p)
	assert.NotNil(t, p.fset)
}

func TestParseFile_ValidGoFile(t *testing.T) {
	// Create a temporary test file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.go")

	content := `package testpkg

import (
	"fmt"
	"strings"
)

// User represents a user in the system
type User struct {
	ID   int
	Name string
}

// GetName returns the user's name
func (u *User) GetName() string {
	return u.Name
}

// NewUser creates a new user
func NewUser(id int, name string) *User {
	return &User{ID: id, Name: name}
}
`

	err := os.WriteFile(testFile, []byte(content), 0644)
	require.NoError(t, err)

	p := New()
	result, err := p.ParseFile(testFile)

	require.NoError(t, err)
	assert.Equal(t, "testpkg", result.PackageName)
	assert.Len(t, result.Imports, 2)
	assert.Empty(t, result.Errors)

	// Check imports
	importPaths := make(map[string]bool)
	for _, imp := range result.Imports {
		importPaths[imp.Path] = true
	}
	assert.True(t, importPaths["fmt"])
	assert.True(t, importPaths["strings"])

	// Check symbols
	symbolNames := make(map[string]bool)
	for _, sym := range result.Symbols {
		symbolNames[sym.Name] = true
	}
	assert.True(t, symbolNames["User"])
	assert.True(t, symbolNames["GetName"])
	assert.True(t, symbolNames["NewUser"])
}

func TestParseFile_WithImportAlias(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "alias.go")

	content := `package main

import (
	. "fmt"
	str "strings"
	_ "database/sql"
)

func test() {}
`

	err := os.WriteFile(testFile, []byte(content), 0644)
	require.NoError(t, err)

	p := New()
	result, err := p.ParseFile(testFile)

	require.NoError(t, err)
	assert.Len(t, result.Imports, 3)

	// Find specific imports
	aliases := make(map[string]string)
	for _, imp := range result.Imports {
		aliases[imp.Path] = imp.Alias
	}

	assert.Equal(t, ".", aliases["fmt"])
	assert.Equal(t, "str", aliases["strings"])
	assert.Equal(t, "_", aliases["database/sql"])
}

func TestParseFile_SyntaxError(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "invalid.go")

	content := `package main

func incomplete( {
	// Missing closing parenthesis
}
`

	err := os.WriteFile(testFile, []byte(content), 0644)
	require.NoError(t, err)

	p := New()
	result, err := p.ParseFile(testFile)

	// Parser should not return error, but result should have errors
	require.NoError(t, err)
	assert.NotEmpty(t, result.Errors)
	assert.Contains(t, result.Errors[0].Message, "syntax error")
}

func TestParseFile_NonExistentFile(t *testing.T) {
	p := New()
	_, err := p.ParseFile("/nonexistent/file.go")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read file")
}

func TestParseFile_EmptyFile(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "empty.go")

	err := os.WriteFile(testFile, []byte(""), 0644)
	require.NoError(t, err)

	p := New()
	result, err := p.ParseFile(testFile)

	require.NoError(t, err)
	assert.NotEmpty(t, result.Errors) // Empty file is a syntax error
}

func TestParseFile_InterfaceType(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "interface.go")

	content := `package testpkg

// Reader interface for reading data
type Reader interface {
	Read(p []byte) (n int, err error)
	Close() error
}
`

	err := os.WriteFile(testFile, []byte(content), 0644)
	require.NoError(t, err)

	p := New()
	result, err := p.ParseFile(testFile)

	require.NoError(t, err)
	assert.Equal(t, "testpkg", result.PackageName)

	// Find the Reader interface
	var readerSym *types.Symbol
	for i := range result.Symbols {
		if result.Symbols[i].Name == "Reader" {
			readerSym = &result.Symbols[i]
			break
		}
	}

	require.NotNil(t, readerSym)
	assert.Equal(t, types.KindInterface, readerSym.Kind)
}

func TestParseFile_TypeAlias(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "alias.go")

	content := `package testpkg

type MyString = string
type MyInt = int
`

	err := os.WriteFile(testFile, []byte(content), 0644)
	require.NoError(t, err)

	p := New()
	result, err := p.ParseFile(testFile)

	require.NoError(t, err)

	// Check that type aliases are captured
	symbolNames := make(map[string]bool)
	for _, sym := range result.Symbols {
		symbolNames[sym.Name] = true
	}
	assert.True(t, symbolNames["MyString"])
	assert.True(t, symbolNames["MyInt"])
}

func TestParseFile_WithComments(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "comments.go")

	content := `package testpkg

// UserService provides user-related operations.
// It implements the Service interface.
type UserService struct {
	db Database
}

// CreateUser creates a new user in the database.
// Returns an error if the user already exists.
func (s *UserService) CreateUser(name string) error {
	return nil
}
`

	err := os.WriteFile(testFile, []byte(content), 0644)
	require.NoError(t, err)

	p := New()
	result, err := p.ParseFile(testFile)

	require.NoError(t, err)

	// Find UserService
	var userServiceSym *types.Symbol
	for i := range result.Symbols {
		if result.Symbols[i].Name == "UserService" {
			userServiceSym = &result.Symbols[i]
			break
		}
	}

	require.NotNil(t, userServiceSym)
	assert.Contains(t, userServiceSym.DocComment, "provides user-related operations")
}

func TestParseFile_UnexportedSymbols(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "unexported.go")

	content := `package testpkg

// Exported function
func PublicFunc() {}

// unexported function
func privateFunc() {}

// Exported type
type PublicType struct{}

// unexported type
type privateType struct{}
`

	err := os.WriteFile(testFile, []byte(content), 0644)
	require.NoError(t, err)

	p := New()
	result, err := p.ParseFile(testFile)

	require.NoError(t, err)

	// Collect all symbols
	symbolMap := make(map[string]bool)
	for _, sym := range result.Symbols {
		symbolMap[sym.Name] = true
	}

	// Both exported and unexported should be captured
	assert.True(t, symbolMap["PublicFunc"])
	assert.True(t, symbolMap["privateFunc"])
	assert.True(t, symbolMap["PublicType"])
	assert.True(t, symbolMap["privateType"])
}

func TestParseFile_ConstAndVar(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "consts.go")

	content := `package testpkg

const (
	MaxSize = 100
	MinSize = 10
)

var (
	DefaultName = "test"
	DefaultAge  = 25
)

const SingleConst = "value"
var SingleVar = 42
`

	err := os.WriteFile(testFile, []byte(content), 0644)
	require.NoError(t, err)

	p := New()
	result, err := p.ParseFile(testFile)

	require.NoError(t, err)

	// Collect symbol types
	symbolKinds := make(map[string]types.SymbolKind)
	for _, sym := range result.Symbols {
		symbolKinds[sym.Name] = sym.Kind
	}

	// Check constants
	assert.Equal(t, types.KindConst, symbolKinds["MaxSize"])
	assert.Equal(t, types.KindConst, symbolKinds["MinSize"])
	assert.Equal(t, types.KindConst, symbolKinds["SingleConst"])

	// Check variables
	assert.Equal(t, types.KindVar, symbolKinds["DefaultName"])
	assert.Equal(t, types.KindVar, symbolKinds["DefaultAge"])
	assert.Equal(t, types.KindVar, symbolKinds["SingleVar"])
}

func TestExtractImports_NoImports(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "no_imports.go")

	content := `package main

func main() {}
`

	err := os.WriteFile(testFile, []byte(content), 0644)
	require.NoError(t, err)

	p := New()
	result, err := p.ParseFile(testFile)

	require.NoError(t, err)
	assert.Empty(t, result.Imports)
}

// T086: Regression test for parser extracting partial results on syntax errors
// Verifies that ParseFile continues after encountering syntax errors
// Implementation: internal/parser/parser.go (lines 37-68)
func TestParseFile_PartialResultsOnSyntaxError(t *testing.T) {
	t.Run("extract package name despite syntax errors", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFile := filepath.Join(tmpDir, "partial.go")

		// Valid package, valid import, but syntax error in function
		content := `package mypackage

import "fmt"

func BrokenFunc( {
	// Missing closing parenthesis
	fmt.Println("broken")
}

// This function is after the error
func ValidFunc() {
	fmt.Println("valid")
}
`

		err := os.WriteFile(testFile, []byte(content), 0644)
		require.NoError(t, err)

		p := New()
		result, err := p.ParseFile(testFile)

		// Parser does not return error, but records it in result
		require.NoError(t, err, "ParseFile should not return error for syntax errors")

		// Verify error was recorded
		assert.NotEmpty(t, result.Errors, "Syntax error should be recorded")
		assert.Contains(t, result.Errors[0].Message, "syntax error")

		// Partial AST extraction: package name should be extracted
		assert.Equal(t, "mypackage", result.PackageName, "Package name should be extracted")

		// Implementation detail: parser.go:39-43
		// Syntax errors are recorded but processing continues with partial AST
	})

	t.Run("extract valid imports despite syntax errors", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFile := filepath.Join(tmpDir, "imports_partial.go")

		content := `package testpkg

import (
	"fmt"
	"strings"
	"os"
)

func Broken( {
	// Syntax error
}
`

		err := os.WriteFile(testFile, []byte(content), 0644)
		require.NoError(t, err)

		p := New()
		result, err := p.ParseFile(testFile)

		require.NoError(t, err)
		assert.NotEmpty(t, result.Errors, "Should have syntax error")

		// Imports should still be extracted
		assert.Len(t, result.Imports, 3, "All imports should be extracted")

		importPaths := make(map[string]bool)
		for _, imp := range result.Imports {
			importPaths[imp.Path] = true
		}
		assert.True(t, importPaths["fmt"])
		assert.True(t, importPaths["strings"])
		assert.True(t, importPaths["os"])

		// Implementation: parser.go:52-53 extracts imports from partial AST
	})

	t.Run("extract valid symbols before error", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFile := filepath.Join(tmpDir, "symbols_partial.go")

		content := `package testpkg

// ValidStruct should be extracted
type ValidStruct struct {
	ID int
}

// ValidFunc should be extracted
func ValidFunc() {}

func BrokenFunc( {
	// Syntax error here
}

// Note: Symbols after error may not be extracted depending on parser behavior
`

		err := os.WriteFile(testFile, []byte(content), 0644)
		require.NoError(t, err)

		p := New()
		result, err := p.ParseFile(testFile)

		require.NoError(t, err)
		assert.NotEmpty(t, result.Errors, "Should have syntax error")

		// Valid symbols before error should be extracted
		symbolNames := make(map[string]bool)
		for _, sym := range result.Symbols {
			symbolNames[sym.Name] = true
		}

		// These symbols are before the syntax error and should be extracted
		assert.True(t, symbolNames["ValidStruct"], "Struct before error should be extracted")
		assert.True(t, symbolNames["ValidFunc"], "Function before error should be extracted")

		// Implementation: parser.go:55-66 uses AST traversal on partial AST
		// ast.Inspect walks the partial tree and extracts valid nodes
	})

	t.Run("multiple syntax errors recorded", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFile := filepath.Join(tmpDir, "multiple_errors.go")

		// Multiple syntax errors
		content := `package testpkg

func First( {
}

func Second( {
}
`

		err := os.WriteFile(testFile, []byte(content), 0644)
		require.NoError(t, err)

		p := New()
		result, err := p.ParseFile(testFile)

		require.NoError(t, err, "ParseFile should not return error")

		// Parser records errors but continues
		assert.NotEmpty(t, result.Errors, "Errors should be recorded")

		// Package name still extracted
		assert.Equal(t, "testpkg", result.PackageName)

		// Implementation: parser.go:40 records error via result.AddError
	})

	t.Run("partial AST inspection continues", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFile := filepath.Join(tmpDir, "ast_partial.go")

		content := `package testpkg

type ValidType struct {
	Name string
}

func Broken( {
}
`

		err := os.WriteFile(testFile, []byte(content), 0644)
		require.NoError(t, err)

		p := New()
		result, err := p.ParseFile(testFile)

		require.NoError(t, err)
		assert.NotEmpty(t, result.Errors)

		// AST inspection at parser.go:64 should process available nodes
		// Even with syntax errors, valid declarations are found

		var foundValidType bool
		for _, sym := range result.Symbols {
			if sym.Name == "ValidType" {
				foundValidType = true
				assert.Equal(t, types.KindStruct, sym.Kind)
			}
		}

		assert.True(t, foundValidType, "Valid type declaration should be found in partial AST")
	})

	t.Run("error details are preserved", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFile := filepath.Join(tmpDir, "error_details.go")

		content := `package testpkg

func Invalid( {
	// Error on line 3
}
`

		err := os.WriteFile(testFile, []byte(content), 0644)
		require.NoError(t, err)

		p := New()
		result, err := p.ParseFile(testFile)

		require.NoError(t, err)
		require.NotEmpty(t, result.Errors)

		// Verify error information is recorded
		parseError := result.Errors[0]
		assert.Equal(t, testFile, parseError.File, "Error file should be set")
		assert.Contains(t, parseError.Message, "syntax error", "Error message should describe the issue")

		// Line/column may be 0 if not available from parser
		// Implementation: parser.go:40 passes 0, 0 for line/column
	})
}

// T086b: Test that parser handles various syntax error scenarios
func TestParseFile_SyntaxErrorScenarios(t *testing.T) {
	tests := []struct {
		name             string
		content          string
		expectPackage    string
		expectSymbols    []string // Symbols that should be extracted
		expectImportPath string   // At least one import that should be extracted
	}{
		{
			name: "missing closing brace",
			content: `package test

type Config struct {
	Value string
// Missing closing brace

func Valid() {}
`,
			expectPackage: "test",
			expectSymbols: []string{"Config"},
		},
		{
			name: "invalid type declaration",
			content: `package mytest

import "fmt"

type Invalid struct {
	Bad syntax here
}

type Valid struct {
	Good string
}
`,
			expectPackage:    "mytest",
			expectImportPath: "fmt",
		},
		{
			name: "unclosed string literal",
			content: `package functest

const BrokenString = "unclosed

func ValidFunc() {
	println("ok")
}
`,
			expectPackage: "functest",
			expectSymbols: []string{"ValidFunc"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			testFile := filepath.Join(tmpDir, "test.go")

			err := os.WriteFile(testFile, []byte(tt.content), 0644)
			require.NoError(t, err)

			p := New()
			result, err := p.ParseFile(testFile)

			// Should not return error
			require.NoError(t, err)

			// Should record the syntax error
			assert.NotEmpty(t, result.Errors, "Syntax error should be recorded")

			// Package name should be extracted
			if tt.expectPackage != "" {
				assert.Equal(t, tt.expectPackage, result.PackageName,
					"Package name should be extracted despite syntax errors")
			}

			// Expected symbols should be extracted
			if len(tt.expectSymbols) > 0 {
				symbolNames := make(map[string]bool)
				for _, sym := range result.Symbols {
					symbolNames[sym.Name] = true
				}
				for _, expectedSym := range tt.expectSymbols {
					assert.True(t, symbolNames[expectedSym],
						"Symbol %s should be extracted from partial AST", expectedSym)
				}
			}

			// Expected import should be extracted
			if tt.expectImportPath != "" {
				importPaths := make(map[string]bool)
				for _, imp := range result.Imports {
					importPaths[imp.Path] = true
				}
				assert.True(t, importPaths[tt.expectImportPath],
					"Import %s should be extracted from partial AST", tt.expectImportPath)
			}
		})
	}
}
