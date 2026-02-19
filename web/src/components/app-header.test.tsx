import { render, screen } from '@testing-library/preact'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { ThemeProvider } from '@/components/theme-provider'
import { AppHeader } from './app-header'

// Mock wouter
const mockSetLocation = vi.fn()
let mockLocation = '/endpoints'

vi.mock('wouter', () => ({
  useLocation: () => [mockLocation, mockSetLocation],
}))

function renderHeader(location = '/endpoints', version?: string) {
  mockLocation = location
  mockSetLocation.mockClear()
  return render(
    <ThemeProvider>
      <AppHeader version={version} />
    </ThemeProvider>,
  )
}

describe('AppHeader', () => {
  it('renders the brand name', () => {
    renderHeader()
    expect(screen.getByText('Meadowlark')).toBeInTheDocument()
  })

  it('renders all navigation buttons', () => {
    renderHeader()
    expect(screen.getByRole('button', { name: 'Endpoints' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Voices' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Aliases' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Settings' })).toBeInTheDocument()
  })

  it('highlights the active route', () => {
    renderHeader('/voices')
    const voicesBtn = screen.getByRole('button', { name: 'Voices' })
    expect(voicesBtn).toHaveAttribute('aria-current', 'page')
    const endpointsBtn = screen.getByRole('button', { name: 'Endpoints' })
    expect(endpointsBtn).not.toHaveAttribute('aria-current')
  })

  it('navigates on nav button click', async () => {
    const user = userEvent.setup()
    renderHeader('/endpoints')
    await user.click(screen.getByRole('button', { name: 'Settings' }))
    expect(mockSetLocation).toHaveBeenCalledWith('/settings')
  })

  it('displays version string when provided', () => {
    renderHeader('/endpoints', 'v1.2.3')
    expect(screen.getByText('v1.2.3')).toBeInTheDocument()
  })

  it('does not display version when not provided', () => {
    renderHeader('/endpoints')
    expect(screen.queryByText(/^v/)).not.toBeInTheDocument()
  })

  it('renders the theme toggle', () => {
    renderHeader()
    expect(screen.getByRole('button', { name: 'Toggle theme' })).toBeInTheDocument()
  })

  it('renders mobile menu button', () => {
    renderHeader()
    expect(screen.getByRole('button', { name: 'Menu' })).toBeInTheDocument()
  })

  it('opens mobile menu on hamburger click', async () => {
    const user = userEvent.setup()
    renderHeader()
    await user.click(screen.getByRole('button', { name: 'Menu' }))
    expect(screen.getByText('Menu')).toBeInTheDocument()
  })
})
