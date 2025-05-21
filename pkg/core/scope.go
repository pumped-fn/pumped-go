package core

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
)

var (
	ErrScopeDisposed     = errors.New("scope is disposed")
	ErrExecutorNotFound  = errors.New("executor not found")
	ErrExecutorNotResolved = errors.New("executor not resolved")
	ErrInvalidExecutorType = errors.New("invalid executor type")
)

// cacheEntry represents a cached value in the scope
type cacheEntry struct {
	accessor any
	value    any
	err      error
	resolving bool
	wg       *sync.WaitGroup
}

// scopeImpl implements the Scope interface
type scopeImpl struct {
	mu       sync.RWMutex
	cache    map[any]*cacheEntry
	cleanups map[any][]Cleanup
	onUpdate map[any][]any
	disposed bool
}

// controllerImpl implements the Controller interface
type controllerImpl struct {
	scope    *scopeImpl
	executor any
	cleanups []Cleanup
}

// Cleanup registers a cleanup function
func (c *controllerImpl) Cleanup(cleanup Cleanup) {
	c.cleanups = append(c.cleanups, cleanup)
}

// Release releases resources
func (c *controllerImpl) Release(ctx context.Context) error {
	return c.scope.releaseExecutor(ctx, c.executor)
}

// Scope returns the scope this controller belongs to
func (c *controllerImpl) Scope() Scope {
	return c.scope
}

// accessorImpl implements the Accessor interface
type accessorImpl[T any] struct {
	scope    *scopeImpl
	executor Executor[T]
	mu       sync.RWMutex
}

// Get returns the current value
func (a *accessorImpl[T]) Get() T {
	a.mu.RLock()
	defer a.mu.RUnlock()

	a.scope.mu.RLock()
	defer a.scope.mu.RUnlock()

	if a.scope.disposed {
		var zero T
		return zero
	}

	entry, ok := a.scope.cache[a.executor]
	if !ok || entry.resolving {
		var zero T
		return zero
	}

	if entry.err != nil {
		var zero T
		return zero
	}

	return entry.value.(T)
}

// Resolve ensures the value is resolved, optionally forcing re-resolution
func (a *accessorImpl[T]) Resolve(ctx context.Context, force bool) (T, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	return a.scope.resolveExecutor(ctx, a.executor, force)
}

// Update updates the value
func (a *accessorImpl[T]) Update(ctx context.Context, value T) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	return a.scope.updateExecutor(ctx, a.executor, value)
}

// UpdateFunc updates the value using a function
func (a *accessorImpl[T]) UpdateFunc(ctx context.Context, fn func(T) T) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.scope.mu.RLock()
	entry, ok := a.scope.cache[a.executor]
	if !ok || entry.resolving || entry.err != nil {
		a.scope.mu.RUnlock()
		return ErrExecutorNotResolved
	}

	currentValue := entry.value.(T)
	a.scope.mu.RUnlock()

	newValue := fn(currentValue)
	return a.scope.updateExecutor(ctx, a.executor, newValue)
}

// Subscribe registers a callback for value changes
func (a *accessorImpl[T]) Subscribe(callback func(T)) Cleanup {
	return a.scope.OnUpdate(a.executor, func(acc Accessor[T]) {
		callback(acc.Get())
	})
}

// Release releases resources associated with this accessor
func (a *accessorImpl[T]) Release(ctx context.Context) error {
	return a.scope.Release(ctx, a.executor)
}

// Metadata returns the metadata associated with this accessor
func (a *accessorImpl[T]) Metadata() map[string]any {
	return a.executor.Metadata()
}

// CreateScope creates a new scope
func CreateScope(options ...ScopeOption) Scope {
	s := &scopeImpl{
		cache:    make(map[any]*cacheEntry),
		cleanups: make(map[any][]Cleanup),
		onUpdate: make(map[any][]any),
	}

	for _, option := range options {
		option(s)
	}

	return s
}

// WithPresets adds presets to a scope
func WithPresets(presets ...any) ScopeOption {
	return func(s *scopeImpl) {
		for _, p := range presets {
			// Use reflection to handle the generic Preset type
			v := reflect.ValueOf(p)
			if v.Kind() != reflect.Struct {
				continue
			}

			execField := v.FieldByName("Executor")
			valueField := v.FieldByName("Value")
			if !execField.IsValid() || !valueField.IsValid() {
				continue
			}

			executor := execField.Interface()
			value := valueField.Interface()

			// Create an accessor for the executor
			accessor := s.createAccessor(executor)
			
			// Cache the value
			s.cache[executor] = &cacheEntry{
				accessor: accessor,
				value:    value,
				resolving: false,
			}
		}
	}
}

// Get returns the current value of an executor
func (s *scopeImpl) Get[T any](executor Executor[T]) (T, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.disposed {
		var zero T
		return zero, ErrScopeDisposed
	}

	entry, ok := s.cache[executor]
	if !ok {
		var zero T
		return zero, ErrExecutorNotFound
	}

	if entry.resolving {
		var zero T
		return zero, ErrExecutorNotResolved
	}

	if entry.err != nil {
		var zero T
		return zero, entry.err
	}

	return entry.value.(T), nil
}

// Resolve ensures an executor is resolved and returns its value
func (s *scopeImpl) Resolve[T any](ctx context.Context, executor Executor[T]) (T, error) {
	return s.resolveExecutor(ctx, executor, false)
}

// GetAccessor returns an accessor for an executor
func (s *scopeImpl) GetAccessor[T any](executor Executor[T]) (Accessor[T], error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.disposed {
		var zero Accessor[T]
		return zero, ErrScopeDisposed
	}

	entry, ok := s.cache[executor]
	if !ok {
		accessor := s.createAccessor(executor)
		return accessor.(Accessor[T]), nil
	}

	return entry.accessor.(Accessor[T]), nil
}

// ResolveAccessor ensures an executor is resolved and returns its accessor
func (s *scopeImpl) ResolveAccessor[T any](ctx context.Context, executor Executor[T]) (Accessor[T], error) {
	accessor, err := s.GetAccessor(executor)
	if err != nil {
		return nil, err
	}

	_, err = accessor.Resolve(ctx, false)
	if err != nil {
		return nil, err
	}

	return accessor, nil
}

// Update updates the value of an executor
func (s *scopeImpl) Update[T any](ctx context.Context, executor Executor[T], value T) error {
	return s.updateExecutor(ctx, executor, value)
}

// UpdateFunc updates the value of an executor using a function
func (s *scopeImpl) UpdateFunc[T any](ctx context.Context, executor Executor[T], fn func(T) T) error {
	s.mu.RLock()
	entry, ok := s.cache[executor]
	if !ok || entry.resolving || entry.err != nil {
		s.mu.RUnlock()
		return ErrExecutorNotResolved
	}

	currentValue := entry.value.(T)
	s.mu.RUnlock()

	newValue := fn(currentValue)
	return s.updateExecutor(ctx, executor, newValue)
}

// Reset resets an executor
func (s *scopeImpl) Reset[T any](ctx context.Context, executor Executor[T]) error {
	if err := s.Release(ctx, executor); err != nil {
		return err
	}
	_, err := s.Resolve(ctx, executor)
	return err
}

// Release releases resources associated with an executor
func (s *scopeImpl) Release[T any](ctx context.Context, executor Executor[T]) error {
	return s.releaseExecutor(ctx, executor)
}

// Dispose releases all resources
func (s *scopeImpl) Dispose(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.disposed {
		return nil
	}

	// Copy keys to avoid concurrent map iteration
	executors := make([]any, 0, len(s.cache))
	for exec := range s.cache {
		executors = append(executors, exec)
	}

	// Release all executors
	for _, exec := range executors {
		if err := s.releaseExecutorLocked(ctx, exec); err != nil {
			// Continue releasing other executors even if one fails
			continue
		}
	}

	s.disposed = true
	s.cache = nil
	s.cleanups = nil
	s.onUpdate = nil

	return nil
}

// OnUpdate registers a callback for when an executor's value changes
func (s *scopeImpl) OnUpdate[T any](executor Executor[T], callback func(Accessor[T])) Cleanup {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.disposed {
		return func() error { return nil }
	}

	if s.onUpdate == nil {
		s.onUpdate = make(map[any][]any)
	}

	callbacks := s.onUpdate[executor]
	s.onUpdate[executor] = append(callbacks, callback)

	return func() error {
		s.mu.Lock()
		defer s.mu.Unlock()

		if s.disposed {
			return nil
		}

		callbacks := s.onUpdate[executor]
		if callbacks == nil {
			return nil
		}

		// Find and remove the callback
		for i, cb := range callbacks {
			if fmt.Sprintf("%p", cb) == fmt.Sprintf("%p", callback) {
				s.onUpdate[executor] = append(callbacks[:i], callbacks[i+1:]...)
				break
			}
		}

		if len(s.onUpdate[executor]) == 0 {
			delete(s.onUpdate, executor)
		}

		return nil
	}
}

// createAccessor creates an accessor for an executor
func (s *scopeImpl) createAccessor(executor any) any {
	// Use reflection to create a typed accessor
	execType := reflect.TypeOf(executor)
	if execType.Kind() != reflect.Ptr {
		return nil
	}

	// Get the type parameter T from Executor[T]
	typeParam := execType.Elem()
	for i := 0; i < typeParam.NumMethod(); i++ {
		method := typeParam.Method(i)
		if method.Name == "Kind" {
			// Found the Executor interface, extract its type parameter
			typeParam = method.Type.Out(0)
			break
		}
	}

	// Create a new accessorImpl with the correct type parameter
	accessorType := reflect.TypeOf((*accessorImpl[any])(nil)).Elem()
	accessorPtrType := reflect.PtrTo(accessorType)

	// Create a new accessor instance
	accessorValue := reflect.New(accessorType).Elem()
	accessorValue.FieldByName("scope").Set(reflect.ValueOf(s))
	accessorValue.FieldByName("executor").Set(reflect.ValueOf(executor))

	return accessorValue.Addr().Interface()
}

// resolveExecutor resolves an executor and returns its value
func (s *scopeImpl) resolveExecutor[T any](ctx context.Context, executor Executor[T], force bool) (T, error) {
	s.mu.Lock()

	if s.disposed {
		s.mu.Unlock()
		var zero T
		return zero, ErrScopeDisposed
	}

	// Check if already resolved
	entry, ok := s.cache[executor]
	if ok && !force {
		if entry.resolving {
			// Wait for resolution to complete
			wg := entry.wg
			s.mu.Unlock()
			wg.Wait()
			return s.Get(executor)
		}

		if entry.err != nil {
			s.mu.Unlock()
			var zero T
			return zero, entry.err
		}

		value := entry.value.(T)
		s.mu.Unlock()
		return value, nil
	}

	// Start resolution
	wg := &sync.WaitGroup{}
	wg.Add(1)
	
	accessor := s.createAccessor(executor).(Accessor[T])
	
	s.cache[executor] = &cacheEntry{
		accessor:  accessor,
		resolving: true,
		wg:        wg,
	}
	
	s.mu.Unlock()

	// Resolve dependencies and execute factory
	var result T
	var err error

	defer func() {
		s.mu.Lock()
		if err != nil {
			s.cache[executor] = &cacheEntry{
				accessor: accessor,
				err:      err,
				resolving: false,
				wg:       nil,
			}
		} else {
			s.cache[executor] = &cacheEntry{
				accessor: accessor,
				value:    result,
				resolving: false,
				wg:       nil,
			}
		}
		wg.Done()
		s.mu.Unlock()
	}()

	// Handle different executor kinds
	switch executor.Kind() {
	case KindMain:
		mainExec := executor.(MainExecutor[T])
		result, err = s.resolveMainExecutor(ctx, mainExec)
	case KindLazy:
		// Lazy executors return an accessor without resolving
		var zero T
		return zero, errors.New("lazy executors must be accessed through ResolveAccessor")
	case KindReactive:
		// For reactive executors, resolve the main executor
		if reactiveExec, ok := executor.(interface{ main() MainExecutor[T] }); ok {
			mainExec := reactiveExec.main()
			result, err = s.resolveMainExecutor(ctx, mainExec)
			
			// Register for updates from dependencies
			if err == nil {
				for _, dep := range mainExec.Dependencies() {
					s.registerDependencyUpdate(dep, executor)
				}
			}
		} else {
			err = ErrInvalidExecutorType
		}
	case KindStatic:
		// Static executors return an accessor
		var zero T
		return zero, errors.New("static executors must be accessed through ResolveAccessor")
	default:
		err = fmt.Errorf("unknown executor kind: %s", executor.Kind())
	}

	return result, err
}

// resolveMainExecutor resolves a main executor
func (s *scopeImpl) resolveMainExecutor[T any](ctx context.Context, executor MainExecutor[T]) (T, error) {
	// Create controller
	controller := &controllerImpl{
		scope:    s,
		executor: executor,
	}

	// Resolve dependencies
	deps, err := s.resolveDependencies(ctx, executor.Dependencies())
	if err != nil {
		var zero T
		return zero, err
	}

	// Execute factory
	// This is a bit tricky because we need to call the factory with the correct type
	// We'll use reflection to call the factory method
	execValue := reflect.ValueOf(executor)
	factoryMethod := execValue.MethodByName("factory")
	if !factoryMethod.IsValid() {
		var zero T
		return zero, errors.New("executor has no factory method")
	}

	args := []reflect.Value{
		reflect.ValueOf(deps),
		reflect.ValueOf(controller),
	}

	results := factoryMethod.Call(args)
	if len(results) != 2 {
		var zero T
		return zero, errors.New("factory method must return (T, error)")
	}

	// Check for error
	if !results[1].IsNil() {
		var zero T
		return zero, results[1].Interface().(error)
	}

	return results[0].Interface().(T), nil
}

// resolveDependencies resolves all dependencies of an executor
func (s *scopeImpl) resolveDependencies(ctx context.Context, dependencies []any) (any, error) {
	if len(dependencies) == 0 {
		return struct{}{}, nil
	}

	if len(dependencies) == 1 {
		// Single dependency
		dep := dependencies[0]
		if !IsExecutor(dep) {
			return nil, fmt.Errorf("dependency is not an executor: %T", dep)
		}

		// Use reflection to call Resolve with the correct type
		scopeValue := reflect.ValueOf(s)
		resolveMethod := scopeValue.MethodByName("Resolve")
		
		// Create a method with the correct type parameter
		execType := reflect.TypeOf(dep)
		typeParam := execType.Elem()
		
		method := resolveMethod.MethodByName(typeParam.Name())
		args := []reflect.Value{
			reflect.ValueOf(ctx),
			reflect.ValueOf(dep),
		}
		
		results := method.Call(args)
		if len(results) != 2 {
			return nil, errors.New("Resolve method must return (T, error)")
		}
		
		// Check for error
		if !results[1].IsNil() {
			return nil, results[1].Interface().(error)
		}
		
		return results[0].Interface(), nil
	}

	// Multiple dependencies
	if reflect.TypeOf(dependencies[0]).Kind() == reflect.Map {
		// Map dependencies
		result := reflect.MakeMap(reflect.TypeOf(dependencies[0]))
		
		for _, dep := range dependencies {
			mapValue := reflect.ValueOf(dep)
			for _, key := range mapValue.MapKeys() {
				depValue := mapValue.MapIndex(key)
				
				// Resolve the dependency
				resolved, err := s.resolveDependencies(ctx, []any{depValue.Interface()})
				if err != nil {
					return nil, err
				}
				
				result.SetMapIndex(key, reflect.ValueOf(resolved))
			}
		}
		
		return result.Interface(), nil
	}

	// Array dependencies
	result := reflect.MakeSlice(reflect.SliceOf(reflect.TypeOf(dependencies[0])), len(dependencies), len(dependencies))
	
	for i, dep := range dependencies {
		// Resolve the dependency
		resolved, err := s.resolveDependencies(ctx, []any{dep})
		if err != nil {
			return nil, err
		}
		
		result.Index(i).Set(reflect.ValueOf(resolved))
	}
	
	return result.Interface(), nil
}

// updateExecutor updates the value of an executor
func (s *scopeImpl) updateExecutor[T any](ctx context.Context, executor Executor[T], value T) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.disposed {
		return ErrScopeDisposed
	}

	// Run cleanups
	if err := s.runCleanups(ctx, executor); err != nil {
		return err
	}

	// Get accessor
	entry, ok := s.cache[executor]
	if !ok {
		accessor := s.createAccessor(executor)
		entry = &cacheEntry{
			accessor: accessor,
			value:    value,
			resolving: false,
		}
		s.cache[executor] = entry
	} else {
		entry.value = value
		entry.err = nil
		entry.resolving = false
	}

	// Trigger updates
	go s.triggerUpdates(ctx, executor)

	return nil
}

// releaseExecutor releases resources associated with an executor
func (s *scopeImpl) releaseExecutor(ctx context.Context, executor any) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.releaseExecutorLocked(ctx, executor)
}

// releaseExecutorLocked releases resources associated with an executor (with lock held)
func (s *scopeImpl) releaseExecutorLocked(ctx context.Context, executor any) error {
	if s.disposed {
		return ErrScopeDisposed
	}

	// Run cleanups
	if err := s.runCleanups(ctx, executor); err != nil {
		return err
	}

	// Release dependent executors
	if callbacks := s.onUpdate[executor]; callbacks != nil {
		for _, callback := range callbacks {
			if IsExecutor(callback) {
				s.releaseExecutorLocked(ctx, callback)
			}
		}
		delete(s.onUpdate, executor)
	}

	// Remove from cache
	delete(s.cache, executor)
	delete(s.cleanups, executor)

	return nil
}

// runCleanups runs all cleanup functions for an executor
func (s *scopeImpl) runCleanups(ctx context.Context, executor any) error {
	cleanups := s.cleanups[executor]
	if cleanups == nil {
		return nil
	}

	// Run cleanups in reverse order
	for i := len(cleanups) - 1; i >= 0; i-- {
		if err := cleanups[i](); err != nil {
			return err
		}
	}

	return nil
}

// registerDependencyUpdate registers for updates from a dependency
func (s *scopeImpl) registerDependencyUpdate(dependency any, dependent any) {
	if !IsExecutor(dependency) {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.disposed {
		return
	}

	if s.onUpdate == nil {
		s.onUpdate = make(map[any][]any)
	}

	callbacks := s.onUpdate[dependency]
	s.onUpdate[dependency] = append(callbacks, dependent)
}

// triggerUpdates triggers updates for all dependents of an executor
func (s *scopeImpl) triggerUpdates(ctx context.Context, executor any) {
	s.mu.RLock()
	callbacks := s.onUpdate[executor]
	if callbacks == nil {
		s.mu.RUnlock()
		return
	}
	callbacksCopy := make([]any, len(callbacks))
	copy(callbacksCopy, callbacks)
	s.mu.RUnlock()

	for _, callback := range callbacksCopy {
		if IsExecutor(callback) {
			// If the callback is an executor, reset it
			s.resetExecutor(ctx, callback)
		} else {
			// Otherwise, call the callback function
			s.callUpdateCallback(ctx, executor, callback)
		}
	}
}

// resetExecutor resets an executor
func (s *scopeImpl) resetExecutor(ctx context.Context, executor any) {
	// Use reflection to call Reset with the correct type
	scopeValue := reflect.ValueOf(s)
	resetMethod := scopeValue.MethodByName("Reset")
	
	// Create a method with the correct type parameter
	execType := reflect.TypeOf(executor)
	typeParam := execType.Elem()
	
	method := resetMethod.MethodByName(typeParam.Name())
	args := []reflect.Value{
		reflect.ValueOf(ctx),
		reflect.ValueOf(executor),
	}
	
	method.Call(args)
}

// callUpdateCallback calls an update callback
func (s *scopeImpl) callUpdateCallback(ctx context.Context, executor any, callback any) {
	// Get the accessor
	s.mu.RLock()
	entry, ok := s.cache[executor]
	if !ok || entry.resolving || entry.err != nil {
		s.mu.RUnlock()
		return
	}
	accessor := entry.accessor
	s.mu.RUnlock()

	// Call the callback
	callbackValue := reflect.ValueOf(callback)
	args := []reflect.Value{reflect.ValueOf(accessor)}
	callbackValue.Call(args)
}

