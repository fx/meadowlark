import { ArrowCounterClockwise, DownloadSimple, Trash } from '@phosphor-icons/react'
import { useState } from 'preact/hooks'
import { Button } from '@/components/ui/button'
import { Waveform } from '@/components/waveform'
import { downloadBlob, downloadFilename } from '@/lib/download'
import { canEncodeMp3, encodeMp3, mp3UnsupportedReason } from '@/lib/mp3'
import type { Take } from '@/lib/takes'
import { cn } from '@/lib/utils'
import { parseWav } from '@/lib/wav'

function formatRate(hz: number): string {
  return `${hz / 1000} kHz`
}

function formatChannels(n: number): string {
  if (n === 1) return 'mono'
  if (n === 2) return 'stereo'
  return `${n} ch`
}

export function formatBytes(n: number): string {
  if (n < 1024 * 1024) return `${Math.max(1, Math.round(n / 1024))} KB`
  return `${(n / (1024 * 1024)).toFixed(1)} MB`
}

export function formatSeconds(s: number): string {
  return `${s.toFixed(2)} s`
}

/** `24 kHz · 16-bit · mono · 3.42 s · 160 KB · 1.24 s latency` */
export function describeTake(take: Take): string {
  return [
    formatRate(take.format.sampleRate),
    `${take.format.bitsPerSample}-bit`,
    formatChannels(take.format.channels),
    formatSeconds(take.duration),
    formatBytes(take.blob.size),
    `${formatSeconds(take.latencyMs / 1000)} latency`,
  ].join(' · ')
}

/** `voice · model · endpoint` */
export function describeParams(take: Take): string {
  return [take.params.voice, take.params.model, take.params.endpointName].join(' · ')
}

type DownloadButtonsProps = {
  take: Take
  /** Icon-sized buttons for the take list. */
  compact?: boolean
}

function DownloadButtons({ take, compact }: DownloadButtonsProps) {
  const [encoding, setEncoding] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const mp3Ok = canEncodeMp3(take.format.sampleRate)
  const filename = (ext: string) => downloadFilename(take.params.voice, ext, take.createdAt)
  const label = (fmt: string) => `Download take #${take.n} as ${fmt}`

  const downloadMp3 = async () => {
    setEncoding(true)
    setError(null)
    try {
      const mp3 = await encodeMp3(parseWav(await take.blob.arrayBuffer()))
      downloadBlob(mp3, filename('mp3'))
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setEncoding(false)
    }
  }

  const size = compact ? 'xs' : 'sm'
  return (
    <div className="flex flex-wrap items-center gap-1">
      {/* The take's own object URL: the download is exactly what the server sent. */}
      <Button asChild variant="outline" size={size}>
        <a href={take.url} download={filename('wav')} aria-label={label('WAV')}>
          <DownloadSimple />
          WAV
        </a>
      </Button>
      <Button
        type="button"
        variant="outline"
        size={size}
        onClick={downloadMp3}
        disabled={!mp3Ok || encoding}
        title={mp3Ok ? undefined : mp3UnsupportedReason(take.format.sampleRate)}
        aria-label={label('MP3')}
      >
        <DownloadSimple />
        {encoding ? 'Encoding…' : 'MP3'}
      </Button>
      {error && (
        <span role="alert" className="text-xs text-destructive">
          {error}
        </span>
      )}
    </div>
  )
}

type TakePlayerProps = {
  take: Take
  /** Plays the take from the start whenever it changes; see Waveform. */
  playRequest?: number
}

/** The main player: the selected take's waveform, format line, and downloads. */
function TakePlayer({ take, playRequest }: TakePlayerProps) {
  return (
    <section aria-label={`Take #${take.n}`} className="space-y-3 border bg-card p-4 pt-8">
      <Waveform
        key={take.id}
        peaks={take.peaks}
        duration={take.duration}
        src={take.url}
        playRequest={playRequest}
      />
      <div className="flex flex-wrap items-center justify-between gap-2">
        <p className="font-mono text-xs text-muted-foreground tabular-nums">
          <span className="font-semibold text-foreground">#{take.n}</span> {describeTake(take)}
        </p>
        <DownloadButtons take={take} />
      </div>
    </section>
  )
}

type TakeListProps = {
  takes: Take[]
  selectedId: string | null
  onSelect: (take: Take) => void
  onRestore: (take: Take) => void
  onRemove: (take: Take) => void
  onClear: () => void
}

function TakeList({ takes, selectedId, onSelect, onRestore, onRemove, onClear }: TakeListProps) {
  return (
    <section aria-label="Takes" className="space-y-2">
      <div className="flex items-center justify-between">
        <h2 className="text-sm font-semibold">
          Takes <span className="text-muted-foreground">({takes.length})</span>
        </h2>
        <Button type="button" variant="ghost" size="xs" onClick={onClear}>
          Clear
        </Button>
      </div>
      <ul className="space-y-1">
        {takes.map((take) => {
          const selected = take.id === selectedId
          const extras = [
            take.params.speed !== undefined ? `speed ${take.params.speed}` : null,
            take.params.instructions ? `“${take.params.instructions}”` : null,
          ].filter(Boolean)
          return (
            <li
              key={take.id}
              className={cn(
                'flex items-center gap-3 border p-2 transition-colors',
                selected ? 'border-primary bg-primary/5' : 'hover:bg-muted/50',
              )}
            >
              <button
                type="button"
                className="h-10 w-28 shrink-0 cursor-pointer outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50"
                onClick={() => onSelect(take)}
                aria-label={`Play take #${take.n}`}
                aria-pressed={selected}
              >
                <Waveform
                  compact
                  active={selected}
                  peaks={take.miniPeaks}
                  duration={take.duration}
                  src={take.url}
                />
              </button>
              <div className="min-w-0 flex-1 space-y-0.5 text-xs">
                <div className="flex items-baseline gap-2">
                  <span className="font-semibold">#{take.n}</span>
                  <span className="truncate">{describeParams(take)}</span>
                  <span className="ml-auto shrink-0 font-mono text-muted-foreground tabular-nums">
                    {formatSeconds(take.latencyMs / 1000)} → {formatSeconds(take.duration)}
                  </span>
                </div>
                {extras.length > 0 && (
                  <p className="truncate text-muted-foreground">{extras.join(' · ')}</p>
                )}
                <p className="truncate text-muted-foreground" title={take.text}>
                  {take.text}
                </p>
              </div>
              <div className="flex shrink-0 items-center gap-1">
                <Button
                  type="button"
                  variant="ghost"
                  size="icon-xs"
                  onClick={() => onRestore(take)}
                  aria-label={`Restore settings from take #${take.n}`}
                  title="Restore settings"
                >
                  <ArrowCounterClockwise />
                </Button>
                <DownloadButtons take={take} compact />
                <Button
                  type="button"
                  variant="ghost"
                  size="icon-xs"
                  onClick={() => onRemove(take)}
                  aria-label={`Remove take #${take.n}`}
                  title="Remove"
                >
                  <Trash />
                </Button>
              </div>
            </li>
          )
        })}
      </ul>
    </section>
  )
}

export { DownloadButtons, TakeList, TakePlayer }
