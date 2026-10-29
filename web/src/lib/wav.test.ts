import { describe, expect, it } from 'vitest'
import { buildWav } from '@/test-audio'
import { parseWav } from './wav'

describe('parseWav', () => {
  it('decodes 16-bit mono', () => {
    const wav = parseWav(buildWav({ sampleRate: 24000, samples: [0, 16384, -32768, 32767] }))
    expect(wav.sampleRate).toBe(24000)
    expect(wav.channels).toBe(1)
    expect(wav.bitsPerSample).toBe(16)
    expect(wav.duration).toBeCloseTo(4 / 24000)
    expect(Array.from(wav.channelData[0])).toEqual([0, 0.5, -1, 32767 / 32768])
  })

  it('de-interleaves stereo', () => {
    const wav = parseWav(buildWav({ channels: 2, samples: [16384, -16384, 8192, -8192] }))
    expect(wav.channelData).toHaveLength(2)
    expect(Array.from(wav.channelData[0])).toEqual([0.5, 0.25])
    expect(Array.from(wav.channelData[1])).toEqual([-0.5, -0.25])
    expect(wav.duration).toBeCloseTo(2 / 24000)
  })

  it('decodes 8-bit unsigned', () => {
    const wav = parseWav(buildWav({ bitsPerSample: 8, samples: [128, 0, 192] }))
    expect(Array.from(wav.channelData[0])).toEqual([0, -1, 0.5])
  })

  it('decodes 24-bit signed', () => {
    const wav = parseWav(buildWav({ bitsPerSample: 24, samples: [0x400000, 0xc00000, 0] }))
    expect(Array.from(wav.channelData[0])).toEqual([0.5, -0.5, 0])
  })

  it('decodes 32-bit signed', () => {
    const wav = parseWav(buildWav({ bitsPerSample: 32, samples: [1073741824, -2147483648] }))
    expect(Array.from(wav.channelData[0])).toEqual([0.5, -1])
  })

  it('skips unknown chunks, including odd-sized ones with a pad byte', () => {
    const wav = parseWav(
      buildWav({
        samples: [16384],
        extraChunks: [
          ['LIST', [1, 2, 3]],
          ['JUNK', [0, 0]],
        ],
      }),
    )
    expect(Array.from(wav.channelData[0])).toEqual([0.5])
  })

  it('ignores a trailing partial frame', () => {
    const buf = buildWav({ channels: 2, samples: [16384, 16384, 1] })
    expect(parseWav(buf).channelData[0]).toHaveLength(1)
  })

  it('rejects a buffer that is too short', () => {
    expect(() => parseWav(new ArrayBuffer(4))).toThrow('Not a RIFF file')
  })

  it('rejects a non-RIFF file', () => {
    const buf = buildWav()
    new DataView(buf).setUint8(0, 'X'.charCodeAt(0))
    expect(() => parseWav(buf)).toThrow('Not a RIFF file')
  })

  it('rejects a non-WAVE file', () => {
    const buf = buildWav()
    new DataView(buf).setUint8(8, 'X'.charCodeAt(0))
    expect(() => parseWav(buf)).toThrow('Not a WAVE file')
  })

  it('rejects non-PCM audio', () => {
    expect(() => parseWav(buildWav({ audioFormat: 3 }))).toThrow(
      'Unsupported WAV audio format 3 (expected PCM)',
    )
  })

  it('rejects an unsupported sample width', () => {
    const buf = buildWav()
    new DataView(buf).setUint16(34, 12, true)
    expect(() => parseWav(buf)).toThrow('Unsupported PCM layout (1 channels, 24000 Hz, 12-bit)')
  })

  it('rejects zero channels', () => {
    const buf = buildWav()
    new DataView(buf).setUint16(22, 0, true)
    expect(() => parseWav(buf)).toThrow('Unsupported PCM layout')
  })

  it('rejects a zero sample rate', () => {
    expect(() => parseWav(buildWav({ sampleRate: 0 }))).toThrow('Unsupported PCM layout')
  })

  it('rejects a fmt chunk smaller than 16 bytes', () => {
    const buf = buildWav()
    new DataView(buf).setUint32(16, 14, true)
    expect(() => parseWav(buf)).toThrow('WAV fmt chunk is truncated')
  })

  it('rejects a fmt chunk that runs past the end', () => {
    expect(() => parseWav(buildWav().slice(0, 30))).toThrow('WAV fmt chunk is truncated')
  })

  it('rejects a data chunk that runs past the end', () => {
    expect(() => parseWav(buildWav({ dataSize: 100 }))).toThrow('WAV data chunk is truncated')
  })

  it('rejects a data chunk before the fmt chunk', () => {
    const buf = buildWav()
    const view = new DataView(buf)
    // Rename "fmt " to an unknown chunk so data is reached first.
    view.setUint8(12, 'X'.charCodeAt(0))
    expect(() => parseWav(buf)).toThrow('WAV data chunk comes before the fmt chunk')
  })

  it('rejects a file with no data chunk', () => {
    expect(() => parseWav(buildWav().slice(0, 36))).toThrow('WAV has no data chunk')
  })

  it('rejects a file with no fmt chunk', () => {
    expect(() => parseWav(buildWav().slice(0, 12))).toThrow('WAV has no fmt chunk')
  })
})
