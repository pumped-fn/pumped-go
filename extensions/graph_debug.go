package extensions

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"

	pumped "github.com/pumped-fn/pumped-go"
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

func (e *GraphDebugExtension) formatDependencyGraph(scope *pumped.Scope, failedExecutor pumped.AnyExecutor, failedErr error) string {
	var sb strings.Builder
	graph := scope.ExportDependencyGraph()

	if len(graph) == 0 {
		sb.WriteString("\n(empty - no reactive dependencies tracked)")
		return sb.String()
	}

	sb.WriteString("\n")

	// Format all dependencies
	for parent, children := range graph {
		parentName := e.getExecutorName(parent)

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

		for i, child := range children {
			childName := e.getExecutorName(child)

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
