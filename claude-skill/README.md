# Pumped-Go Claude Code Skill

Auto-activating Claude Code skill for building production-ready Go applications with [pumped-go](https://github.com/pumped-fn/pumped-go).

## Overview

This skill provides comprehensive guidance for using pumped-go, covering:

- **Architecture Patterns** - When to use Executors vs Flows vs plain functions
- **Package-Level Executor Pattern** - Go-idiomatic dependency declaration
- **Lifecycle Management** - Proper cleanup, graceful shutdown, resource management
- **Testing Strategies** - Unit and integration testing with mocks
- **Production Readiness** - Error handling, goroutine safety, signal handling
- **Common Patterns** - Repository, service layer, handler injection, background workers

## Auto-Activation

The skill automatically activates when your `go.mod` contains:

```go
require github.com/pumped-fn/pumped-go v0.x.x
```

No manual activation needed! Claude will automatically apply pumped-go best practices when working with your code.

## What This Skill Covers

### 1. Decision Tree
Quick guidance on choosing the right pattern:
- Long-lived resources → **Executors** (package-level vars)
- Short-span operations → **Flows** (business logic with tracing)
- Pure transformations → **Plain functions**

### 2. Executors (Long-Lived Resources)
- Package-level `var` pattern
- `Provide` (no deps) vs `Derive1-N` (with deps)
- Controller-based dependency access
- Error handling at every step
- Lifecycle management with `OnCleanup()`
- Static vs Reactive dependencies

### 3. Flows (Short-Span Operations)
- When to use flows vs methods
- Execution contexts and tag propagation
- Sub-flow composition
- Error handling patterns

### 4. Production Lifecycle
- Scope creation and disposal
- Graceful shutdown patterns
- Signal handling
- Resource cleanup ordering
- Background goroutine management

### 5. Testing
- `WithPreset()` for mocking executors
- Table-driven tests (Go idiom)
- Testing reactivity
- Integration vs unit tests

### 6. Enforcement Rules
- **Tier 1 (Critical):** Must follow for production
- **Tier 2 (Important):** Strong recommendations
- **Tier 3 (Best Practices):** Go idioms and conventions

## Quick Examples

### Package-Level Executor Declaration

```go
package graph

import pumped "github.com/pumped-fn/pumped-go"

var (
    Config = pumped.Provide(func(ctx *pumped.ResolveCtx) (*Config, error) {
        return &Config{DBHost: "localhost"}, nil
    })

    DB = pumped.Derive1(
        Config,
        func(ctx *pumped.ResolveCtx, cfgCtrl *pumped.Controller[*Config]) (*sql.DB, error) {
            cfg, err := cfgCtrl.Get()
            if err != nil {
                return nil, err
            }

            db, err := sql.Open("postgres", cfg.ConnectionString())
            if err != nil {
                return nil, err
            }

            ctx.OnCleanup(func() error {
                return db.Close()
            })

            return db, nil
        },
    )
)
```

### Testing with Mocks

```go
func TestUserService(t *testing.T) {
    mockRepo := &MockUserRepository{}

    testScope := pumped.NewScope(
        pumped.WithPreset(UserRepo, mockRepo),
    )
    defer testScope.Dispose()

    service, err := pumped.Resolve(testScope, UserService)
    if err != nil {
        t.Fatalf("failed to resolve: %v", err)
    }

    // Test service...
}
```

### Graceful Shutdown

```go
func main() {
    scope := pumped.NewScope()
    defer scope.Dispose()

    // Resolve components...

    sigCh := make(chan os.Signal, 1)
    signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
    <-sigCh

    log.Println("Shutting down...")
    // scope.Dispose() runs via defer
}
```

## Installation

Install the skill via Claude Code marketplace:

```bash
# Coming soon to Claude Code marketplace
```

Or clone locally for development:

```bash
git clone https://github.com/pumped-fn/pumped-go.git
cd pumped-go/claude-skill
```

## Examples

The skill references real production examples from the `examples/` directory:

- `examples/basic/` - Executor fundamentals
- `examples/health-monitor/` - Production health monitoring service
- `examples/http-api/` - REST API with dependency injection
- `examples/cli-tasks/` - CLI application
- `examples/order-processing/` - Flow execution patterns

## Key Principles

1. **Package-level executors** - All executors are `var` declarations
2. **Controller pattern** - All dependencies passed as `*Controller[T]`
3. **Error handling** - Never ignore errors from `.Get()`
4. **Lifecycle management** - Always `OnCleanup()` for resources
5. **Graceful shutdown** - Always `scope.Dispose()` (defer pattern)
6. **Testing** - Use `WithPreset()` for mocks

## Contributing

Found an issue or want to improve the skill? Contributions welcome!

1. Fork the repository
2. Create your feature branch
3. Update the skill in `claude-skill/skills/pumped-go/SKILL.md`
4. Submit a pull request

## License

MIT - Same as pumped-go

## Links

- [pumped-go Repository](https://github.com/pumped-fn/pumped-go)
- [Documentation](https://pkg.go.dev/github.com/pumped-fn/pumped-go)
- [Examples](../examples/)
