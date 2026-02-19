import { useCallback, useState } from 'preact/hooks'
import { invalidateCache } from './use-fetch'

export interface UseMutationResult<TInput, TOutput> {
  trigger: (body?: TInput) => Promise<TOutput>
  isMutating: boolean
  error: Error | undefined
}

export function useMutation<TInput = void, TOutput = void>(
  url: string,
  method: 'POST' | 'PUT' | 'DELETE',
): UseMutationResult<TInput, TOutput> {
  const [isMutating, setIsMutating] = useState(false)
  const [error, setError] = useState<Error | undefined>(undefined)

  const trigger = useCallback(
    async (body?: TInput): Promise<TOutput> => {
      setIsMutating(true)
      setError(undefined)
      try {
        const options: RequestInit = { method }
        if (body !== undefined) {
          options.headers = { 'Content-Type': 'application/json' }
          options.body = JSON.stringify(body)
        }
        const res = await fetch(url, options)
        if (res.status === 204) {
          // Invalidate related cache entries based on the URL path
          const basePath = url.replace(/\/[^/]+$/, '')
          invalidateCache(basePath)
          setIsMutating(false)
          return undefined as TOutput
        }
        const data = await res.json()
        if (!res.ok) {
          throw new Error(data.error?.message ?? `HTTP ${res.status}`)
        }
        // Invalidate related cache entries based on the URL path
        const basePath = url.replace(/\/[^/]+$/, '')
        invalidateCache(basePath)
        setIsMutating(false)
        return data as TOutput
      } catch (err) {
        const error = err instanceof Error ? err : new Error(String(err))
        setError(error)
        setIsMutating(false)
        throw error
      }
    },
    [url, method],
  )

  return { trigger, isMutating, error }
}
