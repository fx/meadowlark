import { act, fireEvent, render, screen, waitFor } from '@testing-library/preact'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { clearCache } from '@/hooks/use-fetch'

// Store onValueChange callbacks keyed by select trigger id
const selectCallbacks: Record<string, (value: string) => void> = {}

// Mock Select to capture onValueChange callbacks for testing
vi.mock('@/components/ui/select', () => {
  let currentOnValueChange: ((value: string) => void) | undefined

  return {
    Select: ({
      children,
      onValueChange,
    }: {
      children: preact.ComponentChildren
      value?: string
      onValueChange?: (value: string) => void
      disabled?: boolean
    }) => {
      currentOnValueChange = onValueChange
      return <div data-testid="mock-select">{children}</div>
    },
    SelectTrigger: ({ children, id }: { children: preact.ComponentChildren; id?: string }) => {
      if (id && currentOnValueChange) {
        selectCallbacks[id] = currentOnValueChange
      }
      return <div data-testid={`select-trigger-${id}`}>{children}</div>
    },
    SelectValue: ({ placeholder }: { placeholder?: string }) => <span>{placeholder}</span>,
    SelectContent: ({ children }: { children: preact.ComponentChildren }) => <div>{children}</div>,
    SelectItem: ({ children, value }: { children: preact.ComponentChildren; value: string }) => (
      <option value={value}>{children}</option>
    ),
  }
})

import { AliasesPage } from './aliases'

const mockAliases = [
  {
    id: 'alias-1',
    name: 'Friendly Voice',
    endpoint_id: 'ep-1',
    model: 'tts-1',
    voice: 'alloy',
    speed: 1.0,
    instructions: '',
    languages: ['en'],
    enabled: true,
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
  },
  {
    id: 'alias-2',
    name: 'Narrator',
    endpoint_id: 'ep-1',
    model: 'tts-1-hd',
    voice: 'nova',
    languages: ['en', 'fr'],
    enabled: false,
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
  },
]

const mockEndpoints = [
  {
    id: 'ep-1',
    name: 'OpenAI',
    base_url: 'https://api.openai.com',
    models: ['tts-1', 'tts-1-hd'],
    default_response_format: 'wav',
    enabled: true,
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
  },
]

function mockFetch(url: string) {
  if (url === '/api/v1/aliases') {
    return Promise.resolve({
      ok: true,
      status: 200,
      json: () => Promise.resolve(mockAliases),
    })
  }
  if (url === '/api/v1/endpoints') {
    return Promise.resolve({
      ok: true,
      status: 200,
      json: () => Promise.resolve(mockEndpoints),
    })
  }
  if (url.match(/\/api\/v1\/endpoints\/[^/]+\/configured-models/)) {
    return Promise.resolve({
      ok: true,
      json: () => Promise.resolve(['alloy', 'nova', 'shimmer']),
    })
  }
  return Promise.resolve({
    ok: true,
    status: 200,
    json: () => Promise.resolve({}),
  })
}

describe('AliasesPage', () => {
  beforeEach(() => {
    clearCache()
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string | URL | Request, init?: RequestInit) => {
        const urlStr = typeof url === 'string' ? url : url.toString()

        if (init?.method === 'DELETE') {
          return Promise.resolve({
            ok: true,
            status: 204,
            json: () => Promise.resolve(undefined),
          })
        }

        if (init?.method === 'PUT') {
          return Promise.resolve({
            ok: true,
            status: 200,
            json: () =>
              Promise.resolve({
                ...mockAliases[0],
                ...(init.body ? JSON.parse(init.body as string) : {}),
              }),
          })
        }

        if (init?.method === 'POST') {
          if (urlStr.includes('/test')) {
            return Promise.resolve({
              ok: true,
              status: 200,
              json: () => Promise.resolve({ ok: true }),
            })
          }
          return Promise.resolve({
            ok: true,
            status: 200,
            json: () =>
              Promise.resolve({
                id: 'alias-new',
                name: 'Created',
                endpoint_id: 'ep-1',
                model: 'tts-1',
                voice: 'alloy',
                languages: ['en'],
                enabled: true,
                created_at: '2024-01-01T00:00:00Z',
                updated_at: '2024-01-01T00:00:00Z',
              }),
          })
        }

        return mockFetch(urlStr)
      }) as typeof fetch,
    )
  })

  afterEach(() => {
    vi.restoreAllMocks()
    clearCache()
    for (const key of Object.keys(selectCallbacks)) {
      delete selectCallbacks[key]
    }
  })

  it('shows loading state', () => {
    vi.stubGlobal('fetch', vi.fn(() => new Promise(() => {})) as typeof fetch)
    render(<AliasesPage />)
    expect(screen.getByText('Loading aliases...')).toBeInTheDocument()
  })

  it('shows error state', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string | URL | Request) => {
        const urlStr = typeof url === 'string' ? url : url.toString()
        if (urlStr === '/api/v1/aliases') {
          return Promise.resolve({
            ok: false,
            status: 500,
            json: () =>
              Promise.resolve({ error: { code: 'server_error', message: 'Server error' } }),
          })
        }
        return mockFetch(urlStr)
      }) as typeof fetch,
    )
    render(<AliasesPage />)
    await waitFor(() => {
      expect(screen.getByText('Error: Server error')).toBeInTheDocument()
    })
  })

  it('renders alias list', async () => {
    render(<AliasesPage />)
    await waitFor(() => {
      expect(screen.getByText('Friendly Voice')).toBeInTheDocument()
    })
    expect(screen.getByText('Narrator')).toBeInTheDocument()
    expect(screen.getByText('alloy')).toBeInTheDocument()
    expect(screen.getByText('nova')).toBeInTheDocument()
    expect(screen.getAllByText('OpenAI')).toHaveLength(2)
  })

  it('shows + Add Alias button', async () => {
    render(<AliasesPage />)
    await waitFor(() => {
      expect(screen.getByText('+ Add Alias')).toBeInTheDocument()
    })
  })

  it('expands create form when + Add Alias is clicked', async () => {
    render(<AliasesPage />)
    await waitFor(() => {
      expect(screen.getByText('+ Add Alias')).toBeInTheDocument()
    })
    fireEvent.click(screen.getByText('+ Add Alias'))
    expect(screen.getByLabelText('Alias Name')).toBeInTheDocument()
    expect(screen.getByText('Create')).toBeInTheDocument()
  })

  it('collapses create form when + Add Alias clicked again', async () => {
    render(<AliasesPage />)
    await waitFor(() => {
      expect(screen.getByText('+ Add Alias')).toBeInTheDocument()
    })
    fireEvent.click(screen.getByText('+ Add Alias'))
    expect(screen.getByLabelText('Alias Name')).toBeInTheDocument()
    fireEvent.click(screen.getByText('+ Add Alias'))
    expect(screen.queryByLabelText('Alias Name')).not.toBeInTheDocument()
  })

  it('expands alias row on click to show edit form', async () => {
    render(<AliasesPage />)
    await waitFor(() => {
      expect(screen.getByText('Friendly Voice')).toBeInTheDocument()
    })
    fireEvent.click(screen.getByText('Friendly Voice'))
    await waitFor(() => {
      expect(screen.getByText('Update')).toBeInTheDocument()
    })
  })

  it('shows endpoint badge for each alias', async () => {
    render(<AliasesPage />)
    await waitFor(() => {
      expect(screen.getAllByText('OpenAI')).toHaveLength(2)
    })
  })

  it('shows enabled switch for each alias', async () => {
    render(<AliasesPage />)
    await waitFor(() => {
      expect(screen.getByText('Friendly Voice')).toBeInTheDocument()
    })
    const toggles = screen.getAllByRole('switch')
    expect(toggles.length).toBeGreaterThanOrEqual(2)
  })

  it('creates an alias via the form', async () => {
    render(<AliasesPage />)
    await waitFor(() => {
      expect(screen.getByText('+ Add Alias')).toBeInTheDocument()
    })
    fireEvent.click(screen.getByText('+ Add Alias'))

    // Select endpoint and model via captured callbacks
    act(() => {
      selectCallbacks['alias-endpoint']('ep-1')
    })
    act(() => {
      selectCallbacks['alias-model']('tts-1')
    })

    fireEvent.input(screen.getByLabelText('Alias Name'), { target: { value: 'New Alias' } })
    fireEvent.input(screen.getByLabelText('Voice'), { target: { value: 'echo' } })

    const form = screen.getByLabelText('Alias Name').closest('form') as HTMLFormElement
    fireEvent.submit(form)

    await waitFor(() => {
      expect(globalThis.fetch).toHaveBeenCalledWith(
        '/api/v1/aliases',
        expect.objectContaining({ method: 'POST' }),
      )
    })
  })

  it('handles create failure gracefully', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string | URL | Request, init?: RequestInit) => {
        const urlStr = typeof url === 'string' ? url : url.toString()
        if (init?.method === 'POST' && !urlStr.includes('/test')) {
          return Promise.resolve({
            ok: false,
            status: 400,
            json: () =>
              Promise.resolve({ error: { code: 'validation_error', message: 'Validation error' } }),
          })
        }
        return mockFetch(urlStr)
      }) as typeof fetch,
    )

    render(<AliasesPage />)
    await waitFor(() => {
      expect(screen.getByText('+ Add Alias')).toBeInTheDocument()
    })
    fireEvent.click(screen.getByText('+ Add Alias'))

    // Select endpoint and model via captured callbacks
    act(() => {
      selectCallbacks['alias-endpoint']('ep-1')
    })
    act(() => {
      selectCallbacks['alias-model']('tts-1')
    })

    fireEvent.input(screen.getByLabelText('Alias Name'), { target: { value: 'Bad Alias' } })
    fireEvent.input(screen.getByLabelText('Voice'), { target: { value: 'echo' } })

    const form = screen.getByLabelText('Alias Name').closest('form') as HTMLFormElement
    fireEvent.submit(form)

    await waitFor(() => {
      expect(globalThis.fetch).toHaveBeenCalledWith(
        '/api/v1/aliases',
        expect.objectContaining({ method: 'POST' }),
      )
    })
    // Form should still be visible (not collapsed) since create failed
    expect(screen.getByLabelText('Alias Name')).toBeInTheDocument()
  })

  it('updates an alias via the edit form', async () => {
    render(<AliasesPage />)
    await waitFor(() => {
      expect(screen.getByText('Friendly Voice')).toBeInTheDocument()
    })
    fireEvent.click(screen.getByText('Friendly Voice'))

    await waitFor(() => {
      expect(screen.getByText('Update')).toBeInTheDocument()
    })

    fireEvent.input(screen.getByLabelText('Alias Name'), { target: { value: 'Updated Voice' } })
    const form = screen.getByLabelText('Alias Name').closest('form') as HTMLFormElement
    fireEvent.submit(form)

    await waitFor(() => {
      expect(globalThis.fetch).toHaveBeenCalledWith(
        '/api/v1/aliases/alias-1',
        expect.objectContaining({ method: 'PUT' }),
      )
    })
  })

  it('deletes an alias with confirmation', async () => {
    render(<AliasesPage />)
    await waitFor(() => {
      expect(screen.getByText('Friendly Voice')).toBeInTheDocument()
    })
    fireEvent.click(screen.getByText('Friendly Voice'))

    await waitFor(() => {
      expect(screen.getByText('Delete')).toBeInTheDocument()
    })

    fireEvent.click(screen.getByText('Delete'))

    await waitFor(() => {
      expect(screen.getByText('Delete alias')).toBeInTheDocument()
    })

    const confirmBtn = screen
      .getAllByText('Delete')
      .find((el) => el.closest('[role="alertdialog"]') !== null)
    expect(confirmBtn).toBeTruthy()
    fireEvent.click(confirmBtn as HTMLElement)

    await waitFor(() => {
      expect(globalThis.fetch).toHaveBeenCalledWith(
        '/api/v1/aliases/alias-1',
        expect.objectContaining({ method: 'DELETE' }),
      )
    })
  })

  it('tests TTS for an alias', async () => {
    render(<AliasesPage />)
    await waitFor(() => {
      expect(screen.getByText('Friendly Voice')).toBeInTheDocument()
    })
    fireEvent.click(screen.getByText('Friendly Voice'))

    await waitFor(() => {
      expect(screen.getByText('Test TTS')).toBeInTheDocument()
    })

    fireEvent.click(screen.getByText('Test TTS'))

    await waitFor(() => {
      expect(screen.getByText('OK')).toBeInTheDocument()
    })
  })

  it('shows latency when test result includes latency_ms', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string | URL | Request, init?: RequestInit) => {
        const urlStr = typeof url === 'string' ? url : url.toString()
        if (init?.method === 'POST' && urlStr.includes('/test')) {
          return Promise.resolve({
            ok: true,
            status: 200,
            json: () => Promise.resolve({ ok: true, latency_ms: 42 }),
          })
        }
        return mockFetch(urlStr)
      }) as typeof fetch,
    )

    render(<AliasesPage />)
    await waitFor(() => {
      expect(screen.getByText('Friendly Voice')).toBeInTheDocument()
    })
    fireEvent.click(screen.getByText('Friendly Voice'))

    await waitFor(() => {
      expect(screen.getByText('Test TTS')).toBeInTheDocument()
    })

    fireEvent.click(screen.getByText('Test TTS'))

    await waitFor(() => {
      expect(screen.getByText('OK (42ms)')).toBeInTheDocument()
    })
  })

  it('shows test failure result', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string | URL | Request, init?: RequestInit) => {
        const urlStr = typeof url === 'string' ? url : url.toString()
        if (init?.method === 'POST' && urlStr.includes('/test')) {
          return Promise.resolve({
            ok: true,
            status: 200,
            json: () => Promise.resolve({ ok: false, error: 'Voice not found' }),
          })
        }
        return mockFetch(urlStr)
      }) as typeof fetch,
    )

    render(<AliasesPage />)
    await waitFor(() => {
      expect(screen.getByText('Friendly Voice')).toBeInTheDocument()
    })
    fireEvent.click(screen.getByText('Friendly Voice'))

    await waitFor(() => {
      expect(screen.getByText('Test TTS')).toBeInTheDocument()
    })

    fireEvent.click(screen.getByText('Test TTS'))

    await waitFor(() => {
      expect(screen.getByText('Failed: Voice not found')).toBeInTheDocument()
    })
  })

  it('shows network error when test fails', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string | URL | Request, init?: RequestInit) => {
        const urlStr = typeof url === 'string' ? url : url.toString()
        if (init?.method === 'POST' && urlStr.includes('/test')) {
          return Promise.reject(new Error('Network error'))
        }
        return mockFetch(urlStr)
      }) as typeof fetch,
    )

    render(<AliasesPage />)
    await waitFor(() => {
      expect(screen.getByText('Friendly Voice')).toBeInTheDocument()
    })
    fireEvent.click(screen.getByText('Friendly Voice'))

    await waitFor(() => {
      expect(screen.getByText('Test TTS')).toBeInTheDocument()
    })

    fireEvent.click(screen.getByText('Test TTS'))

    await waitFor(() => {
      expect(screen.getByText('Failed: Network error')).toBeInTheDocument()
    })
  })

  it('shows fallback error when test throws non-Error', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string | URL | Request, init?: RequestInit) => {
        const urlStr = typeof url === 'string' ? url : url.toString()
        if (init?.method === 'POST' && urlStr.includes('/test')) {
          return Promise.reject('unexpected')
        }
        return mockFetch(urlStr)
      }) as typeof fetch,
    )

    render(<AliasesPage />)
    await waitFor(() => {
      expect(screen.getByText('Friendly Voice')).toBeInTheDocument()
    })
    fireEvent.click(screen.getByText('Friendly Voice'))

    await waitFor(() => {
      expect(screen.getByText('Test TTS')).toBeInTheDocument()
    })

    fireEvent.click(screen.getByText('Test TTS'))

    await waitFor(() => {
      expect(screen.getByText('Failed: Network error')).toBeInTheDocument()
    })
  })

  it('shows error when test returns non-OK HTTP response', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string | URL | Request, init?: RequestInit) => {
        const urlStr = typeof url === 'string' ? url : url.toString()
        if (init?.method === 'POST' && urlStr.includes('/test')) {
          return Promise.resolve({
            ok: false,
            status: 500,
            json: () =>
              Promise.resolve({
                error: { code: 'tts_error', message: 'TTS service unavailable' },
              }),
          })
        }
        return mockFetch(urlStr)
      }) as typeof fetch,
    )

    render(<AliasesPage />)
    await waitFor(() => {
      expect(screen.getByText('Friendly Voice')).toBeInTheDocument()
    })
    fireEvent.click(screen.getByText('Friendly Voice'))

    await waitFor(() => {
      expect(screen.getByText('Test TTS')).toBeInTheDocument()
    })

    fireEvent.click(screen.getByText('Test TTS'))

    await waitFor(() => {
      expect(screen.getByText('Failed: TTS service unavailable')).toBeInTheDocument()
    })
  })

  it('toggles enabled switch on alias row', async () => {
    render(<AliasesPage />)
    await waitFor(() => {
      expect(screen.getByText('Friendly Voice')).toBeInTheDocument()
    })

    const toggle = screen.getByRole('switch', { name: 'Toggle Friendly Voice' })
    fireEvent.click(toggle)

    await waitFor(() => {
      expect(globalThis.fetch).toHaveBeenCalledWith(
        '/api/v1/aliases/alias-1',
        expect.objectContaining({ method: 'PUT' }),
      )
    })
  })

  it('shows Unknown for alias with missing endpoint', async () => {
    const aliasWithBadEndpoint = [
      {
        ...mockAliases[0],
        endpoint_id: 'nonexistent',
      },
    ]
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string | URL | Request) => {
        const urlStr = typeof url === 'string' ? url : url.toString()
        if (urlStr === '/api/v1/aliases') {
          return Promise.resolve({
            ok: true,
            status: 200,
            json: () => Promise.resolve(aliasWithBadEndpoint),
          })
        }
        return mockFetch(urlStr)
      }) as typeof fetch,
    )

    render(<AliasesPage />)
    await waitFor(() => {
      expect(screen.getByText('Unknown')).toBeInTheDocument()
    })
  })

  it('cancels create form', async () => {
    render(<AliasesPage />)
    await waitFor(() => {
      expect(screen.getByText('+ Add Alias')).toBeInTheDocument()
    })
    fireEvent.click(screen.getByText('+ Add Alias'))
    expect(screen.getByText('Cancel')).toBeInTheDocument()
    fireEvent.click(screen.getByText('Cancel'))
    expect(screen.queryByLabelText('Alias Name')).not.toBeInTheDocument()
  })

  it('cancels edit form', async () => {
    render(<AliasesPage />)
    await waitFor(() => {
      expect(screen.getByText('Friendly Voice')).toBeInTheDocument()
    })
    fireEvent.click(screen.getByText('Friendly Voice'))
    await waitFor(() => {
      expect(screen.getByText('Cancel')).toBeInTheDocument()
    })
    fireEvent.click(screen.getByText('Cancel'))
    expect(screen.queryByText('Update')).not.toBeInTheDocument()
  })

  it('shows empty state when no aliases exist', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string | URL | Request) => {
        const urlStr = typeof url === 'string' ? url : url.toString()
        if (urlStr === '/api/v1/aliases') {
          return Promise.resolve({
            ok: true,
            status: 200,
            json: () => Promise.resolve([]),
          })
        }
        return mockFetch(urlStr)
      }) as typeof fetch,
    )

    render(<AliasesPage />)
    await waitFor(() => {
      expect(screen.getByText('No aliases configured. Add one to get started.')).toBeInTheDocument()
    })
  })

  it('handles toggle enabled failure gracefully', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string | URL | Request, init?: RequestInit) => {
        const urlStr = typeof url === 'string' ? url : url.toString()
        if (init?.method === 'PUT') {
          return Promise.resolve({
            ok: false,
            status: 500,
            json: () =>
              Promise.resolve({ error: { code: 'server_error', message: 'Toggle failed' } }),
          })
        }
        return mockFetch(urlStr)
      }) as typeof fetch,
    )

    render(<AliasesPage />)
    await waitFor(() => {
      expect(screen.getByText('Friendly Voice')).toBeInTheDocument()
    })

    const toggle = screen.getByRole('switch', { name: 'Toggle Friendly Voice' })
    fireEvent.click(toggle)

    // Should not crash - error is handled gracefully
    await waitFor(() => {
      expect(globalThis.fetch).toHaveBeenCalledWith(
        '/api/v1/aliases/alias-1',
        expect.objectContaining({ method: 'PUT' }),
      )
    })
  })
})
