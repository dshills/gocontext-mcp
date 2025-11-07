# Security Policy

## Data Privacy and Security

GoContext is designed with privacy and security in mind. This document outlines our security practices, data handling policies, and vulnerability reporting procedures.

---

## Data Handling

### What Data is Collected?

GoContext operates **locally first** and minimizes data collection:

1. **Indexed Code**:
   - Source code files from your Go projects
   - Extracted symbols (functions, types, etc.)
   - Code chunks for embedding
   - All stored locally in SQLite database (`~/.gocontext/indices/`)

2. **Embeddings**:
   - When using Jina AI or OpenAI providers:
     - Code chunks are sent to external APIs for embedding generation
     - Only code content is sent (no file paths, no metadata)
     - Embeddings returned are stored locally
   - When using local provider:
     - No data sent externally
     - Embeddings generated on your machine

3. **No Telemetry**:
   - GoContext does **not** collect usage statistics
   - No analytics or tracking
   - No automatic error reporting

### What Data is Stored Locally?

**Location**: `~/.gocontext/indices/<project-hash>.db`

**Contents**:
- Project metadata (root path, module name, Go version)
- File paths and content hashes (SHA-256)
- Extracted symbols with documentation
- Code chunks
- Vector embeddings
- Full-text search indexes

**Permissions**: Files are owned by your user account with default filesystem permissions.

### What Data is Sent to External Services?

**Jina AI / OpenAI (Optional)**:
- **What**: Code chunk text only
- **When**: During indexing (when embeddings are generated)
- **Never sent**: File paths, project structure, metadata, your API key is transmitted securely via HTTPS

**Search queries**:
- Query text is embedded using the same provider
- Only the query text is sent (no project information)

**Offline mode**: Use `GOCONTEXT_EMBEDDING_PROVIDER=local` to avoid all external API calls.

---

## Privacy Best Practices

### For Sensitive Codebases

If your codebase contains sensitive information:

1. **Use offline mode**:
   ```bash
   export GOCONTEXT_EMBEDDING_PROVIDER=local
   gocontext index /path/to/sensitive/project
   ```

2. **Review code chunks** before indexing:
   - GoContext chunks code at function/type boundaries
   - Sensitive data in code (API keys, credentials) may be indexed
   - Consider excluding sensitive files via `.gitignore` patterns

3. **Encrypt database files** (optional):
   ```bash
   # Example: Use encrypted filesystem or volume
   # Store indices in encrypted location
   export GOCONTEXT_DB_PATH=/encrypted/volume/gocontext-indices
   ```

4. **API key security**:
   - Store API keys in environment variables (not in code)
   - Use MCP config file `env` section (not shared/committed files)
   - Rotate API keys periodically

### For Open Source Projects

For public repositories:
- Indexing with external APIs is safe (code is already public)
- Consider offline mode to avoid API costs
- No privacy concerns

---

## Supported Versions

We provide security updates for the following versions:

| Version | Supported          |
| ------- | ------------------ |
| 1.0.x   | :white_check_mark: |
| < 1.0   | :x:                |

---

## Vulnerability Reporting

### Reporting a Vulnerability

If you discover a security vulnerability in GoContext, please report it responsibly:

**DO NOT** open a public GitHub issue for security vulnerabilities.

**Instead, please email**: security@[YOUR-DOMAIN].com

**Include in your report**:
1. Description of the vulnerability
2. Steps to reproduce
3. Potential impact
4. Suggested fix (if any)
5. Your contact information (for follow-up)

### What to Expect

1. **Acknowledgment**: We will acknowledge your report within 48 hours
2. **Assessment**: We will assess the vulnerability and determine severity
3. **Fix**: We will work on a fix and coordinate disclosure timing with you
4. **Credit**: We will credit you in the security advisory (unless you prefer to remain anonymous)

### Disclosure Policy

- We follow **coordinated disclosure**
- We aim to fix critical vulnerabilities within 7 days
- We will publish a security advisory after the fix is released
- We will credit security researchers who report vulnerabilities responsibly

---

## Security Best Practices for Users

### API Key Security

**DO**:
- Store API keys in environment variables
- Use MCP configuration file `env` section
- Rotate API keys periodically
- Use separate API keys for different projects/environments

**DON'T**:
- Commit API keys to version control
- Share API keys in plaintext
- Use production API keys in development
- Log API keys in application logs

### Database Security

**Recommendations**:
1. **Backup regularly**: Copy `~/.gocontext/indices/` to secure backup location
2. **Protect filesystem**: Ensure proper file permissions (user-only access)
3. **Encrypt if needed**: Use encrypted volumes for sensitive projects
4. **Clean up**: Delete indexes for projects you no longer need

### Network Security

**HTTPS only**: All API calls to Jina AI / OpenAI use HTTPS (TLS encryption)

**No man-in-the-middle protection**: Standard TLS certificate validation is used

**Firewall considerations**: GoContext requires outbound HTTPS (port 443) for external embedding providers

---

## Security Features

### Input Validation

- Path validation: Prevent path traversal attacks
- Query sanitization: Prevent SQL injection in FTS queries
- Parameter validation: Enforce limits and types

### Database Security

- SQLite database with journal mode for ACID transactions
- Prepared statements prevent SQL injection
- No dynamic SQL query construction with user input

### Code Execution

- GoContext does **not** execute indexed code
- Only parses code using Go's standard AST library
- No `eval()` or dynamic code execution

---

## Known Limitations

### Not a Security Tool

GoContext is a **code search tool**, not a security scanner. It does not:
- Detect vulnerabilities in code
- Scan for secrets or credentials
- Perform static security analysis

For security scanning, use dedicated tools like:
- [gosec](https://github.com/securego/gosec) - Go security checker
- [govulncheck](https://pkg.go.dev/golang.org/x/vuln/cmd/govulncheck) - Go vulnerability scanner
- [truffleHog](https://github.com/trufflesecurity/trufflehog) - Secrets scanner

### Third-Party Dependencies

GoContext depends on:
- Go standard library (official, security-maintained)
- SQLite (widely-used, security-audited)
- sqlite-vec (community extension, review source if concerned)
- mcp-go (MCP protocol implementation)

We monitor dependencies for known vulnerabilities and update promptly.

---

## Compliance

### GDPR Considerations

GoContext operates locally and does not collect personal data. However:

- If you index code containing personal data (e.g., test fixtures with names/emails), that data is stored locally
- If using external embedding APIs, code chunks may be processed by Jina AI / OpenAI (review their privacy policies)
- You are responsible for ensuring your use complies with applicable data protection laws

### Enterprise Use

For enterprise deployments:

1. **Offline mode**: Recommended for compliance with data residency requirements
2. **Access controls**: Use filesystem permissions to restrict database access
3. **Audit logging**: Enable debug logging if audit trails are required:
   ```bash
   export GOCONTEXT_LOG_LEVEL=debug
   ```
4. **Network isolation**: Use firewall rules if external APIs must be blocked

---

## Incident Response

In the event of a security incident:

1. **Report immediately**: Email security@[YOUR-DOMAIN].com
2. **Isolate**: Stop using affected systems
3. **Preserve evidence**: Keep logs and database files for analysis
4. **Coordinate**: Work with maintainers on response

---

## Security Updates

Subscribe to security advisories:
- Watch the [GitHub repository](https://github.com/dshills/gocontext-mcp) for security releases
- Enable GitHub security alerts for dependencies
- Check [CHANGELOG.md](CHANGELOG.md) for security-related updates

---

## Questions?

For security-related questions that are not sensitive vulnerabilities:
- Open a [GitHub Discussion](https://github.com/dshills/gocontext-mcp/discussions)
- Tag with "security" label

For sensitive security issues, always email security@[YOUR-DOMAIN].com.

---

**Last Updated**: 2025-11-06
**Version**: 1.0.0
