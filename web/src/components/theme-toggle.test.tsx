import { cleanup, render, screen } from '@testing-library/preact'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ThemeProvider } from './theme-provider'
import { ThemeToggle } from './theme-toggle'

describe('ThemeToggle', () => {
  beforeEach(() => {
    vi.useFakeTimers({ shouldAdvanceTime: true })
    localStorage.clear()
    document.documentElement.classList.remove('light', 'dark')
    Object.defineProperty(window, 'matchMedia', {
      writable: true,
      value: vi.fn().mockImplementation((query: string) => ({
        matches: false,
        media: query,
        onchange: null,
        addListener: vi.fn(),
        removeListener: vi.fn(),
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
        dispatchEvent: vi.fn(),
      })),
    })
  })

  afterEach(() => {
    cleanup()
    vi.runOnlyPendingTimers()
    vi.useRealTimers()
    vi.restoreAllMocks()
  })

  it('renders toggle button', () => {
    render(
      <ThemeProvider>
        <ThemeToggle />
      </ThemeProvider>,
    )
    expect(screen.getByRole('button', { name: 'Toggle theme' })).toBeInTheDocument()
  })

  it('opens dropdown with theme options', async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
    render(
      <ThemeProvider>
        <ThemeToggle />
      </ThemeProvider>,
    )
    await user.click(screen.getByRole('button', { name: 'Toggle theme' }))
    expect(screen.getByText('Light')).toBeInTheDocument()
    expect(screen.getByText('Dark')).toBeInTheDocument()
    expect(screen.getByText('System')).toBeInTheDocument()
  })

  it('switches to dark theme', async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
    render(
      <ThemeProvider>
        <ThemeToggle />
      </ThemeProvider>,
    )
    await user.click(screen.getByRole('button', { name: 'Toggle theme' }))
    await user.click(screen.getByText('Dark'))
    expect(document.documentElement.classList.contains('dark')).toBe(true)
    expect(localStorage.getItem('meadowlark-theme')).toBe('dark')
  })

  it('switches to light theme', async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
    render(
      <ThemeProvider defaultTheme="dark">
        <ThemeToggle />
      </ThemeProvider>,
    )
    await user.click(screen.getByRole('button', { name: 'Toggle theme' }))
    await user.click(screen.getByText('Light'))
    expect(document.documentElement.classList.contains('light')).toBe(true)
    expect(localStorage.getItem('meadowlark-theme')).toBe('light')
  })

  it('switches to system theme', async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
    render(
      <ThemeProvider defaultTheme="dark">
        <ThemeToggle />
      </ThemeProvider>,
    )
    await user.click(screen.getByRole('button', { name: 'Toggle theme' }))
    await user.click(screen.getByText('System'))
    expect(localStorage.getItem('meadowlark-theme')).toBe('system')
  })
})
