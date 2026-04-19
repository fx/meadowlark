import {
  ArrowsClockwise,
  Check,
  Eye,
  EyeSlash,
  SpinnerGap,
  Warning,
  X,
  XCircle,
} from '@phosphor-icons/react'
import { useCallback, useEffect, useRef, useState } from 'preact/hooks'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Combobox } from '@/components/ui/combobox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { useEndpointProbe } from '@/hooks/use-endpoint-probe'
import type { CreateEndpoint, Endpoint, UpdateEndpoint } from '@/lib/api'

type EndpointFormProps = {
  endpoint?: Endpoint
  onSubmit: (data: CreateEndpoint | UpdateEndpoint) => void
  onCancel: () => void
  isSaving: boolean
}

function EndpointForm({ endpoint, onSubmit, onCancel, isSaving }: EndpointFormProps) {
  const isCreate = !endpoint
  const [name, setName] = useState(endpoint?.name ?? '')
  const [baseUrl, setBaseUrl] = useState(endpoint?.base_url ?? '')
  const [apiKey, setApiKey] = useState(endpoint?.api_key ?? '')
  const [showApiKey, setShowApiKey] = useState(false)
  const [selectedModels, setSelectedModels] = useState<string[]>(endpoint?.models ?? [])
  const [modelInput, setModelInput] = useState('')
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
  const probe = useEndpointProbe(baseUrl, apiKey)
  const urlInvalid =
    urlDirtyRef.current &&
    (!baseUrl || (!baseUrl.startsWith('http://') && !baseUrl.startsWith('https://')))

  // Auto-populate models when probe succeeds after a URL change.
  // The probe hook resets status to 'loading' immediately on URL change,
  // so this only fires with fresh data from a completed probe.
  useEffect(() => {
    if (urlDirtyRef.current && probe.status === 'success') {
      setSelectedModels(probe.models.map((m) => m.id))
      if (probe.voices.length > 0 && !defaultVoice) {
        setDefaultVoice(probe.voices[0].id)
      }
    }
  }, [probe.status, probe.models, probe.voices, defaultVoice])

  const handleUrlChange = useCallback((e: Event) => {
    urlDirtyRef.current = true
    setBaseUrl((e.target as HTMLInputElement).value)
    setSelectedModels([])
    setModelInput('')
    setDefaultVoice('')
  }, [])

  const modelOptions = probe.models
    .filter((m) => !selectedModels.includes(m.id))
    .map((m) => ({ value: m.id, label: m.id }))

  const addModel = useCallback(
    (modelId: string) => {
      const trimmed = modelId.trim()
      if (trimmed && !selectedModels.includes(trimmed)) {
        setSelectedModels([...selectedModels, trimmed])
      }
      setModelInput('')
    },
    [selectedModels],
  )

  const removeModel = useCallback(
    (modelId: string) => {
      setSelectedModels(selectedModels.filter((m) => m !== modelId))
    },
    [selectedModels],
  )

  const handleSubmit = useCallback(
    (e: Event) => {
      e.preventDefault()
      const data: CreateEndpoint | UpdateEndpoint = {
        name,
        base_url: baseUrl,
        api_key: apiKey || undefined,
        models: selectedModels,
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
      defaultVoice,
      speed,
      instructions,
      streamingEnabled,
      streamSampleRate,
      enabled,
      onSubmit,
    ],
  )

  return (
    <form onSubmit={handleSubmit} className="space-y-4">
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

      <div className="space-y-2">
        <Label htmlFor="ep-models">Models</Label>
        {selectedModels.length > 0 && (
          <div className="flex flex-wrap gap-1.5">
            {selectedModels.map((m) => (
              <Badge key={m} variant="secondary" className="gap-1 pr-1">
                {m}
                <button
                  type="button"
                  onClick={() => removeModel(m)}
                  className="hover:text-destructive rounded-sm"
                  aria-label={`Remove ${m}`}
                >
                  <X className="size-3" />
                </button>
              </Badge>
            ))}
          </div>
        )}
        <Combobox
          id="ep-models"
          value={modelInput}
          onChange={(v) => {
            if (modelOptions.some((o) => o.value === v)) {
              addModel(v)
            } else {
              setModelInput(v)
            }
          }}
          options={modelOptions}
          loading={probe.status === 'loading'}
          placeholder={
            selectedModels.length > 0 ? 'Add another model...' : 'Search or type a model name'
          }
          required={isCreate && selectedModels.length === 0}
        />
      </div>

      {(probe.status !== 'idle' || endpoint?.default_voice) && (
        <div className="space-y-2">
          <Label htmlFor="ep-default-voice">Default Voice</Label>
          <Select
            value={defaultVoice || '__none__'}
            onValueChange={(v) => setDefaultVoice(v === '__none__' ? '' : v)}
            disabled={
              probe.status === 'loading' || (probe.status === 'error' && probe.voices.length === 0)
            }
          >
            <SelectTrigger id="ep-default-voice" className="w-full">
              <SelectValue
                placeholder={
                  probe.status === 'loading'
                    ? 'Loading voices...'
                    : probe.voices.length === 0
                      ? 'No voices available'
                      : 'Select default voice'
                }
              />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="__none__">None</SelectItem>
              {probe.voices.map((v) => (
                <SelectItem key={v.id} value={v.id}>
                  {v.name || v.id}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      )}

      <div className="grid gap-4 sm:grid-cols-2">
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
        <div className="flex items-center gap-2 pt-6">
          <Switch
            id="ep-enabled"
            checked={enabled}
            onCheckedChange={setEnabled}
            aria-label="Enabled"
          />
          <Label htmlFor="ep-enabled">Enabled</Label>
        </div>
      </div>

      <div className="grid gap-4 sm:grid-cols-2">
        <div className="flex items-center gap-2 pt-6">
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

      <div className="flex gap-2">
        <Button type="submit" disabled={isSaving}>
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
