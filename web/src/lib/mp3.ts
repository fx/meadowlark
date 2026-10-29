// Client-side MP3 encoding of a decoded take. The encoder is loaded on first
// use so it stays out of the main bundle.
import type { DecodedWav } from '@/lib/wav'

/** Sample rates an MPEG-1/2/2.5 Layer III stream can carry. */
export const MP3_SAMPLE_RATES = [8000, 11025, 12000, 16000, 22050, 24000, 32000, 44100, 48000]

const MP3_KBPS = 128

/** Samples per MP3 frame; the encoder is fed one frame's worth at a time. */
const FRAME_SAMPLES = 1152

export function canEncodeMp3(sampleRate: number): boolean {
  return MP3_SAMPLE_RATES.includes(sampleRate)
}

export function mp3UnsupportedReason(sampleRate: number): string {
  return `MP3 needs a standard sample rate (got ${sampleRate} Hz)`
}

function toInt16(samples: Float32Array): Int16Array {
  const out = new Int16Array(samples.length)
  for (let i = 0; i < samples.length; i++) {
    const s = Math.max(-1, Math.min(1, samples[i]))
    out[i] = s < 0 ? s * 0x8000 : s * 0x7fff
  }
  return out
}

function average(channels: Float32Array[]): Float32Array {
  const out = new Float32Array(channels[0].length)
  for (const ch of channels) {
    for (let i = 0; i < out.length; i++) out[i] += ch[i] / channels.length
  }
  return out
}

/** Mono and stereo pass through; more channels fold to stereo (even → left, odd → right). */
export function toStereoAtMost(channelData: Float32Array[]): Float32Array[] {
  if (channelData.length <= 2) return channelData
  return [
    average(channelData.filter((_, i) => i % 2 === 0)),
    average(channelData.filter((_, i) => i % 2 === 1)),
  ]
}

export async function encodeMp3(wav: DecodedWav): Promise<Blob> {
  if (!canEncodeMp3(wav.sampleRate)) {
    throw new Error(mp3UnsupportedReason(wav.sampleRate))
  }
  const { Mp3Encoder } = await import('@breezystack/lamejs')
  const [left, right] = toStereoAtMost(wav.channelData).map(toInt16)
  const encoder = new Mp3Encoder(right ? 2 : 1, wav.sampleRate, MP3_KBPS)
  // The encoder returns Int8Array views at runtime whatever its typings say,
  // so each frame is copied out as plain bytes.
  const parts: Uint8Array<ArrayBuffer>[] = []
  for (let i = 0; i < left.length; i += FRAME_SAMPLES) {
    const l = left.subarray(i, i + FRAME_SAMPLES)
    const frame = right
      ? encoder.encodeBuffer(l, right.subarray(i, i + FRAME_SAMPLES))
      : encoder.encodeBuffer(l)
    parts.push(new Uint8Array(frame))
  }
  parts.push(new Uint8Array(encoder.flush()))
  return new Blob(parts, { type: 'audio/mpeg' })
}
