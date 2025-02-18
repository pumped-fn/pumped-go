export interface Cleanup {
  (): void
}

export interface GetAccessor<T> {
  get: () => Awaited<T> extends Output<infer X> ? X : T
}

const getAccessor = <T>(get: () => T): GetAccessor<T> => ({
  get: () => get() as any
})

export const outputSymbol = Symbol('jumped-fn.output')

export interface Output<T> {
  value: T
  [outputSymbol]: string
}

export interface MutableOutput<T> extends Output<T> {
  [outputSymbol]: 'mutable'
}

export interface ImmutableOutput<T> extends Output<T> {
  [outputSymbol]: 'immutable'
}

export interface ResourceOutput<T> extends Output<T> {
  cleanup: () => void
  [outputSymbol]: 'resource'
}

export interface EffectOutput extends Output<never> {
  [outputSymbol]: 'effect'
  cleanup: () => void
}

export const isOutput = <T>(value: unknown): value is Output<T> =>
  typeof value === 'object' && value !== null && outputSymbol in value;

export const isResouceOutput = <T>(value: unknown): value is ResourceOutput<T> =>
  isOutput(value) && value[outputSymbol] === 'resource';

export const isEffectOutput = (value: unknown): value is EffectOutput =>
  isOutput(value) && value[outputSymbol] === 'effect';

export const mutable = <T>(value: T): MutableOutput<T> => ({
  value,
  [outputSymbol]: 'mutable'
});

export const resource = <T>(value: T, cleanup: Cleanup): ResourceOutput<T> => ({
  value,
  cleanup,
  [outputSymbol]: 'resource'
});

export const effect = (cleanup: Cleanup): EffectOutput => ({
  value: undefined as never,
  cleanup,
  [outputSymbol]: 'effect'
});

const executorSymbol = Symbol('jumped-fn.executor')

function isExecutor<T>(value: unknown): value is Executor<T> {
  return typeof value === 'object' && value !== null && executorSymbol in value && (value as Executor<T>)[executorSymbol];
}

export interface Executor<T> {
  [executorSymbol]: true
  factory: (dependencies: unknown, scope: Scope) => T | Promise<T>
  dependencies: Executor<unknown>[] | Record<string, Executor<unknown>> | undefined
}

export interface Scope {
  readonly isDisposed: boolean
  get<T>(executor: Executor<T>): GetAccessor<T> | undefined

  resolve<T>(executor: Executor<T>): Promise<GetAccessor<T>>
  update<T>(executor: Executor<MutableOutput<T> | Promise<MutableOutput<T>>>, updateFn: (current: T) => T): Promise<void>
  reset<T>(executor: Executor<T>): Promise<void>
  release(executor: Executor<EffectOutput | ResourceOutput<any>>): Promise<void>

  dispose(): Promise<void>
  on<T>(executor: Executor<T>, listener: (value: T) => void): Cleanup
  once<T>(executor: Executor<T>): Promise<void>
}

export interface ScopeInner {
  getValues(): Map<Executor<unknown>, Container>
  getDependencyMap(): Map<Executor<unknown>, Set<Executor<unknown>>>
  getCleanups(): Map<Executor<unknown>, Cleanup>
}

export const createScope = (): Scope => {
  return new BaseScope()
}

export const provide = <T>(factory: (scope: Scope) => T): Executor<T> => {
  return {
    factory: (_, scope) => factory(scope),
    dependencies: [],
    [executorSymbol]: true
  }
}

export const derive = <T extends Array<unknown> | object, R>(
  dependencies: { [K in keyof T]: Executor<T[K]> },
  factory: (dependency: { [K in keyof T]: GetAccessor<T[K]> }, scope: Scope) => R | Promise<R>
): Executor<R> => {
  return {
    factory: (dependencies, scope) => factory(dependencies as any, scope),
    dependencies,
    [executorSymbol]: true
  }
}

export const ref = <T>(executor: Executor<T>): Executor<Executor<T>> => {
  return {
    factory: async (_, scope) => {
      await scope.resolve(executor)

      return executor
    },
    dependencies: [],
    [executorSymbol]: true
  }
}

export const errors = {
  scopeDisposed: () => new Error('Scope is disposed'),
  unResolved: () => new Error('Executor is not resolved'),
  notMutableExecutor: () => new Error('Reference executor is not mutable'),
  executorIsBeingResolved: () => new Error('Executor is being resolved')
}

type ResolvedContainer = { kind: 'resolved', value: unknown }
type PendingContainer = { kind: 'pending', promise: Promise<unknown> }
type UpdatingContainer = { kind: 'updating', promise: Promise<unknown>, value: unknown }

type Container = ResolvedContainer | PendingContainer | UpdatingContainer

class BaseScope implements Scope, ScopeInner {
  #disposed = false
  #values = new Map<Executor<unknown>, Container>()
  #dependencyMap = new Map<Executor<unknown>, Set<Executor<unknown>>>()
  #cleanups = new Map<Executor<unknown>, Cleanup>()
  #listeners = new Map<Executor<unknown>, Set<(value: unknown) => void>>()

  #ensureNotDisposed() {
    if (this.#disposed) {
      throw errors.scopeDisposed()
    }
  }

  #makeGetAccessor<T>(executor: Executor<T>) {
    return getAccessor(() => {
      this.#ensureNotDisposed()

      const container = this.#values.get(executor)
      if (container === undefined) {
        throw errors.unResolved()
      }

      if (container.kind === 'pending') {
        throw errors.unResolved()
      }

      return container.value as T
    })
  }

  get isDisposed(): boolean {
    return this.#disposed
  }

  get<T>(executor: Executor<T>): GetAccessor<T> | undefined {
    this.#ensureNotDisposed()

    const container = this.#values.get(executor)
    if (container === undefined) {
      return undefined
    }

    if (container.kind === 'resolved') {
      return this.#makeGetAccessor(executor as Executor<T>)
    }
  }

  async #resolveDependencyArray(dependencies: Executor<unknown>[]): Promise<unknown[]> {
    const result: unknown[] = []
    for (const dep of dependencies) {
      result.push(await this.resolve(dep))
    }
    return result
  }

  async #resolveDependencyObject(dependencies: Record<string, Executor<unknown>>): Promise<Record<string, unknown>> {
    const result = {} as Record<string, unknown>
    for (const [key, dep] of Object.entries(dependencies)) {
      result[key] = await this.resolve(dep)
    }

    return result
  }

  async #resolveDependency(dependencies: Executor<unknown>[] | Record<string, Executor<unknown>> | undefined): Promise<unknown> {
    if (!dependencies) return undefined

    return Array.isArray(dependencies)
      ? await this.#resolveDependencyArray(dependencies)
      : await this.#resolveDependencyObject(dependencies)
  }

  #trackDependencies(executor: Executor<unknown>): void {
    if (executor.dependencies === undefined) return

    for (const dependency of Object.values(executor.dependencies)) {
      const currentSet = this.#dependencyMap.get(dependency)
      if (currentSet === undefined) {
        this.#dependencyMap.set(dependency, new Set([executor]))
      } else {
        currentSet.add(executor)
      }
    }
  }

  async #evalute<T>(executor: Executor<T>): Promise<GetAccessor<T>> {
    this.#ensureNotDisposed()
    let container = this.#values.get(executor) || { kind: 'pending', promise: null as any } as Container

    const willResolve = new Promise(async (resolve, reject) => {
      try {
        const dependencies = await this.#resolveDependency(executor.dependencies)
        this.#trackDependencies(executor)

        const value = await executor.factory(dependencies, this)
        if (isEffectOutput(value) || isResouceOutput(value)) {
          this.#cleanups.set(executor, value.cleanup)
        }

        if (isOutput(value)) {
          Object.assign(container, { kind: 'resolved', value: value.value })
        } else {
          Object.assign(container, { kind: 'resolved', value })
        }

        this.#triggerEvent(executor, value)
        resolve(this.#makeGetAccessor(executor))
      } catch (error) {
        this.#values.delete(executor)
        reject(error)
      }
    })

    Object.assign(container, { promise: willResolve })
    this.#values.set(executor, container)

    return await willResolve as Promise<GetAccessor<T>>
  }

  async #reevaluateDependencies(executor: Executor<unknown>): Promise<void> {
    const dependents = this.#dependencyMap.get(executor)
    const promises = [] as Promise<unknown>[]

    if (dependents !== undefined) {
      for (const dependent of dependents) {
        const cleanup = this.#cleanups.get(dependent)
        if (cleanup !== undefined) {
          cleanup()
        }

        promises.push(this.#evalute(dependent))
        promises.push(this.#reevaluateDependencies(dependent))
      }
    }

    await Promise.all(promises)
  }

  resolve<T>(executor: Executor<T>): Promise<GetAccessor<T>> {
    this.#ensureNotDisposed()

    const container = this.#values.get(executor)
    if (container !== undefined) {
      if (container.kind === 'resolved') {
        return Promise.resolve(this.#makeGetAccessor(executor))
      }

      return container.promise as Promise<GetAccessor<T>>
    }

    return this.#evalute(executor)
  }

  async update<T>(executor: Executor<MutableOutput<T>>, updateFn: (current: T) => T): Promise<void> {
    this.#ensureNotDisposed()

    let container = this.#values.get(executor)
    if (container === undefined) {
      await this.resolve(executor)
      container = this.#values.get(executor)!
    }

    if (container.kind === 'pending') {
      throw errors.executorIsBeingResolved()
    }

    if (container.kind === 'updating') {
      await container.promise
    }

    const currentCleanup = this.#cleanups.get(executor)
    if (currentCleanup !== undefined) {
      currentCleanup()
    }

    const promise = Promise.resolve()
      .then(() => {
        const value = updateFn(container.value as T)
        Object.assign(container, { kind: 'resolved', value })

        return value
      })
      .then(async (value) => {
        await this.#reevaluateDependencies(executor)
        this.#triggerEvent(executor, value as unknown)
      })

    Object.assign(container, { kind: 'updating', promise, value: container.value })

    return await promise
  }

  async reset<T>(executor: Executor<T>): Promise<void> {
    this.#ensureNotDisposed()
    const container = this.#values.get(executor)
    if (container === undefined) {
      throw errors.unResolved()
    }

    if (container.kind === 'pending' || container.kind === 'updating') {
      await container.promise
    }

    const currentCleanup = this.#cleanups.get(executor)
    if (currentCleanup !== undefined) {
      currentCleanup()
    }

    const promise = this.#evalute(executor)
      .then(async (value) => {
        await this.#reevaluateDependencies(executor)
        return value
      })
      .then((value) => {
        Object.assign(container, { kind: 'resolved', value })
        this.#triggerEvent(executor, value as unknown)
      })

    Object.assign(container, { kind: 'pending', promise })

    return await promise
  }

  async release(executor: Executor<EffectOutput | ResourceOutput<any>>): Promise<void> {
    this.#ensureNotDisposed()

    const container = this.#values.get(executor)
    if (container === undefined) {
      throw errors.unResolved()
    }

    if (container.kind === 'pending' || container.kind === 'updating') {
      await container.promise
    }

    const currentCleanup = this.#cleanups.get(executor)
    if (currentCleanup !== undefined) {
      currentCleanup()
    }
  }

  async dispose(): Promise<void> {
    this.#ensureNotDisposed()
    this.#cleanups.forEach(cleanup => cleanup())
    this.#cleanups.clear()
    this.#listeners.clear()

    this.#values.clear()
    this.#dependencyMap.clear()

    queueMicrotask(() => {
      this.#disposed = true
    })
  }

  #triggerEvent<T>(executor: Executor<T>, value: T) {
    const listeners = this.#listeners.get(executor)
    if (listeners !== undefined) {
      listeners.forEach(listener => listener(value))
    }
  }

  on<T>(executor: Executor<T>, listener: (value: T) => void): Cleanup {
    this.#ensureNotDisposed()

    let listeners = this.#listeners.get(executor)
    if (listeners === undefined) {
      listeners = new Set()
      this.#listeners.set(executor, listeners)
    }

    listeners.add(listener as (value: unknown) => void)
    return () => {
      listeners?.delete(listener as (value: unknown) => void)
    }
  }

  once<T>(executor: Executor<T>): Promise<void> {
    this.#ensureNotDisposed()
    return new Promise(resolve => {
      const cleanup = this.on(executor, () => {
        cleanup()
        resolve()
      })
    })
  }

  /** SCOPE INNER */
  getValues(): Map<Executor<unknown>, Container> {
    return this.#values
  }

  getDependencyMap(): Map<Executor<unknown>, Set<Executor<unknown>>> {
    return this.#dependencyMap
  }

  getCleanups(): Map<Executor<unknown>, Cleanup> {
    return this.#cleanups
  }
}