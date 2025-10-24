package pumped

import "context"

// Controller provides lifecycle control for an executor's value
type Controller[T any] struct {
	executor *Executor[T]
	scope    *Scope
}

// Get retrieves the latest value (resolves if not cached)
func (c *Controller[T]) Get() (T, error) {
	return Resolve(c.scope, c.executor)
}

// Peek retrieves the cached value without resolving
func (c *Controller[T]) Peek() (T, bool) {
	val, ok := c.scope.cache.Load(c.executor)
	if !ok {
		var zero T
		return zero, false
	}

	typedVal, err := SafeTypeAssertion[T](val)
	if err != nil {
		var zero T
		return zero, false
	}

	return typedVal, true
}

// Update sets a new value and propagates to reactive dependents
func (c *Controller[T]) Update(ctx context.Context, newVal T) error {
	return Update(ctx, c.scope, c.executor, newVal)
}

// Set is an alias for Update
func (c *Controller[T]) Set(ctx context.Context, newVal T) error {
	return c.Update(ctx, newVal)
}

// Release invalidates the cached value
func (c *Controller[T]) Release() error {
	c.scope.cache.Delete(c.executor)
	return nil
}

// Reload invalidates and immediately re-resolves
func (c *Controller[T]) Reload() (T, error) {
	if err := c.Release(); err != nil {
		var zero T
		return zero, err
	}
	return c.Get()
}

// IsCached checks if the value is currently cached
func (c *Controller[T]) IsCached() bool {
	_, ok := c.scope.cache.Load(c.executor)
	return ok
}
