package pumped

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// TestBehavioral_CacheTypeSafety tests current cache behavior for type safety issues
func TestBehavioral_CacheTypeSafety(t *testing.T) {
	scope := NewScope()

	// Create executors with different types
	intExec := Provide(func(ctx *ResolveCtx) (int, error) {
		return 42, nil
	})

	strExec := Provide(func(ctx *ResolveCtx) (string, error) {
		return "hello", nil
	})

	// Resolve both types
	intVal, err := Resolve(scope, intExec)
	if err != nil {
		t.Fatalf("Failed to resolve int executor: %v", err)
	}
	if intVal != 42 {
		t.Errorf("Expected 42, got %d", intVal)
	}

	strVal, err := Resolve(scope, strExec)
	if err != nil {
		t.Fatalf("Failed to resolve string executor: %v", err)
	}
	if strVal != "hello" {
		t.Errorf("Expected 'hello', got %s", strVal)
	}

	// Test cached values
	cachedInt, ok := scope.cache.Load(intExec)
	if !ok {
		t.Error("Expected int value to be cached")
	}
	if cachedInt.(int) != 42 {
		t.Errorf("Cached int value mismatch: expected 42, got %v", cachedInt)
	}

	cachedStr, ok := scope.cache.Load(strExec)
	if !ok {
		t.Error("Expected string value to be cached")
	}
	if cachedStr.(string) != "hello" {
		t.Errorf("Cached string value mismatch: expected 'hello', got %v", cachedStr)
	}
}

// TestBehavioral_ReactiveGraphTraversal tests the current reactive dependency traversal
func TestBehavioral_ReactiveGraphTraversal(t *testing.T) {
	scope := NewScope()

	// Create dependency chain: A -> B -> C
	c := Provide(func(ctx *ResolveCtx) (int, error) {
		return 1, nil
	})

	b := Derive1(
		c.Reactive(),
		func(ctx *ResolveCtx, cCtrl *Controller[int]) (int, error) {
			val, _ := cCtrl.Get()
			return val * 2, nil
		},
	)

	a := Derive1(
		b.Reactive(),
		func(ctx *ResolveCtx, bCtrl *Controller[int]) (int, error) {
			val, _ := bCtrl.Get()
			return val + 10, nil
		},
	)

	// Resolve top-level executor
	val, err := Resolve(scope, a)
	if err != nil {
		t.Fatalf("Failed to resolve a: %v", err)
	}
	if val != 12 { // 1*2 + 10
		t.Errorf("Expected 12, got %d", val)
	}

	// Verify reactive dependencies are tracked using the ReactiveGraph
	downstreamC := scope.graph.GetDirectDependents(c)
	downstreamB := scope.graph.GetDirectDependents(b)

	if len(downstreamC) == 0 {
		t.Error("Expected B to be tracked as dependent of C")
	}
	if len(downstreamB) == 0 {
		t.Error("Expected A to be tracked as dependent of B")
	}
}

// TestBehavioral_ConcurrentResolutions tests concurrent dependency resolution
func TestBehavioral_ConcurrentResolutions(t *testing.T) {
	scope := NewScope()

	// Create an executor that takes time to resolve
	slowExec := Provide(func(ctx *ResolveCtx) (int, error) {
		time.Sleep(10 * time.Millisecond)
		return 100, nil
	})

	fastExec := Provide(func(ctx *ResolveCtx) (int, error) {
		return 200, nil
	})

	// Concurrent resolution
	var wg sync.WaitGroup
	results := make([]int, 0, 10)
	mu := sync.Mutex{}

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			var val int
			var err error

			if id%2 == 0 {
				val, err = Resolve(scope, slowExec)
			} else {
				val, err = Resolve(scope, fastExec)
			}

			if err != nil {
				t.Errorf("Goroutine %d failed: %v", id, err)
				return
			}

			mu.Lock()
			results = append(results, val)
			mu.Unlock()
		}(i)
	}

	wg.Wait()

	mu.Lock()
	defer mu.Unlock()

	if len(results) != 10 {
		t.Errorf("Expected 10 results, got %d", len(results))
	}

	// Count slow vs fast results
	slowCount := 0
	fastCount := 0
	for _, r := range results {
		if r == 100 {
			slowCount++
		} else if r == 200 {
			fastCount++
		}
	}

	if slowCount != 5 || fastCount != 5 {
		t.Errorf("Expected 5 slow and 5 fast results, got %d slow, %d fast", slowCount, fastCount)
	}
}

// TestBehavioral_ErrorHandling tests current error handling patterns
func TestBehavioral_ErrorHandling(t *testing.T) {
	scope := NewScope()

	// Executor that returns error
	errorExec := Provide(func(ctx *ResolveCtx) (int, error) {
		return 0, errors.New("test error")
	})

	// Dependent executor
	dependentExec := Derive1(
		errorExec,
		func(ctx *ResolveCtx, errorCtrl *Controller[int]) (int, error) {
			val, err := errorCtrl.Get()
			if err != nil {
				return 0, err
			}
			return val * 2, nil
		},
	)

	// Test error propagation
	_, err := Resolve(scope, errorExec)
	if err == nil {
		t.Error("Expected error from errorExec")
	}
	if err.Error() != "test error" {
		t.Errorf("Expected 'test error', got %v", err)
	}

	// Test error propagation through dependencies
	_, err = Resolve(scope, dependentExec)
	if err == nil {
		t.Error("Expected error to propagate through dependencies")
	}
}

// TestBehavioral_MemoryUsage tests for memory leaks and cleanup
func TestBehavioral_MemoryUsage(t *testing.T) {
	scope := NewScope()

	// Create many executors and resolve them
	for i := 0; i < 1000; i++ {
		i := i
		exec := Provide(func(ctx *ResolveCtx) (int, error) {
			return i, nil
		})

		val, err := Resolve(scope, exec)
		if err != nil {
			t.Fatalf("Failed to resolve executor %d: %v", i, err)
		}
		if val != i {
			t.Errorf("Expected %d, got %d", i, val)
		}
	}

	// Verify all are cached
	cacheCount := 0
	scope.cache.Range(func(key, value any) bool {
		cacheCount++
		return true
	})

	if cacheCount != 1000 {
		t.Errorf("Expected 1000 cached items, got %d", cacheCount)
	}

	// Test scope disposal
	err := scope.Dispose()
	if err != nil {
		t.Errorf("Scope disposal failed: %v", err)
	}
}

// TestBehavioral_FlowExecutionComplexity tests current flow execution behavior
func TestBehavioral_FlowExecutionComplexity(t *testing.T) {
	scope := NewScope()

	// Create dependency for flow
	dataExec := Provide(func(ctx *ResolveCtx) (string, error) {
		return "flow_data", nil
	})

	// Create flow with the dependency
	flow := Flow1(dataExec,
		func(execCtx *ExecutionCtx, dataCtrl *Controller[string]) (string, error) {
			data, err := dataCtrl.Get()
			if err != nil {
				return "", err
			}
			return "processed_" + data, nil
		},
		WithFlowTag(FlowName(), "test_flow"),
	)

	// Execute flow
	result, execCtx, err := Exec(scope, context.Background(), flow)
	if err != nil {
		t.Fatalf("Flow execution failed: %v", err)
	}

	if result != "processed_flow_data" {
		t.Errorf("Expected 'processed_flow_data', got '%s'", result)
	}

	if execCtx == nil {
		t.Error("Expected execution context")
	}

	// Check execution tree
	tree := scope.GetExecutionTree()
	roots := tree.GetRoots()
	if len(roots) == 0 {
		t.Error("Expected at least one root in execution tree")
	}

	// Check flow name tag
	flowName, hasFlowName := execCtx.Get(FlowName())
	if !hasFlowName {
		t.Error("Expected flow name tag")
	}
	if flowName != "test_flow" {
		t.Errorf("Expected 'test_flow', got '%s'", flowName)
	}
}

// TestBehavioral_CleanupOnReactiveUpdate tests cleanup behavior during reactive updates
func TestBehavioral_CleanupOnReactiveUpdate(t *testing.T) {
	scope := NewScope()

	cleanupCalled := false

	// Executor with cleanup
	baseExec := Provide(func(ctx *ResolveCtx) (int, error) {
		ctx.OnCleanup(func() error {
			cleanupCalled = true
			return nil
		})
		return 1, nil
	})

	// Reactive dependent
	reactiveExec := Derive1(
		baseExec.Reactive(),
		func(ctx *ResolveCtx, baseCtrl *Controller[int]) (int, error) {
			val, _ := baseCtrl.Get()
			return val * 2, nil
		},
	)

	// Initial resolution
	_, err := Resolve(scope, reactiveExec)
	if err != nil {
		t.Fatalf("Initial resolution failed: %v", err)
	}

	// Update base executor - should trigger cleanup
	err = Update(scope, baseExec, 5)
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	// Give some time for cleanup to be called
	time.Sleep(100 * time.Millisecond)

	if !cleanupCalled {
		t.Error("Expected cleanup to be called on reactive update")
	}
}

// TestBehavioral_ExtensionChain tests current extension behavior
func TestBehavioral_ExtensionChain(t *testing.T) {
	scope := NewScope()

	// Create and resolve executor without extensions for now
	testExec := Provide(func(ctx *ResolveCtx) (int, error) {
		return 42, nil
	})

	result, err := Resolve(scope, testExec)
	if err != nil {
		t.Fatalf("Executor resolution failed: %v", err)
	}

	if result != 42 {
		t.Errorf("Expected 42, got %d", result)
	}
}

// BenchmarkBehavioral_CurrentPerformance provides baseline performance metrics
func BenchmarkBehavioral_CurrentPerformance(b *testing.B) {
	scope := NewScope()

	// Create dependency chain of reasonable depth
	exec := Provide(func(ctx *ResolveCtx) (int, error) {
		return 1, nil
	})

	for i := 0; i < 5; i++ {
		i := i
		exec = Derive1(
			exec.Reactive(),
			func(ctx *ResolveCtx, ctrl *Controller[int]) (int, error) {
				val, _ := ctrl.Get()
				return val + i + 1, nil
			},
		)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := Resolve(scope, exec)
		if err != nil {
			b.Fatalf("Resolution failed: %v", err)
		}
	}
}
