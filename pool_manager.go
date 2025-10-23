package pumped

import (
	"context"
	"sync"
)

// PoolManager manages object pools for memory-efficient resolution
type PoolManager struct {
	// Pools for frequently allocated objects
	resolveCtxPool   sync.Pool
	executionCtxPool sync.Pool
	extensionPool    sync.Pool
	cleanupPool      sync.Pool

	// Metrics for pool efficiency
	metrics PoolMetrics
}

// PoolMetrics tracks pool usage statistics
type PoolMetrics struct {
	mu                sync.RWMutex
	resolveCtxHits    uint64
	resolveCtxMisses  uint64
	executionCtxHits  uint64
	executionCtxMisses uint64
	extensionHits     uint64
	extensionMisses   uint64
	cleanupHits       uint64
	cleanupMisses     uint64
}

// NewPoolManager creates a new pool manager with initialized pools
func NewPoolManager() *PoolManager {
	pm := &PoolManager{
		resolveCtxPool: sync.Pool{
			New: func() any {
				return &ResolveCtx{
					cleanups: make([]cleanupEntry, 0, 8), // Pre-allocate capacity
				}
			},
		},
		executionCtxPool: sync.Pool{
			New: func() any {
				return &ExecutionCtx{
					data: make(map[any]any, 16), // Pre-allocate capacity
				}
			},
		},
		extensionPool: sync.Pool{
			New: func() any {
				return make([]Extension, 0, 8) // Pre-allocate capacity
			},
		},
		cleanupPool: sync.Pool{
			New: func() any {
				return make([]cleanupEntry, 0, 8) // Pre-allocate capacity
			},
		},
	}

	return pm
}

// AcquireResolveCtx gets a ResolveCtx from the pool or creates a new one
func (pm *PoolManager) AcquireResolveCtx(scope *Scope, executorID AnyExecutor) *ResolveCtx {
	ctx, ok := pm.resolveCtxPool.Get().(*ResolveCtx)
	if ok {
		// Reset the context for reuse
		ctx.scope = scope
		ctx.executorID = executorID
		ctx.cleanups = ctx.cleanups[:0] // Reset slice but keep capacity

		pm.metrics.mu.Lock()
		pm.metrics.resolveCtxHits++
		pm.metrics.mu.Unlock()
	} else {
		// Create new context (pool miss)
		ctx = &ResolveCtx{
			scope:      scope,
			executorID: executorID,
			cleanups:   make([]cleanupEntry, 0, 8),
		}

		pm.metrics.mu.Lock()
		pm.metrics.resolveCtxMisses++
		pm.metrics.mu.Unlock()
	}

	return ctx
}

// ReleaseResolveCtx returns a ResolveCtx to the pool
func (pm *PoolManager) ReleaseResolveCtx(ctx *ResolveCtx) {
	if ctx == nil {
		return
	}

	// Clean up the context for reuse
	ctx.scope = nil
	ctx.executorID = nil
	// Keep the cleanup slice capacity, just reset length
	ctx.cleanups = ctx.cleanups[:0]

	pm.resolveCtxPool.Put(ctx)
}

// AcquireExecutionCtx gets an ExecutionCtx from the pool or creates a new one
func (pm *PoolManager) AcquireExecutionCtx(id string, parent *ExecutionCtx, scope *Scope, ctxParent context.Context) *ExecutionCtx {
	execCtx, ok := pm.executionCtxPool.Get().(*ExecutionCtx)
	if ok {
		// Reset the context for reuse
		execCtx.id = id
		execCtx.parent = parent
		execCtx.scope = scope
		execCtx.ctx = ctxParent

		// Reset the data map but keep capacity
		for k := range execCtx.data {
			delete(execCtx.data, k)
		}

		pm.metrics.mu.Lock()
		pm.metrics.executionCtxHits++
		pm.metrics.mu.Unlock()
	} else {
		// Create new context (pool miss)
		execCtx = &ExecutionCtx{
			id:     id,
			parent: parent,
			scope:  scope,
			data:   make(map[any]any, 16),
			ctx:    ctxParent,
		}

		pm.metrics.mu.Lock()
		pm.metrics.executionCtxMisses++
		pm.metrics.mu.Unlock()
	}

	return execCtx
}

// ReleaseExecutionCtx returns an ExecutionCtx to the pool
func (pm *PoolManager) ReleaseExecutionCtx(execCtx *ExecutionCtx) {
	if execCtx == nil {
		return
	}

	// Clean up the context for reuse (but don't clear data yet)
	execCtx.id = ""
	execCtx.parent = nil
	execCtx.scope = nil
	execCtx.ctx = nil

	// Note: Don't clear data here - it might be needed by finalize()
	// Data will be cleared when acquired from pool

	pm.executionCtxPool.Put(execCtx)
}

// AcquireExtensionSlice gets an extension slice from the pool or creates a new one
func (pm *PoolManager) AcquireExtensionSlice() []Extension {
	slice, ok := pm.extensionPool.Get().([]Extension)
	if ok {
		// Reset the slice but keep capacity
		slice = slice[:0]

		pm.metrics.mu.Lock()
		pm.metrics.extensionHits++
		pm.metrics.mu.Unlock()
	} else {
		// Create new slice (pool miss)
		slice = make([]Extension, 0, 8)

		pm.metrics.mu.Lock()
		pm.metrics.extensionMisses++
		pm.metrics.mu.Unlock()
	}

	return slice
}

// ReleaseExtensionSlice returns an extension slice to the pool
func (pm *PoolManager) ReleaseExtensionSlice(slice []Extension) {
	if slice == nil {
		return
	}

	// Reset the slice but keep capacity
	slice = slice[:0]

	pm.extensionPool.Put(slice)
}

// AcquireCleanupSlice gets a cleanup slice from the pool or creates a new one
func (pm *PoolManager) AcquireCleanupSlice() []cleanupEntry {
	slice, ok := pm.cleanupPool.Get().([]cleanupEntry)
	if ok {
		// Reset the slice but keep capacity
		slice = slice[:0]

		pm.metrics.mu.Lock()
		pm.metrics.cleanupHits++
		pm.metrics.mu.Unlock()
	} else {
		// Create new slice (pool miss)
		slice = make([]cleanupEntry, 0, 8)

		pm.metrics.mu.Lock()
		pm.metrics.cleanupMisses++
		pm.metrics.mu.Unlock()
	}

	return slice
}

// ReleaseCleanupSlice returns a cleanup slice to the pool
func (pm *PoolManager) ReleaseCleanupSlice(slice []cleanupEntry) {
	if slice == nil {
		return
	}

	// Reset the slice but keep capacity
	slice = slice[:0]

	pm.cleanupPool.Put(slice)
}

// GetMetrics returns a copy of the current pool metrics
func (pm *PoolManager) GetMetrics() PoolMetrics {
	pm.metrics.mu.RLock()
	defer pm.metrics.mu.RUnlock()

	return PoolMetrics{
		resolveCtxHits:     pm.metrics.resolveCtxHits,
		resolveCtxMisses:   pm.metrics.resolveCtxMisses,
		executionCtxHits:   pm.metrics.executionCtxHits,
		executionCtxMisses: pm.metrics.executionCtxMisses,
		extensionHits:      pm.metrics.extensionHits,
		extensionMisses:    pm.metrics.extensionMisses,
		cleanupHits:        pm.metrics.cleanupHits,
		cleanupMisses:      pm.metrics.cleanupMisses,
	}
}

// ResetMetrics resets all pool metrics to zero
func (pm *PoolManager) ResetMetrics() {
	pm.metrics.mu.Lock()
	defer pm.metrics.mu.Unlock()

	pm.metrics.resolveCtxHits = 0
	pm.metrics.resolveCtxMisses = 0
	pm.metrics.executionCtxHits = 0
	pm.metrics.executionCtxMisses = 0
	pm.metrics.extensionHits = 0
	pm.metrics.extensionMisses = 0
	pm.metrics.cleanupHits = 0
	pm.metrics.cleanupMisses = 0
}

// Global pool manager instance
var globalPoolManager = NewPoolManager()

// GetGlobalPoolManager returns the global pool manager instance
func GetGlobalPoolManager() *PoolManager {
	return globalPoolManager
}