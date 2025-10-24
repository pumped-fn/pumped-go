package pumped

import (
	"sync"
)

// ReactiveGraph manages reactive dependency relationships with safe traversal
type ReactiveGraph struct {
	// Using adjacency list representation for better memory efficiency
	downstream map[AnyExecutor][]AnyExecutor
	upstream   map[AnyExecutor][]AnyExecutor
	mu         sync.RWMutex

	// For preventing infinite loops during traversal
	visited  map[AnyExecutor]bool
	visiting map[AnyExecutor]bool
}

// NewReactiveGraph creates a new reactive dependency graph
func NewReactiveGraph() *ReactiveGraph {
	return &ReactiveGraph{
		downstream: make(map[AnyExecutor][]AnyExecutor),
		upstream:   make(map[AnyExecutor][]AnyExecutor),
		visited:    make(map[AnyExecutor]bool),
		visiting:   make(map[AnyExecutor]bool),
	}
}

// AddDependency adds a reactive dependency relationship
func (g *ReactiveGraph) AddDependency(dependent AnyExecutor, dependency AnyExecutor) {
	g.mu.Lock()
	defer g.mu.Unlock()

	// Add to downstream (dependency -> dependent)
	g.downstream[dependency] = appendUnique(g.downstream[dependency], dependent)

	// Add to upstream (dependent -> dependency)
	g.upstream[dependent] = appendUnique(g.upstream[dependent], dependency)
}

// RemoveDependency removes a reactive dependency relationship
func (g *ReactiveGraph) RemoveDependency(dependent AnyExecutor, dependency AnyExecutor) {
	g.mu.Lock()
	defer g.mu.Unlock()

	// Remove from downstream
	g.downstream[dependency] = removeElement(g.downstream[dependency], dependent)
	if len(g.downstream[dependency]) == 0 {
		delete(g.downstream, dependency)
	}

	// Remove from upstream
	g.upstream[dependent] = removeElement(g.upstream[dependent], dependency)
	if len(g.upstream[dependent]) == 0 {
		delete(g.upstream, dependent)
	}
}

// FindDependents performs iterative traversal to find all reactive dependents
// This replaces the recursive implementation to prevent stack overflow
func (g *ReactiveGraph) FindDependents(start AnyExecutor) []AnyExecutor {
	g.mu.RLock()
	defer g.mu.RUnlock()

	// Reset visited state
	g.resetVisited()

	// Use explicit stack instead of recursion
	stack := make([]AnyExecutor, 0, 32)
	stack = append(stack, start)

	dependents := make([]AnyExecutor, 0, 32)
	visited := make(map[AnyExecutor]bool, 32)

	for len(stack) > 0 {
		// Pop from stack
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		// Skip if already visited
		if visited[current] {
			continue
		}
		visited[current] = true

		// Add to dependents (but not the start node itself)
		if current != start {
			dependents = append(dependents, current)
		}

		// Get dependents of current node
		if deps, exists := g.downstream[current]; exists {
			// Add all dependents to stack
			for _, dep := range deps {
				if !visited[dep] {
					stack = append(stack, dep)
				}
			}
		}
	}

	return dependents
}

// GetDirectDependents returns only direct dependents (no recursion)
func (g *ReactiveGraph) GetDirectDependents(executor AnyExecutor) []AnyExecutor {
	g.mu.RLock()
	defer g.mu.RUnlock()

	if deps, exists := g.downstream[executor]; exists {
		// Return a copy to prevent external modification
		result := make([]AnyExecutor, len(deps))
		copy(result, deps)
		return result
	}
	return nil
}

// Internal helper methods

func (g *ReactiveGraph) resetVisited() {
	// Clear visited maps
	for k := range g.visited {
		delete(g.visited, k)
	}
	for k := range g.visiting {
		delete(g.visiting, k)
	}
}

// Utility functions for working with slices efficiently

func appendUnique[T comparable](slice []T, item T) []T {
	for _, existing := range slice {
		if existing == item {
			return slice
		}
	}
	return append(slice, item)
}

func removeElement[T comparable](slice []T, item T) []T {
	for i, existing := range slice {
		if existing == item {
			return append(slice[:i], slice[i+1:]...)
		}
	}
	return slice
}
