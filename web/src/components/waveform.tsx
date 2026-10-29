import { Pause, Play } from '@phosphor-icons/react'
import { useEffect, useId, useMemo, useRef, useState } from 'preact/hooks'
import { Button } from '@/components/ui/button'
import type { Peaks } from '@/lib/peaks'
import { cn } from '@/lib/utils'

/** Format seconds as `m:ss.s`. */
export function formatTime(seconds: number): string {
  const tenths = Math.round(Math.max(0, seconds) * 10)
  const m = Math.floor(tenths / 600)
  const s = (tenths % 600) / 10
  return `${m}:${s.toFixed(1).padStart(4, '0')}`
}

/** Browsers reject play() under autoplay policies; the button still works. */
function playQuietly(audio: HTMLAudioElement) {
  audio.play().catch(() => {})
}

/** Minimum bar height, in viewBox units, so silence still draws a baseline. */
const MIN_BAR = 1.5

function bars(values: Float32Array, scale: number) {
  return Array.from(values, (v, i) => {
    const h = Math.max(v * scale * 100, MIN_BAR)
    return <rect key={i} x={i + 0.15} y={50 - h / 2} width={0.7} height={h} />
  })
}

type WaveformBarsProps = {
  peaks: Peaks
  /** Fraction of the audio that has played, 0..1. */
  progress: number
  className?: string
}

/**
 * Mirrored waveform bars in two layers — the peak envelope, lighter, behind
 * the RMS body — drawn once in the unplayed colour and once in the played
 * colour clipped to `progress`. Colours are theme tokens, so both themes work
 * without JavaScript.
 */
function WaveformBars({ peaks, progress, className }: WaveformBarsProps) {
  const clipId = `wave-${useId()}`
  const n = peaks.peak.length
  // Normalise to the loudest bucket so quiet takes still fill the height.
  const layers = useMemo(() => {
    const max = peaks.peak.reduce((a, b) => Math.max(a, b), 0)
    const scale = max > 0 ? 1 / max : 1
    return { peak: bars(peaks.peak, scale), rms: bars(peaks.rms, scale) }
  }, [peaks])

  return (
    <svg
      viewBox={`0 0 ${n} 100`}
      preserveAspectRatio="none"
      aria-hidden="true"
      className={cn('block h-full w-full', className)}
    >
      <defs>
        <clipPath id={clipId}>
          <rect data-testid="played-clip" x={0} y={0} width={progress * n} height={100} />
        </clipPath>
      </defs>
      <g className="origin-center animate-waveform-grow motion-reduce:animate-none">
        <g data-layer="peak" className="fill-muted-foreground/20">
          {layers.peak}
        </g>
        <g data-layer="rms" className="fill-muted-foreground/45">
          {layers.rms}
        </g>
        <g clipPath={`url(#${clipId})`}>
          <g className="fill-primary/40">{layers.peak}</g>
          <g className="fill-primary">{layers.rms}</g>
        </g>
      </g>
    </svg>
  )
}

type WaveformProps = {
  peaks: Peaks
  /** Seconds. */
  duration: number
  /** Audio source, usually an object URL. */
  src: string
  /** Bars only: no playback, seeking, or labels. */
  compact?: boolean
  /** In compact mode, draw the bars in the played colour. */
  active?: boolean
  /** Start playing on mount. A rejected play() (autoplay policy) is ignored. */
  autoPlay?: boolean
  className?: string
}

function Waveform({ compact, active, peaks, className, ...player }: WaveformProps) {
  if (compact) {
    return <WaveformBars peaks={peaks} progress={active ? 1 : 0} className={className} />
  }
  return <WaveformPlayer peaks={peaks} className={className} {...player} />
}

function WaveformPlayer({
  peaks,
  duration,
  src,
  autoPlay,
  className,
}: Omit<WaveformProps, 'compact' | 'active'>) {
  // The element is never rendered: playback has no visible controls of its own.
  const [audio] = useState(() => new Audio())
  const [time, setTime] = useState(0)
  const [playing, setPlaying] = useState(false)
  const [hover, setHover] = useState<number | null>(null)
  const autoPlayRef = useRef(autoPlay)

  useEffect(() => {
    const onPlay = () => setPlaying(true)
    const onStop = () => {
      setPlaying(false)
      setTime(audio.currentTime)
    }
    const onSeeked = () => setTime(audio.currentTime)
    audio.preload = 'auto'
    audio.src = src
    audio.addEventListener('play', onPlay)
    audio.addEventListener('pause', onStop)
    audio.addEventListener('ended', onStop)
    audio.addEventListener('seeked', onSeeked)
    if (autoPlayRef.current) playQuietly(audio)
    return () => {
      audio.removeEventListener('play', onPlay)
      audio.removeEventListener('pause', onStop)
      audio.removeEventListener('ended', onStop)
      audio.removeEventListener('seeked', onSeeked)
      audio.pause()
    }
  }, [audio, src])

  // Follow the playhead every frame while playing, for a smooth sweep rather
  // than the coarse steps timeupdate gives.
  useEffect(() => {
    if (!playing) return
    let id = 0
    const tick = () => {
      setTime(audio.currentTime)
      id = requestAnimationFrame(tick)
    }
    id = requestAnimationFrame(tick)
    return () => cancelAnimationFrame(id)
  }, [audio, playing])

  const toggle = () => {
    if (audio.paused) playQuietly(audio)
    else audio.pause()
  }

  const seek = (value: number) => {
    audio.currentTime = value
    setTime(value)
  }

  const progress = duration > 0 ? Math.min(time / duration, 1) : 0

  const handleKeyDown = (e: KeyboardEvent) => {
    if (e.key === ' ') {
      e.preventDefault()
      toggle()
      return
    }
    // A 10 ms step is right for dragging and far too fine for arrow keys.
    const step = Math.max(duration / 20, 0.1)
    if (e.key === 'ArrowRight' || e.key === 'ArrowUp') {
      e.preventDefault()
      seek(Math.min(time + step, duration))
    } else if (e.key === 'ArrowLeft' || e.key === 'ArrowDown') {
      e.preventDefault()
      seek(Math.max(time - step, 0))
    }
  }

  const handlePointerMove = (e: PointerEvent) => {
    const rect = (e.currentTarget as HTMLElement).getBoundingClientRect()
    if (rect.width <= 0) return
    setHover(Math.min(Math.max((e.clientX - rect.left) / rect.width, 0), 1))
  }

  return (
    <div className={cn('flex items-center gap-3', className)}>
      <Button
        type="button"
        size="icon-lg"
        aria-label={playing ? 'Pause' : 'Play'}
        onClick={toggle}
        className="shrink-0"
      >
        {playing ? <Pause weight="fill" /> : <Play weight="fill" />}
      </Button>
      <div className="min-w-0 flex-1 space-y-1">
        <div className="relative h-24 has-[input:focus-visible]:ring-[3px] has-[input:focus-visible]:ring-ring/50">
          <WaveformBars peaks={peaks} progress={progress} />
          <div
            data-testid="playhead"
            className="pointer-events-none absolute inset-y-0 w-px bg-primary shadow-[0_0_6px_var(--primary)]"
            style={{ left: `${progress * 100}%` }}
          />
          {hover !== null && (
            <>
              <div
                data-testid="hover-line"
                className="pointer-events-none absolute inset-y-0 w-px bg-foreground/50"
                style={{ left: `${hover * 100}%` }}
              />
              <div
                role="tooltip"
                className="pointer-events-none absolute -top-6 -translate-x-1/2 border bg-popover px-1.5 py-0.5 font-mono text-[11px] text-popover-foreground tabular-nums shadow-sm"
                style={{ left: `${hover * 100}%` }}
              >
                {formatTime(hover * duration)}
              </div>
            </>
          )}
          <input
            type="range"
            min={0}
            max={duration}
            step={0.01}
            value={time}
            aria-label="Seek"
            aria-valuetext={`${formatTime(time)} of ${formatTime(duration)}`}
            onInput={(e) => seek(Number((e.target as HTMLInputElement).value))}
            onKeyDown={handleKeyDown}
            onPointerMove={handlePointerMove}
            onPointerLeave={() => setHover(null)}
            className="absolute inset-0 h-full w-full cursor-pointer appearance-none opacity-0 [&::-moz-range-thumb]:w-0 [&::-moz-range-thumb]:border-0 [&::-webkit-slider-thumb]:w-0 [&::-webkit-slider-thumb]:appearance-none"
          />
        </div>
        <div className="flex justify-between font-mono text-xs text-muted-foreground tabular-nums">
          <span data-testid="current-time">{formatTime(time)}</span>
          <span data-testid="total-time">{formatTime(duration)}</span>
        </div>
      </div>
    </div>
  )
}

export type { WaveformProps }
export { Waveform, WaveformBars }
