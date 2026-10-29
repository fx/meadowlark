import { render, screen } from '@testing-library/preact'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { clearCache } from '@/hooks/use-fetch'
import { App } from './app'

// Real wouter over the jsdom location. Fetches never settle, so every page
// stays in its loading state.
function visit(path: string) {
  window.history.replaceState(null, '', path)
  return render(<App />)
}

beforeEach(() => {
  clearCache()
  vi.stubGlobal(
    'fetch',
    vi.fn(() => new Promise(() => {})),
  )
})

afterEach(() => {
  vi.unstubAllGlobals()
  clearCache()
})

describe('App', () => {
  it('renders Meadowlark heading in header', () => {
    visit('/endpoints')
    expect(screen.getByText('Meadowlark')).toBeInTheDocument()
  })

  it('renders the endpoints page at /endpoints', () => {
    visit('/endpoints')
    expect(screen.getByText('Loading endpoints...')).toBeInTheDocument()
  })

  it('renders the voices page at /voices', () => {
    visit('/voices')
    expect(screen.getByText('Loading voices...')).toBeInTheDocument()
  })

  it('renders the settings page at /settings', () => {
    visit('/settings')
    expect(screen.getByRole('heading', { name: 'Settings' })).toBeInTheDocument()
  })

  it('renders the playground at /playground and /playground/:aliasId', () => {
    const { unmount } = visit('/playground')
    expect(screen.getByText('Loading playground...')).toBeInTheDocument()
    unmount()
    visit('/playground/alias-1')
    expect(screen.getByText('Loading playground...')).toBeInTheDocument()
    expect(fetch).toHaveBeenCalledWith('/api/v1/aliases/alias-1', expect.anything())
  })

  it('redirects / to /playground', () => {
    visit('/')
    expect(window.location.pathname).toBe('/playground')
    expect(screen.getByText('Loading playground...')).toBeInTheDocument()
  })

  it('redirects /aliases to /voices', () => {
    visit('/aliases')
    expect(window.location.pathname).toBe('/voices')
    expect(screen.getByText('Loading voices...')).toBeInTheDocument()
  })
})
