import { Play, Trash } from '@phosphor-icons/react'
import { useCallback, useState } from 'preact/hooks'
import { AliasForm } from '@/components/alias-form'
import { ExpandableRow } from '@/components/expandable-row'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from '@/components/ui/alert-dialog'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Switch } from '@/components/ui/switch'
import { useFetch } from '@/hooks/use-fetch'
import { useMutation } from '@/hooks/use-mutation'
import type {
  CreateVoiceAlias,
  Endpoint,
  TestResult,
  UpdateVoiceAlias,
  VoiceAlias,
} from '@/lib/api'

function AliasesPage() {
  const { data: aliases, error, isLoading, mutate } = useFetch<VoiceAlias[]>('/api/v1/aliases')
  const { data: endpoints } = useFetch<Endpoint[]>('/api/v1/endpoints')
  const [expandedId, setExpandedId] = useState<string | null>(null)
  const [testResults, setTestResults] = useState<Record<string, TestResult>>({})
  const [testingIds, setTestingIds] = useState<Set<string>>(new Set())

  const createMutation = useMutation<CreateVoiceAlias, VoiceAlias>('/api/v1/aliases', 'POST')

  const aliasList = aliases ?? []
  const endpointList = endpoints ?? []
  const endpointMap = new Map<string, Endpoint>()
  for (const ep of endpointList) {
    endpointMap.set(ep.id, ep)
  }

  const handleCreate = useCallback(
    async (data: CreateVoiceAlias | UpdateVoiceAlias) => {
      await createMutation.trigger(data as CreateVoiceAlias)
      setExpandedId(null)
      mutate()
    },
    [createMutation, mutate],
  )

  const handleTest = useCallback(async (id: string) => {
    setTestingIds((prev) => new Set([...prev, id]))
    try {
      const res = await fetch(`/api/v1/aliases/${id}/test`, { method: 'POST' })
      const result = (await res.json()) as TestResult
      setTestResults((prev) => ({ ...prev, [id]: result }))
    } catch {
      setTestResults((prev) => ({
        ...prev,
        [id]: { ok: false, error: 'Network error' },
      }))
    } finally {
      setTestingIds((prev) => {
        const next = new Set(prev)
        next.delete(id)
        return next
      })
    }
  }, [])

  if (isLoading) {
    return <div className="p-6 text-muted-foreground">Loading aliases...</div>
  }

  if (error) {
    return <div className="p-6 text-destructive">Error: {error.message}</div>
  }

  return (
    <div className="p-6 space-y-4">
      <Button onClick={() => setExpandedId(expandedId === 'new' ? null : 'new')}>
        + Add Alias
      </Button>

      {expandedId === 'new' && (
        <div className="border-b bg-muted/50 px-4 pb-4 pt-3">
          <AliasForm
            endpoints={endpointList}
            onSubmit={handleCreate}
            onCancel={() => setExpandedId(null)}
            isSaving={createMutation.isMutating}
          />
        </div>
      )}

      <div className="border-t">
        {aliasList.map((alias) => (
          <AliasRow
            key={alias.id}
            alias={alias}
            endpoints={endpointList}
            endpointName={endpointMap.get(alias.endpoint_id)?.name ?? 'Unknown'}
            expandedId={expandedId}
            onToggle={setExpandedId}
            onUpdate={mutate}
            onTest={handleTest}
            testResult={testResults[alias.id]}
            isTesting={testingIds.has(alias.id)}
          />
        ))}
      </div>
    </div>
  )
}

function AliasRow({
  alias,
  endpoints,
  endpointName,
  expandedId,
  onToggle,
  onUpdate,
  onTest,
  testResult,
  isTesting,
}: {
  alias: VoiceAlias
  endpoints: Endpoint[]
  endpointName: string
  expandedId: string | null
  onToggle: (id: string | null) => void
  onUpdate: () => void
  onTest: (id: string) => void
  testResult?: TestResult
  isTesting: boolean
}) {
  const updateMutation = useMutation<UpdateVoiceAlias, VoiceAlias>(
    `/api/v1/aliases/${alias.id}`,
    'PUT',
  )
  const deleteMutation = useMutation<void, void>(`/api/v1/aliases/${alias.id}`, 'DELETE')

  const handleUpdate = useCallback(
    async (data: CreateVoiceAlias | UpdateVoiceAlias) => {
      await updateMutation.trigger(data as UpdateVoiceAlias)
      onToggle(null)
      onUpdate()
    },
    [updateMutation, onToggle, onUpdate],
  )

  const handleDelete = useCallback(async () => {
    await deleteMutation.trigger()
    onUpdate()
  }, [deleteMutation, onUpdate])

  const handleToggleEnabled = useCallback(
    async (checked: boolean) => {
      await updateMutation.trigger({ enabled: checked })
      onUpdate()
    },
    [updateMutation, onUpdate],
  )

  const stopPropagation = useCallback((e: Event) => {
    e.stopPropagation()
  }, [])

  return (
    <ExpandableRow
      id={alias.id}
      expandedId={expandedId}
      onToggle={onToggle}
      collapsed={
        <div className="flex w-full items-center gap-3">
          <span className="font-medium">{alias.name}</span>
          <span className="text-muted-foreground text-sm">{alias.voice}</span>
          <Badge variant="secondary">{endpointName}</Badge>
          {/* biome-ignore lint/a11y/noStaticElementInteractions: wrapper prevents click propagation to parent row */}
          <div className="ml-auto" onClick={stopPropagation} onKeyDown={stopPropagation}>
            <Switch
              checked={alias.enabled}
              onCheckedChange={handleToggleEnabled}
              aria-label={`Toggle ${alias.name}`}
            />
          </div>
        </div>
      }
      expanded={
        <div className="space-y-4">
          <AliasForm
            alias={alias}
            endpoints={endpoints}
            onSubmit={handleUpdate}
            onCancel={() => onToggle(null)}
            isSaving={updateMutation.isMutating}
          />

          <div className="flex items-center gap-2">
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() => onTest(alias.id)}
              disabled={isTesting}
            >
              <Play className="mr-1 h-4 w-4" />
              {isTesting ? 'Testing...' : 'Test TTS'}
            </Button>

            <AlertDialog>
              <AlertDialogTrigger asChild>
                <Button type="button" variant="outline" size="sm">
                  <Trash className="mr-1 h-4 w-4" />
                  Delete
                </Button>
              </AlertDialogTrigger>
              <AlertDialogContent>
                <AlertDialogHeader>
                  <AlertDialogTitle>Delete alias</AlertDialogTitle>
                  <AlertDialogDescription>
                    Are you sure you want to delete &quot;{alias.name}&quot;? This action cannot be
                    undone.
                  </AlertDialogDescription>
                </AlertDialogHeader>
                <AlertDialogFooter>
                  <AlertDialogCancel>Cancel</AlertDialogCancel>
                  <AlertDialogAction onClick={handleDelete}>Delete</AlertDialogAction>
                </AlertDialogFooter>
              </AlertDialogContent>
            </AlertDialog>

            {testResult && (
              <span
                className={testResult.ok ? 'text-sm text-green-600' : 'text-sm text-destructive'}
              >
                {testResult.ok ? `OK (${testResult.latency_ms}ms)` : `Failed: ${testResult.error}`}
              </span>
            )}
          </div>
        </div>
      }
    />
  )
}

export { AliasesPage }
