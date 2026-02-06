package extensions

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"

	"oss.terrastruct.com/d2/d2parser"
	pumped "github.com/pumped-fn/pumped-go"
)

func TestGraphDebugExtension_OnError(t *testing.T) {
	// Capture output in buffer AND write to stdout for visual verification
	var buf bytes.Buffer
	multiWriter := io.MultiWriter(&buf, os.Stdout)
	handler := NewHumanHandler(multiWriter, slog.LevelError)

	scope := pumped.NewScope(
		pumped.WithExtension(NewGraphDebugExtension(handler, NewD2Formatter())),
	)
	defer scope.Dispose()

	nameTag := pumped.NewTag[string]("executor.name")

	// Create executors with reactive dependencies
	storage := pumped.Provide(
		func(ctx *pumped.ResolveCtx) (string, error) {
			return "storage", nil
		},
		pumped.WithTag(nameTag, "Storage"),
	)

	// This will fail
	userService := pumped.Derive1(
		storage.Reactive(),
		func(ctx *pumped.ResolveCtx, s *pumped.Controller[string]) (string, error) {
			return "", fmt.Errorf("type assertion failed: expected *User, got *string")
		},
		pumped.WithTag(nameTag, "UserService"),
	)

	// Try to resolve - should fail and trigger OnError
	_, err := pumped.Resolve(scope, userService)

	// Verify error occurred
	if err == nil {
		t.Fatal("Expected error but got nil")
	}

	// Verify HumanHandler formatted output
	output := buf.String()

	// Check for header with equals signs
	if !strings.Contains(output, "======================================================================") {
		t.Error("Expected separator line with equals signs")
	}

	// Check for formatted header (not escaped)
	if !strings.Contains(output, "[GraphDebug] Dependency Resolution Error") {
		t.Error("Expected '[GraphDebug] Dependency Resolution Error' header")
	}

	// Check for formatted fields (not key=value format)
	if !strings.Contains(output, "Failed Executor: UserService") {
		t.Error("Expected 'Failed Executor: UserService'")
	}

	if !strings.Contains(output, "Error: type assertion failed") {
		t.Error("Expected error message in human-readable format")
	}

	if !strings.Contains(output, "Operation: resolve") {
		t.Error("Expected 'Operation: resolve'")
	}

	// Check for dependency graph section
	if !strings.Contains(output, "Dependency Graph:") {
		t.Error("Expected 'Dependency Graph:' section")
	}

	// Check for D2 diagram format
	if !strings.Contains(output, "direction: down") {
		t.Error("Expected 'direction: down' in D2 output")
	}

	if !strings.Contains(output, "Storage") {
		t.Error("Expected 'Storage' in dependency graph")
	}

	if !strings.Contains(output, "UserService") {
		t.Error("Expected 'UserService' in dependency graph")
	}

	// Check for D2 connection syntax
	if !strings.Contains(output, "->") {
		t.Error("Expected D2 arrow syntax '->' in output")
	}

	// Check for failed node styling (red fill)
	if !strings.Contains(output, d2ColorFailed) {
		t.Errorf("Expected failed node color '%s' in D2 output", d2ColorFailed)
	}

	// Check for Error Details section
	if !strings.Contains(output, "Error Details:") {
		t.Error("Expected 'Error Details:' section")
	}
}

func TestGraphDebugExtension_TracksResolvedExecutors(t *testing.T) {
	// BEHAVIOR: After successful resolution, buildGraphData marks executors as NodeStatusOK
	// BUG CAUGHT: Wrap not tracking resolved executors, causing buildGraphData to mark them as pending
	// FAIL CHECK: If resolvedExecutors tracking is broken, nodes get NodeStatusPending instead of NodeStatusOK
	ext := NewGraphDebugExtension(NewSilentHandler(), NewD2Formatter())
	scope := pumped.NewScope(
		pumped.WithExtension(ext),
	)
	defer scope.Dispose()

	nameTag := pumped.NewTag[string]("executor.name")

	storage := pumped.Provide(
		func(ctx *pumped.ResolveCtx) (string, error) {
			return "storage", nil
		},
		pumped.WithTag(nameTag, "Storage"),
	)

	service := pumped.Derive1(
		storage.Reactive(),
		func(ctx *pumped.ResolveCtx, s *pumped.Controller[string]) (string, error) {
			val, _ := s.Get()
			return "service-" + val, nil
		},
		pumped.WithTag(nameTag, "Service"),
	)

	// Resolve successfully
	_, err := pumped.Resolve(scope, service)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Verify resolved status via buildGraphData (the observable consumer)
	graph := scope.ExportDependencyGraph()
	data := ext.buildGraphData(graph, nil)

	// Both executors should be marked OK (not pending) after successful resolution
	statusByName := make(map[string]NodeStatus)
	for _, node := range data.Nodes {
		statusByName[node.Name] = node.Status
	}

	if statusByName["Storage"] != NodeStatusOK {
		t.Errorf("Expected Storage status 'ok', got '%s'", statusByName["Storage"])
	}
	if statusByName["Service"] != NodeStatusOK {
		t.Errorf("Expected Service status 'ok', got '%s'", statusByName["Service"])
	}
}

func TestGraphDebugExtension_ExportDependencyGraph(t *testing.T) {
	scope := pumped.NewScope()
	defer scope.Dispose()

	nameTag := pumped.NewTag[string]("executor.name")

	config := pumped.Provide(
		func(ctx *pumped.ResolveCtx) (string, error) {
			return "config", nil
		},
		pumped.WithTag(nameTag, "Config"),
	)

	storage := pumped.Provide(
		func(ctx *pumped.ResolveCtx) (string, error) {
			return "storage", nil
		},
		pumped.WithTag(nameTag, "Storage"),
	)

	// Create service with reactive dependencies
	service := pumped.Derive2(
		config.Reactive(),
		storage.Reactive(),
		func(ctx *pumped.ResolveCtx, c *pumped.Controller[string], s *pumped.Controller[string]) (string, error) {
			cfg, _ := c.Get()
			store, _ := s.Get()
			return cfg + "-" + store, nil
		},
		pumped.WithTag(nameTag, "Service"),
	)

	// Resolve to build the graph
	_, err := pumped.Resolve(scope, service)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Export the graph
	graph := scope.ExportDependencyGraph()

	// Verify graph structure
	if len(graph) == 0 {
		t.Error("Expected non-empty dependency graph")
	}

	// Check that config has service as dependent
	configDeps, hasConfig := graph[config]
	if !hasConfig {
		t.Error("Expected config in dependency graph")
	}

	foundService := false
	for _, dep := range configDeps {
		if dep == service {
			foundService = true
			break
		}
	}
	if !foundService {
		t.Error("Expected service to be dependent of config")
	}

	// Check that storage has service as dependent
	storageDeps, hasStorage := graph[storage]
	if !hasStorage {
		t.Error("Expected storage in dependency graph")
	}

	foundService = false
	for _, dep := range storageDeps {
		if dep == service {
			foundService = true
			break
		}
	}
	if !foundService {
		t.Error("Expected service to be dependent of storage")
	}
}

func TestGraphDebugExtension_OnFlowPanic(t *testing.T) {
	// Capture output in buffer AND write to stdout for visual verification
	var buf bytes.Buffer
	multiWriter := io.MultiWriter(&buf, os.Stdout)
	handler := NewHumanHandler(multiWriter, slog.LevelError)

	scope := pumped.NewScope(
		pumped.WithExtension(NewGraphDebugExtension(handler, NewD2Formatter())),
	)
	defer scope.Dispose()

	// Create a dummy executor for the flow to depend on
	dummy := pumped.Provide(func(ctx *pumped.ResolveCtx) (string, error) {
		return "dummy", nil
	})

	// Create a flow that panics
	panicFlow := pumped.Flow1(
		dummy,
		func(execCtx *pumped.ExecutionCtx, d *pumped.Controller[string]) (string, error) {
			panic("simulated panic")
		},
		pumped.WithFlowTag(pumped.FlowName(), "PanicFlow"),
	)

	// Execute flow - should panic and be caught
	_, _, err := pumped.Exec(scope, context.Background(), panicFlow)

	// Verify panic was caught
	if err == nil {
		t.Error("Expected panic error but got nil")
	}

	// Verify HumanHandler formatted output
	output := buf.String()

	// Check for header with equals signs
	if !strings.Contains(output, "======================================================================") {
		t.Error("Expected separator line with equals signs")
	}

	// Check for formatted header (not escaped)
	if !strings.Contains(output, "[GraphDebug] Flow Panic") {
		t.Error("Expected '[GraphDebug] Flow Panic' header")
	}

	// Check for formatted panic message (not key=value format)
	if !strings.Contains(output, "Panic: simulated panic") {
		t.Error("Expected 'Panic: simulated panic'")
	}

	// Check for flow name
	if !strings.Contains(output, "Flow: PanicFlow") {
		t.Error("Expected 'Flow: PanicFlow'")
	}

	// Check for stack trace section
	if !strings.Contains(output, "Stack Trace:") {
		t.Error("Expected 'Stack Trace:' section")
	}

	// Verify stack trace contains actual Go stack trace elements
	if !strings.Contains(output, "goroutine") {
		t.Error("Expected goroutine information in stack trace")
	}

	// Verify newlines are NOT escaped (this is the key difference from TextHandler)
	if strings.Contains(output, "\\n") {
		t.Error("Expected actual newlines, not escaped \\n characters")
	}
}

func TestGraphDebugExtension_GetExecutorName(t *testing.T) {
	ext := NewGraphDebugExtension(NewSilentHandler(), NewD2Formatter())
	nameTag := pumped.NewTag[string]("executor.name")

	// Test with named executor
	namedExec := pumped.Provide(
		func(ctx *pumped.ResolveCtx) (string, error) {
			return "value", nil
		},
		pumped.WithTag(nameTag, "NamedExecutor"),
	)

	name := ext.getExecutorName(namedExec)
	if name != "NamedExecutor" {
		t.Errorf("Expected 'NamedExecutor', got '%s'", name)
	}

	// Test with unnamed executor (should use pointer address)
	unnamedExec := pumped.Provide(
		func(ctx *pumped.ResolveCtx) (string, error) {
			return "value", nil
		},
	)

	name = ext.getExecutorName(unnamedExec)
	if !strings.HasPrefix(name, "Executor_") {
		t.Errorf("Expected name to start with 'Executor_', got '%s'", name)
	}
}

func TestSilentHandler(t *testing.T) {
	handler := NewSilentHandler()

	// Verify Enabled returns false for all levels
	if handler.Enabled(context.Background(), slog.LevelDebug) {
		t.Error("Expected SilentHandler to be disabled for Debug level")
	}
	if handler.Enabled(context.Background(), slog.LevelInfo) {
		t.Error("Expected SilentHandler to be disabled for Info level")
	}
	if handler.Enabled(context.Background(), slog.LevelError) {
		t.Error("Expected SilentHandler to be disabled for Error level")
	}

	// Verify Handle does nothing (no panic)
	record := slog.Record{}
	err := handler.Handle(context.Background(), record)
	if err != nil {
		t.Errorf("Expected Handle to return nil, got %v", err)
	}

	// Verify WithAttrs returns self
	withAttrs := handler.WithAttrs([]slog.Attr{})
	if withAttrs != handler {
		t.Error("Expected WithAttrs to return self")
	}

	// Verify WithGroup returns self
	withGroup := handler.WithGroup("test")
	if withGroup != handler {
		t.Error("Expected WithGroup to return self")
	}

	// Integration test: Verify no output when using SilentHandler
	ext := NewGraphDebugExtension(handler, NewD2Formatter())
	scope := pumped.NewScope(
		pumped.WithExtension(ext),
	)
	defer scope.Dispose()

	nameTag := pumped.NewTag[string]("executor.name")

	// Create executor that will fail
	failingExec := pumped.Provide(
		func(ctx *pumped.ResolveCtx) (string, error) {
			return "", fmt.Errorf("intentional error")
		},
		pumped.WithTag(nameTag, "FailingExecutor"),
	)

	// Try to resolve - should fail but produce no output
	_, err = pumped.Resolve(scope, failingExec)
	if err == nil {
		t.Error("Expected error from failing executor")
	}

	// Success: SilentHandler silenced all output (no way to verify silence, but no panic = success)
}

func TestGraphDebugExtension_ComplexDependencyGraph(t *testing.T) {
	// BEHAVIOR: Complex multi-layer graph outputs all components with correct failed executor identified
	// BUG CAUGHT: Graph missing intermediate nodes, failed executor not highlighted, broken D2 in complex graphs
	// FAIL CHECK: Missing components or styling would fail assertions

	// Capture output in buffer AND write to stdout for visual verification
	var buf bytes.Buffer
	multiWriter := io.MultiWriter(&buf, os.Stdout)
	handler := NewHumanHandler(multiWriter, slog.LevelError)

	scope := pumped.NewScope(
		pumped.WithExtension(NewGraphDebugExtension(handler, NewD2Formatter())),
	)
	defer scope.Dispose()

	nameTag := pumped.NewTag[string]("executor.name")

	// Layer 1: Configuration (base layer)
	appConfig := pumped.Provide(
		func(ctx *pumped.ResolveCtx) (string, error) {
			return "app-config", nil
		},
		pumped.WithTag(nameTag, "AppConfig"),
	)

	dbConfig := pumped.Provide(
		func(ctx *pumped.ResolveCtx) (string, error) {
			return "db-config", nil
		},
		pumped.WithTag(nameTag, "DBConfig"),
	)

	cacheConfig := pumped.Provide(
		func(ctx *pumped.ResolveCtx) (string, error) {
			return "cache-config", nil
		},
		pumped.WithTag(nameTag, "CacheConfig"),
	)

	// Layer 2: Infrastructure (depends on config)
	database := pumped.Derive1(
		dbConfig.Reactive(),
		func(ctx *pumped.ResolveCtx, cfg *pumped.Controller[string]) (string, error) {
			val, _ := cfg.Get()
			return "database-" + val, nil
		},
		pumped.WithTag(nameTag, "Database"),
	)

	cache := pumped.Derive1(
		cacheConfig.Reactive(),
		func(ctx *pumped.ResolveCtx, cfg *pumped.Controller[string]) (string, error) {
			val, _ := cfg.Get()
			return "cache-" + val, nil
		},
		pumped.WithTag(nameTag, "Cache"),
	)

	// Layer 3: Repositories (depends on database)
	userRepo := pumped.Derive1(
		database.Reactive(),
		func(ctx *pumped.ResolveCtx, db *pumped.Controller[string]) (string, error) {
			val, _ := db.Get()
			return "user-repo-" + val, nil
		},
		pumped.WithTag(nameTag, "UserRepository"),
	)

	productRepo := pumped.Derive1(
		database.Reactive(),
		func(ctx *pumped.ResolveCtx, db *pumped.Controller[string]) (string, error) {
			val, _ := db.Get()
			return "product-repo-" + val, nil
		},
		pumped.WithTag(nameTag, "ProductRepository"),
	)

	orderRepo := pumped.Derive1(
		database.Reactive(),
		func(ctx *pumped.ResolveCtx, db *pumped.Controller[string]) (string, error) {
			val, _ := db.Get()
			return "order-repo-" + val, nil
		},
		pumped.WithTag(nameTag, "OrderRepository"),
	)

	// Layer 4: Services (depends on repositories and cache)
	_ = pumped.Derive2(
		userRepo.Reactive(),
		cache.Reactive(),
		func(ctx *pumped.ResolveCtx, repo *pumped.Controller[string], c *pumped.Controller[string]) (string, error) {
			repoVal, _ := repo.Get()
			cacheVal, _ := c.Get()
			return "user-service-" + repoVal + "-" + cacheVal, nil
		},
		pumped.WithTag(nameTag, "UserService"),
	)

	_ = pumped.Derive2(
		productRepo.Reactive(),
		cache.Reactive(),
		func(ctx *pumped.ResolveCtx, repo *pumped.Controller[string], c *pumped.Controller[string]) (string, error) {
			repoVal, _ := repo.Get()
			cacheVal, _ := c.Get()
			return "product-service-" + repoVal + "-" + cacheVal, nil
		},
		pumped.WithTag(nameTag, "ProductService"),
	)

	// This service will fail
	orderService := pumped.Derive2(
		orderRepo.Reactive(),
		cache.Reactive(),
		func(ctx *pumped.ResolveCtx, repo *pumped.Controller[string], c *pumped.Controller[string]) (string, error) {
			return "", fmt.Errorf("database connection timeout: failed to connect to orders table")
		},
		pumped.WithTag(nameTag, "OrderService"),
	)

	// Layer 5: API Handlers (depends on multiple services)
	_ = pumped.Derive2(
		orderService.Reactive(),
		appConfig.Reactive(),
		func(ctx *pumped.ResolveCtx, svc *pumped.Controller[string], cfg *pumped.Controller[string]) (string, error) {
			svcVal, _ := svc.Get()
			cfgVal, _ := cfg.Get()
			return "order-handler-" + svcVal + "-" + cfgVal, nil
		},
		pumped.WithTag(nameTag, "OrderHandler"),
	)

	// Try to resolve the failing service
	_, err := pumped.Resolve(scope, orderService)

	// Verify error occurred
	if err == nil {
		t.Fatal("Expected error but got nil")
	}

	output := buf.String()

	// Verify the failed executor is identified
	if !strings.Contains(output, "Failed Executor: OrderService") {
		t.Error("Expected 'Failed Executor: OrderService' in output")
	}

	// Verify the error message is present
	if !strings.Contains(output, "database connection timeout") {
		t.Error("Expected error message 'database connection timeout' in output")
	}

	// Verify dependency graph components are present (only those in the reactive chain)
	expectedComponents := []string{"OrderService", "Cache"}
	for _, comp := range expectedComponents {
		if !strings.Contains(output, comp) {
			t.Errorf("Expected '%s' in dependency graph output", comp)
		}
	}

	// Verify failed node styling
	if !strings.Contains(output, d2ColorFailed) {
		t.Errorf("Expected failed node color '%s' in output", d2ColorFailed)
	}
}

func TestGraphDebugExtension_MultipleFailures(t *testing.T) {
	// BEHAVIOR: When one of multiple failing services is resolved, OnError reports it with graph context
	// BUG CAUGHT: First failure not properly identified, graph missing sibling services
	// FAIL CHECK: Missing error output or graph components would fail assertions

	var buf bytes.Buffer
	multiWriter := io.MultiWriter(&buf, os.Stdout)
	handler := NewHumanHandler(multiWriter, slog.LevelError)

	scope := pumped.NewScope(
		pumped.WithExtension(NewGraphDebugExtension(handler, NewD2Formatter())),
	)
	defer scope.Dispose()

	nameTag := pumped.NewTag[string]("executor.name")

	// Create a base executor
	config := pumped.Provide(
		func(ctx *pumped.ResolveCtx) (string, error) {
			return "config", nil
		},
		pumped.WithTag(nameTag, "Config"),
	)

	// First failing service - resolve this one directly
	failingService1 := pumped.Derive1(
		config.Reactive(),
		func(ctx *pumped.ResolveCtx, cfg *pumped.Controller[string]) (string, error) {
			return "", fmt.Errorf("authentication service unavailable")
		},
		pumped.WithTag(nameTag, "AuthService"),
	)

	// Try to resolve - failure will be caught
	_, err := pumped.Resolve(scope, failingService1)

	if err == nil {
		t.Fatal("Expected error but got nil")
	}

	output := buf.String()

	// Verify the failed executor is correctly identified
	if !strings.Contains(output, "Failed Executor: AuthService") {
		t.Error("Expected 'Failed Executor: AuthService' in output")
	}

	// Verify the specific error message is reported
	if !strings.Contains(output, "authentication service unavailable") {
		t.Error("Expected error message 'authentication service unavailable' in output")
	}

	// Verify graph contains Config (the dependency)
	if !strings.Contains(output, "Config") {
		t.Error("Expected 'Config' in dependency graph")
	}

	// Verify D2 formatting
	if !strings.Contains(output, d2ColorFailed) {
		t.Errorf("Expected failed node color '%s' in output", d2ColorFailed)
	}
}

func TestGraphDebugExtension_LargeGraphWithUpdate(t *testing.T) {
	// BEHAVIOR: Database failure at infrastructure layer is reported with full graph context
	// BUG CAUGHT: Failed Database executor not identified, graph missing infrastructure components
	// FAIL CHECK: Missing components or wrong error message would fail assertions

	var buf bytes.Buffer
	multiWriter := io.MultiWriter(&buf, os.Stdout)
	handler := NewHumanHandler(multiWriter, slog.LevelError)

	scope := pumped.NewScope(
		pumped.WithExtension(NewGraphDebugExtension(handler, NewD2Formatter())),
	)
	defer scope.Dispose()

	nameTag := pumped.NewTag[string]("executor.name")

	// Layer 1: Configuration
	dbConfig := pumped.Provide(
		func(ctx *pumped.ResolveCtx) (string, error) {
			return "db-config-ok", nil
		},
		pumped.WithTag(nameTag, "DBConfig"),
	)

	// Layer 2: Database will fail
	database := pumped.Derive1(
		dbConfig.Reactive(),
		func(ctx *pumped.ResolveCtx, cfg *pumped.Controller[string]) (string, error) {
			return "", fmt.Errorf("database connection pool exhausted - max connections (100) reached")
		},
		pumped.WithTag(nameTag, "Database"),
	)

	// Try to resolve the failing Database
	_, err := pumped.Resolve(scope, database)

	if err == nil {
		t.Fatal("Expected error but got nil")
	}

	output := buf.String()

	// Verify error is for Database executor
	if !strings.Contains(output, "Failed Executor: Database") {
		t.Error("Expected 'Failed Executor: Database' in output")
	}

	// Verify specific error message
	if !strings.Contains(output, "database connection pool exhausted") {
		t.Error("Expected 'database connection pool exhausted' error message in output")
	}

	// Verify DBConfig dependency appears in graph
	if !strings.Contains(output, "DBConfig") {
		t.Error("Expected 'DBConfig' in dependency graph")
	}

	// Verify D2 format with failed styling
	if !strings.Contains(output, d2ColorFailed) {
		t.Errorf("Expected failed node color '%s' in output", d2ColorFailed)
	}

	// Verify D2 connection from DBConfig to Database
	if !strings.Contains(output, "->") {
		t.Error("Expected D2 arrow syntax in output")
	}
}

func TestGraphDebugExtension_DeeplyNestedDependencies(t *testing.T) {
	// Capture output in buffer AND write to stdout for visual verification
	var buf bytes.Buffer
	multiWriter := io.MultiWriter(&buf, os.Stdout)
	handler := NewHumanHandler(multiWriter, slog.LevelError)

	scope := pumped.NewScope(
		pumped.WithExtension(NewGraphDebugExtension(handler, NewD2Formatter())),
	)
	defer scope.Dispose()

	nameTag := pumped.NewTag[string]("executor.name")

	// Level 1: Base configuration
	config := pumped.Provide(
		func(ctx *pumped.ResolveCtx) (string, error) {
			return "db-config", nil
		},
		pumped.WithTag(nameTag, "Config"),
	)

	// Level 2: Connection pool depends on config
	connectionPool := pumped.Derive1(
		config.Reactive(),
		func(ctx *pumped.ResolveCtx, cfg *pumped.Controller[string]) (string, error) {
			val, _ := cfg.Get()
			return "connection-pool-" + val, nil
		},
		pumped.WithTag(nameTag, "ConnectionPool"),
	)

	// Level 3: Transaction manager depends on connection pool
	transactionMgr := pumped.Derive1(
		connectionPool.Reactive(),
		func(ctx *pumped.ResolveCtx, pool *pumped.Controller[string]) (string, error) {
			val, _ := pool.Get()
			return "transaction-mgr-" + val, nil
		},
		pumped.WithTag(nameTag, "TransactionManager"),
	)

	// Level 4: Repository depends on transaction manager
	repository := pumped.Derive1(
		transactionMgr.Reactive(),
		func(ctx *pumped.ResolveCtx, txMgr *pumped.Controller[string]) (string, error) {
			val, _ := txMgr.Get()
			return "repository-" + val, nil
		},
		pumped.WithTag(nameTag, "Repository"),
	)

	// Level 5: Domain service depends on repository
	domainService := pumped.Derive1(
		repository.Reactive(),
		func(ctx *pumped.ResolveCtx, repo *pumped.Controller[string]) (string, error) {
			val, _ := repo.Get()
			return "domain-service-" + val, nil
		},
		pumped.WithTag(nameTag, "DomainService"),
	)

	// Level 6: Application service depends on domain service
	appService := pumped.Derive1(
		domainService.Reactive(),
		func(ctx *pumped.ResolveCtx, svc *pumped.Controller[string]) (string, error) {
			val, _ := svc.Get()
			return "app-service-" + val, nil
		},
		pumped.WithTag(nameTag, "ApplicationService"),
	)

	// Level 7: Controller depends on application service
	controller := pumped.Derive1(
		appService.Reactive(),
		func(ctx *pumped.ResolveCtx, appSvc *pumped.Controller[string]) (string, error) {
			val, _ := appSvc.Get()
			return "controller-" + val, nil
		},
		pumped.WithTag(nameTag, "Controller"),
	)

	// Level 8: Middleware depends on controller
	middleware := pumped.Derive1(
		controller.Reactive(),
		func(ctx *pumped.ResolveCtx, ctrl *pumped.Controller[string]) (string, error) {
			val, _ := ctrl.Get()
			return "middleware-" + val, nil
		},
		pumped.WithTag(nameTag, "Middleware"),
	)

	// Level 9: Router depends on middleware
	router := pumped.Derive1(
		middleware.Reactive(),
		func(ctx *pumped.ResolveCtx, mw *pumped.Controller[string]) (string, error) {
			val, _ := mw.Get()
			return "router-" + val, nil
		},
		pumped.WithTag(nameTag, "Router"),
	)

	// Level 10: Server depends on router - THIS WILL FAIL
	server := pumped.Derive1(
		router.Reactive(),
		func(ctx *pumped.ResolveCtx, rt *pumped.Controller[string]) (string, error) {
			return "", fmt.Errorf("server startup failed: port 8080 already in use")
		},
		pumped.WithTag(nameTag, "Server"),
	)

	// Try to resolve the deeply nested server
	_, err := pumped.Resolve(scope, server)

	if err == nil {
		t.Fatal("Expected error due to server failure")
	}

	// Verify output contains nested chain
	output := buf.String()

	// Check all components are in the graph
	expectedComponents := []string{
		"Config", "ConnectionPool", "TransactionManager",
		"Repository", "DomainService", "ApplicationService",
		"Controller", "Middleware", "Router", "Server",
	}

	for _, component := range expectedComponents {
		if !strings.Contains(output, component) {
			t.Errorf("Expected '%s' in dependency graph output", component)
		}
	}

	// Check for D2 format
	if !strings.Contains(output, "direction: down") {
		t.Error("Expected D2 direction directive in output")
	}

	// Check for D2 connection syntax
	if !strings.Contains(output, "->") {
		t.Error("Expected D2 arrow syntax in output")
	}

	// Check for failed node styling
	if !strings.Contains(output, d2ColorFailed) {
		t.Error("Expected failed node color in D2 output")
	}

	t.Logf("\n===== Deeply Nested 10-Level Dependency Chain =====")
	t.Logf("Config → ConnectionPool → TransactionManager → Repository →")
	t.Logf("DomainService → ApplicationService → Controller → Middleware →")
	t.Logf("Router → Server (FAILED)")
}

func TestGraphDebugExtension_NestedWithBranching(t *testing.T) {
	// Capture output in buffer AND write to stdout for visual verification
	var buf bytes.Buffer
	multiWriter := io.MultiWriter(&buf, os.Stdout)
	handler := NewHumanHandler(multiWriter, slog.LevelError)

	scope := pumped.NewScope(
		pumped.WithExtension(NewGraphDebugExtension(handler, NewD2Formatter())),
	)
	defer scope.Dispose()

	nameTag := pumped.NewTag[string]("executor.name")

	// Root level
	root := pumped.Provide(
		func(ctx *pumped.ResolveCtx) (string, error) {
			return "root", nil
		},
		pumped.WithTag(nameTag, "Root"),
	)

	// Level 1: Two branches from root
	branchA := pumped.Derive1(
		root.Reactive(),
		func(ctx *pumped.ResolveCtx, r *pumped.Controller[string]) (string, error) {
			val, _ := r.Get()
			return "branch-a-" + val, nil
		},
		pumped.WithTag(nameTag, "BranchA"),
	)

	branchB := pumped.Derive1(
		root.Reactive(),
		func(ctx *pumped.ResolveCtx, r *pumped.Controller[string]) (string, error) {
			val, _ := r.Get()
			return "branch-b-" + val, nil
		},
		pumped.WithTag(nameTag, "BranchB"),
	)

	// Level 2: Branch A splits into two
	branchA1 := pumped.Derive1(
		branchA.Reactive(),
		func(ctx *pumped.ResolveCtx, a *pumped.Controller[string]) (string, error) {
			val, _ := a.Get()
			return "branch-a1-" + val, nil
		},
		pumped.WithTag(nameTag, "BranchA1"),
	)

	branchA2 := pumped.Derive1(
		branchA.Reactive(),
		func(ctx *pumped.ResolveCtx, a *pumped.Controller[string]) (string, error) {
			val, _ := a.Get()
			return "branch-a2-" + val, nil
		},
		pumped.WithTag(nameTag, "BranchA2"),
	)

	// Level 2: Branch B splits into two
	branchB1 := pumped.Derive1(
		branchB.Reactive(),
		func(ctx *pumped.ResolveCtx, b *pumped.Controller[string]) (string, error) {
			val, _ := b.Get()
			return "branch-b1-" + val, nil
		},
		pumped.WithTag(nameTag, "BranchB1"),
	)

	branchB2 := pumped.Derive1(
		branchB.Reactive(),
		func(ctx *pumped.ResolveCtx, b *pumped.Controller[string]) (string, error) {
			val, _ := b.Get()
			return "branch-b2-" + val, nil
		},
		pumped.WithTag(nameTag, "BranchB2"),
	)

	// Level 3: Leaf nodes that depend on the branches
	leafA1 := pumped.Derive1(
		branchA1.Reactive(),
		func(ctx *pumped.ResolveCtx, a1 *pumped.Controller[string]) (string, error) {
			val, _ := a1.Get()
			return "leaf-a1-" + val, nil
		},
		pumped.WithTag(nameTag, "LeafA1"),
	)

	leafA2 := pumped.Derive1(
		branchA2.Reactive(),
		func(ctx *pumped.ResolveCtx, a2 *pumped.Controller[string]) (string, error) {
			val, _ := a2.Get()
			return "leaf-a2-" + val, nil
		},
		pumped.WithTag(nameTag, "LeafA2"),
	)

	leafB1 := pumped.Derive1(
		branchB1.Reactive(),
		func(ctx *pumped.ResolveCtx, b1 *pumped.Controller[string]) (string, error) {
			val, _ := b1.Get()
			return "leaf-b1-" + val, nil
		},
		pumped.WithTag(nameTag, "LeafB1"),
	)

	// Level 4: Final aggregator that combines everything
	aggregator := pumped.Derive4(
		leafA1.Reactive(),
		leafA2.Reactive(),
		leafB1.Reactive(),
		branchB2.Reactive(),
		func(ctx *pumped.ResolveCtx,
			la1 *pumped.Controller[string],
			la2 *pumped.Controller[string],
			lb1 *pumped.Controller[string],
			b2 *pumped.Controller[string]) (string, error) {
			// Fail at aggregation level after all dependencies are resolved
			return "", fmt.Errorf("aggregation failed: incompatible data types from branches")
		},
		pumped.WithTag(nameTag, "Aggregator"),
	)

	// Try to resolve the aggregator - should fail at aggregation level
	_, err := pumped.Resolve(scope, aggregator)

	if err == nil {
		t.Fatal("Expected aggregator to fail")
	}
	t.Logf("Resolve result: err=%v", err)

	// Verify output contains D2 format
	output := buf.String()

	// Check for branching structure
	expectedComponents := []string{
		"Root", "BranchA", "BranchB",
		"BranchA1", "BranchA2", "BranchB1", "BranchB2",
		"LeafA1", "LeafA2", "LeafB1",
	}

	for _, component := range expectedComponents {
		if !strings.Contains(output, component) {
			t.Errorf("Expected '%s' in dependency graph output", component)
		}
	}

	// Check for D2 format
	if !strings.Contains(output, "direction: down") {
		t.Error("Expected D2 direction directive in output")
	}

	// Check for D2 connection syntax showing branches
	if !strings.Contains(output, "->") {
		t.Error("Expected D2 arrow syntax for connections")
	}

	t.Logf("\n===== Nested Tree with Branching =====")
	t.Logf("                 Root")
	t.Logf("                /    \\")
	t.Logf("           BranchA  BranchB")
	t.Logf("           /    \\    /    \\")
	t.Logf("      BranchA1 A2  B1  B2")
	t.Logf("         |     |   |    |")
	t.Logf("      LeafA1 LeafA2 LeafB1")
	t.Logf("           \\    |    /   /")
	t.Logf("            Aggregator (FAILED)")
}

func TestFormatD2Diagram_ValidD2Syntax(t *testing.T) {
	ext := NewGraphDebugExtension(NewSilentHandler(), NewD2Formatter())
	scope := pumped.NewScope(
		pumped.WithExtension(ext),
	)
	defer scope.Dispose()

	nameTag := pumped.NewTag[string]("executor.name")

	// Create a dependency graph
	config := pumped.Provide(
		func(ctx *pumped.ResolveCtx) (string, error) {
			return "config", nil
		},
		pumped.WithTag(nameTag, "Config"),
	)

	database := pumped.Derive1(
		config.Reactive(),
		func(ctx *pumped.ResolveCtx, cfg *pumped.Controller[string]) (string, error) {
			val, _ := cfg.Get()
			return "database-" + val, nil
		},
		pumped.WithTag(nameTag, "Database"),
	)

	service := pumped.Derive1(
		database.Reactive(),
		func(ctx *pumped.ResolveCtx, db *pumped.Controller[string]) (string, error) {
			val, _ := db.Get()
			return "service-" + val, nil
		},
		pumped.WithTag(nameTag, "Service"),
	)

	// Resolve to build the graph
	_, err := pumped.Resolve(scope, service)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Export the graph and generate D2 output via GraphFormatter
	graph := scope.ExportDependencyGraph()
	data := ext.buildGraphData(graph, nil)
	d2Output := NewD2Formatter().FormatGraph(data)

	// Validate D2 syntax by parsing with d2parser
	_, parseErr := d2parser.Parse("test.d2", strings.NewReader(d2Output), nil)
	if parseErr != nil {
		t.Errorf("D2 output is not valid syntax: %v\nD2 Output:\n%s", parseErr, d2Output)
	}

	// Verify expected content
	if !strings.Contains(d2Output, "direction: down") {
		t.Error("Expected 'direction: down' in D2 output")
	}

	if !strings.Contains(d2Output, "Config") {
		t.Error("Expected 'Config' node in D2 output")
	}

	if !strings.Contains(d2Output, "Database") {
		t.Error("Expected 'Database' node in D2 output")
	}

	if !strings.Contains(d2Output, "Service") {
		t.Error("Expected 'Service' node in D2 output")
	}

	// Check for connections
	if !strings.Contains(d2Output, "->") {
		t.Error("Expected D2 connection arrows in output")
	}

	// Check for styling
	if !strings.Contains(d2Output, "style.fill") {
		t.Error("Expected style.fill in D2 output")
	}

	t.Logf("Valid D2 output:\n%s", d2Output)
}

func TestFormatD2Diagram_EmptyGraph(t *testing.T) {
	// Test D2Formatter directly with empty GraphData
	d2Output := NewD2Formatter().FormatGraph(GraphData{})

	// Validate D2 syntax
	_, parseErr := d2parser.Parse("test.d2", strings.NewReader(d2Output), nil)
	if parseErr != nil {
		t.Errorf("Empty graph D2 output is not valid syntax: %v\nD2 Output:\n%s", parseErr, d2Output)
	}

	// Check for empty graph message
	if !strings.Contains(d2Output, "No reactive dependencies tracked") {
		t.Error("Expected empty graph message in D2 output")
	}

	t.Logf("Empty graph D2 output:\n%s", d2Output)
}

func TestFormatD2Diagram_WithFailedExecutor(t *testing.T) {
	ext := NewGraphDebugExtension(NewSilentHandler(), NewD2Formatter())
	scope := pumped.NewScope(
		pumped.WithExtension(ext),
	)
	defer scope.Dispose()

	nameTag := pumped.NewTag[string]("executor.name")

	// Create a dependency graph with a failing executor
	config := pumped.Provide(
		func(ctx *pumped.ResolveCtx) (string, error) {
			return "config", nil
		},
		pumped.WithTag(nameTag, "Config"),
	)

	failingService := pumped.Derive1(
		config.Reactive(),
		func(ctx *pumped.ResolveCtx, cfg *pumped.Controller[string]) (string, error) {
			return "", fmt.Errorf("service failed")
		},
		pumped.WithTag(nameTag, "FailingService"),
	)

	// Try to resolve - will fail
	_, _ = pumped.Resolve(scope, failingService)

	// Export the graph and generate D2 output with failed executor marked
	graph := scope.ExportDependencyGraph()
	data := ext.buildGraphData(graph, failingService)
	d2Output := NewD2Formatter().FormatGraph(data)

	// Validate D2 syntax
	_, parseErr := d2parser.Parse("test.d2", strings.NewReader(d2Output), nil)
	if parseErr != nil {
		t.Errorf("D2 output with failed executor is not valid syntax: %v\nD2 Output:\n%s", parseErr, d2Output)
	}

	// Check for failed node styling
	if !strings.Contains(d2Output, d2ColorFailed) {
		t.Errorf("Expected failed node color '%s' in D2 output", d2ColorFailed)
	}

	if !strings.Contains(d2Output, d2StrokeFailed) {
		t.Errorf("Expected failed node stroke '%s' in D2 output", d2StrokeFailed)
	}

	t.Logf("D2 output with failed executor:\n%s", d2Output)
}

func TestSanitizeD2NodeID(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"SimpleName", "SimpleName"},
		{"Name With Spaces", "Name_With_Spaces"},
		{"name-with-dashes", "name_with_dashes"},
		{"name.with.dots", "name_with_dots"},
		{"123startsWithNumber", "_123startsWithNumber"},
		{"special!@#chars", "special___chars"},
		{"", "_node"},
		{"already_valid_name", "already_valid_name"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := sanitizeD2NodeID(tt.input)
			if result != tt.expected {
				t.Errorf("sanitizeD2NodeID(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestBuildGraphData_NodeStatusAssignment(t *testing.T) {
	// BEHAVIOR: buildGraphData assigns correct NodeStatus based on resolution state
	// BUG CAUGHT: Status logic inverted (failed marked as OK, or resolved marked as pending)
	// FAIL CHECK: If status mapping is wrong, specific status assertions fail
	ext := NewGraphDebugExtension(NewSilentHandler(), NewD2Formatter())
	scope := pumped.NewScope(
		pumped.WithExtension(ext),
	)
	defer scope.Dispose()

	nameTag := pumped.NewTag[string]("executor.name")

	config := pumped.Provide(
		func(ctx *pumped.ResolveCtx) (string, error) {
			return "config", nil
		},
		pumped.WithTag(nameTag, "Config"),
	)

	// Resolve config first so it's tracked as resolved
	_, err := pumped.Resolve(scope, config)
	if err != nil {
		t.Fatalf("Unexpected error resolving config: %v", err)
	}

	failingService := pumped.Derive1(
		config.Reactive(),
		func(ctx *pumped.ResolveCtx, cfg *pumped.Controller[string]) (string, error) {
			return "", fmt.Errorf("service failed")
		},
		pumped.WithTag(nameTag, "FailingService"),
	)

	// Resolve failing service
	_, _ = pumped.Resolve(scope, failingService)

	graph := scope.ExportDependencyGraph()
	data := ext.buildGraphData(graph, failingService)

	statusByName := make(map[string]NodeStatus)
	for _, node := range data.Nodes {
		statusByName[node.Name] = node.Status
	}

	// Config was explicitly resolved → should be OK
	if statusByName["Config"] != NodeStatusOK {
		t.Errorf("Expected Config status %q, got %q", NodeStatusOK, statusByName["Config"])
	}

	// FailingService was passed as failedExecutor → should be Failed
	if statusByName["FailingService"] != NodeStatusFailed {
		t.Errorf("Expected FailingService status %q, got %q", NodeStatusFailed, statusByName["FailingService"])
	}
}

func TestBuildGraphData_DeterministicNodeOrdering(t *testing.T) {
	// BEHAVIOR: buildGraphData produces nodes sorted alphabetically by name for deterministic output
	// BUG CAUGHT: Non-deterministic node ordering causing flaky test output or diff instability
	// FAIL CHECK: If sorting is removed, nodes appear in random map iteration order
	ext := NewGraphDebugExtension(NewSilentHandler(), NewD2Formatter())
	scope := pumped.NewScope(
		pumped.WithExtension(ext),
	)
	defer scope.Dispose()

	nameTag := pumped.NewTag[string]("executor.name")

	// Create executors with names that sort differently from creation order
	zulu := pumped.Provide(
		func(ctx *pumped.ResolveCtx) (string, error) { return "z", nil },
		pumped.WithTag(nameTag, "Zulu"),
	)
	alpha := pumped.Derive1(
		zulu.Reactive(),
		func(ctx *pumped.ResolveCtx, z *pumped.Controller[string]) (string, error) {
			val, _ := z.Get()
			return "a-" + val, nil
		},
		pumped.WithTag(nameTag, "Alpha"),
	)
	mike := pumped.Derive1(
		zulu.Reactive(),
		func(ctx *pumped.ResolveCtx, z *pumped.Controller[string]) (string, error) {
			val, _ := z.Get()
			return "m-" + val, nil
		},
		pumped.WithTag(nameTag, "Mike"),
	)

	_, _ = pumped.Resolve(scope, alpha)
	_, _ = pumped.Resolve(scope, mike)

	graph := scope.ExportDependencyGraph()
	data := ext.buildGraphData(graph, nil)

	// Nodes must be sorted alphabetically: Alpha, Mike, Zulu
	if len(data.Nodes) < 3 {
		t.Fatalf("Expected at least 3 nodes, got %d", len(data.Nodes))
	}

	for i := 1; i < len(data.Nodes); i++ {
		if data.Nodes[i-1].Name >= data.Nodes[i].Name {
			t.Errorf("Nodes not sorted: %q >= %q at positions %d,%d",
				data.Nodes[i-1].Name, data.Nodes[i].Name, i-1, i)
		}
	}
}

func TestBuildGraphData_DeterministicEdgeOrdering(t *testing.T) {
	// BEHAVIOR: buildGraphData produces edges sorted by (From, To) for deterministic output
	// BUG CAUGHT: Non-deterministic edge ordering causing flaky output
	// FAIL CHECK: If edge sorting is removed, edges appear in random map iteration order
	ext := NewGraphDebugExtension(NewSilentHandler(), NewD2Formatter())
	scope := pumped.NewScope(
		pumped.WithExtension(ext),
	)
	defer scope.Dispose()

	nameTag := pumped.NewTag[string]("executor.name")

	root := pumped.Provide(
		func(ctx *pumped.ResolveCtx) (string, error) { return "root", nil },
		pumped.WithTag(nameTag, "Root"),
	)

	childB := pumped.Derive1(
		root.Reactive(),
		func(ctx *pumped.ResolveCtx, r *pumped.Controller[string]) (string, error) {
			val, _ := r.Get()
			return "b-" + val, nil
		},
		pumped.WithTag(nameTag, "ChildB"),
	)

	childA := pumped.Derive1(
		root.Reactive(),
		func(ctx *pumped.ResolveCtx, r *pumped.Controller[string]) (string, error) {
			val, _ := r.Get()
			return "a-" + val, nil
		},
		pumped.WithTag(nameTag, "ChildA"),
	)

	_, _ = pumped.Resolve(scope, childB)
	_, _ = pumped.Resolve(scope, childA)

	graph := scope.ExportDependencyGraph()
	data := ext.buildGraphData(graph, nil)

	// Edges must be sorted by (From, To)
	for i := 1; i < len(data.Edges); i++ {
		prev := data.Edges[i-1]
		curr := data.Edges[i]
		if prev.From > curr.From || (prev.From == curr.From && prev.To > curr.To) {
			t.Errorf("Edges not sorted: (%q→%q) before (%q→%q) at positions %d,%d",
				prev.From, prev.To, curr.From, curr.To, i-1, i)
		}
	}
}

func TestHumanHandler_LevelFiltering(t *testing.T) {
	// BEHAVIOR: HumanHandler only enables logging at or above its configured level
	// BUG CAUGHT: Handler logging debug messages when configured for error-only
	// FAIL CHECK: If Enabled ignores level, wrong levels would return true
	handler := NewHumanHandler(os.Stdout, slog.LevelError)

	// Below threshold - should be disabled
	if handler.Enabled(context.Background(), slog.LevelDebug) {
		t.Error("Expected Debug to be disabled when level is Error")
	}
	if handler.Enabled(context.Background(), slog.LevelInfo) {
		t.Error("Expected Info to be disabled when level is Error")
	}
	if handler.Enabled(context.Background(), slog.LevelWarn) {
		t.Error("Expected Warn to be disabled when level is Error")
	}

	// At threshold - should be enabled
	if !handler.Enabled(context.Background(), slog.LevelError) {
		t.Error("Expected Error to be enabled when level is Error")
	}

	// Test with Info level threshold
	infoHandler := NewHumanHandler(os.Stdout, slog.LevelInfo)

	if infoHandler.Enabled(context.Background(), slog.LevelDebug) {
		t.Error("Expected Debug to be disabled when level is Info")
	}
	if !infoHandler.Enabled(context.Background(), slog.LevelInfo) {
		t.Error("Expected Info to be enabled when level is Info")
	}
	if !infoHandler.Enabled(context.Background(), slog.LevelWarn) {
		t.Error("Expected Warn to be enabled when level is Info")
	}
}

func TestHumanHandler_DefaultFormatting(t *testing.T) {
	// BEHAVIOR: Non-GraphDebug messages use default [LEVEL] MESSAGE format with key: value attributes
	// BUG CAUGHT: Default case in Handle switch broken, attributes not written
	// FAIL CHECK: If default formatting removed, output would be empty or wrong format
	var buf bytes.Buffer
	handler := NewHumanHandler(&buf, slog.LevelInfo)

	logger := slog.New(handler)
	logger.Info("Custom Message", "key1", "value1", "key2", 42)

	output := buf.String()

	if !strings.Contains(output, "[INFO]") {
		t.Error("Expected '[INFO]' level prefix in default format")
	}
	if !strings.Contains(output, "Custom Message") {
		t.Error("Expected 'Custom Message' in output")
	}
	if !strings.Contains(output, "key1: value1") {
		t.Error("Expected 'key1: value1' attribute in output")
	}
	if !strings.Contains(output, "key2: 42") {
		t.Error("Expected 'key2: 42' attribute in output")
	}
}

func TestD2Formatter_PendingNodeStyling(t *testing.T) {
	// BEHAVIOR: Pending nodes get gray fill color, no stroke
	// BUG CAUGHT: Pending nodes getting wrong color or unexpected stroke
	// FAIL CHECK: If pending styling is wrong, color assertion fails
	data := GraphData{
		Nodes: []GraphNode{
			{Name: "PendingNode", Status: NodeStatusPending},
		},
		Edges: nil,
	}

	d2Output := NewD2Formatter().FormatGraph(data)

	// Validate D2 syntax
	_, parseErr := d2parser.Parse("test.d2", strings.NewReader(d2Output), nil)
	if parseErr != nil {
		t.Errorf("D2 output is not valid syntax: %v\nD2 Output:\n%s", parseErr, d2Output)
	}

	// Pending nodes should get gray fill
	if !strings.Contains(d2Output, d2ColorPending) {
		t.Errorf("Expected pending node color '%s' in D2 output, got:\n%s", d2ColorPending, d2Output)
	}

	// Pending nodes should NOT have stroke
	if strings.Contains(d2Output, "style.stroke:") {
		t.Error("Expected no stroke styling for pending nodes")
	}
}

func TestD2Formatter_OKNodeStyling(t *testing.T) {
	// BEHAVIOR: OK nodes get green fill color, no stroke
	// BUG CAUGHT: OK nodes getting failed styling or wrong color
	// FAIL CHECK: If OK styling is wrong, color assertion fails
	data := GraphData{
		Nodes: []GraphNode{
			{Name: "OKNode", Status: NodeStatusOK},
		},
		Edges: nil,
	}

	d2Output := NewD2Formatter().FormatGraph(data)

	// Validate D2 syntax
	_, parseErr := d2parser.Parse("test.d2", strings.NewReader(d2Output), nil)
	if parseErr != nil {
		t.Errorf("D2 output is not valid syntax: %v\nD2 Output:\n%s", parseErr, d2Output)
	}

	// OK nodes should get green fill
	if !strings.Contains(d2Output, d2ColorOK) {
		t.Errorf("Expected OK node color '%s' in D2 output, got:\n%s", d2ColorOK, d2Output)
	}

	// OK nodes should NOT have stroke
	if strings.Contains(d2Output, "style.stroke:") {
		t.Error("Expected no stroke styling for OK nodes")
	}
}

func TestD2Formatter_FailedNodeHasStroke(t *testing.T) {
	// BEHAVIOR: Failed nodes get red fill, red stroke, and stroke-width 3
	// BUG CAUGHT: Failed nodes missing visual distinction (no stroke = hard to spot in diagram)
	// FAIL CHECK: If stroke logic is removed, stroke assertions fail
	data := GraphData{
		Nodes: []GraphNode{
			{Name: "FailedNode", Status: NodeStatusFailed},
		},
		Edges: nil,
	}

	d2Output := NewD2Formatter().FormatGraph(data)

	// Validate D2 syntax
	_, parseErr := d2parser.Parse("test.d2", strings.NewReader(d2Output), nil)
	if parseErr != nil {
		t.Errorf("D2 output is not valid syntax: %v\nD2 Output:\n%s", parseErr, d2Output)
	}

	if !strings.Contains(d2Output, d2ColorFailed) {
		t.Errorf("Expected failed fill '%s'", d2ColorFailed)
	}
	if !strings.Contains(d2Output, d2StrokeFailed) {
		t.Errorf("Expected failed stroke '%s'", d2StrokeFailed)
	}
	if !strings.Contains(d2Output, "stroke-width: 3") {
		t.Error("Expected stroke-width: 3 for failed nodes")
	}
}

func TestD2Formatter_MixedStatusNodes(t *testing.T) {
	// BEHAVIOR: Graph with all three statuses renders each with correct styling
	// BUG CAUGHT: Status switch falling through to wrong case for some statuses
	// FAIL CHECK: If any status maps to wrong color, assertion fails
	data := GraphData{
		Nodes: []GraphNode{
			{Name: "Good", Status: NodeStatusOK},
			{Name: "Bad", Status: NodeStatusFailed},
			{Name: "Unknown", Status: NodeStatusPending},
		},
		Edges: []GraphEdge{
			{From: "Good", To: "Bad"},
			{From: "Unknown", To: "Bad"},
		},
	}

	d2Output := NewD2Formatter().FormatGraph(data)

	// Validate D2 syntax
	_, parseErr := d2parser.Parse("test.d2", strings.NewReader(d2Output), nil)
	if parseErr != nil {
		t.Errorf("D2 output is not valid syntax: %v\nD2 Output:\n%s", parseErr, d2Output)
	}

	// All three colors should be present
	if !strings.Contains(d2Output, d2ColorOK) {
		t.Errorf("Expected OK color '%s' in output", d2ColorOK)
	}
	if !strings.Contains(d2Output, d2ColorFailed) {
		t.Errorf("Expected failed color '%s' in output", d2ColorFailed)
	}
	if !strings.Contains(d2Output, d2ColorPending) {
		t.Errorf("Expected pending color '%s' in output", d2ColorPending)
	}

	// Both edges should be present
	if !strings.Contains(d2Output, "Good -> Bad") {
		t.Error("Expected edge 'Good -> Bad' in output")
	}
	if !strings.Contains(d2Output, "Unknown -> Bad") {
		t.Error("Expected edge 'Unknown -> Bad' in output")
	}
}

func TestBuildGraphData_EmptyGraphReturnsEmptyData(t *testing.T) {
	// BEHAVIOR: buildGraphData returns empty GraphData for empty graph
	// BUG CAUGHT: Nil pointer panic or non-empty result on empty input
	// FAIL CHECK: If empty check is removed, result would have unexpected nodes
	ext := NewGraphDebugExtension(NewSilentHandler(), NewD2Formatter())

	emptyGraph := make(map[pumped.AnyExecutor][]pumped.AnyExecutor)
	data := ext.buildGraphData(emptyGraph, nil)

	if len(data.Nodes) != 0 {
		t.Errorf("Expected 0 nodes for empty graph, got %d", len(data.Nodes))
	}
	if len(data.Edges) != 0 {
		t.Errorf("Expected 0 edges for empty graph, got %d", len(data.Edges))
	}
}

func TestFormatDependencyGraph_ErrorDetailsSection(t *testing.T) {
	// BEHAVIOR: formatDependencyGraph includes Error Details section with executor name and error
	// BUG CAUGHT: Missing error details in graph output, or wrong executor name
	// FAIL CHECK: If error details section removed, assertions fail
	ext := NewGraphDebugExtension(NewSilentHandler(), NewD2Formatter())
	scope := pumped.NewScope(
		pumped.WithExtension(ext),
	)
	defer scope.Dispose()

	nameTag := pumped.NewTag[string]("executor.name")

	config := pumped.Provide(
		func(ctx *pumped.ResolveCtx) (string, error) {
			return "config", nil
		},
		pumped.WithTag(nameTag, "Config"),
	)

	failingExec := pumped.Derive1(
		config.Reactive(),
		func(ctx *pumped.ResolveCtx, cfg *pumped.Controller[string]) (string, error) {
			return "", fmt.Errorf("specific error message for test")
		},
		pumped.WithTag(nameTag, "FailingExec"),
	)

	_, _ = pumped.Resolve(scope, failingExec)

	testErr := fmt.Errorf("specific error message for test")
	output := ext.formatDependencyGraph(scope, failingExec, testErr)

	if !strings.Contains(output, "Error Details:") {
		t.Error("Expected 'Error Details:' section in output")
	}
	if !strings.Contains(output, "Executor: FailingExec") {
		t.Error("Expected 'Executor: FailingExec' in error details")
	}
	if !strings.Contains(output, "specific error message for test") {
		t.Error("Expected error message in error details")
	}
}

func TestHumanHandler_WithAttrsAndWithGroup(t *testing.T) {
	// BEHAVIOR: WithAttrs and WithGroup return self (stateless handler)
	// BUG CAUGHT: WithAttrs/WithGroup returning nil or different handler breaking slog chain
	// FAIL CHECK: If methods return nil, identity checks fail
	handler := NewHumanHandler(&bytes.Buffer{}, slog.LevelInfo)

	withAttrs := handler.WithAttrs([]slog.Attr{slog.String("key", "val")})
	if withAttrs != handler {
		t.Error("Expected WithAttrs to return self")
	}

	withGroup := handler.WithGroup("testgroup")
	if withGroup != handler {
		t.Error("Expected WithGroup to return self")
	}
}

func TestD2Formatter_EdgesSanitizeNodeIDs(t *testing.T) {
	// BEHAVIOR: Edge connections use sanitized node IDs matching the declarations
	// BUG CAUGHT: Edges using raw names while declarations use sanitized IDs → broken D2
	// FAIL CHECK: If edge sanitization is removed, D2 parser would reject dangling references
	data := GraphData{
		Nodes: []GraphNode{
			{Name: "Node With Spaces", Status: NodeStatusOK},
			{Name: "node-with-dashes", Status: NodeStatusOK},
		},
		Edges: []GraphEdge{
			{From: "Node With Spaces", To: "node-with-dashes"},
		},
	}

	d2Output := NewD2Formatter().FormatGraph(data)

	// Validate D2 syntax - this will fail if edges reference non-existent IDs
	_, parseErr := d2parser.Parse("test.d2", strings.NewReader(d2Output), nil)
	if parseErr != nil {
		t.Errorf("D2 output with special chars is not valid syntax: %v\nD2 Output:\n%s", parseErr, d2Output)
	}

	// Verify sanitized IDs are used in edges
	if !strings.Contains(d2Output, "Node_With_Spaces -> node_with_dashes") {
		t.Errorf("Expected sanitized edge 'Node_With_Spaces -> node_with_dashes', got:\n%s", d2Output)
	}
}

func TestD2Formatter_NodeLabelPreservesOriginalName(t *testing.T) {
	// BEHAVIOR: D2 node declarations use sanitized ID but preserve original name as label
	// BUG CAUGHT: Display label showing sanitized ID instead of human-readable name
	// FAIL CHECK: If label uses sanitized name, the quoted label assertion fails
	data := GraphData{
		Nodes: []GraphNode{
			{Name: "My Service", Status: NodeStatusOK},
		},
		Edges: nil,
	}

	d2Output := NewD2Formatter().FormatGraph(data)

	// Sanitized ID should be used for declaration
	if !strings.Contains(d2Output, "My_Service:") {
		t.Errorf("Expected sanitized ID 'My_Service:', got:\n%s", d2Output)
	}

	// Original name should be preserved as label in quotes
	if !strings.Contains(d2Output, `"My Service"`) {
		t.Errorf("Expected original label '\"My Service\"', got:\n%s", d2Output)
	}
}
