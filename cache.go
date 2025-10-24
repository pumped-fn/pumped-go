package pumped

import (
	"sync"
)

type CacheKey interface{}

type TypeSafeCache[T any] struct {
	data sync.Map
}

func NewTypeSafeCache[T any](initialCapacity int) *TypeSafeCache[T] {
	return &TypeSafeCache[T]{
		data: sync.Map{},
	}
}

func (c *TypeSafeCache[T]) Load(key CacheKey) (T, bool) {
	value, ok := c.data.Load(key)
	if !ok {
		var zero T
		return zero, false
	}
	return value.(T), true
}

func (c *TypeSafeCache[T]) Store(key CacheKey, value T) {
	c.data.Store(key, value)
}

func (c *TypeSafeCache[T]) Delete(key CacheKey) {
	c.data.Delete(key)
}

func (c *TypeSafeCache[T]) Range(fn func(key CacheKey, value T) bool) {
	c.data.Range(func(key, value any) bool {
		return fn(key.(CacheKey), value.(T))
	})
}

func (c *TypeSafeCache[T]) Size() int {
	count := 0
	c.data.Range(func(key, value any) bool {
		count++
		return true
	})
	return count
}

func (c *TypeSafeCache[T]) Clear() {
	c.data.Range(func(key, value any) bool {
		c.data.Delete(key)
		return true
	})
}

func (c *TypeSafeCache[T]) Capacity() int {
	return c.Size()
}
