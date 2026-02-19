import { useCallback, useEffect, useRef, useState } from 'preact/hooks'

interface CacheEntry<T> {
  data: T
  timestamp: number
}

interface InFlightEntry {
  promise: Promise<unknown>
  controller: AbortController
  subscribers: number
}

const cache = new Map<string, CacheEntry<unknown>>()
const inFlight = new Map<string, InFlightEntry>()

const DEFAULT_TTL = 5000

export interface UseFetchResult<T> {
  data: T | undefined
  error: Error | undefined
  isLoading: boolean
  mutate: () => void
}

function getCached<T>(url: string, ttl: number): T | undefined {
  const cached = cache.get(url) as CacheEntry<T> | undefined
  if (cached && Date.now() - cached.timestamp < ttl) {
    return cached.data
  }
  return undefined
}

export function useFetch<T>(url: string, ttl = DEFAULT_TTL): UseFetchResult<T> {
  const [data, setData] = useState<T | undefined>(() => getCached<T>(url, ttl))
  const [error, setError] = useState<Error | undefined>(undefined)
  const [isLoading, setIsLoading] = useState<boolean>(() => getCached<T>(url, ttl) === undefined)

  const urlRef = useRef(url)
  urlRef.current = url

  const fetchData = useCallback(async () => {
    setIsLoading(true)
    setError(undefined)

    const existing = inFlight.get(url)
    if (existing) {
      existing.subscribers++
      try {
        const result = (await existing.promise) as T
        if (urlRef.current === url) {
          setData(result)
          setIsLoading(false)
        }
      } catch (err) {
        if (urlRef.current === url) {
          setError(err instanceof Error ? err : new Error(String(err)))
          setIsLoading(false)
        }
      }
      return
    }

    const controller = new AbortController()
    const promise = fetch(url, { signal: controller.signal })
      .then(async (res) => {
        if (!res.ok) {
          const body = await res.json()
          throw new Error(body.error?.message ?? `HTTP ${res.status}`)
        }
        return res.json()
      })
      .finally(() => {
        inFlight.delete(url)
      })

    inFlight.set(url, { promise, controller, subscribers: 1 })

    try {
      const result = (await promise) as T
      cache.set(url, { data: result, timestamp: Date.now() })
      if (urlRef.current === url) {
        setData(result)
        setIsLoading(false)
      }
    } catch (err) {
      if (urlRef.current === url) {
        setError(err instanceof Error ? err : new Error(String(err)))
        setIsLoading(false)
      }
    }
  }, [url])

  useEffect(() => {
    if (getCached<T>(url, ttl) !== undefined) {
      setData(getCached<T>(url, ttl))
      setIsLoading(false)
      return
    }
    fetchData()
    return () => {
      const entry = inFlight.get(url)
      if (entry) {
        entry.subscribers--
        if (entry.subscribers <= 0) {
          entry.controller.abort()
          inFlight.delete(url)
        }
      }
    }
  }, [url, fetchData, ttl])

  const mutate = useCallback(() => {
    cache.delete(url)
    fetchData()
  }, [url, fetchData])

  return { data, error, isLoading, mutate }
}

/** Invalidate all cache entries whose key starts with the given prefix. */
export function invalidateCache(prefix: string): void {
  for (const key of cache.keys()) {
    if (key.startsWith(prefix)) {
      cache.delete(key)
    }
  }
}

/** Clear the entire cache. Useful in tests. */
export function clearCache(): void {
  cache.clear()
  inFlight.clear()
}
