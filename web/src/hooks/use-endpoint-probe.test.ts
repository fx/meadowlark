import { renderHook, waitFor } from '@testing-library/preact'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useEndpointProbe } from './use-endpoint-probe'

const mockFetch = vi.fn()

beforeEach(() => {
  mockFetch.mockReset()
  vi.stubGlobal('fetch', mockFetch)
})

afterEach(() => {
  vi.restoreAllMocks()
})

function mockProbeOk(models: { id: string }[], voices: { id: string; name: string }[]) {
  mockFetch.mockResolvedValue({
    ok: true,
    status: 200,
    json: () => Promise.resolve({ models, voices }),
  })
}

const WAIT_OPTS = { timeout: 2000 }

describe('useEndpointProbe', () => {
  it('returns empty state for invalid URL', () => {
    const { result } = renderHook(() => useEndpointProbe('not-a-url', ''))
    expect(result.current.models).toEqual([])
    expect(result.current.voices).toEqual([])
    expect(result.current.loading).toBe(false)
    expect(result.current.error).toBeUndefined()
  })

  it('returns empty state for empty URL', () => {
    const { result } = renderHook(() => useEndpointProbe('', ''))
    expect(result.current.models).toEqual([])
    expect(result.current.loading).toBe(false)
  })

  it('probes valid URL after debounce', async () => {
    mockProbeOk([{ id: 'tts-1' }], [{ id: 'alloy', name: 'Alloy' }])

    const { result } = renderHook(() => useEndpointProbe('https://api.example.com/v1', 'sk-test'))
    expect(result.current.loading).toBe(true)

    await waitFor(() => {
      expect(result.current.loading).toBe(false)
    }, WAIT_OPTS)

    expect(mockFetch).toHaveBeenCalledWith(
      '/api/v1/endpoints/probe',
      expect.objectContaining({
        method: 'POST',
        body: JSON.stringify({ url: 'https://api.example.com/v1', api_key: 'sk-test' }),
      }),
    )
    expect(result.current.models).toEqual([{ id: 'tts-1' }])
    expect(result.current.voices).toEqual([{ id: 'alloy', name: 'Alloy' }])
  })

  it('sets error on probe failure', async () => {
    mockFetch.mockResolvedValue({
      ok: false,
      status: 500,
      json: () => Promise.resolve({ error: { message: 'server error' } }),
    })

    const { result } = renderHook(() => useEndpointProbe('https://api.example.com/v1', ''))

    await waitFor(() => {
      expect(result.current.error).toBe('server error')
    }, WAIT_OPTS)

    expect(result.current.models).toEqual([])
    expect(result.current.voices).toEqual([])
    expect(result.current.loading).toBe(false)
  })

  it('sets error on network failure', async () => {
    mockFetch.mockRejectedValue(new Error('Network error'))

    const { result } = renderHook(() => useEndpointProbe('http://localhost:9999', ''))

    await waitFor(() => {
      expect(result.current.error).toBe('Network error')
    }, WAIT_OPTS)

    expect(result.current.loading).toBe(false)
  })

  it('handles null models/voices in response', async () => {
    mockFetch.mockResolvedValue({
      ok: true,
      status: 200,
      json: () => Promise.resolve({ models: null, voices: null }),
    })

    const { result } = renderHook(() => useEndpointProbe('https://api.example.com/v1', ''))

    await waitFor(() => {
      expect(result.current.loading).toBe(false)
    }, WAIT_OPTS)

    expect(result.current.models).toEqual([])
    expect(result.current.voices).toEqual([])
  })

  it('clears results when URL becomes invalid', async () => {
    mockProbeOk([{ id: 'tts-1' }], [{ id: 'alloy', name: 'Alloy' }])

    const { result, rerender } = renderHook(({ url }) => useEndpointProbe(url, ''), {
      initialProps: { url: 'https://api.example.com/v1' },
    })

    await waitFor(() => {
      expect(result.current.models).toHaveLength(1)
    }, WAIT_OPTS)

    rerender({ url: 'not-valid' })

    expect(result.current.models).toEqual([])
    expect(result.current.voices).toEqual([])
    expect(result.current.loading).toBe(false)
  })

  it('handles error response without message field', async () => {
    mockFetch.mockResolvedValue({
      ok: false,
      status: 503,
      json: () => Promise.resolve({ error: {} }),
    })

    const { result } = renderHook(() => useEndpointProbe('https://api.example.com/v1', ''))

    await waitFor(() => {
      expect(result.current.error).toBe('HTTP 503')
    }, WAIT_OPTS)

    expect(result.current.loading).toBe(false)
  })

  it('ignores AbortError without setting error state', async () => {
    const abortError = new DOMException('The operation was aborted.', 'AbortError')
    mockFetch.mockRejectedValue(abortError)

    const { result } = renderHook(() => useEndpointProbe('https://api.example.com/v1', ''))

    // Wait for the debounce + fetch to fire
    await waitFor(() => {
      expect(mockFetch).toHaveBeenCalledTimes(1)
    }, WAIT_OPTS)

    // Give time for the catch handler to run
    await new Promise((r) => setTimeout(r, 50))

    // AbortError should be silently ignored — no error, still loading
    expect(result.current.error).toBeUndefined()
  })

  it('handles non-Error rejection values', async () => {
    mockFetch.mockRejectedValue('string rejection')

    const { result } = renderHook(() => useEndpointProbe('https://api.example.com/v1', ''))

    await waitFor(() => {
      expect(result.current.error).toBe('string rejection')
    }, WAIT_OPTS)

    expect(result.current.loading).toBe(false)
  })

  it('does not update state after abort', async () => {
    let resolveFetch: ((v: unknown) => void) | null = null
    mockFetch.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          resolveFetch = resolve
        }),
    )
    mockFetch.mockResolvedValue({
      ok: true,
      status: 200,
      json: () => Promise.resolve({ models: [{ id: 'new' }], voices: [] }),
    })

    const { result, rerender } = renderHook(({ url }) => useEndpointProbe(url, ''), {
      initialProps: { url: 'https://first.example.com/v1' },
    })

    await waitFor(() => {
      expect(mockFetch).toHaveBeenCalledTimes(1)
    }, WAIT_OPTS)

    rerender({ url: 'https://second.example.com/v1' })

    // Resolve the first (now aborted) request after the abort
    if (resolveFetch) {
      resolveFetch({
        ok: true,
        status: 200,
        json: () => Promise.resolve({ models: [{ id: 'old' }], voices: [] }),
      })
    }

    await waitFor(() => {
      expect(result.current.models).toEqual([{ id: 'new' }])
    }, WAIT_OPTS)
  })
})
