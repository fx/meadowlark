import {
  ArrowsClockwise,
  Check,
  Eye,
  EyeSlash,
  SpinnerGap,
  Warning,
  XCircle,
} from '@phosphor-icons/react'
import { useCallback, useEffect, useMemo, useRef, useState } from 'preact/hooks'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { useEndpointProbe } from '@/hooks/use-endpoint-probe'
import type { CreateEndpoint, Endpoint, EndpointVoice, UpdateEndpoint } from '@/lib/api'
import { api } from '@/lib/api'

type EndpointFormProps = {
  endpoint?: Endpoint
  onSubmit: (data: CreateEndpoint | UpdateEndpoint) => void
  onCancel: () => void
  isSaving: boolean
  onVoicesChanged?: () => void
}

type ModelToggleListProps = {
  modelIds: string[]
  enabled: string[]
  defaultModel: string
  onToggle: (id: string, on: boolean) => void
  onDefaultChange: (id: string) => void
  loading: boolean
}

type ProbeVoicePreview = { id: string; name: string }

function ModelToggleList({
  modelIds,
  enabled,
  defaultModel,
  onToggle,
  onDefaultChange,
  loading,
}: ModelToggleListProps) {
  if (modelIds.length === 0) {
    return (
      <p className="text-muted-foreground text-sm">
        {loading ? 'Discovering models...' : 'No models discovered yet. Probe the base URL above.'}
      </p>
    )
  }
  return (
    <ul className="border-border divide-border divide-y border">
      {modelIds.map((id) => {
        const isEnabled = enabled.includes(id)
        const isDefault = defaultModel === id
        const switchId = `model-toggle-${id}`
        const radioId = `model-default-${id}`
        return (
          <li key={id} className="flex items-center gap-3 px-3 py-2" data-model-id={id}>
            <Switch
              id={switchId}
              checked={isEnabled}
              onCheckedChange={(v) => onToggle(id, v)}
              aria-label={`Enable ${id}`}
            />
            <Label htmlFor={switchId} className="flex-1 font-mono text-sm">
              {id}
            </Label>
            <input
              id={radioId}
              type="radio"
              name="default-model"
              value={id}
              checked={isDefault}
              disabled={!isEnabled}
              onChange={() => onDefaultChange(id)}
              aria-label={`Set ${id} as default`}
              className="size-4 cursor-pointer disabled:cursor-not-allowed disabled:opacity-50"
            />
            <Label
              htmlFor={radioId}
              className={`text-xs ${isEnabled ? 'text-muted-foreground' : 'text-muted-foreground/50'}`}
            >
              default
            </Label>
          </li>
        )
      })}
    </ul>
  )
}

type VoiceToggleListProps = {
  voices: EndpointVoice[]
  defaultVoice: string
  onToggle: (voiceId: string, on: boolean) => void
  onDefaultChange: (voiceId: string) => void
  loading: boolean
  error?: string
}

function VoiceToggleList({
  voices,
  defaultVoice,
  onToggle,
  onDefaultChange,
  loading,
  error,
}: VoiceToggleListProps) {
  return (
    <>
      {error && <p className="text-destructive text-xs">{error}</p>}
      {voices.length === 0 ? (
        <p className="text-muted-foreground text-sm">
          {loading ? 'Loading voices...' : 'No voices discovered yet — click Refresh'}
        </p>
      ) : (
        <ul className="border-border divide-border divide-y border">
          {voices.map((v) => {
            const switchId = `voice-toggle-${v.voice_id}`
            const radioId = `voice-default-${v.voice_id}`
            const isDefault = defaultVoice === v.voice_id
            return (
              <li
                key={v.voice_id}
                className="flex items-center gap-3 px-3 py-2"
                data-voice-id={v.voice_id}
              >
                <Switch
                  id={switchId}
                  checked={v.enabled}
                  onCheckedChange={(on) => onToggle(v.voice_id, on)}
                  aria-label={`Enable voice ${v.voice_id}`}
                />
                <Label htmlFor={switchId} className="flex-1 font-mono text-sm">
                  {v.voice_id}
                </Label>
                {v.name && v.name !== v.voice_id && (
                  <span className="text-muted-foreground text-xs">{v.name}</span>
                )}
                <input
                  id={radioId}
                  type="radio"
                  name="default-voice"
                  value={v.voice_id}
                  checked={isDefault}
                  disabled={!v.enabled}
                  onChange={() => onDefaultChange(v.voice_id)}
                  aria-label={`Set ${v.voice_id} as default voice`}
                  className="size-4 cursor-pointer disabled:cursor-not-allowed disabled:opacity-50"
                />
                <Label
                  htmlFor={radioId}
                  className={`text-xs ${v.enabled ? 'text-muted-foreground' : 'text-muted-foreground/50'}`}
                >
                  default
                </Label>
              </li>
            )
          })}
        </ul>
      )}
    </>
  )
}

type VoicePreviewListProps = {
  voices: ProbeVoicePreview[]
  loading: boolean
}

function VoicePreviewList({ voices, loading }: VoicePreviewListProps) {
  return (
    <>
      <p className="text-muted-foreground text-sm">
        Voices become enable-able after saving the endpoint.
      </p>
      {voices.length === 0 ? (
        <p className="text-muted-foreground text-sm">
          {loading ? 'Loading voices...' : 'No voices discovered yet.'}
        </p>
      ) : (
        <ul className="border-border divide-border divide-y border">
          {voices.map((v) => {
            const switchId = `voice-preview-${v.id}`
            return (
              <li key={v.id} className="flex items-center gap-3 px-3 py-2" data-voice-id={v.id}>
                <Switch
                  id={switchId}
                  checked={false}
                  disabled
                  aria-label={`Enable voice ${v.id}`}
                />
                <Label htmlFor={switchId} className="flex-1 font-mono text-sm">
                  {v.id}
                </Label>
                {v.name && v.name !== v.id && (
                  <span className="text-muted-foreground text-xs">{v.name}</span>
                )}
              </li>
            )
          })}
        </ul>
      )}
    </>
  )
}

function EndpointForm({
  endpoint,
  onSubmit,
  onCancel,
  isSaving,
  onVoicesChanged,
}: EndpointFormProps) {
  const [name, setName] = useState(endpoint?.name ?? '')
  const [baseUrl, setBaseUrl] = useState(endpoint?.base_url ?? '')
  const [apiKey, setApiKey] = useState(endpoint?.api_key ?? '')
  const [showApiKey, setShowApiKey] = useState(false)
  const [selectedModels, setSelectedModels] = useState<string[]>(endpoint?.models ?? [])
  const [defaultModel, setDefaultModel] = useState(endpoint?.default_model ?? '')
  const [defaultVoice, setDefaultVoice] = useState(endpoint?.default_voice ?? '')
  const [speed, setSpeed] = useState(endpoint?.default_speed?.toString() ?? '')
  const [instructions, setInstructions] = useState(endpoint?.default_instructions ?? '')
  const [enabled, setEnabled] = useState(endpoint?.enabled ?? true)
  const [streamingEnabled, setStreamingEnabled] = useState(endpoint?.streaming_enabled ?? false)
  const [streamSampleRate, setStreamSampleRate] = useState(() => {
    const sampleRate = endpoint?.stream_sample_rate
    return sampleRate && sampleRate > 0 ? sampleRate.toString() : '24000'
  })

  const urlDirtyRef = useRef(false)
  // Set whenever a URL edit or an explicit Models-Refresh click should propagate
  // a voices.refresh on the next successful probe. Cleared after the refresh fires.
  const pendingVoicesRefreshRef = useRef(false)
  const probe = useEndpointProbe(baseUrl, apiKey)
  const urlInvalid =
    urlDirtyRef.current &&
    (!baseUrl || (!baseUrl.startsWith('http://') && !baseUrl.startsWith('https://')))

  const endpointId = endpoint?.id
  const [endpointVoices, setEndpointVoices] = useState<EndpointVoice[]>([])
  const [voicesLoading, setVoicesLoading] = useState(false)
  const [voicesRefreshing, setVoicesRefreshing] = useState(false)
  const [voicesError, setVoicesError] = useState<string | undefined>()

  useEffect(() => {
    if (!endpointId) return
    setVoicesLoading(true)
    api.endpoints.voices
      .list(endpointId)
      .then((rows) => {
        setEndpointVoices(rows)
      })
      .catch(() => {
        // List failure is non-fatal; the user can hit Refresh.
      })
      .finally(() => setVoicesLoading(false))
  }, [endpointId])

  const handleRefreshVoices = useCallback(async () => {
    setVoicesRefreshing(true)
    setVoicesError(undefined)
    try {
      const rows = await api.endpoints.voices.refresh(endpointId as string)
      setEndpointVoices(rows)
      onVoicesChanged?.()
    } catch (e) {
      setVoicesError((e as Error).message)
    } finally {
      setVoicesRefreshing(false)
    }
  }, [endpointId, onVoicesChanged])

  // Probe doesn't write the per-endpoint voices table; only an explicit
  // /voices/refresh does. Propagate the refresh on probe-success ONLY when the
  // user just edited the URL or clicked Models-Refresh — not on the auto-fire
  // that runs on form open (the list effect already loaded that data).
  const probeStatus = probe.status
  useEffect(() => {
    if (!endpointId) return
    if (probeStatus !== 'success') return
    if (!pendingVoicesRefreshRef.current) return
    pendingVoicesRefreshRef.current = false
    void handleRefreshVoices()
  }, [probeStatus, endpointId, handleRefreshVoices])

  const handleToggleVoice = useCallback(
    async (voiceId: string, on: boolean) => {
      setEndpointVoices((prev) => {
        const next = prev.map((r) => (r.voice_id === voiceId ? { ...r, enabled: on } : r))
        setDefaultVoice((prevDefault) => {
          if (on) {
            return prevDefault === '' ? voiceId : prevDefault
          }
          if (prevDefault !== voiceId) return prevDefault
          // Move default to the next still-enabled voice in display order, or clear.
          return next.find((r) => r.enabled && r.voice_id !== voiceId)?.voice_id ?? ''
        })
        return next
      })
      setVoicesError(undefined)
      try {
        await api.endpoints.voices.setEnabled(endpointId as string, voiceId, on)
        onVoicesChanged?.()
      } catch (e) {
        // Per-voice rollback only — sibling toggles in flight must not be reverted.
        setEndpointVoices((rows) =>
          rows.map((r) => (r.voice_id === voiceId ? { ...r, enabled: !on } : r)),
        )
        setVoicesError((e as Error).message)
      }
    },
    [endpointId, onVoicesChanged],
  )

  // The list of model ids surfaced to the user: discovered models from probe, plus
  // any persisted-but-no-longer-discovered models so the operator can disable them.
  const modelIds = useMemo(() => {
    const ids = probe.models.map((m) => m.id)
    for (const m of selectedModels) {
      if (!ids.includes(m)) ids.push(m)
    }
    return ids
  }, [probe.models, selectedModels])

  const handleUrlChange = useCallback((e: Event) => {
    urlDirtyRef.current = true
    pendingVoicesRefreshRef.current = true
    setBaseUrl((e.target as HTMLInputElement).value)
    setSelectedModels([])
    setDefaultModel('')
    setDefaultVoice('')
  }, [])

  const handleToggleModel = useCallback(
    (id: string, on: boolean) => {
      setSelectedModels((prev) => {
        const next = on ? [...prev, id] : prev.filter((m) => m !== id)
        setDefaultModel((prevDefault) => {
          if (on) {
            return prevDefault === '' ? id : prevDefault
          }
          if (prevDefault !== id) return prevDefault
          // Move default to the next still-enabled model in display order, or clear.
          return modelIds.find((m) => m !== id && next.includes(m)) ?? ''
        })
        return next
      })
    },
    [modelIds],
  )

  const handleDefaultChange = useCallback((id: string) => {
    setDefaultModel(id)
  }, [])

  const handleSubmit = useCallback(
    (e: Event) => {
      e.preventDefault()
      const data: CreateEndpoint | UpdateEndpoint = {
        name,
        base_url: baseUrl,
        api_key: apiKey || undefined,
        models: selectedModels,
        default_model: defaultModel,
        default_voice: defaultVoice,
        default_speed: speed && Number.isFinite(Number(speed)) ? Number(speed) : undefined,
        default_instructions: instructions || undefined,
        streaming_enabled: streamingEnabled,
        stream_sample_rate: streamingEnabled
          ? Math.min(48000, Math.max(8000, Math.round(Number(streamSampleRate) || 24000)))
          : undefined,
        enabled,
      }
      onSubmit(data)
    },
    [
      name,
      baseUrl,
      apiKey,
      selectedModels,
      defaultModel,
      defaultVoice,
      speed,
      instructions,
      streamingEnabled,
      streamSampleRate,
      enabled,
      onSubmit,
    ],
  )

  const submitDisabled = isSaving || selectedModels.length === 0 || defaultModel === ''

  return (
    <form onSubmit={handleSubmit} className="space-y-6">
      <section className="space-y-4" data-section="connection">
        <h3 className="text-sm font-semibold tracking-wide uppercase">Connection</h3>
        <div className="grid gap-4 sm:grid-cols-2">
          <div className="space-y-2">
            <Label htmlFor="ep-name">Name</Label>
            <Input
              id="ep-name"
              value={name}
              onInput={(e) => setName((e.target as HTMLInputElement).value)}
              required
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="ep-url">Base URL</Label>
            <div className="flex gap-2">
              <Input id="ep-url" value={baseUrl} onInput={handleUrlChange} required />
              <Button
                type="button"
                variant="ghost"
                size="icon"
                className="shrink-0"
                aria-label="Refresh endpoint"
                disabled={probe.status === 'loading' || urlInvalid}
                onClick={probe.refresh}
              >
                {urlInvalid && <Warning className="h-4 w-4 text-yellow-600" />}
                {!urlInvalid && probe.status === 'loading' && (
                  <SpinnerGap className="h-4 w-4 animate-spin" />
                )}
                {!urlInvalid && probe.status === 'success' && (
                  <Check className="h-4 w-4 text-green-600" />
                )}
                {!urlInvalid && probe.status === 'error' && (
                  <XCircle className="h-4 w-4 text-destructive" />
                )}
                {!urlInvalid && probe.status === 'idle' && <ArrowsClockwise className="h-4 w-4" />}
              </Button>
            </div>
            {urlInvalid && baseUrl && (
              <p className="text-sm text-yellow-600">URL must start with http:// or https://</p>
            )}
            {probe.error && <p className="text-sm text-destructive">{probe.error}</p>}
          </div>
        </div>

        <div className="space-y-2">
          <Label htmlFor="ep-key">API Key</Label>
          <div className="flex gap-2">
            <Input
              id="ep-key"
              type={showApiKey ? 'text' : 'password'}
              value={apiKey}
              onInput={(e) => setApiKey((e.target as HTMLInputElement).value)}
            />
            <Button
              type="button"
              variant="ghost"
              size="icon"
              aria-label={showApiKey ? 'Hide API key' : 'Show API key'}
              onClick={() => setShowApiKey(!showApiKey)}
            >
              {showApiKey ? <EyeSlash className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
            </Button>
          </div>
        </div>

        <div className="flex items-center gap-2" data-field="enabled">
          <Switch
            id="ep-enabled"
            checked={enabled}
            onCheckedChange={setEnabled}
            aria-label="Enabled"
          />
          <Label htmlFor="ep-enabled">Enabled</Label>
        </div>
      </section>

      <section className="space-y-2" data-section="models">
        <div className="flex items-center justify-between gap-2">
          <h3 className="text-sm font-semibold tracking-wide uppercase">Models</h3>
          <Button
            type="button"
            variant="outline"
            size="sm"
            aria-label="Refresh models"
            onClick={() => {
              pendingVoicesRefreshRef.current = true
              probe.refresh()
            }}
            disabled={probe.status === 'loading' || urlInvalid || !baseUrl}
          >
            {probe.status === 'loading' ? 'Refreshing...' : 'Refresh'}
          </Button>
        </div>
        <ModelToggleList
          modelIds={modelIds}
          enabled={selectedModels}
          defaultModel={defaultModel}
          onToggle={handleToggleModel}
          onDefaultChange={handleDefaultChange}
          loading={probe.status === 'loading'}
        />
      </section>

      <section className="space-y-2" data-section="voices">
        <div className="flex items-center justify-between gap-2">
          <h3 className="text-sm font-semibold tracking-wide uppercase">Voices</h3>
          {endpointId && (
            <Button
              type="button"
              variant="outline"
              size="sm"
              aria-label="Refresh voices"
              onClick={handleRefreshVoices}
              disabled={voicesRefreshing}
            >
              {voicesRefreshing ? 'Refreshing...' : 'Refresh'}
            </Button>
          )}
        </div>
        {endpointId ? (
          <VoiceToggleList
            voices={endpointVoices}
            defaultVoice={defaultVoice}
            onToggle={handleToggleVoice}
            onDefaultChange={setDefaultVoice}
            loading={voicesLoading}
            error={voicesError}
          />
        ) : (
          <VoicePreviewList voices={probe.voices} loading={probe.status === 'loading'} />
        )}
      </section>

      <section className="space-y-4" data-section="defaults">
        <h3 className="text-sm font-semibold tracking-wide uppercase">Defaults</h3>
        <div className="space-y-2">
          <Label htmlFor="ep-speed">Default Speed</Label>
          <Input
            id="ep-speed"
            type="number"
            step="0.05"
            min="0.25"
            max="4.0"
            value={speed}
            onInput={(e) => setSpeed((e.target as HTMLInputElement).value)}
            placeholder="1.0"
          />
        </div>
        <div className="space-y-2">
          <Label htmlFor="ep-instructions">Default Instructions</Label>
          <Textarea
            id="ep-instructions"
            value={instructions}
            onInput={(e) => setInstructions((e.target as HTMLTextAreaElement).value)}
            placeholder="Optional instructions for TTS"
          />
        </div>
      </section>

      <section className="space-y-4" data-section="streaming">
        <h3 className="text-sm font-semibold tracking-wide uppercase">Streaming</h3>
        <div className="flex items-center gap-2">
          <Switch
            id="ep-streaming"
            checked={streamingEnabled}
            onCheckedChange={setStreamingEnabled}
            aria-label="Streaming"
          />
          <Label htmlFor="ep-streaming">
            Streaming
            <span className="text-muted-foreground ml-2 text-xs font-normal">
              Enable streaming PCM responses for lower time-to-first-audio
            </span>
          </Label>
        </div>
        {streamingEnabled && (
          <div className="space-y-2">
            <Label htmlFor="ep-sample-rate">Sample Rate</Label>
            <Input
              id="ep-sample-rate"
              type="number"
              min="8000"
              max="48000"
              step="1"
              value={streamSampleRate}
              onInput={(e) => setStreamSampleRate((e.target as HTMLInputElement).value)}
              onBlur={(e) => {
                const value = (e.target as HTMLInputElement).value
                if (value === '') return
                const clamped = Math.min(48000, Math.max(8000, Math.round(Number(value))))
                setStreamSampleRate(String(clamped))
              }}
              placeholder="24000"
            />
          </div>
        )}
      </section>

      <div className="flex gap-2">
        <Button type="submit" disabled={submitDisabled}>
          {isSaving ? 'Saving...' : endpoint ? 'Update' : 'Create'}
        </Button>
        <Button type="button" variant="outline" onClick={onCancel}>
          Cancel
        </Button>
      </div>
    </form>
  )
}

export type { EndpointFormProps }
export { EndpointForm }
