<!--
Sync Impact Report:
- Version Change: N/A (initial creation) → 1.0.0
- New Constitution: First ratification
- Modified Principles: N/A (initial version)
- Added Sections: Core Principles (5), Quality Gates, Development Workflow, Governance
- Removed Sections: N/A
- Templates Requiring Updates:
  ✅ plan-template.md - Constitution Check section aligns with principles
  ✅ spec-template.md - Requirements align with testing and quality standards
  ✅ tasks-template.md - Task structure supports concurrent execution and testing discipline
- Follow-up TODOs: None
-->

# GoContext MCP Server Constitution

## Core Principles

### I. Code Quality (NON-NEGOTIABLE)

All linter issues MUST be resolved before any commit. The project uses golangci-lint with comprehensive checks (gofmt, govet, staticcheck, errcheck, gosimple, ineffassign, unused, typecheck, gocyclo, misspell, goconst). Zero tolerance for linter violations in committed code.

**Rationale**: Code quality issues compound over time. Enforcing clean code at commit time prevents technical debt accumulation and maintains codebase health for a production-ready MCP server.

### II. Concurrent Execution

Use concurrent agents, goroutines, and parallel processing whenever possible. The architecture leverages Go's concurrency primitives (goroutines, channels, errgroup) for indexing, parsing, and search operations.

**Rationale**: GoContext targets 100k+ LOC codebases with <5 minute indexing. This is only achievable through aggressive concurrency. All components (Parser, Indexer, Embedder) must be designed for concurrent execution.

### III. Test-Driven Quality

Run tests often and fix root causes of test failures, not symptoms. Target >80% code coverage. Tests must be written before implementation when following TDD discipline.

**Rationale**: GoContext is a production tool requiring high reliability. Surface-level fixes mask underlying issues. Root cause analysis prevents recurring failures and builds robust systems.

### IV. Performance First

All components must meet strict performance targets:
- Initial indexing: <5 minutes for 100k LOC
- Search latency: p95 <500ms
- Re-indexing: <30 seconds for 10 file changes
- Memory usage: <500MB for 100k LOC

**Rationale**: Performance is a core feature, not an optimization. These targets are success criteria that define production readiness.

### V. AST-Native Design

Leverage Go's native `go/parser`, `go/ast`, `go/types` packages for all code analysis. No regex parsing or text-based heuristics for Go code structure.

**Rationale**: AST parsing provides accurate, type-aware symbol extraction. This differentiates GoContext from general-purpose tools and enables domain-aware search for DDD patterns.

## Quality Gates

### Pre-Commit Requirements

1. **Linting**: `golangci-lint run` must pass with zero issues
2. **Tests**: All existing tests must pass (`go test ./...`)
3. **Formatting**: Code must be gofmt-compliant
4. **Build**: Both CGO and pure Go builds must succeed

### Pre-Push Requirements

1. All pre-commit gates pass
2. New functionality includes tests (unit or integration)
3. Coverage does not decrease
4. No commented-out code or debug statements

### Definition of Done

A task is complete when:
1. Implementation passes all quality gates
2. Tests verify the behavior (and fail before implementation if TDD)
3. Documentation updated if API/behavior changed
4. Code reviewed for concurrency safety
5. Performance targets validated for critical paths

## Development Workflow

### Concurrent Development

- Use goroutines and worker pools for I/O-bound operations (file parsing, API calls)
- Launch multiple Claude Code agents in parallel when tasks are independent
- Design components with concurrent usage in mind (thread-safe, minimal locking)
- Use `errgroup` for concurrent error handling

### Testing Discipline

- Run `go test ./...` frequently during development
- Investigate test failures deeply - never ignore or work around failing tests
- Use race detector (`go test -race ./...`) for concurrency testing
- Mock external dependencies (embedders, APIs) in unit tests
- Integration tests use in-memory SQLite (`:memory:`)

### Code Review Standards

- Reviewers must verify linter passes
- Verify concurrency patterns are correct (no data races, proper synchronization)
- Check for performance regressions in hot paths
- Validate test coverage for new code
- Ensure AST usage over text parsing

## Governance

This constitution defines non-negotiable standards for the GoContext MCP Server project. All code contributions, reviews, and architectural decisions must comply with these principles.

### Amendment Process

1. Amendments require documentation of rationale and impact analysis
2. Version must increment per semantic versioning:
   - MAJOR: Backward-incompatible governance changes or principle removal
   - MINOR: New principle added or materially expanded guidance
   - PATCH: Clarifications, wording improvements, typo fixes
3. All dependent templates must be updated to reflect changes

### Compliance Verification

- All pull requests must pass automated quality gates (linting, tests, builds)
- Code reviews must explicitly verify principle compliance
- Performance benchmarks run on critical paths
- Complexity violations (e.g., high cyclomatic complexity) require justification

### Runtime Development Guidance

For additional context and implementation details, refer to:
- `CLAUDE.md` - Development guidance for Claude Code
- `specs/gocontext-mcp_spec.md` - Complete technical specification
- `.golangci.yml` - Linter configuration and rules

**Version**: 1.0.0 | **Ratified**: 2025-11-06 | **Last Amended**: 2025-11-06
