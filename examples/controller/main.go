package main

import (
	"context"
	"fmt"
	"time"

	"github.com/pumped-fn/pumped-go/pkg/core"
)

// TodoItem represents a todo item
type TodoItem struct {
	ID        int
	Title     string
	Completed bool
}

// TodoState represents the state of the todo list
type TodoState struct {
	Items []TodoItem
	Filter string // "all", "active", "completed"
}

// TodoController provides methods to manipulate the todo list
type TodoController struct {
	AddTodo     func(title string) error
	ToggleTodo  func(id int) error
	RemoveTodo  func(id int) error
	SetFilter   func(filter string) error
	ClearCompleted func() error
}

func main() {
	// Create a todo state executor
	todoState := core.Provide(func(ctrl core.Controller) (TodoState, error) {
		fmt.Println("Initializing todo state")
		return TodoState{
			Items: []TodoItem{
				{ID: 1, Title: "Learn Go", Completed: false},
				{ID: 2, Title: "Learn Pumped Go", Completed: false},
			},
			Filter: "all",
		}, nil
	})

	// Create a filtered todos executor that depends on the state
	filteredTodos := core.Derive(todoState, func(state TodoState, ctrl core.Controller) ([]TodoItem, error) {
		fmt.Println("Computing filtered todos")
		
		if state.Filter == "all" {
			return state.Items, nil
		}
		
		var filtered []TodoItem
		for _, item := range state.Items {
			if (state.Filter == "active" && !item.Completed) || 
			   (state.Filter == "completed" && item.Completed) {
				filtered = append(filtered, item)
			}
		}
		
		return filtered, nil
	})

	// Create a todo controller that can update the state
	todoController := core.Derive(todoState.Static(), func(stateAccessor core.Accessor[TodoState], ctrl core.Controller) (TodoController, error) {
		fmt.Println("Creating todo controller")
		
		return TodoController{
			AddTodo: func(title string) error {
				state := stateAccessor.Get()
				
				// Find the highest ID
				highestID := 0
				for _, item := range state.Items {
					if item.ID > highestID {
						highestID = item.ID
					}
				}
				
				// Add the new todo
				newTodo := TodoItem{
					ID:        highestID + 1,
					Title:     title,
					Completed: false,
				}
				
				state.Items = append(state.Items, newTodo)
				
				return stateAccessor.Update(context.Background(), state)
			},
			
			ToggleTodo: func(id int) error {
				state := stateAccessor.Get()
				
				for i, item := range state.Items {
					if item.ID == id {
						state.Items[i].Completed = !state.Items[i].Completed
						break
					}
				}
				
				return stateAccessor.Update(context.Background(), state)
			},
			
			RemoveTodo: func(id int) error {
				state := stateAccessor.Get()
				
				for i, item := range state.Items {
					if item.ID == id {
						state.Items = append(state.Items[:i], state.Items[i+1:]...)
						break
					}
				}
				
				return stateAccessor.Update(context.Background(), state)
			},
			
			SetFilter: func(filter string) error {
				state := stateAccessor.Get()
				state.Filter = filter
				return stateAccessor.Update(context.Background(), state)
			},
			
			ClearCompleted: func() error {
				state := stateAccessor.Get()
				
				var active []TodoItem
				for _, item := range state.Items {
					if !item.Completed {
						active = append(active, item)
					}
				}
				
				state.Items = active
				return stateAccessor.Update(context.Background(), state)
			},
		}, nil
	})

	// Create a logger that logs when the filtered todos change
	todoLogger := core.Derive(filteredTodos.Reactive(), func(todos []TodoItem, ctrl core.Controller) (any, error) {
		fmt.Println("Setting up todo logger")
		
		// Log the initial todos
		fmt.Printf("Todos (%d items):\n", len(todos))
		for _, todo := range todos {
			status := " "
			if todo.Completed {
				status = "✓"
			}
			fmt.Printf("  [%s] %d. %s\n", status, todo.ID, todo.Title)
		}
		
		// Register cleanup
		ctrl.Cleanup(func() error {
			fmt.Println("Cleaning up todo logger")
			return nil
		})
		
		return nil, nil
	})

	// Create a scope
	scope := core.CreateScope()

	// Resolve the controller
	controller, err := scope.Resolve(context.Background(), todoController)
	if err != nil {
		fmt.Printf("Error resolving controller: %v\n", err)
		return
	}

	// Resolve the logger to set it up
	_, err = scope.Resolve(context.Background(), todoLogger)
	if err != nil {
		fmt.Printf("Error resolving logger: %v\n", err)
		return
	}

	// Add a new todo
	fmt.Println("\nAdding a new todo...")
	err = controller.AddTodo("Build a todo app")
	if err != nil {
		fmt.Printf("Error adding todo: %v\n", err)
		return
	}
	time.Sleep(100 * time.Millisecond)

	// Toggle a todo
	fmt.Println("\nCompleting 'Learn Go'...")
	err = controller.ToggleTodo(1)
	if err != nil {
		fmt.Printf("Error toggling todo: %v\n", err)
		return
	}
	time.Sleep(100 * time.Millisecond)

	// Filter to show only active todos
	fmt.Println("\nFiltering to show only active todos...")
	err = controller.SetFilter("active")
	if err != nil {
		fmt.Printf("Error setting filter: %v\n", err)
		return
	}
	time.Sleep(100 * time.Millisecond)

	// Filter to show only completed todos
	fmt.Println("\nFiltering to show only completed todos...")
	err = controller.SetFilter("completed")
	if err != nil {
		fmt.Printf("Error setting filter: %v\n", err)
		return
	}
	time.Sleep(100 * time.Millisecond)

	// Show all todos again
	fmt.Println("\nShowing all todos...")
	err = controller.SetFilter("all")
	if err != nil {
		fmt.Printf("Error setting filter: %v\n", err)
		return
	}
	time.Sleep(100 * time.Millisecond)

	// Clear completed todos
	fmt.Println("\nClearing completed todos...")
	err = controller.ClearCompleted()
	if err != nil {
		fmt.Printf("Error clearing completed todos: %v\n", err)
		return
	}
	time.Sleep(100 * time.Millisecond)

	// Dispose the scope
	err = scope.Dispose(context.Background())
	if err != nil {
		fmt.Printf("Error disposing scope: %v\n", err)
		return
	}
}

