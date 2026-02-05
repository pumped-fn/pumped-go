# Flows (Short-Span Operations)

**What:** Operations with defined beginning and end (request handling, business logic, transactions)

**When to use:**
- Request/response cycles
- Business operations with multiple steps
- Operations that need execution tracing
- Operations that compose sub-operations
- Operations that need tag-based data flow

**When NOT to use:**
- Pure transformations (use plain functions)
- Single method calls (use executors directly)
- Long-running background jobs (use executors with goroutines)

## Common Mistake: Nil Executor References

**CRITICAL:** Flow dependencies MUST be direct executor references, never nil.

```go
// WRONG: Nil executor placeholders
var MyFlow = pumped.Flow2(
    nil, // UserRepo - will be set later    THIS WILL FAIL
    nil, // Logger - will be set later      THIS WILL FAIL
    func(execCtx *pumped.ExecutionCtx,
        userRepoCtrl *pumped.Controller[*UserRepository],
        logCtrl *pumped.Controller[*Logger]) (*Result, error) {
        // This will never work - executors are nil!
    },
    pumped.WithFlowTag(pumped.FlowName(), "myFlow"),
)

// CORRECT: Direct executor references
var (
    // Declare executors first
    UserRepo = pumped.Derive1(DB, func(...) (*UserRepository, error) { ... })
    Logger = pumped.Provide(func(...) (*Logger, error) { ... })

    // Flows reference executors directly
    MyFlow = pumped.Flow2(
        UserRepo,  // Direct reference to package-level var
        Logger,    // Direct reference to package-level var
        func(execCtx *pumped.ExecutionCtx,
            userRepoCtrl *pumped.Controller[*UserRepository],
            logCtrl *pumped.Controller[*Logger]) (*Result, error) {

            // Now Get() works correctly
            userRepo, err := userRepoCtrl.Get()
            if err != nil {
                return nil, err
            }
            // ...
        },
        pumped.WithFlowTag(pumped.FlowName(), "myFlow"),
    )
)
```

**Why this works:** Executors are package-level variables, so they're available when flows are declared. No separate "initialization" step is needed.

## Flow vs Direct Call Decision

```
Need execution tracing/debugging? ───────YES────→ Use Flow
         │
         NO
         ↓
Multiple steps with branching logic? ────YES────→ Use Flow
         │
         NO
         ↓
Compose sub-operations? ─────────────────YES────→ Use Flow
         │
         NO
         ↓
Single service method call? ─────────────YES────→ Direct call
```

**Use Flow:**
- Process campaign: Fetch jobs → Send emails → Update stats → Log results
- Schedule campaign: Validate input → Check time → Create jobs → Update status
- User registration: Validate → Check duplicates → Create user → Send welcome email

**Don't Use Flow:**
- Get user by ID (single repo call)
- Increment counter (single atomic operation)
- Format string (pure transformation)

## Basic Flow Definition

```go
// Flow with no dependencies
var SimpleFlow = pumped.Flow0(
    func(execCtx *pumped.ExecutionCtx) (string, error) {
        return "result", nil
    },
    pumped.WithFlowTag(pumped.FlowName(), "simpleFlow"),
)

// Flow with 1 dependency
var FetchUserFlow = pumped.Flow1(
    UserRepo,
    func(execCtx *pumped.ExecutionCtx,
        userRepoCtrl *pumped.Controller[*UserRepository]) (*User, error) {

        repo, err := userRepoCtrl.Get()
        if err != nil {
            return nil, err
        }

        // Access Go context (for cancellation, deadlines, values)
        ctx := execCtx.Context()

        // Get input from execution context
        userIDRaw, ok := execCtx.Get(pumped.Input())
        if !ok {
            return nil, fmt.Errorf("user ID not found in context")
        }
        userID := userIDRaw.(string)

        user, err := repo.FindByID(ctx, userID)
        if err != nil {
            return nil, fmt.Errorf("failed to fetch user: %w", err)
        }

        return user, nil
    },
    pumped.WithFlowTag(pumped.FlowName(), "fetchUser"),
)

// Flow with multiple dependencies
var ProcessOrderFlow = pumped.Flow3(
    UserRepo,
    PaymentGateway,
    Logger,
    func(execCtx *pumped.ExecutionCtx,
        userRepoCtrl *pumped.Controller[*UserRepository],
        paymentCtrl *pumped.Controller[*PaymentGateway],
        logCtrl *pumped.Controller[*Logger]) (*Order, error) {

        // Get all dependencies
        userRepo, err := userRepoCtrl.Get()
        if err != nil {
            return nil, err
        }
        payment, err := paymentCtrl.Get()
        if err != nil {
            return nil, err
        }
        log, err := logCtrl.Get()
        if err != nil {
            return nil, err
        }

        // Business logic...
        log.Info("Processing order")
        return processOrder(execCtx.Context(), userRepo, payment)
    },
    pumped.WithFlowTag(pumped.FlowName(), "processOrder"),
)
```

## Executing Flows

**From main or HTTP handlers:**

```go
func main() {
    scope := pumped.NewScope()
    defer scope.Dispose()

    ctx := context.Background()

    // Execute flow
    result, execNode, err := pumped.Exec(scope, ctx, FetchUserFlow)
    if err != nil {
        log.Fatalf("flow execution failed: %v", err)
    }

    // Access execution metadata
    if name, ok := execNode.Get(pumped.FlowName()); ok {
        log.Printf("Flow name: %s", name)
    }

    // Access execution tree
    tree := scope.GetExecutionTree()
    roots := tree.GetRoots()
    // Analyze execution tree for debugging/tracing
}

// HTTP handler example
func (h *Handler) HandleRequest(w http.ResponseWriter, r *http.Request) {
    result, _, err := pumped.Exec(h.scope, r.Context(), ProcessOrderFlow)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    json.NewEncoder(w).Encode(result)
}
```

## Sub-Flow Composition

**Execute child flows with `Exec1`, `Exec2`, etc.**

```go
var ParentFlow = pumped.Flow2(
    UserRepo,
    OrderRepo,
    func(execCtx *pumped.ExecutionCtx,
        userRepoCtrl *pumped.Controller[*UserRepository],
        orderRepoCtrl *pumped.Controller[*OrderRepository]) (string, error) {

        // Set data for child flows
        execCtx.Set(pumped.Input(), "user-123")

        // Execute sub-flow (inherits scope from execCtx)
        user, userCtx, err := pumped.Exec1(execCtx, FetchUserFlow)
        if err != nil {
            return "", fmt.Errorf("fetch user failed: %w", err)
        }

        // Execute another sub-flow (can use parent or sibling context)
        orders, _, err := pumped.Exec1(userCtx, FetchOrdersFlow)
        if err != nil {
            return "", fmt.Errorf("fetch orders failed: %w", err)
        }

        result := fmt.Sprintf("User %s has %d orders", user.Name, len(orders))
        return result, nil
    },
    pumped.WithFlowTag(pumped.FlowName(), "parentFlow"),
)
```

**Execution context tree:**
- `Exec1(execCtx, flow)` - Execute with current context as parent
- Child contexts inherit scope
- Tag lookups traverse upward (child → parent → scope)

## Tag-Based Data Flow

**Execution contexts support tag-based data storage and lookup:**

```go
var ParentFlow = pumped.Flow0(
    func(execCtx *pumped.ExecutionCtx) (string, error) {
        // Set data in current context
        execCtx.Set(pumped.Input(), "user-123")
        execCtx.Set(customTag, "custom-value")

        // Child flows can access this data
        result, _, err := pumped.Exec1(execCtx, ChildFlow)
        return result, err
    },
)

var ChildFlow = pumped.Flow0(
    func(execCtx *pumped.ExecutionCtx) (string, error) {
        // Get from current context only
        val, ok := execCtx.Get(pumped.Input())

        // Get from parent contexts (traverses upward)
        val, ok := execCtx.GetFromParent(pumped.Input())

        // Get from scope tags
        val, ok := execCtx.GetFromScope(customScopeTag)

        // Lookup: Try current, then parents, then scope
        val, ok := execCtx.Lookup(pumped.Input())  // Most common pattern
        if !ok {
            return "", fmt.Errorf("required data not found")
        }

        userID := val.(string)
        return fmt.Sprintf("Processing user: %s", userID), nil
    },
)
```

**Common tags:**
- `pumped.Input()` - Input data for flows
- `pumped.FlowName()` - Flow identification
- `pumped.Status()` - Execution status
- Custom tags - Create with `pumped.NewTag[T]()`

## Executing Flows in HTTP Handlers

Handlers should use flows for multi-step operations to enable execution tracing and observability.

```go
// Define custom tags for flow input (in flows/ or graph.go)
var (
    OrderIDTag = pumped.NewTag[string]("order.id")
    OrderItemsTag = pumped.NewTag[[]OrderItem]("order.items")
)

// Define flow with dependencies
var ProcessOrderFlow = pumped.Flow3(
    OrderRepo,
    InventoryService,
    PaymentService,
    func(execCtx *pumped.ExecutionCtx,
        orderRepoCtrl *pumped.Controller[*OrderRepository],
        inventoryCtrl *pumped.Controller[*InventoryService],
        paymentCtrl *pumped.Controller[*PaymentService]) (*Order, error) {

        // Get dependencies
        orderRepo, err := orderRepoCtrl.Get()
        if err != nil {
            return nil, err
        }
        inventory, err := inventoryCtrl.Get()
        if err != nil {
            return nil, err
        }
        payment, err := paymentCtrl.Get()
        if err != nil {
            return nil, err
        }

        // Get input from execution context
        orderID, ok := execCtx.Lookup(OrderIDTag)
        if !ok {
            return nil, fmt.Errorf("order ID not found")
        }
        items, ok := execCtx.Lookup(OrderItemsTag)
        if !ok {
            return nil, fmt.Errorf("order items not found")
        }

        // Multi-step business logic
        // Step 1: Check inventory
        available, err := inventory.CheckStock(execCtx.Context(), items.([]OrderItem))
        if err != nil {
            return nil, fmt.Errorf("inventory check failed: %w", err)
        }
        if !available {
            return nil, fmt.Errorf("insufficient stock")
        }

        // Step 2: Process payment
        charged, err := payment.Charge(execCtx.Context(), calculateTotal(items.([]OrderItem)))
        if err != nil {
            return nil, fmt.Errorf("payment failed: %w", err)
        }

        // Step 3: Create order
        order, err := orderRepo.Create(execCtx.Context(), orderID.(string), items.([]OrderItem), charged)
        if err != nil {
            return nil, fmt.Errorf("order creation failed: %w", err)
        }

        return order, nil
    },
    pumped.WithFlowTag(pumped.FlowName(), "processOrder"),
)

// Handler with scope
type OrderHandler struct {
    scope *pumped.Scope
}

func (h *OrderHandler) ProcessOrder(w http.ResponseWriter, r *http.Request) {
    var input ProcessOrderInput
    if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
        http.Error(w, "Invalid input", http.StatusBadRequest)
        return
    }

    // Validate input
    if len(input.Items) == 0 {
        http.Error(w, "No items in order", http.StatusBadRequest)
        return
    }

    // Set input tags in scope
    h.scope.SetTag(OrderIDTag, input.OrderID)
    h.scope.SetTag(OrderItemsTag, input.Items)

    // Execute flow
    order, execNode, err := pumped.Exec(h.scope, r.Context(), ProcessOrderFlow)
    if err != nil {
        log.Printf("Flow execution failed: %v", err)
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

    // Access execution metadata for logging/tracing
    if flowName, ok := execNode.Get(pumped.FlowName()); ok {
        log.Printf("Successfully executed flow: %s", flowName)
    }

    w.WriteHeader(http.StatusCreated)
    json.NewEncoder(w).Encode(order)
}
```

**Why use flows in handlers:**
- Execution tracing - see exactly what happened in request
- Execution tree - visualize operation flow for debugging
- Tag-based input - clean data passing without global state
- Observable - extensions can hook into flow lifecycle
- Testable - mock executors, verify flow logic independently

**When NOT to use flows in handlers:**
- Simple CRUD operations (single repo call)
- Direct data fetching (no business logic)
- Trivial transformations
