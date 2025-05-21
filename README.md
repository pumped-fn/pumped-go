# Pumped Go

A Go implementation of the `@pumped-fn/core-next` library, providing a powerful dependency injection and state management system for Go applications.

## Core Concepts

- **Executors**: Functions with dependencies, metadata, and reactivity
  - Types: Main, Lazy, Reactive, Static
- **Scope**: Manages executor lifecycle (caching, resolution, cleanup)
- **Meta**: Attaches metadata to executors

## Installation

```bash
go get github.com/pumped-fn/pumped-go
```

## Quick Reference

### Creation

```go
// Create executor with no dependencies
counter := core.Provide(func(ctrl core.Controller) (int, error) {
    return 0, nil
})

// Create executor with dependencies
doubled := core.Derive(counter, func(count int, ctrl core.Controller) (int, error) {
    return count * 2, nil
})

// Create executor with metadata
counter.WithMeta("name", "counter")
```

### Scope Operations

```go
// Create scope (optionally with presets)
scope := core.CreateScope(
    core.WithPresets(core.CreatePreset(counter, 5)),
)

// Resolve value
count, err := scope.Resolve(context.Background(), counter)

// Update value
err = scope.Update(context.Background(), counter, 10)
// Or with function
err = scope.UpdateFunc(context.Background(), counter, func(current int) int {
    return current + 1
})

// Subscribe to changes
cleanup := scope.OnUpdate(counter, func(accessor core.Accessor[int]) {
    fmt.Printf("New value: %d\n", accessor.Get())
})
```

### Accessor Operations

```go
// Get accessor
accessor, err := scope.ResolveAccessor(context.Background(), counter)

// Get current value
value := accessor.Get()

// Update value
err = accessor.Update(context.Background(), 5)

// Subscribe to changes
cleanup := accessor.Subscribe(func(value int) {
    fmt.Printf("New value: %d\n", value)
})
```

### Variants

```go
// Lazy (doesn't resolve immediately)
lazyCounter := counter.Lazy()

// Reactive (updates when dependencies change)
reactiveCounter := counter.Reactive()

// Static (doesn't track for reactivity)
staticCounter := counter.Static()
```

## Common Patterns

- **State management**: `core.Provide(func(ctrl core.Controller) (State, error) { return initialState, nil })`
- **Derived state**: `core.Derive(state, func(s State, ctrl core.Controller) (TransformedState, error) { return transform(s), nil })`
- **Controllers**: `core.Derive(state.Static(), func(stateCtl core.Accessor[State], ctrl core.Controller) (Controller, error) { return Controller{Update: func() { stateCtl.Update(ctx, newState) }}, nil })`
- **Side effects**: `core.Derive([]Executor{dep1, dep2}, func(deps []any, ctrl core.Controller) (any, error) { cleanup := setup(); ctrl.Cleanup(cleanup); return nil, nil })`
- **Metadata**: `executor.WithMeta("key", value)`

## Tips

- Use `.Reactive()` for values that should update automatically
- Use `.Static()` for controllers that update other values
- Use `ctrl.Cleanup()` to register cleanup functions
- Prefer `accessor.Update()` over direct mutation

## Example

```go
package main

import (
	"context"
	"fmt"

	"github.com/pumped-fn/pumped-go/pkg/core"
)

func main() {
	// Create a counter executor
	counter := core.Provide(func(ctrl core.Controller) (int, error) {
		return 0, nil
	})

	// Create a doubled executor
	doubled := core.Derive(counter, func(count int, ctrl core.Controller) (int, error) {
		return count * 2, nil
	})

	// Create a scope
	scope := core.CreateScope()

	// Resolve the counter value
	count, err := scope.Resolve(context.Background(), counter)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	fmt.Printf("Counter: %d\n", count)

	// Resolve the doubled value
	doubledValue, err := scope.Resolve(context.Background(), doubled)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	fmt.Printf("Doubled: %d\n", doubledValue)

	// Update the counter
	err = scope.Update(context.Background(), counter, 10)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	// Resolve the doubled value again
	doubledValue, err = scope.Resolve(context.Background(), doubled)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	fmt.Printf("Doubled after update: %d\n", doubledValue)
}
```

## License

MIT

