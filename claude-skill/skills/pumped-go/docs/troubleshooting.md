# Troubleshooting Common Issues

## Problem: Flows defined but not executing

**Symptom:** You created flows but they're never called, or handlers don't use them.

**Solution:** Flows must be explicitly executed with `pumped.Exec()`:

```go
// Wrong - Flow defined but never used
var MyFlow = pumped.Flow1(UserRepo, func(...) { ... })

func (h *Handler) DoWork(w http.ResponseWriter, r *http.Request) {
    // Handler calls services directly, flow is ignored
    result := h.service.DoWork()
}

// Correct - Execute flow from handler
func (h *Handler) DoWork(w http.ResponseWriter, r *http.Request) {
    result, _, err := pumped.Exec(h.scope, r.Context(), MyFlow)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    // Use result...
}
```

## Problem: Nil pointer dereference in flow execution

**Symptom:** `panic: nil pointer dereference` when executing flow, or `nil executor` error.

**Cause:** Flow defined with nil executors instead of executor references.

**Solution:** Use direct executor references:

```go
// Wrong
var MyFlow = pumped.Flow1(
    nil,  // This causes nil pointer errors
    func(...) { ... },
)

// Correct
var (
    UserRepo = pumped.Derive1(DB, func(...) (*UserRepository, error) { ... })

    MyFlow = pumped.Flow1(
        UserRepo,  // Direct reference to executor
        func(...) { ... },
    )
)
```

## Problem: Resource leaks on shutdown

**Symptom:** Database connections, goroutines, or file handles not cleaned up. Application hangs on shutdown.

**Cause:** Missing `OnCleanup()` registration or missing `scope.Dispose()`.

**Solution 1:** Register cleanup in executor:

```go
var DB = pumped.Derive1(Config, func(ctx *pumped.ResolveCtx, ...) (*sql.DB, error) {
    db, err := sql.Open(...)
    if err != nil {
        return nil, err
    }

    // Register cleanup
    ctx.OnCleanup(func() error {
        log.Println("Closing database connection")
        return db.Close()
    })

    return db, nil
})
```

**Solution 2:** Always dispose scope:

```go
func main() {
    scope := pumped.NewScope()
    defer scope.Dispose()  // CRITICAL - runs all OnCleanup functions

    // Application code...
}
```

**Solution 3:** Stop goroutines in cleanup:

```go
var Worker = pumped.Derive1(Logger, func(ctx *pumped.ResolveCtx, ...) (*Worker, error) {
    worker := NewWorker()
    worker.Start()  // Starts background goroutine

    // Register goroutine cleanup
    ctx.OnCleanup(func() error {
        log.Println("Stopping worker")
        worker.Stop()  // Must stop goroutine!
        return nil
    })

    return worker, nil
})
```

## Problem: Config changes don't propagate

**Symptom:** Updated config via `Accessor.Update()` but dependent services didn't reload.

**Cause:** Forgot `.Reactive()` on dependency declaration.

**Solution:** Use `.Reactive()` for dependencies that should reload:

```go
// Wrong - Static dependency (won't reload)
var Service = pumped.Derive1(
    Config,  // Static mode (default)
    func(...) { ... },
)

// Correct - Reactive dependency (reloads on config update)
var Service = pumped.Derive1(
    Config.Reactive(),  // Will re-resolve when Config updates
    func(ctx *pumped.ResolveCtx, cfgCtrl *pumped.Controller[*Config]) (*Service, error) {
        cfg, err := cfgCtrl.Get()
        if err != nil {
            return nil, err
        }
        return NewService(cfg.MaxConnections), nil
    },
)

// Update config (triggers reactive dependents)
configAcc := pumped.Accessor(scope, Config)
err := configAcc.Update(newConfig)
```

## Problem: Tests fail with "executor not found" or similar

**Symptom:** Tests can't resolve executors or get unexpected errors.

**Cause:** Test scope doesn't have required presets or wrong executor referenced.

**Solution:** Use `WithPreset()` to provide test implementations:

```go
func TestService(t *testing.T) {
    // Create mock
    mockRepo := &MockUserRepository{
        users: map[string]*User{
            "test-id": {ID: "test-id", Name: "Test User"},
        },
    }

    // Create test scope with preset
    testScope := pumped.NewScope(
        pumped.WithPreset(UserRepo, mockRepo),  // Provide mock
    )
    defer testScope.Dispose()

    // Resolve service (will get mock repo)
    service, err := pumped.Resolve(testScope, UserService)
    if err != nil {
        t.Fatalf("failed to resolve: %v", err)
    }

    // Test with mock...
}
```

## Problem: Executor resolution fails with dependency errors

**Symptom:** `failed to get <dependency>: ...` errors during executor resolution.

**Cause:** Dependency chain broken, or executor factory returns error.

**Solution:** Check entire dependency chain:

```go
// If UserService fails to resolve:
// 1. Check UserService factory
var UserService = pumped.Derive1(
    UserRepo,
    func(ctx *pumped.ResolveCtx, repoCtrl *pumped.Controller[*UserRepository]) (*UserService, error) {
        repo, err := repoCtrl.Get()  // Check this error
        if err != nil {
            return nil, fmt.Errorf("failed to get repo: %w", err)  // This tells you the issue
        }
        return NewUserService(repo), nil
    },
)

// 2. Check UserRepo factory
var UserRepo = pumped.Derive1(
    DB,
    func(ctx *pumped.ResolveCtx, dbCtrl *pumped.Controller[*sql.DB]) (*UserRepository, error) {
        db, err := dbCtrl.Get()  // Check this error
        if err != nil {
            return nil, fmt.Errorf("failed to get DB: %w", err)  // Propagates the issue
        }
        return NewUserRepository(db), nil
    },
)

// 3. Check DB factory (root cause might be here)
var DB = pumped.Derive1(
    Config,
    func(ctx *pumped.ResolveCtx, cfgCtrl *pumped.Controller[*Config]) (*sql.DB, error) {
        cfg, err := cfgCtrl.Get()
        if err != nil {
            return nil, fmt.Errorf("failed to get config: %w", err)
        }
        db, err := sql.Open("postgres", cfg.DSN)
        if err != nil {
            return nil, fmt.Errorf("failed to open DB: %w", err)  // Root cause
        }
        return db, nil
    },
)
```

**Debugging tip:** Errors propagate up the chain with context. Read the full error message to trace back to the root cause.

## Problem: Application hangs on shutdown

**Symptom:** Application doesn't exit cleanly when interrupted (Ctrl+C).

**Cause:** Missing signal handling or cleanup taking too long.

**Solution:** Implement proper graceful shutdown:

```go
func main() {
    scope := pumped.NewScope()
    defer scope.Dispose()

    // Setup signal handling
    sigCh := make(chan os.Signal, 1)
    signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

    // Start server in goroutine
    go func() {
        if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
            log.Fatalf("server error: %v", err)
        }
    }()

    // Wait for signal
    <-sigCh
    log.Println("Shutting down...")

    // Shutdown server with timeout
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    if err := server.Shutdown(ctx); err != nil {
        log.Printf("server shutdown error: %v", err)
    }

    // scope.Dispose() runs via defer (cleanup executors)
    log.Println("Shutdown complete")
}
```

## Problem: Circular dependency error

**Symptom:** `circular dependency detected` error during resolution.

**Cause:** Executor A depends on B, and B depends on A (directly or indirectly).

**Solution:** Restructure dependencies:

```go
// Wrong - circular dependency
var A = pumped.Derive1(B, func(...) { ... })
var B = pumped.Derive1(A, func(...) { ... })

// Correct - extract shared logic
var Shared = pumped.Provide(func(...) { ... })
var A = pumped.Derive1(Shared, func(...) { ... })
var B = pumped.Derive1(Shared, func(...) { ... })
```

## Problem: Memory leak with long-running scope

**Symptom:** Memory usage grows over time, especially with execution tree.

**Cause:** Execution tree accumulates nodes without bound.

**Solution:** Execution tree has a default limit of 1000 nodes. Oldest roots are evicted automatically. For very high-throughput scenarios, consider:

1. Periodic scope recreation (for CLI-like usage patterns)
2. Explicit tree management via extensions
3. Increasing node limit if memory permits
