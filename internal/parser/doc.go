// Package parser extracts symbols and metadata from Go source files using AST parsing.
//
// The parser leverages Go's standard library (go/parser, go/ast, go/token) to accurately
// extract functions, methods, types, and other language constructs from Go source code.
//
// # Basic Usage
//
//	p := parser.New()
//	result, err := p.ParseFile("/path/to/file.go")
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	for _, symbol := range result.Symbols {
//	    fmt.Printf("Found %s: %s\n", symbol.Kind, symbol.Name)
//	}
//
// # Features
//
// Symbol extraction includes:
//   - Functions and methods (with receiver types)
//   - Structs, interfaces, and type aliases
//   - Constants and variables
//   - Documentation comments
//   - Exported vs unexported scope
//   - Precise source positions (line/column)
//
// # Domain-Driven Design (DDD) Pattern Detection
//
// The parser automatically detects common DDD patterns based on naming conventions:
//
//	symbol.IsRepository  // "*Repository" suffix
//	symbol.IsService     // "*Service" suffix
//	symbol.IsEntity      // "*Entity" suffix or struct with "ID" field
//	symbol.IsAggregate   // "*Aggregate" suffix
//	symbol.IsCommand     // "*Command" suffix (CQRS)
//	symbol.IsQuery       // "*Query" suffix (CQRS)
//	symbol.IsHandler     // "*Handler" suffix (CQRS)
//
// # Error Handling
//
// The parser handles syntax errors gracefully:
//
//	result, err := p.ParseFile("broken.go")
//	// err is nil even for syntax errors
//
//	if result.HasErrors() {
//	    for _, parseErr := range result.Errors {
//	        fmt.Printf("Parse error: %v\n", parseErr)
//	    }
//	}
//
//	// Partial results still returned for valid symbols
//	fmt.Printf("Extracted %d symbols despite errors\n", len(result.Symbols))
//
// This allows indexing to continue even when some files have syntax errors.
//
// # Performance
//
// The parser is optimized for batch processing:
//   - Reuses token.FileSet for memory efficiency
//   - Extracts all symbols in a single AST traversal
//   - Targets 100+ files/second on modern hardware
package parser
