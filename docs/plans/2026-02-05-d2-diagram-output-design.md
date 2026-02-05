# GraphDebugExtension D2 Diagram Output

**Date:** 2026-02-05
**Status:** Approved
**Scope:** Replace treedrawer ASCII output with D2 diagram text format

## Overview

Change `GraphDebugExtension` to output D2 diagram markup instead of ASCII tree visualizations. This simplifies the codebase by removing the `treedrawer` dependency while providing a more versatile output format that can be rendered by D2 CLI or other tools.

## Design Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Output format | D2 text only | Users can render with preferred tools; no image generation dependency |
| Replace or add | Replace entirely | Simpler codebase, fewer dependencies |
| Status representation | Node styling (colors) | Visually clear when rendered |
| Arrow direction | Parent → Child (downstream) | Matches existing data structure; shows failure impact |
| Layout direction | Top to bottom | Common convention for dependency trees |

## D2 Output Format

### Example Output

```d2
direction: down

Config_Loader: "Config Loader" {
  style.fill: "#90EE90"
}
Database: "Database" {
  style.fill: "#90EE90"
}
User_Service: "User Service" {
  style.fill: "#FFB6C1"
  style.stroke: "#FF0000"
  style.stroke-width: 3
}

Config_Loader -> Database
Config_Loader -> User_Service
Database -> User_Service
```

### Status Styling

| Status | Fill Color | Additional Styling |
|--------|------------|-------------------|
| OK (resolved) | `#90EE90` (light green) | None |
| FAILED | `#FFB6C1` (light red) | Red stroke (`#FF0000`), stroke-width: 3 |
| PENDING | `#D3D3D3` (light gray) | None |

### Node ID Sanitization

D2 node IDs cannot contain spaces or special characters. Executor names are sanitized:
- Replace spaces with underscores
- Keep alphanumeric characters and underscores only
- Use sanitized name as ID, original name as label

### Empty Graph

```d2
direction: down
empty: "No reactive dependencies tracked" {
  style.fill: "#F5F5F5"
}
```

## Implementation

### New Method

```go
func (e *GraphDebugExtension) formatD2Diagram(
    graph map[pumped.AnyExecutor][]pumped.AnyExecutor,
    failedExecutor pumped.AnyExecutor,
) string
```

**Logic:**
1. Output `direction: down`
2. Collect all unique nodes (parents and children)
3. Sort nodes alphabetically for deterministic output
4. Emit node declarations with status-based styling
5. Emit connection lines (`parent -> child`)
6. Sort connections for deterministic output

### Simplified `formatDependencyGraph()`

Reduced from ~100 lines to ~20 lines:
- Call `formatD2Diagram()` for the graph visualization
- Append error details section if applicable

### Code Removal

| Method | Lines | Action |
|--------|-------|--------|
| `tryFormatHorizontalTree()` | ~50 | Delete |
| `buildTree()` | ~40 | Delete |
| `addTreeAsChild()` | ~10 | Delete |
| Detailed View ASCII section | ~60 | Delete |

**Net change:** ~160 lines removed, ~50 lines added

### Dependency Removal

Remove from `go.mod`:
```
github.com/m1gwings/treedrawer
```

Run `go mod tidy` after changes.

## Public API

No changes. Users create the extension the same way:

```go
handler := extensions.NewHumanHandler(os.Stdout, slog.LevelError)
ext := extensions.NewGraphDebugExtension(handler)
```

## Testing

### D2 Syntax Validation

Use the `oss.terrastruct.com/d2` Go library to validate D2 output in tests:

```go
import (
    "context"
    "oss.terrastruct.com/d2/d2lib"
)

func TestD2OutputIsValid(t *testing.T) {
    // Generate D2 output
    output := ext.formatD2Diagram(graph, failedExec)

    // Validate by compiling - error means invalid D2 syntax
    _, _, err := d2lib.Compile(context.Background(), output, nil, nil)
    require.NoError(t, err, "D2 output should be valid syntax")
}
```

This is a **test-only dependency** - not imported in production code.

### Content Assertions

Also verify expected content:

```go
output := ext.formatD2Diagram(graph, failedExec)
assert.Contains(t, output, "direction: down")
assert.Contains(t, output, "style.fill: \"#FFB6C1\"")
assert.Contains(t, output, "ConfigLoader -> Database")
```

## Implementation Tasks

1. Add `formatD2Diagram()` method with node sanitization
2. Add `sanitizeD2NodeID()` helper method
3. Simplify `formatDependencyGraph()` to use D2 output
4. Remove `tryFormatHorizontalTree()`, `buildTree()`, `addTreeAsChild()`
5. Remove treedrawer import
6. Run `go mod tidy` to remove dependency
7. Update tests for D2 format expectations
