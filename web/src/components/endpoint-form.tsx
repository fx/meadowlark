import { Eye, EyeSlash } from '@phosphor-icons/react'
import { useCallback, useState } from 'preact/hooks'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import type { CreateEndpoint, Endpoint, UpdateEndpoint } from '@/lib/api'

type EndpointFormProps = {
  endpoint?: Endpoint
  onSubmit: (data: CreateEndpoint | UpdateEndpoint) => void
  onCancel: () => void
  isSaving: boolean
}

function EndpointForm({ endpoint, onSubmit, onCancel, isSaving }: EndpointFormProps) {
  const [name, setName] = useState(endpoint?.name ?? '')
  const [baseUrl, setBaseUrl] = useState(endpoint?.base_url ?? '')
  const [apiKey, setApiKey] = useState(endpoint?.api_key ?? '')
  const [showApiKey, setShowApiKey] = useState(false)
  const [models, setModels] = useState(endpoint?.models?.join(', ') ?? '')
  const [speed, setSpeed] = useState(endpoint?.default_speed?.toString() ?? '')
  const [instructions, setInstructions] = useState(endpoint?.default_instructions ?? '')
  const [enabled, setEnabled] = useState(endpoint?.enabled ?? true)

  const handleSubmit = useCallback(
    (e: Event) => {
      e.preventDefault()
      const modelList = models
        .split(',')
        .map((m) => m.trim())
        .filter(Boolean)
      const data: CreateEndpoint | UpdateEndpoint = {
        name,
        base_url: baseUrl,
        api_key: apiKey || undefined,
        models: modelList,
        default_speed: speed ? Number(speed) : undefined,
        default_instructions: instructions || undefined,
        enabled,
      }
      onSubmit(data)
    },
    [name, baseUrl, apiKey, models, speed, instructions, enabled, onSubmit],
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
          <Input
            id="ep-url"
            value={baseUrl}
            onInput={(e) => setBaseUrl((e.target as HTMLInputElement).value)}
            required
          />
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
        <Label htmlFor="ep-models">Models (comma-separated)</Label>
        <Input
          id="ep-models"
          value={models}
          onInput={(e) => setModels((e.target as HTMLInputElement).value)}
          placeholder="tts-1, tts-1-hd"
        />
      </div>

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

export { EndpointForm }
export type { EndpointFormProps }
