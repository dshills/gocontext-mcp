package parser_test

import (
	"path/filepath"
	"testing"

	"github.com/dshills/gocontext-mcp/internal/parser"
	"github.com/dshills/gocontext-mcp/pkg/types"
)

func TestParseFile_Simple(t *testing.T) {
	p := parser.New()
	filePath := filepath.Join("..", "..", "testdata", "fixtures", "sample_simple.go")

	result, err := p.ParseFile(filePath)
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}

	if result == nil {
		t.Fatal("ParseFile() returned nil result")
	}

	// Check package name
	if result.PackageName != "sample" {
		t.Errorf("PackageName = %q, want %q", result.PackageName, "sample")
	}

	// Check imports
	if len(result.Imports) != 2 {
		t.Errorf("len(Imports) = %d, want 2", len(result.Imports))
	}

	expectedImports := map[string]bool{
		"context": true,
		"fmt":     true,
	}

	for _, imp := range result.Imports {
		if !expectedImports[imp.Path] {
			t.Errorf("unexpected import: %q", imp.Path)
		}
	}

	// Check symbols
	if len(result.Symbols) == 0 {
		t.Fatal("no symbols extracted")
	}

	// Count symbols by kind
	symbolCounts := make(map[types.SymbolKind]int)
	for _, sym := range result.Symbols {
		symbolCounts[sym.Kind]++
	}

	// Expected: 1 struct (User), 1 interface (UserRepository), 1 method (Greet),
	// 1 function (ValidateEmail), 2 consts, 2 vars, 4 fields (ID, Name, Email, CreatedAt)
	expectedCounts := map[types.SymbolKind]int{
		types.KindStruct:    1,
		types.KindInterface: 1,
		types.KindMethod:    1,
		types.KindFunction:  1,
		types.KindConst:     2,
		types.KindVar:       2,
		types.KindField:     4,
	}

	for kind, expectedCount := range expectedCounts {
		if symbolCounts[kind] != expectedCount {
			t.Errorf("symbol count for %s = %d, want %d", kind, symbolCounts[kind], expectedCount)
		}
	}
}

func TestParseFile_DDD_Patterns(t *testing.T) {
	p := parser.New()
	filePath := filepath.Join("..", "..", "testdata", "fixtures", "sample_ddd.go")

	result, err := p.ParseFile(filePath)
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}

	// Find specific DDD patterns
	tests := []struct {
		name         string
		symbolName   string
		wantPattern  string
		checkPattern func(*types.Symbol) bool
	}{
		{
			name:        "Aggregate Root",
			symbolName:  "OrderAggregate",
			wantPattern: "IsAggregateRoot",
			checkPattern: func(s *types.Symbol) bool {
				return s.IsAggregateRoot
			},
		},
		{
			name:        "Value Object",
			symbolName:  "OrderItemVO",
			wantPattern: "IsValueObject",
			checkPattern: func(s *types.Symbol) bool {
				return s.IsValueObject
			},
		},
		{
			name:        "Repository",
			symbolName:  "OrderRepository",
			wantPattern: "IsRepository",
			checkPattern: func(s *types.Symbol) bool {
				return s.IsRepository
			},
		},
		{
			name:        "Service",
			symbolName:  "OrderService",
			wantPattern: "IsService",
			checkPattern: func(s *types.Symbol) bool {
				return s.IsService
			},
		},
		{
			name:        "Command",
			symbolName:  "PlaceOrderCommand",
			wantPattern: "IsCommand",
			checkPattern: func(s *types.Symbol) bool {
				return s.IsCommand
			},
		},
		{
			name:        "Handler",
			symbolName:  "OrderPlacedHandler",
			wantPattern: "IsHandler",
			checkPattern: func(s *types.Symbol) bool {
				return s.IsHandler
			},
		},
		{
			name:        "Query",
			symbolName:  "ProcessOrderQuery",
			wantPattern: "IsQuery",
			checkPattern: func(s *types.Symbol) bool {
				return s.IsQuery
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			found := false
			for i := range result.Symbols {
				sym := &result.Symbols[i]
				if sym.Name == tt.symbolName {
					found = true
					if !tt.checkPattern(sym) {
						t.Errorf("symbol %s: %s = false, want true", tt.symbolName, tt.wantPattern)
					}
					break
				}
			}
			if !found {
				t.Errorf("symbol %s not found", tt.symbolName)
			}
		})
	}
}

func TestParseFile_SymbolExtraction(t *testing.T) {
	p := parser.New()
	filePath := filepath.Join("..", "..", "testdata", "fixtures", "sample_simple.go")

	result, err := p.ParseFile(filePath)
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}

	tests := []struct {
		name         string
		symbolName   string
		wantKind     types.SymbolKind
		wantScope    types.SymbolScope
		wantReceiver string
		wantHasDoc   bool
		wantHasSig   bool
	}{
		{
			name:         "User struct",
			symbolName:   "User",
			wantKind:     types.KindStruct,
			wantScope:    types.ScopeExported,
			wantReceiver: "",
			wantHasDoc:   true,
			wantHasSig:   true,
		},
		{
			name:         "UserRepository interface",
			symbolName:   "UserRepository",
			wantKind:     types.KindInterface,
			wantScope:    types.ScopeExported,
			wantReceiver: "",
			wantHasDoc:   true,
			wantHasSig:   true,
		},
		{
			name:         "Greet method",
			symbolName:   "Greet",
			wantKind:     types.KindMethod,
			wantScope:    types.ScopeExported,
			wantReceiver: "User",
			wantHasDoc:   true,
			wantHasSig:   true,
		},
		{
			name:         "ValidateEmail function",
			symbolName:   "ValidateEmail",
			wantKind:     types.KindFunction,
			wantScope:    types.ScopeExported,
			wantReceiver: "",
			wantHasDoc:   true,
			wantHasSig:   true,
		},
		{
			name:         "MaxNameLength const",
			symbolName:   "MaxNameLength",
			wantKind:     types.KindConst,
			wantScope:    types.ScopeExported,
			wantReceiver: "",
			wantHasDoc:   false,
			wantHasSig:   true,
		},
		{
			name:         "DefaultName var",
			symbolName:   "DefaultName",
			wantKind:     types.KindVar,
			wantScope:    types.ScopeExported,
			wantReceiver: "",
			wantHasDoc:   false,
			wantHasSig:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var sym *types.Symbol
			for i := range result.Symbols {
				s := &result.Symbols[i]
				if s.Name == tt.symbolName && s.Kind == tt.wantKind {
					sym = s
					break
				}
			}

			if sym == nil {
				t.Fatalf("symbol %s (kind: %s) not found", tt.symbolName, tt.wantKind)
			}

			if sym.Kind != tt.wantKind {
				t.Errorf("Kind = %s, want %s", sym.Kind, tt.wantKind)
			}

			if sym.Scope != tt.wantScope {
				t.Errorf("Scope = %s, want %s", sym.Scope, tt.wantScope)
			}

			if sym.Receiver != tt.wantReceiver {
				t.Errorf("Receiver = %q, want %q", sym.Receiver, tt.wantReceiver)
			}

			if tt.wantHasDoc && sym.DocComment == "" {
				t.Errorf("expected doc comment, got empty string")
			}

			if tt.wantHasSig && sym.Signature == "" {
				t.Errorf("expected signature, got empty string")
			}

			if sym.Package != "sample" {
				t.Errorf("Package = %q, want %q", sym.Package, "sample")
			}

			// Validate positions
			if sym.Start.Line <= 0 || sym.End.Line <= 0 {
				t.Errorf("invalid position: Start=%d, End=%d", sym.Start.Line, sym.End.Line)
			}

			if sym.Start.Line > sym.End.Line {
				t.Errorf("Start line (%d) > End line (%d)", sym.Start.Line, sym.End.Line)
			}
		})
	}
}

func TestParseFile_ErrorHandling(t *testing.T) {
	p := parser.New()
	filePath := filepath.Join("..", "..", "testdata", "fixtures", "sample_error.go")

	result, err := p.ParseFile(filePath)

	// Should not return an error - errors are captured in result
	if err != nil {
		t.Fatalf("ParseFile() unexpected error = %v", err)
	}

	if result == nil {
		t.Fatal("ParseFile() returned nil result")
	}

	// Should have parse errors
	if !result.HasErrors() {
		t.Error("expected parse errors, got none")
	}

	if len(result.Errors) == 0 {
		t.Error("expected Errors slice to be populated")
	}
}

func TestParseFile_NonExistent(t *testing.T) {
	p := parser.New()
	filePath := filepath.Join("..", "..", "testdata", "fixtures", "nonexistent.go")

	result, err := p.ParseFile(filePath)
	if err == nil {
		t.Error("expected error for non-existent file, got nil")
	}

	if result != nil {
		t.Error("expected nil result for non-existent file")
	}
}

func TestSymbol_Validation(t *testing.T) {
	tests := []struct {
		name    string
		symbol  types.Symbol
		wantErr bool
	}{
		{
			name: "valid function",
			symbol: types.Symbol{
				Name:      "TestFunc",
				Kind:      types.KindFunction,
				Package:   "test",
				Scope:     types.ScopeExported,
				Signature: "func TestFunc() error",
				Start:     types.Position{Line: 1, Column: 1},
				End:       types.Position{Line: 3, Column: 1},
			},
			wantErr: false,
		},
		{
			name: "valid method with receiver",
			symbol: types.Symbol{
				Name:      "Method",
				Kind:      types.KindMethod,
				Package:   "test",
				Scope:     types.ScopeExported,
				Receiver:  "MyType",
				Signature: "func (m *MyType) Method()",
				Start:     types.Position{Line: 1, Column: 1},
				End:       types.Position{Line: 3, Column: 1},
			},
			wantErr: false,
		},
		{
			name: "invalid - missing name",
			symbol: types.Symbol{
				Name:    "",
				Kind:    types.KindFunction,
				Package: "test",
				Scope:   types.ScopeExported,
				Start:   types.Position{Line: 1, Column: 1},
				End:     types.Position{Line: 3, Column: 1},
			},
			wantErr: true,
		},
		{
			name: "invalid - method without receiver",
			symbol: types.Symbol{
				Name:     "Method",
				Kind:     types.KindMethod,
				Package:  "test",
				Scope:    types.ScopeExported,
				Receiver: "",
				Start:    types.Position{Line: 1, Column: 1},
				End:      types.Position{Line: 3, Column: 1},
			},
			wantErr: true,
		},
		{
			name: "invalid - function with receiver",
			symbol: types.Symbol{
				Name:     "Func",
				Kind:     types.KindFunction,
				Package:  "test",
				Scope:    types.ScopeExported,
				Receiver: "MyType",
				Start:    types.Position{Line: 1, Column: 1},
				End:      types.Position{Line: 3, Column: 1},
			},
			wantErr: true,
		},
		{
			name: "invalid - start line after end line",
			symbol: types.Symbol{
				Name:    "Func",
				Kind:    types.KindFunction,
				Package: "test",
				Scope:   types.ScopeExported,
				Start:   types.Position{Line: 5, Column: 1},
				End:     types.Position{Line: 3, Column: 1},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.symbol.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSymbol_IsExported(t *testing.T) {
	tests := []struct {
		name   string
		symbol types.Symbol
		want   bool
	}{
		{
			name: "exported symbol",
			symbol: types.Symbol{
				Name:  "ExportedFunc",
				Scope: types.ScopeExported,
			},
			want: true,
		},
		{
			name: "unexported symbol",
			symbol: types.Symbol{
				Name:  "unexportedFunc",
				Scope: types.ScopeUnexported,
			},
			want: false,
		},
		{
			name: "exported name but unexported scope",
			symbol: types.Symbol{
				Name:  "ExportedName",
				Scope: types.ScopeUnexported,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.symbol.IsExported(); got != tt.want {
				t.Errorf("IsExported() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSymbol_IsDDDPattern(t *testing.T) {
	tests := []struct {
		name   string
		symbol types.Symbol
		want   bool
	}{
		{
			name: "aggregate root",
			symbol: types.Symbol{
				IsAggregateRoot: true,
			},
			want: true,
		},
		{
			name: "repository",
			symbol: types.Symbol{
				IsRepository: true,
			},
			want: true,
		},
		{
			name: "service",
			symbol: types.Symbol{
				IsService: true,
			},
			want: true,
		},
		{
			name: "no DDD pattern",
			symbol: types.Symbol{
				Name: "RegularStruct",
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.symbol.IsDDDPattern(); got != tt.want {
				t.Errorf("IsDDDPattern() = %v, want %v", got, tt.want)
			}
		})
	}
}
