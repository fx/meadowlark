// Test helpers for building WAV files and decoded takes.
import type { Peaks } from '@/lib/peaks'
import type { NewTake } from '@/lib/takes'

export interface WavSpec {
  sampleRate?: number
  channels?: number
  bitsPerSample?: number
  audioFormat?: number
  /** Interleaved integer sample values, written at `bitsPerSample`. */
  samples?: number[]
  /** Extra chunks written between fmt and data, as [id, payload]. */
  extraChunks?: [string, number[]][]
  /** Override the data chunk's declared size. */
  dataSize?: number
}

function writeTag(view: DataView, offset: number, tag: string) {
  for (let i = 0; i < 4; i++) view.setUint8(offset + i, tag.charCodeAt(i))
}

/** Build a PCM WAV file as an ArrayBuffer. */
export function buildWav({
  sampleRate = 24000,
  channels = 1,
  bitsPerSample = 16,
  audioFormat = 1,
  samples = [0, 1000, -1000, 0],
  extraChunks = [],
  dataSize,
}: WavSpec = {}): ArrayBuffer {
  const bytes = bitsPerSample / 8
  const pcmLen = samples.length * bytes
  const extraLen = extraChunks.reduce((n, [, p]) => n + 8 + p.length + (p.length % 2), 0)
  const buf = new ArrayBuffer(44 + extraLen + pcmLen)
  const view = new DataView(buf)
  writeTag(view, 0, 'RIFF')
  view.setUint32(4, buf.byteLength - 8, true)
  writeTag(view, 8, 'WAVE')
  writeTag(view, 12, 'fmt ')
  view.setUint32(16, 16, true)
  view.setUint16(20, audioFormat, true)
  view.setUint16(22, channels, true)
  view.setUint32(24, sampleRate, true)
  view.setUint32(28, sampleRate * channels * bytes, true)
  view.setUint16(32, channels * bytes, true)
  view.setUint16(34, bitsPerSample, true)
  let o = 36
  for (const [id, payload] of extraChunks) {
    writeTag(view, o, id)
    view.setUint32(o + 4, payload.length, true)
    payload.forEach((b, i) => {
      view.setUint8(o + 8 + i, b)
    })
    o += 8 + payload.length + (payload.length % 2)
  }
  writeTag(view, o, 'data')
  view.setUint32(o + 4, dataSize ?? pcmLen, true)
  o += 8
  for (const s of samples) {
    if (bytes === 1) view.setUint8(o, s)
    else if (bytes === 2) view.setInt16(o, s, true)
    else if (bytes === 3) {
      view.setUint8(o, s & 0xff)
      view.setUint8(o + 1, (s >> 8) & 0xff)
      view.setUint8(o + 2, (s >> 16) & 0xff)
    } else view.setInt32(o, s, true)
    o += bytes
  }
  return buf
}

/** A 16-bit mono sine WAV of `seconds` at `sampleRate`. */
export function sineWav(seconds = 0.1, sampleRate = 24000): ArrayBuffer {
  const n = Math.round(seconds * sampleRate)
  const samples = Array.from({ length: n }, (_, i) => Math.round(Math.sin(i / 8) * 12000))
  return buildWav({ sampleRate, samples })
}

export function flatPeaks(n: number, value = 0.5): Peaks {
  return { peak: new Float32Array(n).fill(value), rms: new Float32Array(n).fill(value / 2) }
}

let urlCounter = 0

/** A NewTake with sensible defaults for store and page tests. */
export function makeNewTake(overrides: Partial<NewTake> = {}): NewTake {
  urlCounter++
  return {
    params: { endpointId: 'ep-1', endpointName: 'OpenAI', model: 'tts-1', voice: 'alloy' },
    text: 'Hello',
    blob: new Blob([sineWav()], { type: 'audio/wav' }),
    url: `blob:take/${urlCounter}`,
    format: { sampleRate: 24000, channels: 1, bitsPerSample: 16 },
    duration: 3.42,
    peaks: flatPeaks(200),
    miniPeaks: flatPeaks(64),
    latencyMs: 1240,
    createdAt: new Date(2026, 9, 29, 14, 3, 7),
    ...overrides,
  }
}
