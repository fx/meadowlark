import { render, screen } from '@testing-library/preact'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ThemeProvider, useTheme } from './theme-provider'

function TestConsumer() {
  const { theme, setTheme } = useTheme()
  return (
    <div>
      <span data-testid="theme">{theme}</span>
      <button type="button" onClick={() => setTheme('dark')}>
        Set Dark
      </button>
      <button type="button" onClick={() => setTheme('light')}>
        Set Light
      </button>
      <button type="button" onClick={() => setTheme('system')}>
        Set System
      </button>
    </div>
  )
}

describe('ThemeProvider', () => {
  let matchMediaListeners: Array<() => void>

  beforeEach(() => {
    localStorage.clear()
    document.documentElement.classList.remove('light', 'dark')
    matchMediaListeners = []
    Object.defineProperty(window, 'matchMedia', {
      writable: true,
      value: vi.fn().mockImplementation((query: string) => ({
        matches: false,
        media: query,
        onchange: null,
        addListener: vi.fn(),
        removeListener: vi.fn(),
        addEventListener: (_: string, cb: () => void) => {
          matchMediaListeners.push(cb)
        },
        removeEventListener: vi.fn(),
        dispatchEvent: vi.fn(),
      })),
    })
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('defaults to system theme', () => {
    render(
      <ThemeProvider>
        <TestConsumer />
      </ThemeProvider>,
    )
    expect(screen.getByTestId('theme').textContent).toBe('system')
    expect(document.documentElement.classList.contains('light')).toBe(true)
  })

  it('reads stored theme from localStorage', () => {
    localStorage.setItem('meadowlark-theme', 'dark')
    render(
      <ThemeProvider>
        <TestConsumer />
      </ThemeProvider>,
    )
    expect(screen.getByTestId('theme').textContent).toBe('dark')
    expect(document.documentElement.classList.contains('dark')).toBe(true)
  })

  it('switches theme and persists to localStorage', async () => {
    const user = userEvent.setup()
    render(
      <ThemeProvider>
        <TestConsumer />
      </ThemeProvider>,
    )
    await user.click(screen.getByText('Set Dark'))
    expect(screen.getByTestId('theme').textContent).toBe('dark')
    expect(document.documentElement.classList.contains('dark')).toBe(true)
    expect(localStorage.getItem('meadowlark-theme')).toBe('dark')
  })

  it('switches from dark to light', async () => {
    const user = userEvent.setup()
    localStorage.setItem('meadowlark-theme', 'dark')
    render(
      <ThemeProvider>
        <TestConsumer />
      </ThemeProvider>,
    )
    await user.click(screen.getByText('Set Light'))
    expect(screen.getByTestId('theme').textContent).toBe('light')
    expect(document.documentElement.classList.contains('light')).toBe(true)
    expect(document.documentElement.classList.contains('dark')).toBe(false)
  })

  it('respects system dark preference', () => {
    Object.defineProperty(window, 'matchMedia', {
      writable: true,
      value: vi.fn().mockImplementation((query: string) => ({
        matches: query === '(prefers-color-scheme: dark)',
        media: query,
        onchange: null,
        addListener: vi.fn(),
        removeListener: vi.fn(),
        addEventListener: (_: string, cb: () => void) => {
          matchMediaListeners.push(cb)
        },
        removeEventListener: vi.fn(),
        dispatchEvent: vi.fn(),
      })),
    })
    render(
      <ThemeProvider>
        <TestConsumer />
      </ThemeProvider>,
    )
    expect(document.documentElement.classList.contains('dark')).toBe(true)
  })

  it('updates when system preference changes while on system theme', async () => {
    const listeners: Array<() => void> = []
    Object.defineProperty(window, 'matchMedia', {
      writable: true,
      value: vi.fn().mockImplementation((query: string) => ({
        matches: false,
        media: query,
        onchange: null,
        addListener: vi.fn(),
        removeListener: vi.fn(),
        addEventListener: (_: string, cb: () => void) => {
          listeners.push(cb)
        },
        removeEventListener: vi.fn(),
        dispatchEvent: vi.fn(),
      })),
    })
    render(
      <ThemeProvider>
        <TestConsumer />
      </ThemeProvider>,
    )
    expect(document.documentElement.classList.contains('light')).toBe(true)
    // Simulate system theme change
    Object.defineProperty(window, 'matchMedia', {
      writable: true,
      value: vi.fn().mockImplementation((query: string) => ({
        matches: query === '(prefers-color-scheme: dark)',
        media: query,
        onchange: null,
        addListener: vi.fn(),
        removeListener: vi.fn(),
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
        dispatchEvent: vi.fn(),
      })),
    })
    for (const listener of listeners) listener()
    expect(document.documentElement.classList.contains('dark')).toBe(true)
  })

  it('uses custom storageKey', async () => {
    const user = userEvent.setup()
    render(
      <ThemeProvider storageKey="custom-key">
        <TestConsumer />
      </ThemeProvider>,
    )
    await user.click(screen.getByText('Set Dark'))
    expect(localStorage.getItem('custom-key')).toBe('dark')
  })

  it('uses custom defaultTheme', () => {
    render(
      <ThemeProvider defaultTheme="dark">
        <TestConsumer />
      </ThemeProvider>,
    )
    expect(screen.getByTestId('theme').textContent).toBe('dark')
  })

  it('provides default noop setTheme when used outside provider', () => {
    function Bare() {
      const { theme, setTheme } = useTheme()
      // Calling setTheme outside provider should not throw
      setTheme('dark')
      return <span data-testid="bare">{theme}</span>
    }
    render(<Bare />)
    expect(screen.getByTestId('bare').textContent).toBe('system')
  })

  describe('dark mode CSS selector', () => {
    it('demonstrates .dark * does not match the html element itself', () => {
      document.documentElement.classList.add('dark')

      // Bug: `.dark *` only matches descendants of .dark, not .dark itself
      expect(document.documentElement.matches('.dark *')).toBe(false)
      // Fix: `:where(.dark, .dark *)` matches both .dark and its descendants
      expect(document.documentElement.matches(':where(.dark, .dark *)')).toBe(true)
    })

    it('matches child elements with both selectors', () => {
      document.documentElement.classList.add('dark')
      const child = document.createElement('div')
      document.documentElement.appendChild(child)

      try {
        // Both selectors match descendants
        expect(child.matches('.dark *')).toBe(true)
        expect(child.matches(':where(.dark, .dark *)')).toBe(true)
      } finally {
        child.remove()
      }
    })
  })

  it('does not listen for system changes when not on system theme', async () => {
    const user = userEvent.setup()
    const removeListener = vi.fn()
    Object.defineProperty(window, 'matchMedia', {
      writable: true,
      value: vi.fn().mockImplementation((query: string) => ({
        matches: false,
        media: query,
        onchange: null,
        addListener: vi.fn(),
        removeListener: vi.fn(),
        addEventListener: vi.fn(),
        removeEventListener: removeListener,
        dispatchEvent: vi.fn(),
      })),
    })
    render(
      <ThemeProvider>
        <TestConsumer />
      </ThemeProvider>,
    )
    // Switch away from system theme
    await user.click(screen.getByText('Set Dark'))
    // The cleanup should have been called
    expect(removeListener).toHaveBeenCalled()
  })
})
