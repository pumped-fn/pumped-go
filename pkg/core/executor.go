package core

import (
	"context"
	"fmt"
	"reflect"
)

// executorBase is the base implementation for all executors
type executorBase[T any] struct {
	kind         ExecutorKind
	dependencies []any
	metadata     map[string]any
}

// Kind returns the kind of executor
func (e *executorBase[T]) Kind() ExecutorKind {
	return e.kind
}

// Dependencies returns the dependencies of this executor
func (e *executorBase[T]) Dependencies() []any {
	return e.dependencies
}

// Metadata returns the metadata associated with this executor
func (e *executorBase[T]) Metadata() map[string]any {
	return e.metadata
}

// WithMeta adds metadata to the executor
func (e *executorBase[T]) WithMeta(key string, value any) Executor[T] {
	if e.metadata == nil {
		e.metadata = make(map[string]any)
	}
	e.metadata[key] = value
	return e
}

// mainExecutor is the primary executor implementation
type mainExecutor[T any, D any] struct {
	executorBase[T]
	factory func(D, Controller) (T, error)
}

// Lazy returns a lazy version of this executor
func (e *mainExecutor[T, D]) Lazy() Executor[Accessor[T]] {
	return &lazyExecutor[T]{
		executorBase: executorBase[Accessor[T]]{
			kind:         KindLazy,
			dependencies: nil,
			metadata:     e.metadata,
		},
		main: e,
	}
}

// Reactive returns a reactive version of this executor
func (e *mainExecutor[T, D]) Reactive() Executor[T] {
	return &reactiveExecutor[T]{
		executorBase: executorBase[T]{
			kind:         KindReactive,
			dependencies: nil,
			metadata:     e.metadata,
		},
		main: e,
	}
}

// Static returns a static version of this executor
func (e *mainExecutor[T, D]) Static() Executor[Accessor[T]] {
	return &staticExecutor[T]{
		executorBase: executorBase[Accessor[T]]{
			kind:         KindStatic,
			dependencies: nil,
			metadata:     e.metadata,
		},
		main: e,
	}
}

// lazyExecutor is an executor that doesn't resolve immediately
type lazyExecutor[T any] struct {
	executorBase[Accessor[T]]
	main *mainExecutor[T, any]
}

// reactiveExecutor is an executor that updates when dependencies change
type reactiveExecutor[T any] struct {
	executorBase[T]
	main *mainExecutor[T, any]
}

// staticExecutor is an executor that doesn't track for reactivity
type staticExecutor[T any] struct {
	executorBase[Accessor[T]]
	main *mainExecutor[T, any]
}

// Provide creates an executor with no dependencies
func Provide[T any](factory func(Controller) (T, error)) MainExecutor[T] {
	return &mainExecutor[T, struct{}]{
		executorBase: executorBase[T]{
			kind:         KindMain,
			dependencies: nil,
			metadata:     make(map[string]any),
		},
		factory: func(_ struct{}, ctrl Controller) (T, error) {
			return factory(ctrl)
		},
	}
}

// Derive creates an executor with a single dependency
func Derive[T any, D any](
	dependency Executor[D],
	factory func(D, Controller) (T, error),
) MainExecutor[T] {
	return &mainExecutor[T, D]{
		executorBase: executorBase[T]{
			kind:         KindMain,
			dependencies: []any{dependency},
			metadata:     make(map[string]any),
		},
		factory: factory,
	}
}

// DeriveMulti creates an executor with multiple dependencies of the same type
func DeriveMulti[T any, D any](
	dependencies []Executor[D],
	factory func([]D, Controller) (T, error),
) MainExecutor[T] {
	deps := make([]any, len(dependencies))
	for i, dep := range dependencies {
		deps[i] = dep
	}

	return &mainExecutor[T, []D]{
		executorBase: executorBase[T]{
			kind:         KindMain,
			dependencies: deps,
			metadata:     make(map[string]any),
		},
		factory: factory,
	}
}

// DeriveMap creates an executor with map dependencies
func DeriveMap[T any, K comparable, V any](
	dependencies map[K]Executor[V],
	factory func(map[K]V, Controller) (T, error),
) MainExecutor[T] {
	deps := make([]any, 0, len(dependencies))
	for _, dep := range dependencies {
		deps = append(deps, dep)
	}

	return &mainExecutor[T, map[K]V]{
		executorBase: executorBase[T]{
			kind:         KindMain,
			dependencies: deps,
			metadata:     make(map[string]any),
		},
		factory: factory,
	}
}

// DeriveTyped creates an executor with multiple dependencies of different types
func DeriveTyped[T any, D1 any, D2 any](
	dep1 Executor[D1],
	dep2 Executor[D2],
	factory func(D1, D2, Controller) (T, error),
) MainExecutor[T] {
	return &mainExecutor[T, struct {
		Dep1 D1
		Dep2 D2
	}]{
		executorBase: executorBase[T]{
			kind:         KindMain,
			dependencies: []any{dep1, dep2},
			metadata:     make(map[string]any),
		},
		factory: func(deps struct {
			Dep1 D1
			Dep2 D2
		}, ctrl Controller) (T, error) {
			return factory(deps.Dep1, deps.Dep2, ctrl)
		},
	}
}

// DeriveTyped3 creates an executor with three dependencies of different types
func DeriveTyped3[T any, D1 any, D2 any, D3 any](
	dep1 Executor[D1],
	dep2 Executor[D2],
	dep3 Executor[D3],
	factory func(D1, D2, D3, Controller) (T, error),
) MainExecutor[T] {
	return &mainExecutor[T, struct {
		Dep1 D1
		Dep2 D2
		Dep3 D3
	}]{
		executorBase: executorBase[T]{
			kind:         KindMain,
			dependencies: []any{dep1, dep2, dep3},
			metadata:     make(map[string]any),
		},
		factory: func(deps struct {
			Dep1 D1
			Dep2 D2
			Dep3 D3
		}, ctrl Controller) (T, error) {
			return factory(deps.Dep1, deps.Dep2, deps.Dep3, ctrl)
		},
	}
}

// IsExecutor checks if a value is an executor
func IsExecutor(value any) bool {
	if value == nil {
		return false
	}

	t := reflect.TypeOf(value)
	if t.Kind() != reflect.Ptr {
		return false
	}

	// Check if it implements the Executor interface
	execType := reflect.TypeOf((*Executor[any])(nil)).Elem()
	return reflect.PtrTo(t.Elem()).Implements(execType)
}

// IsMainExecutor checks if a value is a main executor
func IsMainExecutor(value any) bool {
	if !IsExecutor(value) {
		return false
	}

	// Use type assertion to check the kind
	if exec, ok := value.(interface{ Kind() ExecutorKind }); ok {
		return exec.Kind() == KindMain
	}
	return false
}

// IsLazyExecutor checks if a value is a lazy executor
func IsLazyExecutor(value any) bool {
	if !IsExecutor(value) {
		return false
	}

	// Use type assertion to check the kind
	if exec, ok := value.(interface{ Kind() ExecutorKind }); ok {
		return exec.Kind() == KindLazy
	}
	return false
}

// IsReactiveExecutor checks if a value is a reactive executor
func IsReactiveExecutor(value any) bool {
	if !IsExecutor(value) {
		return false
	}

	// Use type assertion to check the kind
	if exec, ok := value.(interface{ Kind() ExecutorKind }); ok {
		return exec.Kind() == KindReactive
	}
	return false
}

// IsStaticExecutor checks if a value is a static executor
func IsStaticExecutor(value any) bool {
	if !IsExecutor(value) {
		return false
	}

	// Use type assertion to check the kind
	if exec, ok := value.(interface{ Kind() ExecutorKind }); ok {
		return exec.Kind() == KindStatic
	}
	return false
}

// CreatePreset creates a preset value for an executor
func CreatePreset[T any](executor Executor[T], value T) Preset[T] {
	return Preset[T]{
		Executor: executor,
		Value:    value,
	}
}

