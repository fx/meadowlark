import { describe, expect, it } from 'vitest'
import { computePeaks } from './peaks'

describe('computePeaks', () => {
  it('measures peak and RMS per bucket', () => {
    const samples = new Float32Array([0.5, -1, 0.25, 0.25])
    const { peak, rms } = computePeaks([samples], 2)
    expect(Array.from(peak)).toEqual([1, 0.25])
    expect(rms[0]).toBeCloseTo(Math.sqrt((0.25 + 1) / 2))
    expect(rms[1]).toBeCloseTo(0.25)
  })

  it('takes the loudest channel for the peak and pools channels for RMS', () => {
    const { peak, rms } = computePeaks([new Float32Array([0.2]), new Float32Array([-0.8])], 1)
    expect(peak[0]).toBeCloseTo(0.8)
    expect(rms[0]).toBeCloseTo(Math.sqrt((0.04 + 0.64) / 2))
  })

  it('returns zeros for silence', () => {
    const { peak, rms } = computePeaks([new Float32Array(100)], 10)
    expect(Array.from(peak)).toEqual(new Array(10).fill(0))
    expect(Array.from(rms)).toEqual(new Array(10).fill(0))
  })

  it('returns zeros for empty audio or no channels', () => {
    expect(Array.from(computePeaks([new Float32Array(0)], 3).peak)).toEqual([0, 0, 0])
    expect(Array.from(computePeaks([], 3).rms)).toEqual([0, 0, 0])
  })

  it('fills every bucket when there are fewer samples than buckets', () => {
    const { peak } = computePeaks([new Float32Array([0.1, 0.9])], 4)
    expect(Array.from(peak).map((v) => Number(v.toFixed(1)))).toEqual([0.1, 0.1, 0.9, 0.9])
  })
})
