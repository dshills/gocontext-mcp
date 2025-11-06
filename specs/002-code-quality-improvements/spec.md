# Feature Specification: Comprehensive Code Quality Improvements

**Feature Branch**: `002-code-quality-improvements`
**Created**: 2025-11-06
**Status**: Draft
**Input**: User description: "Fix 94 code quality issues from multi-LLM review: 4 critical (security/reliability), 18 high (performance/safety), 46 medium (maintainability), 26 low (style)"

## User Scenarios & Testing *(mandatory)*

### User Story 1 - System Stability and Security (Priority: P1)

As an operator deploying GoContext MCP to production, I need the system to be free of critical security vulnerabilities and crash-inducing bugs so that my users' data is protected and the service remains reliable.

**Why this priority**: Critical issues represent immediate risks of system crashes (nil pointer dereferences, invalid method calls), data leaks (database connection leaks), or security breaches (FTS5 injection, insecure password hashing). These must be fixed before the system can be safely deployed.

**Independent Test**: Can be fully tested by running the comprehensive test suite with race detection enabled, attempting SQL injection attacks on FTS5 search, verifying database connections are properly closed under failure conditions, and validating password hashing uses industry-standard algorithms.

**Acceptance Scenarios**:

1. **Given** a codebase with sync.Mutex.TryLock usage, **When** the code is compiled, **Then** it compiles successfully using a valid non-blocking lock mechanism (atomic operations or channels)

2. **Given** a database initialization that encounters PRAGMA failures, **When** the failure occurs, **Then** the database connection is properly closed with no resource leak

3. **Given** a malicious user providing FTS5-specific operators in search queries, **When** the search is executed, **Then** special characters are properly escaped and no SQL injection occurs

4. **Given** a user creating an account with a password, **When** the password is stored, **Then** it is hashed using bcrypt/scrypt/Argon2id with per-password salt

---

### User Story 2 - Performance and Scalability (Priority: P2)

As a developer indexing large Go codebases (100k+ LOC), I need the system to perform search and indexing operations efficiently without excessive memory usage or CPU consumption so that the tool remains responsive.

**Why this priority**: Performance issues directly impact user experience. Inefficient algorithms (O(n²) bubble sort), loading entire datasets into memory, and aggressive cache clearing cause slowdowns that make the tool unusable for large projects.

**Independent Test**: Can be tested by indexing a 100k LOC codebase, measuring search latency (target p95 < 500ms), memory usage (target < 500MB), and verifying cache hit rates improve over time rather than being cleared unnecessarily.

**Acceptance Scenarios**:

1. **Given** a nil embedder in the Searcher, **When** vector search is attempted, **Then** a descriptive error is returned instead of a nil pointer dereference

2. **Given** 1000 vector search candidates, **When** sorting by similarity score, **Then** an O(n log n) algorithm is used instead of O(n²) bubble sort

3. **Given** repeated identical search queries, **When** caching is enabled, **Then** subsequent queries use cached results without regenerating embeddings

4. **Given** a cache reaching capacity, **When** new items are added, **Then** only least-recently-used items are evicted rather than clearing the entire cache

5. **Given** large embedding datasets, **When** performing vector search, **Then** filtering and similarity computation are pushed to the database layer rather than loading all data into memory

6. **Given** concurrent search operations, **When** one operation is cancelled, **Then** channel sends include timeout handling to prevent panics or deadlocks

---

### User Story 3 - Data Integrity and Transaction Safety (Priority: P2)

As a developer relying on GoContext MCP's SQLite storage, I need all database operations within a transaction to see a consistent view of the data and handle migrations correctly so that my indexed codebase data remains accurate and uncorrupted.

**Why this priority**: Transaction isolation issues and migration bugs can lead to data corruption, race conditions, and incorrect application of schema changes. These issues affect data integrity but are less immediately dangerous than P1 security issues.

**Independent Test**: Can be tested by running concurrent indexing operations, performing reads within transactions after uncommitted writes, applying migrations in various version orders, and verifying unique constraints prevent duplicate data.

**Acceptance Scenarios**:

1. **Given** a read operation inside a transaction, **When** the transaction has uncommitted writes, **Then** the read sees the uncommitted changes (proper transaction isolation)

2. **Given** concurrent upsert operations on the same symbol/chunk, **When** both execute simultaneously, **Then** SQLite's UPSERT clause handles conflicts atomically without race conditions

3. **Given** database schema versions "1.2.0" and "1.10.0", **When** comparing versions, **Then** semantic versioning is used correctly ("1.10.0" > "1.2.0")

4. **Given** a failed embedding generation after chunk storage, **When** the error occurs, **Then** orphaned chunks are cleaned up automatically

5. **Given** nested transaction attempts, **When** BeginTx is called within an existing transaction, **Then** behavior is consistent (error or savepoint)

---

### User Story 4 - Code Maintainability and Consistency (Priority: P3)

As a developer contributing to or extending GoContext MCP, I need the codebase to follow consistent patterns, avoid duplication, and properly implement documented features so that I can understand and modify the code efficiently.

**Why this priority**: Maintainability issues don't cause immediate problems but accumulate technical debt. Incomplete features (ContextAfter never populated), duplicated logic (retry mechanisms), and inconsistent patterns make the codebase harder to work with over time.

**Independent Test**: Can be tested through code review, checking for duplicate code patterns, verifying all documented features are implemented, and ensuring consistent error handling patterns throughout the codebase.

**Acceptance Scenarios**:

1. **Given** chunk creation logic, **When** chunks are generated, **Then** ContextAfter field is populated with related symbols or the field is removed from types if not implemented

2. **Given** retry logic needed in multiple providers, **When** implementing retries, **Then** a common retry function is used across all providers

3. **Given** embedder instances needed by both indexer and searcher, **When** initializing the MCP server, **Then** a single shared embedder instance is used

4. **Given** regular expression patterns needed for query sanitization, **When** called repeatedly, **Then** precompiled regex patterns are reused

5. **Given** parser encountering syntax errors, **When** partial AST is available, **Then** partial results (package name, imports, valid symbols) are extracted

6. **Given** migration error conditions, **When** querying schema_version fails, **Then** database errors are distinguished from "no migrations applied" state

---

### User Story 5 - Code Style and Consistency (Priority: P4)

As a developer working in the GoContext MCP codebase, I need consistent naming, error handling, and code organization so that the codebase follows Go best practices and is easy to navigate.

**Why this priority**: Style issues have the lowest impact on functionality but improve developer experience and code quality. These include inconsistent naming, verbose error handling, unused code, and minor inefficiencies.

**Independent Test**: Can be tested by running linters (golangci-lint), code formatters (gofmt), and reviewing code for naming consistency and idiomatic Go patterns.

**Acceptance Scenarios**:

1. **Given** SQL queries with ORDER BY clauses, **When** executed, **Then** all column references exist in the SELECT list or are valid column names

2. **Given** error handling patterns, **When** errors occur, **Then** consistent wrapping with context is used throughout the codebase

3. **Given** exported types and functions, **When** reading code, **Then** all public APIs have complete documentation comments

4. **Given** duplicate string literals used multiple times, **When** code is analyzed, **Then** constants are defined for repeated values

5. **Given** test files, **When** linting, **Then** appropriate exclusions allow test-specific patterns without false positives

---

### Edge Cases

- What happens when multiple migrations need to be applied in sequence and one fails partway through?
- How does the system handle very large vector result sets (>10k candidates)?
- What occurs when cache eviction happens during an active search operation?
- How are race conditions handled when concurrent operations attempt to create the same symbol or chunk?
- What happens when an embedder provider API rate limit is exceeded during batch operations?
- How does the parser handle files with mixed valid and invalid Go syntax?
- What occurs when database connections are exhausted under high concurrent load?
- How are orphaned chunks handled when embedding generation fails intermittently?

## Requirements *(mandatory)*

### Functional Requirements

#### Critical Security & Reliability (P1)

- **FR-001**: System MUST use a valid concurrency primitive for non-blocking lock attempts (atomic.CompareAndSwap or channel-based semaphore) instead of nonexistent sync.Mutex.TryLock
- **FR-002**: System MUST ensure database connections are closed via defer immediately after opening, even if subsequent PRAGMA statements fail
- **FR-003**: System MUST sanitize all user-provided FTS5 search queries by escaping special operators (AND, OR, NOT, NEAR, *, ", ()) to prevent SQL injection
- **FR-004**: System MUST hash passwords using bcrypt, scrypt, or Argon2id with per-password salts and appropriate cost factors

#### Performance & Scalability (P2)

- **FR-005**: System MUST validate embedder is non-nil before use and return descriptive errors when vector search is attempted without configured embedder
- **FR-006**: System MUST implement functional query result caching with LRU eviction policy to avoid redundant embedding generation and API calls
- **FR-007**: System MUST use sort.Slice (O(n log n)) for sorting search candidates instead of bubble sort (O(n²))
- **FR-008**: System MUST perform vector similarity filtering and computation in the database layer rather than loading all embeddings into memory
- **FR-009**: System MUST implement proper LRU cache eviction for embeddings instead of clearing entire cache when capacity is reached
- **FR-010**: System MUST include timeout handling in channel send operations for concurrent search to prevent deadlocks and panics
- **FR-011**: System MUST either generate embeddings before transaction commit or implement cleanup mechanisms for chunks with failed embeddings

#### Data Integrity & Transactions (P2)

- **FR-012**: System MUST use semantic version comparison library for migration ordering instead of string comparison
- **FR-013**: System MUST distinguish between "no migrations applied" and database errors when querying schema_version
- **FR-014**: System MUST pass transaction querier to all read operations within a transaction to maintain isolation
- **FR-015**: System MUST use SQLite's "INSERT ... ON CONFLICT DO UPDATE" for atomic upserts instead of check-then-act patterns
- **FR-016**: System MUST validate all ORDER BY column references exist in SELECT list or are valid column names

#### Maintainability (P3)

- **FR-017**: System MUST either populate ContextAfter field with related symbols or remove field from Chunk type if feature is not implemented
- **FR-018**: System MUST extract common retry logic into reusable function used by all embedder providers
- **FR-019**: System MUST share single embedder instance between indexer and searcher components
- **FR-020**: System MUST precompile regular expressions at package level instead of compiling on each request
- **FR-021**: Parser MUST extract partial results (package name, imports, valid symbols) even when syntax errors occur
- **FR-022**: System MUST return immutable copies of cached embeddings to prevent caller mutations and data races

#### Code Quality (P3-P4)

- **FR-023**: System MUST implement token generation using crypto/rand or secure JWT libraries instead of predictable SHA-256 hashing
- **FR-024**: System MUST implement token validation against persistent storage or cryptographic signatures instead of accepting any non-empty token
- **FR-025**: System MUST define constants for repeated string literals used more than 3 times
- **FR-026**: System MUST provide complete documentation comments for all exported types, functions, and methods
- **FR-027**: System MUST handle error wrapping consistently using fmt.Errorf with %w verb throughout codebase

### Key Entities *(data integrity focus)*

- **Migration**: Represents database schema version and SQL statements; must be compared using semantic versioning
- **Cache Entry**: Represents cached embedding or search result; must include access time for LRU eviction
- **Transaction Context**: Represents isolated database view; all operations must use transaction querier for consistency
- **Search Candidate**: Represents potential search result; must be sorted efficiently (O(n log n))
- **Security Token**: Represents user session; must be generated using cryptographically secure random values

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: All 4 critical issues are resolved and system passes security audit (no SQL injection vulnerabilities, no crash-inducing bugs, secure password storage)
- **SC-002**: Search operation p95 latency remains < 500ms for codebases up to 100k LOC after performance fixes
- **SC-003**: Memory usage remains < 500MB for 100k LOC codebase after implementing efficient vector operations
- **SC-004**: Test suite passes with -race flag enabled, demonstrating no data races in cache or concurrent operations
- **SC-005**: Cache hit rate for repeated queries exceeds 80% after implementing LRU eviction
- **SC-006**: Database migration tests pass with semantic version ordering ("1.10.0" > "1.2.0")
- **SC-007**: Transaction isolation tests confirm reads within transactions see uncommitted writes
- **SC-008**: golangci-lint reports zero issues after addressing all style and best practice recommendations
- **SC-009**: Code coverage remains > 80% after all fixes with comprehensive tests for edge cases
- **SC-010**: All exported APIs have complete documentation comments passing go doc standards

### Qualitative Outcomes

- Codebase demonstrates consistent error handling patterns throughout
- Retry logic is centralized and reusable across all providers
- Security-sensitive operations (auth, search) use industry-standard libraries and practices
- Database operations follow proper transaction semantics and resource cleanup patterns
- Cache implementations follow established algorithms (LRU) rather than ad-hoc strategies

## Out of Scope

- Refactoring storage layer to use different database backend (PostgreSQL, etc.)
- Implementing distributed caching across multiple instances
- Complete rewrite of embedder provider integration
- Migration to different vector similarity search algorithms
- Changing MCP protocol implementation
- Redesigning chunk boundary algorithms
- Modifying parser to support additional languages beyond Go

## Technical Constraints

- Must maintain backward compatibility with existing SQLite database schemas
- Must work with existing MCP protocol implementation (github.com/mark3labs/mcp-go)
- Must support both CGO-enabled (sqlite-vec) and pure Go (purego) builds
- Must maintain current performance targets (indexing < 5 min for 100k LOC, search p95 < 500ms)
- All changes must pass existing test suite plus new tests for fixed issues
- Must continue to support Go 1.25.4 or later

## Dependencies

- Semantic versioning library (e.g., github.com/Masterminds/semver) for migration version comparison
- LRU cache implementation (e.g., github.com/hashicorp/golang-lru) or custom implementation
- golang.org/x/crypto/bcrypt for secure password hashing
- Existing dependencies: go/parser, go/ast, database/sql, github.com/mark3labs/mcp-go

## Assumptions

- Multi-LLM code review report accurately identifies legitimate issues (validated by consensus and severity)
- Existing test suite provides adequate coverage to detect regressions
- Database migration strategy allows for adding semantic version comparison
- Performance impact of fixes (e.g., deep copying cached embeddings) is acceptable given correctness requirements
- Current embedder API providers support retry mechanisms and rate limit handling

## Open Questions

- Should cache invalidation be automatic on project updates or manual via explicit API calls?
- What specific semantic versioning library should be used for migrations?
- Should nested transactions use savepoints or return explicit errors?
- What is acceptable performance overhead for returning immutable cache copies?
- Should token validation implementation be in production code or is it test-only fixture code?
- How should migration rollback be handled if partial migrations succeed?
- What monitoring/metrics should be added to track cache performance and eviction behavior?

## Notes

**Source Data**: Issues identified via multi-LLM code review using Anthropic, Google, and OpenAI providers
- 38 files reviewed
- 94 total issues found
- Categories: 8 Security, 19 Performance, 49 Best Practices, 18 Style
- Review report: `review-results/review-report-20251106-160257.md`

**Implementation Approach**:
- Fix issues in priority order (P1 → P2 → P3 → P4)
- Each priority group can be implemented as independent unit of work
- Add regression tests for each fixed issue
- Validate fixes against acceptance scenarios before marking complete

**Risk Assessment**:
- P1 issues present immediate security and reliability risks
- P2 performance issues limit scalability for large codebases
- P3 maintainability issues accumulate technical debt
- P4 style issues have minimal functional impact

**Success Validation**:
- All tests pass including new regression tests
- golangci-lint reports zero issues
- Code review confirms fixes address root causes
- Performance benchmarks meet or exceed targets
- Security audit confirms no remaining critical vulnerabilities
