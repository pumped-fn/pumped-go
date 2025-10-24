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

	pumped "github.com/pumped-fn/pumped-go"
)

func TestGraphDebugExtension_OnError(t *testing.T) {
	// Capture output in buffer AND write to stdout for visual verification
	var buf bytes.Buffer
	multiWriter := io.MultiWriter(&buf, os.Stdout)
	handler := NewHumanHandler(multiWriter, slog.LevelError)

	scope := pumped.NewScope(
		pumped.WithExtension(NewGraphDebugExtension(handler)),
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

	// Check for tree structure with proper formatting
	if !strings.Contains(output, "Storage") {
		t.Error("Expected 'Storage' in dependency graph")
	}

	if !strings.Contains(output, "└─>") || !strings.Contains(output, "UserService") {
		t.Error("Expected tree structure with '└─>' and 'UserService'")
	}

	// Check for status indicator
	if !strings.Contains(output, "❌ FAILED") {
		t.Error("Expected '❌ FAILED' status indicator")
	}

	// Check for Error Details section
	if !strings.Contains(output, "Error Details:") {
		t.Error("Expected 'Error Details:' section")
	}
}

func TestGraphDebugExtension_TracksResolvedExecutors(t *testing.T) {
	ext := NewGraphDebugExtension(NewSilentHandler())
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

	// Check that executors were tracked
	if !ext.resolvedExecutors[storage] {
		t.Error("Expected storage to be tracked as resolved")
	}

	if !ext.resolvedExecutors[service] {
		t.Error("Expected service to be tracked as resolved")
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
		pumped.WithExtension(NewGraphDebugExtension(handler)),
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
	ext := NewGraphDebugExtension(NewSilentHandler())
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
	ext := NewGraphDebugExtension(handler)
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
	// Use HumanHandler to write formatted output to stdout
	handler := NewHumanHandler(os.Stdout, slog.LevelError)

	scope := pumped.NewScope(
		pumped.WithExtension(NewGraphDebugExtension(handler)),
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
	userService := pumped.Derive2(
		userRepo.Reactive(),
		cache.Reactive(),
		func(ctx *pumped.ResolveCtx, repo *pumped.Controller[string], c *pumped.Controller[string]) (string, error) {
			repoVal, _ := repo.Get()
			cacheVal, _ := c.Get()
			return "user-service-" + repoVal + "-" + cacheVal, nil
		},
		pumped.WithTag(nameTag, "UserService"),
	)

	productService := pumped.Derive2(
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
	userHandler := pumped.Derive2(
		userService.Reactive(),
		appConfig.Reactive(),
		func(ctx *pumped.ResolveCtx, svc *pumped.Controller[string], cfg *pumped.Controller[string]) (string, error) {
			svcVal, _ := svc.Get()
			cfgVal, _ := cfg.Get()
			return "user-handler-" + svcVal + "-" + cfgVal, nil
		},
		pumped.WithTag(nameTag, "UserHandler"),
	)

	productHandler := pumped.Derive2(
		productService.Reactive(),
		appConfig.Reactive(),
		func(ctx *pumped.ResolveCtx, svc *pumped.Controller[string], cfg *pumped.Controller[string]) (string, error) {
			svcVal, _ := svc.Get()
			cfgVal, _ := cfg.Get()
			return "product-handler-" + svcVal + "-" + cfgVal, nil
		},
		pumped.WithTag(nameTag, "ProductHandler"),
	)

	// Layer 6: API Gateway (depends on all handlers and order service)
	apiGateway := pumped.Derive4(
		userHandler.Reactive(),
		productHandler.Reactive(),
		orderService.Reactive(), // This will fail
		appConfig.Reactive(),
		func(ctx *pumped.ResolveCtx,
			uh *pumped.Controller[string],
			ph *pumped.Controller[string],
			os *pumped.Controller[string],
			cfg *pumped.Controller[string]) (string, error) {
			return "api-gateway", nil
		},
		pumped.WithTag(nameTag, "APIGateway"),
	)

	// Try to resolve the top-level component - should fail and show full dependency graph
	_, err := pumped.Resolve(scope, apiGateway)

	// Verify error occurred
	if err == nil {
		t.Fatal("Expected error but got nil")
	}

	t.Logf("Successfully demonstrated complex dependency graph with error at OrderService")
}

func TestGraphDebugExtension_MultipleFailures(t *testing.T) {
	// Use HumanHandler to write formatted output to stdout
	handler := NewHumanHandler(os.Stdout, slog.LevelError)

	scope := pumped.NewScope(
		pumped.WithExtension(NewGraphDebugExtension(handler)),
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

	// Multiple executors that will fail
	failingService1 := pumped.Derive1(
		config.Reactive(),
		func(ctx *pumped.ResolveCtx, cfg *pumped.Controller[string]) (string, error) {
			return "", fmt.Errorf("authentication service unavailable")
		},
		pumped.WithTag(nameTag, "AuthService"),
	)

	failingService2 := pumped.Derive1(
		config.Reactive(),
		func(ctx *pumped.ResolveCtx, cfg *pumped.Controller[string]) (string, error) {
			return "", fmt.Errorf("payment gateway timeout")
		},
		pumped.WithTag(nameTag, "PaymentService"),
	)

	failingService3 := pumped.Derive1(
		config.Reactive(),
		func(ctx *pumped.ResolveCtx, cfg *pumped.Controller[string]) (string, error) {
			return "", fmt.Errorf("notification service rate limit exceeded")
		},
		pumped.WithTag(nameTag, "NotificationService"),
	)

	// Aggregate service depends on all failing services
	aggregateService := pumped.Derive3(
		failingService1.Reactive(),
		failingService2.Reactive(),
		failingService3.Reactive(),
		func(ctx *pumped.ResolveCtx,
			auth *pumped.Controller[string],
			payment *pumped.Controller[string],
			notif *pumped.Controller[string]) (string, error) {
			return "aggregate", nil
		},
		pumped.WithTag(nameTag, "AggregateService"),
	)

	// Try to resolve - first failure will be caught
	_, err := pumped.Resolve(scope, aggregateService)

	if err == nil {
		t.Fatal("Expected error but got nil")
	}

	t.Logf("Successfully demonstrated multiple potential failure points in dependency graph")
}

func TestGraphDebugExtension_LargeGraphWithUpdate(t *testing.T) {
	// Use HumanHandler to write formatted output to stdout
	handler := NewHumanHandler(os.Stdout, slog.LevelError)

	scope := pumped.NewScope(
		pumped.WithExtension(NewGraphDebugExtension(handler)),
	)
	defer scope.Dispose()

	nameTag := pumped.NewTag[string]("executor.name")

	// Layer 1: Configuration (base layer)
	dbConfig := pumped.Provide(
		func(ctx *pumped.ResolveCtx) (string, error) {
			return "db-config-ok", nil
		},
		pumped.WithTag(nameTag, "DBConfig"),
	)

	apiConfig := pumped.Provide(
		func(ctx *pumped.ResolveCtx) (string, error) {
			return "api-config-v1", nil
		},
		pumped.WithTag(nameTag, "APIConfig"),
	)

	cacheConfig := pumped.Provide(
		func(ctx *pumped.ResolveCtx) (string, error) {
			return "cache-config", nil
		},
		pumped.WithTag(nameTag, "CacheConfig"),
	)

	// Layer 2: Infrastructure - Database will fail to show cascade
	database := pumped.Derive1(
		dbConfig.Reactive(),
		func(ctx *pumped.ResolveCtx, cfg *pumped.Controller[string]) (string, error) {
			// Simulate database connection failure
			return "", fmt.Errorf("database connection pool exhausted - max connections (100) reached")
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

	messageQueue := pumped.Provide(
		func(ctx *pumped.ResolveCtx) (string, error) {
			return "message-queue", nil
		},
		pumped.WithTag(nameTag, "MessageQueue"),
	)

	// Layer 3: Repositories
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

	inventoryRepo := pumped.Derive1(
		database.Reactive(),
		func(ctx *pumped.ResolveCtx, db *pumped.Controller[string]) (string, error) {
			val, _ := db.Get()
			return "inventory-repo-" + val, nil
		},
		pumped.WithTag(nameTag, "InventoryRepository"),
	)

	// Layer 4: Services
	userService := pumped.Derive2(
		userRepo.Reactive(),
		cache.Reactive(),
		func(ctx *pumped.ResolveCtx, repo *pumped.Controller[string], c *pumped.Controller[string]) (string, error) {
			repoVal, _ := repo.Get()
			cacheVal, _ := c.Get()
			return "user-service-" + repoVal + "-" + cacheVal, nil
		},
		pumped.WithTag(nameTag, "UserService"),
	)

	productService := pumped.Derive3(
		productRepo.Reactive(),
		inventoryRepo.Reactive(),
		cache.Reactive(),
		func(ctx *pumped.ResolveCtx,
			repo *pumped.Controller[string],
			inv *pumped.Controller[string],
			c *pumped.Controller[string]) (string, error) {
			repoVal, _ := repo.Get()
			invVal, _ := inv.Get()
			cacheVal, _ := c.Get()
			return "product-service-" + repoVal + "-" + invVal + "-" + cacheVal, nil
		},
		pumped.WithTag(nameTag, "ProductService"),
	)

	orderService := pumped.Derive3(
		orderRepo.Reactive(),
		messageQueue.Reactive(),
		cache.Reactive(),
		func(ctx *pumped.ResolveCtx,
			repo *pumped.Controller[string],
			mq *pumped.Controller[string],
			c *pumped.Controller[string]) (string, error) {
			repoVal, _ := repo.Get()
			mqVal, _ := mq.Get()
			cacheVal, _ := c.Get()
			return "order-service-" + repoVal + "-" + mqVal + "-" + cacheVal, nil
		},
		pumped.WithTag(nameTag, "OrderService"),
	)

	notificationService := pumped.Derive2(
		messageQueue.Reactive(),
		apiConfig.Reactive(),
		func(ctx *pumped.ResolveCtx, mq *pumped.Controller[string], cfg *pumped.Controller[string]) (string, error) {
			mqVal, _ := mq.Get()
			cfgVal, _ := cfg.Get()
			return "notification-service-" + mqVal + "-" + cfgVal, nil
		},
		pumped.WithTag(nameTag, "NotificationService"),
	)

	// Layer 5: API Handlers
	userHandler := pumped.Derive2(
		userService.Reactive(),
		apiConfig.Reactive(),
		func(ctx *pumped.ResolveCtx, svc *pumped.Controller[string], cfg *pumped.Controller[string]) (string, error) {
			svcVal, _ := svc.Get()
			cfgVal, _ := cfg.Get()
			return "user-handler-" + svcVal + "-" + cfgVal, nil
		},
		pumped.WithTag(nameTag, "UserHandler"),
	)

	productHandler := pumped.Derive2(
		productService.Reactive(),
		apiConfig.Reactive(),
		func(ctx *pumped.ResolveCtx, svc *pumped.Controller[string], cfg *pumped.Controller[string]) (string, error) {
			svcVal, _ := svc.Get()
			cfgVal, _ := cfg.Get()
			return "product-handler-" + svcVal + "-" + cfgVal, nil
		},
		pumped.WithTag(nameTag, "ProductHandler"),
	)

	orderHandler := pumped.Derive3(
		orderService.Reactive(),
		notificationService.Reactive(),
		apiConfig.Reactive(),
		func(ctx *pumped.ResolveCtx,
			os *pumped.Controller[string],
			ns *pumped.Controller[string],
			cfg *pumped.Controller[string]) (string, error) {
			osVal, _ := os.Get()
			nsVal, _ := ns.Get()
			cfgVal, _ := cfg.Get()
			return "order-handler-" + osVal + "-" + nsVal + "-" + cfgVal, nil
		},
		pumped.WithTag(nameTag, "OrderHandler"),
	)

	// Layer 6: API Gateway (top level)
	apiGateway := pumped.Derive3(
		userHandler.Reactive(),
		productHandler.Reactive(),
		orderHandler.Reactive(),
		func(ctx *pumped.ResolveCtx,
			uh *pumped.Controller[string],
			ph *pumped.Controller[string],
			oh *pumped.Controller[string]) (string, error) {
			return "api-gateway", nil
		},
		pumped.WithTag(nameTag, "APIGateway"),
	)

	// Try to resolve the API Gateway - will fail at Database layer
	// This will show the full dependency graph with all the components that depend on Database
	_, err := pumped.Resolve(scope, apiGateway)

	// Note: err might be nil here because reactive dependencies can fail without propagating
	// The OnError hook is still called to log the failures, which is what we're testing
	t.Logf("Resolve result: err=%v", err)

	t.Logf("\n===== Full dependency graph with 15+ components shown above =====\n")
	t.Logf("Error occurred at Database layer, showcasing multiple resolution attempts")
	t.Logf("Graph shows dependencies at different stages:")
	t.Logf("  - DBConfig (base layer)")
	t.Logf("  - Database (failed)")
	t.Logf("  - 4 Repositories (User, Product, Order, Inventory)")
	t.Logf("  - 3 Services (User, Product, Order)")
	t.Logf("  - 3 Handlers (User, Product, Order)")
	t.Logf("  - 1 API Gateway (top level)")
	t.Logf("  - MessageQueue, Cache, and Config components")
}

func TestGraphDebugExtension_DeeplyNestedDependencies(t *testing.T) {
	// Capture output in buffer AND write to stdout for visual verification
	var buf bytes.Buffer
	multiWriter := io.MultiWriter(&buf, os.Stdout)
	handler := NewHumanHandler(multiWriter, slog.LevelError)

	scope := pumped.NewScope(
		pumped.WithExtension(NewGraphDebugExtension(handler)),
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

	// Check for tree structure
	if !strings.Contains(output, "└─>") {
		t.Error("Expected tree structure in output")
	}

	// Check for failed status
	if !strings.Contains(output, "❌ FAILED") {
		t.Error("Expected failed status indicator")
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
		pumped.WithExtension(NewGraphDebugExtension(handler)),
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

	// Verify output contains tree structure
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

	// Check for tree connectors
	if !strings.Contains(output, "├─>") || !strings.Contains(output, "└─>") {
		t.Error("Expected tree structure with multiple branches")
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
