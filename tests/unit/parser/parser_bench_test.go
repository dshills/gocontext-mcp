package parser_test

import (
	"path/filepath"
	"testing"

	"github.com/dshills/gocontext-mcp/internal/parser"
)

func BenchmarkParseFile_Simple(b *testing.B) {
	p := parser.New()
	filePath := filepath.Join("..", "..", "testdata", "fixtures", "sample_simple.go")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result, err := p.ParseFile(filePath)
		if err != nil {
			b.Fatal(err)
		}
		if result == nil {
			b.Fatal("nil result")
		}
	}
}

func BenchmarkParseFile_DDD(b *testing.B) {
	p := parser.New()
	filePath := filepath.Join("..", "..", "testdata", "fixtures", "sample_ddd.go")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result, err := p.ParseFile(filePath)
		if err != nil {
			b.Fatal(err)
		}
		if result == nil {
			b.Fatal("nil result")
		}
	}
}

func BenchmarkSymbolExtraction(b *testing.B) {
	p := parser.New()
	filePath := filepath.Join("..", "..", "testdata", "fixtures", "sample_simple.go")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result, err := p.ParseFile(filePath)
		if err != nil {
			b.Fatal(err)
		}

		// Access symbols to ensure they're extracted
		_ = len(result.Symbols)
	}
}

func BenchmarkDDDDetection(b *testing.B) {
	p := parser.New()
	filePath := filepath.Join("..", "..", "testdata", "fixtures", "sample_ddd.go")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result, err := p.ParseFile(filePath)
		if err != nil {
			b.Fatal(err)
		}

		// Check DDD patterns on all symbols
		for _, sym := range result.Symbols {
			_ = sym.IsDDDPattern()
		}
	}
}

func BenchmarkImportExtraction(b *testing.B) {
	p := parser.New()
	filePath := filepath.Join("..", "..", "testdata", "fixtures", "sample_simple.go")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result, err := p.ParseFile(filePath)
		if err != nil {
			b.Fatal(err)
		}

		// Access imports
		_ = len(result.Imports)
	}
}
