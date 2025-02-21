import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { Suspense, act } from "react";
import { createScope, provide, mutable, Executor, derive, ref } from "@pumped-fn/core";
import { ScopeProvider, useResolve, useUpdate } from "../src/index";

describe("React Integration", () => {
  it("handles complex state management scenarios", async () => {
    const scope = createScope();
    const countExecutor = provide(() => mutable(0));
    const derivedCount = derive([countExecutor], ([v]) => v + 2);
    const wholesum = derive([countExecutor, derivedCount], ([v1, v2]) => {
      return v1 + v2
    });

    const updateCount = derive([ref(countExecutor)], ([ref], scope) => {
      return (value: number) => scope.update(ref, () => value);
    });

    const fn = vi.fn()

    const { result } = renderHook(
      () => {
        fn()
        // Test multiple hooks working together
        const derived = useResolve(derivedCount);
        const update = useResolve(updateCount);
        const wholesumValue = useResolve(wholesum);

        return { derived, update, wholesumValue };
      },
      {
        wrapper: ({ children }) => (
          <ScopeProvider scope={scope}>
            <Suspense>{children}</Suspense>
          </ScopeProvider>
        ),
      },
    );

    await waitFor(() => {
      expect(result.current.derived).toBe(2);
      expect(result.current.wholesumValue).toBe(2);
    });

    act(() => {
      result.current.update(4);
    });

    await waitFor(() => {
      expect(result.current.derived).toBe(6);
    });

    await waitFor(() => {
      expect(result.current.wholesumValue).toBe(10);
    });

  });
});
