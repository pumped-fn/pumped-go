# Testing Strategies

## Testing Executors with WithPreset

**Replace executors with mocks/test implementations:**

```go
func TestGraph_UserService(t *testing.T) {
    // Create mock repository
    mockRepo := &MockUserRepository{
        users: map[string]*User{
            "user-1": {ID: "user-1", Name: "Alice"},
        },
    }

    // Create test scope with preset
    testScope := pumped.NewScope(
        pumped.WithPreset(UserRepo, mockRepo),
    )
    defer testScope.Dispose()

    // Resolve service (will get mock repo)
    service, err := pumped.Resolve(testScope, UserService)
    if err != nil {
        t.Fatalf("failed to resolve service: %v", err)
    }

    // Test service
    user, err := service.GetUser("user-1")
    if err != nil {
        t.Fatalf("failed to get user: %v", err)
    }

    if user.Name != "Alice" {
        t.Errorf("expected name Alice, got %s", user.Name)
    }
}
```

## Table-Driven Tests (Go Idiom)

```go
func TestGraph_AllComponentsResolve(t *testing.T) {
    testScope := pumped.NewScope(
        pumped.WithPreset(Config, &Config{DBPath: ":memory:"}),
    )
    defer testScope.Dispose()

    tests := []struct {
        name string
        fn   func() error
    }{
        {"Logger", func() error {
            _, err := pumped.Resolve(testScope, Logger)
            return err
        }},
        {"DB", func() error {
            _, err := pumped.Resolve(testScope, DB)
            return err
        }},
        {"UserRepo", func() error {
            _, err := pumped.Resolve(testScope, UserRepo)
            return err
        }},
        {"UserService", func() error {
            _, err := pumped.Resolve(testScope, UserService)
            return err
        }},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            if err := tt.fn(); err != nil {
                t.Errorf("failed to resolve %s: %v", tt.name, err)
            }
        })
    }
}
```

## Testing Reactivity

```go
func TestGraph_ConfigReactivity(t *testing.T) {
    initialConfig := &Config{LogLevel: "info"}

    testScope := pumped.NewScope(
        pumped.WithPreset(Config, initialConfig),
    )
    defer testScope.Dispose()

    // Resolve logger (reactive to Config)
    logger1, _ := pumped.Resolve(testScope, Logger)
    if logger1.Level != "info" {
        t.Errorf("expected info level, got %s", logger1.Level)
    }

    // Update config
    configAcc := pumped.Accessor(testScope, Config)
    newConfig := &Config{LogLevel: "debug"}
    err := configAcc.Update(newConfig)
    if err != nil {
        t.Fatalf("failed to update config: %v", err)
    }

    // Resolve logger again (should be new instance)
    logger2, _ := pumped.Resolve(testScope, Logger)
    if logger1 == logger2 {
        t.Error("expected logger to be re-initialized")
    }
    if logger2.Level != "debug" {
        t.Errorf("expected debug level, got %s", logger2.Level)
    }
}
```

## Testing Flows

```go
func TestFlow_FetchUser(t *testing.T) {
    mockRepo := &MockUserRepository{
        users: map[string]*User{
            "user-1": {ID: "user-1", Name: "Alice"},
        },
    }

    testScope := pumped.NewScope(
        pumped.WithPreset(UserRepo, mockRepo),
    )
    defer testScope.Dispose()

    ctx := context.Background()

    // Execute flow
    result, execNode, err := pumped.Exec(testScope, ctx, FetchUserFlow)
    if err != nil {
        t.Fatalf("flow execution failed: %v", err)
    }

    if result.Name != "Alice" {
        t.Errorf("expected Alice, got %s", result.Name)
    }

    // Verify execution metadata
    if name, ok := execNode.Get(pumped.FlowName()); ok {
        if name != "fetchUser" {
            t.Errorf("expected flow name fetchUser, got %s", name)
        }
    }
}
```

## Integration Tests

```go
func TestIntegration_HealthMonitor(t *testing.T) {
    if testing.Short() {
        t.Skip("skipping integration test in short mode")
    }

    // Use real implementations
    testScope := pumped.NewScope(
        pumped.WithPreset(Config, &Config{
            DBPath:   ":memory:",  // In-memory SQLite
            LogLevel: "info",
        }),
    )
    defer testScope.Dispose()

    // Resolve and test with real database
    db, err := pumped.Resolve(testScope, DB)
    if err != nil {
        t.Fatalf("failed to resolve DB: %v", err)
    }

    // Run migrations, seed data, etc.
    // Test actual database operations
}
```

## Testing Best Practices

**DO:**
- Test all executors resolve without errors
- Test reactivity when using `.Reactive()`
- Use table-driven tests (Go idiom)
- Separate unit tests from integration tests
- Use `WithPreset()` to inject mocks

**DON'T:**
- Test implementation details
- Skip error handling in tests
- Mix unit and integration tests
- Ignore cleanup (`defer testScope.Dispose()`)
