import '@testing-library/jest-dom'
import { afterEach, beforeAll, beforeEach, vi } from 'vitest'

// Mock ResizeObserver for Radix UI components (Select, Switch use-size)
globalThis.ResizeObserver = class ResizeObserver {
  observe() {}
  unobserve() {}
  disconnect() {}
}

// Radix UI's floating-ui and focus-scope rely on DOM APIs unavailable in jsdom
// Suppress unhandled errors/rejections from these async cleanup operations
beforeAll(() => {
  // Mock matchMedia for ThemeProvider and other components
  Object.defineProperty(window, 'matchMedia', {
    writable: true,
    value: (query: string) => ({
      matches: false,
      media: query,
      onchange: null,
      addListener: () => {},
      removeListener: () => {},
      addEventListener: () => {},
      removeEventListener: () => {},
      dispatchEvent: () => false,
    }),
  })

  // Mock scrollIntoView
  Element.prototype.scrollIntoView = () => {}

  // Mock pointer capture methods
  Element.prototype.hasPointerCapture = () => false
  Element.prototype.setPointerCapture = () => {}
  Element.prototype.releasePointerCapture = () => {}

  // Catch unhandled rejections from floating-ui positioning
  const handler = (event: PromiseRejectionEvent) => {
    const msg = String(event.reason?.message || event.reason || '')
    if (
      msg.includes('getBoundingClientRect') ||
      msg.includes('focus is not a function') ||
      msg.includes('not of type')
    ) {
      event.preventDefault()
    }
  }
  window.addEventListener('unhandledrejection', handler)
})

// jsdom has no object URLs, media playback, or animation frames. Fresh mocks
// per test keep call history from leaking between tests.
let objectUrlCounter = 0
beforeEach(() => {
  URL.createObjectURL = vi.fn(() => `blob:mock/${++objectUrlCounter}`)
  URL.revokeObjectURL = vi.fn()
  HTMLMediaElement.prototype.play = vi.fn(() => Promise.resolve())
  HTMLMediaElement.prototype.pause = vi.fn()
  HTMLMediaElement.prototype.load = vi.fn()
  globalThis.requestAnimationFrame = vi.fn((cb: FrameRequestCallback) =>
    Number(setTimeout(() => cb(performance.now()), 16)),
  )
  globalThis.cancelAnimationFrame = vi.fn((id: number) => clearTimeout(id))
})

afterEach(() => {
  document.documentElement.classList.remove('light', 'dark')
})
