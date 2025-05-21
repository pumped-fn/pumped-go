package core

import (
	"context"
	"sync"
)

// ExecutorKind represents the type of executor
type ExecutorKind string

const (
	// KindMain is the standard executor that holds a factory function and dependencies
	KindMain ExecutorKind = "main"
	// KindLazy doesn't resolve immediately, returns an accessor
	KindLazy ExecutorKind = "lazy"
	// KindReactive updates when dependencies change
	KindReactive ExecutorKind = "reactive"
	// KindStatic doesn't track for reactivity
	KindStatic ExecutorKind = "static"
)

// Cleanup is a function that performs cleanup operations
type Cleanup func() error

// Executor is the core interface for all executor types
type Executor[T any] interface {
	// Kind returns the kind of executor
	Kind() ExecutorKind
	// Dependencies returns the dependencies of this executor
	Dependencies() []any
	// Metadata returns the metadata associated with this executor
	Metadata() map[string]any
	// WithMeta adds metadata to the executor
	WithMeta(key string, value any) Executor[T]
}

// MainExecutor is the primary executor type that holds a factory function
type MainExecutor[T any] interface {
	Executor[T]
	// Lazy returns a lazy version of this executor
	Lazy() Executor[Accessor[T]]
	// Reactive returns a reactive version of this executor
	Reactive() Executor[T]
	// Static returns a static version of this executor
	Static() Executor[Accessor[T]]
}

// Accessor provides access to a resolved value
type Accessor[T any] interface {
	// Get returns the current value
	Get() T
	// Resolve ensures the value is resolved, optionally forcing re-resolution
	Resolve(ctx context.Context, force bool) (T, error)
	// Update updates the value
	Update(ctx context.Context, value T) error
	// UpdateFunc updates the value using a function
	UpdateFunc(ctx context.Context, fn func(T) T) error
	// Subscribe registers a callback for value changes
	Subscribe(callback func(T)) Cleanup
	// Release releases resources associated with this accessor
	Release(ctx context.Context) error
	// Metadata returns the metadata associated with this accessor
	Metadata() map[string]any
}

// Controller provides control over an executor's lifecycle
type Controller interface {
	// Cleanup registers a cleanup function
	Cleanup(cleanup Cleanup)
	// Release releases resources
	Release(ctx context.Context) error
	// Scope returns the scope this controller belongs to
	Scope() Scope
}

// Scope manages executor lifecycle
type Scope interface {
	// Get returns the current value of an executor
	Get[T any](executor Executor[T]) (T, error)
	// Resolve ensures an executor is resolved and returns its value
	Resolve[T any](ctx context.Context, executor Executor[T]) (T, error)
	// GetAccessor returns an accessor for an executor
	GetAccessor[T any](executor Executor[T]) (Accessor[T], error)
	// ResolveAccessor ensures an executor is resolved and returns its accessor
	ResolveAccessor[T any](ctx context.Context, executor Executor[T]) (Accessor[T], error)
	// Update updates the value of an executor
	Update[T any](ctx context.Context, executor Executor[T], value T) error
	// UpdateFunc updates the value of an executor using a function
	UpdateFunc[T any](ctx context.Context, executor Executor[T], fn func(T) T) error
	// Reset resets an executor
	Reset[T any](ctx context.Context, executor Executor[T]) error
	// Release releases resources associated with an executor
	Release[T any](ctx context.Context, executor Executor[T]) error
	// Dispose releases all resources
	Dispose(ctx context.Context) error
	// OnUpdate registers a callback for when an executor's value changes
	OnUpdate[T any](executor Executor[T], callback func(Accessor[T])) Cleanup
}

// ScopeOption configures a scope
type ScopeOption func(*scopeImpl)

// Preset represents a preset value for an executor
type Preset[T any] struct {
	Executor Executor[T]
	Value    T
}

