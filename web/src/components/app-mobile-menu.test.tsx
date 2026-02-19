import { render, screen } from '@testing-library/preact'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { ThemeProvider } from '@/components/theme-provider'
import { AppMobileMenu } from './app-mobile-menu'

const mockSetLocation = vi.fn()

vi.mock('wouter', () => ({
  useLocation: () => ['/endpoints', mockSetLocation],
}))

function renderMenu(open = true, currentPath = '/endpoints') {
  const onOpenChange = vi.fn()
  mockSetLocation.mockClear()
  const result = render(
    <ThemeProvider>
      <AppMobileMenu open={open} onOpenChange={onOpenChange} currentPath={currentPath} />
    </ThemeProvider>,
  )
  return { ...result, onOpenChange }
}

describe('AppMobileMenu', () => {
  it('renders navigation links when open', () => {
    renderMenu(true)
    expect(screen.getByRole('button', { name: 'Endpoints' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Voices' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Aliases' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Settings' })).toBeInTheDocument()
  })

  it('does not render navigation links when closed', () => {
    renderMenu(false)
    expect(screen.queryByRole('navigation', { name: 'Mobile navigation' })).not.toBeInTheDocument()
  })

  it('highlights the current path', () => {
    renderMenu(true, '/aliases')
    const aliasesBtn = screen.getByRole('button', { name: 'Aliases' })
    expect(aliasesBtn).toHaveAttribute('aria-current', 'page')
    const endpointsBtn = screen.getByRole('button', { name: 'Endpoints' })
    expect(endpointsBtn).not.toHaveAttribute('aria-current')
  })

  it('navigates and closes on item click', async () => {
    const user = userEvent.setup()
    const { onOpenChange } = renderMenu(true, '/endpoints')
    await user.click(screen.getByRole('button', { name: 'Voices' }))
    expect(mockSetLocation).toHaveBeenCalledWith('/voices')
    expect(onOpenChange).toHaveBeenCalledWith(false)
  })

  it('renders the title', () => {
    renderMenu(true)
    expect(screen.getByText('Meadowlark')).toBeInTheDocument()
  })
})
