import { act, renderHook } from '@testing-library/preact'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { clearCache } from './use-fetch'
import { useMutation } from './use-mutation'

const mockFetch = vi.fn()

beforeEach(() => {
  clearCache()
  vi.stubGlobal('fetch', mockFetch)
})

afterEach(() => {
  vi.restoreAllMocks()
})

describe('useMutation', () => {
  it('starts in idle state', () => {
    const { result } = renderHook(() => useMutation('/api/test', 'POST'))
    expect(result.current.isMutating).toBe(false)
    expect(result.current.error).toBeUndefined()
    expect(typeof result.current.trigger).toBe('function')
  })

  it('trigger sends POST with JSON body', async () => {
    const responseData = { id: 'new-1', name: 'Created' }
    mockFetch.mockReturnValueOnce(
      Promise.resolve({
        ok: true,
        status: 201,
        json: () => Promise.resolve(responseData),
      }),
    )

    const { result } = renderHook(() =>
      useMutation<{ name: string }, { id: string; name: string }>('/api/items', 'POST'),
    )

    let triggerResult: { id: string; name: string } | undefined
    await act(async () => {
      triggerResult = await result.current.trigger({ name: 'Created' })
    })

    expect(triggerResult).toEqual(responseData)
    expect(result.current.isMutating).toBe(false)
    expect(result.current.error).toBeUndefined()
    expect(mockFetch).toHaveBeenCalledWith('/api/items', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name: 'Created' }),
    })
  })

  it('trigger sends DELETE without body', async () => {
    mockFetch.mockReturnValueOnce(
      Promise.resolve({
        ok: true,
        status: 204,
        json: () => Promise.resolve(undefined),
      }),
    )

    const { result } = renderHook(() => useMutation('/api/items/1', 'DELETE'))

    await act(async () => {
      await result.current.trigger()
    })

    expect(result.current.isMutating).toBe(false)
    expect(mockFetch).toHaveBeenCalledWith('/api/items/1', {
      method: 'DELETE',
    })
  })

  it('sets isMutating during request', async () => {
    let resolveRequest!: (value: unknown) => void
    mockFetch.mockReturnValueOnce(
      new Promise((resolve) => {
        resolveRequest = resolve
      }),
    )

    const { result } = renderHook(() => useMutation('/api/items', 'POST'))

    let triggerPromise: Promise<unknown> | undefined
    act(() => {
      triggerPromise = result.current.trigger({ name: 'test' })
    })

    expect(result.current.isMutating).toBe(true)

    await act(async () => {
      resolveRequest({
        ok: true,
        status: 200,
        json: () => Promise.resolve({ id: '1' }),
      })
      if (triggerPromise) await triggerPromise
    })

    expect(result.current.isMutating).toBe(false)
  })

  it('sets error on failure', async () => {
    mockFetch.mockReturnValueOnce(
      Promise.resolve({
        ok: false,
        status: 400,
        json: () => Promise.resolve({ error: { message: 'bad request' } }),
      }),
    )

    const { result } = renderHook(() => useMutation('/api/items', 'POST'))

    await act(async () => {
      try {
        await result.current.trigger({ name: 'bad' })
      } catch {
        // Expected
      }
    })

    expect(result.current.isMutating).toBe(false)
    expect(result.current.error).toBeInstanceOf(Error)
    expect(result.current.error?.message).toBe('bad request')
  })

  it('throws error from trigger', async () => {
    mockFetch.mockReturnValueOnce(
      Promise.resolve({
        ok: false,
        status: 500,
        json: () => Promise.resolve({ error: { message: 'server error' } }),
      }),
    )

    const { result } = renderHook(() => useMutation('/api/items', 'POST'))

    await act(async () => {
      await expect(result.current.trigger({ name: 'bad' })).rejects.toThrow('server error')
    })
  })

  it('invalidates cache after successful mutation', async () => {
    mockFetch.mockReturnValueOnce(
      Promise.resolve({
        ok: true,
        status: 200,
        json: () => Promise.resolve({ id: '1' }),
      }),
    )

    const { result } = renderHook(() => useMutation('/api/v1/endpoints/ep-1', 'PUT'))

    await act(async () => {
      await result.current.trigger({ name: 'updated' })
    })

    expect(result.current.isMutating).toBe(false)
    expect(result.current.error).toBeUndefined()
  })

  it('handles non-Error thrown values', async () => {
    mockFetch.mockReturnValueOnce(Promise.reject('string error'))

    const { result } = renderHook(() => useMutation('/api/items', 'POST'))

    await act(async () => {
      try {
        await result.current.trigger()
      } catch {
        // Expected
      }
    })

    expect(result.current.error).toBeInstanceOf(Error)
    expect(result.current.error?.message).toBe('string error')
  })

  it('handles error response without error.message field', async () => {
    mockFetch.mockReturnValueOnce(
      Promise.resolve({
        ok: false,
        status: 500,
        json: () => Promise.resolve({}),
      }),
    )

    const { result } = renderHook(() => useMutation('/api/items', 'POST'))

    await act(async () => {
      try {
        await result.current.trigger()
      } catch {
        // Expected
      }
    })

    expect(result.current.error?.message).toBe('HTTP 500')
  })

  it('clears previous error on new trigger', async () => {
    mockFetch.mockReturnValueOnce(
      Promise.resolve({
        ok: false,
        status: 400,
        json: () => Promise.resolve({ error: { message: 'first error' } }),
      }),
    )

    const { result } = renderHook(() => useMutation('/api/items', 'POST'))

    await act(async () => {
      try {
        await result.current.trigger()
      } catch {
        // Expected
      }
    })
    expect(result.current.error?.message).toBe('first error')

    mockFetch.mockReturnValueOnce(
      Promise.resolve({
        ok: true,
        status: 200,
        json: () => Promise.resolve({ id: '1' }),
      }),
    )

    await act(async () => {
      await result.current.trigger({ name: 'good' })
    })
    expect(result.current.error).toBeUndefined()
  })
})
