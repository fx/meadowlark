import { Plus, Trash, Waveform } from '@phosphor-icons/react'
import { useState } from 'preact/hooks'
import { Link, useLocation } from 'wouter'
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
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { invalidateCache, useFetchList } from '@/hooks/use-fetch'
import { useMutation } from '@/hooks/use-mutation'
import type { Endpoint, ResolvedVoice, UpdateVoiceAlias, VoiceAlias } from '@/lib/api'
import { cn } from '@/lib/utils'

/** The playground, preset to a canonical voice. */
function tryHref(voice: ResolvedVoice): string {
  const query = new URLSearchParams({
    endpoint: voice.endpoint_id,
    model: voice.model,
    voice: voice.voice,
  })
  return `/playground?${query}`
}

/**
 * Every voice Meadowlark can speak with: all aliases, disabled ones included,
 * and the canonical voices of enabled endpoints. Aliases open the playground
 * to edit them; canonical voices open it preset to try them.
 */
function VoicesPage() {
  const aliases = useFetchList<VoiceAlias>('/api/v1/aliases')
  const voices = useFetchList<ResolvedVoice>('/api/v1/voices')
  const { data: endpoints } = useFetchList<Endpoint>('/api/v1/endpoints')
  const [, setLocation] = useLocation()
  const [search, setSearch] = useState('')

  const error = aliases.error ?? voices.error
  if (error) {
    return <div className="p-4 text-destructive">Error: {error.message}</div>
  }
  if (!aliases.data || !voices.data) {
    return <div className="p-4 text-muted-foreground">Loading voices...</div>
  }

  const refresh = () => {
    invalidateCache('/api/v1/voices')
    aliases.mutate()
    voices.mutate()
  }

  const endpointNames = new Map((endpoints ?? []).map((ep) => [ep.id, ep.name]))
  const matches = (name: string) => name.toLowerCase().includes(search.toLowerCase())
  const aliasRows = [...aliases.data]
    .sort((a, b) => a.name.localeCompare(b.name))
    .filter((a) => matches(a.name))
  const canonicalRows = voices.data.filter((v) => !v.is_alias && matches(v.name))

  return (
    <div className="p-4 space-y-4">
      <div className="flex gap-2">
        <Input
          placeholder="Search voices..."
          value={search}
          onInput={(e) => setSearch((e.target as HTMLInputElement).value)}
        />
        <Button type="button" onClick={() => setLocation('/playground')}>
          <Plus />
          New alias
        </Button>
      </div>

      {aliasRows.length + canonicalRows.length > 0 ? (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Voice Name</TableHead>
              <TableHead>Voice</TableHead>
              <TableHead>Endpoint</TableHead>
              <TableHead>Model</TableHead>
              <TableHead>Type</TableHead>
              <TableHead className="text-right">Actions</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {aliasRows.map((alias) => (
              <AliasRow
                key={alias.id}
                alias={alias}
                endpointName={endpointNames.get(alias.endpoint_id) ?? 'Unknown'}
                onChange={refresh}
              />
            ))}
            {canonicalRows.map((voice) => (
              <TableRow key={`${voice.endpoint_id}-${voice.model}-${voice.voice}`}>
                <TableCell>{voice.name}</TableCell>
                <TableCell>{voice.voice}</TableCell>
                <TableCell>{voice.endpoint}</TableCell>
                <TableCell>{voice.model}</TableCell>
                <TableCell>
                  <Badge>canonical</Badge>
                </TableCell>
                <TableCell className="text-right">
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon-sm"
                    onClick={() => setLocation(tryHref(voice))}
                    aria-label={`Try ${voice.name} in playground`}
                    title="Try in playground"
                  >
                    <Waveform />
                  </Button>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      ) : (
        <div className="text-muted-foreground">No voices found</div>
      )}
    </div>
  )
}

type AliasRowProps = {
  alias: VoiceAlias
  endpointName: string
  onChange: () => void
}

function AliasRow({ alias, endpointName, onChange }: AliasRowProps) {
  const [, setLocation] = useLocation()
  const update = useMutation<UpdateVoiceAlias, VoiceAlias>(`/api/v1/aliases/${alias.id}`, 'PUT')
  const remove = useMutation<void, void>(`/api/v1/aliases/${alias.id}`, 'DELETE')
  const href = `/playground/${alias.id}`

  const setEnabled = async (enabled: boolean) => {
    try {
      await update.trigger({ enabled })
    } catch {
      // The refetch below shows the stored state.
    }
    onChange()
  }

  const handleDelete = async () => {
    try {
      await remove.trigger()
    } catch {
      // The refetch below shows the stored state.
    }
    onChange()
  }

  return (
    <TableRow className={cn(!alias.enabled && 'text-muted-foreground')}>
      <TableCell>
        <Link href={href} className="font-medium hover:underline">
          {alias.name}
        </Link>
      </TableCell>
      <TableCell>{alias.voice}</TableCell>
      <TableCell>{endpointName}</TableCell>
      <TableCell>{alias.model}</TableCell>
      <TableCell>
        <div className="flex gap-1">
          <Badge variant="secondary">alias</Badge>
          {!alias.enabled && <Badge variant="outline">disabled</Badge>}
        </div>
      </TableCell>
      <TableCell>
        <div className="flex items-center justify-end gap-1">
          <Button
            type="button"
            variant="ghost"
            size="icon-sm"
            onClick={() => setLocation(href)}
            aria-label={`Open ${alias.name} in playground`}
            title="Open in playground"
          >
            <Waveform />
          </Button>
          <Switch
            checked={alias.enabled}
            onCheckedChange={setEnabled}
            disabled={update.isMutating}
            aria-label={`Toggle ${alias.name}`}
          />
          <AlertDialog>
            <AlertDialogTrigger asChild>
              <Button
                type="button"
                variant="ghost"
                size="icon-sm"
                aria-label={`Delete ${alias.name}`}
                title="Delete"
              >
                <Trash />
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
        </div>
      </TableCell>
    </TableRow>
  )
}

export { VoicesPage }
