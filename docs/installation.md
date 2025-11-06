# Installation Guide

This guide provides detailed installation instructions for the GoContext MCP Server across different platforms and build configurations.

## Table of Contents

- [Quick Installation](#quick-installation)
- [Platform-Specific Instructions](#platform-specific-instructions)
  - [macOS](#macos)
  - [Linux](#linux)
  - [Windows](#windows)
- [Build from Source](#build-from-source)
- [CGO vs Pure Go Builds](#cgo-vs-pure-go-builds)
- [MCP Client Configuration](#mcp-client-configuration)
- [Environment Variables](#environment-variables)
- [Verification](#verification)
- [Troubleshooting](#troubleshooting)

---

## Quick Installation

### Option 1: Download Pre-built Binary (Recommended)

1. Visit the [releases page](https://github.com/dshills/gocontext-mcp/releases)
2. Download the appropriate binary for your platform:
   - **macOS Apple Silicon**: `gocontext-darwin-arm64`
   - **macOS Intel**: `gocontext-darwin-amd64`
   - **Linux x86_64**: `gocontext-linux-amd64`
   - **Linux ARM64**: `gocontext-linux-arm64`
   - **Windows**: `gocontext-windows-amd64.exe`

3. Verify the download (recommended):
   ```bash
   # Download checksums
   curl -LO https://github.com/dshills/gocontext-mcp/releases/download/v1.0.0/checksums.txt

   # Verify (Linux)
   sha256sum -c --ignore-missing checksums.txt

   # Verify (macOS)
   shasum -a 256 -c checksums.txt
   ```

4. Make executable and install:
   ```bash
   chmod +x gocontext-*
   sudo mv gocontext-* /usr/local/bin/gocontext
   ```

### Option 2: Install with Go

```bash
go install github.com/dshills/gocontext-mcp/cmd/gocontext@latest
```

**Note**: This builds from source and requires Go 1.25.4+. The build mode (CGO vs Pure Go) depends on your `CGO_ENABLED` environment variable.

### Option 3: Build from Source

```bash
# Clone repository
git clone https://github.com/dshills/gocontext-mcp.git
cd gocontext-mcp

# Build (CGO with vector extension)
make build

# Or build pure Go version
make build-purego

# Binary available at bin/gocontext
```

---

## Platform-Specific Instructions

### macOS

#### Prerequisites

**For CGO Build** (recommended):
- Go 1.25.4 or later
- Xcode Command Line Tools: `xcode-select --install`

**For Pure Go Build**:
- Go 1.25.4 or later only

#### Installation Steps

1. **Download Binary**:
   ```bash
   # Apple Silicon (M1/M2/M3)
   curl -LO https://github.com/dshills/gocontext-mcp/releases/download/v1.0.0/gocontext-darwin-arm64

   # Intel Macs
   curl -LO https://github.com/dshills/gocontext-mcp/releases/download/v1.0.0/gocontext-darwin-amd64
   ```

2. **Verify and Install**:
   ```bash
   # Verify checksum
   curl -LO https://github.com/dshills/gocontext-mcp/releases/download/v1.0.0/checksums.txt
   shasum -a 256 -c checksums.txt

   # Make executable
   chmod +x gocontext-darwin-*

   # Move to PATH
   sudo mv gocontext-darwin-* /usr/local/bin/gocontext
   ```

3. **Handle macOS Gatekeeper** (if needed):
   ```bash
   # If you get "unidentified developer" warning
   xattr -d com.apple.quarantine /usr/local/bin/gocontext

   # Or allow in System Preferences > Security & Privacy
   ```

4. **Verify Installation**:
   ```bash
   gocontext --version
   ```

#### Homebrew Installation (Future)

```bash
# Coming soon
brew install dshills/tap/gocontext
```

### Linux

#### Prerequisites

**For CGO Build** (recommended):
- Go 1.25.4 or later
- GCC: `sudo apt-get install build-essential` (Debian/Ubuntu) or `sudo yum groupinstall "Development Tools"` (RHEL/CentOS)

**For Pure Go Build**:
- Go 1.25.4 or later only

#### Installation Steps

1. **Download Binary**:
   ```bash
   # x86_64
   curl -LO https://github.com/dshills/gocontext-mcp/releases/download/v1.0.0/gocontext-linux-amd64

   # ARM64 (Raspberry Pi, AWS Graviton, etc.)
   curl -LO https://github.com/dshills/gocontext-mcp/releases/download/v1.0.0/gocontext-linux-arm64
   ```

2. **Verify and Install**:
   ```bash
   # Verify checksum
   curl -LO https://github.com/dshills/gocontext-mcp/releases/download/v1.0.0/checksums.txt
   sha256sum -c --ignore-missing checksums.txt

   # Make executable
   chmod +x gocontext-linux-*

   # Move to PATH (system-wide)
   sudo mv gocontext-linux-* /usr/local/bin/gocontext

   # Or install for current user only
   mkdir -p ~/.local/bin
   mv gocontext-linux-* ~/.local/bin/gocontext
   # Ensure ~/.local/bin is in PATH
   ```

3. **Verify Installation**:
   ```bash
   gocontext --version
   ```

#### Docker Installation

```bash
# Pull image
docker pull ghcr.io/dshills/gocontext-mcp:latest

# Run server
docker run -i ghcr.io/dshills/gocontext-mcp:latest

# With volume mount for persistent storage
docker run -i \
  -v /path/to/data:/data \
  -e GOCONTEXT_DB_PATH=/data/gocontext.db \
  ghcr.io/dshills/gocontext-mcp:latest
```

### Windows

#### Prerequisites

**For CGO Build**:
- Go 1.25.4 or later
- MinGW-w64 or MSYS2 with GCC

**For Pure Go Build** (recommended for Windows):
- Go 1.25.4 or later only

#### Installation Steps

1. **Download Binary**:
   - Visit [releases page](https://github.com/dshills/gocontext-mcp/releases)
   - Download `gocontext-windows-amd64.exe` (CGO) or `gocontext-windows-amd64-purego.exe` (Pure Go)

2. **Verify Checksum** (PowerShell):
   ```powershell
   # Download checksums
   Invoke-WebRequest -Uri "https://github.com/dshills/gocontext-mcp/releases/download/v1.0.0/checksums.txt" -OutFile checksums.txt

   # Verify
   Get-FileHash gocontext-windows-amd64.exe -Algorithm SHA256
   # Compare with value in checksums.txt
   ```

3. **Install**:
   ```powershell
   # Move to a directory in PATH
   Move-Item gocontext-windows-amd64.exe C:\Windows\System32\gocontext.exe

   # Or add to user PATH
   $env:Path += ";C:\tools"
   Move-Item gocontext-windows-amd64.exe C:\tools\gocontext.exe
   ```

4. **Verify Installation**:
   ```powershell
   gocontext --version
   ```

#### Windows Subsystem for Linux (WSL)

Follow the Linux installation instructions within WSL.

---

## Build from Source

### Prerequisites

- **Go 1.25.4 or later**: [Download](https://golang.org/dl/)
- **Git**: For cloning the repository
- **C Compiler** (for CGO builds only): GCC, Clang, or MSVC

### Clone Repository

```bash
git clone https://github.com/dshills/gocontext-mcp.git
cd gocontext-mcp
```

### Build Options

#### Option 1: CGO Build (Recommended)

Includes sqlite-vec extension for fast vector search:

```bash
make build
```

Or manually:

```bash
CGO_ENABLED=1 go build -tags "sqlite_vec" -ldflags "-X main.version=1.0.0" -o bin/gocontext ./cmd/gocontext
```

#### Option 2: Pure Go Build

No C compiler needed, fully portable:

```bash
make build-purego
```

Or manually:

```bash
CGO_ENABLED=0 go build -tags "purego" -ldflags "-X main.version=1.0.0" -o bin/gocontext-purego ./cmd/gocontext
```

### Install Locally

```bash
# Install to $GOPATH/bin
go install -tags "sqlite_vec" ./cmd/gocontext

# Or copy to system PATH
sudo cp bin/gocontext /usr/local/bin/
```

### Cross-Compilation

**Pure Go builds** can be cross-compiled easily:

```bash
# Linux from macOS
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -tags "purego" -o gocontext-linux ./cmd/gocontext

# Windows from macOS
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -tags "purego" -o gocontext.exe ./cmd/gocontext
```

**CGO builds** require platform-specific C toolchains (more complex, see `scripts/release.sh`).

---

## CGO vs Pure Go Builds

### CGO Build (with sqlite-vec)

**Pros**:
- ✓ 5-10x faster vector similarity search
- ✓ Lower memory usage for large codebases
- ✓ Native C performance for vector operations
- ✓ Recommended for production use

**Cons**:
- ✗ Requires C compiler at build time
- ✗ Platform-specific binaries
- ✗ Harder to cross-compile
- ✗ Larger binary size (~20MB)

**When to Use**:
- Production deployments
- Large codebases (>50k LOC)
- Performance-critical scenarios
- Local development with available C compiler

### Pure Go Build

**Pros**:
- ✓ No C compiler needed
- ✓ Easy cross-compilation
- ✓ Single static binary
- ✓ Portable across platforms
- ✓ Smaller binary size (~15MB)

**Cons**:
- ✗ Slower vector operations (pure Go)
- ✗ Higher memory usage
- ✗ No sqlite-vec extension

**When to Use**:
- Quick testing and prototyping
- Environments without C compiler
- Cross-platform distribution
- Small to medium codebases (<50k LOC)

### Performance Comparison

| Operation | CGO Build | Pure Go Build |
|-----------|-----------|---------------|
| Vector Search (1000 results) | 12ms | 45ms |
| Indexing (100k LOC) | 3.5 min | 5.2 min |
| Memory (100k LOC) | 200MB | 320MB |

---

## MCP Client Configuration

### Claude Code

Add to `~/.config/claude-code/mcp_settings.json`:

```json
{
  "mcpServers": {
    "gocontext": {
      "command": "/usr/local/bin/gocontext",
      "env": {
        "JINA_API_KEY": "your-jina-api-key-here",
        "GOCONTEXT_DB_PATH": "~/.cache/gocontext/db.sqlite"
      }
    }
  }
}
```

### Codex CLI

Add to Codex configuration:

```json
{
  "mcpServers": {
    "gocontext": {
      "command": "/usr/local/bin/gocontext"
    }
  }
}
```

### Custom MCP Client

GoContext communicates over stdio using the MCP protocol. Invoke as:

```bash
/usr/local/bin/gocontext
```

The server reads MCP requests from stdin and writes responses to stdout.

---

## Environment Variables

### Required

**JINA_API_KEY** (for Jina embeddings):
```bash
export JINA_API_KEY="jina_xxx..."
```
Get your API key at: https://jina.ai/embeddings/

### Optional

**OPENAI_API_KEY** (for OpenAI embeddings):
```bash
export OPENAI_API_KEY="sk-xxx..."
export GOCONTEXT_EMBEDDING_PROVIDER="openai"
```

**GOCONTEXT_EMBEDDING_PROVIDER**:
- `jina` (default): Jina AI embeddings
- `openai`: OpenAI embeddings
- `local`: Offline local embeddings (no API key)

**GOCONTEXT_DB_PATH**:
- Default: `~/.cache/gocontext/db.sqlite`
- Custom: `/path/to/custom/db.sqlite`

**GOCONTEXT_LOG_LEVEL**:
- `debug`, `info` (default), `warn`, `error`

### Setting Permanently

**macOS/Linux** (add to `~/.bashrc` or `~/.zshrc`):
```bash
export JINA_API_KEY="jina_xxx..."
export GOCONTEXT_DB_PATH="$HOME/.cache/gocontext/db.sqlite"
```

**Windows** (PowerShell):
```powershell
[Environment]::SetEnvironmentVariable("JINA_API_KEY", "jina_xxx...", "User")
```

---

## Verification

### Check Version

```bash
gocontext --version
```

Expected output:
```
GoContext MCP Server
Version: 1.0.0
Build Time: 2025-11-06T12:00:00Z
Build Mode: CGO with sqlite-vec
SQLite Driver: mattn/go-sqlite3
Vector Extension: true
```

### Test MCP Server

Start the server:
```bash
gocontext
```

The server should start and wait for MCP protocol input on stdin.

### Integration Test

Use the included test suite:
```bash
cd /path/to/gocontext-mcp
go test ./tests/e2e/...
```

---

## Troubleshooting

### macOS: "Cannot be opened because the developer cannot be verified"

**Solution**:
```bash
xattr -d com.apple.quarantine /usr/local/bin/gocontext
```

Or go to **System Preferences > Security & Privacy** and click "Open Anyway".

### Linux: "Permission denied" when running binary

**Solution**:
```bash
chmod +x /path/to/gocontext
```

### Windows: "This app can't run on your PC"

**Causes**:
- Downloaded wrong architecture (ARM instead of AMD64)
- Antivirus blocking execution

**Solution**:
- Verify you downloaded `gocontext-windows-amd64.exe`
- Add exception to Windows Defender or antivirus

### CGO Build Fails: "gcc: command not found"

**Solution (macOS)**:
```bash
xcode-select --install
```

**Solution (Linux - Debian/Ubuntu)**:
```bash
sudo apt-get install build-essential
```

**Solution (Linux - RHEL/CentOS)**:
```bash
sudo yum groupinstall "Development Tools"
```

**Alternative**: Use Pure Go build instead:
```bash
make build-purego
```

### MCP Server Not Starting

**Check**:
1. Is binary executable? `ls -l $(which gocontext)`
2. Is API key set? `echo $JINA_API_KEY`
3. Check logs: `gocontext 2>&1 | tee server.log`

### "Vector extension not available" Warning

**Cause**: Using Pure Go build without sqlite-vec.

**Solution**: Either:
1. Rebuild with CGO: `make build`
2. Continue with Pure Go (slower but functional)

### Performance Issues

**Check**:
1. Using CGO build? `gocontext --version` should show "sqlite-vec: true"
2. Sufficient memory? Minimum 1GB RAM for 100k LOC
3. Fast disk? SSD recommended for large codebases

**Optimize**:
- Use CGO build for better performance
- Increase worker pool: `GOCONTEXT_WORKERS=8`
- Use local embeddings: `GOCONTEXT_EMBEDDING_PROVIDER=local`

### Database Corruption

**Symptoms**: "database disk image is malformed"

**Solution**:
```bash
# Backup old database
mv ~/.cache/gocontext/db.sqlite ~/.cache/gocontext/db.sqlite.bak

# Server will create new database on next index
```

---

## Updating

### Update Binary

Download new version from releases and replace existing binary:

```bash
# Backup current version
sudo mv /usr/local/bin/gocontext /usr/local/bin/gocontext.bak

# Install new version
curl -LO https://github.com/dshills/gocontext-mcp/releases/download/vX.Y.Z/gocontext-<platform>
chmod +x gocontext-<platform>
sudo mv gocontext-<platform> /usr/local/bin/gocontext
```

### Update with Go Install

```bash
go install github.com/dshills/gocontext-mcp/cmd/gocontext@latest
```

### Database Migration

Check CHANGELOG for database schema changes. Generally:
- **Minor updates** (1.0.x → 1.1.x): No re-indexing needed
- **Major updates** (1.x → 2.x): May require re-indexing

To re-index after update:
```json
{
  "path": "/path/to/project",
  "force_reindex": true
}
```

---

## Support

- **Issues**: [GitHub Issues](https://github.com/dshills/gocontext-mcp/issues)
- **Discussions**: [GitHub Discussions](https://github.com/dshills/gocontext-mcp/discussions)
- **Documentation**: [docs/](https://github.com/dshills/gocontext-mcp/tree/main/docs)

---

## Next Steps

After installation:

1. **Configure MCP Client**: Add GoContext to your MCP client configuration
2. **Set API Key**: Configure Jina AI or OpenAI API key
3. **Index Your First Project**: Use `index_codebase` tool
4. **Try a Search**: Use `search_code` tool
5. **Read the Docs**: Explore [README.md](../README.md) and other documentation
