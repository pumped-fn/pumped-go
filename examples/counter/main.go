package main

import (
	"context"
	"fmt"
	"time"

	"github.com/pumped-fn/pumped-go/pkg/core"
)

func main() {
	// Create a counter executor with no dependencies
	counter := core.Provide(func(ctrl core.Controller) (int, error) {
		fmt.Println("Initializing counter")
		return 0, nil
	})

	// Add metadata to the counter
	counter.WithMeta("name", "counter")

	// Create a doubled executor that depends on counter
	doubled := core.Derive(counter, func(count int, ctrl core.Controller) (int, error) {
		fmt.Println("Computing doubled value")
		return count * 2, nil
	})

	// Create a scope
	scope := core.CreateScope(
		core.WithPresets(core.CreatePreset(counter, 5)),
	)

	// Resolve the counter value
	count, err := scope.Resolve(context.Background(), counter)
	if err != nil {
		fmt.Printf("Error resolving counter: %v\n", err)
		return
	}
	fmt.Printf("Counter value: %d\n", count)

	// Resolve the doubled value
	doubledValue, err := scope.Resolve(context.Background(), doubled)
	if err != nil {
		fmt.Printf("Error resolving doubled: %v\n", err)
		return
	}
	fmt.Printf("Doubled value: %d\n", doubledValue)

	// Update the counter value
	err = scope.Update(context.Background(), counter, 10)
	if err != nil {
		fmt.Printf("Error updating counter: %v\n", err)
		return
	}

	// Resolve the doubled value again
	doubledValue, err = scope.Resolve(context.Background(), doubled)
	if err != nil {
		fmt.Printf("Error resolving doubled: %v\n", err)
		return
	}
	fmt.Printf("Doubled value after update: %d\n", doubledValue)

	// Get an accessor for the counter
	counterAccessor, err := scope.ResolveAccessor(context.Background(), counter)
	if err != nil {
		fmt.Printf("Error resolving counter accessor: %v\n", err)
		return
	}

	// Subscribe to counter changes
	cleanup := counterAccessor.Subscribe(func(value int) {
		fmt.Printf("Counter changed to: %d\n", value)
	})

	// Update the counter using the accessor
	err = counterAccessor.Update(context.Background(), 20)
	if err != nil {
		fmt.Printf("Error updating counter: %v\n", err)
		return
	}

	// Wait for the subscription to be triggered
	time.Sleep(100 * time.Millisecond)

	// Cleanup the subscription
	cleanup()

	// Create a reactive executor
	reactiveDoubled := doubled.Reactive()

	// Get an accessor for the reactive executor
	reactiveAccessor, err := scope.ResolveAccessor(context.Background(), reactiveDoubled)
	if err != nil {
		fmt.Printf("Error resolving reactive accessor: %v\n", err)
		return
	}

	// Subscribe to reactive changes
	reactiveCleanup := reactiveAccessor.Subscribe(func(value int) {
		fmt.Printf("Reactive doubled changed to: %d\n", value)
	})

	// Update the counter, which should trigger the reactive executor
	err = scope.Update(context.Background(), counter, 30)
	if err != nil {
		fmt.Printf("Error updating counter: %v\n", err)
		return
	}

	// Wait for the subscription to be triggered
	time.Sleep(100 * time.Millisecond)

	// Cleanup the subscription
	reactiveCleanup()

	// Dispose the scope
	err = scope.Dispose(context.Background())
	if err != nil {
		fmt.Printf("Error disposing scope: %v\n", err)
		return
	}
}

