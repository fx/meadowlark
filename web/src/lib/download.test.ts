import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { downloadBlob, downloadFilename } from './download'

describe('downloadFilename', () => {
  it('stamps the voice and local time', () => {
    expect(downloadFilename('alloy', 'wav', new Date(2026, 0, 5, 7, 8, 9))).toBe(
      'meadowlark-alloy-20260105-070809.wav',
    )
  })

  it('makes the voice filename-safe', () => {
    expect(
      downloadFilename('clone:foo bar/é.v1_x-y', 'mp3', new Date(2026, 11, 31, 23, 59, 58)),
    ).toBe('meadowlark-clone-foo-bar--.v1_x-y-20261231-235958.mp3')
  })
})

describe('downloadBlob', () => {
  beforeEach(() => {
    vi.useFakeTimers()
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.restoreAllMocks()
  })

  it('clicks a temporary anchor and revokes its URL afterwards', () => {
    const click = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(function (
      this: HTMLAnchorElement,
    ) {
      expect(this.download).toBe('take.mp3')
      expect(this.href).toMatch(/^blob:/)
      expect(this.isConnected).toBe(true)
    })
    const blob = new Blob(['x'])

    downloadBlob(blob, 'take.mp3')

    expect(URL.createObjectURL).toHaveBeenCalledWith(blob)
    expect(click).toHaveBeenCalledOnce()
    expect(document.querySelector('a[download]')).toBeNull()
    expect(URL.revokeObjectURL).not.toHaveBeenCalled()
    vi.runAllTimers()
    expect(URL.revokeObjectURL).toHaveBeenCalledWith(
      vi.mocked(URL.createObjectURL).mock.results[0].value,
    )
  })
})
