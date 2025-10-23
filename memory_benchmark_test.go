package pumped

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"testing"
)

// MemoryAllocationMetrics captures memory statistics for benchmarking
type MemoryAllocationMetrics struct {
	Allocs      uint64
	TotalAlloc  uint64
	Sys         uint64
	NumGC       uint32
	GCCPUFraction float64
}

// getMemoryMetrics captures current memory statistics
func getMemoryMetrics() MemoryAllocationMetrics {
	var m runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m)
	return MemoryAllocationMetrics{
		Allocs:      m.Mallocs,
		TotalAlloc:  m.TotalAlloc,
		Sys:         m.Sys,
		NumGC:       m.NumGC,
		GCCPUFraction: m.GCCPUFraction,
	}
}

// createTestDependencyChain creates a chain of dependencies for testing
func createTestDependencyChain(depth int) []*Executor[int] {
	executors := make([]*Executor[int], depth)

	for i := 0; i < depth; i++ {
		if i == 0 {
			executors[i] = Provide(func(ctx *ResolveCtx) (int, error) {
				return 1, nil
			})
		} else {
			prev := executors[i-1]
			executors[i] = Derive1(prev, func(ctx *ResolveCtx, ctrl *Controller[int]) (int, error) {
				val, err := ctrl.Get()
				if err != nil {
					return 0, err
				}
				return val + 1, nil
			})
		}
	}

	return executors
}

// createTestFlowChain creates a chain of flows for testing
func createTestFlowChain(depth int) []*Flow[int] {
	flows := make([]*Flow[int], depth)

	for i := 0; i < depth; i++ {
		if i == 0 {
			baseExec := Provide(func(ctx *ResolveCtx) (int, error) {
				return 1, nil
			})
			flows[i] = Flow1(baseExec, func(execCtx *ExecutionCtx, ctrl *Controller[int]) (int, error) {
				val, err := ctrl.Get()
				if err != nil {
					return 0, err
				}
				return val * 2, nil
			})
		} else {
			prev := flows[i-1]
			flows[i] = Flow1(Provide(func(ctx *ResolveCtx) (int, error) {
				return i + 1, nil
			}), func(execCtx *ExecutionCtx, ctrl *Controller[int]) (int, error) {
				baseVal, err := ctrl.Get()
				if err != nil {
					return 0, err
				}

				// Execute previous flow
				prevResult, _, err := Exec1(execCtx, prev)
				if err != nil {
					return 0, err
				}

				return baseVal + prevResult, nil
			})
		}
	}

	return flows
}

// BenchmarkResolveCtxAllocation measures memory allocation during executor resolution
func BenchmarkResolveCtxAllocation(b *testing.B) {
	scope := NewScope()
	defer scope.Dispose()

	// Create a moderately complex dependency graph
	base := Provide(func(ctx *ResolveCtx) (string, error) {
		return "base", nil
	})

	dependent := Derive1(base, func(ctx *ResolveCtx, ctrl *Controller[string]) (string, error) {
		val, err := ctrl.Get()
		if err != nil {
			return "", err
		}
		return val + "-dependent", nil
	})

	final := Derive1(dependent, func(ctx *ResolveCtx, ctrl *Controller[string]) (string, error) {
		val, err := ctrl.Get()
		if err != nil {
			return "", err
		}
		return val + "-final", nil
	})

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Clear cache to force re-resolution
		scope.cache = sync.Map{}

		_, err := Resolve(scope, final)
		if err != nil {
			b.Fatalf("resolution failed: %v", err)
		}
	}
}

// BenchmarkExecutionCtxAllocation measures memory allocation during flow execution
func BenchmarkExecutionCtxAllocation(b *testing.B) {
	scope := NewScope()
	defer scope.Dispose()

	input := Provide(func(ctx *ResolveCtx) (int, error) {
		return 42, nil
	})

	flow := Flow1(input, func(execCtx *ExecutionCtx, ctrl *Controller[int]) (int, error) {
		val, err := ctrl.Get()
		if err != nil {
			return 0, err
		}
		return val * 2, nil
	})

	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _, err := Exec(scope, ctx, flow)
		if err != nil {
			b.Fatalf("flow execution failed: %v", err)
		}
	}
}

// BenchmarkExtensionCopying measures memory allocation from extension slice copying
func BenchmarkExtensionCopying(b *testing.B) {
	// Create a scope with multiple extensions to maximize copying overhead
	scope := NewScope()
	defer scope.Dispose()

	// Add multiple mock extensions
	for i := 0; i < 10; i++ {
		ext := &mockExtension{id: i}
		scope.UseExtension(ext)
	}

	input := Provide(func(ctx *ResolveCtx) (int, error) {
		return 42, nil
	})

	output := Derive1(input, func(ctx *ResolveCtx, ctrl *Controller[int]) (int, error) {
		val, err := ctrl.Get()
		if err != nil {
			return 0, err
		}
		return val * 2, nil
	})

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := Resolve(scope, output)
		if err != nil {
			b.Fatalf("resolution failed: %v", err)
		}
	}
}

// BenchmarkReactiveDependencyTracking measures memory allocation in reactive dependency tracking
func BenchmarkReactiveDependencyTracking(b *testing.B) {
	scope := NewScope()
	defer scope.Dispose()

	base := Provide(func(ctx *ResolveCtx) (int, error) {
		return 0, nil
	})

	// Create a tree of reactive dependencies
	level1 := Derive1(base.Reactive(), func(ctx *ResolveCtx, ctrl *Controller[int]) (int, error) {
		val, err := ctrl.Get()
		if err != nil {
			return 0, err
		}
		return val + 1, nil
	})

	level2 := make([]*Executor[int], 10)
	for i := range level2 {
		level2[i] = Derive1(level1.Reactive(), func(ctx *ResolveCtx, ctrl *Controller[int]) (int, error) {
			val, err := ctrl.Get()
			if err != nil {
				return 0, err
			}
			return val + i + 1, nil
		})
	}

	// Initial resolution
	for _, exec := range level2 {
		_, err := Resolve(scope, exec)
		if err != nil {
			b.Fatalf("initial resolution failed: %v", err)
		}
	}

	baseCtrl := Accessor(scope, base)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Trigger reactive updates
		err := baseCtrl.Update(i)
		if err != nil {
			b.Fatalf("update failed: %v", err)
		}
	}
}

// BenchmarkConcurrentResolutions measures memory allocation under concurrent load
func BenchmarkConcurrentResolutions(b *testing.B) {
	scope := NewScope()
	defer scope.Dispose()

	// Create multiple independent dependency chains
	chains := make([]*Executor[int], 10)
	for i := range chains {
		chain := createTestDependencyChain(5)
		chains[i] = chain[len(chain)-1]
	}

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			for _, chain := range chains {
				_, err := Resolve(scope, chain)
				if err != nil {
					b.Fatalf("resolution failed: %v", err)
				}
			}
		}
	})
}

// BenchmarkComplexDependencyGraph measures memory allocation in complex scenarios
func BenchmarkComplexDependencyGraph(b *testing.B) {
	scope := NewScope()
	defer scope.Dispose()

	// Create a complex dependency graph with multiple levels
	base := Provide(func(ctx *ResolveCtx) (int, error) {
		return 1, nil
	})

	// Level 1: 3 dependencies
	l1 := make([]*Executor[int], 3)
	for i := range l1 {
		l1[i] = Derive1(base, func(ctx *ResolveCtx, ctrl *Controller[int]) (int, error) {
			val, err := ctrl.Get()
			if err != nil {
				return 0, err
			}
			return val + i + 1, nil
		})
	}

	// Level 2: 6 dependencies combining level 1
	l2 := make([]*Executor[int], 6)
	for i := range l2 {
		l2[i] = Derive2(l1[i%3], l1[(i+1)%3], func(ctx *ResolveCtx, ctrl1, ctrl2 *Controller[int]) (int, error) {
			v1, err := ctrl1.Get()
			if err != nil {
				return 0, err
			}
			v2, err := ctrl2.Get()
			if err != nil {
				return 0, err
			}
			return v1 + v2, nil
		})
	}

	// Level 3: Final result combining all level 2
	final := Derive6(l2[0], l2[1], l2[2], l2[3], l2[4], l2[5],
		func(ctx *ResolveCtx, c1, c2, c3, c4, c5, c6 *Controller[int]) (int, error) {
			sum := 0
			for _, ctrl := range []*Controller[int]{c1, c2, c3, c4, c5, c6} {
				val, err := ctrl.Get()
				if err != nil {
					return 0, err
				}
				sum += val
			}
			return sum, nil
		})

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Clear cache to force re-resolution
		scope.cache = sync.Map{}

		_, err := Resolve(scope, final)
		if err != nil {
			b.Fatalf("resolution failed: %v", err)
		}
	}
}

// BenchmarkMemoryUsageProfile provides detailed memory usage analysis
func BenchmarkMemoryUsageProfile(b *testing.B) {
	scenarios := []struct {
		name string
		fn   func(scope *Scope) error
	}{
		{
			name: "SimpleResolution",
			fn: func(scope *Scope) error {
				exec := Provide(func(ctx *ResolveCtx) (int, error) { return 42, nil })
				_, err := Resolve(scope, exec)
				return err
			},
		},
		{
			name: "DeepDependencyChain",
			fn: func(scope *Scope) error {
				chain := createTestDependencyChain(20)
				_, err := Resolve(scope, chain[len(chain)-1])
				return err
			},
		},
		{
			name: "WideDependencyGraph",
			fn: func(scope *Scope) error {
				base := Provide(func(ctx *ResolveCtx) (int, error) { return 1, nil })
				dependents := make([]*Executor[int], 50)
				for i := range dependents {
					dependents[i] = Derive1(base, func(ctx *ResolveCtx, ctrl *Controller[int]) (int, error) {
						val, err := ctrl.Get()
						if err != nil {
							return 0, err
						}
						return val + i + 1, nil
					})
				}

				// Resolve all dependents
				for _, dep := range dependents {
					_, err := Resolve(scope, dep)
					if err != nil {
						return err
					}
				}
				return nil
			},
		},
		{
			name: "FlowExecution",
			fn: func(scope *Scope) error {
				input := Provide(func(ctx *ResolveCtx) (int, error) { return 42, nil })
				flow := Flow1(input, func(execCtx *ExecutionCtx, ctrl *Controller[int]) (int, error) {
					val, err := ctrl.Get()
					if err != nil {
						return 0, err
					}
					return val * 2, nil
				})
				_, _, err := Exec(scope, context.Background(), flow)
				return err
			},
		},
		{
			name: "ComplexFlowChain",
			fn: func(scope *Scope) error {
				flows := createTestFlowChain(10)
				_, _, err := Exec(scope, context.Background(), flows[len(flows)-1])
				return err
			},
		},
	}

	for _, scenario := range scenarios {
		b.Run(scenario.name, func(b *testing.B) {
			b.StopTimer()
			initialMetrics := getMemoryMetrics()

			b.StartTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				scope := NewScope()

				err := scenario.fn(scope)
				if err != nil {
					b.Fatalf("scenario failed: %v", err)
				}

				scope.Dispose()
			}

			b.StopTimer()
			finalMetrics := getMemoryMetrics()

			// Report memory usage statistics
			allocDiff := finalMetrics.TotalAlloc - initialMetrics.TotalAlloc
			b.ReportMetric(float64(allocDiff)/float64(b.N), "bytes/op_total")
			b.ReportMetric(float64(finalMetrics.Allocs-initialMetrics.Allocs)/float64(b.N), "allocs/op")
		})
	}
}

// BenchmarkStressTest performs stress testing with high allocation rates
func BenchmarkStressTest(b *testing.B) {
	const (
		numScopes       = 100
		numExecutors    = 50
		numResolutions  = 10
	)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		var wg sync.WaitGroup

		for s := 0; s < numScopes; s++ {
			wg.Add(1)
			go func(scopeID int) {
				defer wg.Done()

				scope := NewScope()
				defer scope.Dispose()

				// Create multiple executors
				executors := make([]*Executor[string], numExecutors)
				for i := range executors {
					executors[i] = Provide(func(ctx *ResolveCtx) (string, error) {
						return fmt.Sprintf("exec-%d-%d", scopeID, i), nil
					})
				}

				// Perform multiple resolutions
				for r := 0; r < numResolutions; r++ {
					for _, exec := range executors {
						_, err := Resolve(scope, exec)
						if err != nil {
							b.Errorf("resolution failed: %v", err)
							return
						}
					}
				}
			}(s)
		}

		wg.Wait()
	}
}

// Mock extension for testing extension copying overhead
type mockExtension struct {
	id int
}

func (m *mockExtension) Name() string {
	return fmt.Sprintf("mock-extension-%d", m.id)
}

func (m *mockExtension) Order() int {
	return m.id
}

func (m *mockExtension) Init(s *Scope) error {
	return nil
}

func (m *mockExtension) Wrap(ctx context.Context, next func() (any, error), op *Operation) (any, error) {
	return next()
}

func (m *mockExtension) OnError(err error, op *Operation, s *Scope) {
}

func (m *mockExtension) OnFlowStart(ctx *ExecutionCtx, flow AnyFlow) error {
	return nil
}

func (m *mockExtension) OnFlowEnd(ctx *ExecutionCtx, result any, err error) error {
	return nil
}

func (m *mockExtension) OnFlowPanic(ctx *ExecutionCtx, panic any, stack []byte) error {
	return nil
}

func (m *mockExtension) OnCleanupError(err *CleanupError) bool {
	return false
}

func (m *mockExtension) Dispose(s *Scope) error {
	return nil
}