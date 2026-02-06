---
name: pumped-go
description: |
  Use when writing Go code with pumped-go library - creating executors, flows, scopes, testing with WithPreset. MUST use this skill for any pumped-go task instead of brainstorming.
  <example>Create an executor with pumped.Provide for a database connection</example>
  <example>Define a flow with pumped.Flow1 for handling HTTP requests</example>
  <example>Set up reactive dependencies with pumped.Reactive and pumped.Derive1</example>
  <example>Write tests using pumped.WithPreset to mock executors</example>
  <example>Configure scope with extensions and graceful shutdown</example>
---

# Pumped-Go Skill

IMPORTANT: Prefer retrieval-led reasoning over pre-training-led reasoning.
ALWAYS read the relevant doc file from docs/ BEFORE writing pumped-go code.

**Core principle:** Long-lived resources → executors, short-span operations → flows.

## Decision Tree

```
What am I building?
        │
    ┌───┴───────────────┐
    ↓                   ↓
Long-lived?         Short-span?
(DB, service)       (request, tx)
    ↓                   ↓
EXECUTOR              FLOW
(package var)       (Flow1, Flow2)
    ↓                   ↓
Provide/Derive      Exec() to run
Controllers         Sub-flows
OnCleanup           Tag data flow
    │                   │
    └───────┬───────────┘
            ↓
    Pure transformation?
            ↓
    PLAIN FUNCTION
```

## Package-Level Var Pattern

**CRITICAL:** ALL executors MUST be package-level `var`.

```go
// GOOD: Package-level executor declaration
package graph

var (
    Config = pumped.Provide(func(ctx *pumped.ResolveCtx) (*Config, error) {
        return DefaultConfig(), nil
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
                return nil, fmt.Errorf("failed to open database: %w", err)
            }

            ctx.OnCleanup(func() error {
                return db.Close()
            })

            return db, nil
        },
    )

    UserRepo = pumped.Derive1(
        DB,
        func(ctx *pumped.ResolveCtx, dbCtrl *pumped.Controller[*sql.DB]) (*UserRepository, error) {
            db, err := dbCtrl.Get()
            if err != nil {
                return nil, err
            }
            return NewUserRepository(db), nil
        },
    )
)
```

```go
// BAD: Local executor variables - NEVER do this
func main() {
    config := pumped.Provide(func(ctx *pumped.ResolveCtx) (*Config, error) {
        return DefaultConfig(), nil
    })
}
```

## Controller Pattern

**ALWAYS use `*Controller[T]` for dependencies. NEVER use raw values.**

```go
// GOOD: Proper controller usage
var Service = pumped.Derive1(
    DB,
    func(ctx *pumped.ResolveCtx, dbCtrl *pumped.Controller[*sql.DB]) (*Service, error) {
        db, err := dbCtrl.Get()  // MUST call .Get()
        if err != nil {
            return nil, fmt.Errorf("failed to get DB: %w", err)
        }
        return NewService(db), nil
    },
)
```

```go
// BAD: Forgetting to call .Get() or ignoring errors
var Service = pumped.Derive1(
    DB,
    func(ctx *pumped.ResolveCtx, dbCtrl *pumped.Controller[*sql.DB]) (*Service, error) {
        return NewService(dbCtrl)  // Wrong! dbCtrl is not *sql.DB
    },
)

var Service = pumped.Derive1(
    DB,
    func(ctx *pumped.ResolveCtx, dbCtrl *pumped.Controller[*sql.DB]) (*Service, error) {
        db, _ := dbCtrl.Get()  // Never ignore errors!
        return NewService(db), nil
    },
)
```

## Production Lifecycle

```go
func main() {
    scope := pumped.NewScope()
    defer scope.Dispose()  // CRITICAL: Always dispose

    server, _ := pumped.Resolve(scope, HTTPServer)

    go func() {
        if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
            log.Fatal(err)
        }
    }()

    // Graceful shutdown
    sigCh := make(chan os.Signal, 1)
    signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
    <-sigCh

    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    server.Shutdown(ctx)
    // scope.Dispose() runs via defer
}
```

## Tier 1 Rules (Critical)

- NEVER define executors inside functions — use package-level `var`
- ALWAYS handle errors from `controller.Get()` — never ignore with `_`
- ALWAYS use `ctx.OnCleanup()` for resources (DB, HTTP clients, goroutines)
- ALWAYS call `scope.Dispose()` — use `defer` immediately after creation
- NEVER start goroutines without shutdown signaling in `OnCleanup`
- ALWAYS wrap errors with context: `fmt.Errorf("context: %w", err)`

## Docs Index

IMPORTANT: Read the relevant doc file BEFORE implementing pumped-go patterns.

| File | Content |
|------|---------|
| [docs/flows.md](docs/flows.md) | Flow execution, sub-flows, tag-based data, nil executor gotcha |
| [docs/testing.md](docs/testing.md) | WithPreset, table-driven tests, reactivity testing |
| [docs/troubleshooting.md](docs/troubleshooting.md) | 10 common issues with solutions |
| [docs/configuration.md](docs/configuration.md) | Dual-mode config, env vars, tag overrides |
| [docs/patterns.md](docs/patterns.md) | Repository, service, handler, worker patterns |
| [docs/enforcement.md](docs/enforcement.md) | Tier 2 + Tier 3 rules |

## Quick Reference

```go
// Resolve executor
value, err := pumped.Resolve(scope, MyExecutor)

// Execute flow
result, execNode, err := pumped.Exec(scope, ctx, MyFlow)

// Update for reactive dependencies
accessor := pumped.Accessor(scope, Config)
err := accessor.Update(newConfig)

// Testing with mocks
testScope := pumped.NewScope(
    pumped.WithPreset(UserRepo, mockRepo),
)
```

## Remember

- Package-level `var` for all executors
- Controllers for all dependencies (`.Get()` with error handling)
- `OnCleanup()` for all resources that need cleanup
- `scope.Dispose()` for proper shutdown
- Flows for traced operations, plain functions for simple transformations
- `WithPreset()` for testing
- Graceful shutdown with signal handling
- Examples directory has production-ready patterns
