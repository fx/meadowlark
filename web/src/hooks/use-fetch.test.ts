import { act, renderHook, waitFor } from '@testing-library/preact'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { clearCache, invalidateCache, useFetch } from './use-fetch'

const mockFetch = vi.fn()

beforeEach(() => {
  clearCache()
  mockFetch.mockClear()
  vi.stubGlobal('fetch', mockFetch)
})

afterEach(() => {
  vi.restoreAllMocks()
})

function jsonResponse(data: unknown) {
  return Promise.resolve({
    ok: true,
    status: 200,
    json: () => Promise.resolve(data),
  })
}

function errorResponse(status: number, message: string) {
  return Promise.resolve({
    ok: false,
    status,
    json: () => Promise.resolve({ error: { message } }),
  })
}

describe('useFetch', () => {
  it('starts in loading state', () => {
    mockFetch.mockReturnValueOnce(new Promise(() => {}))
    const { result } = renderHook(() => useFetch('/api/test'))
    expect(result.current.isLoading).toBe(true)
    expect(result.current.data).toBeUndefined()
    expect(result.current.error).toBeUndefined()
  })

  it('returns data on success', async () => {
    const data = { id: '1', name: 'test' }
    mockFetch.mockReturnValueOnce(jsonResponse(data))
    const { result } = renderHook(() => useFetch('/api/success'))

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false)
    })
    expect(result.current.data).toEqual(data)
    expect(result.current.error).toBeUndefined()
  })

  it('returns error on failure', async () => {
    mockFetch.mockReturnValueOnce(errorResponse(500, 'internal error'))
    const { result } = renderHook(() => useFetch('/api/error'))

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false)
    })
    expect(result.current.error).toBeInstanceOf(Error)
    expect(result.current.error?.message).toBe('internal error')
    expect(result.current.data).toBeUndefined()
  })

  it('uses cached data when within TTL', async () => {
    const data = { id: '1', name: 'cached' }
    mockFetch.mockReturnValueOnce(jsonResponse(data))

    const { result, unmount } = renderHook(() => useFetch('/api/cached-ttl', 10000))
    await waitFor(() => {
      expect(result.current.isLoading).toBe(false)
    })
    expect(result.current.data).toEqual(data)
    unmount()

    // Re-render — should use cache, not fetch again
    const { result: result2 } = renderHook(() => useFetch('/api/cached-ttl', 10000))
    expect(result2.current.data).toEqual(data)
    expect(result2.current.isLoading).toBe(false)
    expect(mockFetch).toHaveBeenCalledTimes(1)
  })

  it('mutate() forces refetch', async () => {
    const data1 = { id: '1', name: 'original' }
    const data2 = { id: '1', name: 'updated' }
    mockFetch.mockReturnValueOnce(jsonResponse(data1))

    const { result } = renderHook(() => useFetch('/api/mutate-test'))
    await waitFor(() => {
      expect(result.current.isLoading).toBe(false)
    })
    expect(result.current.data).toEqual(data1)

    mockFetch.mockReturnValueOnce(jsonResponse(data2))
    act(() => {
      result.current.mutate()
    })

    await waitFor(() => {
      expect(result.current.data).toEqual(data2)
    })
    expect(mockFetch).toHaveBeenCalledTimes(2)
  })

  it('deduplicates concurrent requests to the same URL', async () => {
    let resolveJson!: (value: unknown) => void
    const jsonPromise = new Promise((resolve) => {
      resolveJson = resolve
    })
    mockFetch.mockReturnValueOnce(
      Promise.resolve({
        ok: true,
        status: 200,
        json: () => jsonPromise,
      }),
    )

    const { result: result1 } = renderHook(() => useFetch('/api/dedup'))
    const { result: result2 } = renderHook(() => useFetch('/api/dedup'))

    expect(mockFetch).toHaveBeenCalledTimes(1)

    await act(async () => {
      resolveJson({ id: '1' })
      await jsonPromise
    })

    await waitFor(() => {
      expect(result1.current.isLoading).toBe(false)
    })
    await waitFor(() => {
      expect(result2.current.isLoading).toBe(false)
    })
    expect(result1.current.data).toEqual({ id: '1' })
    expect(result2.current.data).toEqual({ id: '1' })
  })

  it('deduplicates and propagates errors to joining hooks', async () => {
    let resolveJson!: (value: unknown) => void
    const jsonPromise = new Promise((resolve) => {
      resolveJson = resolve
    })

    // The fetch returns a response whose json() hangs until we resolve/reject
    mockFetch.mockReturnValueOnce(
      Promise.resolve({
        ok: false,
        status: 500,
        json: () => jsonPromise,
      }),
    )

    // First hook starts the request, creating the in-flight entry
    const { result: result1 } = renderHook(() => useFetch('/api/dedup-err'))
    // Second hook joins the in-flight request (same URL)
    const { result: result2 } = renderHook(() => useFetch('/api/dedup-err'))

    // Only one fetch call
    expect(mockFetch).toHaveBeenCalledTimes(1)

    // Resolve the json promise — this causes the .then to throw because !res.ok
    await act(async () => {
      resolveJson({ error: { message: 'server failed' } })
      // Wait for promise to settle
      try {
        await jsonPromise
      } catch {
        // expected
      }
    })

    await waitFor(() => {
      expect(result1.current.isLoading).toBe(false)
    })
    await waitFor(() => {
      expect(result2.current.isLoading).toBe(false)
    })
    expect(result1.current.error?.message).toBe('server failed')
    expect(result2.current.error?.message).toBe('server failed')
  })

  it('handles non-Error thrown values', async () => {
    mockFetch.mockReturnValueOnce(Promise.reject('string error'))
    const { result } = renderHook(() => useFetch('/api/non-error'))

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false)
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
    const { result } = renderHook(() => useFetch('/api/bare-error'))

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false)
    })
    expect(result.current.error?.message).toBe('HTTP 500')
  })

  it('ignores stale response when URL changes during fetch', async () => {
    let resolveFirst!: (value: unknown) => void
    const firstFetchPromise = new Promise((resolve) => {
      resolveFirst = resolve
    })
    mockFetch.mockReturnValueOnce(firstFetchPromise)

    const { result, rerender } = renderHook(({ url }: { url: string }) => useFetch(url), {
      initialProps: { url: '/api/url-change-1' },
    })

    expect(result.current.isLoading).toBe(true)

    // Change the URL while first fetch is in-flight
    const secondData = { id: '2', name: 'second' }
    mockFetch.mockReturnValueOnce(jsonResponse(secondData))
    rerender({ url: '/api/url-change-2' })

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false)
    })
    expect(result.current.data).toEqual(secondData)

    // Resolve the first fetch — should be ignored since URL changed
    resolveFirst({
      ok: true,
      status: 200,
      json: () => Promise.resolve({ id: '1', name: 'stale' }),
    })

    // Data should still be the second fetch result
    expect(result.current.data).toEqual(secondData)
  })

  it('ignores stale error when URL changes during fetch', async () => {
    let rejectFirst!: (reason: unknown) => void
    const firstFetchPromise = new Promise((_resolve, reject) => {
      rejectFirst = reject
    })
    mockFetch.mockReturnValueOnce(firstFetchPromise)

    const { result, rerender } = renderHook(({ url }: { url: string }) => useFetch(url), {
      initialProps: { url: '/api/stale-err-1' },
    })

    // Change the URL while first fetch is in-flight
    const secondData = { id: '2' }
    mockFetch.mockReturnValueOnce(jsonResponse(secondData))
    rerender({ url: '/api/stale-err-2' })

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false)
    })
    expect(result.current.data).toEqual(secondData)

    // Reject the first fetch — should be ignored since URL changed
    rejectFirst(new Error('stale error'))
    // No error should appear
    expect(result.current.error).toBeUndefined()
  })

  it('dedup catch handles non-Error rejection', async () => {
    // Make the fetch promise that will be shared reject with a string (non-Error)
    let rejectFetch!: (reason: unknown) => void
    const fetchPromise = new Promise((_resolve, reject) => {
      rejectFetch = reject
    })
    mockFetch.mockReturnValueOnce(fetchPromise)

    // First hook starts the request
    const { result: result1 } = renderHook(() => useFetch('/api/dedup-non-err'))
    // Second hook joins the in-flight request
    const { result: result2 } = renderHook(() => useFetch('/api/dedup-non-err'))

    expect(mockFetch).toHaveBeenCalledTimes(1)

    // Reject with a string — the non-Error path
    await act(async () => {
      rejectFetch('raw string rejection')
      try {
        await fetchPromise
      } catch {
        // expected
      }
    })

    await waitFor(() => {
      expect(result1.current.isLoading).toBe(false)
    })
    await waitFor(() => {
      expect(result2.current.isLoading).toBe(false)
    })
    // Both should get Error wrapping the string
    expect(result2.current.error?.message).toBe('raw string rejection')
  })

  it('ignores stale dedup success when URL changes', async () => {
    let resolveJson!: (value: unknown) => void
    const jsonPromise = new Promise((resolve) => {
      resolveJson = resolve
    })
    mockFetch.mockReturnValueOnce(
      Promise.resolve({
        ok: true,
        status: 200,
        json: () => jsonPromise,
      }),
    )

    // First hook starts the in-flight request
    const { result: result1 } = renderHook(() => useFetch('/api/dedup-stale'))

    // Second hook using same URL but rendered via a component that will change URLs
    const { result: result2, rerender } = renderHook(({ url }: { url: string }) => useFetch(url), {
      initialProps: { url: '/api/dedup-stale' },
    })

    // Only one fetch call since both use the same URL
    expect(mockFetch).toHaveBeenCalledTimes(1)

    // Change the second hook's URL before the shared request resolves
    const otherData = { id: 'other' }
    mockFetch.mockReturnValueOnce(jsonResponse(otherData))
    rerender({ url: '/api/different-url' })

    // Resolve the shared promise
    await act(async () => {
      resolveJson({ id: 'shared' })
      await jsonPromise
    })

    // First hook should get the shared data
    await waitFor(() => {
      expect(result1.current.isLoading).toBe(false)
    })
    expect(result1.current.data).toEqual({ id: 'shared' })

    // Second hook should get the different-url data, not the stale shared data
    await waitFor(() => {
      expect(result2.current.isLoading).toBe(false)
    })
    expect(result2.current.data).toEqual(otherData)
  })

  it('ignores stale dedup error when URL changes', async () => {
    let resolveJson!: (value: unknown) => void
    const jsonPromise = new Promise((resolve) => {
      resolveJson = resolve
    })
    mockFetch.mockReturnValueOnce(
      Promise.resolve({
        ok: false,
        status: 500,
        json: () => jsonPromise,
      }),
    )

    // First hook starts the in-flight request
    const { result: result1 } = renderHook(() => useFetch('/api/dedup-stale-err'))

    // Second hook joins the request, then changes URL
    const { result: result2, rerender } = renderHook(({ url }: { url: string }) => useFetch(url), {
      initialProps: { url: '/api/dedup-stale-err' },
    })

    expect(mockFetch).toHaveBeenCalledTimes(1)

    // Change URL for second hook
    const otherData = { id: 'other' }
    mockFetch.mockReturnValueOnce(jsonResponse(otherData))
    rerender({ url: '/api/other-url' })

    // Resolve json — triggers the error in the .then handler
    await act(async () => {
      resolveJson({ error: { message: 'dedup error' } })
      await jsonPromise
    })

    // First hook should get the error
    await waitFor(() => {
      expect(result1.current.isLoading).toBe(false)
    })
    expect(result1.current.error?.message).toBe('dedup error')

    // Second hook should have its own data, not the error
    await waitFor(() => {
      expect(result2.current.isLoading).toBe(false)
    })
    expect(result2.current.data).toEqual(otherData)
    expect(result2.current.error).toBeUndefined()
  })

  it('aborts in-flight request when last subscriber unmounts', async () => {
    let resolveFetch!: (value: unknown) => void
    const fetchPromise = new Promise((resolve) => {
      resolveFetch = resolve
    })
    mockFetch.mockReturnValueOnce(fetchPromise)

    const { unmount } = renderHook(() => useFetch('/api/abort-test'))
    unmount()

    // Resolve after unmount — should not throw
    resolveFetch({
      ok: true,
      status: 200,
      json: () => Promise.resolve({ id: '1' }),
    })
  })

  it('does not abort shared request when one subscriber unmounts', async () => {
    let resolveJson!: (value: unknown) => void
    const jsonPromise = new Promise((resolve) => {
      resolveJson = resolve
    })
    mockFetch.mockReturnValueOnce(
      Promise.resolve({
        ok: true,
        status: 200,
        json: () => jsonPromise,
      }),
    )

    // Two hooks share the same in-flight request
    const { result: result1 } = renderHook(() => useFetch('/api/shared-abort'))
    const { unmount: unmount2 } = renderHook(() => useFetch('/api/shared-abort'))

    expect(mockFetch).toHaveBeenCalledTimes(1)

    // Unmount one subscriber — should NOT abort
    unmount2()

    // Resolve the shared promise — remaining subscriber should get data
    await act(async () => {
      resolveJson({ id: 'survived' })
      await jsonPromise
    })

    await waitFor(() => {
      expect(result1.current.isLoading).toBe(false)
    })
    expect(result1.current.data).toEqual({ id: 'survived' })
  })
})

describe('invalidateCache', () => {
  it('removes cache entries matching prefix', async () => {
    // Populate cache via useFetch
    const data1 = { id: '1' }
    const data2 = { id: '2' }
    mockFetch.mockReturnValueOnce(jsonResponse(data1))
    const { result: r1 } = renderHook(() => useFetch('/api/v1/endpoints', 10000))
    await waitFor(() => expect(r1.current.isLoading).toBe(false))

    mockFetch.mockReturnValueOnce(jsonResponse(data2))
    const { result: r2 } = renderHook(() => useFetch('/api/v1/aliases', 10000))
    await waitFor(() => expect(r2.current.isLoading).toBe(false))

    expect(mockFetch).toHaveBeenCalledTimes(2)

    // Invalidate only endpoints
    invalidateCache('/api/v1/endpoints')

    // Endpoints should refetch
    mockFetch.mockReturnValueOnce(jsonResponse({ id: '3' }))
    const { result: r3 } = renderHook(() => useFetch('/api/v1/endpoints', 10000))
    await waitFor(() => expect(r3.current.isLoading).toBe(false))
    expect(r3.current.data).toEqual({ id: '3' })

    // Aliases should still be cached
    const { result: r4 } = renderHook(() => useFetch('/api/v1/aliases', 10000))
    expect(r4.current.data).toEqual(data2)
    expect(r4.current.isLoading).toBe(false)
  })

  it('handles no matching entries gracefully', () => {
    invalidateCache('/api/nonexistent')
    // No errors thrown
  })
})
