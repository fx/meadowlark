import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { useFetch } from '@/hooks/use-fetch'
import type { ServerStatus } from '@/lib/api'

function formatUptime(seconds: number): string {
  const days = Math.floor(seconds / 86400)
  const hours = Math.floor((seconds % 86400) / 3600)
  const minutes = Math.floor((seconds % 3600) / 60)
  const secs = seconds % 60

  const parts: string[] = []
  if (days > 0) parts.push(`${days}d`)
  if (hours > 0) parts.push(`${hours}h`)
  if (minutes > 0) parts.push(`${minutes}m`)
  parts.push(`${secs}s`)
  return parts.join(' ')
}

function StatusField({ label, value }: { label: string; value: string | number }) {
  return (
    <div className="flex justify-between py-1">
      <span className="text-muted-foreground">{label}</span>
      <span className="font-mono">{value}</span>
    </div>
  )
}

function SettingsPage() {
  const { data, error, isLoading } = useFetch<ServerStatus>('/api/v1/status')

  if (isLoading) {
    return (
      <div className="p-6">
        <h1 className="text-2xl font-bold mb-6">Settings</h1>
        <p>Loading status...</p>
      </div>
    )
  }

  if (error) {
    return (
      <div className="p-6">
        <h1 className="text-2xl font-bold mb-6">Settings</h1>
        <p className="text-destructive">Failed to load status: {error.message}</p>
      </div>
    )
  }

  if (!data) {
    return null
  }

  return (
    <div className="p-6">
      <h1 className="text-2xl font-bold mb-6">Settings</h1>
      <div className="grid gap-4 md:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle>Server Info</CardTitle>
          </CardHeader>
          <CardContent>
            <StatusField label="Version" value={data.version} />
            <StatusField label="Uptime" value={formatUptime(data.uptime_seconds)} />
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Wyoming</CardTitle>
          </CardHeader>
          <CardContent>
            <StatusField label="Port" value={data.wyoming_port} />
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>HTTP</CardTitle>
          </CardHeader>
          <CardContent>
            <StatusField label="Port" value={data.http_port} />
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Database</CardTitle>
          </CardHeader>
          <CardContent>
            <StatusField label="Driver" value={data.db_driver} />
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Voices</CardTitle>
          </CardHeader>
          <CardContent>
            <StatusField label="Total Voices" value={data.voice_count} />
            <StatusField label="Endpoints" value={data.endpoint_count} />
            <StatusField label="Aliases" value={data.alias_count} />
          </CardContent>
        </Card>
      </div>
    </div>
  )
}

export { SettingsPage, formatUptime }
