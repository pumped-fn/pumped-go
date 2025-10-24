package pumped

import (
	"fmt"
	"runtime/debug"
)

type ResolveError struct {
	ExecutorID AnyExecutor
	Cause      error
	Context    string
	StackTrace []byte
}

func (e *ResolveError) Error() string {
	if e.Context != "" {
		return fmt.Sprintf("resolve error in executor %v during %s: %v", e.ExecutorID, e.Context, e.Cause)
	}
	return fmt.Sprintf("resolve error in executor %v: %v", e.ExecutorID, e.Cause)
}

func (e *ResolveError) Unwrap() error {
	return e.Cause
}

// SafeTypeAssertion performs safe type assertion with proper error
func SafeTypeAssertion[T any](value any) (T, error) {
	if value == nil {
		var zero T
		return zero, nil
	}

	typed, ok := value.(T)
	if !ok {
		var zero T
		return zero, fmt.Errorf("type assertion error: expected %T, got %T (value: %v)", zero, value, value)
	}

	return typed, nil
}

func CreateResolveError(executor AnyExecutor, cause error, context string) *ResolveError {
	return &ResolveError{
		ExecutorID: executor,
		Cause:      cause,
		Context:    context,
		StackTrace: debug.Stack(),
	}
}
