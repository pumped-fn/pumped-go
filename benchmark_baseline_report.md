# Memory Allocation Benchmark Baseline Report

**Generated:** 2025-10-23
**Go Version:** arm64/darwin
**CPU:** Apple M3

## Summary

This report establishes the baseline memory allocation patterns for the pumped-go library's resolve logic. The benchmarks measure the key memory hotspots identified during the analysis phase.

## Baseline Measurements

### Core Resolution Operations

| Benchmark | ns/op | B/op | allocs/op | Description |
|-----------|-------|------|-----------|-------------|
| BenchmarkResolveCtxAllocation | 1,391-1,507 | 1,078 | 26 | Simple executor resolution with dependency chain |
| BenchmarkExecutionCtxAllocation | 3,674-3,830 | 1,351 | 21 | Flow execution with ExecutionCtx creation |
| BenchmarkExtensionCopying | 14.9-16.1 | 0 | 0 | Extension slice copying (minimal overhead) |
| BenchmarkReactiveDependencyTracking | 2,639-4,266 | 1,104 | 11 | Reactive dependency updates and invalidation |

### Complex Scenarios

| Benchmark | ns/op | B/op | allocs/op | Description |
|-----------|-------|------|-----------|-------------|
| BenchmarkComplexDependencyGraph | 7,738-10,474 | 4,749-4,750 | 113 | Complex dependency tree with multiple levels |
| BenchmarkStressTest | 2,386,582 | 2,078,798 | 47,788 | High-load concurrent scenario |

### Memory Usage Profile by Scenario

| Scenario | ns/op | bytes/op_total | B/op | allocs/op | Memory Efficiency |
|----------|-------|----------------|------|-----------|-------------------|
| SimpleResolution | 734.4 | 976.0 | 976 | 14 | **High** - Minimal overhead |
| DeepDependencyChain | 11,905 | 10,670 | 10,669 | 229 | **Medium** - Chain allocation cost |
| WideDependencyGraph | 27,963 | 26,267 | 26,266 | 582 | **Low** - High graph expansion cost |
| FlowExecution | 4,795 | 2,657 | 2,657 | 41 | **Medium** - ExecutionCtx overhead |
| ComplexFlowChain | 56,426 | 20,779 | 20,779 | 357 | **Low** - Nested flow complexity |

## Key Memory Allocation Hotspots Identified

### 1. ResolveCtx Creation (26 allocs/op)
- **Location:** `scope.go:32-36`
- **Issue:** New ResolveCtx created for each executor resolution
- **Memory Impact:** 1,078 B/op for simple chains
- **Root Cause:** Fresh `[]cleanupEntry{}` slice allocation

### 2. ExecutionCtx Creation (41 allocs/op)
- **Location:** `flow.go:425-431`
- **Issue:** New ExecutionCtx with `data: make(map[any]any)` allocation
- **Memory Impact:** 2,657 B/op for flow execution
- **Root Cause:** Map allocation for execution data

### 3. Complex Dependency Graphs (582 allocs/op)
- **Location:** Multiple dependency resolution points
- **Issue:** Linear increase in allocations with graph complexity
- **Memory Impact:** 26,266 B/op for wide graphs
- **Root Cause:** No pooling or reuse of intermediate structures

### 4. Reactive Dependency Tracking (11 allocs/op)
- **Location:** `scope.go:111-115`
- **Issue:** Slice growth for reactive dependent tracking
- **Memory Impact:** 1,104 B/op per update
- **Root Cause:** Dynamic slice append operations

## Performance Impact Analysis

### High-Impact Areas (>1000 B/op)
1. **Wide Dependency Graphs:** 26,266 B/op (26x baseline)
2. **Deep Dependency Chains:** 10,669 B/op (10x baseline)
3. **Complex Flow Chains:** 20,779 B/op (20x baseline)
4. **Stress Test Scenarios:** 2,078,798 B/op (2000x baseline)

### Allocation Count Drivers
1. **Map Creation:** ExecutionCtx.data maps
2. **Slice Growth:** Reactive dependency tracking
3. **Interface Allocation:** Cleanup functions
4. **Extension Copying:** Multiple extensions (though minimal)

## Target Reduction Goals

Based on the analysis, the optimization should target:

### Primary Goals (60-80% reduction)
- **ResolveCtx Pooling:** Target 5 allocs/op (from 26)
- **ExecutionCtx Reuse:** Target 8 allocs/op (from 41)
- **Structural Sharing:** Target 40% memory reduction for complex graphs

### Secondary Goals (30-50% reduction)
- **Lazy Allocation:** Reduce slice pre-allocation
- **Buffer Optimization:** Smarter capacity estimation
- **Extension Snapshots:** Eliminate unnecessary copying

## Next Steps

1. **Implement Pool Manager:** Object pooling for ResolveCtx and ExecutionCtx
2. **Add Structural Sharing:** Copy-on-write for common data structures
3. **Lazy Allocation:** Conditional allocation based on usage patterns
4. **Re-run Benchmarks:** Compare against this baseline
5. **Validate Functional Equivalence:** Ensure no behavioral changes

## Measurement Methodology

- **Environment:** Consistent hardware and Go version
- **GC State:** Forced garbage collection before measurements
- **Multiple Runs:** Each benchmark run 3 times for consistency
- **Statistical Significance:** Median values reported for consistency

This baseline provides the foundation for measuring the effectiveness of memory optimization implementations.