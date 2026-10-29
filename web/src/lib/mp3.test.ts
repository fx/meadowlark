import { describe, expect, it } from 'vitest'
import {
  canEncodeMp3,
  encodeMp3,
  MP3_SAMPLE_RATES,
  mp3UnsupportedReason,
  toStereoAtMost,
} from './mp3'
import type { DecodedWav } from './wav'

function decoded(channels: number, sampleRate = 24000, frames = 3000): DecodedWav {
  return {
    sampleRate,
    channels,
    bitsPerSample: 16,
    duration: frames / sampleRate,
    channelData: Array.from({ length: channels }, (_, c) =>
      Float32Array.from(
        { length: frames },
        (_, i) => Math.sin((i + c) / 6) * (c === 0 ? 1.5 : -1.5),
      ),
    ),
  }
}

async function bytesOf(blob: Blob): Promise<Uint8Array> {
  return new Uint8Array(await blob.arrayBuffer())
}

describe('canEncodeMp3', () => {
  it('accepts every standard MP3 sample rate', () => {
    for (const rate of MP3_SAMPLE_RATES) expect(canEncodeMp3(rate)).toBe(true)
  })

  it('rejects non-standard rates', () => {
    expect(canEncodeMp3(96000)).toBe(false)
    expect(canEncodeMp3(23000)).toBe(false)
  })

  it('explains why', () => {
    expect(mp3UnsupportedReason(96000)).toBe('MP3 needs a standard sample rate (got 96000 Hz)')
  })
})

describe('toStereoAtMost', () => {
  it('passes mono and stereo through', () => {
    const mono = [new Float32Array([1])]
    expect(toStereoAtMost(mono)).toBe(mono)
    const stereo = [new Float32Array([1]), new Float32Array([2])]
    expect(toStereoAtMost(stereo)).toBe(stereo)
  })

  it('folds more channels to stereo, even to left and odd to right', () => {
    const out = toStereoAtMost([
      new Float32Array([0.2]),
      new Float32Array([0.4]),
      new Float32Array([0.6]),
      new Float32Array([0.8]),
    ])
    expect(out).toHaveLength(2)
    expect(out[0][0]).toBeCloseTo(0.4)
    expect(out[1][0]).toBeCloseTo(0.6)
  })
})

describe('encodeMp3', () => {
  it('encodes mono to an MPEG audio stream', async () => {
    const blob = await encodeMp3(decoded(1))
    expect(blob.type).toBe('audio/mpeg')
    const bytes = await bytesOf(blob)
    expect(bytes.length).toBeGreaterThan(0)
    expect(bytes[0]).toBe(0xff)
    expect(bytes[1] & 0xe0).toBe(0xe0)
  })

  it('encodes stereo', async () => {
    const bytes = await bytesOf(await encodeMp3(decoded(2, 44100)))
    expect(bytes[0]).toBe(0xff)
    expect(bytes[1] & 0xe0).toBe(0xe0)
  })

  it('downmixes more than two channels', async () => {
    const bytes = await bytesOf(await encodeMp3(decoded(3, 48000)))
    expect(bytes.length).toBeGreaterThan(0)
  })

  it('refuses a non-standard sample rate', async () => {
    await expect(encodeMp3(decoded(1, 96000))).rejects.toThrow(
      'MP3 needs a standard sample rate (got 96000 Hz)',
    )
  })
})
