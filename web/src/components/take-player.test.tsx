import { fireEvent, render, screen, waitFor, within } from '@testing-library/preact'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { Take } from '@/lib/takes'
import { buildWav, makeNewTake, sineWav } from '@/test-audio'

const encodeMp3 = vi.hoisted(() => vi.fn())
vi.mock('@/lib/mp3', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/mp3')>()),
  encodeMp3,
}))
const downloadBlob = vi.hoisted(() => vi.fn())
vi.mock('@/lib/download', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/download')>()),
  downloadBlob,
}))

import {
  describeParams,
  describeTake,
  formatBytes,
  formatSeconds,
  TakeList,
  TakePlayer,
} from './take-player'

let n = 0
function take(overrides: Partial<Take> = {}): Take {
  n++
  return { ...makeNewTake(), id: `take-${n}`, n, ...overrides }
}

beforeEach(() => {
  n = 0
  encodeMp3.mockResolvedValue(new Blob(['mp3'], { type: 'audio/mpeg' }))
})

afterEach(() => {
  encodeMp3.mockReset()
  downloadBlob.mockReset()
})

describe('formatting', () => {
  it('formats sizes and seconds', () => {
    expect(formatBytes(100)).toBe('1 KB')
    expect(formatBytes(163_840)).toBe('160 KB')
    expect(formatBytes(3 * 1024 * 1024 + 300_000)).toBe('3.3 MB')
    expect(formatSeconds(3.4189)).toBe('3.42 s')
  })

  it('describes a take format line', () => {
    const blob = new Blob([new Uint8Array(163_840)])
    expect(describeTake(take({ blob }))).toBe(
      '24 kHz · 16-bit · mono · 3.42 s · 160 KB · 1.24 s latency',
    )
    expect(
      describeTake(take({ blob, format: { sampleRate: 44100, channels: 2, bitsPerSample: 24 } })),
    ).toMatch(/^44\.1 kHz · 24-bit · stereo · /)
    expect(
      describeTake(take({ blob, format: { sampleRate: 22050, channels: 6, bitsPerSample: 16 } })),
    ).toMatch(/^22\.05 kHz · 16-bit · 6 ch · /)
  })

  it('describes take parameters', () => {
    expect(describeParams(take())).toBe('alloy · tts-1 · OpenAI')
  })
})

describe('TakePlayer', () => {
  it('shows the waveform player, format line, and downloads', () => {
    const t = take()
    render(<TakePlayer take={t} />)
    expect(screen.getByRole('region', { name: 'Take #1' })).toBeInTheDocument()
    expect(screen.getByRole('slider', { name: 'Seek' })).toBeInTheDocument()
    expect(screen.getByText(/24 kHz · 16-bit · mono/)).toBeInTheDocument()

    const wav = screen.getByRole('link', { name: 'Download take #1 as WAV' })
    expect(wav).toHaveAttribute('href', t.url)
    expect(wav).toHaveAttribute('download', 'meadowlark-alloy-20261029-140307.wav')
  })

  it('plays when a play request is present', () => {
    render(<TakePlayer take={take()} playRequest={1} />)
    expect(HTMLMediaElement.prototype.play).toHaveBeenCalled()
  })

  it('encodes and downloads MP3 from the exact WAV bytes', async () => {
    const t = take({ blob: new Blob([sineWav(0.05, 22050)], { type: 'audio/wav' }) })
    render(<TakePlayer take={t} />)
    fireEvent.click(screen.getByRole('button', { name: 'Download take #1 as MP3' }))
    expect(screen.getByRole('button', { name: 'Download take #1 as MP3' })).toHaveTextContent(
      'Encoding…',
    )
    await waitFor(() => expect(downloadBlob).toHaveBeenCalledOnce())
    expect(encodeMp3.mock.calls[0][0]).toMatchObject({ sampleRate: 22050, channels: 1 })
    expect(downloadBlob).toHaveBeenCalledWith(
      expect.any(Blob),
      'meadowlark-alloy-20261029-140307.mp3',
    )
    expect(screen.getByRole('button', { name: 'Download take #1 as MP3' })).toHaveTextContent('MP3')
  })

  it('shows an MP3 failure', async () => {
    const t = take({ blob: new Blob([buildWav({ audioFormat: 3 })]) })
    render(<TakePlayer take={t} />)
    fireEvent.click(screen.getByRole('button', { name: 'Download take #1 as MP3' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('Unsupported WAV audio format 3')
    expect(downloadBlob).not.toHaveBeenCalled()
  })

  it('reports a non-Error MP3 failure', async () => {
    encodeMp3.mockRejectedValueOnce('lame exploded')
    render(<TakePlayer take={take()} />)
    fireEvent.click(screen.getByRole('button', { name: 'Download take #1 as MP3' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('lame exploded')
  })

  it('disables MP3 for a non-standard sample rate and says why', () => {
    render(
      <TakePlayer take={take({ format: { sampleRate: 96000, channels: 1, bitsPerSample: 16 } })} />,
    )
    const mp3 = screen.getByRole('button', { name: 'Download take #1 as MP3' })
    expect(mp3).toBeDisabled()
    expect(mp3).toHaveAttribute('title', 'MP3 needs a standard sample rate (got 96000 Hz)')
  })
})

describe('TakeList', () => {
  function renderList(takes: Take[], selectedId: string | null = null) {
    const handlers = {
      onSelect: vi.fn(),
      onRestore: vi.fn(),
      onRemove: vi.fn(),
      onClear: vi.fn(),
    }
    render(<TakeList takes={takes} selectedId={selectedId} {...handlers} />)
    return handlers
  }

  it('lists takes with parameters, timing, and text', () => {
    const a = take({ text: 'First words' })
    const b = take({
      params: { ...a.params, speed: 1.5, instructions: 'whisper' },
      latencyMs: 800,
      duration: 2,
    })
    renderList([b, a], b.id)
    expect(screen.getByRole('heading', { name: 'Takes (2)' })).toBeInTheDocument()
    const items = screen.getAllByRole('listitem')
    expect(within(items[0]).getByText('#2')).toBeInTheDocument()
    expect(within(items[0]).getByText('speed 1.5 · “whisper”')).toBeInTheDocument()
    expect(within(items[0]).getByText('0.80 s → 2.00 s')).toBeInTheDocument()
    expect(within(items[1]).getByText('First words')).toBeInTheDocument()
    expect(within(items[1]).getByText('alloy · tts-1 · OpenAI')).toBeInTheDocument()
    expect(within(items[1]).queryByText(/speed/)).toBeNull()
  })

  it('shows only the extras a take has', () => {
    const a = take()
    renderList([take({ params: { ...a.params, instructions: 'calm' } })])
    expect(screen.getByText('“calm”')).toBeInTheDocument()
  })

  it('marks the selected take', () => {
    const a = take()
    const b = take()
    renderList([b, a], a.id)
    expect(screen.getByRole('button', { name: 'Play take #1' })).toHaveAttribute(
      'aria-pressed',
      'true',
    )
    expect(screen.getByRole('button', { name: 'Play take #2' })).toHaveAttribute(
      'aria-pressed',
      'false',
    )
  })

  it('wires select, restore, remove, and clear', () => {
    const a = take()
    const h = renderList([a])
    fireEvent.click(screen.getByRole('button', { name: 'Play take #1' }))
    expect(h.onSelect).toHaveBeenCalledWith(a)
    fireEvent.click(screen.getByRole('button', { name: 'Restore settings from take #1' }))
    expect(h.onRestore).toHaveBeenCalledWith(a)
    fireEvent.click(screen.getByRole('button', { name: 'Remove take #1' }))
    expect(h.onRemove).toHaveBeenCalledWith(a)
    fireEvent.click(screen.getByRole('button', { name: 'Clear' }))
    expect(h.onClear).toHaveBeenCalled()
  })

  it('offers per-take downloads', () => {
    const a = take()
    renderList([a])
    expect(screen.getByRole('link', { name: 'Download take #1 as WAV' })).toHaveAttribute(
      'href',
      a.url,
    )
    expect(screen.getByRole('button', { name: 'Download take #1 as MP3' })).toBeEnabled()
  })
})
