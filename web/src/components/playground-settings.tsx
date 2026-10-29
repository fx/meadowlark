import { useEffect, useState } from 'preact/hooks'
import { Combobox, type ComboboxOption } from '@/components/ui/combobox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'
import { api, type Endpoint } from '@/lib/api'

/** Everything the settings panel edits. Numbers stay strings while typed. */
export interface PlaygroundForm {
  endpointId: string
  model: string
  voice: string
  /** Empty means "endpoint default". */
  speed: string
  /** Empty means "endpoint default". */
  instructions: string
  /** Alias mode only. */
  name: string
  /** Alias mode only; comma-separated. */
  languages: string
}

/** The model and voice an endpoint starts with when it is chosen. */
export function endpointDefaults(ep: Endpoint | undefined): { model: string; voice: string } {
  return {
    model: ep ? ep.default_model || ep.models[0] || '' : '',
    voice: ep?.default_voice ?? '',
  }
}

type PlaygroundSettingsProps = {
  endpoints: Endpoint[]
  value: PlaygroundForm
  onChange: (next: PlaygroundForm) => void
  /** Show the alias name and languages fields. */
  aliasMode?: boolean
}

function PlaygroundSettings({ endpoints, value, onChange, aliasMode }: PlaygroundSettingsProps) {
  const [voices, setVoices] = useState<ComboboxOption[]>([])
  const [voicesLoading, setVoicesLoading] = useState(false)
  const endpoint = endpoints.find((ep) => ep.id === value.endpointId)
  const set = (patch: Partial<PlaygroundForm>) => onChange({ ...value, ...patch })

  // Offer the endpoint's enabled voices; any other id can still be typed.
  useEffect(() => {
    setVoices([])
    if (!value.endpointId) return
    const controller = new AbortController()
    setVoicesLoading(true)
    api.endpoints.voices
      .list(value.endpointId, controller.signal)
      .then((rows) => {
        if (controller.signal.aborted) return
        setVoices(
          rows
            .filter((v) => v.enabled)
            .map((v) => ({ value: v.voice_id, label: v.name || v.voice_id })),
        )
      })
      .catch(() => {})
      .finally(() => {
        if (!controller.signal.aborted) setVoicesLoading(false)
      })
    return () => controller.abort()
  }, [value.endpointId])

  const handleEndpointChange = (id: string) => {
    set({ endpointId: id, ...endpointDefaults(endpoints.find((ep) => ep.id === id)) })
  }

  return (
    <div className="space-y-4">
      <div className="space-y-2">
        <Label htmlFor="playground-endpoint">Endpoint</Label>
        <Select value={value.endpointId} onValueChange={handleEndpointChange}>
          <SelectTrigger id="playground-endpoint" className="w-full">
            <SelectValue placeholder="Select endpoint" />
          </SelectTrigger>
          <SelectContent>
            {endpoints.map((ep) => (
              <SelectItem key={ep.id} value={ep.id}>
                {ep.enabled ? ep.name : `${ep.name} (disabled)`}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      <div className="space-y-2">
        <Label htmlFor="playground-model">Model</Label>
        <Select
          value={value.model}
          onValueChange={(model: string) => set({ model })}
          disabled={!endpoint}
        >
          <SelectTrigger id="playground-model" className="w-full">
            <SelectValue placeholder="Select model" />
          </SelectTrigger>
          <SelectContent>
            {(endpoint?.models ?? []).map((m) => (
              <SelectItem key={m} value={m}>
                {m}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      <div className="space-y-2">
        <Label htmlFor="playground-voice">Voice</Label>
        <Combobox
          id="playground-voice"
          value={value.voice}
          onChange={(voice) => set({ voice })}
          options={voices}
          loading={voicesLoading}
          disabled={!endpoint}
          placeholder="Voice id"
        />
      </div>

      <div className="space-y-2">
        <Label htmlFor="playground-speed">Speed</Label>
        <Input
          id="playground-speed"
          type="number"
          step="0.05"
          min="0.25"
          max="4.0"
          value={value.speed}
          onInput={(e) => set({ speed: (e.target as HTMLInputElement).value })}
          placeholder="Endpoint default"
        />
      </div>

      <div className="space-y-2">
        <Label htmlFor="playground-instructions">Instructions</Label>
        <Textarea
          id="playground-instructions"
          value={value.instructions}
          onInput={(e) => set({ instructions: (e.target as HTMLTextAreaElement).value })}
          placeholder="Endpoint default"
          rows={3}
        />
      </div>

      {aliasMode && (
        <fieldset className="space-y-4 border-t pt-4">
          <legend className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
            Alias
          </legend>
          <div className="space-y-2">
            <Label htmlFor="playground-alias-name">Name</Label>
            <Input
              id="playground-alias-name"
              value={value.name}
              onInput={(e) => set({ name: (e.target as HTMLInputElement).value })}
              required
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="playground-alias-languages">Languages (comma-separated)</Label>
            <Input
              id="playground-alias-languages"
              value={value.languages}
              onInput={(e) => set({ languages: (e.target as HTMLInputElement).value })}
              placeholder="en"
            />
          </div>
        </fieldset>
      )}
    </div>
  )
}

export type { PlaygroundSettingsProps }
export { PlaygroundSettings }
