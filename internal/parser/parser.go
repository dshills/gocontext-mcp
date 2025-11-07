package parser

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"

	"github.com/dshills/gocontext-mcp/pkg/types"
)

// Parser handles AST-based parsing of Go source files
type Parser struct {
	fset *token.FileSet
}

// New creates a new Parser instance
func New() *Parser {
	return &Parser{
		fset: token.NewFileSet(),
	}
}

// ParseFile parses a Go source file and extracts symbols, imports, and package information
func (p *Parser) ParseFile(filePath string) (*types.ParseResult, error) {
	result := &types.ParseResult{}

	// Read the file
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	// Parse the file with comments for doc extraction
	file, err := parser.ParseFile(p.fset, filePath, content, parser.ParseComments)
	if err != nil {
		// Syntax errors are non-fatal - record error but continue with partial AST
		result.AddError(filePath, 0, 0, fmt.Sprintf("syntax error: %v", err))
		// Note: parser.ParseFile may return partial AST even on error
		// Continue processing if we have any AST nodes
	}

	// Extract partial results from whatever AST we have
	if file != nil {
		// Extract package name
		if file.Name != nil {
			result.PackageName = file.Name.Name
		}

		// Extract imports
		result.Imports = p.extractImports(file)

		// Extract symbols using AST traversal
		extractor := &symbolExtractor{
			fset:        p.fset,
			file:        file,
			filePath:    filePath,
			packageName: result.PackageName,
			symbols:     make([]types.Symbol, 0),
		}

		ast.Inspect(file, extractor.visit)
		result.Symbols = extractor.symbols
	}

	return result, nil
}

// extractImports extracts import statements from the AST
func (p *Parser) extractImports(file *ast.File) []types.Import {
	imports := make([]types.Import, 0, len(file.Imports))

	for _, imp := range file.Imports {
		importSpec := types.Import{
			Path: strings.Trim(imp.Path.Value, `"`),
		}

		// Check for alias
		if imp.Name != nil {
			importSpec.Alias = imp.Name.Name
		}

		imports = append(imports, importSpec)
	}

	return imports
}

// symbolExtractor is a visitor for AST traversal that extracts symbols
type symbolExtractor struct {
	fset        *token.FileSet
	file        *ast.File
	filePath    string
	packageName string
	symbols     []types.Symbol
}

// visit is called for each AST node during traversal
func (e *symbolExtractor) visit(node ast.Node) bool {
	if node == nil {
		return false
	}

	switch n := node.(type) {
	case *ast.FuncDecl:
		e.extractFunction(n)
	case *ast.GenDecl:
		e.extractGenDecl(n)
	}

	return true
}

// extractFunction extracts function and method declarations
func (e *symbolExtractor) extractFunction(funcDecl *ast.FuncDecl) {
	sym := types.Symbol{
		Name:       funcDecl.Name.Name,
		Package:    e.packageName,
		DocComment: e.extractDocComment(funcDecl.Doc),
		Start:      e.positionFromToken(funcDecl.Pos()),
		End:        e.positionFromToken(funcDecl.End()),
	}

	// Determine if this is a method or function
	if funcDecl.Recv != nil && len(funcDecl.Recv.List) > 0 {
		sym.Kind = types.KindMethod
		sym.Receiver = e.extractReceiverType(funcDecl.Recv.List[0].Type)
	} else {
		sym.Kind = types.KindFunction
	}

	// Extract function signature
	sym.Signature = e.extractFunctionSignature(funcDecl)

	// Determine scope
	sym.Scope = e.determineScope(sym.Name)

	// Detect DDD patterns
	detectDDDPatterns(&sym)

	e.symbols = append(e.symbols, sym)
}

// extractGenDecl extracts type, const, and var declarations
func (e *symbolExtractor) extractGenDecl(genDecl *ast.GenDecl) {
	for _, spec := range genDecl.Specs {
		switch s := spec.(type) {
		case *ast.TypeSpec:
			e.extractTypeSpec(s, genDecl.Doc)
		case *ast.ValueSpec:
			e.extractValueSpec(s, genDecl.Doc, genDecl.Tok)
		}
	}
}

// extractTypeSpec extracts struct, interface, and type alias declarations
func (e *symbolExtractor) extractTypeSpec(typeSpec *ast.TypeSpec, doc *ast.CommentGroup) {
	sym := types.Symbol{
		Name:       typeSpec.Name.Name,
		Package:    e.packageName,
		DocComment: e.extractDocComment(doc),
		Scope:      e.determineScope(typeSpec.Name.Name),
		Start:      e.positionFromToken(typeSpec.Pos()),
		End:        e.positionFromToken(typeSpec.End()),
	}

	// Determine the specific type
	switch t := typeSpec.Type.(type) {
	case *ast.StructType:
		sym.Kind = types.KindStruct
		sym.Signature = e.extractStructSignature(typeSpec.Name.Name, t)
	case *ast.InterfaceType:
		sym.Kind = types.KindInterface
		sym.Signature = e.extractInterfaceSignature(typeSpec.Name.Name, t)
	default:
		sym.Kind = types.KindType
		sym.Signature = fmt.Sprintf("type %s", typeSpec.Name.Name)
	}

	// Detect DDD patterns
	detectDDDPatterns(&sym)

	e.symbols = append(e.symbols, sym)

	// Extract struct fields as separate symbols
	if structType, ok := typeSpec.Type.(*ast.StructType); ok {
		e.extractStructFields(typeSpec.Name.Name, structType)
	}
}

// extractStructFields extracts field symbols from a struct
func (e *symbolExtractor) extractStructFields(structName string, structType *ast.StructType) {
	if structType.Fields == nil {
		return
	}

	for _, field := range structType.Fields.List {
		for _, name := range field.Names {
			fieldSym := types.Symbol{
				Name:      name.Name,
				Kind:      types.KindField,
				Package:   e.packageName,
				Receiver:  structName,
				Scope:     e.determineScope(name.Name),
				Start:     e.positionFromToken(field.Pos()),
				End:       e.positionFromToken(field.End()),
				Signature: fmt.Sprintf("%s %s", name.Name, e.exprToString(field.Type)),
			}

			e.symbols = append(e.symbols, fieldSym)
		}
	}
}

// extractValueSpec extracts const and var declarations
func (e *symbolExtractor) extractValueSpec(valueSpec *ast.ValueSpec, doc *ast.CommentGroup, tok token.Token) {
	var kind types.SymbolKind
	if tok == token.CONST {
		kind = types.KindConst
	} else {
		kind = types.KindVar
	}

	for _, name := range valueSpec.Names {
		sym := types.Symbol{
			Name:       name.Name,
			Kind:       kind,
			Package:    e.packageName,
			DocComment: e.extractDocComment(doc),
			Scope:      e.determineScope(name.Name),
			Start:      e.positionFromToken(valueSpec.Pos()),
			End:        e.positionFromToken(valueSpec.End()),
		}

		// Build signature
		if valueSpec.Type != nil {
			sym.Signature = fmt.Sprintf("%s %s", name.Name, e.exprToString(valueSpec.Type))
		} else if len(valueSpec.Values) > 0 {
			sym.Signature = fmt.Sprintf("%s = ...", name.Name)
		} else {
			sym.Signature = name.Name
		}

		e.symbols = append(e.symbols, sym)
	}
}

// extractReceiverType extracts the receiver type name from a method
func (e *symbolExtractor) extractReceiverType(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.StarExpr:
		if ident, ok := t.X.(*ast.Ident); ok {
			return ident.Name
		}
	case *ast.Ident:
		return t.Name
	}
	return ""
}

// extractFunctionSignature builds a function signature string
func (e *symbolExtractor) extractFunctionSignature(funcDecl *ast.FuncDecl) string {
	var sig strings.Builder

	sig.WriteString("func ")

	// Add receiver for methods
	if funcDecl.Recv != nil && len(funcDecl.Recv.List) > 0 {
		sig.WriteString("(")
		sig.WriteString(e.exprToString(funcDecl.Recv.List[0].Type))
		sig.WriteString(") ")
	}

	sig.WriteString(funcDecl.Name.Name)

	// Parameters
	sig.WriteString("(")
	if funcDecl.Type.Params != nil {
		sig.WriteString(e.fieldListToString(funcDecl.Type.Params))
	}
	sig.WriteString(")")

	// Results
	if funcDecl.Type.Results != nil {
		results := e.fieldListToString(funcDecl.Type.Results)
		if results != "" {
			if funcDecl.Type.Results.NumFields() > 1 {
				sig.WriteString(" (")
				sig.WriteString(results)
				sig.WriteString(")")
			} else {
				sig.WriteString(" ")
				sig.WriteString(results)
			}
		}
	}

	return sig.String()
}

// extractStructSignature builds a struct signature string
func (e *symbolExtractor) extractStructSignature(name string, structType *ast.StructType) string {
	fieldCount := 0
	if structType.Fields != nil {
		fieldCount = structType.Fields.NumFields()
	}
	return fmt.Sprintf("type %s struct { ... } // %d fields", name, fieldCount)
}

// extractInterfaceSignature builds an interface signature string
func (e *symbolExtractor) extractInterfaceSignature(name string, interfaceType *ast.InterfaceType) string {
	methodCount := 0
	if interfaceType.Methods != nil {
		methodCount = interfaceType.Methods.NumFields()
	}
	return fmt.Sprintf("type %s interface { ... } // %d methods", name, methodCount)
}

// fieldListToString converts a field list to a string representation
func (e *symbolExtractor) fieldListToString(fieldList *ast.FieldList) string {
	if fieldList == nil || len(fieldList.List) == 0 {
		return ""
	}

	var parts []string
	for _, field := range fieldList.List {
		typeStr := e.exprToString(field.Type)
		if len(field.Names) > 0 {
			for _, name := range field.Names {
				parts = append(parts, fmt.Sprintf("%s %s", name.Name, typeStr))
			}
		} else {
			parts = append(parts, typeStr)
		}
	}

	return strings.Join(parts, ", ")
}

// exprToString converts an expression to a string representation
func (e *symbolExtractor) exprToString(expr ast.Expr) string {
	if expr == nil {
		return ""
	}

	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return "*" + e.exprToString(t.X)
	case *ast.ArrayType:
		return "[]" + e.exprToString(t.Elt)
	case *ast.MapType:
		return fmt.Sprintf("map[%s]%s", e.exprToString(t.Key), e.exprToString(t.Value))
	case *ast.ChanType:
		return "chan " + e.exprToString(t.Value)
	case *ast.FuncType:
		return "func(...)"
	case *ast.InterfaceType:
		return "interface{}"
	case *ast.SelectorExpr:
		return e.exprToString(t.X) + "." + t.Sel.Name
	case *ast.Ellipsis:
		return "..." + e.exprToString(t.Elt)
	default:
		return "..."
	}
}

// extractDocComment extracts documentation from a comment group
func (e *symbolExtractor) extractDocComment(doc *ast.CommentGroup) string {
	if doc == nil {
		return ""
	}
	return strings.TrimSpace(doc.Text())
}

// determineScope determines if a symbol is exported or unexported
func (e *symbolExtractor) determineScope(name string) types.SymbolScope {
	if token.IsExported(name) {
		return types.ScopeExported
	}
	return types.ScopeUnexported
}

// positionFromToken converts a token position to our Position type
func (e *symbolExtractor) positionFromToken(pos token.Pos) types.Position {
	position := e.fset.Position(pos)
	return types.Position{
		Line:   position.Line,
		Column: position.Column,
	}
}
