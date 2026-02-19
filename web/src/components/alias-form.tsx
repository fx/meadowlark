import { useCallback, useEffect, useState } from 'preact/hooks'
import { Button } from '@/components/ui/button'
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
import type { CreateVoiceAlias, Endpoint, UpdateVoiceAlias, VoiceAlias } from '@/lib/api'

type AliasFormProps = {
  alias?: VoiceAlias
  endpoints: Endpoint[]
  onSubmit: (data: CreateVoiceAlias | UpdateVoiceAlias) => void
  onCancel: () => void
  isSaving: boolean
}

function AliasForm({ alias, endpoints, onSubmit, onCancel, isSaving }: AliasFormProps) {
  const [name, setName] = useState(alias?.name ?? '')
  const [endpointId, setEndpointId] = useState(alias?.endpoint_id ?? '')
  const [model, setModel] = useState(alias?.model ?? '')
  const [voice, setVoice] = useState(alias?.voice ?? '')
  const [speed, setSpeed] = useState(alias?.speed?.toString() ?? '')
  const [instructions, setInstructions] = useState(alias?.instructions ?? '')
  const [languages, setLanguages] = useState(alias?.languages?.join(', ') ?? 'en')
  const [enabled, setEnabled] = useState(alias?.enabled ?? true)
  const [voices, setVoices] = useState<string[]>([])
  const [voicesLoading, setVoicesLoading] = useState(false)

  const selectedEndpoint = endpoints.find((ep) => ep.id === endpointId)
  const models = selectedEndpoint?.models ?? []

  useEffect(() => {
    if (!endpointId) {
      setVoices([])
      return
    }
    setVoicesLoading(true)
    fetch(`/api/v1/endpoints/${endpointId}/voices`)
      .then(async (res) => {
        if (res.ok) {
          const data = await res.json()
          setVoices(data as string[])
        } else {
          setVoices([])
        }
      })
      .catch(() => setVoices([]))
      .finally(() => setVoicesLoading(false))
  }, [endpointId])

  const handleEndpointChange = useCallback((value: string) => {
    setEndpointId(value)
    setModel('')
    setVoice('')
  }, [])

  const handleModelChange = useCallback((value: string) => {
    setModel(value)
  }, [])

  const handleSubmit = useCallback(
    (e: Event) => {
      e.preventDefault()
      const langList = languages
        .split(',')
        .map((l) => l.trim())
        .filter(Boolean)
      const data: CreateVoiceAlias | UpdateVoiceAlias = {
        name,
        endpoint_id: endpointId,
        model,
        voice,
        speed: speed ? Number(speed) : undefined,
        instructions: instructions || undefined,
        languages: langList.length > 0 ? langList : undefined,
        enabled,
      }
      onSubmit(data)
    },
    [name, endpointId, model, voice, speed, instructions, languages, enabled, onSubmit],
  )

  return (
    <form onSubmit={handleSubmit} className="space-y-4">
      <div className="space-y-2">
        <Label htmlFor="alias-name">Alias Name</Label>
        <Input
          id="alias-name"
          value={name}
          onInput={(e) => setName((e.target as HTMLInputElement).value)}
          required
        />
      </div>

      <div className="grid gap-4 sm:grid-cols-2">
        <div className="space-y-2">
          <Label htmlFor="alias-endpoint">Endpoint</Label>
          <Select value={endpointId} onValueChange={handleEndpointChange}>
            <SelectTrigger id="alias-endpoint">
              <SelectValue placeholder="Select endpoint" />
            </SelectTrigger>
            <SelectContent>
              {endpoints.map((ep) => (
                <SelectItem key={ep.id} value={ep.id}>
                  {ep.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>

        <div className="space-y-2">
          <Label htmlFor="alias-model">Model</Label>
          <Select value={model} onValueChange={handleModelChange} disabled={!endpointId}>
            <SelectTrigger id="alias-model">
              <SelectValue placeholder="Select model" />
            </SelectTrigger>
            <SelectContent>
              {models.map((m) => (
                <SelectItem key={m} value={m}>
                  {m}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      </div>

      <div className="space-y-2">
        <Label htmlFor="alias-voice">Voice</Label>
        {voices.length > 0 ? (
          <Select value={voice} onValueChange={setVoice} disabled={!endpointId}>
            <SelectTrigger id="alias-voice">
              <SelectValue placeholder={voicesLoading ? 'Loading voices...' : 'Select voice'} />
            </SelectTrigger>
            <SelectContent>
              {voices.map((v) => (
                <SelectItem key={v} value={v}>
                  {v}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        ) : (
          <Input
            id="alias-voice"
            value={voice}
            onInput={(e) => setVoice((e.target as HTMLInputElement).value)}
            placeholder={voicesLoading ? 'Loading voices...' : 'Enter voice name'}
            required
          />
        )}
      </div>

      <div className="grid gap-4 sm:grid-cols-2">
        <div className="space-y-2">
          <Label htmlFor="alias-speed">Speed</Label>
          <Input
            id="alias-speed"
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
            id="alias-enabled"
            checked={enabled}
            onCheckedChange={setEnabled}
            aria-label="Enabled"
          />
          <Label htmlFor="alias-enabled">Enabled</Label>
        </div>
      </div>

      <div className="space-y-2">
        <Label htmlFor="alias-instructions">Instructions</Label>
        <Textarea
          id="alias-instructions"
          value={instructions}
          onInput={(e) => setInstructions((e.target as HTMLTextAreaElement).value)}
          placeholder="Optional instructions for TTS"
        />
      </div>

      <div className="space-y-2">
        <Label htmlFor="alias-languages">Languages (comma-separated)</Label>
        <Input
          id="alias-languages"
          value={languages}
          onInput={(e) => setLanguages((e.target as HTMLInputElement).value)}
          placeholder="en"
        />
      </div>

      <div className="flex gap-2">
        <Button type="submit" disabled={isSaving}>
          {isSaving ? 'Saving...' : alias ? 'Update' : 'Create'}
        </Button>
        <Button type="button" variant="outline" onClick={onCancel}>
          Cancel
        </Button>
      </div>
    </form>
  )
}

export { AliasForm }
export type { AliasFormProps }
