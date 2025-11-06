# Quickstart Guide: GoContext MCP Server

**Date**: 2025-11-06
**Feature**: GoContext MCP Server
**Audience**: Developers using Claude Code or Codex CLI with Go projects

## Overview

GoContext MCP Server enables semantic code search for Go codebases within AI coding assistants. This guide walks through installation, configuration, and usage.

**What you'll learn**:
- How to install and configure GoContext
- How to index your Go codebase
- How to search code with Claude Code
- Troubleshooting common issues

**Time to complete**: 10 minutes

---

## Prerequisites

### System Requirements

- **Operating System**: Linux, macOS, or Windows
- **Go Version**: 1.21 or later (for codebases to be indexed)
- **Disk Space**: ~10-20% of your codebase size
- **Memory**: 500MB available during indexing

### Optional Requirements

For vector embeddings (semantic search):
- **Internet connection** (initial setup)
- **API Key**: Jina AI or OpenAI account (free tier sufficient)

**OR**

- **Local embedding model** (for fully offline operation)

---

## Installation

### Option 1: Binary Release (Recommended)

Download the latest release for your platform:

```bash
# macOS (Apple Silicon)
curl -L https://github.com/yourorg/gocontext/releases/latest/download/gocontext-darwin-arm64 -o gocontext
chmod +x gocontext
sudo mv gocontext /usr/local/bin/

# macOS (Intel)
curl -L https://github.com/yourorg/gocontext/releases/latest/download/gocontext-darwin-amd64 -o gocontext
chmod +x gocontext
sudo mv gocontext /usr/local/bin/

# Linux
curl -L https://github.com/yourorg/gocontext/releases/latest/download/gocontext-linux-amd64 -o gocontext
chmod +x gocontext
sudo mv gocontext /usr/local/bin/

# Windows (PowerShell)
Invoke-WebRequest -Uri "https://github.com/yourorg/gocontext/releases/latest/download/gocontext-windows-amd64.exe" -OutFile "gocontext.exe"
# Move to PATH location
```

**Verify installation**:
```bash
gocontext --version
# Expected output: gocontext version 1.0.0
```

### Option 2: Build from Source

Requires Go 1.21+ and C compiler (for CGO build with vector extension):

```bash
# Clone repository
git clone https://github.com/yourorg/gocontext.git
cd gocontext

# Build with CGO (recommended - includes vector search)
CGO_ENABLED=1 go build -tags "sqlite_vec" -o gocontext ./cmd/gocontext

# OR: Pure Go build (no vector search, keyword-only)
CGO_ENABLED=0 go build -tags "purego" -o gocontext ./cmd/gocontext

# Install
sudo mv gocontext /usr/local/bin/
```

---

## Configuration

### Step 1: Configure Embedding Provider (Optional but Recommended)

For semantic search, configure an embedding provider:

**Option A: Jina AI** (recommended, optimized for code):
```bash
export JINA_API_KEY="your-jina-api-key-here"
```

Get free API key: https://jina.ai/embeddings/

**Option B: OpenAI**:
```bash
export OPENAI_API_KEY="your-openai-api-key-here"
export GOCONTEXT_EMBEDDING_PROVIDER="openai"
```

**Option C: Offline Mode** (no API key required):
```bash
export GOCONTEXT_EMBEDDING_PROVIDER="local"
```
Note: Local embeddings have lower quality but work fully offline.

**Environment Variables Reference**:

| Variable | Description | Default | Examples |
|----------|-------------|---------|----------|
| `JINA_API_KEY` | Jina AI API key for embeddings | None | `jina_abc123...` |
| `OPENAI_API_KEY` | OpenAI API key for embeddings | None | `sk-abc123...` |
| `GOCONTEXT_EMBEDDING_PROVIDER` | Explicit provider selection | Auto-detect | `jina`, `openai`, `local` |
| `GOCONTEXT_DB_PATH` | Custom database location | `~/.gocontext/indices` | `/custom/path` |

**Provider Auto-Detection Logic**:
1. If `GOCONTEXT_EMBEDDING_PROVIDER` is set → use specified provider
2. Else if `JINA_API_KEY` is set → use Jina AI
3. Else if `OPENAI_API_KEY` is set → use OpenAI
4. Else → fallback to `local` provider (offline mode)

**Important**: When using MCP, set environment variables in the MCP configuration file, NOT in your shell profile.

### Step 2: Configure Claude Code Integration

Add GoContext to your Claude Code MCP servers:

**Configuration File Location**:
- **macOS**: `~/Library/Application Support/Claude/claude_desktop_config.json`
- **Linux**: `~/.config/Claude/claude_desktop_config.json`
- **Windows**: `%APPDATA%\Claude\claude_desktop_config.json`

**Example Configuration**:

```json
{
  "mcpServers": {
    "gocontext": {
      "command": "/usr/local/bin/gocontext",
      "args": ["serve"],
      "env": {
        "JINA_API_KEY": "jina_xxxxxxxxxxxxxxxxxxxx"
      }
    }
  }
}
```

**Important Configuration Notes**:
1. Use **absolute path** to gocontext binary (not just `gocontext`)
2. Find your binary location: `which gocontext` or `whereis gocontext`
3. Only use `"serve"` argument (not additional flags)
4. API key should be in `env` section (not system environment)

**Alternative: Using OpenAI**:
```json
{
  "mcpServers": {
    "gocontext": {
      "command": "/usr/local/bin/gocontext",
      "args": ["serve"],
      "env": {
        "OPENAI_API_KEY": "sk-xxxxxxxxxxxxxxxxxxxx",
        "GOCONTEXT_EMBEDDING_PROVIDER": "openai"
      }
    }
  }
}
```

**Alternative: Offline Mode (No API Key)**:
```json
{
  "mcpServers": {
    "gocontext": {
      "command": "/usr/local/bin/gocontext",
      "args": ["serve"],
      "env": {
        "GOCONTEXT_EMBEDDING_PROVIDER": "local"
      }
    }
  }
}
```

**Custom Database Location** (Optional):
```json
{
  "mcpServers": {
    "gocontext": {
      "command": "/usr/local/bin/gocontext",
      "args": ["serve"],
      "env": {
        "JINA_API_KEY": "your-api-key-here",
        "GOCONTEXT_DB_PATH": "/custom/path/to/indices"
      }
    }
  }
}
```

**After Configuration**:
1. Save the configuration file
2. **Restart Claude Desktop** completely (Cmd+Q on macOS, then reopen)
3. Verify connection in Claude Code by asking: "What MCP servers are available?"

---

## Usage

### Index Your First Codebase

**Example: Index a Go project**

```bash
# Navigate to your Go project
cd /path/to/your/go/project

# Index the codebase
gocontext index .

# Expected output:
# Indexing project: /path/to/your/go/project
# Module: github.com/yourorg/yourproject
# Found 247 Go files
# Parsing files... ████████████████████ 100% (247/247)
# Generating embeddings... ████████████████████ 100% (1834/1834)
# ✓ Indexed 247 files, 1834 chunks in 32.5s
# Index stored at: ~/.gocontext/indices/abc123def456.db
```

**Incremental re-indexing** (after code changes):
```bash
gocontext index .
# Only changed files are re-indexed
# ✓ Re-indexed 3 files (244 skipped) in 4.2s
```

**Force full re-index**:
```bash
gocontext index --force .
# Rebuilds entire index from scratch
```

### Search from Claude Code

Once indexed, use Claude Code to search your codebase:

**Example conversations with Claude Code**:

1. **Semantic search**:
   ```
   You: "Find the authentication logic in this codebase"

   Claude: *searches using GoContext*
   I found the authentication logic in internal/auth/service.go:

   func AuthenticateUser(username, password string) (*User, error) {
       // ... implementation
   }

   This function handles user credential verification...
   ```

2. **Symbol search**:
   ```
   You: "Where is the ParseFile function defined?"

   Claude: *searches using GoContext*
   The ParseFile function is defined in internal/parser/parser.go:45

   func ParseFile(path string) (*ParseResult, error) {
       // ... implementation
   }
   ```

3. **Domain pattern search**:
   ```
   You: "Show me all repository implementations"

   Claude: *searches for repository pattern*
   I found 3 repository implementations:
   1. UserRepository (internal/users/repository.go)
   2. OrderRepository (internal/orders/repository.go)
   3. ProductRepository (internal/products/repository.go)
   ```

### Command-Line Search (Direct)

You can also search directly from command line:

```bash
# Semantic search
gocontext search "user authentication logic"

# Symbol search
gocontext search "func ParseFile"

# Limit results
gocontext search "error handling" --limit 5

# Filter by symbol type
gocontext search "validation" --type function,method

# Search specific directory
gocontext search "database" --path internal/db

# Keyword-only search (faster, no embeddings)
gocontext search "ParseFile" --mode keyword
```

---

## Project Status

Check indexing status for a project:

```bash
gocontext status /path/to/project

# Output:
# Project: github.com/yourorg/yourproject
# Status: Indexed
# Files: 247 indexed, 3 failed
# Symbols: 1,834 extracted
# Chunks: 1,834 created
# Embeddings: 1,834 generated
# Index size: 45.2 MB
# Last indexed: 2025-11-06 10:30:45
```

---

## Troubleshooting

### MCP Server Connection Issues

#### Issue: Claude Code doesn't show GoContext server

**Symptoms**: When you ask Claude "What MCP servers are available?", gocontext is not listed.

**Solutions**:

1. **Check configuration file location**:
   ```bash
   # macOS
   cat ~/Library/Application\ Support/Claude/claude_desktop_config.json

   # Linux
   cat ~/.config/Claude/claude_desktop_config.json
   ```

2. **Verify JSON is valid**:
   - No trailing commas
   - All quotes are double quotes (not single)
   - Use a JSON validator: https://jsonlint.com

3. **Check binary path is absolute**:
   ```bash
   # Find your gocontext binary
   which gocontext
   # Or if built from source:
   ls /path/to/gocontext-mcp/bin/gocontext
   ```

4. **Test binary works independently**:
   ```bash
   /usr/local/bin/gocontext --version
   # Should print version information
   ```

5. **Check Claude Desktop logs** (macOS):
   ```bash
   # View logs in Console.app
   # Filter for "Claude" process
   # Look for MCP connection errors

   # Or check stderr output:
   tail -f ~/Library/Logs/Claude/mcp-server-gocontext.log
   ```

6. **Restart Claude Desktop completely**:
   - Quit Claude Desktop (Cmd+Q on macOS, not just close window)
   - Wait 5 seconds
   - Reopen Claude Desktop

#### Issue: "gocontext: command not found" in MCP logs

**Cause**: Binary path is not absolute or incorrect.

**Solution**: Update configuration with full path:
```json
{
  "mcpServers": {
    "gocontext": {
      "command": "/Users/yourusername/path/to/gocontext",
      "args": ["serve"]
    }
  }
}
```

#### Issue: "Failed to initialize storage" error

**Cause**: Database directory not writable or disk full.

**Solutions**:
1. **Check disk space**:
   ```bash
   df -h ~
   ```

2. **Check directory permissions**:
   ```bash
   ls -la ~/.gocontext/
   # Should be owned by your user
   ```

3. **Manually create directory**:
   ```bash
   mkdir -p ~/.gocontext/indices
   chmod 755 ~/.gocontext/indices
   ```

4. **Use custom database path**:
   Add to MCP configuration:
   ```json
   "env": {
     "GOCONTEXT_DB_PATH": "/tmp/gocontext-indices"
   }
   ```

#### Issue: "API key missing or invalid" error

**Symptoms**: Indexing starts but fails with embedding errors.

**Solutions**:

1. **Verify API key in MCP config** (not system environment):
   ```json
   {
     "mcpServers": {
       "gocontext": {
         "command": "/usr/local/bin/gocontext",
         "args": ["serve"],
         "env": {
           "JINA_API_KEY": "jina_xxxx"  ← Should be here
         }
       }
     }
   }
   ```

2. **Test API key manually**:
   ```bash
   # Test Jina API
   curl -X POST 'https://api.jina.ai/v1/embeddings' \
     -H 'Content-Type: application/json' \
     -H 'Authorization: Bearer YOUR_API_KEY' \
     -d '{"input": ["test"], "model": "jina-embeddings-v3"}'
   ```

3. **Use offline mode instead**:
   ```json
   "env": {
     "GOCONTEXT_EMBEDDING_PROVIDER": "local"
   }
   ```

### Indexing Issues

### Issue: "Project not indexed" error

**Solution**: Run indexing first:
```bash
cd /path/to/project
gocontext index .
```

**Or ask Claude to index**:
```
Claude, please index this Go project at /path/to/project
```

### Issue: Slow indexing (>10 minutes for 100k LOC)

**Possible causes**:
1. **Slow internet**: Embedding API calls are slow
   - Solution: Use `--batch-size 50` to increase batch size

2. **Many files changed**: Incremental indexing helps
   - Solution: Subsequent runs will be faster

3. **Disk I/O bottleneck**: Check disk performance
   - Solution: Use SSD, close other I/O-heavy programs

### Issue: Search returns no results

**Debugging steps**:

1. **Check project is indexed**:
   ```bash
   gocontext status /path/to/project
   ```

2. **Verify embeddings generated**:
   ```bash
   gocontext status /path/to/project | grep "Embeddings"
   # Should show: Embeddings: 1,834 generated
   ```

3. **Try keyword search** (no embeddings required):
   ```bash
   gocontext search "your query" --mode keyword
   ```

4. **Check API key** (if using Jina/OpenAI):
   ```bash
   echo $JINA_API_KEY  # or $OPENAI_API_KEY
   # Should print your API key
   ```

### Issue: Parse errors during indexing

**Example**:
```
⚠ Failed to parse 2 files:
  - internal/broken/file.go: syntax error at line 45
```

**Solution**: This is expected for files with syntax errors. GoContext will:
- Index all valid files successfully
- Track failed files in the database
- Continue indexing other files

To fix:
1. Correct syntax errors in failed files
2. Re-run indexing: `gocontext index .`

### Issue: High memory usage during indexing

**Cause**: Large codebase (>500k LOC) or large individual files

**Solutions**:
1. **Increase system memory** (recommended minimum: 2GB)
2. **Index in chunks**: Index subdirectories separately
   ```bash
   gocontext index ./internal
   gocontext index ./pkg
   gocontext index ./cmd
   ```
3. **Reduce concurrency**:
   ```bash
   gocontext index --workers 2 .
   ```

### Issue: "Database is locked" error

**Cause**: Multiple indexing operations running simultaneously

**Solution**: Wait for current indexing to complete, or:
```bash
# Find and stop running gocontext process
ps aux | grep gocontext
kill <PID>

# Then re-run indexing
gocontext index .
```

---

## Performance Tuning

### Faster Indexing

```bash
# Use more workers (default: CPU count)
gocontext index --workers 8 .

# Larger embedding batches (faster API calls)
gocontext index --batch-size 100 .

# Skip tests (if you don't need them indexed)
gocontext index --exclude-tests .

# Skip vendor directory (third-party code)
gocontext index --exclude-vendor .
```

### Faster Search

```bash
# Use keyword search for exact matches (no embedding needed)
gocontext search "exact function name" --mode keyword

# Reduce result limit (less processing)
gocontext search "query" --limit 5

# Use cache (automatically enabled, clears after 1 hour)
```

---

## Data Storage

### Index Location

Indexes are stored in:
- **Linux/macOS**: `~/.gocontext/indices/`
- **Windows**: `%USERPROFILE%\.gocontext\indices\`

Each project gets a unique database file: `<project-hash>.db`

### Backup

```bash
# Backup all indexes
cp -r ~/.gocontext/indices ~/.gocontext/indices.backup

# Backup specific project
cp ~/.gocontext/indices/abc123def456.db ~/backups/
```

### Clean Up

```bash
# Remove all indexes
rm -rf ~/.gocontext/indices

# Remove specific project index
gocontext delete /path/to/project
```

---

## Offline Operation

GoContext supports full offline operation with no network dependencies.

### Understanding Offline Modes

**Three Operational Modes**:

1. **Online Mode** (Default with API key)
   - Uses Jina AI or OpenAI for embeddings
   - Best search quality (~90% accuracy)
   - Requires internet for indexing only
   - Search works offline after indexing

2. **Local Embeddings Mode** (Offline-capable)
   - Uses built-in local embedding model
   - Good search quality (~75-80% accuracy)
   - No internet required at any time
   - Slightly slower indexing

3. **Keyword-Only Mode** (Fallback)
   - Uses BM25 full-text search only
   - No embeddings needed
   - Fast but less semantic understanding
   - Always available as fallback

### Fully Offline Setup (Recommended for Air-Gapped Environments)

**Step 1: Build with Pure Go (No CGO Dependencies)**

```bash
# Build pure Go binary (includes local embeddings)
CGO_ENABLED=0 go build -tags "purego" -o gocontext ./cmd/gocontext

# Verify build
./gocontext --version
# Should show: "Vector Extension: false" for pure Go build
```

**Step 2: Configure for Offline Operation**

MCP configuration (`claude_desktop_config.json`):
```json
{
  "mcpServers": {
    "gocontext": {
      "command": "/path/to/gocontext",
      "args": ["serve"],
      "env": {
        "GOCONTEXT_EMBEDDING_PROVIDER": "local"
      }
    }
  }
}
```

**Step 3: Index Projects Offline**

```bash
# Set offline mode
export GOCONTEXT_EMBEDDING_PROVIDER="local"

# Index projects (no network calls)
gocontext index /path/to/project1
gocontext index /path/to/project2

# Verify no network calls with monitoring tool:
# sudo tcpdump -i any port 443  # Should show no HTTPS traffic
```

### Hybrid Setup: Online Indexing, Offline Search

**Best of both worlds**: Index with API (high quality), search offline:

**Step 1: Index with API (Online)**
```bash
export JINA_API_KEY="your-api-key"
gocontext index /path/to/project
```

**Step 2: Search Works Offline**
```bash
# No API key needed for search
unset JINA_API_KEY
gocontext search "authentication logic"
# Uses cached embeddings from indexing phase
```

**Step 3: MCP Configuration**
```json
{
  "mcpServers": {
    "gocontext": {
      "command": "/path/to/gocontext",
      "args": ["serve"],
      "env": {
        "GOCONTEXT_EMBEDDING_PROVIDER": "local"
      }
    }
  }
}
```

### Auto-Detection of Offline Mode

GoContext automatically detects and adapts to offline conditions:

**Detection Logic**:
1. Check for `GOCONTEXT_EMBEDDING_PROVIDER=local` environment variable
2. Check for API keys (`JINA_API_KEY`, `OPENAI_API_KEY`)
3. If no provider and no keys → Auto-enable local provider
4. If API call fails → Fallback to keyword-only search

**Status Check**:
```bash
gocontext status /path/to/project
# Shows:
# - Embeddings available: Yes/No
# - Provider: jina/openai/local
# - Offline capable: Yes/No
```

### Performance Comparison

| Mode | Indexing Speed | Search Quality | Network Required |
|------|----------------|----------------|------------------|
| Jina AI (Online) | Fast (API batch) | 90% | Indexing only |
| OpenAI (Online) | Fast (API batch) | 88% | Indexing only |
| Local Embeddings | Medium (CPU) | 75-80% | Never |
| Keyword Only | Fastest | 60% | Never |

**Benchmark Results** (100k LOC codebase):
- **Jina AI**: 3.5 min indexing, p95 < 400ms search
- **Local**: 8 min indexing, p95 < 500ms search
- **Keyword**: 1 min indexing, p95 < 200ms search

### Verifying Offline Operation

**Test 1: Network Isolation**
```bash
# Disconnect network or use network namespace
sudo unshare -n gocontext index /path/to/test-project
# Should succeed with local provider
```

**Test 2: Monitor Network Calls**
```bash
# Terminal 1: Monitor network
sudo tcpdump -i any host api.jina.ai or host api.openai.com

# Terminal 2: Index with local provider
export GOCONTEXT_EMBEDDING_PROVIDER="local"
gocontext index /path/to/project

# Should see ZERO packets to API endpoints
```

**Test 3: Verify Pure Go Build**
```bash
# Check binary has no CGO dependencies
ldd gocontext-purego
# Should show: "not a dynamic executable" (static binary)

# Or on macOS:
otool -L gocontext-purego
# Should show only system libraries
```

### Offline Mode Limitations

**Current Limitations**:
1. Local embeddings have 10-15% lower accuracy than Jina/OpenAI
2. Pure Go build has slightly slower vector operations
3. Local model increases binary size by ~50MB
4. Cannot use Jina/OpenAI reranking features offline

**Mitigations**:
- Hybrid approach: Index online, search offline
- Use keyword search for exact symbol lookups
- Combine search modes: Run both semantic and keyword
- Re-index periodically with API for best quality

### Air-Gapped Deployment Checklist

For completely isolated environments:

- [ ] Build pure Go binary (`CGO_ENABLED=0`)
- [ ] Copy binary to air-gapped system
- [ ] Set `GOCONTEXT_EMBEDDING_PROVIDER=local`
- [ ] Test indexing on sample project
- [ ] Verify no network calls with `tcpdump`
- [ ] Configure MCP client with local provider
- [ ] Document performance differences for users
- [ ] Set up periodic re-indexing for code changes

### Fallback Behavior

GoContext gracefully degrades when features are unavailable:

**Embedding Generation Fails**:
```
⚠ Warning: Failed to generate embeddings (network error)
→ Continuing with keyword-only indexing
→ Search will use BM25 full-text search
```

**Vector Extension Unavailable (Pure Go Build)**:
```
ℹ Info: Vector extension not available
→ Using pure Go vector operations
→ Search may be 20-30% slower for large codebases
```

**No Provider Configured**:
```
ℹ Info: No embedding provider configured
→ Auto-detected: local provider
→ Indexing with built-in embeddings
```

These fallbacks ensure GoContext remains functional in any environment.

---

## Next Steps

### Advanced Usage

- **Custom filters**: Filter by DDD patterns, symbol types
  ```bash
  gocontext search "repositories" --ddd repository,service
  ```

- **Multi-project search**: Index multiple projects, search across all
  ```bash
  gocontext search "common utility" --all-projects
  ```

- **Export results**: Save search results to file
  ```bash
  gocontext search "query" --output results.json
  ```

### Integration with Other Tools

- **Codex CLI**: Same MCP configuration as Claude Code
- **Custom scripts**: Use `gocontext` CLI in shell scripts
- **CI/CD**: Index codebase in CI for documentation generation

### Learning Resources

- **Full documentation**: [Link to docs]
- **Architecture guide**: See `specs/gocontext-mcp_spec.md`
- **API contracts**: See `specs/001-gocontext-mcp-server/contracts/`
- **GitHub issues**: Report bugs or request features

---

## Common Workflows

### Workflow 1: Daily Development

```bash
# Morning: Update index after pulling changes
cd /path/to/project
git pull
gocontext index .  # Incremental, only ~5-10s

# During development: Search as needed via Claude Code
# (No manual commands - just talk to Claude)

# Evening: Commit changes (index updates automatically on next run)
```

### Workflow 2: Onboarding to New Codebase

```bash
# Clone repository
git clone https://github.com/yourorg/newproject.git
cd newproject

# Index entire codebase
gocontext index .
# First indexing: ~2-5 minutes for typical project

# Ask Claude Code exploratory questions:
# "What are the main components of this system?"
# "Show me the database schema definitions"
# "Where is the authentication logic?"
```

### Workflow 3: Code Review

```bash
# Checkout PR branch
git fetch origin pull/123/head:pr-123
git checkout pr-123

# Re-index to include PR changes
gocontext index .

# Ask Claude Code about changes:
# "What does this PR change?"
# "Find the functions modified in this PR"
# "Are there any similar patterns elsewhere in the codebase?"
```

---

## FAQ

**Q: Does GoContext modify my code?**
A: No, GoContext is read-only. It only indexes and searches code.

**Q: Can I use GoContext with non-Go code?**
A: Not currently. GoContext v1.0 only supports Go codebases.

**Q: How much does it cost?**
A: GoContext is free and open source. Embedding APIs (Jina/OpenAI) have free tiers sufficient for most projects.

**Q: Does GoContext send my code to external services?**
A: Only code chunks are sent to embedding APIs (Jina/OpenAI) to generate vectors. You can use local embeddings for fully offline operation.

**Q: Can I index multiple projects?**
A: Yes, each project gets its own index. Search across projects is supported with `--all-projects` flag.

**Q: How do I update GoContext?**
A: Download the latest binary and replace the existing one. Indexes are forward-compatible.

**Q: Can I use GoContext in CI/CD?**
A: Yes, use `gocontext index` in CI to keep indexes up-to-date automatically.

---

## Support

- **Issues**: https://github.com/yourorg/gocontext/issues
- **Discussions**: https://github.com/yourorg/gocontext/discussions
- **Documentation**: https://gocontext.dev/docs

**Status**: Phase 1 Quickstart Guide Complete ✅
