import '@testing-library/jest-dom'
import { afterEach, beforeAll } from 'vitest'

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

afterEach(() => {
  document.documentElement.classList.remove('light', 'dark')
})
