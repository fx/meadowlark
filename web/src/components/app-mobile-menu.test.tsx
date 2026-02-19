import { cleanup, render, screen, waitFor } from '@testing-library/preact'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ThemeProvider } from '@/components/theme-provider'
import { AppMobileMenu } from './app-mobile-menu'

const mockSetLocation = vi.fn()
let mockLocation = '/endpoints'

vi.mock('wouter', () => ({
  useLocation: () => [mockLocation, mockSetLocation],
}))

function renderMenu(location = '/endpoints') {
  mockLocation = location
  mockSetLocation.mockClear()
  return render(
    <ThemeProvider>
      <AppMobileMenu />
    </ThemeProvider>,
  )
}

async function openMenu(user: ReturnType<typeof userEvent.setup>) {
  await user.click(screen.getByRole('button', { name: 'Menu' }))
}

describe('AppMobileMenu', () => {
  beforeEach(() => {
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
    vi.restoreAllMocks()
  })

  it('renders the trigger button', () => {
    renderMenu()
    expect(screen.getByRole('button', { name: 'Menu' })).toBeInTheDocument()
  })

  it('renders navigation links when opened', async () => {
    const user = userEvent.setup()
    renderMenu()
    await openMenu(user)
    expect(screen.getByRole('button', { name: 'Endpoints' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Voices' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Aliases' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Settings' })).toBeInTheDocument()
  })

  it('does not render navigation links when closed', () => {
    renderMenu()
    expect(screen.queryByRole('button', { name: 'Endpoints' })).not.toBeInTheDocument()
  })

  it('navigates and closes on item click', async () => {
    const user = userEvent.setup()
    renderMenu()
    await openMenu(user)
    await user.click(screen.getByRole('button', { name: 'Voices' }))
    expect(mockSetLocation).toHaveBeenCalledWith('/voices')
    await waitFor(() => {
      expect(screen.queryByRole('button', { name: 'Endpoints' })).not.toBeInTheDocument()
    })
  })

  it('renders the title when open', async () => {
    const user = userEvent.setup()
    renderMenu()
    await openMenu(user)
    expect(screen.getByText('Menu')).toBeInTheDocument()
  })

  it('renders theme buttons when open', async () => {
    const user = userEvent.setup()
    renderMenu()
    await openMenu(user)
    expect(screen.getByRole('button', { name: 'Light' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Dark' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'System' })).toBeInTheDocument()
  })

  it('switches to dark theme via theme button', async () => {
    const user = userEvent.setup()
    renderMenu()
    await openMenu(user)
    await user.click(screen.getByRole('button', { name: 'Dark' }))
    expect(document.documentElement.classList.contains('dark')).toBe(true)
    expect(localStorage.getItem('meadowlark-theme')).toBe('dark')
  })

  it('switches to light theme via theme button', async () => {
    const user = userEvent.setup()
    renderMenu()
    await openMenu(user)
    await user.click(screen.getByRole('button', { name: 'Light' }))
    expect(document.documentElement.classList.contains('light')).toBe(true)
    expect(localStorage.getItem('meadowlark-theme')).toBe('light')
  })

  it('switches to system theme via theme button', async () => {
    const user = userEvent.setup()
    renderMenu()
    await openMenu(user)
    await user.click(screen.getByRole('button', { name: 'System' }))
    expect(localStorage.getItem('meadowlark-theme')).toBe('system')
  })
})
