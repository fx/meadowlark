import { render, screen } from '@testing-library/preact'
import { describe, expect, it, vi } from 'vitest'

// Mock wouter to control routing
let mockLocation = '/'
const mockSetLocation = vi.fn()

vi.mock('wouter', () => ({
  useLocation: () => [mockLocation, mockSetLocation],
  Route: ({
    path,
    component: Comp,
    children,
  }: {
    path: string
    component?: () => preact.VNode
    children?: preact.ComponentChildren
  }) => {
    if (mockLocation === path) {
      return Comp ? <Comp /> : children
    }
    return null
  },
  Switch: ({ children }: { children: preact.ComponentChildren }) => children,
  Redirect: ({ to }: { to: string }) => {
    mockLocation = to
    return null
  },
}))

import { App } from './app'

describe('App', () => {
  it('renders Meadowlark heading in header', () => {
    mockLocation = '/endpoints'
    render(<App />)
    expect(screen.getByText('Meadowlark')).toBeInTheDocument()
  })

  it('renders the endpoints page at /endpoints', () => {
    mockLocation = '/endpoints'
    render(<App />)
    expect(screen.getByText('Endpoints')).toBeInTheDocument()
  })

  it('renders the voices page at /voices', () => {
    mockLocation = '/voices'
    render(<App />)
    expect(screen.getByText('Loading voices...')).toBeInTheDocument()
  })

  it('renders the aliases page at /aliases', () => {
    mockLocation = '/aliases'
    render(<App />)
    expect(screen.getByText('Aliases')).toBeInTheDocument()
  })

  it('renders the settings page at /settings', () => {
    mockLocation = '/settings'
    render(<App />)
    expect(screen.getByText('Settings')).toBeInTheDocument()
  })

  it('redirects / to /endpoints', () => {
    mockLocation = '/'
    render(<App />)
    expect(mockLocation).toBe('/endpoints')
  })
})
