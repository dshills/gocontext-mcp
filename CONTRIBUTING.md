# Contributing to GoContext

Thank you for your interest in contributing to GoContext! This document provides guidelines and instructions for contributing to the project.

## Table of Contents

- [Code of Conduct](#code-of-conduct)
- [Getting Started](#getting-started)
- [Development Setup](#development-setup)
- [Development Workflow](#development-workflow)
- [Coding Standards](#coding-standards)
- [Testing Guidelines](#testing-guidelines)
- [Submitting Changes](#submitting-changes)
- [Release Process](#release-process)

---

## Code of Conduct

This project adheres to the [Contributor Covenant Code of Conduct](CODE_OF_CONDUCT.md). By participating, you are expected to uphold this code. Please report unacceptable behavior to the project maintainers.

---

## Getting Started

### Prerequisites

- **Go 1.21 or later**: [Download Go](https://golang.org/dl/)
- **Git**: [Install Git](https://git-scm.com/downloads)
- **C Compiler** (for CGO build): gcc or clang
  - macOS: `xcode-select --install`
  - Linux: `apt-get install build-essential` or `yum install gcc`
  - Windows: [MinGW-w64](http://mingw-w64.org/)
- **golangci-lint**: [Install golangci-lint](https://golangci-lint.run/usage/install/)

### Fork and Clone

1. Fork the repository on GitHub
2. Clone your fork:
   ```bash
   git clone https://github.com/YOUR_USERNAME/gocontext-mcp.git
   cd gocontext-mcp
   ```
3. Add upstream remote:
   ```bash
   git remote add upstream https://github.com/dshills/gocontext-mcp.git
   ```

---

## Development Setup

### Install Dependencies

```bash
# Download Go dependencies
go mod download

# Verify dependencies
go mod verify
```

### Build the Project

```bash
# CGO build (recommended for development)
make build

# Pure Go build (for testing portability)
make build-purego

# Verify build
./bin/gocontext --version
```

### Run Tests

```bash
# Run all tests
make test

# Run tests with coverage
make test-coverage

# Run tests with race detector
make test-race

# Run benchmarks
make bench
```

### Development Tools

Install recommended development tools:

```bash
# golangci-lint for linting
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# staticcheck for static analysis
go install honnef.co/go/tools/cmd/staticcheck@latest

# goimports for import formatting
go install golang.org/x/tools/cmd/goimports@latest
```

---

## Development Workflow

### 1. Create a Feature Branch

```bash
# Update main branch
git checkout main
git pull upstream main

# Create feature branch
git checkout -b feature/your-feature-name
```

Branch naming conventions:
- Features: `feature/description`
- Bug fixes: `fix/description`
- Documentation: `docs/description`
- Refactoring: `refactor/description`

### 2. Make Changes

Follow the [Coding Standards](#coding-standards) below.

### 3. Write Tests

All new features and bug fixes must include tests:
- Unit tests for business logic
- Integration tests for component interactions
- Benchmarks for performance-critical code

### 4. Run Quality Checks

Before committing, run:

```bash
# Format code
make fmt

# Run linters
make lint

# Run tests
make test

# Or run all checks at once
make dev
```

### 5. Commit Changes

Follow [Conventional Commits](https://www.conventionalcommits.org/):

```bash
git add .
git commit -m "feat: add incremental indexing support"
```

Commit message format:
```
<type>(<scope>): <subject>

<body>

<footer>
```

**Types**:
- `feat`: New feature
- `fix`: Bug fix
- `docs`: Documentation changes
- `refactor`: Code refactoring (no functional changes)
- `test`: Adding or updating tests
- `perf`: Performance improvements
- `chore`: Build process, dependencies, etc.

**Examples**:
```
feat(parser): add support for generics parsing

Implement Go 1.18+ generics support in AST parser.
Adds type parameter extraction and constraint handling.

Closes #42
```

```
fix(searcher): correct BM25 ranking calculation

The BM25 implementation was not normalizing document lengths
correctly, leading to biased results for short documents.

Fixes #58
```

### 6. Push and Create Pull Request

```bash
# Push to your fork
git push origin feature/your-feature-name

# Create pull request on GitHub
```

---

## Coding Standards

### Go Code Style

Follow the official [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments).

**Key principles**:
1. **Simplicity**: Prefer clear code over clever code
2. **Idiomatic Go**: Use Go idioms and conventions
3. **Error handling**: Always handle errors explicitly
4. **Documentation**: Document all exported symbols
5. **Consistency**: Match existing code style

### Code Organization

**Package structure**:
```
gocontext-mcp/
├── cmd/              # Command-line entry points
├── internal/         # Private application code
│   ├── parser/      # Component packages
│   ├── chunker/
│   └── ...
├── pkg/             # Public library code (if any)
│   └── types/       # Shared types
└── tests/           # Integration tests
```

**File naming**:
- Implementation: `parser.go`
- Tests: `parser_test.go`
- Benchmarks: `parser_bench_test.go`
- Platform-specific: `storage_unix.go`, `storage_windows.go`
- Build tags: `build_cgo.go`, `build_purego.go`

### Naming Conventions

**Exported symbols** (public API):
```go
// Package names: short, lowercase, no underscores
package parser

// Interfaces: noun or noun phrase
type Embedder interface { ... }

// Structs: noun or noun phrase
type ParseResult struct { ... }

// Functions: verb or verb phrase
func ParseFile(path string) (*ParseResult, error) { ... }

// Constants: MixedCaps or ALL_CAPS for emphasis
const DefaultTimeout = 30 * time.Second
const MAX_WORKERS = 100
```

**Unexported symbols** (internal):
```go
// Start with lowercase
type workerPool struct { ... }
func parseSymbols(node ast.Node) []Symbol { ... }
```

### Documentation

**Package documentation** (package comment in any file, conventionally doc.go):
```go
// Package parser extracts symbols and metadata from Go source files
// using the standard library's go/parser and go/ast packages.
//
// The parser identifies functions, methods, types, and other symbols,
// along with their documentation, signatures, and positions.
//
// Example usage:
//
//   p := parser.New()
//   result, err := p.ParseFile("internal/example.go")
//   if err != nil {
//       log.Fatal(err)
//   }
//   fmt.Printf("Found %d symbols\n", len(result.Symbols))
//
package parser
```

**Function documentation**:
```go
// ParseFile parses a Go source file and extracts symbols.
//
// The path parameter must be an absolute path to a .go file.
// Returns a ParseResult containing all extracted symbols,
// or an error if parsing fails.
//
// ParseFile handles syntax errors gracefully and returns
// partial results when possible.
func ParseFile(path string) (*ParseResult, error) {
    // ...
}
```

**Struct documentation**:
```go
// Symbol represents a code symbol extracted from Go source.
// Symbols include functions, methods, types, constants, and variables.
//
// The DDD pattern flags (IsRepository, IsService, etc.) are populated
// by detectDDDPatterns based on naming conventions.
type Symbol struct {
    Name       string      // Symbol name (e.g., "ParseFile")
    Kind       SymbolKind  // Type of symbol (function, method, etc.)
    Signature  string      // Full signature (for functions/methods)
    DocComment string      // Extracted doc comment
    // ... more fields
}
```

### Error Handling

**Explicit error handling**:
```go
// Good: Handle errors explicitly
result, err := ParseFile(path)
if err != nil {
    return nil, fmt.Errorf("parsing file %s: %w", path, err)
}

// Bad: Ignore errors
result, _ := ParseFile(path)
```

**Error wrapping**:
```go
// Use %w to wrap errors for errors.Is() and errors.As()
if err := processFile(path); err != nil {
    return fmt.Errorf("processing %s: %w", path, err)
}
```

**Sentinel errors**:
```go
// Define package-level sentinel errors for known conditions
var (
    ErrNotFound = errors.New("resource not found")
    ErrInvalidInput = errors.New("invalid input")
)

// Use errors.Is() for checking
if errors.Is(err, ErrNotFound) {
    // handle not found
}
```

### Concurrency

**Use errgroup for parallel tasks**:
```go
import "golang.org/x/sync/errgroup"

func processFiles(files []string) error {
    g, ctx := errgroup.WithContext(context.Background())
    g.SetLimit(runtime.NumCPU())

    for _, file := range files {
        file := file  // capture loop variable
        g.Go(func() error {
            return processFile(ctx, file)
        })
    }

    return g.Wait()
}
```

**Always provide cancellation**:
```go
// Good: Context-aware goroutine
func worker(ctx context.Context) error {
    for {
        select {
        case <-ctx.Done():
            return ctx.Err()
        case work := <-workChan:
            // process work
        }
    }
}
```

---

## Testing Guidelines

### Test Organization

**Unit tests** (`*_test.go` in same package):
```go
package parser

import "testing"

func TestParseFile(t *testing.T) {
    // Use table-driven tests
    tests := []struct {
        name    string
        input   string
        want    *ParseResult
        wantErr bool
    }{
        {
            name:    "simple function",
            input:   "testdata/simple.go",
            want:    &ParseResult{...},
            wantErr: false,
        },
        {
            name:    "syntax error",
            input:   "testdata/broken.go",
            want:    nil,
            wantErr: true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := ParseFile(tt.input)
            if (err != nil) != tt.wantErr {
                t.Errorf("ParseFile() error = %v, wantErr %v", err, tt.wantErr)
                return
            }
            if !reflect.DeepEqual(got, tt.want) {
                t.Errorf("ParseFile() = %v, want %v", got, tt.want)
            }
        })
    }
}
```

**Integration tests** (`tests/integration/`):
```go
package integration_test

func TestIndexAndSearch(t *testing.T) {
    // Setup: Create temp directory with Go files
    tmpDir := t.TempDir()
    writeTestFile(t, tmpDir, "main.go", mainGoContent)

    // Index
    indexer := indexer.New(storage, embedder)
    stats, err := indexer.IndexProject(tmpDir)
    require.NoError(t, err)
    assert.Equal(t, 1, stats.FilesIndexed)

    // Search
    results, err := searcher.Search(tmpDir, "main function")
    require.NoError(t, err)
    assert.NotEmpty(t, results)
}
```

**Benchmarks** (`*_bench_test.go`):
```go
func BenchmarkParseFile(b *testing.B) {
    parser := parser.New()
    b.ResetTimer()

    for i := 0; i < b.N; i++ {
        _, err := parser.ParseFile("testdata/large.go")
        if err != nil {
            b.Fatal(err)
        }
    }
}

// Run with: go test -bench=. -benchmem
```

### Test Data

Place test data in `testdata/` directories:
```
internal/parser/
├── parser.go
├── parser_test.go
└── testdata/
    ├── simple.go
    ├── complex.go
    └── broken.go
```

### Coverage Targets

- **Unit tests**: 80% coverage minimum
- **Integration tests**: Cover all major user workflows
- **Critical paths**: 95% coverage for parser, indexer, searcher

Check coverage:
```bash
go test -cover ./...
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

### Mocking

Use interfaces for dependency injection and mocking:

```go
// Interface for mocking in tests
type Embedder interface {
    Embed(texts []string) ([][]float32, error)
}

// Test with mock
type mockEmbedder struct {
    embedFunc func([]string) ([][]float32, error)
}

func (m *mockEmbedder) Embed(texts []string) ([][]float32, error) {
    return m.embedFunc(texts)
}
```

---

## Submitting Changes

### Pull Request Process

1. **Update documentation**: If your change affects user-facing behavior, update docs
2. **Add tests**: Ensure adequate test coverage
3. **Run checks**: `make dev` should pass
4. **Update CHANGELOG**: Add entry under "Unreleased" section
5. **Create PR**: Provide clear description of changes

### Pull Request Template

When creating a PR, include:

```markdown
## Description

Brief description of changes.

## Motivation

Why is this change needed? What problem does it solve?

## Changes

- Added X feature
- Fixed Y bug
- Refactored Z component

## Testing

- [ ] Unit tests added/updated
- [ ] Integration tests added/updated
- [ ] Benchmarks added (if performance-related)
- [ ] Manual testing performed

## Checklist

- [ ] Code follows project style guidelines
- [ ] Documentation updated
- [ ] Tests pass (`make test`)
- [ ] Linters pass (`make lint`)
- [ ] CHANGELOG updated

## Related Issues

Closes #42
Related to #38
```

### Review Process

1. Automated checks run (CI/CD)
2. Maintainer review
3. Address feedback
4. Approval and merge

**Review criteria**:
- Code quality and style
- Test coverage
- Documentation
- Performance impact
- Backward compatibility

---

## Release Process

Maintainers follow this process for releases:

### Versioning

GoContext follows [Semantic Versioning](https://semver.org/):

- **MAJOR** (1.x.x): Breaking changes
- **MINOR** (x.1.x): New features (backward-compatible)
- **PATCH** (x.x.1): Bug fixes (backward-compatible)

### Release Checklist

1. Update version in code
2. Update CHANGELOG
3. Create git tag: `git tag v1.2.0`
4. Push tag: `git push origin v1.2.0`
5. GitHub Actions builds release binaries
6. Create GitHub release with notes

---

## Getting Help

- **Questions**: Open a [GitHub Discussion](https://github.com/dshills/gocontext-mcp/discussions)
- **Bugs**: Open a [GitHub Issue](https://github.com/dshills/gocontext-mcp/issues)
- **Security**: See [SECURITY.md](SECURITY.md)
- **Chat**: Join our community (link TBD)

---

## Recognition

Contributors are recognized in:
- CHANGELOG release notes
- GitHub contributors page
- Annual contributor acknowledgments

Thank you for contributing to GoContext!
