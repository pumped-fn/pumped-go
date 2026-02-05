# Enforcement Rules

## Tier 2: Important (Strong Recommendations)

### Dependency Modes
- Use `.Reactive()` only for runtime-updatable dependencies
- Default to static dependencies (better performance)
- Document why reactive is needed

### Flow Usage
- Use flows for multi-step operations with tracing needs
- Use plain functions for simple transformations
- Don't over-use flows for single method calls

### Testing
- Test all executors resolve without errors
- Test reactivity when using `.Reactive()`
- Use table-driven tests (Go idiom)
- Separate unit tests from integration tests

### Code Organization
- Group related executors in `graph.go` or `graph/` package
- Separate concerns: config → infra → repos → services → handlers
- Use meaningful executor variable names (e.g., `UserRepo`, not `UR`)

## Tier 3: Best Practices

### Naming Conventions
- Executor variables: `PascalCase` (e.g., `UserService`, `DBExec`)
- Flow variables: `PascalCase` with `Flow` suffix (e.g., `ProcessOrderFlow`)
- Factory functions: Use controllers with meaningful parameter names

### Documentation
- Comment executor groups explaining purpose
- Document reactive dependencies (why reactive vs static)
- Explain cleanup behavior for non-obvious resources

### Extensions
- Use logging extension for production observability
- Add metrics extension for monitoring
- Consider tracing extension for distributed systems

### Graceful Shutdown
- Always implement signal handling
- Use context timeouts for shutdown operations
- Log shutdown steps for debugging

## Quick Reference

| Rule | Tier | Description |
|------|------|-------------|
| Package-level vars | 1 (Critical) | ALL executors MUST be package-level var |
| Error handling | 1 (Critical) | ALWAYS handle errors from ctrl.Get() |
| Lifecycle management | 1 (Critical) | ALWAYS call scope.Dispose() |
| OnCleanup | 1 (Critical) | ALWAYS register for resources |
| Goroutine safety | 1 (Critical) | ALWAYS stop goroutines in OnCleanup |
| Static by default | 2 (Important) | Use Reactive only when needed |
| Flow for multi-step | 2 (Important) | Single calls use direct executor |
| Table-driven tests | 2 (Important) | Go idiom for test organization |
| PascalCase naming | 3 (Best Practice) | Consistent naming |
| Zero comments | 3 (Best Practice) | Self-documenting code |
| Flat structure | 3 (Best Practice) | Avoid deep nesting |
