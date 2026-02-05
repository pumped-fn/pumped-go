# Configuration Patterns

## Dual-Mode Configuration

**CRITICAL:** Configuration should support both default values AND tag-based overrides via scope.

## Pattern 1: Default Configuration

**Every application needs sensible defaults.**

```go
var Config = pumped.Provide(func(ctx *pumped.ResolveCtx) (*Config, error) {
    return &Config{
        DBHost:     "localhost",
        DBPort:     5432,
        DBName:     "myapp",
        ServerPort: 8080,
        LogLevel:   "info",
    }, nil
})
```

## Pattern 2: Tag-Based Override

**Allow runtime configuration via scope tags.**

```go
var (
    // Define tags for configuration
    DBHostTag     = pumped.NewTag[string]("config.db.host")
    DBPortTag     = pumped.NewTag[int]("config.db.port")
    ServerPortTag = pumped.NewTag[int]("config.server.port")
)

var Config = pumped.Provide(func(ctx *pumped.ResolveCtx) (*Config, error) {
    cfg := &Config{
        DBHost:     "localhost",
        DBPort:     5432,
        DBName:     "myapp",
        ServerPort: 8080,
        LogLevel:   "info",
    }

    scope := ctx.Scope()

    if host, ok := scope.GetTag(DBHostTag); ok {
        cfg.DBHost = host
    }

    if port, ok := scope.GetTag(DBPortTag); ok {
        cfg.DBPort = port
    }

    if serverPort, ok := scope.GetTag(ServerPortTag); ok {
        cfg.ServerPort = serverPort
    }

    return cfg, nil
})
```

## Pattern 3: Usage in Main

**Set tags on scope creation for different environments.**

```go
func main() {
    scope := pumped.NewScope(
        pumped.WithTag(DBHostTag, os.Getenv("DB_HOST")),
        pumped.WithTag(DBPortTag, 5433),
        pumped.WithTag(ServerPortTag, 3000),
    )
    defer scope.Dispose()

    configCtrl := pumped.Controller(scope, Config)
    cfg, err := configCtrl.Get()
    if err != nil {
        log.Fatalf("failed to load config: %v", err)
    }

    log.Printf("Starting server on port %d", cfg.ServerPort)
}
```

## Pattern 4: Environment-Based Configuration

**Load from environment variables with defaults.**

```go
var Config = pumped.Provide(func(ctx *pumped.ResolveCtx) (*Config, error) {
    cfg := &Config{
        DBHost:     getEnv("DB_HOST", "localhost"),
        DBPort:     getEnvInt("DB_PORT", 5432),
        DBName:     getEnv("DB_NAME", "myapp"),
        ServerPort: getEnvInt("SERVER_PORT", 8080),
        LogLevel:   getEnv("LOG_LEVEL", "info"),
    }

    scope := ctx.Scope()

    if host, ok := scope.GetTag(DBHostTag); ok {
        cfg.DBHost = host
    }

    if port, ok := scope.GetTag(DBPortTag); ok {
        cfg.DBPort = port
    }

    return cfg, nil
})

func getEnv(key, defaultValue string) string {
    if value := os.Getenv(key); value != "" {
        return value
    }
    return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
    if value := os.Getenv(key); value != "" {
        if i, err := strconv.Atoi(value); err == nil {
            return i
        }
    }
    return defaultValue
}
```

## Configuration Priority

**Order of precedence (highest to lowest):**

1. **Scope tags** - Highest priority, set at runtime
2. **Environment variables** - Middle priority, set at deployment
3. **Default values** - Lowest priority, hardcoded in Config executor

**Example:**
```go
// main.go
scope := pumped.NewScope(
    pumped.WithTag(ServerPortTag, 9000), // Highest priority
)

// Config executor checks:
// 1. Scope tag (9000) - Used
// 2. Environment variable (8080) - Skipped
// 3. Default (8080) - Skipped
```

## Testing with Configuration

**Override config in tests using WithPreset.**

```go
func TestUserService(t *testing.T) {
    testConfig := &Config{
        DBHost: "localhost",
        DBPort: 5433,
        DBName: "testdb",
    }

    testScope := pumped.NewScope(
        pumped.WithPreset(Config, testConfig),
    )
    defer testScope.Dispose()

    serviceCtrl := pumped.Controller(testScope, UserService)
    service, err := serviceCtrl.Get()
    if err != nil {
        t.Fatalf("failed to get service: %v", err)
    }
}
```

## Configuration Best Practices

**DO:**
- Provide sensible defaults for local development
- Use environment variables for deployment configuration
- Use scope tags for test-specific overrides
- Keep configuration struct simple (basic types)
- Document environment variables in README

**DON'T:**
- Load configuration from files (use env vars or tags)
- Use global variables for configuration
- Parse flags in Config executor (do it in main, pass via tags)
- Make Config mutable (read-only after creation)
- Ignore errors from missing required configuration

## Configuration with Reactive Dependencies

**When configuration changes trigger re-resolution:**

```go
var (
    Config = pumped.Provide(func(ctx *pumped.ResolveCtx) (*Config, error) {
        scope := ctx.Scope()

        rateLimitTag := pumped.NewTag[int]("rateLimit")
        rateLimit := 100

        if limit, ok := scope.GetTag(rateLimitTag); ok {
            rateLimit = limit
        }

        return &Config{RateLimit: rateLimit}, nil
    })

    RateLimiter = pumped.Derive1(
        Config.Reactive(),
        func(ctx *pumped.ResolveCtx, cfgCtrl *pumped.Controller[*Config]) (*RateLimiter, error) {
            cfg, err := cfgCtrl.Get()
            if err != nil {
                return nil, err
            }

            return NewRateLimiter(cfg.RateLimit), nil
        },
    )
)
```

**When Config changes, RateLimiter automatically re-resolves with new rate limit.**
