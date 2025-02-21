import type { MutableOutput, Scope, InferOutput } from "@pumped-fn/core";
import type { GetAccessor } from "@pumped-fn/core";
import { createScope, type Executor } from "@pumped-fn/core";
import { createContext, useContext, useEffect, useMemo, useRef, useSyncExternalStore } from "react";

type ValueEntry = { kind: "value"; value: GetAccessor<unknown> };
type ErrorEntry = { kind: "error"; error: unknown };
type PendingEntry = { kind: "pending"; promise: Promise<unknown> };
type Entry = ValueEntry | ErrorEntry | PendingEntry;

const isValueEntry = (entry: Entry): entry is ValueEntry => entry.kind === "value";
const isErrorEntry = (entry: Entry): entry is ErrorEntry => entry.kind === "error";
const isPendingEntry = (entry: Entry): entry is PendingEntry => entry.kind === "pending";

type CacheEntry = [Executor<unknown>, Entry];

class ScopeContainer {
  #scope: Scope;
  #cache: CacheEntry[] = [];

  constructor(scope?: Scope) {
    this.#scope = scope ?? createScope();
  }

  get scope() {
    return this.#scope;
  }

  getResolved(executor: Executor<unknown>): CacheEntry {
    const maybeEntry = this.#cache.find(([e]) => e === executor);

    if (maybeEntry) {
      return maybeEntry;
    }

    const cacheEntry: CacheEntry = [
      executor,
      {
        kind: "pending",
        promise: this.#scope
          .resolve(executor)
          .then((value) => {
            cacheEntry[1] = { kind: "value", value };
          })
          .catch((error) => {
            cacheEntry[1] = { kind: "error", error };
          }),
      },
    ];

    this.#cache.push(cacheEntry);
    return cacheEntry;
  }

  static create(scope?: Scope) {
    return new ScopeContainer(scope);
  }
}

const scopeContainerContext = createContext<ScopeContainer | undefined>(undefined);

export function useScope() {
  const context = useContext(scopeContainerContext);
  if (context === undefined) {
    throw new Error("useScope must be used within a ScopeProvider");
  }
  return context;
}

export function ScopeProvider({ children, scope }: { children: React.ReactNode; scope?: Scope }) {
  return (
    <scopeContainerContext.Provider value={ScopeContainer.create(scope)}>{children}</scopeContainerContext.Provider>
  );
}

type PendingState<T> = { state: "pending"; promise: Promise<T> };
type ResolvedState<T> = { state: "resolved"; value: T };
type ErrorState<T> = { state: "error"; error: T };

export type ResolveState<T> = PendingState<T> | ResolvedState<T> | ErrorState<T>;

export function useResolve<T>(executor: Executor<T>): InferOutput<T>;
export function useResolve<T, K>(executor: Executor<T>, selector: (value: InferOutput<T>) => K): K;
export function useResolve<T, K = InferOutput<T>>(
  executor: Executor<T>,
  selector?: (value: InferOutput<T>) => K
): K {
  const scope = useScope();

  const [_, entry] = scope.getResolved(executor);

  if (isPendingEntry(entry)) {
    throw entry.promise;
  }

  if (isErrorEntry(entry)) {
    throw entry.error;
  }

  const resolved = useSyncExternalStore(
    (cb) => scope.scope.on(executor, cb),
    () => entry.value.get() as InferOutput<T>,
    () => entry.value.get() as InferOutput<T>,
	);

	const snapshotRef = useRef<any>();
	const value = selector ? selector(resolved) : resolved;

	if (!!!snapshotRef.current || (JSON.stringify(value) !== JSON.stringify(snapshotRef.current))) {
		snapshotRef.current = value;
	}

	return snapshotRef.current;
}

export function useUpdate<T>(executor: Executor<MutableOutput<T>>): (updateFn: (current: T) => T) => void {
  const scope = useScope();

  return (updateFn: (current: T) => T) => {
    scope.scope.update(executor, updateFn);
  };
}
