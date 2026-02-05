package extensions

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"regexp"
	"sort"
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

// D2 status colors
const (
	d2ColorOK      = "#90EE90" // Light green
	d2ColorFailed  = "#FFB6C1" // Light red
	d2ColorPending = "#D3D3D3" // Light gray
	d2ColorEmpty   = "#F5F5F5" // Very light gray
	d2StrokeFailed = "#FF0000" // Red
)

// sanitizeD2NodeID converts an executor name to a valid D2 node ID
func sanitizeD2NodeID(name string) string {
	// Replace spaces with underscores, keep only alphanumeric and underscores
	re := regexp.MustCompile(`[^a-zA-Z0-9_]`)
	sanitized := strings.ReplaceAll(name, " ", "_")
	sanitized = re.ReplaceAllString(sanitized, "_")
	// Ensure it doesn't start with a number
	if len(sanitized) > 0 && sanitized[0] >= '0' && sanitized[0] <= '9' {
		sanitized = "_" + sanitized
	}
	if sanitized == "" {
		sanitized = "_node"
	}
	return sanitized
}

// formatD2Diagram generates a D2 diagram representation of the dependency graph
func (e *GraphDebugExtension) formatD2Diagram(graph map[pumped.AnyExecutor][]pumped.AnyExecutor, failedExecutor pumped.AnyExecutor) string {
	var sb strings.Builder

	sb.WriteString("direction: down\n\n")

	// Handle empty graph
	if len(graph) == 0 {
		sb.WriteString(fmt.Sprintf("empty: \"No reactive dependencies tracked\" {\n  style.fill: \"%s\"\n}\n", d2ColorEmpty))
		return sb.String()
	}

	// Collect all unique nodes
	allNodes := make(map[pumped.AnyExecutor]bool)
	for parent, children := range graph {
		allNodes[parent] = true
		for _, child := range children {
			allNodes[child] = true
		}
	}

	// Sort nodes by name for deterministic output
	type nodeEntry struct {
		executor pumped.AnyExecutor
		name     string
	}
	nodes := make([]nodeEntry, 0, len(allNodes))
	for exec := range allNodes {
		nodes = append(nodes, nodeEntry{
			executor: exec,
			name:     e.getExecutorName(exec),
		})
	}
	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].name < nodes[j].name
	})

	// Emit node declarations with styling
	for _, node := range nodes {
		nodeID := sanitizeD2NodeID(node.name)
		label := node.name

		// Determine status and styling
		var fill, stroke string
		var strokeWidth int

		if node.executor == failedExecutor {
			fill = d2ColorFailed
			stroke = d2StrokeFailed
			strokeWidth = 3
		} else if e.resolvedExecutors[node.executor] {
			fill = d2ColorOK
		} else {
			fill = d2ColorPending
		}

		sb.WriteString(fmt.Sprintf("%s: \"%s\" {\n", nodeID, label))
		sb.WriteString(fmt.Sprintf("  style.fill: \"%s\"\n", fill))
		if stroke != "" {
			sb.WriteString(fmt.Sprintf("  style.stroke: \"%s\"\n", stroke))
			sb.WriteString(fmt.Sprintf("  style.stroke-width: %d\n", strokeWidth))
		}
		sb.WriteString("}\n")
	}

	sb.WriteString("\n")

	// Collect and sort connections for deterministic output
	type connection struct {
		from string
		to   string
	}
	var connections []connection

	for parent, children := range graph {
		parentID := sanitizeD2NodeID(e.getExecutorName(parent))
		for _, child := range children {
			childID := sanitizeD2NodeID(e.getExecutorName(child))
			connections = append(connections, connection{from: parentID, to: childID})
		}
	}

	sort.Slice(connections, func(i, j int) bool {
		if connections[i].from != connections[j].from {
			return connections[i].from < connections[j].from
		}
		return connections[i].to < connections[j].to
	})

	// Emit connections
	for _, conn := range connections {
		sb.WriteString(fmt.Sprintf("%s -> %s\n", conn.from, conn.to))
	}

	return sb.String()
}

func (e *GraphDebugExtension) formatDependencyGraph(scope *pumped.Scope, failedExecutor pumped.AnyExecutor, failedErr error) string {
	var sb strings.Builder
	graph := scope.ExportDependencyGraph()

	sb.WriteString("\n")
	sb.WriteString(e.formatD2Diagram(graph, failedExecutor))

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
