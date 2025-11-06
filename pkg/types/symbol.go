package types

import (
	"errors"
	"go/token"
)

// SymbolKind represents the type of Go language symbol
type SymbolKind string

const (
	KindFunction  SymbolKind = "function"
	KindMethod    SymbolKind = "method"
	KindStruct    SymbolKind = "struct"
	KindInterface SymbolKind = "interface"
	KindType      SymbolKind = "type"
	KindConst     SymbolKind = "const"
	KindVar       SymbolKind = "var"
	KindField     SymbolKind = "field"
)

// SymbolScope represents the visibility scope of a symbol
type SymbolScope string

const (
	ScopeExported     SymbolScope = "exported"
	ScopeUnexported   SymbolScope = "unexported"
	ScopePackageLocal SymbolScope = "package_local"
)

// Position represents a location in source code
type Position struct {
	Line   int
	Column int
}

// Symbol represents a code symbol extracted from Go source via AST parsing
type Symbol struct {
	// Identification
	Name    string
	Kind    SymbolKind
	Package string

	// Content
	Signature  string // Function signature or type definition
	DocComment string

	// Scope
	Scope    SymbolScope
	Receiver string // For methods: receiver type name

	// Location
	Start Position
	End   Position

	// DDD Pattern Detection Flags
	IsAggregateRoot bool
	IsEntity        bool
	IsValueObject   bool
	IsRepository    bool
	IsService       bool
	IsCommand       bool
	IsQuery         bool
	IsHandler       bool
}

// ValidateKind checks if the symbol kind is valid
func (s *Symbol) ValidateKind() error {
	switch s.Kind {
	case KindFunction, KindMethod, KindStruct, KindInterface, KindType, KindConst, KindVar, KindField:
		return nil
	default:
		return errors.New("invalid symbol kind")
	}
}

// ValidateScope checks if the symbol scope is valid
func (s *Symbol) ValidateScope() error {
	switch s.Scope {
	case ScopeExported, ScopeUnexported, ScopePackageLocal:
		return nil
	default:
		return errors.New("invalid symbol scope")
	}
}

// IsExported returns true if the symbol is exported (visible outside package)
func (s *Symbol) IsExported() bool {
	return s.Scope == ScopeExported && token.IsExported(s.Name)
}

// Validate performs comprehensive validation of the symbol
func (s *Symbol) Validate() error {
	if s.Name == "" {
		return errors.New("symbol name is required")
	}

	if err := s.ValidateKind(); err != nil {
		return err
	}

	if err := s.ValidateScope(); err != nil {
		return err
	}

	if s.Package == "" {
		return errors.New("package name is required")
	}

	// Methods must have a receiver
	if s.Kind == KindMethod && s.Receiver == "" {
		return errors.New("methods must have a receiver type")
	}

	// Non-methods should not have a receiver
	if s.Kind != KindMethod && s.Receiver != "" {
		return errors.New("only methods can have a receiver type")
	}

	// Position validation
	if s.Start.Line <= 0 || s.End.Line <= 0 {
		return errors.New("invalid position: line numbers must be positive")
	}

	if s.Start.Line > s.End.Line {
		return errors.New("invalid position: start line must be before or equal to end line")
	}

	return nil
}

// IsDDDPattern returns true if this symbol matches any DDD pattern
func (s *Symbol) IsDDDPattern() bool {
	return s.IsAggregateRoot || s.IsEntity || s.IsValueObject ||
		s.IsRepository || s.IsService || s.IsCommand || s.IsQuery || s.IsHandler
}
