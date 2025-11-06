package types

// ParseResult represents the output of parsing a Go source file
type ParseResult struct {
	// Extracted data
	Symbols     []Symbol
	Imports     []Import
	PackageName string

	// Errors encountered during parsing
	Errors []ParseError
}

// Import represents an import statement in a Go file
type Import struct {
	Path  string // Import path (e.g., "github.com/pkg/errors")
	Alias string // Import alias if present (e.g., ".")
}

// ParseError represents an error that occurred during parsing
type ParseError struct {
	File    string
	Line    int
	Column  int
	Message string
}

// Error implements the error interface
func (pe *ParseError) Error() string {
	return pe.Message
}

// HasErrors returns true if any parsing errors occurred
func (pr *ParseResult) HasErrors() bool {
	return len(pr.Errors) > 0
}

// AddError adds a parsing error to the result
func (pr *ParseResult) AddError(file string, line, col int, msg string) {
	pr.Errors = append(pr.Errors, ParseError{
		File:    file,
		Line:    line,
		Column:  col,
		Message: msg,
	})
}
