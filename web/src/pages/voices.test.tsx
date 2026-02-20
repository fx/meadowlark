import { render, screen } from '@testing-library/preact'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { clearCache } from '@/hooks/use-fetch'
import { VoicesPage } from './voices'

const mockFetch = vi.fn()

beforeEach(() => {
  clearCache()
  mockFetch.mockClear()
  vi.stubGlobal('fetch', mockFetch)
})

afterEach(() => {
  vi.restoreAllMocks()
})

const voices = [
  {
    name: 'alloy (OpenAI, tts-1)',
    endpoint: 'OpenAI',
    model: 'tts-1',
    voice: 'alloy',
    is_alias: false,
  },
  {
    name: 'nova (OpenAI, tts-1)',
    endpoint: 'OpenAI',
    model: 'tts-1',
    voice: 'nova',
    is_alias: false,
  },
  { name: 'custom-voice', endpoint: 'Piper', model: 'en-us', voice: 'jenny', is_alias: true },
]

function mockVoicesResponse(data: unknown) {
  mockFetch.mockReturnValueOnce(
    Promise.resolve({
      ok: true,
      status: 200,
      json: () => Promise.resolve(data),
    }),
  )
}

function mockErrorResponse(status: number, message: string) {
  mockFetch.mockReturnValueOnce(
    Promise.resolve({
      ok: false,
      status,
      json: () => Promise.resolve({ error: { message } }),
    }),
  )
}

describe('VoicesPage', () => {
  it('shows loading state', () => {
    mockFetch.mockReturnValueOnce(new Promise(() => {}))
    render(<VoicesPage />)
    expect(screen.getByText('Loading voices...')).toBeInTheDocument()
  })

  it('renders voice table on success', async () => {
    mockVoicesResponse(voices)
    render(<VoicesPage />)

    expect(await screen.findByText('alloy (OpenAI, tts-1)')).toBeInTheDocument()
    expect(screen.getByText('nova (OpenAI, tts-1)')).toBeInTheDocument()
    expect(screen.getByText('custom-voice')).toBeInTheDocument()

    // Check table headers
    expect(screen.getByText('Voice Name')).toBeInTheDocument()
    expect(screen.getByText('Voice')).toBeInTheDocument()
    expect(screen.getByText('Endpoint')).toBeInTheDocument()
    expect(screen.getByText('Model')).toBeInTheDocument()
    expect(screen.getByText('Type')).toBeInTheDocument()
  })

  it('displays correct type badges', async () => {
    mockVoicesResponse(voices)
    render(<VoicesPage />)

    await screen.findByText('alloy (OpenAI, tts-1)')

    const canonicalBadges = screen.getAllByText('canonical')
    const aliasBadges = screen.getAllByText('alias')
    expect(canonicalBadges).toHaveLength(2)
    expect(aliasBadges).toHaveLength(1)
  })

  it('filters voices by search input', async () => {
    mockVoicesResponse(voices)
    const user = userEvent.setup()
    render(<VoicesPage />)

    await screen.findByText('alloy (OpenAI, tts-1)')

    const searchInput = screen.getByPlaceholderText('Search voices...')
    await user.type(searchInput, 'custom')

    expect(screen.getByText('custom-voice')).toBeInTheDocument()
    expect(screen.queryByText('alloy (OpenAI, tts-1)')).not.toBeInTheDocument()
    expect(screen.queryByText('nova (OpenAI, tts-1)')).not.toBeInTheDocument()
  })

  it('search is case-insensitive', async () => {
    mockVoicesResponse(voices)
    const user = userEvent.setup()
    render(<VoicesPage />)

    await screen.findByText('alloy (OpenAI, tts-1)')

    const searchInput = screen.getByPlaceholderText('Search voices...')
    await user.type(searchInput, 'ALLOY')

    expect(screen.getByText('alloy (OpenAI, tts-1)')).toBeInTheDocument()
    expect(screen.queryByText('custom-voice')).not.toBeInTheDocument()
  })

  it('shows empty state when no voices match search', async () => {
    mockVoicesResponse(voices)
    const user = userEvent.setup()
    render(<VoicesPage />)

    await screen.findByText('alloy (OpenAI, tts-1)')

    const searchInput = screen.getByPlaceholderText('Search voices...')
    await user.type(searchInput, 'nonexistent')

    expect(screen.getByText('No voices found')).toBeInTheDocument()
  })

  it('shows empty state when API returns empty array', async () => {
    mockVoicesResponse([])
    render(<VoicesPage />)

    expect(await screen.findByText('No voices found')).toBeInTheDocument()
  })

  it('shows error state on fetch failure', async () => {
    mockErrorResponse(500, 'internal error')
    render(<VoicesPage />)

    expect(await screen.findByText('Error: internal error')).toBeInTheDocument()
  })

  it('displays voice, endpoint and model columns', async () => {
    mockVoicesResponse(voices)
    render(<VoicesPage />)

    await screen.findByText('alloy (OpenAI, tts-1)')

    // Check voice column values
    const alloyCells = screen.getAllByText('alloy')
    expect(alloyCells.length).toBeGreaterThanOrEqual(1)
    expect(screen.getByText('jenny')).toBeInTheDocument()

    // Check endpoint column values
    const openaiCells = screen.getAllByText('OpenAI')
    expect(openaiCells.length).toBeGreaterThanOrEqual(2)
    expect(screen.getByText('Piper')).toBeInTheDocument()

    // Check model column values
    const tts1Cells = screen.getAllByText('tts-1')
    expect(tts1Cells.length).toBeGreaterThanOrEqual(2)
    expect(screen.getByText('en-us')).toBeInTheDocument()
  })
})
