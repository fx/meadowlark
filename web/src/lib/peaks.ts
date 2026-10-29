// Downsample decoded audio into waveform buckets.

export interface Peaks {
  /** Per-bucket maximum absolute amplitude across all channels, 0..1. */
  peak: Float32Array
  /** Per-bucket root-mean-square amplitude across all channels, 0..1. */
  rms: Float32Array
}

/**
 * Split the audio into `buckets` equal spans and measure each. When there are
 * fewer samples than buckets, neighbouring buckets share a sample.
 */
export function computePeaks(channelData: Float32Array[], buckets: number): Peaks {
  const peak = new Float32Array(buckets)
  const rms = new Float32Array(buckets)
  const length = channelData.length > 0 ? channelData[0].length : 0
  if (length === 0) return { peak, rms }

  for (let b = 0; b < buckets; b++) {
    const start = Math.floor((b * length) / buckets)
    const end = Math.max(Math.floor(((b + 1) * length) / buckets), start + 1)
    let max = 0
    let sumSquares = 0
    let count = 0
    for (const samples of channelData) {
      for (let i = start; i < end; i++) {
        const v = Math.abs(samples[i])
        if (v > max) max = v
        sumSquares += v * v
        count++
      }
    }
    peak[b] = max
    rms[b] = Math.sqrt(sumSquares / count)
  }
  return { peak, rms }
}
