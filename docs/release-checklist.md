# Release Checklist

This document provides a comprehensive checklist for preparing and publishing a new GoContext MCP Server release.

## Version Information

- **Current Version**: 1.0.0
- **Release Date**: 2025-11-06
- **Go Version**: 1.25.4

---

## Pre-Release Checklist

### Code Quality

- [ ] **All tests pass**
  ```bash
  go test ./...
  # Expected: PASS
  ```
  - [ ] Unit tests (pkg/*, internal/*)
  - [ ] Integration tests (tests/integration/*)
  - [ ] End-to-end tests (tests/e2e/*)

- [ ] **Linter passes with no errors**
  ```bash
  golangci-lint run
  # Expected: no issues found
  ```

- [ ] **Race detector passes**
  ```bash
  go test -race ./...
  # Expected: no data races detected
  ```

- [ ] **Code coverage meets targets**
  ```bash
  go test -cover ./...
  # Expected: >80% overall coverage
  # Domain logic: >85%
  # Service layer: >80%
  ```

### Performance Validation

- [ ] **Benchmarks meet documented targets**
  ```bash
  make bench
  # Compare with docs/performance-report.md
  ```

  Performance Targets:
  - [ ] Indexing: < 5 minutes for 100k LOC ✓
  - [ ] Search latency: p95 < 500ms ✓
  - [ ] Re-indexing: < 30 seconds for 10 file changes ✓
  - [ ] Memory: < 500MB for 100k LOC ✓
  - [ ] Parsing: 100 files in < 1 second ✓

- [ ] **CPU profiling reviewed**
  ```bash
  make bench-cpu
  go tool pprof cpu.prof
  # Review top functions for hotspots
  ```

- [ ] **Memory profiling reviewed**
  ```bash
  make bench-mem
  go tool pprof mem.prof
  # Check for memory leaks or excessive allocations
  ```

### Documentation

- [ ] **README.md is up to date**
  - [ ] Installation instructions current
  - [ ] API examples work
  - [ ] Links are valid
  - [ ] Version badges updated

- [ ] **CHANGELOG.md is complete**
  - [ ] All features documented
  - [ ] Breaking changes highlighted
  - [ ] Performance improvements noted
  - [ ] Known limitations listed

- [ ] **docs/installation.md is accurate**
  - [ ] Platform-specific instructions tested
  - [ ] Download links work (after release)
  - [ ] MCP client configurations correct
  - [ ] Troubleshooting section complete

- [ ] **API documentation (godoc) is complete**
  ```bash
  go doc -all ./...
  # All exported types/functions documented
  ```

- [ ] **CLAUDE.md updated**
  - [ ] Current technologies listed
  - [ ] Build commands accurate
  - [ ] Performance targets match reality

### Security Review

- [ ] **No secrets in codebase**
  ```bash
  git grep -i "api_key\|secret\|password\|token" | grep -v "JINA_API_KEY\|OPENAI_API_KEY"
  # Expected: only environment variable references
  ```

- [ ] **Input validation on all boundaries**
  - [ ] MCP tool parameters validated
  - [ ] File paths sanitized
  - [ ] SQL uses prepared statements

- [ ] **Dependencies audited**
  ```bash
  go list -m all | nancy sleuth
  # Or use: go mod vendor && snyk test
  # Expected: no critical vulnerabilities
  ```

- [ ] **No hardcoded paths or assumptions**
  - [ ] Works on Linux, macOS, Windows
  - [ ] Respects environment variables
  - [ ] Database path configurable

### Build Verification

- [ ] **CGO build succeeds**
  ```bash
  make build
  bin/gocontext --version
  # Expected: "Build Mode: CGO with sqlite-vec"
  ```

- [ ] **Pure Go build succeeds**
  ```bash
  make build-purego
  bin/gocontext-purego --version
  # Expected: "Build Mode: Pure Go"
  ```

- [ ] **Release script works**
  ```bash
  VERSION=1.0.0 bash scripts/release.sh
  # Expected: binaries in dist/ with checksums
  ```

- [ ] **All platform binaries generated**
  - [ ] linux-amd64
  - [ ] linux-arm64
  - [ ] darwin-amd64
  - [ ] darwin-arm64
  - [ ] windows-amd64.exe
  - [ ] Pure Go variants

- [ ] **Version info embedded correctly**
  ```bash
  dist/gocontext-darwin-arm64 --version
  # Expected: Version: 1.0.0, Commit: <hash>, Build Time: <timestamp>
  ```

### Integration Testing

- [ ] **End-to-end test passes**
  ```bash
  go test -v ./tests/e2e/...
  # Expected: all subtests pass
  ```

- [ ] **Test on actual codebase**
  - [ ] Index this project
  - [ ] Search for various queries
  - [ ] Verify incremental reindex works
  - [ ] Check status tool

- [ ] **MCP client integration tested**
  - [ ] Claude Code configuration works
  - [ ] Stdio communication verified
  - [ ] All three tools functional (index, search, status)

- [ ] **Cross-platform testing**
  - [ ] macOS (Intel and Apple Silicon if available)
  - [ ] Linux (Ubuntu/Debian tested)
  - [ ] Windows (if possible)

### Git Hygiene

- [ ] **All changes committed**
  ```bash
  git status
  # Expected: working tree clean
  ```

- [ ] **On correct branch**
  ```bash
  git branch
  # Expected: main or release branch
  ```

- [ ] **All tests pass in CI**
  - [ ] GitHub Actions successful
  - [ ] All jobs green

- [ ] **Version tag not yet created**
  ```bash
  git tag | grep v1.0.0
  # Expected: empty (we'll create this during release)
  ```

---

## Release Process

### 1. Final Preparation

- [ ] **Update version in files**
  - [ ] `go.mod` (if needed)
  - [ ] `CHANGELOG.md` (update release date)
  - [ ] `README.md` (version badges)

- [ ] **Commit all final changes**
  ```bash
  git add .
  git commit -m "Release v1.0.0"
  git push origin main
  ```

### 2. Build Release Artifacts

- [ ] **Run release script**
  ```bash
  VERSION=1.0.0 bash scripts/release.sh
  ```

- [ ] **Verify all binaries work**
  ```bash
  # Test each binary
  dist/gocontext-darwin-arm64 --version
  dist/gocontext-linux-amd64 --version
  # etc.
  ```

- [ ] **Verify checksums**
  ```bash
  cd dist
  sha256sum -c checksums.txt
  # or shasum -a 256 -c checksums.txt on macOS
  ```

### 3. Create GitHub Release

- [ ] **Create and push tag**
  ```bash
  git tag -a v1.0.0 -m "Release v1.0.0"
  git push origin v1.0.0
  ```

- [ ] **Create GitHub release**
  1. Go to https://github.com/dshills/gocontext-mcp/releases/new
  2. Select tag: v1.0.0
  3. Release title: "GoContext MCP Server v1.0.0"
  4. Copy content from `dist/RELEASE_NOTES.md`
  5. Upload binaries from `dist/`:
     - All `gocontext-*` binaries
     - `checksums.txt`
     - `VERSION.txt`

- [ ] **Mark as latest release**

- [ ] **Verify release page**
  - [ ] All binaries downloadable
  - [ ] Checksums correct
  - [ ] Release notes formatted properly

### 4. Post-Release

- [ ] **Announce release**
  - [ ] GitHub Discussions post
  - [ ] Social media (if applicable)
  - [ ] MCP community channels

- [ ] **Update documentation links**
  - [ ] README.md download links point to v1.0.0
  - [ ] docs/installation.md download links updated

- [ ] **Create milestone for next version**
  - [ ] GitHub milestone for v1.1.0
  - [ ] Move unfinished issues to next milestone

- [ ] **Monitor for issues**
  - [ ] Watch GitHub Issues
  - [ ] Test installation on fresh systems
  - [ ] Respond to user feedback

### 5. Optional: Package Distribution

- [ ] **Homebrew formula** (future)
  ```bash
  # Submit to homebrew-core or create tap
  # Formula in homebrew-tap repo
  ```

- [ ] **Docker image** (future)
  ```bash
  docker build -t gocontext:1.0.0 .
  docker tag gocontext:1.0.0 ghcr.io/dshills/gocontext-mcp:1.0.0
  docker tag gocontext:1.0.0 ghcr.io/dshills/gocontext-mcp:latest
  docker push ghcr.io/dshills/gocontext-mcp:1.0.0
  docker push ghcr.io/dshills/gocontext-mcp:latest
  ```

- [ ] **Go package** (automatic)
  ```bash
  # Users can install with:
  go install github.com/dshills/gocontext-mcp/cmd/gocontext@v1.0.0
  # or
  go install github.com/dshills/gocontext-mcp/cmd/gocontext@latest
  ```

---

## Rollback Plan

If critical issues are discovered after release:

### Option 1: Hotfix Release (Minor Issues)

1. Create hotfix branch: `git checkout -b hotfix/v1.0.1`
2. Fix issue and test thoroughly
3. Update CHANGELOG.md
4. Follow release process for v1.0.1
5. Mark v1.0.0 as deprecated in release notes

### Option 2: Yank Release (Critical Issues)

1. Mark release as pre-release on GitHub
2. Update release notes with warning
3. Pin v0.9.x as latest (if exists)
4. Communicate issue to users
5. Prepare fixed release ASAP

### Option 3: Patch and Re-release

1. Delete tag: `git tag -d v1.0.0 && git push origin :refs/tags/v1.0.0`
2. Fix issue and test
3. Re-create tag and release
4. Note: Avoid this if users have downloaded - prefer hotfix

---

## Post-1.0.0 Planning

### Future Releases

**v1.1.0 (Minor)** - Planned features:
- [ ] Python language support
- [ ] Remote repository indexing
- [ ] Multi-project workspace
- [ ] Enhanced reranking models

**v1.0.x (Patches)** - Bug fixes only:
- [ ] Critical bug fixes
- [ ] Security patches
- [ ] Documentation corrections

**v2.0.0 (Major)** - Breaking changes:
- [ ] API redesign
- [ ] Database schema changes
- [ ] New architecture

### Maintenance

- [ ] Regular dependency updates
- [ ] Security advisory monitoring
- [ ] Performance optimization
- [ ] Bug triage and fixes
- [ ] Community support

---

## Checklist Summary

**Total Items**: 89
**Critical Items**: 35
**Optional Items**: 15

### Critical Path (Must Complete)

1. All tests pass ✓
2. Linter clean ✓
3. Benchmarks meet targets ✓
4. Documentation complete ✓
5. Security review done ✓
6. Build verified ✓
7. E2E tests pass ✓
8. Git hygiene ✓
9. Release artifacts built ✓
10. GitHub release created ✓

### Sign-Off

- [ ] **Lead Developer**: Doug Shills (@dshills)
- [ ] **QA**: Tests passing, coverage >80%
- [ ] **Security**: No vulnerabilities, secrets removed
- [ ] **Documentation**: README, CHANGELOG, installation guide
- [ ] **Build**: All platforms successful

**Release Status**: ✓ READY FOR RELEASE

---

**Last Updated**: 2025-11-06
**Next Review**: Before v1.1.0 release
