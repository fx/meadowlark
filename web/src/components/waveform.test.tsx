import { act, fireEvent, render, screen } from '@testing-library/preact'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { Peaks } from '@/lib/peaks'
import { flatPeaks } from '@/test-audio'
import { formatTime, Waveform } from './waveform'

let audio: HTMLAudioElement
let currentTime: number
let paused: boolean
let frames: Map<number, FrameRequestCallback>
let nextFrame: number

function flushFrame() {
  const pending = [...frames.values()]
  frames.clear()
  act(() => {
    for (const cb of pending) cb(0)
  })
}

function emit(type: string) {
  act(() => {
    audio.dispatchEvent(new Event(type))
  })
}

beforeEach(() => {
  audio = document.createElement('audio')
  currentTime = 0
  paused = true
  Object.defineProperty(audio, 'currentTime', {
    configurable: true,
    get: () => currentTime,
    set: (v: number) => {
      currentTime = v
    },
  })
  Object.defineProperty(audio, 'paused', { configurable: true, get: () => paused })
  // `new Audio()` yields the element under test.
  vi.stubGlobal(
    'Audio',
    vi.fn(function Audio() {
      return audio
    }),
  )
  frames = new Map()
  nextFrame = 0
  vi.stubGlobal(
    'requestAnimationFrame',
    vi.fn((cb: FrameRequestCallback) => {
      nextFrame++
      frames.set(nextFrame, cb)
      return nextFrame
    }),
  )
  vi.stubGlobal(
    'cancelAnimationFrame',
    vi.fn((id: number) => {
      frames.delete(id)
    }),
  )
})

afterEach(() => {
  vi.unstubAllGlobals()
})

function peaksOf(peak: number[], rms: number[]): Peaks {
  return { peak: Float32Array.from(peak), rms: Float32Array.from(rms) }
}

function renderPlayer(props: Partial<Parameters<typeof Waveform>[0]> = {}) {
  return render(<Waveform peaks={flatPeaks(200)} duration={4} src="blob:take/1" {...props} />)
}

const seekInput = () => screen.getByRole('slider', { name: 'Seek' }) as HTMLInputElement
const clipWidth = (container: ParentNode) =>
  Number(container.querySelector('[data-testid="played-clip"]')?.getAttribute('width'))

describe('formatTime', () => {
  it('formats m:ss.s', () => {
    expect(formatTime(0)).toBe('0:00.0')
    expect(formatTime(1.24)).toBe('0:01.2')
    expect(formatTime(65.04)).toBe('1:05.0')
    expect(formatTime(59.96)).toBe('1:00.0')
    expect(formatTime(-3)).toBe('0:00.0')
  })
})

describe('Waveform bars', () => {
  it('draws one bar per bucket in each layer, played and unplayed', () => {
    const { container } = renderPlayer()
    expect(container.querySelectorAll('g[data-layer="peak"] rect')).toHaveLength(200)
    expect(container.querySelectorAll('g[data-layer="rms"] rect')).toHaveLength(200)
    // Unplayed peak + RMS, then played peak + RMS.
    expect(container.querySelectorAll('svg rect:not([data-testid])')).toHaveLength(800)
    const svg = container.querySelector('svg[preserveAspectRatio="none"]')
    expect(svg).toHaveAttribute('viewBox', '0 0 200 100')
    expect(svg).toHaveAttribute('aria-hidden', 'true')
  })

  it('normalises to the loudest bucket and mirrors bars around the centre', () => {
    const { container } = render(
      <Waveform compact peaks={peaksOf([0.25, 0.5], [0.1, 0.25])} duration={1} src="x" />,
    )
    const peak = container.querySelectorAll('g[data-layer="peak"] rect')
    expect(peak[1]).toHaveAttribute('height', '100')
    expect(peak[1]).toHaveAttribute('y', '0')
    expect(peak[0]).toHaveAttribute('height', '50')
    expect(peak[0]).toHaveAttribute('y', '25')
    const rms = container.querySelectorAll('g[data-layer="rms"] rect')
    expect(rms[1]).toHaveAttribute('height', '50')
  })

  it('draws silence as a thin baseline', () => {
    const { container } = render(
      <Waveform compact peaks={peaksOf([0, 0], [0, 0])} duration={1} src="x" />,
    )
    for (const rect of container.querySelectorAll('g[data-layer] rect')) {
      expect(rect).toHaveAttribute('height', '1.5')
    }
  })

  it('compact mode has no controls and colours an active take as played', () => {
    const { container, rerender } = render(
      <Waveform compact peaks={flatPeaks(64)} duration={2} src="x" />,
    )
    expect(screen.queryByRole('slider')).toBeNull()
    expect(screen.queryByRole('button')).toBeNull()
    expect(clipWidth(container)).toBe(0)
    rerender(<Waveform compact active peaks={flatPeaks(64)} duration={2} src="x" />)
    expect(clipWidth(container)).toBe(64)
  })
})

describe('Waveform player', () => {
  it('loads the source and shows times', () => {
    renderPlayer()
    expect(audio.src).toBe('blob:take/1')
    expect(audio.preload).toBe('auto')
    expect(screen.getByTestId('current-time')).toHaveTextContent('0:00.0')
    expect(screen.getByTestId('total-time')).toHaveTextContent('0:04.0')
    expect(seekInput()).toHaveAttribute('aria-valuetext', '0:00.0 of 0:04.0')
  })

  it('plays and pauses from the button', () => {
    renderPlayer()
    fireEvent.click(screen.getByRole('button', { name: 'Play' }))
    expect(audio.play).toHaveBeenCalledOnce()

    paused = false
    emit('play')
    fireEvent.click(screen.getByRole('button', { name: 'Pause' }))
    expect(audio.pause).toHaveBeenCalledOnce()
  })

  it('sweeps the playhead every animation frame while playing', () => {
    const { container } = renderPlayer()
    emit('play')
    expect(requestAnimationFrame).toHaveBeenCalledOnce()

    currentTime = 1
    flushFrame()
    expect(clipWidth(container)).toBe(50)
    expect(screen.getByTestId('playhead').style.left).toBe('25%')
    expect(screen.getByTestId('current-time')).toHaveTextContent('0:01.0')

    currentTime = 3
    flushFrame()
    expect(clipWidth(container)).toBe(150)
    expect(seekInput()).toHaveAttribute('aria-valuetext', '0:03.0 of 0:04.0')
  })

  it('stops the frame loop on pause, syncing the final position', () => {
    const { container } = renderPlayer()
    emit('play')
    currentTime = 2.5
    emit('pause')
    expect(cancelAnimationFrame).toHaveBeenCalled()
    expect(frames.size).toBe(0)
    expect(clipWidth(container)).toBe(125)
    expect(screen.getByRole('button', { name: 'Play' })).toBeInTheDocument()
  })

  it('stops on ended', () => {
    const { container } = renderPlayer()
    emit('play')
    currentTime = 4
    emit('ended')
    expect(frames.size).toBe(0)
    expect(clipWidth(container)).toBe(200)
  })

  it('stops the frame loop and playback on unmount', () => {
    const { unmount } = renderPlayer()
    emit('play')
    unmount()
    expect(frames.size).toBe(0)
    expect(audio.pause).toHaveBeenCalled()
  })

  it('releases its media resource on unmount', () => {
    const { unmount } = renderPlayer()
    expect(audio.getAttribute('src')).toBe('blob:take/1')
    unmount()
    expect(audio.hasAttribute('src')).toBe(false)
    expect(audio.load).toHaveBeenCalledOnce()
  })

  it('releases the old source before loading a new one', () => {
    const { rerender } = renderPlayer()
    rerender(<Waveform peaks={flatPeaks(200)} duration={4} src="blob:take/2" />)
    expect(audio.load).toHaveBeenCalledOnce()
    expect(audio.getAttribute('src')).toBe('blob:take/2')
  })

  it('seeks from the range input', () => {
    const { container } = renderPlayer()
    fireEvent.input(seekInput(), { target: { value: '2' } })
    expect(currentTime).toBe(2)
    expect(clipWidth(container)).toBe(100)
  })

  it('syncs on seeked', () => {
    const { container } = renderPlayer()
    currentTime = 1
    emit('seeked')
    expect(clipWidth(container)).toBe(50)
  })

  it('toggles playback with Space', () => {
    renderPlayer()
    fireEvent.keyDown(seekInput(), { key: ' ' })
    expect(audio.play).toHaveBeenCalledOnce()
  })

  it('seeks by 5% with the arrow keys, clamped to the ends', () => {
    renderPlayer({ duration: 10 })
    const input = seekInput()
    fireEvent.keyDown(input, { key: 'ArrowRight' })
    expect(currentTime).toBe(0.5)
    fireEvent.keyDown(input, { key: 'ArrowUp' })
    expect(currentTime).toBe(1)
    fireEvent.keyDown(input, { key: 'ArrowLeft' })
    expect(currentTime).toBe(0.5)
    fireEvent.keyDown(input, { key: 'ArrowDown' })
    fireEvent.keyDown(input, { key: 'ArrowDown' })
    expect(currentTime).toBe(0)
    fireEvent.input(input, { target: { value: '9.8' } })
    fireEvent.keyDown(input, { key: 'ArrowRight' })
    expect(currentTime).toBe(10)
  })

  it('uses a 100 ms minimum arrow step and ignores other keys', () => {
    renderPlayer({ duration: 1 })
    fireEvent.keyDown(seekInput(), { key: 'ArrowRight' })
    expect(currentTime).toBeCloseTo(0.1)
    fireEvent.keyDown(seekInput(), { key: 'a' })
    expect(currentTime).toBeCloseTo(0.1)
    expect(audio.play).not.toHaveBeenCalled()
  })

  it('shows a hover cursor with its time, and hides it on leave', () => {
    renderPlayer()
    const input = seekInput()
    input.getBoundingClientRect = () => ({ left: 100, width: 400 }) as DOMRect
    fireEvent.pointerMove(input, { clientX: 200 })
    expect(screen.getByRole('tooltip')).toHaveTextContent('0:01.0')
    expect(screen.getByTestId('hover-line').style.left).toBe('25%')

    fireEvent.pointerMove(input, { clientX: 900 })
    expect(screen.getByRole('tooltip')).toHaveTextContent('0:04.0')
    fireEvent.pointerMove(input, { clientX: 0 })
    expect(screen.getByRole('tooltip')).toHaveTextContent('0:00.0')

    fireEvent.pointerLeave(input)
    expect(screen.queryByRole('tooltip')).toBeNull()
    expect(screen.queryByTestId('hover-line')).toBeNull()
  })

  it('ignores hover before layout', () => {
    renderPlayer()
    fireEvent.pointerMove(seekInput(), { clientX: 50 })
    expect(screen.queryByRole('tooltip')).toBeNull()
  })

  it('keeps progress at zero for a zero-length take', () => {
    const { container } = renderPlayer({ duration: 0 })
    currentTime = 1
    emit('seeked')
    expect(clipWidth(container)).toBe(0)
  })

  it('auto-plays on mount when asked, ignoring a rejected play', async () => {
    vi.mocked(HTMLMediaElement.prototype.play).mockRejectedValueOnce(new Error('NotAllowedError'))
    renderPlayer({ autoPlay: true })
    expect(audio.play).toHaveBeenCalledOnce()
    await Promise.resolve()
    expect(screen.getByRole('button', { name: 'Play' })).toBeInTheDocument()
  })

  it('does not auto-play by default', () => {
    renderPlayer()
    expect(audio.play).not.toHaveBeenCalled()
  })
})
