import { useCallback, useEffect, useRef, useState } from 'preact/hooks'
import type { ProbeModel, ProbeVoice } from '@/lib/api'

export type ProbeStatus = 'idle' | 'loading' | 'success' | 'error'

export interface UseEndpointProbeResult {
  models: ProbeModel[]
  voices: ProbeVoice[]
  status: ProbeStatus
  error: string | undefined
  refresh: () => void
}

function isValidUrl(url: string): boolean {
  return url.startsWith('http://') || url.startsWith('https://')
}

export function useEndpointProbe(url: string, apiKey: string): UseEndpointProbeResult {
  const [models, setModels] = useState<ProbeModel[]>([])
  const [voices, setVoices] = useState<ProbeVoice[]>([])
  const [status, setStatus] = useState<ProbeStatus>('idle')
  const [error, setError] = useState<string | undefined>(undefined)
  const abortRef = useRef<AbortController | null>(null)
  const urlRef = useRef(url)
  const apiKeyRef = useRef(apiKey)

  urlRef.current = url
  apiKeyRef.current = apiKey

  const doProbe = useCallback((probeUrl: string, probeKey: string) => {
    abortRef.current?.abort()

    if (!isValidUrl(probeUrl)) {
      setModels([])
      setVoices([])
      setStatus('idle')
      setError(undefined)
      return
    }

    const controller = new AbortController()
    abortRef.current = controller
    setStatus('loading')
    setError(undefined)

    fetch('/api/v1/endpoints/probe', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ url: probeUrl, api_key: probeKey }),
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
          setStatus('success')
        }
      })
      .catch((err) => {
        if (err instanceof DOMException && err.name === 'AbortError') return
        setError(err instanceof Error ? err.message : String(err))
        setModels([])
        setVoices([])
        setStatus('error')
      })
  }, [])

  // Auto-probe on URL/key change with debounce
  useEffect(() => {
    abortRef.current?.abort()
    abortRef.current = null

    if (!isValidUrl(url)) {
      setModels([])
      setVoices([])
      setStatus('idle')
      setError(undefined)
      return
    }

    const timer = setTimeout(() => {
      doProbe(url, apiKey)
    }, 500)

    return () => {
      clearTimeout(timer)
      abortRef.current?.abort()
    }
  }, [url, apiKey, doProbe])

  const refresh = useCallback(() => {
    doProbe(urlRef.current, apiKeyRef.current)
  }, [doProbe])

  return { models, voices, status, error, refresh }
}
