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

// NodeStatus represents the resolution state of a node in the dependency graph.
type NodeStatus string

const (
	NodeStatusOK      NodeStatus = "ok"
	NodeStatusFailed  NodeStatus = "failed"
	NodeStatusPending NodeStatus = "pending"
)

// GraphNode represents a single node in the dependency graph.
type GraphNode struct {
	Name   string
	Status NodeStatus
}

// GraphEdge represents a directed edge between two nodes.
type GraphEdge struct {
	From string
	To   string
}

// GraphData is the format-agnostic representation of a dependency graph.
// Formatters receive this struct and produce a string visualization.
type GraphData struct {
	Nodes []GraphNode
	Edges []GraphEdge
}

// GraphFormatter converts dependency graph data into a string representation.
// Implement this interface to provide custom graph output formats (e.g., Mermaid, PlantUML, DOT).
type GraphFormatter interface {
	FormatGraph(data GraphData) string
}

// GraphDebugExtension logs dependency graph visualization when errors occur.
//
// Usage:
//
//	// Human-readable formatted output with D2 diagrams
//	handler := extensions.NewHumanHandler(os.Stdout, slog.LevelError)
//	ext := extensions.NewGraphDebugExtension(handler, extensions.NewD2Formatter())
//
//	// Custom graph format
//	ext := extensions.NewGraphDebugExtension(handler, myCustomFormatter)
//
//	// Silent (for testing)
//	ext := extensions.NewGraphDebugExtension(extensions.NewSilentHandler(), extensions.NewD2Formatter())
//
// The extension logs at ERROR level for both resolution errors and flow panics.
type GraphDebugExtension struct {
	pumped.BaseExtension
	nameTag pumped.Tag[string]

	// Track executors as they're resolved
	resolvedExecutors map[pumped.AnyExecutor]bool
	failedExecutors   map[pumped.AnyExecutor]error
	logger            *slog.Logger
	formatter         GraphFormatter
}

// NewGraphDebugExtension creates a new graph debug extension.
// logHandler: slog.Handler for logging (use HumanHandler for formatted output, or any other slog.Handler)
// formatter: GraphFormatter for rendering the dependency graph (use NewD2Formatter() for D2 diagrams)
func NewGraphDebugExtension(logHandler slog.Handler, formatter GraphFormatter) *GraphDebugExtension {
	logger := slog.New(logHandler)
	return &GraphDebugExtension{
		BaseExtension:     pumped.NewBaseExtension("graph-debug"),
		nameTag:           pumped.NewTag[string]("executor.name"),
		resolvedExecutors: make(map[pumped.AnyExecutor]bool),
		failedExecutors:   make(map[pumped.AnyExecutor]error),
		logger:            logger,
		formatter:         formatter,
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

// buildGraphData converts internal executor state into format-agnostic GraphData.
func (e *GraphDebugExtension) buildGraphData(graph map[pumped.AnyExecutor][]pumped.AnyExecutor, failedExecutor pumped.AnyExecutor) GraphData {
	// Handle empty graph
	if len(graph) == 0 {
		return GraphData{}
	}

	// Collect all unique executors
	allExecs := make(map[pumped.AnyExecutor]bool)
	for parent, children := range graph {
		allExecs[parent] = true
		for _, child := range children {
			allExecs[child] = true
		}
	}

	// Build nodes sorted by name for deterministic output
	type execEntry struct {
		executor pumped.AnyExecutor
		name     string
	}
	entries := make([]execEntry, 0, len(allExecs))
	for exec := range allExecs {
		entries = append(entries, execEntry{
			executor: exec,
			name:     e.getExecutorName(exec),
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].name < entries[j].name
	})

	nodes := make([]GraphNode, 0, len(entries))
	for _, entry := range entries {
		var status NodeStatus
		if entry.executor == failedExecutor {
			status = NodeStatusFailed
		} else if e.resolvedExecutors[entry.executor] {
			status = NodeStatusOK
		} else {
			status = NodeStatusPending
		}
		nodes = append(nodes, GraphNode{
			Name:   entry.name,
			Status: status,
		})
	}

	// Build edges sorted for deterministic output
	var edges []GraphEdge
	for parent, children := range graph {
		parentName := e.getExecutorName(parent)
		for _, child := range children {
			childName := e.getExecutorName(child)
			edges = append(edges, GraphEdge{From: parentName, To: childName})
		}
	}
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].From != edges[j].From {
			return edges[i].From < edges[j].From
		}
		return edges[i].To < edges[j].To
	})

	return GraphData{Nodes: nodes, Edges: edges}
}

func (e *GraphDebugExtension) formatDependencyGraph(scope *pumped.Scope, failedExecutor pumped.AnyExecutor, failedErr error) string {
	var sb strings.Builder
	graph := scope.ExportDependencyGraph()

	data := e.buildGraphData(graph, failedExecutor)
	sb.WriteString("\n")
	sb.WriteString(e.formatter.FormatGraph(data))

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

// --- D2Formatter: Built-in GraphFormatter producing D2 diagram syntax ---

// D2 status colors
const (
	d2ColorOK      = "#90EE90" // Light green
	d2ColorFailed  = "#FFB6C1" // Light red
	d2ColorPending = "#D3D3D3" // Light gray
	d2ColorEmpty   = "#F5F5F5" // Very light gray
	d2StrokeFailed = "#FF0000" // Red
)

// D2Formatter renders dependency graphs as D2 diagram text.
// See https://d2lang.com for the D2 language specification.
type D2Formatter struct{}

// NewD2Formatter creates a new D2 diagram formatter.
func NewD2Formatter() *D2Formatter {
	return &D2Formatter{}
}

// FormatGraph produces a D2 diagram string from the graph data.
func (f *D2Formatter) FormatGraph(data GraphData) string {
	var sb strings.Builder

	sb.WriteString("direction: down\n\n")

	// Handle empty graph
	if len(data.Nodes) == 0 {
		sb.WriteString(fmt.Sprintf("empty: \"No reactive dependencies tracked\" {\n  style.fill: \"%s\"\n}\n", d2ColorEmpty))
		return sb.String()
	}

	// Emit node declarations with styling
	for _, node := range data.Nodes {
		nodeID := sanitizeD2NodeID(node.Name)

		var fill, stroke string
		var strokeWidth int

		switch node.Status {
		case NodeStatusFailed:
			fill = d2ColorFailed
			stroke = d2StrokeFailed
			strokeWidth = 3
		case NodeStatusOK:
			fill = d2ColorOK
		default:
			fill = d2ColorPending
		}

		sb.WriteString(fmt.Sprintf("%s: \"%s\" {\n", nodeID, node.Name))
		sb.WriteString(fmt.Sprintf("  style.fill: \"%s\"\n", fill))
		if stroke != "" {
			sb.WriteString(fmt.Sprintf("  style.stroke: \"%s\"\n", stroke))
			sb.WriteString(fmt.Sprintf("  style.stroke-width: %d\n", strokeWidth))
		}
		sb.WriteString("}\n")
	}

	sb.WriteString("\n")

	// Emit connections
	for _, edge := range data.Edges {
		fromID := sanitizeD2NodeID(edge.From)
		toID := sanitizeD2NodeID(edge.To)
		sb.WriteString(fmt.Sprintf("%s -> %s\n", fromID, toID))
	}

	return sb.String()
}

// sanitizeD2NodeID converts a node name to a valid D2 node ID.
func sanitizeD2NodeID(name string) string {
	re := regexp.MustCompile(`[^a-zA-Z0-9_]`)
	sanitized := strings.ReplaceAll(name, " ", "_")
	sanitized = re.ReplaceAllString(sanitized, "_")
	if len(sanitized) > 0 && sanitized[0] >= '0' && sanitized[0] <= '9' {
		sanitized = "_" + sanitized
	}
	if sanitized == "" {
		sanitized = "_node"
	}
	return sanitized
}

// --- SilentHandler ---

// SilentHandler is a slog.Handler that discards all log output.
// Useful for testing when you don't want log output.
type SilentHandler struct{}

// NewSilentHandler creates a new silent log handler.
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

// --- HumanHandler ---

// HumanHandler is a slog.Handler that formats logs for human readability
// with proper line breaks and visual formatting (especially for dependency graphs).
type HumanHandler struct {
	writer io.Writer
	level  slog.Level
}

// NewHumanHandler creates a new human-readable log handler.
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
