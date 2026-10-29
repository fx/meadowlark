import { CopySimple, FloppyDisk, Play, Stop } from '@phosphor-icons/react'
import { useRef, useState } from 'preact/hooks'
import { Link, useLocation, useParams, useSearch } from 'wouter'
import {
  endpointDefaults,
  type PlaygroundForm,
  PlaygroundSettings,
} from '@/components/playground-settings'
import { TakeList, TakePlayer } from '@/components/take-player'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { invalidateCache, useFetch, useFetchList } from '@/hooks/use-fetch'
import { useMutation } from '@/hooks/use-mutation'
import {
  api,
  type CreateVoiceAlias,
  type Endpoint,
  type SynthesizeRequest,
  type UpdateVoiceAlias,
  type VoiceAlias,
} from '@/lib/api'
import { computePeaks } from '@/lib/peaks'
import {
  addTake,
  clearTakes,
  removeTake,
  selectTake,
  setDraftText,
  type Take,
  useTakes,
} from '@/lib/takes'
import { cn } from '@/lib/utils'
import { type DecodedWav, parseWav } from '@/lib/wav'

/** POST /api/v1/synthesize rejects longer text. */
export const MAX_TEXT_CHARS = 4096

const MAIN_BUCKETS = 200
const MINI_BUCKETS = 64

/** Text length in code points, which is what the server counts. */
function charCount(text: string): number {
  return [...text].length
}

function parseLanguages(languages: string): string[] {
  return languages
    .split(',')
    .map((l) => l.trim())
    .filter(Boolean)
}

/** The synthesis fields every request and alias shares. Optional fields are sent only when set. */
function synthesisFields(form: PlaygroundForm) {
  const fields: Pick<
    SynthesizeRequest,
    'endpoint_id' | 'model' | 'voice' | 'speed' | 'instructions'
  > = { endpoint_id: form.endpointId, model: form.model, voice: form.voice.trim() }
  if (form.speed.trim() !== '') fields.speed = Number(form.speed)
  if (form.instructions.trim() !== '') fields.instructions = form.instructions
  return fields
}

function aliasPayload(form: PlaygroundForm): CreateVoiceAlias {
  return {
    name: form.name.trim(),
    ...synthesisFields(form),
    languages: parseLanguages(form.languages),
  }
}

function aliasToForm(alias: VoiceAlias): PlaygroundForm {
  return {
    endpointId: alias.endpoint_id,
    model: alias.model,
    voice: alias.voice,
    speed: alias.speed?.toString() ?? '',
    instructions: alias.instructions ?? '',
    name: alias.name,
    languages: alias.languages.join(', '),
  }
}

function blankForm(endpoints: Endpoint[], search: URLSearchParams): PlaygroundForm {
  const preset = endpoints.find((ep) => ep.id === search.get('endpoint'))
  // Callers guarantee at least one endpoint.
  const endpoint = preset ?? endpoints.find((ep) => ep.enabled) ?? endpoints[0]
  const defaults = endpointDefaults(endpoint)
  const model = search.get('model')
  const voice = search.get('voice')
  return {
    endpointId: endpoint.id,
    model: preset && model && preset.models.includes(model) ? model : defaults.model,
    voice: preset && voice ? voice : defaults.voice,
    speed: '',
    instructions: '',
    name: '',
    languages: 'en',
  }
}

function errorMessage(err: unknown): string {
  return err instanceof Error ? err.message : String(err)
}

/** /playground and /playground/:aliasId */
function PlaygroundPage() {
  const { aliasId } = useParams<{ aliasId?: string }>()
  return aliasId ? <AliasPlayground key={aliasId} aliasId={aliasId} /> : <BlankPlayground />
}

function Loading() {
  return <div className="p-4 text-muted-foreground">Loading playground...</div>
}

function NoEndpoints() {
  return (
    <div className="p-4 text-muted-foreground">
      No endpoints configured.{' '}
      <Link href="/endpoints" className="text-primary underline-offset-4 hover:underline">
        Add one
      </Link>{' '}
      to start generating.
    </div>
  )
}

function BlankPlayground() {
  const { data: endpoints, error } = useFetchList<Endpoint>('/api/v1/endpoints')
  const search = useSearch()

  if (error) return <div className="p-4 text-destructive">Error: {error.message}</div>
  if (!endpoints) return <Loading />
  if (endpoints.length === 0) return <NoEndpoints />
  return (
    <Workspace
      key={search}
      endpoints={endpoints}
      initialForm={blankForm(endpoints, new URLSearchParams(search))}
    />
  )
}

function AliasPlayground({ aliasId }: { aliasId: string }) {
  const endpoints = useFetchList<Endpoint>('/api/v1/endpoints')
  const alias = useFetch<VoiceAlias>(`/api/v1/aliases/${aliasId}`)

  if (alias.error) {
    return (
      <div className="p-4 space-y-2">
        <p className="text-destructive">
          {alias.error.message === 'alias not found'
            ? 'Alias not found'
            : `Error: ${alias.error.message}`}
        </p>
        <Link href="/playground" className="text-primary underline-offset-4 hover:underline">
          Open a blank playground
        </Link>
      </div>
    )
  }
  if (endpoints.error) {
    return <div className="p-4 text-destructive">Error: {endpoints.error.message}</div>
  }
  if (!alias.data || !endpoints.data) return <Loading />
  return (
    <Workspace
      endpoints={endpoints.data}
      alias={alias.data}
      initialForm={aliasToForm(alias.data)}
    />
  )
}

type WorkspaceProps = {
  endpoints: Endpoint[]
  initialForm: PlaygroundForm
  /** Set in alias mode: the alias as last loaded or saved. */
  alias?: VoiceAlias
}

function Workspace({ endpoints, initialForm, alias: loadedAlias }: WorkspaceProps) {
  const { takes, selectedId, text } = useTakes()
  const [form, setForm] = useState(initialForm)
  const [savedAlias, setSavedAlias] = useState(loadedAlias)
  const [generating, setGenerating] = useState(false)
  const [generateError, setGenerateError] = useState<string | null>(null)
  const [autoPlayId, setAutoPlayId] = useState<string | null>(null)
  const [dialog, setDialog] = useState<SaveDialogValues | null>(null)
  const abortRef = useRef<AbortController | null>(null)

  const endpoint = endpoints.find((ep) => ep.id === form.endpointId)
  const chars = charCount(text)
  const tooLong = chars > MAX_TEXT_CHARS
  const synthesisReady = Boolean(endpoint && form.model && form.voice.trim())
  const canGenerate = synthesisReady && text.trim() !== '' && !tooLong && !generating
  const selected = takes.find((t) => t.id === selectedId)

  const generate = async () => {
    if (!canGenerate || !endpoint) return
    const controller = new AbortController()
    abortRef.current = controller
    setGenerating(true)
    setGenerateError(null)
    const fields = synthesisFields(form)
    const start = performance.now()
    try {
      const buf = await api.synthesize({ ...fields, text }, controller.signal)
      const latencyMs = performance.now() - start
      let wav: DecodedWav
      try {
        wav = parseWav(buf)
      } catch {
        setGenerateError('Could not decode audio')
        return
      }
      const blob = new Blob([buf], { type: 'audio/wav' })
      const take = addTake({
        params: {
          endpointId: endpoint.id,
          endpointName: endpoint.name,
          model: fields.model,
          voice: fields.voice,
          speed: fields.speed,
          instructions: fields.instructions,
        },
        text,
        blob,
        url: URL.createObjectURL(blob),
        format: {
          sampleRate: wav.sampleRate,
          channels: wav.channels,
          bitsPerSample: wav.bitsPerSample,
        },
        duration: wav.duration,
        peaks: computePeaks(wav.channelData, MAIN_BUCKETS),
        miniPeaks: computePeaks(wav.channelData, MINI_BUCKETS),
        latencyMs,
        createdAt: new Date(),
      })
      setAutoPlayId(take.id)
    } catch (err) {
      if (!controller.signal.aborted) setGenerateError(errorMessage(err))
    } finally {
      abortRef.current = null
      setGenerating(false)
    }
  }

  const cancel = () => abortRef.current?.abort()

  const handleTextKeyDown = (e: KeyboardEvent) => {
    if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) {
      e.preventDefault()
      generate()
    }
  }

  const playTake = (take: Take) => {
    selectTake(take.id)
    setAutoPlayId(take.id)
  }

  const restoreTake = (take: Take) => {
    setForm({
      ...form,
      endpointId: take.params.endpointId,
      model: take.params.model,
      voice: take.params.voice,
      speed: take.params.speed?.toString() ?? '',
      instructions: take.params.instructions ?? '',
    })
    setDraftText(take.text)
  }

  return (
    <div className="grid gap-6 p-4 md:grid-cols-[20rem_minmax(0,1fr)]">
      <aside className="space-y-4">
        {savedAlias ? (
          <div className="space-y-1">
            <p className="text-xs uppercase tracking-wide text-muted-foreground">Editing alias</p>
            <h1 className="flex items-center gap-2 text-lg font-semibold">
              <span className="truncate">{savedAlias.name}</span>
              {!savedAlias.enabled && <Badge variant="outline">disabled</Badge>}
            </h1>
          </div>
        ) : (
          <h1 className="text-lg font-semibold">Playground</h1>
        )}

        <PlaygroundSettings
          endpoints={endpoints}
          value={form}
          onChange={setForm}
          aliasMode={savedAlias !== undefined}
        />

        {savedAlias ? (
          <AliasSaveActions
            alias={savedAlias}
            form={form}
            ready={synthesisReady}
            onSaved={(updated) => {
              // Show what the server stored, which is not always what was sent.
              setSavedAlias(updated)
              setForm(aliasToForm(updated))
            }}
            onSaveAsNew={() =>
              setDialog({ name: `${savedAlias.name} copy`, languages: form.languages })
            }
          />
        ) : (
          <div className="border-t pt-4">
            <Button
              type="button"
              variant="outline"
              onClick={() => setDialog({ name: '', languages: 'en' })}
              disabled={!synthesisReady}
            >
              <FloppyDisk />
              Save as alias
            </Button>
          </div>
        )}
      </aside>

      <main className="min-w-0 space-y-4">
        <div className="space-y-2">
          <div className="flex items-baseline justify-between">
            <Label htmlFor="playground-text">Text</Label>
            <span
              data-testid="char-count"
              className={cn(
                'font-mono text-xs tabular-nums',
                tooLong ? 'text-destructive' : 'text-muted-foreground',
              )}
            >
              {chars} / {MAX_TEXT_CHARS}
            </span>
          </div>
          <Textarea
            id="playground-text"
            value={text}
            onInput={(e) => setDraftText((e.target as HTMLTextAreaElement).value)}
            onKeyDown={handleTextKeyDown}
            rows={5}
            aria-invalid={tooLong}
            className="min-h-28"
          />
          <div className="flex flex-wrap items-center gap-2">
            {generating ? (
              <Button type="button" variant="outline" onClick={cancel}>
                <Stop />
                Cancel
              </Button>
            ) : (
              <Button type="button" onClick={generate} disabled={!canGenerate}>
                <Play weight="fill" />
                Generate
              </Button>
            )}
            <span className="text-xs text-muted-foreground">
              {generating ? 'Generating…' : 'Ctrl/⌘ + Enter'}
            </span>
          </div>
          {generateError && (
            <p role="alert" className="text-sm text-destructive">
              {generateError}
            </p>
          )}
        </div>

        {selected ? (
          <TakePlayer take={selected} autoPlay={selected.id === autoPlayId} />
        ) : (
          <div className="flex h-36 items-center justify-center border border-dashed text-sm text-muted-foreground">
            Generate a take to hear it here.
          </div>
        )}

        {takes.length > 0 && (
          <TakeList
            takes={takes}
            selectedId={selectedId}
            onSelect={playTake}
            onRestore={restoreTake}
            onRemove={(take) => removeTake(take.id)}
            onClear={clearTakes}
          />
        )}
      </main>

      {dialog && (
        <SaveAliasDialog
          title={savedAlias ? 'Save as new alias' : 'Save as alias'}
          initial={dialog}
          form={form}
          onClose={() => setDialog(null)}
        />
      )}
    </div>
  )
}

type AliasSaveActionsProps = {
  alias: VoiceAlias
  form: PlaygroundForm
  /** Endpoint, model, and voice are all set. */
  ready: boolean
  onSaved: (alias: VoiceAlias) => void
  onSaveAsNew: () => void
}

function AliasSaveActions({ alias, form, ready, onSaved, onSaveAsNew }: AliasSaveActionsProps) {
  const update = useMutation<UpdateVoiceAlias, VoiceAlias>(`/api/v1/aliases/${alias.id}`, 'PUT')
  const [error, setError] = useState<string | null>(null)
  const dirty =
    JSON.stringify(aliasPayload(form)) !== JSON.stringify(aliasPayload(aliasToForm(alias)))

  const save = async () => {
    setError(null)
    try {
      const updated = await update.trigger(aliasPayload(form))
      invalidateCache('/api/v1/voices')
      onSaved(updated)
    } catch (err) {
      setError(errorMessage(err))
    }
  }

  return (
    <div className="space-y-2 border-t pt-4">
      <div className="flex flex-wrap gap-2">
        <Button
          type="button"
          onClick={save}
          disabled={!dirty || !ready || !form.name.trim() || update.isMutating}
        >
          <FloppyDisk />
          {update.isMutating ? 'Saving...' : 'Save'}
        </Button>
        <Button type="button" variant="outline" onClick={onSaveAsNew} disabled={!ready}>
          <CopySimple />
          Save as new
        </Button>
      </div>
      {error && <p className="text-sm text-destructive">{error}</p>}
    </div>
  )
}

type SaveDialogValues = { name: string; languages: string }

type SaveAliasDialogProps = {
  title: string
  initial: SaveDialogValues
  form: PlaygroundForm
  onClose: () => void
}

/** Creates an alias from the panel's settings, then opens it in the playground. */
function SaveAliasDialog({ title, initial, form, onClose }: SaveAliasDialogProps) {
  const [, setLocation] = useLocation()
  const create = useMutation<CreateVoiceAlias, VoiceAlias>('/api/v1/aliases', 'POST')
  const [values, setValues] = useState(initial)
  const [error, setError] = useState<string | null>(null)

  const submit = async (e: Event) => {
    e.preventDefault()
    setError(null)
    try {
      const created = await create.trigger(aliasPayload({ ...form, ...values }))
      invalidateCache('/api/v1/voices')
      onClose()
      setLocation(`/playground/${created.id}`)
    } catch (err) {
      setError(errorMessage(err))
    }
  }

  return (
    <Dialog open onOpenChange={(open: boolean) => !open && onClose()}>
      <DialogContent>
        <form onSubmit={submit} className="space-y-4">
          <DialogHeader>
            <DialogTitle>{title}</DialogTitle>
            <DialogDescription>
              Saves the current endpoint, model, voice, speed, and instructions as a voice Home
              Assistant can select.
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-2">
            <Label htmlFor="save-alias-name">Name</Label>
            <Input
              id="save-alias-name"
              value={values.name}
              onInput={(e) => setValues({ ...values, name: (e.target as HTMLInputElement).value })}
              required
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="save-alias-languages">Languages (comma-separated)</Label>
            <Input
              id="save-alias-languages"
              value={values.languages}
              onInput={(e) =>
                setValues({ ...values, languages: (e.target as HTMLInputElement).value })
              }
              placeholder="en"
            />
          </div>
          {error && <p className="text-sm text-destructive">{error}</p>}
          <DialogFooter>
            <Button type="button" variant="outline" onClick={onClose}>
              Cancel
            </Button>
            <Button type="submit" disabled={!values.name.trim() || create.isMutating}>
              {create.isMutating ? 'Saving...' : 'Save'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

export { PlaygroundPage }
