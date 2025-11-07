# Tasks: Comprehensive Code Quality Improvements

**Input**: Design documents from `/specs/002-code-quality-improvements/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/, quickstart.md

**Tests**: Regression tests are REQUIRED for all fixes per Success Criteria SC-004 and SC-009.

**Organization**: Tasks are grouped by user story (priority-based) to enable systematic fixing of 94 code quality issues.

## Format: `- [ ] [ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (US1-US5)
- Include exact file paths in descriptions

## Path Conventions

Single project structure at repository root:
- `internal/` - Core implementation
- `pkg/` - Public types
- `tests/` - Test files
- `cmd/` - Command-line tools

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Prepare dependencies and tooling for code quality fixes

- [X] T001 Add github.com/Masterminds/semver/v3@v3.2.1 to go.mod for semantic versioning
- [X] T002 Add github.com/hashicorp/golang-lru/v2@v2.0.7 to go.mod for LRU cache implementation
- [X] T003 Verify golang.org/x/crypto/bcrypt is available (already indirect dependency)
- [X] T004 Run golangci-lint run to establish baseline (should show existing issues)
- [X] T005 [P] Create backup of critical files before modifications (internal/storage/, internal/embedder/, internal/indexer/)

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Core type definitions and abstractions needed by all user stories

**⚠️ CRITICAL**: No user story work can begin until this phase is complete

- [X] T006 Create IndexLock type with TryAcquire/Release methods using atomic.Int32 in internal/indexer/lock.go
- [X] T007 [P] Define querier interface in internal/storage/sqlite.go (QueryRowContext, QueryContext, ExecContext)
- [X] T008 [P] Create RetryConfig struct and retryWithBackoff function in internal/embedder/retry.go
- [X] T009 [P] Precompile FTS5 regex patterns at package level in internal/storage/vector_ops.go

**Checkpoint**: Foundation ready - user story implementation can now begin in parallel

---

## Phase 3: User Story 1 - System Stability and Security (Priority: P1) 🎯 MVP

**Goal**: Fix 4 critical security vulnerabilities and crash-inducing bugs to enable safe production deployment

**Independent Test**: Run go test -race ./..., attempt FTS5 SQL injection attacks, verify DB connection cleanup, validate bcrypt password hashing

### Tests for User Story 1

> **NOTE: Write these tests FIRST, ensure they FAIL before implementation**

- [X] T010 [P] [US1] Regression test for IndexLock concurrent acquisition in internal/indexer/indexer_test.go
- [X] T011 [P] [US1] Regression test for DB connection leak on PRAGMA failure in internal/storage/sqlite_test.go
- [X] T012 [P] [US1] Regression test for FTS5 injection prevention in internal/storage/vector_ops_test.go
- [X] T013 [P] [US1] Regression test for bcrypt password hashing in tests/testdata/fixtures/authentication_test.go

### Implementation for User Story 1

**Issue #1: Fix TryLock Compilation Error (FR-001)**
- [X] T014 [US1] Replace sync.Mutex with IndexLock in internal/indexer/indexer.go:85
- [X] T015 [US1] Update IndexProject method to use indexLock.TryAcquire() with defer Release()

**Issue #2: Fix Database Connection Leak (FR-002)**
- [X] T016 [US1] Add defer cleanup for DB connection in NewSQLiteStorage in internal/storage/sqlite.go:39
- [X] T017 [US1] Ensure all PRAGMA failures properly close database before returning error

**Issue #3: Fix FTS5 Injection Vulnerability (FR-003)**
- [X] T018 [US1] Implement sanitizeFTSQuery with proper operator escaping in internal/storage/vector_ops.go:45
- [X] T019 [US1] Escape special characters: quotes, wildcards, parentheses using precompiled patterns
- [X] T020 [US1] Escape Boolean operators: AND, OR, NOT, NEAR using pattern replacement

**Issue #4: Fix Insecure Password Hashing (FR-004)**
- [X] T021 [US1] Replace SHA-256 with bcrypt.GenerateFromPassword in tests/testdata/fixtures/authentication.go:118
- [X] T022 [US1] Implement verifyPassword using bcrypt.CompareHashAndPassword
- [X] T023 [US1] Update all password validation logic to use bcrypt comparison

**Validation**
- [X] T024 [US1] Run all US1 regression tests - verify they pass
- [X] T025 [US1] Run golangci-lint run - verify zero issues introduced
- [X] T026 [US1] Run go test -race ./internal/indexer ./internal/storage ./tests/testdata/fixtures/

**Checkpoint**: Critical security issues fixed - system can now be safely deployed

---

## Phase 4: User Story 2 - Performance and Scalability (Priority: P2)

**Goal**: Optimize algorithms and caching to handle 100k+ LOC codebases efficiently (p95 < 500ms, memory < 500MB)

**Independent Test**: Benchmark sorting with 1000+ candidates, measure cache hit rates, verify search latency targets, run memory profiling

### Tests for User Story 2

- [X] T027 [P] [US2] Regression test for nil embedder validation in internal/searcher/searcher_test.go
- [X] T028 [P] [US2] Benchmark test for sortCandidates O(n log n) vs O(n²) in internal/storage/vector_ops_test.go
- [X] T029 [P] [US2] Integration test for query result caching in internal/searcher/searcher_test.go
- [X] T030 [P] [US2] Integration test for LRU cache eviction behavior in internal/embedder/embedder_test.go
- [X] T031 [P] [US2] Test for channel timeout handling in internal/searcher/searcher_test.go

### Implementation for User Story 2

**Issue #5: Validate Embedder Before Use (FR-005)**
- [X] T032 [US2] Add nil check for s.embedder in runVectorSearch in internal/searcher/searcher.go:58
- [X] T033 [US2] Return descriptive error if embedder is nil

**Issue #7: Replace Bubble Sort (FR-007)**
- [X] T034 [US2] Replace bubble sort with sort.Slice in sortCandidates in internal/storage/vector_ops.go:204
- [X] T035 [US2] Verify sorting maintains descending score order

**Issue #6: Implement Query Caching (FR-006)**
- [X] T036 [US2] Implement checkCache to actually retrieve cached results in internal/searcher/searcher.go:295
- [X] T037 [US2] Implement storeInCache to save SearchResponse with TTL in internal/searcher/searcher.go:295
- [X] T038 [US2] Use computeQueryHash for cache key generation

**Issue #9, #20, #24, #25, #26: LRU Cache Implementation (FR-009)**
- [X] T039 [US2] Replace simple map with lru.Cache in internal/embedder/embedder.go:65
- [X] T040 [US2] Update Cache.Get to return deep copy of Embedding in internal/embedder/embedder.go:67
- [X] T041 [US2] Update Cache.Set to use LRU automatic eviction in internal/embedder/embedder.go:88
- [X] T042 [US2] Remove manual cache clearing logic (replaced by LRU)

**Issue #16, #8: Vector Search Optimization (FR-008)**
- [X] T043 [US2] Refactor searchVector to use sqlite-vec SQL-based filtering in internal/storage/vector_ops.go:21
- [X] T044 [US2] Remove in-memory loading of all embeddings
- [X] T045 [US2] Apply candidate filtering in SQL WHERE clause

**Issue #10, #22: Channel Timeout Handling (FR-010)**
- [X] T046 [US2] Add select with ctx.Done() to runVectorSearch channel send in internal/searcher/searcher.go:141
- [X] T047 [US2] Add select with ctx.Done() to runTextSearch channel send in internal/searcher/searcher.go:141

**Issue #11: Embedding Cleanup (FR-011)**
- [X] T048 [US2] Add cleanup logic for orphaned chunks when embedding fails in internal/indexer/indexer.go:193
- [X] T049 [US2] Either move embedding generation before commit OR implement rollback mechanism

**Validation**
- [X] T050 [US2] Run all US2 regression tests and benchmarks
- [X] T051 [US2] Verify sort benchmark shows O(n log n) performance
- [X] T052 [US2] Verify cache hit rate >80% for repeated queries
- [X] T053 [US2] Run go test -race ./internal/searcher ./internal/embedder ./internal/storage

**Checkpoint**: Performance optimizations complete - system handles large codebases efficiently

---

## Phase 5: User Story 3 - Data Integrity and Transaction Safety (Priority: P2)

**Goal**: Ensure correct semantic versioning for migrations and proper transaction isolation for data consistency

**Independent Test**: Run concurrent indexing operations, verify transaction reads see uncommitted writes, test migration ordering with various semver sequences

### Tests for User Story 3

- [X] T054 [P] [US3] Test semantic version comparison (1.10.0 > 1.2.0) in internal/storage/migrations_test.go
- [X] T055 [P] [US3] Test migration error handling distinguishes DB errors from "no migrations" in internal/storage/migrations_test.go
- [X] T056 [P] [US3] Test transaction isolation - reads see uncommitted writes in internal/storage/sqlite_test.go
- [X] T057 [P] [US3] Test concurrent upsert operations with atomic UPSERT clause in internal/storage/sqlite_test.go
- [X] T058 [P] [US3] Test nested transaction behavior in internal/storage/sqlite_test.go

### Implementation for User Story 3

**Issue #7, #9, #32: Semantic Versioning (FR-012)**
- [X] T059 [US3] Change Migration.Version from string to *semver.Version in internal/storage/migrations.go
- [X] T060 [US3] Update ApplyMigrations to parse version strings as semver in internal/storage/migrations.go:131
- [X] T061 [US3] Replace string comparison with semver.LessThanOrEqual() in internal/storage/migrations.go:390
- [X] T062 [US3] Handle semver parsing errors gracefully with context

**Issue #8, #13: Migration Error Handling (FR-013)**
- [X] T063 [US3] Distinguish sql.ErrNoRows from other errors in ApplyMigrations in internal/storage/migrations.go:133
- [X] T064 [US3] Return wrapped error for database failures: fmt.Errorf("failed to read schema_version: %w", err)
- [X] T065 [US3] Set currentVersion = "0.0.0" only for sql.ErrNoRows case

**Issue #11, #14, #19: Transaction Isolation (FR-014)**
- [X] T066 [US3] Refactor GetProject to internal getProjectWithQuerier accepting querier in internal/storage/sqlite.go:150
- [X] T067 [P] [US3] Refactor GetFileByID to internal getFileByIDWithQuerier accepting querier in internal/storage/sqlite.go
- [X] T068 [P] [US3] Refactor ListFiles to internal listFilesWithQuerier accepting querier in internal/storage/sqlite.go
- [X] T069 [P] [US3] Refactor SearchSymbols to internal searchSymbolsWithQuerier accepting querier in internal/storage/sqlite.go:306
- [X] T070 [P] [US3] Refactor GetStatus to internal getStatusWithQuerier accepting querier in internal/storage/sqlite.go
- [X] T071 [US3] Update public Storage methods to call internal methods with s.db
- [X] T072 [US3] Update sqliteTx methods to call internal methods with t.tx in internal/storage/sqlite.go:498,643

**Issue #11: Atomic Upserts (FR-015)**
- [X] T073 [US3] Add UNIQUE constraint to symbols table on (file_id, name, start_line, start_col) in internal/storage/migrations.go
- [X] T074 [US3] Replace check-then-act with INSERT ... ON CONFLICT DO UPDATE in upsertSymbolWithQuerier in internal/storage/sqlite.go:150
- [X] T075 [US3] Add UNIQUE constraint to chunks table on (file_id, start_line, end_line) in internal/storage/migrations.go
- [X] T076 [US3] Replace check-then-act with INSERT ... ON CONFLICT DO UPDATE in upsertChunkWithQuerier in internal/storage/sqlite.go

**Issue #12: Fix ORDER BY References (FR-016)**
- [X] T077 [US3] Fix SearchSymbols ORDER BY to use valid column or ranking function in internal/storage/sqlite.go:306

**Issue #15: Nested Transaction Handling**
- [X] T078 [US3] Ensure sqliteTx.BeginTx behavior is consistent (error or savepoint) in internal/storage/sqlite.go:799

**Validation**
- [X] T079 [US3] Run all US3 regression tests for migrations and transactions
- [X] T080 [US3] Verify migration ordering with semver test cases (1.10.0 > 1.2.0)
- [X] T081 [US3] Verify transaction isolation test passes
- [X] T082 [US3] Run go test -race ./internal/storage

**Checkpoint**: Data integrity guaranteed - migrations ordered correctly, transactions properly isolated

---

## Phase 6: User Story 4 - Code Maintainability and Consistency (Priority: P3)

**Goal**: Eliminate code duplication, implement documented features, and establish consistent patterns for easier maintenance

**Independent Test**: Code review for duplicate patterns, verify ContextAfter population or removal, check single embedder instance shared, verify regex precompilation

### Tests for User Story 4

- [X] T083 [P] [US4] Test ContextAfter field is populated or field removed from Chunk in internal/chunker/chunker_test.go
- [X] T084 [P] [US4] Test retry logic abstraction works for all providers in internal/embedder/providers_test.go
- [X] T085 [P] [US4] Test shared embedder instance between indexer and searcher in internal/mcp/server_test.go
- [X] T086 [P] [US4] Test parser extracts partial results on syntax errors in internal/parser/parser_test.go

### Implementation for User Story 4

**Issue #23: ContextAfter Population (FR-017)**
- [X] T087 [US4] Evaluate if ContextAfter feature is implemented in internal/chunker/chunker.go:138
- [X] T088 [US4] Either implement ContextAfter population using ExtractRelatedContext OR remove field from pkg/types.Chunk
- [X] T089 [US4] Update documentation to reflect actual behavior

**Issue #27: Extract Retry Logic (FR-018)**
- [X] T090 [US4] Move JinaProvider retry logic to retryWithBackoff in internal/embedder/providers.go:104
- [X] T091 [US4] Move OpenAIProvider retry logic to retryWithBackoff in internal/embedder/providers.go
- [X] T092 [US4] Update both providers to use centralized retry function

**Issue #28: Share Embedder Instance (FR-019)**
- [X] T093 [US4] Create single embedder in NewServer in internal/mcp/server.go:40
- [X] T094 [US4] Pass embedder to indexer via NewWithEmbedder constructor
- [X] T095 [US4] Pass same embedder instance to searcher constructor

**Issue #29: Precompile Regex Patterns (FR-020)**
- [X] T096 [US4] Move regex compilation to package-level vars in internal/mcp/tools.go:370
- [X] T097 [US4] Reuse precompiled patterns in sanitizeQueryForFTS

**Issue #30: Parser Partial Results (FR-021)**
- [X] T098 [US4] Remove early return after AddError in ParseFile in internal/parser/parser.go:35
- [X] T099 [US4] Continue processing partial AST even when err != nil
- [X] T100 [US4] Extract package name, imports, valid symbols from partial AST

**Issue #22: Immutable Cache Returns (FR-022)**
- [X] T101 [US4] Update Cache.Get to return deep copy of Embedding in internal/embedder/embedder.go:67
- [X] T102 [US4] Ensure Vector slice is newly allocated: append([]float32{}, cached.Vector...)

**Validation**
- [X] T103 [US4] Run all US4 regression tests
- [X] T104 [US4] Verify no code duplication in retry logic
- [X] T105 [US4] Verify embedder instance shared via code inspection
- [X] T106 [US4] Run go test ./internal/chunker ./internal/embedder ./internal/mcp ./internal/parser

**Checkpoint**: Codebase maintainability improved - consistent patterns, no duplication, features implemented

---

## Phase 7: User Story 5 - Code Style and Consistency (Priority: P4)

**Goal**: Achieve zero golangci-lint issues and consistent Go best practices throughout codebase

**Independent Test**: Run golangci-lint run with zero issues, verify gofmt compliance, check documentation completeness

### Tests for User Story 5

- [X] T107 [P] [US5] Verify SQL ORDER BY column references are valid
- [X] T108 [P] [US5] Verify consistent error wrapping with %w verb
- [X] T109 [P] [US5] Verify all exported APIs have documentation comments

### Implementation for User Story 5

**Issue #18, #19: Secure Token Generation and Validation (FR-023, FR-024)**
- [X] T110 [P] [US5] Replace SHA-256 with crypto/rand for token generation in tests/testdata/fixtures/authentication.go:66
- [X] T111 [P] [US5] Implement proper token validation against persistent storage in tests/testdata/fixtures/authentication.go:82

**Style and Consistency Fixes**
- [X] T112 [P] [US5] Fix all SQL ORDER BY column references across codebase
- [X] T113 [P] [US5] Update error wrapping to use fmt.Errorf with %w consistently
- [X] T114 [P] [US5] Add documentation comments to all exported types and functions
- [X] T115 [P] [US5] Define constants for repeated string literals (>3 occurrences)
- [X] T116 [P] [US5] Remove commented-out code and debug statements
- [X] T117 [P] [US5] Fix any remaining golangci-lint warnings

**Validation**
- [X] T118 [US5] Run golangci-lint run - verify zero issues
- [X] T119 [US5] Run gofmt -l . - verify all files formatted
- [X] T120 [US5] Run go doc for exported APIs - verify completeness
- [X] T121 [US5] Run go test ./tests/testdata/fixtures/

**Checkpoint**: Code quality excellent - zero linter issues, consistent style, complete documentation

---

## Phase 8: Polish & Cross-Cutting Concerns

**Purpose**: Final validation and improvements affecting multiple user stories

- [X] T122 [P] Run full test suite: go test ./...
- [X] T123 [P] Run tests with race detector: go test -race ./...
- [X] T124 [P] Generate coverage report: go test -cover ./... -coverprofile=coverage.out
- [X] T125 Verify coverage >80%: go tool cover -func=coverage.out | grep total
- [X] T126 [P] Run all benchmarks: go test -bench=. ./internal/searcher ./internal/storage
- [X] T127 [P] Verify sort benchmark shows O(n log n) performance improvement
- [X] T128 [P] Verify cache benchmark shows sustained hit rate >80%
- [X] T129 Update CLAUDE.md with any new patterns or lessons learned
- [X] T130 Update documentation in specs/002-code-quality-improvements/quickstart.md if implementation differed
- [X] T131 Run quickstart.md validation - verify all commands work
- [X] T132 Security audit - verify no SQL injection vulnerabilities remain
- [X] T133 Performance validation - verify search p95 < 500ms for 100k LOC codebase
- [X] T134 Memory profiling - verify usage < 500MB for 100k LOC codebase
- [X] T135 Final golangci-lint run across entire codebase - verify zero issues

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies - can start immediately
- **Foundational (Phase 2)**: Depends on Setup completion - BLOCKS all user stories
- **User Stories (Phase 3-7)**: All depend on Foundational phase completion
  - US1 (P1) → US2 (P2) → US3 (P2) → US4 (P3) → US5 (P4)
  - Or work in parallel with sufficient team capacity
- **Polish (Phase 8)**: Depends on all user stories being complete

### User Story Dependencies

- **User Story 1 (P1 - Critical)**: Can start after Foundational - No dependencies on other stories
- **User Story 2 (P2 - Performance)**: Can start after Foundational - Independent (but benefits from US1 stability)
- **User Story 3 (P2 - Data Integrity)**: Can start after Foundational - Independent (but benefits from US1 stability)
- **User Story 4 (P3 - Maintainability)**: Can start after Foundational - May reference US2/US3 fixes
- **User Story 5 (P4 - Style)**: Can start after Foundational - Affects all previous stories

### Within Each User Story

- Tests MUST be written and FAIL before implementation
- Fix issues in order listed (critical issues first within each story)
- Run story-specific tests after each fix
- Run golangci-lint after completing story
- Story complete before moving to next priority

### Parallel Opportunities

- **Phase 1 (Setup)**: All 5 tasks can run in parallel
- **Phase 2 (Foundational)**: Tasks T007, T008, T009 marked [P] can run in parallel
- **User Story Tests**: All test tasks within a story marked [P] can run in parallel
- **User Story Implementation**: Tasks marked [P] within same story can run in parallel (different files)
- **Different User Stories**: Can be worked on by different team members simultaneously (with team capacity)
- **Phase 8 (Polish)**: Most tasks marked [P] can run in parallel

---

## Parallel Example: User Story 1

```bash
# Write all US1 tests together (T010-T013):
Task T010: "Regression test for IndexLock concurrent acquisition in internal/indexer/indexer_test.go"
Task T011: "Regression test for DB connection leak in internal/storage/sqlite_test.go"
Task T012: "Regression test for FTS5 injection prevention in internal/storage/vector_ops_test.go"
Task T013: "Regression test for bcrypt password hashing in tests/testdata/fixtures/authentication_test.go"

# Verify all tests FAIL before implementation

# Then fix issues sequentially (T014-T023) - not parallel due to same files
```

---

## Parallel Example: User Story 2

```bash
# Write all US2 tests together (T027-T031):
Task T027: "Regression test for nil embedder validation"
Task T028: "Benchmark test for sortCandidates O(n log n)"
Task T029: "Integration test for query caching"
Task T030: "Integration test for LRU cache eviction"
Task T031: "Test for channel timeout handling"

# Some fixes can run in parallel (different files):
Task T032-T033: "Nil embedder check in searcher.go"
Task T034-T035: "Replace bubble sort in vector_ops.go"
Task T039-T042: "LRU cache in embedder.go"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only - Critical Security)

1. Complete Phase 1: Setup (T001-T005)
2. Complete Phase 2: Foundational (T006-T009)
3. Complete Phase 3: User Story 1 (T010-T026)
4. **STOP and VALIDATE**: Test US1 independently, verify critical issues fixed
5. Deploy if security audit passes

### Incremental Delivery

1. Complete Setup + Foundational → Foundation ready
2. Add User Story 1 → Test independently → **Deploy/Demo (MVP - Security fixed!)**
3. Add User Story 2 → Test independently → Deploy/Demo (Performance optimized)
4. Add User Story 3 → Test independently → Deploy/Demo (Data integrity guaranteed)
5. Add User Story 4 → Test independently → Deploy/Demo (Maintainability improved)
6. Add User Story 5 → Test independently → Deploy/Demo (Style perfected)
7. Each story adds value without breaking previous stories

### Parallel Team Strategy

With multiple developers:

1. Team completes Setup + Foundational together
2. Once Foundational is done:
   - Developer A: User Story 1 (P1 - Critical)
   - Developer B: User Story 2 (P2 - Performance)
   - Developer C: User Story 3 (P2 - Data Integrity)
3. Then:
   - Developer A: User Story 4 (P3)
   - Developer B: User Story 5 (P4)
4. Final: Team validates together (Phase 8)

---

## Task Summary

**Total Tasks**: 135
- Phase 1 (Setup): 5 tasks
- Phase 2 (Foundational): 4 tasks
- Phase 3 (US1 - Security): 17 tasks (4 issues)
- Phase 4 (US2 - Performance): 26 tasks (11 issues)
- Phase 5 (US3 - Data Integrity): 29 tasks (11 issues)
- Phase 6 (US4 - Maintainability): 24 tasks (6 issues)
- Phase 7 (US5 - Style): 15 tasks
- Phase 8 (Polish): 14 tasks

**Parallel Opportunities**: 43 tasks marked [P] can run in parallel

**Independent Test Criteria**:
- US1: Security audit passes, race detector clean, no SQL injection
- US2: Benchmarks show O(n log n), cache hit rate >80%, p95 < 500ms
- US3: Semver ordering correct, transaction isolation verified
- US4: No code duplication, embedder shared, features implemented
- US5: golangci-lint clean, gofmt compliant, docs complete

**Suggested MVP**: Phase 1 + Phase 2 + Phase 3 (US1 only) = Critical security fixes enabling production deployment

---

## Notes

- [P] tasks = different files, no dependencies, safe to parallelize
- [US#] label maps task to specific user story for traceability
- Each user story is independently completable and testable
- Write tests FIRST, verify they FAIL before implementation
- Commit after each fix or logical group of related fixes
- Run golangci-lint after completing each user story
- Stop at any checkpoint to validate story independently
- Priority order: P1 (Critical) → P2 (High) → P3 (Medium) → P4 (Low)
- All 94 issues from code review are addressed across the 5 user stories
