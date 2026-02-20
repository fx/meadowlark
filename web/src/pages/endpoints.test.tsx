import { render, screen, waitFor } from '@testing-library/preact'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { clearCache } from '@/hooks/use-fetch'
import type { Endpoint } from '@/lib/api'
import { EndpointsPage } from './endpoints'

const mockEndpoints: Endpoint[] = [
  {
    id: 'ep-1',
    name: 'OpenAI',
    base_url: 'https://api.openai.com/v1',
    api_key: 'sk-123',
    models: ['tts-1', 'tts-1-hd'],
    default_speed: 1.0,
    default_instructions: '',
    default_response_format: 'wav',
    enabled: true,
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
  },
  {
    id: 'ep-2',
    name: 'Local TTS',
    base_url: 'http://localhost:8080',
    models: ['piper'],
    default_response_format: 'wav',
    enabled: false,
    created_at: '2024-01-02T00:00:00Z',
    updated_at: '2024-01-02T00:00:00Z',
  },
]

function mockFetchWith(data: unknown) {
  return vi.fn().mockResolvedValue({
    ok: true,
    status: 200,
    json: () => Promise.resolve(data),
  })
}

beforeEach(() => {
  clearCache()
  vi.stubGlobal('fetch', mockFetchWith(mockEndpoints))
})

afterEach(() => {
  vi.restoreAllMocks()
})

describe('EndpointsPage', () => {
  it('renders list of endpoints', async () => {
    render(<EndpointsPage />)
    await waitFor(() => {
      expect(screen.getByText('OpenAI')).toBeInTheDocument()
    })
    expect(screen.getByText('Local TTS')).toBeInTheDocument()
  })

  it('shows model count badges', async () => {
    render(<EndpointsPage />)
    await waitFor(() => {
      expect(screen.getByText('2 models')).toBeInTheDocument()
    })
    expect(screen.getByText('1 model')).toBeInTheDocument()
  })

  it('shows empty state when no endpoints', async () => {
    vi.stubGlobal('fetch', mockFetchWith([]))
    render(<EndpointsPage />)
    await waitFor(() => {
      expect(screen.getByText(/No endpoints configured/)).toBeInTheDocument()
    })
  })

  it('opens create form when Add Endpoint is clicked', async () => {
    const user = userEvent.setup()
    render(<EndpointsPage />)
    await waitFor(() => {
      expect(screen.getByText('OpenAI')).toBeInTheDocument()
    })
    await user.click(screen.getByText('+ Add Endpoint'))
    expect(screen.getByLabelText('Name')).toBeInTheDocument()
    expect(screen.getByLabelText('Base URL')).toBeInTheDocument()
  })

  it('creates an endpoint', async () => {
    const user = userEvent.setup()
    const fetchMock = mockFetchWith(mockEndpoints)
    vi.stubGlobal('fetch', fetchMock)
    render(<EndpointsPage />)
    await waitFor(() => {
      expect(screen.getByText('OpenAI')).toBeInTheDocument()
    })
    await user.click(screen.getByText('+ Add Endpoint'))
    await user.type(screen.getByLabelText('Name'), 'New EP')
    await user.type(screen.getByLabelText('Base URL'), 'https://new.api.com')
    await user.type(screen.getByLabelText('Models'), 'model-1')
    await user.click(screen.getByText('Create'))
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        '/api/v1/endpoints',
        expect.objectContaining({ method: 'POST' }),
      )
    })
  })

  it('expands row to show edit form', async () => {
    const user = userEvent.setup()
    render(<EndpointsPage />)
    await waitFor(() => {
      expect(screen.getByText('OpenAI')).toBeInTheDocument()
    })
    await user.click(screen.getByText('OpenAI'))
    expect(screen.getByLabelText('Name')).toHaveValue('OpenAI')
    expect(screen.getByText('Update')).toBeInTheDocument()
  })

  it('updates an endpoint', async () => {
    const user = userEvent.setup()
    const fetchMock = mockFetchWith(mockEndpoints)
    vi.stubGlobal('fetch', fetchMock)
    render(<EndpointsPage />)
    await waitFor(() => {
      expect(screen.getByText('OpenAI')).toBeInTheDocument()
    })
    await user.click(screen.getByText('OpenAI'))
    await user.clear(screen.getByLabelText('Name'))
    await user.type(screen.getByLabelText('Name'), 'Updated')
    await user.click(screen.getByText('Update'))
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        '/api/v1/endpoints/ep-1',
        expect.objectContaining({ method: 'PUT' }),
      )
    })
  })

  it('shows delete confirmation dialog', async () => {
    const user = userEvent.setup()
    render(<EndpointsPage />)
    await waitFor(() => {
      expect(screen.getByText('OpenAI')).toBeInTheDocument()
    })
    await user.click(screen.getByText('OpenAI'))
    await user.click(screen.getByRole('button', { name: 'Delete OpenAI' }))
    expect(screen.getByText('Delete Endpoint')).toBeInTheDocument()
    expect(screen.getByText(/Are you sure you want to delete "OpenAI"/)).toBeInTheDocument()
  })

  it('deletes an endpoint after confirmation', async () => {
    const user = userEvent.setup()
    const fetchMock = mockFetchWith(mockEndpoints)
    vi.stubGlobal('fetch', fetchMock)
    render(<EndpointsPage />)
    await waitFor(() => {
      expect(screen.getByText('OpenAI')).toBeInTheDocument()
    })
    await user.click(screen.getByText('OpenAI'))
    await user.click(screen.getByRole('button', { name: 'Delete OpenAI' }))
    await user.click(screen.getByRole('button', { name: 'Delete' }))
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        '/api/v1/endpoints/ep-1',
        expect.objectContaining({ method: 'DELETE' }),
      )
    })
  })

  it('cancels delete dialog', async () => {
    const user = userEvent.setup()
    render(<EndpointsPage />)
    await waitFor(() => {
      expect(screen.getByText('OpenAI')).toBeInTheDocument()
    })
    await user.click(screen.getByText('OpenAI'))
    await user.click(screen.getByRole('button', { name: 'Delete OpenAI' }))
    expect(screen.getByText('Delete Endpoint')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Cancel' }))
    await waitFor(() => {
      expect(screen.queryByText('Delete Endpoint')).not.toBeInTheDocument()
    })
  })

  it('toggles endpoint enabled switch', async () => {
    const user = userEvent.setup()
    const fetchMock = mockFetchWith(mockEndpoints)
    vi.stubGlobal('fetch', fetchMock)
    render(<EndpointsPage />)
    await waitFor(() => {
      expect(screen.getByText('OpenAI')).toBeInTheDocument()
    })
    const enabledSwitch = screen.getByRole('switch', { name: 'OpenAI enabled' })
    await user.click(enabledSwitch)
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        '/api/v1/endpoints/ep-1',
        expect.objectContaining({
          method: 'PUT',
          body: JSON.stringify({ enabled: false }),
        }),
      )
    })
  })

  it('shows error state when fetch fails', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: false,
        status: 500,
        json: () => Promise.resolve({ error: { message: 'Server error' } }),
      }),
    )
    render(<EndpointsPage />)
    await waitFor(() => {
      expect(screen.getByText(/Error:/)).toBeInTheDocument()
    })
  })

  it('toggles create form closed via Add Endpoint button', async () => {
    const user = userEvent.setup()
    render(<EndpointsPage />)
    await waitFor(() => {
      expect(screen.getByText('OpenAI')).toBeInTheDocument()
    })
    await user.click(screen.getByText('+ Add Endpoint'))
    expect(screen.getByLabelText('Name')).toBeInTheDocument()
    await user.click(screen.getByText('+ Add Endpoint'))
    expect(screen.queryByLabelText('Name')).not.toBeInTheDocument()
  })

  it('cancels create form', async () => {
    const user = userEvent.setup()
    render(<EndpointsPage />)
    await waitFor(() => {
      expect(screen.getByText('OpenAI')).toBeInTheDocument()
    })
    await user.click(screen.getByText('+ Add Endpoint'))
    expect(screen.getByLabelText('Name')).toBeInTheDocument()
    await user.click(screen.getByText('Cancel'))
    expect(screen.queryByLabelText('Name')).not.toBeInTheDocument()
  })

  it('cancels edit form', async () => {
    const user = userEvent.setup()
    render(<EndpointsPage />)
    await waitFor(() => {
      expect(screen.getByText('OpenAI')).toBeInTheDocument()
    })
    await user.click(screen.getByText('OpenAI'))
    expect(screen.getByLabelText('Name')).toHaveValue('OpenAI')
    await user.click(screen.getByText('Cancel'))
    expect(screen.queryByLabelText('Name')).not.toBeInTheDocument()
  })
})
