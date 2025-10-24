package pumped

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// TestGracefulShutdown_UpdateCancellation tests context cancellation during Update
// with reactive dependents, simulating graceful shutdown scenario
func TestGracefulShutdown_UpdateCancellation(t *testing.T) {
	scope := NewScope()

	// Track cleanup calls
	var mu sync.Mutex
	cleanupCalls := []string{}

	// Root executor
	root := Provide(func(ctx *ResolveCtx) (int, error) {
		ctx.OnCleanup(func() error {
			mu.Lock()
			cleanupCalls = append(cleanupCalls, "root")
			mu.Unlock()
			return nil
		})
		return 0, nil
	})

	// Create a chain of reactive dependents
	dep1 := Derive1(
		root.Reactive(),
		func(ctx *ResolveCtx, rootCtrl *Controller[int]) (int, error) {
			val, _ := rootCtrl.Get()
			ctx.OnCleanup(func() error {
				mu.Lock()
				cleanupCalls = append(cleanupCalls, "dep1")
				mu.Unlock()
				return nil
			})
			return val + 1, nil
		},
	)

	dep2 := Derive1(
		root.Reactive(),
		func(ctx *ResolveCtx, rootCtrl *Controller[int]) (int, error) {
			val, _ := rootCtrl.Get()
			ctx.OnCleanup(func() error {
				mu.Lock()
				cleanupCalls = append(cleanupCalls, "dep2")
				mu.Unlock()
				return nil
			})
			return val + 2, nil
		},
	)

	dep3 := Derive1(
		root.Reactive(),
		func(ctx *ResolveCtx, rootCtrl *Controller[int]) (int, error) {
			val, _ := rootCtrl.Get()
			ctx.OnCleanup(func() error {
				mu.Lock()
				cleanupCalls = append(cleanupCalls, "dep3")
				mu.Unlock()
				return nil
			})
			return val + 3, nil
		},
	)

	// Resolve all executors to cache them
	_, err := Resolve(scope, root)
	if err != nil {
		t.Fatalf("failed to resolve root: %v", err)
	}

	_, err = Resolve(scope, dep1)
	if err != nil {
		t.Fatalf("failed to resolve dep1: %v", err)
	}

	_, err = Resolve(scope, dep2)
	if err != nil {
		t.Fatalf("failed to resolve dep2: %v", err)
	}

	_, err = Resolve(scope, dep3)
	if err != nil {
		t.Fatalf("failed to resolve dep3: %v", err)
	}

	// Create a context with timeout to simulate graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// Create an extension that introduces delay to trigger cancellation
	slowExt := &slowUpdateExtension{
		delay: 20 * time.Millisecond,
	}
	scope.UseExtension(slowExt)

	// Attempt to update with a context that will be cancelled
	rootCtrl := Accessor(scope, root)
	err = rootCtrl.Update(ctx, 10)

	// We expect either success or context cancellation
	// The behavior depends on timing - if context is cancelled before update completes,
	// we should get context.DeadlineExceeded
	if err != nil && !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected nil or context.DeadlineExceeded, got: %v", err)
	}

	// Verify cleanups were called (root + some dependents)
	mu.Lock()
	numCleanups := len(cleanupCalls)
	mu.Unlock()

	if numCleanups == 0 {
		t.Error("expected at least root cleanup to be called")
	}

	// The root should always be cleaned up
	mu.Lock()
	hasRootCleanup := false
	for _, call := range cleanupCalls {
		if call == "root" {
			hasRootCleanup = true
			break
		}
	}
	mu.Unlock()

	if !hasRootCleanup {
		t.Error("expected root cleanup to be called")
	}
}

// TestGracefulShutdown_ImmediateCancellation tests Update with already-cancelled context
func TestGracefulShutdown_ImmediateCancellation(t *testing.T) {
	scope := NewScope()

	root := Provide(func(ctx *ResolveCtx) (int, error) {
		return 0, nil
	})

	dep := Derive1(
		root.Reactive(),
		func(ctx *ResolveCtx, rootCtrl *Controller[int]) (int, error) {
			val, _ := rootCtrl.Get()
			return val + 1, nil
		},
	)

	// Resolve both
	Resolve(scope, root)
	Resolve(scope, dep)

	// Add extension that checks context
	scope.UseExtension(&contextCheckExtension{})

	// Create already-cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// Attempt update with cancelled context
	rootCtrl := Accessor(scope, root)
	err := rootCtrl.Update(ctx, 10)

	// Should get context.Canceled error (detected by extension)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got: %v", err)
	}

	// Dependent should not be invalidated since update failed
	depVal, _ := Resolve(scope, dep)
	if depVal != 1 {
		t.Errorf("expected dep to still have old value 1, got %d", depVal)
	}
}

// TestGracefulShutdown_PartialInvalidation tests that graceful cancellation
// leaves the cache in a consistent state even with partial invalidation
func TestGracefulShutdown_PartialInvalidation(t *testing.T) {
	scope := NewScope()

	root := Provide(func(ctx *ResolveCtx) (int, error) {
		return 0, nil
	})

	// Create multiple reactive dependents
	dep1 := Derive1(root.Reactive(), func(ctx *ResolveCtx, rootCtrl *Controller[int]) (int, error) {
		val, _ := rootCtrl.Get()
		return val + 1, nil
	})

	dep2 := Derive1(root.Reactive(), func(ctx *ResolveCtx, rootCtrl *Controller[int]) (int, error) {
		val, _ := rootCtrl.Get()
		return val + 2, nil
	})

	// Resolve all
	Resolve(scope, root)
	Resolve(scope, dep1)
	Resolve(scope, dep2)

	// Use a context with short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	// Add slow extension
	scope.UseExtension(&slowUpdateExtension{delay: 100 * time.Millisecond})

	// Attempt update
	rootCtrl := Accessor(scope, root)
	err := rootCtrl.Update(ctx, 10)

	// Should get timeout
	if err != nil && !errors.Is(err, context.DeadlineExceeded) {
		t.Logf("got error: %v", err)
	}

	// Even if update partially failed, the scope should still be usable
	// Try to dispose cleanly
	disposeErr := scope.Dispose()
	if disposeErr != nil {
		t.Errorf("dispose should succeed even after partial update, got: %v", disposeErr)
	}
}

// TestResolutionCancellation_FlowExecution tests context cancellation during flow execution
func TestResolutionCancellation_FlowExecution(t *testing.T) {
	scope := NewScope()

	// Create slow dependency
	slowDep := Provide(func(ctx *ResolveCtx) (int, error) {
		time.Sleep(100 * time.Millisecond)
		return 42, nil
	})

	// Create flow that depends on slow dependency
	flow := Flow1(
		slowDep,
		func(ctx *ExecutionCtx, slowCtrl *Controller[int]) (int, error) {
			val, err := slowCtrl.Get()
			if err != nil {
				return 0, err
			}
			return val * 2, nil
		},
	)

	// Create context with short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	// Execute flow with cancellable context
	_, execCtx, err := Exec(scope, ctx, flow)

	// Should get context error
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected context.DeadlineExceeded, got: %v", err)
	}

	// Execution context should show cancelled status
	if execCtx != nil {
		status, ok := execCtx.Get(statusTag)
		if !ok {
			t.Errorf("expected status tag to be present")
		}
		execStatus, _ := status.(ExecutionStatus)
		if execStatus != ExecutionStatusCancelled {
			t.Errorf("expected ExecutionStatusCancelled, got: %v", execStatus)
		}
	}
}

// TestResolutionCancellation_BeforeFlowExecution tests cancellation before flow starts
func TestResolutionCancellation_BeforeFlowExecution(t *testing.T) {
	scope := NewScope()

	dep := Provide(func(ctx *ResolveCtx) (int, error) {
		return 42, nil
	})

	flow := Flow1(
		dep,
		func(ctx *ExecutionCtx, depCtrl *Controller[int]) (int, error) {
			val, _ := depCtrl.Get()
			return val * 2, nil
		},
	)

	// Create already-cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Execute flow
	result, execCtx, err := Exec(scope, ctx, flow)

	// Should fail immediately with context.Canceled
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got: %v", err)
	}

	// Result should be zero value
	if result != 0 {
		t.Errorf("expected zero result, got: %d", result)
	}

	// Execution context should exist and show cancelled
	if execCtx == nil {
		t.Fatal("expected execution context to exist")
	}

	status, ok := execCtx.Get(statusTag)
	if !ok {
		t.Fatal("expected status tag to be set")
	}

	execStatus, ok := status.(ExecutionStatus)
	if !ok {
		t.Fatal("expected status to be ExecutionStatus type")
	}

	if execStatus != ExecutionStatusCancelled {
		t.Errorf("expected ExecutionStatusCancelled, got: %v", execStatus)
	}

	// Error should be recorded
	errorVal, ok := execCtx.Get(errorTag)
	if !ok {
		t.Error("expected error tag to be set")
	}

	if !errors.Is(errorVal.(error), context.Canceled) {
		t.Errorf("expected context.Canceled in error tag, got: %v", errorVal)
	}
}

// TestResolutionCancellation_DuringDependencyResolution tests cancellation
// while resolving flow dependencies
func TestResolutionCancellation_DuringDependencyResolution(t *testing.T) {
	scope := NewScope()

	// First dependency is fast
	fastDep := Provide(func(ctx *ResolveCtx) (int, error) {
		return 1, nil
	})

	// Second dependency is slow
	slowDep := Provide(func(ctx *ResolveCtx) (int, error) {
		time.Sleep(100 * time.Millisecond)
		return 2, nil
	})

	// Flow depends on both
	flow := Flow2(
		fastDep,
		slowDep,
		func(ctx *ExecutionCtx, fastCtrl *Controller[int], slowCtrl *Controller[int]) (int, error) {
			fast, _ := fastCtrl.Get()
			slow, _ := slowCtrl.Get()
			return fast + slow, nil
		},
	)

	// Context with timeout that allows first dep but not second
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	// Execute
	_, execCtx, err := Exec(scope, ctx, flow)

	// Should get cancellation error
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected context.DeadlineExceeded, got: %v", err)
	}

	// Status should be cancelled
	if execCtx != nil {
		status, _ := execCtx.Get(statusTag)
		execStatus, _ := status.(ExecutionStatus)
		if execStatus != ExecutionStatusCancelled {
			t.Errorf("expected ExecutionStatusCancelled, got: %v", execStatus)
		}
	}
}

// TestResolutionCancellation_PropagationToFlow tests that context cancellation
// is properly detected during flow execution
func TestResolutionCancellation_PropagationToFlow(t *testing.T) {
	scope := NewScope()

	dep := Provide(func(ctx *ResolveCtx) (int, error) {
		return 1, nil
	})

	// Flow that checks context during execution
	flow := Flow1(
		dep,
		func(ctx *ExecutionCtx, depCtrl *Controller[int]) (int, error) {
			// Simulate some work
			select {
			case <-ctx.Context().Done():
				return 0, ctx.Context().Err()
			case <-time.After(100 * time.Millisecond):
				val, _ := depCtrl.Get()
				return val * 2, nil
			}
		},
	)

	// Context with short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	// Execute
	_, execCtx, err := Exec(scope, ctx, flow)

	// Should get cancellation error
	if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
		t.Errorf("expected context error, got: %v", err)
	}

	// Status should reflect cancellation
	if execCtx != nil {
		status, _ := execCtx.Get(statusTag)
		execStatus, _ := status.(ExecutionStatus)
		if execStatus != ExecutionStatusCancelled && execStatus != ExecutionStatusFailed {
			t.Errorf("expected ExecutionStatusCancelled or ExecutionStatusFailed, got: %v", execStatus)
		}
	}
}

// slowUpdateExtension is a test extension that introduces delay during updates
type slowUpdateExtension struct {
	BaseExtension
	delay time.Duration
}

func (e *slowUpdateExtension) Name() string {
	return "slow-update"
}

func (e *slowUpdateExtension) Order() int {
	return 1000
}

func (e *slowUpdateExtension) Wrap(ctx context.Context, next func() (any, error), op *Operation) (any, error) {
	if op.Kind == OpUpdate {
		// Check if context is already cancelled
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		// Introduce delay
		select {
		case <-time.After(e.delay):
			return next()
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return next()
}

// contextCheckExtension is a test extension that immediately checks for context cancellation
type contextCheckExtension struct {
	BaseExtension
}

func (e *contextCheckExtension) Name() string {
	return "context-check"
}

func (e *contextCheckExtension) Order() int {
	return 0 // Run first
}

func (e *contextCheckExtension) Wrap(ctx context.Context, next func() (any, error), op *Operation) (any, error) {
	// Check if context is cancelled before proceeding
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
		return next()
	}
}

// TestFrameworkEnforcesContextCancellation verifies that the framework itself
// checks context cancellation, NOT relying on extensions
func TestFrameworkEnforcesContextCancellation(t *testing.T) {
	// Create scope WITHOUT any extensions
	scope := NewScope()

	root := Provide(func(ctx *ResolveCtx) (int, error) {
		return 0, nil
	})

	dep := Derive1(
		root.Reactive(),
		func(ctx *ResolveCtx, rootCtrl *Controller[int]) (int, error) {
			val, _ := rootCtrl.Get()
			return val + 1, nil
		},
	)

	// Resolve both
	Resolve(scope, root)
	Resolve(scope, dep)

	// Create cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Attempt update with cancelled context - should fail even without extensions
	rootCtrl := Accessor(scope, root)
	err := rootCtrl.Update(ctx, 10)

	// Framework should detect cancellation
	if !errors.Is(err, context.Canceled) {
		t.Errorf("framework should enforce context cancellation, got: %v", err)
	}

	// Dependent should not be updated
	depVal, _ := Resolve(scope, dep)
	if depVal != 1 {
		t.Errorf("expected dep to have old value 1, got %d (update should have been cancelled)", depVal)
	}
}

// TestFrameworkGracefulCancellation verifies graceful cancellation behavior
// where cleanup starts but is interrupted partway through
func TestFrameworkGracefulCancellation(t *testing.T) {
	scope := NewScope()

	cleanupCalls := 0
	var mu sync.Mutex

	root := Provide(func(ctx *ResolveCtx) (int, error) {
		ctx.OnCleanup(func() error {
			mu.Lock()
			cleanupCalls++
			mu.Unlock()
			return nil
		})
		return 0, nil
	})

	// Create many reactive dependents
	deps := make([]*Executor[int], 10)
	for i := 0; i < 10; i++ {
		deps[i] = Derive1(
			root.Reactive(),
			func(ctx *ResolveCtx, rootCtrl *Controller[int]) (int, error) {
				val, _ := rootCtrl.Get()
				ctx.OnCleanup(func() error {
					mu.Lock()
					cleanupCalls++
					mu.Unlock()
					// Simulate slow cleanup
					time.Sleep(20 * time.Millisecond)
					return nil
				})
				return val + 1, nil
			},
		)
	}

	// Resolve all
	Resolve(scope, root)
	for _, dep := range deps {
		Resolve(scope, dep)
	}

	// Context with timeout that will cancel mid-cleanup
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// Attempt update
	rootCtrl := Accessor(scope, root)
	err := rootCtrl.Update(ctx, 10)

	// Should get partial completion error
	if err == nil {
		t.Error("expected error due to context cancellation")
	}

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Logf("got error (may be partial completion): %v", err)
	}

	// Some cleanups should have been called, but not all
	mu.Lock()
	calls := cleanupCalls
	mu.Unlock()

	if calls == 0 {
		t.Error("expected at least root cleanup to be called")
	}

	if calls > 11 {
		t.Errorf("expected at most 11 cleanups (root + 10 deps), got %d", calls)
	}

	t.Logf("Partial cleanup: %d/%d cleanups completed before cancellation", calls, 11)
}

// TestFrameworkContextCheckPoints verifies context is checked at all key points
func TestFrameworkContextCheckPoints(t *testing.T) {
	scope := NewScope()

	root := Provide(func(ctx *ResolveCtx) (int, error) {
		return 0, nil
	})

	dep1 := Derive1(root.Reactive(), func(ctx *ResolveCtx, rootCtrl *Controller[int]) (int, error) {
		val, _ := rootCtrl.Get()
		return val + 1, nil
	})

	dep2 := Derive1(root.Reactive(), func(ctx *ResolveCtx, rootCtrl *Controller[int]) (int, error) {
		val, _ := rootCtrl.Get()
		return val + 2, nil
	})

	// Resolve all
	Resolve(scope, root)
	Resolve(scope, dep1)
	Resolve(scope, dep2)

	// Test 1: Cancel before update starts
	ctx1, cancel1 := context.WithCancel(context.Background())
	cancel1()

	rootCtrl := Accessor(scope, root)
	err := rootCtrl.Update(ctx1, 10)

	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled at start, got: %v", err)
	}

	// Test 2: Cancel after some work (using timeout)
	ctx2, cancel2 := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel2()

	// Give it a moment to expire
	time.Sleep(5 * time.Millisecond)

	err = rootCtrl.Update(ctx2, 20)

	if err == nil {
		t.Error("expected error from expired context")
	}

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Logf("got error (acceptable): %v", err)
	}
}
