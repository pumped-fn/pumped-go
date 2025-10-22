# Contributing to Pumped Go

Thank you for your interest in contributing to Pumped Go! This document provides guidelines and information about our development process.

## Table of Contents

- [Development Setup](#development-setup)
- [Development Workflow](#development-workflow)
- [Testing](#testing)
- [Code Quality](#code-quality)
- [CI/CD Pipeline](#cicd-pipeline)
- [Release Process](#release-process)
- [Submitting Changes](#submitting-changes)

## Development Setup

We recommend using [Devbox](https://www.jetify.com/devbox) for environment management. It ensures all developers have the exact same tools and versions.

### Option 1: Using Devbox (Recommended)

Devbox provides an isolated, reproducible development environment.

1. **Install Devbox**
   ```bash
   curl -fsSL https://get.jetify.com/devbox | bash
   ```

2. **Clone the repository**
   ```bash
   git clone https://github.com/pumped-fn/pumped-go.git
   cd pumped-go
   ```

3. **Start the Devbox shell**
   ```bash
   devbox shell
   ```

   This automatically installs and configures:
   - Go 1.23
   - golangci-lint
   - goreleaser
   - Task runner
   - cosign (for signing releases)
   - gh (GitHub CLI)

4. **Run setup**
   ```bash
   devbox run setup
   ```

5. **Verify setup**
   ```bash
   task test
   ```

### Option 2: Manual Setup

If you prefer not to use Devbox:

#### Prerequisites

- Go 1.23 or higher
- [Task](https://taskfile.dev/) - Task runner
- Git
- golangci-lint
- goreleaser (for releases)

#### Installing Tools

**Task Runner:**
```bash
# macOS
brew install go-task

# Linux
sh -c "$(curl --location https://taskfile.dev/install.sh)" -- -d -b ~/.local/bin

# Windows
choco install go-task

# Or install via go
go install github.com/go-task/task/v3/cmd/task@latest
```

**Development Tools:**
```bash
task install-tools
```

#### Getting Started

1. **Clone the repository**
   ```bash
   git clone https://github.com/pumped-fn/pumped-go.git
   cd pumped-go
   ```

2. **Download dependencies**
   ```bash
   task deps
   ```

3. **Run tests to verify setup**
   ```bash
   task test
   ```

## Development Workflow

### Using Devbox Scripts

All development tasks are defined as devbox scripts. Here are the most common commands:

```bash
# Setup environment
devbox run setup

# Download dependencies
devbox run deps

# Run tests
devbox run test

# Run tests with coverage
devbox run test-coverage

# View coverage report
devbox run coverage

# Run linters
devbox run lint

# Fix linting issues automatically
devbox run lint-fix

# Format code
devbox run fmt

# Build library
devbox run build

# Build examples
devbox run build-examples

# Run full CI pipeline locally
devbox run ci

# Run security checks
devbox run security

# Check for vulnerabilities
devbox run vulnerability-check

# Run pre-commit checks
devbox run pre-commit

# Clean build artifacts
devbox run clean

# Run benchmarks
devbox run benchmark

# Run integration tests
devbox run integration-test

# Test release process locally
devbox run release-snapshot
```

All scripts are defined in `devbox.json` ensuring consistency between local development and CI/CD.

### Code Organization

```
pumped-go/
├── *.go              # Core library files
├── examples/         # Example applications
│   ├── basic/
│   ├── health-monitor/
│   ├── http-api/
│   └── cli-tasks/
├── codegen/          # Code generation utilities
├── extensions/       # Extension implementations
└── *_test.go        # Test files
```

## Testing

### Running Tests

```bash
# Run all tests
devbox run test

# Run tests with coverage
devbox run test-coverage

# View coverage report
devbox run coverage

# Run integration tests
devbox run integration-test

# Run benchmarks
devbox run benchmark
```

### Writing Tests

- Place test files alongside the code they test
- Use table-driven tests where appropriate
- Aim for >80% code coverage for new features
- Include both unit tests and integration tests

Example:
```go
func TestExecutor(t *testing.T) {
    tests := []struct {
        name    string
        input   int
        want    int
        wantErr bool
    }{
        {"positive", 5, 10, false},
        {"zero", 0, 0, false},
        {"negative", -5, -10, false},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // test implementation
        })
    }
}
```

## Code Quality

### Linting

We use `golangci-lint` with a comprehensive set of linters configured in `.golangci.yml`.

```bash
# Run linters
devbox run lint

# Auto-fix issues where possible
devbox run lint-fix
```

### Code Formatting

```bash
# Format all Go files
devbox run fmt
```

### Security Scanning

```bash
# Run security checks
devbox run security

# Check for vulnerabilities
devbox run vulnerability-check
```

### Pre-commit Checks

Before committing, run:
```bash
devbox run pre-commit
```

This will:
1. Format code
2. Run linters
3. Run tests

## CI/CD Pipeline

Our CI/CD pipeline is implemented using GitHub Actions with multiple workflows.

### CI Workflow (`.github/workflows/ci.yml`)

Triggered on: Push to `main`/`develop` branches and all pull requests

**Jobs:**
1. **Lint** - Code quality checks using golangci-lint via devbox
2. **Test** - Tests across multiple OS (Linux, macOS) using devbox
3. **Build** - Verify library and examples build successfully via devbox
4. **Integration** - Run integration tests via devbox
5. **Security** - Security scanning with gosec via devbox

**Environment Management:**
- All CI jobs use Devbox for consistent tooling across local and CI environments
- Devbox caching is enabled for faster CI runs

**Coverage Reporting:**
- Coverage is uploaded to Codecov on successful Ubuntu test runs
- Coverage token is stored in repository secrets as `CODECOV_TOKEN`

### Release Workflow (`.github/workflows/release.yml`)

Triggered on: Tag push matching `v*.*.*` (e.g., `v0.1.0`)

**Jobs:**
1. **goreleaser** - Creates GitHub release with artifacts
   - Builds example binaries for multiple platforms
   - Generates checksums
   - Creates changelog from commits
   - Signs artifacts with cosign

2. **publish-go-module** - Triggers Go proxy update

### Local CI Testing

Run the full CI pipeline locally:
```bash
devbox run ci
```

Test the release process locally:
```bash
# Create a snapshot release (doesn't publish)
devbox run release-snapshot

# Test release configuration
devbox run release-test
```

**Note:** Using devbox ensures your local environment matches CI exactly.

## Release Process

We follow [Semantic Versioning](https://semver.org/) (MAJOR.MINOR.PATCH).

### Creating a Release

1. **Ensure all tests pass**
   ```bash
   devbox run ci
   ```

2. **Update version references** (if needed)
   - Update README examples if API changed
   - Update CHANGELOG.md

3. **Commit changes**
   ```bash
   git add .
   git commit -m "chore: prepare for v0.x.0 release"
   git push
   ```

4. **Create and push tag**
   ```bash
   git tag -a v0.x.0 -m "Release v0.x.0"
   git push origin v0.x.0
   ```

5. **GitHub Actions will automatically:**
   - Run all CI checks
   - Build release artifacts
   - Create GitHub release with changelog
   - Sign artifacts
   - Publish to Go module proxy

### Commit Message Convention

We use conventional commits for automatic changelog generation:

- `feat:` - New features (triggers MINOR version bump)
- `fix:` - Bug fixes (triggers PATCH version bump)
- `perf:` - Performance improvements
- `docs:` - Documentation changes
- `test:` - Test changes
- `ci:` - CI/CD changes
- `chore:` - Maintenance tasks
- `refactor:` - Code refactoring
- `BREAKING CHANGE:` - Breaking changes (triggers MAJOR version bump)

Examples:
```bash
git commit -m "feat: add new Flow execution tracing"
git commit -m "fix: correct race condition in Scope cache"
git commit -m "perf: optimize dependency resolution"
```

## Submitting Changes

### Pull Request Process

1. **Fork the repository** and create a feature branch
   ```bash
   git checkout -b feature/my-new-feature
   ```

2. **Make your changes** following our coding standards

3. **Add tests** for new functionality

4. **Run pre-commit checks**
   ```bash
   devbox run pre-commit
   ```

5. **Commit with conventional commit messages**
   ```bash
   git commit -m "feat: add awesome feature"
   ```

6. **Push to your fork**
   ```bash
   git push origin feature/my-new-feature
   ```

7. **Create a Pull Request** with:
   - Clear description of changes
   - Reference to related issues
   - Test results
   - Breaking changes (if any)

### PR Requirements

Before your PR can be merged:
- ✅ All CI checks must pass
- ✅ Code coverage should not decrease
- ✅ At least one maintainer approval
- ✅ All conversations resolved
- ✅ Commits follow conventional commit format

### Review Process

1. Automated CI checks run on PR creation
2. Maintainers review code quality and design
3. Feedback is addressed through additional commits
4. Once approved, maintainer merges the PR

## Getting Help

- **Issues**: [GitHub Issues](https://github.com/pumped-fn/pumped-go/issues)
- **Discussions**: [GitHub Discussions](https://github.com/pumped-fn/pumped-go/discussions)
- **Questions**: Feel free to open a discussion or issue

## Code of Conduct

Please note that this project follows a Code of Conduct. By participating, you are expected to uphold this code.

## License

By contributing to Pumped Go, you agree that your contributions will be licensed under the MIT License.
