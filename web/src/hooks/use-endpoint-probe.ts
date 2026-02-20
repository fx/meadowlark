import { useEffect, useRef, useState } from 'preact/hooks'
import type { ProbeModel, ProbeVoice } from '@/lib/api'

export interface UseEndpointProbeResult {
  models: ProbeModel[]
  voices: ProbeVoice[]
  loading: boolean
  error: string | undefined
}

function isValidUrl(url: string): boolean {
  return url.startsWith('http://') || url.startsWith('https://')
}

export function useEndpointProbe(url: string, apiKey: string): UseEndpointProbeResult {
  const [models, setModels] = useState<ProbeModel[]>([])
  const [voices, setVoices] = useState<ProbeVoice[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | undefined>(undefined)
  const abortRef = useRef<AbortController | null>(null)

  useEffect(() => {
    // Abort any in-flight request from previous render
    abortRef.current?.abort()
    abortRef.current = null

    if (!isValidUrl(url)) {
      setModels([])
      setVoices([])
      setLoading(false)
      setError(undefined)
      return
    }

    setError(undefined)

    const controller = new AbortController()
    abortRef.current = controller

    const timer = setTimeout(() => {
      setLoading(true)
      fetch('/api/v1/endpoints/probe', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ url, api_key: apiKey }),
        signal: controller.signal,
      })
        .then(async (res) => {
          if (!res.ok) {
            const body = await res.json()
            throw new Error(body.error?.message ?? `HTTP ${res.status}`)
          }
          return res.json()
        })
        .then((data) => {
          if (!controller.signal.aborted) {
            setModels(data.models ?? [])
            setVoices(data.voices ?? [])
            setLoading(false)
          }
        })
        .catch((err) => {
          if (err instanceof DOMException && err.name === 'AbortError') return
          setError(err instanceof Error ? err.message : String(err))
          setModels([])
          setVoices([])
          setLoading(false)
        })
    }, 500)

    return () => {
      clearTimeout(timer)
      controller.abort()
    }
  }, [url, apiKey])

  return { models, voices, loading, error }
}
