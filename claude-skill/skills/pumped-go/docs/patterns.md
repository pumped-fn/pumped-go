# Common Patterns

## Repository Pattern with Executors

```go
var (
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

    PostRepo = pumped.Derive1(
        DB,
        func(ctx *pumped.ResolveCtx, dbCtrl *pumped.Controller[*sql.DB]) (*PostRepository, error) {
            db, err := dbCtrl.Get()
            if err != nil {
                return nil, err
            }
            return NewPostRepository(db), nil
        },
    )
)
```

## Service Layer Composition

```go
var (
    UserService = pumped.Derive2(
        UserRepo,
        Logger,
        func(ctx *pumped.ResolveCtx,
            userRepoCtrl *pumped.Controller[*UserRepository],
            logCtrl *pumped.Controller[*Logger]) (*UserService, error) {

            userRepo, err := userRepoCtrl.Get()
            if err != nil {
                return nil, err
            }
            log, err := logCtrl.Get()
            if err != nil {
                return nil, err
            }

            return NewUserService(userRepo, log), nil
        },
    )

    PostService = pumped.Derive3(
        PostRepo,
        UserService,  // Service depends on another service
        Logger,
        func(ctx *pumped.ResolveCtx,
            postRepoCtrl *pumped.Controller[*PostRepository],
            userServiceCtrl *pumped.Controller[*UserService],
            logCtrl *pumped.Controller[*Logger]) (*PostService, error) {

            postRepo, err := postRepoCtrl.Get()
            if err != nil {
                return nil, err
            }
            userService, err := userServiceCtrl.Get()
            if err != nil {
                return nil, err
            }
            log, err := logCtrl.Get()
            if err != nil {
                return nil, err
            }

            return NewPostService(postRepo, userService, log), nil
        },
    )
)
```

## Handler Dependency Injection

```go
var (
    UserHandler = pumped.Derive2(
        UserService,
        Logger,
        func(ctx *pumped.ResolveCtx,
            userServiceCtrl *pumped.Controller[*UserService],
            logCtrl *pumped.Controller[*Logger]) (*UserHandler, error) {

            userService, err := userServiceCtrl.Get()
            if err != nil {
                return nil, err
            }
            log, err := logCtrl.Get()
            if err != nil {
                return nil, err
            }

            return &UserHandler{
                userService: userService,
                logger:      log,
            }, nil
        },
    )
)

func main() {
    scope := pumped.NewScope()
    defer scope.Dispose()

    userHandler, _ := pumped.Resolve(scope, UserHandler)

    mux := http.NewServeMux()
    mux.HandleFunc("GET /users/{id}", userHandler.GetUser)
    mux.HandleFunc("POST /users", userHandler.CreateUser)

    // Server setup...
}
```

## Background Worker Lifecycle

```go
var Scheduler = pumped.Derive2(
    ServiceRepo,
    Logger,
    func(ctx *pumped.ResolveCtx,
        serviceRepoCtrl *pumped.Controller[ServiceRepo],
        logCtrl *pumped.Controller[*Logger]) (*Scheduler, error) {

        serviceRepo, err := serviceRepoCtrl.Get()
        if err != nil {
            return nil, err
        }
        log, err := logCtrl.Get()
        if err != nil {
            return nil, err
        }

        sched := NewScheduler(serviceRepo, log)
        sched.Start()  // Starts background goroutine

        ctx.OnCleanup(func() error {
            log.Info("Stopping scheduler")
            sched.Stop()  // Stops goroutine
            return nil
        })

        return sched, nil
    },
)

type Scheduler struct {
    serviceRepo ServiceRepo
    logger      *Logger
    ticker      *time.Ticker
    stopCh      chan struct{}
}

func (s *Scheduler) Start() {
    s.ticker = time.NewTicker(5 * time.Second)
    s.stopCh = make(chan struct{})

    go func() {
        for {
            select {
            case <-s.ticker.C:
                s.runHealthChecks()
            case <-s.stopCh:
                s.logger.Info("Scheduler stopped")
                return
            }
        }
    }()
}

func (s *Scheduler) Stop() {
    s.ticker.Stop()
    close(s.stopCh)
}
```

## CLI Command Pattern

```go
func main() {
    scope := pumped.NewScope()
    defer scope.Dispose()

    taskService, err := pumped.Resolve(scope, TaskService)
    if err != nil {
        log.Fatalf("failed to resolve service: %v", err)
    }

    app := &cli.App{
        Commands: []*cli.Command{
            {
                Name:  "add",
                Usage: "Add a new task",
                Action: func(c *cli.Context) error {
                    task := &Task{
                        Title: c.Args().First(),
                    }
                    return taskService.Create(c.Context, task)
                },
            },
            {
                Name:  "list",
                Usage: "List all tasks",
                Action: func(c *cli.Context) error {
                    tasks, err := taskService.List(c.Context)
                    if err != nil {
                        return err
                    }
                    for _, task := range tasks {
                        fmt.Printf("- %s\n", task.Title)
                    }
                    return nil
                },
            },
        },
    }

    if err := app.Run(os.Args); err != nil {
        log.Fatal(err)
    }
}
```

## Coding Style

### Flat Structure Over Subfolders

**CRITICAL:** Keep project structure flat. Avoid overcomplicated nested folders.

**CORRECT: Flat structure**
```
myapp/
├── main.go
├── graph.go          // All executors
├── graph_test.go
├── user.go           // User domain (repo + service)
├── user_test.go
├── order.go          // Order domain
├── order_test.go
└── worker.go         // Background workers
```

**WRONG: Over-nested structure**
```
myapp/
├── cmd/
│   └── server/
│       └── main.go
├── internal/
│   ├── domain/
│   │   ├── user/
│   │   │   ├── repository/
│   │   │   │   └── postgres/
│   │   │   │       └── user_repository.go
```

**Why flat structure:**
- Faster navigation (no deep nesting)
- Clearer dependencies (all in one file)
- Less boilerplate (no package-per-file overhead)
- Go idiom: organize by feature, not by layer

**When to create subdirectories:**
- Multiple related types need grouping (e.g., `models/` for 10+ domain models)
- Shared utilities used across features (e.g., `testutil/` for test helpers)
- Examples or docs (e.g., `examples/`, `docs/`)

### Zero Comments Rule

**CRITICAL:** Code should be self-documenting. No comments needed.

**CORRECT: Self-documenting code**
```go
var UserRepo = pumped.Derive1(
    DB,
    func(ctx *pumped.ResolveCtx, dbCtrl *pumped.Controller[*sql.DB]) (*UserRepository, error) {
        db, err := dbCtrl.Get()
        if err != nil {
            return nil, fmt.Errorf("failed to get database: %w", err)
        }

        ctx.OnCleanup(func() error {
            return db.Close()
        })

        return &UserRepository{db: db}, nil
    },
)
```

**WRONG: Over-commented code**
```go
// UserRepo is a repository for users
var UserRepo = pumped.Derive1(
    DB, // Database dependency
    func(ctx *pumped.ResolveCtx, dbCtrl *pumped.Controller[*sql.DB]) (*UserRepository, error) {
        // Get the database controller
        db, err := dbCtrl.Get()
        // ...
    },
)
```

**Exceptions (rare):**
- Complex algorithms requiring explanation (document the "why", not the "what")
- Non-obvious performance optimizations
- Package-level godoc for exported APIs
- TODO markers for known issues
