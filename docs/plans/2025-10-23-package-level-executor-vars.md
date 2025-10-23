# Package-Level Executor Variables Implementation Plan

> **For Claude:** Use `${SUPERPOWERS_SKILLS_ROOT}/skills/collaboration/executing-plans/SKILL.md` to implement this plan task-by-task.

**Goal:** Replace `Define()` function pattern with package-level `var` declarations for executors across all examples.

**Architecture:** Change from functional graph creation (requiring `Define()` call) to declarative package-level executor variables. This eliminates extra function calls and makes executors directly accessible while maintaining zero side effects until scope resolution.

**Tech Stack:** Go 1.21+, pumped-go dependency injection library

---

## Task 1: Refactor http-api example graph

**Files:**
- Modify: `examples/http-api/graph/graph.go`
- Modify: `examples/http-api/main.go:28`
- Modify: `examples/http-api/handlers/handlers.go:15,34,73,98,138,163`
- Test: `examples/http-api/graph/graph_test.go`

**Step 1: Backup current graph pattern for reference**

```bash
git diff examples/http-api/graph/graph.go > /tmp/graph-before.txt
```

**Step 2: Replace Define() function with package-level vars**

In `examples/http-api/graph/graph.go`, replace lines 10-89 with:

```go
type ConfigType struct {
	MaxUsersCache   int
	RateLimitPerMin int
}

var (
	// Configuration (no dependencies)
	Config = pumped.Provide(func(ctx *pumped.ResolveCtx) (*ConfigType, error) {
		return &ConfigType{
			MaxUsersCache:   100,
			RateLimitPerMin: 60,
		}, nil
	})

	// Infrastructure
	Storage = pumped.Provide(func(ctx *pumped.ResolveCtx) (storage.Storage, error) {
		return storage.NewMemoryStorage(), nil
	})

	// Services
	UserService = pumped.Derive1(
		Storage,
		func(ctx *pumped.ResolveCtx, storageCtrl *pumped.Controller[storage.Storage]) (*services.UserService, error) {
			store, err := storageCtrl.Get()
			if err != nil {
				return nil, err
			}
			return services.NewUserService(store), nil
		},
	)

	PostService = pumped.Derive2(
		Storage,
		UserService,
		func(ctx *pumped.ResolveCtx,
			storageCtrl *pumped.Controller[storage.Storage],
			userServiceCtrl *pumped.Controller[*services.UserService]) (*services.PostService, error) {
			store, err := storageCtrl.Get()
			if err != nil {
				return nil, err
			}
			userSvc, err := userServiceCtrl.Get()
			if err != nil {
				return nil, err
			}
			return services.NewPostService(store, userSvc), nil
		},
	)

	StatsService = pumped.Derive2(
		Storage,
		Config.Reactive(),
		func(ctx *pumped.ResolveCtx,
			storageCtrl *pumped.Controller[storage.Storage],
			configCtrl *pumped.Controller[*ConfigType]) (*services.StatsService, error) {
			store, err := storageCtrl.Get()
			if err != nil {
				return nil, err
			}
			cfg, err := configCtrl.Get()
			if err != nil {
				return nil, err
			}
			return services.NewStatsService(store, cfg.MaxUsersCache), nil
		},
	)
)
```

**Step 3: Remove Graph struct definition**

Delete the `Graph` struct (lines 10-16 in original).

**Step 4: Update main.go to remove Define() call**

In `examples/http-api/main.go:28`, change:
```go
g := graph.Define()

mux := http.NewServeMux()
handlers.Register(mux, scope, g)
```

to:

```go
mux := http.NewServeMux()
handlers.Register(mux, scope)
```

**Step 5: Update handlers.go Register function signature**

In `examples/http-api/handlers/handlers.go:15`, change:
```go
func Register(mux *http.ServeMux, scope *pumped.Scope, g *graph.Graph) {
```

to:

```go
func Register(mux *http.ServeMux, scope *pumped.Scope) {
```

**Step 6: Update all handler functions to use direct package references**

In `examples/http-api/handlers/handlers.go`:

- Line 17: Change `handleUsers(scope, g)` to `handleUsers(scope)`
- Line 18: Change `handleUserByID(scope, g)` to `handleUserByID(scope)`
- Line 19: Change `handlePosts(scope, g)` to `handlePosts(scope)`
- Line 20: Change `handlePostByID(scope, g)` to `handlePostByID(scope)`
- Line 21: Change `handleStats(scope, g)` to `handleStats(scope)`

**Step 7: Update handler function signatures and implementations**

Replace each handler function's signature and Graph usage:

```go
// Line 34
func handleUsers(scope *pumped.Scope) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userSvc, err := pumped.Resolve(scope, graph.UserService)
		// ... rest unchanged
	}
}

// Line 73
func handleUserByID(scope *pumped.Scope) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// ... id parsing unchanged
		userSvc, err := pumped.Resolve(scope, graph.UserService)
		// ... rest unchanged
	}
}

// Line 98
func handlePosts(scope *pumped.Scope) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		postSvc, err := pumped.Resolve(scope, graph.PostService)
		// ... rest unchanged
	}
}

// Line 138
func handlePostByID(scope *pumped.Scope) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// ... id parsing unchanged
		postSvc, err := pumped.Resolve(scope, graph.PostService)
		// ... rest unchanged
	}
}

// Line 163
func handleStats(scope *pumped.Scope) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		statsSvc, err := pumped.Resolve(scope, graph.StatsService)
		// ... rest unchanged
	}
}
```

**Step 8: Run tests to verify http-api example**

```bash
cd examples/http-api && go test ./... -v
```

Expected: All tests pass

**Step 9: Build and smoke test http-api**

```bash
cd examples/http-api && go build -o /tmp/http-api-test .
```

Expected: Clean build with no errors

**Step 10: Commit http-api changes**

```bash
git add examples/http-api/
git commit -m "refactor(http-api): replace Define() with package-level executor vars

Replace Graph.Define() pattern with direct package-level var declarations
for executors. This eliminates extra function call while maintaining zero
side effects until scope resolution.

- Remove Graph struct wrapper
- Export executors as package vars (Config, Storage, UserService, etc)
- Update handlers to reference graph.ExecutorName directly
- Maintain dependency order: Config → Infrastructure → Services"
```

---

## Task 2: Refactor cli-tasks example graph

**Files:**
- Modify: `examples/cli-tasks/graph/graph.go`
- Modify: `examples/cli-tasks/main.go:25`
- Modify: `examples/cli-tasks/commands/commands.go:13,34,66,90`
- Test: `examples/cli-tasks/graph/graph_test.go`

**Step 1: Replace Define() function with package-level vars**

In `examples/cli-tasks/graph/graph.go`, replace lines 10-77 with:

```go
type ConfigType struct {
	StorageType string
	FilePath    string
}

var (
	// Configuration (no dependencies)
	Config = pumped.Provide(func(ctx *pumped.ResolveCtx) (*ConfigType, error) {
		return &ConfigType{
			StorageType: "memory",
			FilePath:    "tasks.json",
		}, nil
	})

	// Infrastructure
	Storage = pumped.Derive1(
		Config,
		func(ctx *pumped.ResolveCtx, cfgCtrl *pumped.Controller[*ConfigType]) (storage.Storage, error) {
			cfg, err := cfgCtrl.Get()
			if err != nil {
				return nil, err
			}

			switch cfg.StorageType {
			case "memory":
				return storage.NewMemoryStorage(), nil
			case "file":
				return storage.NewFileStorage(cfg.FilePath)
			default:
				return storage.NewMemoryStorage(), nil
			}
		},
	)

	// Services
	TaskService = pumped.Derive1(
		Storage,
		func(ctx *pumped.ResolveCtx, storageCtrl *pumped.Controller[storage.Storage]) (*services.TaskService, error) {
			store, err := storageCtrl.Get()
			if err != nil {
				return nil, err
			}
			return services.NewTaskService(store), nil
		},
	)

	StatsService = pumped.Derive1(
		Storage,
		func(ctx *pumped.ResolveCtx, storageCtrl *pumped.Controller[storage.Storage]) (*services.StatsService, error) {
			store, err := storageCtrl.Get()
			if err != nil {
				return nil, err
			}
			return services.NewStatsService(store), nil
		},
	)
)
```

**Step 2: Remove Graph struct definition**

Delete the `Graph` struct (lines 10-15 in original).

**Step 3: Update main.go to remove Define() call**

In `examples/cli-tasks/main.go:25`, change:
```go
g := graph.Define()

cmd := os.Args[1]
```

to:

```go
cmd := os.Args[1]
```

**Step 4: Update command function calls to remove graph parameter**

In `examples/cli-tasks/main.go`:

- Line 32: Change `commands.Add(scope, g, args)` to `commands.Add(scope, args)`
- Line 37: Change `commands.List(scope, g, args)` to `commands.List(scope, args)`
- Line 42: Change `commands.Complete(scope, g, args)` to `commands.Complete(scope, args)`
- Line 47: Change `commands.Stats(scope, g, args)` to `commands.Stats(scope, args)`

**Step 5: Update command function signatures**

In `examples/cli-tasks/commands/commands.go`:

```go
// Line 13
func Add(scope *pumped.Scope, args []string) error {
	// ... title validation unchanged
	taskSvc, err := pumped.Resolve(scope, graph.TaskService)
	// ... rest unchanged
}

// Line 34
func List(scope *pumped.Scope, args []string) error {
	// ... filter logic unchanged
	taskSvc, err := pumped.Resolve(scope, graph.TaskService)
	// ... rest unchanged
}

// Line 66
func Complete(scope *pumped.Scope, args []string) error {
	// ... id parsing unchanged
	taskSvc, err := pumped.Resolve(scope, graph.TaskService)
	// ... rest unchanged
}

// Line 90
func Stats(scope *pumped.Scope, args []string) error {
	statsSvc, err := pumped.Resolve(scope, graph.StatsService)
	// ... rest unchanged
}
```

**Step 6: Run tests to verify cli-tasks example**

```bash
cd examples/cli-tasks && go test ./... -v
```

Expected: All tests pass

**Step 7: Build and smoke test cli-tasks**

```bash
cd examples/cli-tasks && go build -o /tmp/cli-tasks-test .
/tmp/cli-tasks-test add "Test task"
/tmp/cli-tasks-test list
```

Expected: Clean build, task added and listed successfully

**Step 8: Commit cli-tasks changes**

```bash
git add examples/cli-tasks/
git commit -m "refactor(cli-tasks): replace Define() with package-level executor vars

Replace Graph.Define() pattern with direct package-level var declarations
for executors. Removes Graph struct wrapper and Define() call overhead.

- Export executors as package vars (Config, Storage, TaskService, etc)
- Update commands to reference graph.ExecutorName directly
- Maintain dependency order: Config → Infrastructure → Services"
```

---

## Task 3: Refactor health-monitor example graph

**Files:**
- Modify: `examples/health-monitor/graph.go`
- Modify: `examples/health-monitor/main.go:14,18,24,30,35,42`
- Test: `examples/health-monitor/graph_integration_test.go`

**Step 1: Replace DefineGraph() function with package-level vars**

In `examples/health-monitor/graph.go`, replace lines 9-192 with:

```go
var (
	// Configuration (no dependencies)
	Config = pumped.Provide(func(ctx *pumped.ResolveCtx) (*Config, error) {
		return DefaultConfig(), nil
	})

	// Infrastructure - Logger
	Logger = pumped.Derive1(
		Config.Reactive(),
		func(ctx *pumped.ResolveCtx, cfgCtrl *pumped.Controller[*Config]) (*Logger, error) {
			cfg, err := cfgCtrl.Get()
			if err != nil {
				return nil, err
			}
			return NewLogger(cfg.LogLevel, nil), nil
		},
	)

	// Infrastructure - Database
	DB = pumped.Derive1(
		Config.Reactive(),
		func(ctx *pumped.ResolveCtx, cfgCtrl *pumped.Controller[*Config]) (*sql.DB, error) {
			cfg, err := cfgCtrl.Get()
			if err != nil {
				return nil, err
			}
			database, err := NewDB(cfg.DBPath)
			if err != nil {
				return nil, err
			}

			ctx.OnCleanup(func() error {
				return database.Close()
			})

			return database, nil
		},
	)

	// Repositories
	ServiceRepo = pumped.Derive1(
		DB,
		func(ctx *pumped.ResolveCtx, dbCtrl *pumped.Controller[*sql.DB]) (ServiceRepo, error) {
			database, err := dbCtrl.Get()
			if err != nil {
				return nil, err
			}
			return NewServiceRepository(database), nil
		},
	)

	HealthRepo = pumped.Derive1(
		DB,
		func(ctx *pumped.ResolveCtx, dbCtrl *pumped.Controller[*sql.DB]) (HealthCheckRepo, error) {
			database, err := dbCtrl.Get()
			if err != nil {
				return nil, err
			}
			return NewHealthCheckRepository(database), nil
		},
	)

	IncidentRepo = pumped.Derive1(
		DB,
		func(ctx *pumped.ResolveCtx, dbCtrl *pumped.Controller[*sql.DB]) (IncidentRepo, error) {
			database, err := dbCtrl.Get()
			if err != nil {
				return nil, err
			}
			return NewIncidentRepository(database), nil
		},
	)

	// Core Services
	HealthChecker = pumped.Provide(func(ctx *pumped.ResolveCtx) (*HealthChecker, error) {
		return NewHealthChecker(), nil
	})

	IncidentDetector = pumped.Derive1(
		IncidentRepo,
		func(ctx *pumped.ResolveCtx, repoCtrl *pumped.Controller[IncidentRepo]) (*IncidentDetector, error) {
			repo, err := repoCtrl.Get()
			if err != nil {
				return nil, err
			}
			return NewIncidentDetector(repo), nil
		},
	)

	Scheduler = pumped.Derive5(
		ServiceRepo,
		HealthRepo,
		HealthChecker,
		IncidentDetector,
		Logger,
		func(ctx *pumped.ResolveCtx,
			srCtrl *pumped.Controller[ServiceRepo],
			hrCtrl *pumped.Controller[HealthCheckRepo],
			hcCtrl *pumped.Controller[*HealthChecker],
			idCtrl *pumped.Controller[*IncidentDetector],
			logCtrl *pumped.Controller[*Logger]) (*Scheduler, error) {
			sr, _ := srCtrl.Get()
			hr, _ := hrCtrl.Get()
			hc, _ := hcCtrl.Get()
			id, _ := idCtrl.Get()
			log, _ := logCtrl.Get()

			sched := NewScheduler(sr, hr, hc, id, log)
			sched.Start()

			ctx.OnCleanup(func() error {
				sched.Stop()
				return nil
			})

			return sched, nil
		},
	)

	// HTTP Handlers
	ServiceHandler = pumped.Derive2(
		ServiceRepo,
		HealthRepo,
		func(ctx *pumped.ResolveCtx,
			srCtrl *pumped.Controller[ServiceRepo],
			hrCtrl *pumped.Controller[HealthCheckRepo]) (*ServiceHandler, error) {
			sr, _ := srCtrl.Get()
			hr, _ := hrCtrl.Get()
			return NewServiceHandler(sr, hr), nil
		},
	)

	HealthHandler = pumped.Derive3(
		ServiceRepo,
		HealthRepo,
		HealthChecker,
		func(ctx *pumped.ResolveCtx,
			srCtrl *pumped.Controller[ServiceRepo],
			hrCtrl *pumped.Controller[HealthCheckRepo],
			hcCtrl *pumped.Controller[*HealthChecker]) (*HealthHandler, error) {
			sr, _ := srCtrl.Get()
			hr, _ := hrCtrl.Get()
			hc, _ := hcCtrl.Get()
			return NewHealthHandler(sr, hr, hc), nil
		},
	)

	IncidentHandler = pumped.Derive1(
		IncidentRepo,
		func(ctx *pumped.ResolveCtx, irCtrl *pumped.Controller[IncidentRepo]) (*IncidentHandler, error) {
			ir, _ := irCtrl.Get()
			return NewIncidentHandler(ir), nil
		},
	)
)
```

**Step 2: Remove Graph struct definition**

Delete the `Graph` struct (lines 9-27 in original).

**Step 3: Update main.go to remove DefineGraph() call and use direct references**

In `examples/health-monitor/main.go`, change:

```go
func main() {
	g := DefineGraph()

	scope := pumped.NewScope()

	logger, err := pumped.Resolve(scope, g.Logger)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to resolve logger: %v\n", err)
		os.Exit(1)
	}

	_, err = pumped.Resolve(scope, g.Scheduler)
	if err != nil {
		logger.Error("failed to resolve scheduler: %v", err)
		os.Exit(1)
	}

	serviceHandler, err := pumped.Resolve(scope, g.ServiceHandler)
	if err != nil {
		logger.Error("failed to resolve service handler: %v", err)
		os.Exit(1)
	}

	healthHandler, err := pumped.Resolve(scope, g.HealthHandler)
	if err != nil {
		logger.Error("failed to resolve health handler: %v", err)
		os.Exit(1)
	}

	incidentHandler, err := pumped.Resolve(scope, g.IncidentHandler)
	if err != nil {
		logger.Error("failed to resolve incident handler: %v", err)
		os.Exit(1)
	}
```

to:

```go
func main() {
	scope := pumped.NewScope()

	logger, err := pumped.Resolve(scope, Logger)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to resolve logger: %v\n", err)
		os.Exit(1)
	}

	_, err = pumped.Resolve(scope, Scheduler)
	if err != nil {
		logger.Error("failed to resolve scheduler: %v", err)
		os.Exit(1)
	}

	serviceHandler, err := pumped.Resolve(scope, ServiceHandler)
	if err != nil {
		logger.Error("failed to resolve service handler: %v", err)
		os.Exit(1)
	}

	healthHandler, err := pumped.Resolve(scope, HealthHandler)
	if err != nil {
		logger.Error("failed to resolve health handler: %v", err)
		os.Exit(1)
	}

	incidentHandler, err := pumped.Resolve(scope, IncidentHandler)
	if err != nil {
		logger.Error("failed to resolve incident handler: %v", err)
		os.Exit(1)
	}
```

**Step 4: Run tests to verify health-monitor example**

```bash
cd examples/health-monitor && go test ./... -v
```

Expected: All tests pass

**Step 5: Build and smoke test health-monitor**

```bash
cd examples/health-monitor && go build -o /tmp/health-monitor-test .
```

Expected: Clean build with no errors

**Step 6: Commit health-monitor changes**

```bash
git add examples/health-monitor/
git commit -m "refactor(health-monitor): replace DefineGraph() with package-level executor vars

Replace Graph.DefineGraph() pattern with direct package-level var declarations
for executors. Removes Graph struct wrapper and function call overhead.

- Export executors as package vars (Config, Logger, DB, ServiceRepo, etc)
- Update main.go to reference executors directly
- Maintain dependency order: Config → Infrastructure → Repositories → Services → Handlers"
```

---

## Task 4: Run full test suite and verify

**Files:**
- Test: All examples

**Step 1: Run all example tests**

```bash
cd /home/lagz0ne/dev/pumped-go && devbox run test
```

Expected: All tests pass across all examples

**Step 2: Run lint checks**

```bash
cd /home/lagz0ne/dev/pumped-go && devbox run lint
```

Expected: No linting errors

**Step 3: Build all examples**

```bash
cd /home/lagz0ne/dev/pumped-go && devbox run build-examples
```

Expected: All examples build successfully

**Step 4: Final verification commit**

If all tests pass:

```bash
git add -A
git commit -m "test: verify all examples after executor var refactoring

All tests passing, builds clean after replacing Define() pattern with
package-level executor variables across http-api, cli-tasks, and
health-monitor examples."
```

---

## Notes

- **Dependency Order Convention**: Config → Infrastructure → Services
- **Zero Side Effects**: Package init only builds graph structure, no execution until scope resolution
- **Backwards Compatibility**: This is a breaking change for examples only, core library unchanged
- **Testing**: Each task includes test verification before committing
- **Atomic Commits**: Each example refactored in separate commit for easy rollback if needed
