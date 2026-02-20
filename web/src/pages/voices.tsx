import { useState } from 'preact/hooks'
import { Badge } from '@/components/ui/badge'
import { Input } from '@/components/ui/input'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { useFetch } from '@/hooks/use-fetch'
import type { ResolvedVoice } from '@/lib/api'

function VoicesPage() {
  const { data: voices, error, isLoading } = useFetch<ResolvedVoice[]>('/api/v1/voices')
  const [search, setSearch] = useState('')

  const filtered = voices?.filter((v) => v.name.toLowerCase().includes(search.toLowerCase()))

  if (isLoading) {
    return <div className="p-4 text-muted-foreground">Loading voices...</div>
  }

  if (error) {
    return <div className="p-4 text-destructive">Error: {error.message}</div>
  }

  return (
    <div className="p-4 space-y-4">
      <Input
        placeholder="Search voices..."
        value={search}
        onInput={(e) => setSearch((e.target as HTMLInputElement).value)}
      />

      {filtered && filtered.length > 0 ? (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Voice Name</TableHead>
              <TableHead>Voice</TableHead>
              <TableHead>Endpoint</TableHead>
              <TableHead>Model</TableHead>
              <TableHead>Type</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {filtered.map((voice, index) => (
              <TableRow key={`${voice.endpoint}-${voice.model}-${voice.name}-${index}`}>
                <TableCell>{voice.name}</TableCell>
                <TableCell>{voice.voice}</TableCell>
                <TableCell>{voice.endpoint}</TableCell>
                <TableCell>{voice.model}</TableCell>
                <TableCell>
                  <Badge variant={voice.is_alias ? 'secondary' : 'default'}>
                    {voice.is_alias ? 'alias' : 'canonical'}
                  </Badge>
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

export { VoicesPage }
