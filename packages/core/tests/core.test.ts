import { describe, it, expect, vi, beforeEach } from 'vitest'
import { createScope, provide, derive, mutable, resource, effect } from '../src/core/core'

describe('core', () => {
  it('syntax', async () => {
    const stringValue = provide(async () => 'hello')
    const numberValue = provide(() => 1)

    const combinedObject = derive({ stringValue, numberValue }, async ({ stringValue, numberValue }) => {
      return { stringValue: stringValue.get(), numberValue: numberValue.get() }
    })

    const combinedArray = derive([stringValue, numberValue], async ([stringValue, numberValue]) => {
      return [stringValue.get(), numberValue.get()]
    })

    const scope = createScope()
    const [combinedObj, combinedArr] = await Promise.all([
      scope.resolve(combinedObject),
      scope.resolve(combinedArray)
    ])

    expect(combinedObj.get()).toEqual({ stringValue: 'hello', numberValue: 1 })
    expect(combinedArr.get()).toEqual(['hello', 1])
  })


  it('complex_scenario', async () => {
    const scope = createScope()
    const cleanup = vi.fn()

    const computedFn = vi.fn()

    // Setup a complex state graph with multiple dependencies
    const base = provide(async () => mutable({ count: 1, text: 'hello' }))
    const computed = derive([base], async ([v]) => {
      return v.get().count * 2
    })

    const resolvedComputed = await scope.resolve(computed)
    expect(resolvedComputed.get()).toBe(2)

    // Update the base value
    await scope.update(base, (v) => ({ ...v, count: 2 }))
    expect(resolvedComputed.get()).toBe(4)

  }, 10000) // Increase timeout

  it('errors_and_cleanup', async () => {
    const scope = createScope()
    const cleanups = {
      resource1: vi.fn(),
      resource2: vi.fn(),
      effect: vi.fn()
    }

    // Create a chain of resources and effects that might fail
    const base = provide(() => mutable(1))
    const resource1 = derive([base], ([v]) => resource(v.get(), cleanups.resource1))
    const resource2 = derive([resource1], ([v]) => resource(v.get() * 2, cleanups.resource2))
    const effectVal = derive([base], ([v]) => effect(cleanups.effect))

    // Test successful initialization
    await Promise.all([
      scope.resolve(resource1),
      scope.resolve(resource2),
      scope.resolve(effectVal)
    ])

    // Force cleanup by disposing
    await scope.dispose()
    expect(cleanups.resource1).toHaveBeenCalled()
    expect(cleanups.resource2).toHaveBeenCalled()
    expect(cleanups.effect).toHaveBeenCalled()
  })
})
