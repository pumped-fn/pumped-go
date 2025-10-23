# Pumped Go

[![CI](https://github.com/pumped-fn/pumped-go/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/pumped-fn/pumped-go/actions/workflows/ci.yml)
[![GoDoc](https://pkg.go.dev/badge/github.com/pumped-fn/pumped-go.svg)](https://pkg.go.dev/github.com/pumped-fn/pumped-go)
[![codecov](https://codecov.io/gh/pumped-fn/pumped-go/graph/badge.svg?token=CODECOV_TOKEN)](https://codecov.io/gh/pumped-fn/pumped-go)

A powerful dependency injection and reactive execution library for Go, inspired by pumped-fn.

## Features

- **Graph-based DI**: Declare dependencies explicitly, resolve lazily
- **Reactive Updates**: Automatic propagation of changes through dependency graph
- **Flow Execution**: Short-span operations with execution context trees and tracing
- **Type-safe**: Full generic support with minimal casting
- **Controller Pattern**: Fine-grained lifecycle control (get, update, reload, release)
- **Tags**: Type-safe metadata system for executors, scopes, and flows
- **Extensions**: Powerful middleware system for cross-cutting concerns
- **No IDs Required**: Executors are their own keys

## Installation

```bash
go get github.com/pumped-fn/pumped-go
```

## Quick Example

```go
package main

import (
    "fmt"
    pumped "github.com/pumped-fn/pumped-go"
)

func main() {
    scope := pumped.NewScope()

    // Define executors
    counter := pumped.Provide(func(ctx *pumped.ResolveCtx) (int, error) {
        return 0, nil
    })

    doubled := pumped.Derive1(
        pumped.Reactive(counter),
        func(ctx *pumped.ResolveCtx, counterCtrl *pumped.Controller[int]) (int, error) {
            count, _ := counterCtrl.Get()
            return count * 2, nil
        },
    )

    // Use accessors
    doubledAcc := scope.Accessor(doubled)
    val, _ := doubledAcc.Get()
    fmt.Printf("Result: %d\n", val) // 0

    // Update triggers reactivity
    counterAcc := scope.Accessor(counter)
    counterAcc.Update(5)

    val, _ = doubledAcc.Get()
    fmt.Printf("Result: %d\n", val) // 10
}
```

## Concepts

### Executors

Executors are units of computation with explicit dependencies:

```go
// No dependencies
config := pumped.Provide(func(ctx *pumped.ResolveCtx) (string, error) {
    return "config-value", nil
})

// With dependencies - receive controllers
service := pumped.Derive1(
    pumped.Static(config),
    func(ctx *pumped.ResolveCtx, configCtrl *pumped.Controller[string]) (*Service, error) {
        cfg, _ := configCtrl.Get()
        return &Service{config: cfg}, nil
    },
)
```

### Dependency Modes

- **Static**: Resolve once, cache forever (default)
- **Reactive**: Invalidate when dependency changes

```go
// This executor re-resolves when counter changes
reactive := pumped.Derive1(
    pumped.Reactive(counter),
    func(ctx *pumped.ResolveCtx, ctrl *pumped.Controller[int]) (int, error) {
        // ...
    },
)

// This executor doesn't re-resolve
static := pumped.Derive1(
    pumped.Static(counter),
    func(ctx *pumped.ResolveCtx, ctrl *pumped.Controller[int]) (int, error) {
        // ...
    },
)
```

### Controllers

All dependencies are passed as controllers for maximum flexibility:

```go
button := pumped.Derive1(
    pumped.Static(counter),
    func(ctx *pumped.ResolveCtx, counterCtrl *pumped.Controller[int]) (*Button, error) {
        return &Button{
            onClick: func() error {
                current, _ := counterCtrl.Get()      // Get value
                return counterCtrl.Update(current + 1) // Update
            },
        }, nil
    },
)
```

### Tags

Type-safe metadata for executors and scopes:

```go
versionTag := pumped.NewTag[string]("version")

executor := pumped.Provide(
    func(ctx *pumped.ResolveCtx) (int, error) { return 42, nil },
    pumped.WithTag(versionTag, "1.0.0"),
)

version, _ := versionTag.Get(executor)
```

### Extensions

Powerful middleware for cross-cutting concerns:

```go
logging := pumped.NewLoggingExtension()
metrics := pumped.NewMetricsExtension()

scope := pumped.NewScope(
    pumped.WithExtension(logging),
    pumped.WithExtension(metrics),
)
```

### Flows

Flows are short-span executable units with context trees and tracing:

```go
// Define long-running resources as executors
db := pumped.Derive1(config, func(ctx *pumped.ResolveCtx, cfg *pumped.Controller[*Config]) (*DB, error) {
    return NewDB(cfg.Get().DBHost)
})

// Define short-span operations as flows
fetchUser := pumped.Flow1(db,
    func(execCtx *pumped.ExecutionCtx, db *pumped.Controller[*DB]) (*User, error) {
        database, _ := db.Get()
        return database.QueryUser("123")
    },
    pumped.WithFlowTag(pumped.FlowName(), "fetchUser"),
)

// Execute flow with context
result, execNode, err := pumped.Exec(scope, context.Background(), fetchUser)

// Query execution tree
tree := scope.GetExecutionTree()
roots := tree.GetRoots()
for _, root := range roots {
    children := tree.GetChildren(root.ID)
    // Visualize execution tree
}
```

**Sub-flow execution:**

```go
parentFlow := pumped.Flow1(db, func(execCtx *pumped.ExecutionCtx, db *pumped.Controller[*DB]) (string, error) {
    // Execute sub-flows
    user, userCtx, _ := pumped.Exec1(execCtx, fetchUserFlow)
    orders, _, _ := pumped.Exec1(userCtx, fetchOrdersFlow)

    // Child contexts can read parent data via tags
    userID, _ := execCtx.Lookup(customTag)

    return fmt.Sprintf("User %s has %d orders", user, len(orders)), nil
})
```

**Tag-based data flow:**

```go
// Set data in parent flow
execCtx.Set(pumped.Input(), "user-123")

// Child flows can read upward (but not write)
userID, _ := childCtx.GetFromParent(pumped.Input())
userID, _ := childCtx.Lookup(pumped.Input()) // checks self, then parents, then scope
```

**Execution lifecycle:**

- Flows execute with `ExecutionCtx` (execution-specific context tree)
- Executors resolve with `ResolveCtx` (scope-level resolution)
- Extensions hook into flow lifecycle: `OnFlowStart`, `OnFlowEnd`, `OnFlowPanic`
- Execution tree automatically tracks all executions with tags

## Development

This project uses [Devbox](https://www.jetify.com/devbox/) for reproducible development environments.

### Quick Start

1. **Install Devbox:**
   ```bash
   curl -fsSL https://get.jetify.com/devbox | bash
   ```

2. **Clone and setup:**
   ```bash
   git clone https://github.com/pumped-fn/pumped-go.git
   cd pumped-go
   devbox shell
   devbox run setup
   ```

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

## CI/CD Pipeline

### Overview

Our CI/CD pipeline uses:
- **GitHub Actions** for automation
- **Devbox** for reproducible development environments
- **GoReleaser** for release automation
- **Codecov** for coverage reporting
- **cosign** for artifact signing

### Workflows

#### CI Workflow

**Trigger:** Push to `main`/`develop` branches or pull requests

**Jobs:**
- **Lint**: Runs golangci-lint with comprehensive linter configuration
- **Test**: Matrix strategy (Ubuntu and macOS) with race detection and coverage
- **Build**: Validates library and example builds
- **Integration**: Runs integration tests with `-tags=integration`
- **Security**: Runs gosec security scanner

#### Release Workflow

**Trigger:** Push of version tag (`v*.*.*`)

**Features:**
- Runs full test suite
- Builds multi-platform binaries for examples
- Generates changelog from conventional commits
- Creates checksums
- Signs artifacts with cosign
- Creates GitHub release
- Updates Go proxy

### Linting Configuration

Enabled linters include:
- errcheck, gosimple, govet, ineffassign, staticcheck, unused
- gofmt, goimports, misspell, revive, gosec, gocritic
- And more...

Timeout: 5 minutes with error severity by default.

### Release Process

#### Semantic Versioning

We follow [Semantic Versioning](https://semver.org/):
- **MAJOR**: Breaking changes
- **MINOR**: New features (backward compatible)
- **PATCH**: Bug fixes

#### Conventional Commits

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

#### Creating a Release

1. **Ensure CI passes:** `devbox run ci`
2. **Test release locally:** `devbox run release-snapshot`
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

GitHub Actions will automatically handle the rest.

## Examples

See [examples/](./examples/) for complete working examples:

- `basic/` - Executor fundamentals with reactivity
- `health-monitor/` - Production-ready health monitoring service
- `order-processing/` - Flow execution with context trees
- `http-api/` - REST API with dependency injection
- `cli-tasks/` - CLI application with services

## Contributing

1. Fork the repository
2. Create a feature branch: `git checkout -b feature/amazing-feature`
3. Follow the development workflow above
4. Commit with conventional commits
5. Push to your fork: `git push origin feature/amazing-feature`
6. Open a Pull Request

## License

MIT
