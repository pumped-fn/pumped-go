package extensions

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sort"
	"strings"

	pumped "github.com/pumped-fn/pumped-go"
	"github.com/m1gwings/treedrawer/tree"
)

// GraphDebugExtension logs dependency graph visualization when errors occur.
//
// Usage:
//
//	// Human-readable formatted output (with line breaks)
//	handler := extensions.NewHumanHandler(os.Stdout, slog.LevelError)
//	ext := extensions.NewGraphDebugExtension(handler)
//
//	// Structured JSON logging (compact, machine-readable)
//	handler := slog.NewJSONHandler(os.Stdout, nil)
//	ext := extensions.NewGraphDebugExtension(handler)
//
//	// Silent (for testing)
//	ext := extensions.NewGraphDebugExtension(extensions.NewSilentHandler())
//
// The extension logs at ERROR level for both resolution errors and flow panics.
type GraphDebugExtension struct {
	pumped.BaseExtension
	nameTag pumped.Tag[string]

	// Track executors as they're resolved
	resolvedExecutors map[pumped.AnyExecutor]bool
	failedExecutors   map[pumped.AnyExecutor]error
	logger            *slog.Logger
}

// NewGraphDebugExtension creates a new graph debug extension.
// logHandler: slog.Handler for logging (use HumanHandler for formatted output, or any other slog.Handler)
func NewGraphDebugExtension(logHandler slog.Handler) *GraphDebugExtension {
	logger := slog.New(logHandler)
	return &GraphDebugExtension{
		BaseExtension:     pumped.NewBaseExtension("graph-debug"),
		nameTag:           pumped.NewTag[string]("executor.name"),
		resolvedExecutors: make(map[pumped.AnyExecutor]bool),
		failedExecutors:   make(map[pumped.AnyExecutor]error),
		logger:            logger,
	}
}

// Wrap tracks operations for debugging
func (e *GraphDebugExtension) Wrap(ctx context.Context, next func() (any, error), op *pumped.Operation) (any, error) {
	result, err := next()

	if err == nil && op.Kind == pumped.OpResolve {
		e.resolvedExecutors[op.Executor] = true
	} else if err != nil && op.Kind == pumped.OpResolve {
		e.failedExecutors[op.Executor] = err
	}

	return result, err
}

// OnError logs the dependency graph when resolution fails
func (e *GraphDebugExtension) OnError(err error, op *pumped.Operation, scope *pumped.Scope) {
	execName := e.getExecutorName(op.Executor)
	graphOutput := e.formatDependencyGraph(scope, op.Executor, err)

	e.logger.Error("Dependency Resolution Error",
		"executor", execName,
		"error", err.Error(),
		"operation", string(op.Kind),
		"dependency_graph", graphOutput,
	)
}

// OnFlowPanic logs context when flow panics
func (e *GraphDebugExtension) OnFlowPanic(execCtx *pumped.ExecutionCtx, recovered any, stack []byte) error {
	attrs := []any{
		"panic", fmt.Sprintf("%v", recovered),
		"stack_trace", string(stack),
	}

	if flowName, ok := execCtx.Get(pumped.FlowName()); ok {
		attrs = append(attrs, "flow", flowName)
	}

	e.logger.Error("Flow Panic", attrs...)

	return nil // Don't suppress the error
}

// tryFormatHorizontalTree attempts to render the dependency graph as a horizontal tree using treedrawer
func (e *GraphDebugExtension) tryFormatHorizontalTree(graph map[pumped.AnyExecutor][]pumped.AnyExecutor, failedExecutor pumped.AnyExecutor) string {
	// Build reverse map (child -> parents) to find roots
	parents := make(map[pumped.AnyExecutor][]pumped.AnyExecutor)
	allNodes := make(map[pumped.AnyExecutor]bool)

	for parent, children := range graph {
		allNodes[parent] = true
		for _, child := range children {
			allNodes[child] = true
			parents[child] = append(parents[child], parent)
		}
	}

	// Find root nodes (no parents)
	var roots []pumped.AnyExecutor
	for node := range allNodes {
		if len(parents[node]) == 0 {
			roots = append(roots, node)
		}
	}

	// Sort roots by name for deterministic output
	sort.Slice(roots, func(i, j int) bool {
		return e.getExecutorName(roots[i]) < e.getExecutorName(roots[j])
	})

	// If no clear root, return empty (fallback to vertical)
	if len(roots) == 0 {
		return ""
	}

	// Use only the first root for tree visualization (or combine multiple roots)
	var rootNode *tree.Tree
	if len(roots) == 1 {
		rootNode = e.buildTree(roots[0], graph, failedExecutor, make(map[pumped.AnyExecutor]bool))
	} else {
		// Multiple roots: create a virtual root
		rootNode = tree.NewTree(tree.NodeString("Dependencies"))
		for _, root := range roots {
			childTree := e.buildTree(root, graph, failedExecutor, make(map[pumped.AnyExecutor]bool))
			if childTree != nil {
				// Manually add the child's structure
				e.addTreeAsChild(rootNode, childTree)
			}
		}
	}

	if rootNode == nil {
		return ""
	}

	return rootNode.String()
}

// buildTree recursively builds a tree structure from the dependency graph
func (e *GraphDebugExtension) buildTree(executor pumped.AnyExecutor, graph map[pumped.AnyExecutor][]pumped.AnyExecutor, failedExecutor pumped.AnyExecutor, visited map[pumped.AnyExecutor]bool) *tree.Tree {
	// Prevent cycles
	if visited[executor] {
		return nil
	}
	visited[executor] = true

	// Create node label with status
	label := e.getExecutorName(executor)
	if executor == failedExecutor {
		label += " ❌"
	} else if e.resolvedExecutors[executor] {
		label += " ✓"
	}

	// Create tree node
	node := tree.NewTree(tree.NodeString(label))

	// Add children (dependents)
	if children, ok := graph[executor]; ok {
		// Sort children for deterministic order
		sortedChildren := make([]pumped.AnyExecutor, len(children))
		copy(sortedChildren, children)
		sort.Slice(sortedChildren, func(i, j int) bool {
			return e.getExecutorName(sortedChildren[i]) < e.getExecutorName(sortedChildren[j])
		})

		for _, child := range sortedChildren {
			childTree := e.buildTree(child, graph, failedExecutor, visited)
			if childTree != nil {
				e.addTreeAsChild(node, childTree)
			}
		}
	}

	return node
}

// addTreeAsChild adds a tree as a child to another tree node
func (e *GraphDebugExtension) addTreeAsChild(parent *tree.Tree, child *tree.Tree) {
	// Get the child's value and create a new child node
	childVal := child.Val()
	newChild := parent.AddChild(childVal)

	// Recursively add all of child's children to newChild
	for _, grandchild := range child.Children() {
		e.addTreeAsChild(newChild, grandchild)
	}
}

func (e *GraphDebugExtension) formatDependencyGraph(scope *pumped.Scope, failedExecutor pumped.AnyExecutor, failedErr error) string {
	var sb strings.Builder
	graph := scope.ExportDependencyGraph()

	if len(graph) == 0 {
		sb.WriteString("\n(empty - no reactive dependencies tracked)")
		return sb.String()
	}

	// Try horizontal tree format first
	horizontalTree := e.tryFormatHorizontalTree(graph, failedExecutor)
	if horizontalTree != "" {
		sb.WriteString("\n")
		sb.WriteString(horizontalTree)
		sb.WriteString("\n")
	}

	sb.WriteString("\nDetailed View:\n")

	// Sort executors by name for deterministic output
	type sortEntry struct {
		parent   pumped.AnyExecutor
		name     string
		children []pumped.AnyExecutor
	}

	entries := make([]sortEntry, 0, len(graph))
	for parent, children := range graph {
		entries = append(entries, sortEntry{
			parent:   parent,
			name:     e.getExecutorName(parent),
			children: children,
		})
	}

	// Sort by name for consistent output
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].name < entries[j].name
	})

	// Format all dependencies
	for _, entry := range entries {
		parent := entry.parent
		children := entry.children
		parentName := entry.name

		// Mark parent status
		parentStatus := ""
		if e.resolvedExecutors[parent] {
			parentStatus = " ✓"
		} else if _, failed := e.failedExecutors[parent]; failed {
			parentStatus = " ❌"
		}

		if len(children) == 0 {
			sb.WriteString(fmt.Sprintf("  %s%s (no dependents)\n", parentName, parentStatus))
			continue
		}

		sb.WriteString(fmt.Sprintf("  %s%s\n", parentName, parentStatus))

		// Sort children by name for deterministic output
		type childEntry struct {
			executor pumped.AnyExecutor
			name     string
		}
		childEntries := make([]childEntry, 0, len(children))
		for _, child := range children {
			childEntries = append(childEntries, childEntry{
				executor: child,
				name:     e.getExecutorName(child),
			})
		}
		sort.Slice(childEntries, func(i, j int) bool {
			return childEntries[i].name < childEntries[j].name
		})

		for i, childEntry := range childEntries {
			child := childEntry.executor
			childName := childEntry.name

			// Mark the failed executor with error details
			if child == failedExecutor {
				childName = childName + " ❌ FAILED"
			} else if e.resolvedExecutors[child] {
				childName = childName + " ✓"
			} else if childErr, failed := e.failedExecutors[child]; failed {
				childName = fmt.Sprintf("%s ❌ (error: %v)", childName, childErr)
			} else {
				childName = childName + " (pending)"
			}

			// Use tree characters
			if i == len(children)-1 {
				sb.WriteString(fmt.Sprintf("    └─> %s\n", childName))
			} else {
				sb.WriteString(fmt.Sprintf("    ├─> %s\n", childName))
			}
		}
	}

	// Show error details for the failed executor
	if failedErr != nil {
		sb.WriteString("\nError Details:\n")
		sb.WriteString(fmt.Sprintf("  Executor: %s\n", e.getExecutorName(failedExecutor)))
		sb.WriteString(fmt.Sprintf("  Error: %v\n", failedErr))
	}

	return sb.String()
}

func (e *GraphDebugExtension) getExecutorName(exec pumped.AnyExecutor) string {
	if name, ok := e.nameTag.Get(exec); ok {
		return name
	}
	return fmt.Sprintf("Executor_%p", exec)
}

// SilentHandler is a slog.Handler that discards all log output
// Useful for testing when you don't want log output
type SilentHandler struct{}

// NewSilentHandler creates a new silent log handler
func NewSilentHandler() *SilentHandler {
	return &SilentHandler{}
}

func (h *SilentHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return false // Never enabled, discards everything
}

func (h *SilentHandler) Handle(ctx context.Context, record slog.Record) error {
	return nil // Do nothing
}

func (h *SilentHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return h // Return self, no state to modify
}

func (h *SilentHandler) WithGroup(name string) slog.Handler {
	return h // Return self, no state to modify
}

// HumanHandler is a slog.Handler that formats logs for human readability
// with proper line breaks and visual formatting (especially for dependency graphs)
type HumanHandler struct {
	writer io.Writer
	level  slog.Level
}

// NewHumanHandler creates a new human-readable log handler
func NewHumanHandler(writer io.Writer, level slog.Level) *HumanHandler {
	return &HumanHandler{
		writer: writer,
		level:  level,
	}
}

func (h *HumanHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return level >= h.level
}

func (h *HumanHandler) Handle(ctx context.Context, record slog.Record) error {
	// Special formatting for GraphDebug messages
	switch record.Message {
	case "Dependency Resolution Error":
		return h.handleDependencyError(record)
	case "Flow Panic":
		return h.handleFlowPanic(record)
	}

	// Default formatting for other messages
	if _, err := fmt.Fprintf(h.writer, "[%s] %s\n", record.Level, record.Message); err != nil {
		return err
	}
	var writeErr error
	record.Attrs(func(a slog.Attr) bool {
		if _, err := fmt.Fprintf(h.writer, "  %s: %v\n", a.Key, a.Value); err != nil {
			writeErr = err
			return false
		}
		return true
	})
	return writeErr
}

func (h *HumanHandler) handleDependencyError(record slog.Record) error {
	var executor, errorMsg, operation, dependencyGraph string

	record.Attrs(func(a slog.Attr) bool {
		switch a.Key {
		case "executor":
			executor = a.Value.String()
		case "error":
			errorMsg = a.Value.String()
		case "operation":
			operation = a.Value.String()
		case "dependency_graph":
			dependencyGraph = a.Value.String()
		}
		return true
	})

	writes := []func() error{
		func() error { _, err := fmt.Fprintln(h.writer); return err },
		func() error { _, err := fmt.Fprintln(h.writer, strings.Repeat("=", 70)); return err },
		func() error { _, err := fmt.Fprintln(h.writer, "[GraphDebug] Dependency Resolution Error"); return err },
		func() error { _, err := fmt.Fprintln(h.writer, strings.Repeat("=", 70)); return err },
		func() error { _, err := fmt.Fprintf(h.writer, "\nFailed Executor: %s\n", executor); return err },
		func() error { _, err := fmt.Fprintf(h.writer, "Error: %s\n", errorMsg); return err },
		func() error { _, err := fmt.Fprintf(h.writer, "Operation: %s\n", operation); return err },
		func() error { _, err := fmt.Fprintf(h.writer, "\nDependency Graph:%s", dependencyGraph); return err },
		func() error { _, err := fmt.Fprintln(h.writer, strings.Repeat("=", 70)); return err },
		func() error { _, err := fmt.Fprintln(h.writer); return err },
	}

	for _, write := range writes {
		if err := write(); err != nil {
			return err
		}
	}

	return nil
}

func (h *HumanHandler) handleFlowPanic(record slog.Record) error {
	var panicMsg, stackTrace, flow string
	var hasFlow bool

	record.Attrs(func(a slog.Attr) bool {
		switch a.Key {
		case "panic":
			panicMsg = a.Value.String()
		case "stack_trace":
			stackTrace = a.Value.String()
		case "flow":
			flow = a.Value.String()
			hasFlow = true
		}
		return true
	})

	writes := []func() error{
		func() error { _, err := fmt.Fprintln(h.writer); return err },
		func() error { _, err := fmt.Fprintln(h.writer, strings.Repeat("=", 70)); return err },
		func() error { _, err := fmt.Fprintln(h.writer, "[GraphDebug] Flow Panic"); return err },
		func() error { _, err := fmt.Fprintln(h.writer, strings.Repeat("=", 70)); return err },
		func() error { _, err := fmt.Fprintf(h.writer, "\nPanic: %s\n", panicMsg); return err },
	}

	for _, write := range writes {
		if err := write(); err != nil {
			return err
		}
	}

	if hasFlow {
		if _, err := fmt.Fprintf(h.writer, "Flow: %s\n", flow); err != nil {
			return err
		}
	}

	finalWrites := []func() error{
		func() error { _, err := fmt.Fprintf(h.writer, "\nStack Trace:\n%s\n", stackTrace); return err },
		func() error { _, err := fmt.Fprintln(h.writer, strings.Repeat("=", 70)); return err },
		func() error { _, err := fmt.Fprintln(h.writer); return err },
	}

	for _, write := range finalWrites {
		if err := write(); err != nil {
			return err
		}
	}

	return nil
}

func (h *HumanHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	// For simplicity, return self (could create new handler with attrs if needed)
	return h
}

func (h *HumanHandler) WithGroup(name string) slog.Handler {
	// For simplicity, return self (could create new handler with group if needed)
	return h
}
