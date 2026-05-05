import { Trash } from '@phosphor-icons/react'
import { useCallback, useEffect, useState } from 'preact/hooks'
import { EndpointForm } from '@/components/endpoint-form'
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
import type { CreateEndpoint, Endpoint, EndpointVoice, UpdateEndpoint } from '@/lib/api'
import { api } from '@/lib/api'

function EndpointsPage() {
  const { data: endpoints, error, isLoading, mutate } = useFetch<Endpoint[]>('/api/v1/endpoints')
  const [expandedId, setExpandedId] = useState<string | null>(null)

  const createMutation = useMutation<CreateEndpoint, Endpoint>('/api/v1/endpoints', 'POST')

  const handleCreate = useCallback(
    async (data: CreateEndpoint | UpdateEndpoint) => {
      await createMutation.trigger(data as CreateEndpoint)
      setExpandedId(null)
      mutate()
    },
    [createMutation, mutate],
  )

  if (isLoading) {
    return <div className="p-4 text-muted-foreground">Loading endpoints...</div>
  }

  if (error) {
    return <div className="p-4 text-destructive">Error: {error.message}</div>
  }

  return (
    <div className="p-4 space-y-4">
      <Button onClick={() => setExpandedId(expandedId === 'new' ? null : 'new')}>
        + Add Endpoint
      </Button>

      {expandedId === 'new' && (
        <div className="border-b bg-muted/50 px-4 pb-4 pt-3">
          <EndpointForm
            onSubmit={handleCreate}
            onCancel={() => setExpandedId(null)}
            isSaving={createMutation.isMutating}
          />
        </div>
      )}

      {endpoints?.length === 0 && (
        <p className="text-muted-foreground">No endpoints configured. Add one to get started.</p>
      )}

      <div className="border-t">
        {endpoints?.map((ep) => (
          <EndpointRow
            key={ep.id}
            endpoint={ep}
            expandedId={expandedId}
            onToggle={setExpandedId}
            onUpdate={mutate}
          />
        ))}
      </div>
    </div>
  )
}

function EndpointRow({
  endpoint,
  expandedId,
  onToggle,
  onUpdate,
}: {
  endpoint: Endpoint
  expandedId: string | null
  onToggle: (id: string | null) => void
  onUpdate: () => void
}) {
  const [saving, setSaving] = useState(false)
  const [enabledVoiceCount, setEnabledVoiceCount] = useState<number | null>(null)
  const [voicesNonce, setVoicesNonce] = useState(0)

  useEffect(() => {
    api.endpoints.voices
      .list(endpoint.id)
      .then((rows: EndpointVoice[]) => {
        setEnabledVoiceCount(rows.filter((r) => r.enabled).length)
      })
      .catch(() => {
        setEnabledVoiceCount(0)
      })
  }, [endpoint.id, voicesNonce])

  const handleVoicesChanged = useCallback(() => {
    setVoicesNonce((n) => n + 1)
  }, [])

  const handleUpdate = useCallback(
    async (data: CreateEndpoint | UpdateEndpoint) => {
      setSaving(true)
      try {
        await api.endpoints.update(endpoint.id, data as UpdateEndpoint)
        onToggle(null)
        onUpdate()
      } finally {
        setSaving(false)
      }
    },
    [endpoint.id, onToggle, onUpdate],
  )

  const handleDelete = useCallback(async () => {
    await api.endpoints.delete(endpoint.id)
    onUpdate()
  }, [endpoint.id, onUpdate])

  const handleToggleEnabled = useCallback(
    async (checked: boolean) => {
      try {
        await api.endpoints.update(endpoint.id, { enabled: checked })
        onUpdate()
      } catch {
        // revert will happen on next fetch
      }
    },
    [endpoint.id, onUpdate],
  )

  return (
    <ExpandableRow
      id={endpoint.id}
      expandedId={expandedId}
      onToggle={onToggle}
      collapsed={
        <div className="flex w-full items-center gap-3">
          <span className="font-medium">{endpoint.name}</span>
          <Badge variant="secondary">
            {endpoint.models.length} {endpoint.models.length === 1 ? 'model' : 'models'}
          </Badge>
          {enabledVoiceCount !== null && (
            <Badge variant="secondary">
              {enabledVoiceCount} {enabledVoiceCount === 1 ? 'voice' : 'voices'}
            </Badge>
          )}
          {!endpoint.enabled && <Badge variant="outline">Disabled</Badge>}
          {/* biome-ignore lint/a11y/useKeyWithClickEvents lint/a11y/noStaticElementInteractions: stopPropagation prevents row toggle when clicking switch */}
          <div className="ml-auto" onClick={(e) => e.stopPropagation()}>
            <Switch
              checked={endpoint.enabled}
              onCheckedChange={handleToggleEnabled}
              aria-label={`${endpoint.name} enabled`}
            />
          </div>
        </div>
      }
      expanded={
        <div className="space-y-4">
          <EndpointForm
            endpoint={endpoint}
            onSubmit={handleUpdate}
            onCancel={() => onToggle(null)}
            isSaving={saving}
            onVoicesChanged={handleVoicesChanged}
          />

          <AlertDialog>
            <AlertDialogTrigger asChild>
              <Button
                type="button"
                variant="outline"
                size="sm"
                aria-label={`Delete ${endpoint.name}`}
              >
                <Trash className="mr-1 h-4 w-4" />
                Delete
              </Button>
            </AlertDialogTrigger>
            <AlertDialogContent>
              <AlertDialogHeader>
                <AlertDialogTitle>Delete Endpoint</AlertDialogTitle>
                <AlertDialogDescription>
                  Are you sure you want to delete &quot;{endpoint.name}&quot;? This action cannot be
                  undone.
                </AlertDialogDescription>
              </AlertDialogHeader>
              <AlertDialogFooter>
                <AlertDialogCancel>Cancel</AlertDialogCancel>
                <AlertDialogAction onClick={handleDelete}>Delete</AlertDialogAction>
              </AlertDialogFooter>
            </AlertDialogContent>
          </AlertDialog>
        </div>
      }
    />
  )
}

export { EndpointsPage }
