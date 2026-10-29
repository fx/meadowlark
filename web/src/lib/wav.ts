// Minimal PCM WAV decoder for audio returned by POST /api/v1/synthesize.

export interface DecodedWav {
  sampleRate: number
  channels: number
  bitsPerSample: number
  /** Duration in seconds. */
  duration: number
  /** One array per channel, samples scaled to [-1, 1). */
  channelData: Float32Array[]
}

interface WavFormat {
  sampleRate: number
  channels: number
  bitsPerSample: number
}

const SUPPORTED_BITS = [8, 16, 24, 32]

function chunkId(view: DataView, offset: number): string {
  return String.fromCharCode(
    view.getUint8(offset),
    view.getUint8(offset + 1),
    view.getUint8(offset + 2),
    view.getUint8(offset + 3),
  )
}

function readFormat(view: DataView, offset: number, size: number): WavFormat {
  if (size < 16 || offset + size > view.byteLength) {
    throw new Error('WAV fmt chunk is truncated')
  }
  const audioFormat = view.getUint16(offset, true)
  if (audioFormat !== 1) {
    throw new Error(`Unsupported WAV audio format ${audioFormat} (expected PCM)`)
  }
  const channels = view.getUint16(offset + 2, true)
  const sampleRate = view.getUint32(offset + 4, true)
  const bitsPerSample = view.getUint16(offset + 14, true)
  if (channels === 0 || sampleRate === 0 || !SUPPORTED_BITS.includes(bitsPerSample)) {
    throw new Error(
      `Unsupported PCM layout (${channels} channels, ${sampleRate} Hz, ${bitsPerSample}-bit)`,
    )
  }
  return { sampleRate, channels, bitsPerSample }
}

function sampleReader(view: DataView, bitsPerSample: number): (offset: number) => number {
  switch (bitsPerSample) {
    case 8:
      return (o) => (view.getUint8(o) - 128) / 128
    case 16:
      return (o) => view.getInt16(o, true) / 32768
    case 24:
      return (o) => {
        const v = view.getUint8(o) | (view.getUint8(o + 1) << 8) | (view.getUint8(o + 2) << 16)
        return ((v << 8) >> 8) / 8388608
      }
    default:
      return (o) => view.getInt32(o, true) / 2147483648
  }
}

function decode(view: DataView, offset: number, size: number, fmt: WavFormat): DecodedWav {
  const bytesPerSample = fmt.bitsPerSample / 8
  const frameSize = bytesPerSample * fmt.channels
  const frames = Math.floor(size / frameSize)
  const read = sampleReader(view, fmt.bitsPerSample)
  const channelData = Array.from({ length: fmt.channels }, () => new Float32Array(frames))
  for (let i = 0; i < frames; i++) {
    const base = offset + i * frameSize
    for (let c = 0; c < fmt.channels; c++) {
      channelData[c][i] = read(base + c * bytesPerSample)
    }
  }
  return { ...fmt, duration: frames / fmt.sampleRate, channelData }
}

/** Decode a PCM WAV file. Throws on anything that is not well-formed PCM WAV. */
export function parseWav(buf: ArrayBuffer): DecodedWav {
  const view = new DataView(buf)
  if (buf.byteLength < 12 || chunkId(view, 0) !== 'RIFF') {
    throw new Error('Not a RIFF file')
  }
  if (chunkId(view, 8) !== 'WAVE') {
    throw new Error('Not a WAVE file')
  }

  let fmt: WavFormat | undefined
  let offset = 12
  while (offset + 8 <= buf.byteLength) {
    const id = chunkId(view, offset)
    const size = view.getUint32(offset + 4, true)
    const body = offset + 8
    if (id === 'fmt ') {
      fmt = readFormat(view, body, size)
    } else if (id === 'data') {
      if (!fmt) {
        throw new Error('WAV data chunk comes before the fmt chunk')
      }
      if (body + size > buf.byteLength) {
        throw new Error('WAV data chunk is truncated')
      }
      return decode(view, body, size, fmt)
    }
    // Chunks are word-aligned: an odd-sized chunk is followed by a pad byte.
    offset = body + size + (size % 2)
  }
  throw new Error(fmt ? 'WAV has no data chunk' : 'WAV has no fmt chunk')
}
