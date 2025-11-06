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
