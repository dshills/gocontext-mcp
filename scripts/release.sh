#!/usr/bin/env bash
set -euo pipefail

# GoContext MCP Server Release Build Script
# Builds cross-platform binaries with version info and generates checksums

VERSION="${VERSION:-1.0.0}"
COMMIT=$(git rev-parse HEAD)
BUILD_TIME=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
PROJECT="github.com/dshills/gocontext-mcp"
DIST_DIR="dist"

echo "Building GoContext MCP Server v${VERSION}"
echo "Commit: ${COMMIT}"
echo "Build Time: ${BUILD_TIME}"
echo ""

# Clean and create dist directory
rm -rf "${DIST_DIR}"
mkdir -p "${DIST_DIR}"

# Build flags
LDFLAGS="-X main.version=${VERSION} -X main.buildTime=${BUILD_TIME} -s -w"

# Platform configurations
# Format: "GOOS GOARCH CGO_ENABLED TAGS OUTPUT_SUFFIX"
PLATFORMS=(
    "linux amd64 1 sqlite_vec linux-amd64"
    "linux arm64 1 sqlite_vec linux-arm64"
    "darwin amd64 1 sqlite_vec darwin-amd64"
    "darwin arm64 1 sqlite_vec darwin-arm64"
    "windows amd64 1 sqlite_vec windows-amd64.exe"
    "linux amd64 0 purego linux-amd64-purego"
    "darwin amd64 0 purego darwin-amd64-purego"
    "darwin arm64 0 purego darwin-arm64-purego"
    "windows amd64 0 purego windows-amd64-purego.exe"
)

build_platform() {
    local goos=$1
    local goarch=$2
    local cgo_enabled=$3
    local tags=$4
    local output_suffix=$5

    local output_name="gocontext-${output_suffix}"
    local build_mode=""

    if [ "${cgo_enabled}" = "1" ]; then
        build_mode="CGO"
    else
        build_mode="Pure Go"
    fi

    echo "Building ${goos}/${goarch} (${build_mode})..."

    GOOS="${goos}" GOARCH="${goarch}" CGO_ENABLED="${cgo_enabled}" \
        go build \
        -tags "${tags}" \
        -ldflags "${LDFLAGS}" \
        -o "${DIST_DIR}/${output_name}" \
        ./cmd/gocontext

    if [ $? -eq 0 ]; then
        echo "  ✓ ${output_name}"

        # Generate checksum
        if [ "${goos}" = "darwin" ]; then
            # macOS uses shasum
            (cd "${DIST_DIR}" && shasum -a 256 "${output_name}" >> checksums.txt)
        else
            # Linux/Windows use sha256sum if available, otherwise shasum
            if command -v sha256sum &> /dev/null; then
                (cd "${DIST_DIR}" && sha256sum "${output_name}" >> checksums.txt)
            else
                (cd "${DIST_DIR}" && shasum -a 256 "${output_name}" >> checksums.txt)
            fi
        fi
    else
        echo "  ✗ Failed to build ${output_name}"
        return 1
    fi
}

# Build for all platforms
for platform in "${PLATFORMS[@]}"; do
    IFS=' ' read -r goos goarch cgo_enabled tags output_suffix <<< "${platform}"

    # Skip CGO builds on non-native platforms without cross-compiler
    if [ "${cgo_enabled}" = "1" ] && [ "${goos}-${goarch}" != "$(go env GOOS)-$(go env GOARCH)" ]; then
        echo "Skipping ${goos}/${goarch} (CGO cross-compile requires cross-compiler)"
        continue
    fi

    build_platform "${goos}" "${goarch}" "${cgo_enabled}" "${tags}" "${output_suffix}"
    echo ""
done

# Create version info file
cat > "${DIST_DIR}/VERSION.txt" <<EOF
GoContext MCP Server
Version: ${VERSION}
Commit: ${COMMIT}
Build Time: ${BUILD_TIME}

Binary Variants:
- *-amd64, *-arm64: CGO build with sqlite-vec extension (recommended)
- *-purego: Pure Go build without CGO (portable, no C compiler needed)

For installation instructions, see:
https://github.com/dshills/gocontext-mcp#installation
EOF

# Create release notes template
cat > "${DIST_DIR}/RELEASE_NOTES.md" <<EOF
# GoContext MCP Server v${VERSION}

Release Date: $(date +"%Y-%m-%d")
Commit: ${COMMIT}

## Installation

Download the appropriate binary for your platform from the assets below.

### Recommended (CGO builds):
- \`gocontext-linux-amd64\` - Linux x86_64
- \`gocontext-linux-arm64\` - Linux ARM64
- \`gocontext-darwin-amd64\` - macOS Intel
- \`gocontext-darwin-arm64\` - macOS Apple Silicon
- \`gocontext-windows-amd64.exe\` - Windows x86_64

### Pure Go builds (no CGO):
- \`gocontext-linux-amd64-purego\` - Linux x86_64 (portable)
- \`gocontext-darwin-amd64-purego\` - macOS Intel (portable)
- \`gocontext-darwin-arm64-purego\` - macOS Apple Silicon (portable)
- \`gocontext-windows-amd64-purego.exe\` - Windows x86_64 (portable)

### Verify Download

\`\`\`bash
# Download binary and checksums
curl -LO https://github.com/dshills/gocontext-mcp/releases/download/v${VERSION}/gocontext-<platform>
curl -LO https://github.com/dshills/gocontext-mcp/releases/download/v${VERSION}/checksums.txt

# Verify (Linux/macOS)
sha256sum -c --ignore-missing checksums.txt

# Or on macOS
shasum -a 256 -c checksums.txt
\`\`\`

### Make Executable

\`\`\`bash
chmod +x gocontext-<platform>
sudo mv gocontext-<platform> /usr/local/bin/gocontext
\`\`\`

## What's Changed

See [CHANGELOG.md](https://github.com/dshills/gocontext-mcp/blob/main/CHANGELOG.md) for full details.

## Full Changelog

**Full Changelog**: https://github.com/dshills/gocontext-mcp/commits/v${VERSION}
EOF

echo "=========================================="
echo "Release build complete!"
echo ""
echo "Artifacts in ${DIST_DIR}/"
ls -lh "${DIST_DIR}/"
echo ""
echo "Checksums:"
cat "${DIST_DIR}/checksums.txt"
echo ""
echo "Next steps:"
echo "1. Review ${DIST_DIR}/RELEASE_NOTES.md"
echo "2. Create GitHub release with tag v${VERSION}"
echo "3. Upload binaries from ${DIST_DIR}/"
echo "4. Update CHANGELOG.md with release notes"
