# CI/CD Pipeline Documentation

This document describes the CI/CD pipeline setup for pumped-go.

## Overview

Our CI/CD pipeline uses:
- **GitHub Actions** for automation
- **Devbox** for reproducible development environments
- **GoReleaser** for release automation
- **Codecov** for coverage reporting
- **cosign** for artifact signing

## Workflows

### 1. CI Workflow (`.github/workflows/ci.yml`)

**Trigger:**  Push to `main`/`develop` branches or pull requests

**Jobs:**

#### Lint
- Runs golangci-lint with comprehensive linter configuration
- Uses devbox for consistent tooling
- Timeout: 5 minutes

#### Test
- Matrix strategy: Ubuntu and macOS
- Runs tests with race detection
- Generates coverage reports
- Uploads coverage to Codecov (Ubuntu only)

#### Build
- Builds main library
- Builds all example applications
- Validates no build errors

#### Integration
- Runs integration tests with `-tags=integration`

#### Security
- Runs gosec security scanner
- Checks for common security issues

**Environment:**
All jobs use devbox for consistent tooling across local and CI environments.

### 2. Release Workflow (`.github/workflows/release.yml`)

**Trigger:** Push of version tag (`v*.*.*`)

**Jobs:**

#### GoReleaser
- Runs full test suite
- Builds multi-platform binaries for examples
- Generates changelog from conventional commits
- Creates checksums
- Signs artifacts with cosign
- Creates GitHub release

#### Publish Go Module
- Triggers Go proxy to update module cache

**Required Secrets:**
- `GITHUB_TOKEN` (automatically provided)
- `CODECOV_TOKEN` (for coverage upload)

## Devbox Configuration

All development commands are defined in `devbox.json`:

### Environment Packages
- Go 1.23
- golangci-lint (latest)
- goreleaser (latest)
- cosign (latest)
- gh (GitHub CLI)
- gosec (latest)

### Available Commands

```bash
# Setup & Dependencies
devbox run setup           # Initial setup
devbox run deps            # Download dependencies
devbox run tidy            # Tidy go.mod

# Testing
devbox run test            # Run tests
devbox run test-coverage   # Run with coverage
devbox run coverage        # View coverage report
devbox run integration-test # Integration tests
devbox run benchmark       # Run benchmarks

# Code Quality
devbox run lint            # Run linters
devbox run lint-fix        # Auto-fix issues
devbox run fmt             # Format code
devbox run vet             # Run go vet

# Building
devbox run build           # Build library
devbox run build-examples  # Build examples

# Security
devbox run security        # Run gosec
devbox run vulnerability-check # Check vulnerabilities

# CI/CD
devbox run ci              # Full CI pipeline
devbox run pre-commit      # Pre-commit checks
devbox run release-snapshot # Test release locally
devbox run release-test    # Validate release config

# Maintenance
devbox run clean           # Clean artifacts
devbox run all             # Run everything
```

## Local Development

### Setup

1. **Install Devbox:**
   ```bash
   curl -fsSL https://get.jetify.com/devbox | bash
   ```

2. **Clone and enter devbox shell:**
   ```bash
   git clone https://github.com/pumped-fn/pumped-go.git
   cd pumped-go
   devbox shell
   ```

3. **Run setup:**
   ```bash
   devbox run setup
   ```

### Development Workflow

1. Make your changes
2. Run tests: `devbox run test`
3. Run linters: `devbox run lint`
4. Fix any issues: `devbox run lint-fix`
5. Run full CI: `devbox run ci`
6. Commit with conventional commits

### Pre-commit Checks

Before every commit:
```bash
devbox run pre-commit
```

This runs:
1. Code formatting
2. Linters
3. Tests

## Release Process

### Semantic Versioning

We follow [Semantic Versioning](https://semver.org/):
- **MAJOR**: Breaking changes
- **MINOR**: New features (backward compatible)
- **PATCH**: Bug fixes

### Conventional Commits

We use conventional commits for automatic changelog generation:

```
feat: Add new feature (MINOR bump)
fix: Fix bug (PATCH bump)
perf: Performance improvement
docs: Documentation changes
test: Test changes
ci: CI/CD changes
chore: Maintenance
refactor: Code refactoring

BREAKING CHANGE: (MAJOR bump)
```

### Creating a Release

1. **Ensure CI passes:**
   ```bash
   devbox run ci
   ```

2. **Test release locally:**
   ```bash
   devbox run release-snapshot
   ```

3. **Update version references if needed**

4. **Commit and push:**
   ```bash
   git add .
   git commit -m "chore: prepare for v0.x.0 release"
   git push
   ```

5. **Create and push tag:**
   ```bash
   git tag -a v0.x.0 -m "Release v0.x.0"
   git push origin v0.x.0
   ```

6. **GitHub Actions will automatically:**
   - Run CI checks
   - Build artifacts
   - Sign with cosign
   - Create GitHub release
   - Update Go proxy

## Linting Configuration

Located in `.golangci.yml`:

**Enabled Linters:**
- errcheck - Check unchecked errors
- gosimple - Simplify code
- govet - Advanced checks
- ineffassign - Unused assignments
- staticcheck - Advanced static analysis
- unused - Unused code
- gofmt - Formatting
- goimports - Import formatting
- misspell - Spell checking
- revive - Comprehensive linting
- gosec - Security
- gocritic - Opinionated checks
- And more...

**Timeout:** 5 minutes
**Severity:** Error by default

## GoReleaser Configuration

Located in `.goreleaser.yml`:

**Builds:**
- Example binaries (health-monitor, http-api, cli-tasks)
- Multi-platform: Linux, macOS, Windows
- Architectures: amd64, arm64

**Features:**
- Automatic changelog from conventional commits
- Checksum generation
- Artifact signing with cosign
- Archive creation (tar.gz, zip)
- GitHub release creation

## Dependency Management

### Dependabot

Located in `.github/dependabot.yml`:

- **Go modules:** Weekly updates (Monday)
- **GitHub Actions:** Weekly updates (Monday)
- Automatic PR creation with conventional commits

## Coverage Reporting

- Coverage generated during CI test job
- Uploaded to Codecov (Ubuntu builds only)
- Token stored in `CODECOV_TOKEN` secret
- Reports available at: https://codecov.io/gh/pumped-fn/pumped-go

## Troubleshooting

### CI Failures

1. **Lint failures:**
   ```bash
   devbox run lint-fix
   ```

2. **Test failures:**
   ```bash
   devbox run test -v
   ```

3. **Build failures:**
   ```bash
   devbox run build
   ```

### Local Issues

1. **Environment mismatch:**
   ```bash
   devbox shell --pure  # Clean environment
   devbox run setup     # Reinstall deps
   ```

2. **Stale cache:**
   ```bash
   devbox run clean
   ```

## Best Practices

1. **Always use devbox** for consistency
2. **Run `devbox run ci`** before pushing
3. **Follow conventional commits** for automatic changelogs
4. **Keep coverage above 80%** for new code
5. **Fix lint issues** before committing
6. **Test releases locally** with `devbox run release-snapshot`

## Maintenance

### Updating Tools

Tools are managed by Devbox. To update:

1. Edit `devbox.json` package versions
2. Run `devbox shell` to pull updates
3. Test with `devbox run ci`

### Updating Workflows

1. Edit workflow files in `.github/workflows/`
2. Test locally where possible
3. Create PR and verify CI passes
4. Merge after review

## Support

- **Issues:** [GitHub Issues](https://github.com/pumped-fn/pumped-go/issues)
- **Discussions:** [GitHub Discussions](https://github.com/pumped-fn/pumped-go/discussions)
- **Contributing:** See [CONTRIBUTING.md](../CONTRIBUTING.md)
