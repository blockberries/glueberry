# Contributing to Glueberry

Thank you for your interest in contributing to Glueberry! This document provides guidelines and instructions for contributing.

## Table of Contents

- [Code of Conduct](#code-of-conduct)
- [Getting Started](#getting-started)
- [Development Setup](#development-setup)
- [Making Changes](#making-changes)
- [Testing](#testing)
- [Code Style](#code-style)
- [Pull Request Process](#pull-request-process)
- [Issue Guidelines](#issue-guidelines)
- [Security](#security)
- [Release Process](#release-process)

## Code of Conduct

Please be respectful and constructive in all interactions. We welcome contributors of all backgrounds and experience levels.

## Getting Started

### Prerequisites

- Go 1.21 or later
- Make
- Git

### Fork and Clone

1. Fork the repository on GitHub
2. Clone your fork:
   ```bash
   git clone https://github.com/YOUR-USERNAME/glueberry.git
   cd glueberry
   ```
3. Add upstream remote:
   ```bash
   git remote add upstream https://github.com/blockberries/glueberry.git
   ```

## Development Setup

### Initial Setup

```bash
# Install Go dependencies
go mod download

# Verify setup
make check
```

### Project Structure

```
glueberry/
├── node.go                    # Main Node implementation
├── config.go                  # Configuration types and options
├── errors.go                  # Error types and helpers
├── events.go                  # Connection events
├── pkg/
│   ├── addressbook/           # Peer address book with persistence
│   ├── connection/            # Connection lifecycle management
│   ├── crypto/                # Cryptographic operations
│   ├── protocol/              # libp2p protocol handling
│   └── streams/               # Encrypted/unencrypted streams
├── internal/
│   ├── eventdispatch/         # Event dispatcher
│   ├── flow/                  # Flow control
│   └── pool/                  # Buffer pooling
├── testing/                   # Mock implementations
├── fuzz/                      # Fuzz tests
├── benchmark/                 # Performance benchmarks
└── examples/                  # Example applications
```

### Available Make Targets

```bash
make help          # Show all available targets
make build         # Build all packages
make test          # Run tests with race detection
make test-short    # Run tests without race detection (faster)
make lint          # Run golangci-lint
make check         # Run all checks (format, vet, lint, test)
make bench         # Run benchmarks
make fuzz          # Run fuzz tests
make coverage      # Generate coverage report
make clean         # Clean build artifacts
```

## Making Changes

### Branch Naming

Use descriptive branch names:
- `feature/add-session-resumption`
- `fix/reconnection-timeout`
- `docs/update-api-reference`
- `refactor/simplify-handshake`

### Commit Messages

Follow conventional commit format:

```
type(scope): short description

Longer description if needed. Explain the motivation
for the change and contrast with previous behavior.

Fixes #123
```

**Types:**
- `feat`: New feature
- `fix`: Bug fix
- `docs`: Documentation only
- `style`: Code style (formatting, no logic change)
- `refactor`: Code refactoring
- `perf`: Performance improvement
- `test`: Adding or updating tests
- `chore`: Maintenance tasks

**Examples:**
```
feat(connection): add session resumption support

Add session ticket mechanism for faster reconnects.
Tickets are encrypted with a per-node secret and expire after 24 hours.

Fixes #45
```

```
fix(crypto): zero key material on disconnect

Shared keys were remaining in memory after peer disconnect.
Now explicitly zeroed using SecureZero() to prevent memory leaks.

Fixes #78
```

## Testing

### Running Tests

```bash
# Run all tests with race detection
make test

# Run specific package tests
go test -v ./pkg/crypto/...

# Run specific test
go test -v ./pkg/crypto -run TestCipher

# Run with coverage
make coverage
```

### Writing Tests

1. **Test file naming**: `*_test.go` in the same package
2. **Test function naming**: `TestFunctionName`, `TestType_Method`
3. **Table-driven tests** for multiple cases
4. **Benchmark naming**: `BenchmarkFunctionName`

```go
func TestNode_Connect(t *testing.T) {
    tests := []struct {
        name    string
        peerID  peer.ID
        wantErr error
    }{
        {
            name:    "valid peer",
            peerID:  validPeerID,
            wantErr: nil,
        },
        {
            name:    "blacklisted peer",
            peerID:  blacklistedPeerID,
            wantErr: ErrPeerBlacklisted,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            node := setupTestNode(t)
            err := node.Connect(tt.peerID)
            if !errors.Is(err, tt.wantErr) {
                t.Errorf("Connect() error = %v, wantErr %v", err, tt.wantErr)
            }
        })
    }
}
```

### Test Coverage

Aim for high coverage on:
- Public APIs (Node methods)
- Edge cases (nil, empty, max values)
- Error paths
- Concurrent operations
- Cryptographic operations

### Fuzz Testing

Run fuzz tests for security-critical code:

```bash
# Run all fuzz tests
make fuzz

# Run specific fuzz target
go test -fuzz=FuzzDecrypt -fuzztime=1m ./fuzz/
```

### Stress Testing

Run concurrent stress tests:

```bash
go test -race -run="Test.*Concurrent" ./pkg/connection/...
```

## Code Style

### Go Code Style

- Follow [Effective Go](https://golang.org/doc/effective_go)
- Use `gofmt` (enforced by CI)
- Use `golangci-lint` (run `make lint`)

**Key points:**
- Export only necessary symbols
- Document all exported types/functions
- Handle all errors
- Avoid global state
- Use context for cancellation
- Never log sensitive data (keys, secrets)

### Naming Conventions

```go
// Public types: PascalCase
type ConnectionEvent struct {}

// Public functions: PascalCase
func New(cfg *Config) (*Node, error)

// Private types/functions: camelCase
type peerConnection struct {}
func handleHandshake(stream network.Stream) error

// Constants: PascalCase for public, camelCase for private
const DefaultHandshakeTimeout = 30 * time.Second
const maxReconnectAttempts = 10
```

### Error Handling

```go
// Use sentinel errors for known conditions
var ErrPeerBlacklisted = errors.New("peer is blacklisted")

// Wrap errors with context
return fmt.Errorf("connect to %s: %w", peerID, err)

// Use typed errors for rich context
return &Error{
    Code:    ErrCodeConnectionFailed,
    PeerID:  peerID,
    Message: "connection refused",
    Cause:   err,
}
```

### Documentation

```go
// Package glueberry provides secure P2P communications over libp2p.
package glueberry

// Node represents a P2P communication node with encrypted streams.
// All methods are safe for concurrent use.
//
// A typical usage pattern:
//
//     node, err := glueberry.New(cfg)
//     node.Start()
//     defer node.Stop()
//
//     // Handle events
//     for event := range node.Events() {
//         // ...
//     }
type Node struct {
    // ...
}
```

## Pull Request Process

### Before Submitting

1. **Sync with upstream**:
   ```bash
   git fetch upstream
   git rebase upstream/main
   ```

2. **Run all checks**:
   ```bash
   make check
   ```

3. **Update documentation** if needed

4. **Add tests** for new functionality

5. **Update CHANGELOG.md** for user-facing changes

### PR Description Template

```markdown
## Summary
Brief description of changes.

## Motivation
Why is this change needed?

## Changes
- Change 1
- Change 2

## Testing
How was this tested?

## Breaking Changes
List any breaking changes (if applicable).

## Related Issues
Fixes #123
Related to #456
```

### Review Process

1. PRs require at least one approval
2. CI must pass (tests, lint, build)
3. Address all review comments
4. Squash commits if requested
5. Maintainer will merge when ready

### After Merge

- Delete your feature branch
- Update your local main:
  ```bash
  git checkout main
  git pull upstream main
  ```

## Issue Guidelines

### Bug Reports

Include:
- Glueberry version
- Go version (`go version`)
- Operating system
- Minimal reproduction case
- Expected vs actual behavior
- Stack trace (if applicable)

### Feature Requests

Include:
- Use case description
- Proposed API (if applicable)
- Alternative approaches considered
- Impact on existing functionality

### Labels

- `bug`: Something isn't working
- `enhancement`: New feature or request
- `documentation`: Documentation improvements
- `good first issue`: Good for newcomers
- `help wanted`: Extra attention needed
- `security`: Security-related issues
- `breaking`: Breaking change

## Security

### Reporting Security Issues

**Do NOT open public issues for security vulnerabilities.**

Please report security issues privately to: security@blockberries.com

Include:
- Description of the vulnerability
- Steps to reproduce
- Potential impact
- Suggested fix (if any)

### Security Considerations for Contributors

When contributing:
- Never log private keys, shared secrets, or decrypted data
- Always validate input from the network
- Use constant-time comparisons for cryptographic operations
- Zero sensitive memory after use
- Review the [SECURITY_REVIEW.md](../SECURITY_REVIEW.md) for guidelines

## Release Process

### Version Numbering

We use [Semantic Versioning](https://semver.org/):
- **MAJOR**: Breaking changes
- **MINOR**: New features (backward compatible)
- **PATCH**: Bug fixes (backward compatible)

### Release Checklist

1. Update CHANGELOG.md
2. Update version constants
3. Run full test suite including fuzz tests
4. Create release tag
5. Create GitHub release
6. Update documentation

### Changelog Format

```markdown
## [1.2.0] - 2026-02-15

### Added
- Session resumption for faster reconnects (#45)

### Changed
- Improved handshake performance by 30%

### Fixed
- Memory leak in shared key handling (#78)

### Security
- Key material now zeroed on disconnect
```

## Getting Help

- **Questions**: Open a GitHub Discussion
- **Bugs**: Open a GitHub Issue
- **Security**: Email security@blockberries.com

## Recognition

Contributors are recognized in:
- CHANGELOG.md (for the release)
- GitHub contributors list

Thank you for contributing to Glueberry!
