import { render, screen, waitFor } from '@testing-library/preact'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { clearCache } from '@/hooks/use-fetch'
import { formatUptime, SettingsPage } from './settings'

const mockFetch = vi.fn()

beforeEach(() => {
  clearCache()
  mockFetch.mockClear()
  vi.stubGlobal('fetch', mockFetch)
})

afterEach(() => {
  vi.restoreAllMocks()
})

function jsonResponse(data: unknown) {
  return Promise.resolve({
    ok: true,
    status: 200,
    json: () => Promise.resolve(data),
  })
}

function errorResponse(status: number, message: string) {
  return Promise.resolve({
    ok: false,
    status,
    json: () => Promise.resolve({ error: { message } }),
  })
}

const mockStatus = {
  version: '1.2.3',
  uptime_seconds: 90061,
  wyoming_port: 10400,
  http_port: 8080,
  db_driver: 'sqlite',
  voice_count: 5,
  endpoint_count: 2,
  alias_count: 3,
}

describe('SettingsPage', () => {
  it('shows loading state', () => {
    mockFetch.mockReturnValueOnce(new Promise(() => {}))
    render(<SettingsPage />)
    expect(screen.getByText('Loading status...')).toBeInTheDocument()
  })

  it('renders all status fields', async () => {
    mockFetch.mockReturnValueOnce(jsonResponse(mockStatus))
    render(<SettingsPage />)

    await waitFor(() => {
      expect(screen.queryByText('Loading status...')).not.toBeInTheDocument()
    })

    expect(screen.getByText('Server Info')).toBeInTheDocument()
    expect(screen.getByText('1.2.3')).toBeInTheDocument()
    expect(screen.getByText('1d 1h 1m 1s')).toBeInTheDocument()

    expect(screen.getByText('Wyoming')).toBeInTheDocument()
    expect(screen.getByText('10400')).toBeInTheDocument()

    expect(screen.getByText('HTTP')).toBeInTheDocument()
    expect(screen.getByText('8080')).toBeInTheDocument()

    expect(screen.getByText('Database')).toBeInTheDocument()
    expect(screen.getByText('sqlite')).toBeInTheDocument()

    expect(screen.getByText('Voices')).toBeInTheDocument()
    expect(screen.getByText('5')).toBeInTheDocument()
    expect(screen.getByText('2')).toBeInTheDocument()
    expect(screen.getByText('3')).toBeInTheDocument()
  })

  it('shows error state', async () => {
    mockFetch.mockReturnValueOnce(errorResponse(500, 'server down'))
    render(<SettingsPage />)

    await waitFor(() => {
      expect(screen.queryByText('Loading status...')).not.toBeInTheDocument()
    })

    expect(screen.getByText('Failed to load status: server down')).toBeInTheDocument()
  })

  it('renders nothing when data is undefined after load', async () => {
    mockFetch.mockReturnValueOnce(
      Promise.resolve({
        ok: true,
        status: 200,
        json: () => Promise.resolve(null),
      }),
    )
    const { container } = render(<SettingsPage />)

    await waitFor(() => {
      expect(screen.queryByText('Loading status...')).not.toBeInTheDocument()
    })

    expect(container.innerHTML).toBe('')
  })
})

describe('formatUptime', () => {
  it('formats seconds only', () => {
    expect(formatUptime(45)).toBe('45s')
  })

  it('formats minutes and seconds', () => {
    expect(formatUptime(125)).toBe('2m 5s')
  })

  it('formats hours, minutes, seconds', () => {
    expect(formatUptime(3661)).toBe('1h 1m 1s')
  })

  it('formats days, hours, minutes, seconds', () => {
    expect(formatUptime(90061)).toBe('1d 1h 1m 1s')
  })

  it('formats zero seconds', () => {
    expect(formatUptime(0)).toBe('0s')
  })

  it('omits zero intermediate units', () => {
    expect(formatUptime(86401)).toBe('1d 1s')
  })
})
