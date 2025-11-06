package chunker_test

import (
	"path/filepath"
	"testing"

	"github.com/dshills/gocontext-mcp/internal/chunker"
	"github.com/dshills/gocontext-mcp/internal/parser"
)

func BenchmarkChunkFile_Simple(b *testing.B) {
	p := parser.New()
	filePath := filepath.Join("..", "..", "testdata", "fixtures", "sample_simple.go")
	parseResult, err := p.ParseFile(filePath)
	if err != nil {
		b.Fatal(err)
	}

	c := chunker.New()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		chunks, err := c.ChunkFile(filePath, parseResult, 1)
		if err != nil {
			b.Fatal(err)
		}
		if len(chunks) == 0 {
			b.Fatal("no chunks")
		}
	}
}

func BenchmarkChunkFile_DDD(b *testing.B) {
	p := parser.New()
	filePath := filepath.Join("..", "..", "testdata", "fixtures", "sample_ddd.go")
	parseResult, err := p.ParseFile(filePath)
	if err != nil {
		b.Fatal(err)
	}

	c := chunker.New()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		chunks, err := c.ChunkFile(filePath, parseResult, 2)
		if err != nil {
			b.Fatal(err)
		}
		if len(chunks) == 0 {
			b.Fatal("no chunks")
		}
	}
}

func BenchmarkChunkFile_FunctionLevel(b *testing.B) {
	p := parser.New()
	filePath := filepath.Join("..", "..", "testdata", "fixtures", "sample_simple.go")
	parseResult, err := p.ParseFile(filePath)
	if err != nil {
		b.Fatal(err)
	}

	c := chunker.New()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		chunks, err := c.ChunkFileWithStrategy(filePath, parseResult, 1, chunker.StrategyFunctionLevel)
		if err != nil {
			b.Fatal(err)
		}
		if len(chunks) == 0 {
			b.Fatal("no chunks")
		}
	}
}

func BenchmarkChunkFile_PackageLevel(b *testing.B) {
	p := parser.New()
	filePath := filepath.Join("..", "..", "testdata", "fixtures", "sample_simple.go")
	parseResult, err := p.ParseFile(filePath)
	if err != nil {
		b.Fatal(err)
	}

	c := chunker.New()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		chunks, err := c.ChunkFileWithStrategy(filePath, parseResult, 1, chunker.StrategyPackageLevel)
		if err != nil {
			b.Fatal(err)
		}
		if len(chunks) == 0 {
			b.Fatal("no chunks")
		}
	}
}

func BenchmarkTokenCount(b *testing.B) {
	text := "func HelloWorld(name string) string { return fmt.Sprintf(\"Hello, %s!\", name) }"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = chunker.EstimateTokenCount(text)
	}
}

func BenchmarkContentHash(b *testing.B) {
	content := "func HelloWorld(name string) string { return fmt.Sprintf(\"Hello, %s!\", name) }"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = chunker.ComputeChunkHash(content)
	}
}

func BenchmarkFullPipeline(b *testing.B) {
	// Benchmark the full parse + chunk pipeline
	filePath := filepath.Join("..", "..", "testdata", "fixtures", "sample_simple.go")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p := parser.New()
		parseResult, err := p.ParseFile(filePath)
		if err != nil {
			b.Fatal(err)
		}

		c := chunker.New()
		chunks, err := c.ChunkFile(filePath, parseResult, 1)
		if err != nil {
			b.Fatal(err)
		}
		if len(chunks) == 0 {
			b.Fatal("no chunks")
		}
	}
}
