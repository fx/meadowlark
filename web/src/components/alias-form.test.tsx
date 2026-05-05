import { act, fireEvent, render, screen, waitFor } from '@testing-library/preact'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { Endpoint, VoiceAlias } from '@/lib/api'

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

import { AliasForm } from './alias-form'

const mockEndpoints: Endpoint[] = [
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
  {
    id: 'ep-2',
    name: 'Local',
    base_url: 'http://localhost:8080',
    models: ['piper'],
    default_response_format: 'wav',
    enabled: true,
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
  },
]

function makeEv(voiceId: string, name: string, enabled = true) {
  return {
    endpoint_id: 'ep-1',
    voice_id: voiceId,
    name,
    enabled,
    created_at: '',
    updated_at: '',
  }
}

const mockAlias: VoiceAlias = {
  id: 'alias-1',
  name: 'Test Alias',
  endpoint_id: 'ep-1',
  model: 'tts-1',
  voice: 'alloy',
  speed: 1.5,
  instructions: 'Speak clearly',
  languages: ['en', 'fr'],
  enabled: true,
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-01T00:00:00Z',
}

describe('AliasForm', () => {
  let fetchMock: ReturnType<typeof vi.fn>

  beforeEach(() => {
    fetchMock = vi.fn()
    vi.stubGlobal('fetch', fetchMock)
    // Default: voice discovery returns empty
    fetchMock.mockResolvedValue({
      ok: true,
      json: () => Promise.resolve([]),
    })
  })

  afterEach(() => {
    vi.restoreAllMocks()
    for (const key of Object.keys(selectCallbacks)) {
      delete selectCallbacks[key]
    }
  })

  it('renders create form with defaults', () => {
    const onSubmit = vi.fn()
    const onCancel = vi.fn()
    render(
      <AliasForm
        endpoints={mockEndpoints}
        onSubmit={onSubmit}
        onCancel={onCancel}
        isSaving={false}
      />,
    )
    expect(screen.getByLabelText('Alias Name')).toHaveValue('')
    expect(screen.getByLabelText('Speed')).toHaveValue(null)
    expect(screen.getByText('Create')).toBeInTheDocument()
    expect(screen.getByText('Cancel')).toBeInTheDocument()
  })

  it('renders edit form with alias data', async () => {
    const onSubmit = vi.fn()
    const onCancel = vi.fn()
    fetchMock.mockResolvedValue({
      ok: true,
      json: () =>
        Promise.resolve([
          {
            endpoint_id: 'ep-1',
            voice_id: 'alloy',
            name: 'Alloy',
            enabled: true,
            created_at: '',
            updated_at: '',
          },
          {
            endpoint_id: 'ep-1',
            voice_id: 'nova',
            name: 'Nova',
            enabled: true,
            created_at: '',
            updated_at: '',
          },
        ]),
    })
    render(
      <AliasForm
        alias={mockAlias}
        endpoints={mockEndpoints}
        onSubmit={onSubmit}
        onCancel={onCancel}
        isSaving={false}
      />,
    )
    expect(screen.getByLabelText('Alias Name')).toHaveValue('Test Alias')
    expect(screen.getByLabelText('Speed')).toHaveValue(1.5)
    expect(screen.getByLabelText('Instructions')).toHaveValue('Speak clearly')
    expect(screen.getByLabelText('Languages (comma-separated)')).toHaveValue('en, fr')
    expect(screen.getByText('Update')).toBeInTheDocument()
  })

  it('shows Saving... when isSaving is true', () => {
    render(
      <AliasForm endpoints={mockEndpoints} onSubmit={vi.fn()} onCancel={vi.fn()} isSaving={true} />,
    )
    expect(screen.getByText('Saving...')).toBeInTheDocument()
  })

  it('calls onCancel when cancel is clicked', () => {
    const onCancel = vi.fn()
    render(
      <AliasForm
        endpoints={mockEndpoints}
        onSubmit={vi.fn()}
        onCancel={onCancel}
        isSaving={false}
      />,
    )
    fireEvent.click(screen.getByText('Cancel'))
    expect(onCancel).toHaveBeenCalled()
  })

  it('submits form data', () => {
    const onSubmit = vi.fn()
    render(
      <AliasForm
        endpoints={mockEndpoints}
        onSubmit={onSubmit}
        onCancel={vi.fn()}
        isSaving={false}
      />,
    )
    // Select endpoint and model via captured callbacks
    act(() => {
      selectCallbacks['alias-endpoint']('ep-1')
    })
    act(() => {
      selectCallbacks['alias-model']('tts-1')
    })

    fireEvent.input(screen.getByLabelText('Alias Name'), { target: { value: 'My Alias' } })
    fireEvent.input(screen.getByLabelText('Voice'), { target: { value: 'nova' } })
    fireEvent.input(screen.getByLabelText('Speed'), { target: { value: '1.2' } })
    fireEvent.input(screen.getByLabelText('Instructions'), { target: { value: 'Be nice' } })
    fireEvent.input(screen.getByLabelText('Languages (comma-separated)'), {
      target: { value: 'en, de' },
    })

    const form = screen.getByLabelText('Alias Name').closest('form') as HTMLFormElement
    fireEvent.submit(form)
    expect(onSubmit).toHaveBeenCalledWith({
      name: 'My Alias',
      endpoint_id: 'ep-1',
      model: 'tts-1',
      voice: 'nova',
      speed: 1.2,
      instructions: 'Be nice',
      languages: ['en', 'de'],
      enabled: true,
    })
  })

  it('submits with empty optional fields as undefined', () => {
    const onSubmit = vi.fn()
    render(
      <AliasForm
        endpoints={mockEndpoints}
        onSubmit={onSubmit}
        onCancel={vi.fn()}
        isSaving={false}
      />,
    )
    // Select endpoint and model via captured callbacks
    act(() => {
      selectCallbacks['alias-endpoint']('ep-1')
    })
    act(() => {
      selectCallbacks['alias-model']('tts-1')
    })

    fireEvent.input(screen.getByLabelText('Alias Name'), { target: { value: 'Min' } })
    fireEvent.input(screen.getByLabelText('Voice'), { target: { value: 'echo' } })
    // Clear languages
    fireEvent.input(screen.getByLabelText('Languages (comma-separated)'), {
      target: { value: '' },
    })

    const form = screen.getByLabelText('Alias Name').closest('form') as HTMLFormElement
    fireEvent.submit(form)
    expect(onSubmit).toHaveBeenCalledWith({
      name: 'Min',
      endpoint_id: 'ep-1',
      model: 'tts-1',
      voice: 'echo',
      speed: undefined,
      instructions: undefined,
      languages: undefined,
      enabled: true,
    })
  })

  it('fetches voices when endpoint is selected and shows select', async () => {
    fetchMock.mockResolvedValue({
      ok: true,
      json: () =>
        Promise.resolve([
          makeEv('alloy', 'Alloy'),
          makeEv('nova', 'Nova'),
          makeEv('shimmer', 'Shimmer'),
        ]),
    })
    const { rerender } = render(
      <AliasForm
        alias={mockAlias}
        endpoints={mockEndpoints}
        onSubmit={vi.fn()}
        onCancel={vi.fn()}
        isSaving={false}
      />,
    )
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        '/api/v1/endpoints/ep-1/voices',
        expect.objectContaining({ signal: expect.any(AbortSignal) }),
      )
    })
    // With voices loaded, it should show select trigger instead of input
    rerender(
      <AliasForm
        alias={mockAlias}
        endpoints={mockEndpoints}
        onSubmit={vi.fn()}
        onCancel={vi.fn()}
        isSaving={false}
      />,
    )
    await waitFor(() => {
      expect(screen.queryByPlaceholderText('Enter voice name')).not.toBeInTheDocument()
    })
  })

  it('lists voice names from /voices and submits the voice id', async () => {
    fetchMock.mockResolvedValue({
      ok: true,
      json: () => Promise.resolve([makeEv('alloy', 'Alloy'), makeEv('clone:abc', 'Clone ABC')]),
    })
    const onSubmit = vi.fn()
    render(
      <AliasForm
        alias={mockAlias}
        endpoints={mockEndpoints}
        onSubmit={onSubmit}
        onCancel={vi.fn()}
        isSaving={false}
      />,
    )
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        '/api/v1/endpoints/ep-1/voices',
        expect.objectContaining({ signal: expect.any(AbortSignal) }),
      )
    })
    await waitFor(() => {
      expect(screen.getByText('Alloy')).toBeInTheDocument()
    })
    expect(screen.getByText('Clone ABC')).toBeInTheDocument()
    // Secondary id label appears when name !== id
    expect(screen.getByText('clone:abc')).toBeInTheDocument()
    // The Select option values are the voice ids
    const cloneOption = screen.getByText('Clone ABC').closest('option') as HTMLOptionElement
    expect(cloneOption).toBeTruthy()
    expect(cloneOption.getAttribute('value')).toBe('clone:abc')

    // Submit by selecting the clone voice via captured callback and submitting form
    act(() => {
      selectCallbacks['alias-voice']('clone:abc')
    })
    const form = screen.getByLabelText('Alias Name').closest('form') as HTMLFormElement
    fireEvent.submit(form)
    expect(onSubmit).toHaveBeenCalledWith(
      expect.objectContaining({
        voice: 'clone:abc',
      }),
    )
  })

  it('renders voice options without secondary id text when name equals id or is empty', async () => {
    fetchMock.mockResolvedValue({
      ok: true,
      json: () =>
        Promise.resolve([
          // name === id: no secondary id text
          makeEv('alloy', 'alloy'),
          // empty name: label falls back to id, no secondary id text
          makeEv('echo', ''),
        ]),
    })
    render(
      <AliasForm
        alias={mockAlias}
        endpoints={mockEndpoints}
        onSubmit={vi.fn()}
        onCancel={vi.fn()}
        isSaving={false}
      />,
    )
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        '/api/v1/endpoints/ep-1/voices',
        expect.objectContaining({ signal: expect.any(AbortSignal) }),
      )
    })
    await waitFor(() => {
      expect(screen.getByText('alloy')).toBeInTheDocument()
    })
    // 'echo' rendered (fallback from empty name)
    expect(screen.getByText('echo')).toBeInTheDocument()
    // Each voice should appear exactly once -- no secondary id span
    expect(screen.getAllByText('alloy')).toHaveLength(1)
    expect(screen.getAllByText('echo')).toHaveLength(1)
  })

  it('falls back to text input on 502 and submits typed voice value', async () => {
    fetchMock.mockResolvedValue({
      ok: false,
      status: 502,
      json: () => Promise.resolve({ error: { message: 'bad gateway' } }),
    })
    const onSubmit = vi.fn()
    render(
      <AliasForm
        endpoints={mockEndpoints}
        onSubmit={onSubmit}
        onCancel={vi.fn()}
        isSaving={false}
      />,
    )
    act(() => {
      selectCallbacks['alias-endpoint']('ep-2')
    })
    act(() => {
      selectCallbacks['alias-model']('piper')
    })
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        '/api/v1/endpoints/ep-2/voices',
        expect.objectContaining({ signal: expect.any(AbortSignal) }),
      )
    })
    await waitFor(() => {
      expect(screen.getByPlaceholderText('Enter voice name')).toBeInTheDocument()
    })
    fireEvent.input(screen.getByLabelText('Alias Name'), { target: { value: 'Manual' } })
    fireEvent.input(screen.getByLabelText('Voice'), { target: { value: 'typed-voice' } })
    const form = screen.getByLabelText('Alias Name').closest('form') as HTMLFormElement
    fireEvent.submit(form)
    expect(onSubmit).toHaveBeenCalledWith(
      expect.objectContaining({
        endpoint_id: 'ep-2',
        voice: 'typed-voice',
      }),
    )
  })

  it('filters out disabled voices from the dropdown', async () => {
    fetchMock.mockResolvedValue({
      ok: true,
      json: () =>
        Promise.resolve([
          makeEv('alloy', 'Alloy', true),
          makeEv('hidden', 'Hidden', false),
          makeEv('nova', 'Nova', true),
        ]),
    })
    render(
      <AliasForm
        alias={mockAlias}
        endpoints={mockEndpoints}
        onSubmit={vi.fn()}
        onCancel={vi.fn()}
        isSaving={false}
      />,
    )
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        '/api/v1/endpoints/ep-1/voices',
        expect.objectContaining({ signal: expect.any(AbortSignal) }),
      )
    })
    await waitFor(() => {
      expect(screen.getByText('Alloy')).toBeInTheDocument()
    })
    expect(screen.getByText('Nova')).toBeInTheDocument()
    expect(screen.queryByText('Hidden')).not.toBeInTheDocument()
  })

  it('falls back to text input when /voices returns empty array', async () => {
    fetchMock.mockResolvedValue({
      ok: true,
      json: () => Promise.resolve([]),
    })
    render(
      <AliasForm
        alias={{ ...mockAlias, endpoint_id: 'ep-2' }}
        endpoints={mockEndpoints}
        onSubmit={vi.fn()}
        onCancel={vi.fn()}
        isSaving={false}
      />,
    )
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        '/api/v1/endpoints/ep-2/voices',
        expect.objectContaining({ signal: expect.any(AbortSignal) }),
      )
    })
    await waitFor(() => {
      expect(screen.getByPlaceholderText('Enter voice name')).toBeInTheDocument()
    })
  })

  it('shows input for voice when voices fetch fails', async () => {
    fetchMock.mockRejectedValue(new Error('Network error'))
    render(
      <AliasForm
        alias={{ ...mockAlias, endpoint_id: 'ep-2' }}
        endpoints={mockEndpoints}
        onSubmit={vi.fn()}
        onCancel={vi.fn()}
        isSaving={false}
      />,
    )
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        '/api/v1/endpoints/ep-2/voices',
        expect.objectContaining({ signal: expect.any(AbortSignal) }),
      )
    })
    // Should fall back to input
    await waitFor(() => {
      expect(screen.getByPlaceholderText('Enter voice name')).toBeInTheDocument()
    })
  })

  it('shows input for voice when voices response is not ok', async () => {
    fetchMock.mockResolvedValue({
      ok: false,
      json: () => Promise.resolve({ error: { message: 'not found' } }),
    })
    render(
      <AliasForm
        alias={{ ...mockAlias, endpoint_id: 'ep-2' }}
        endpoints={mockEndpoints}
        onSubmit={vi.fn()}
        onCancel={vi.fn()}
        isSaving={false}
      />,
    )
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        '/api/v1/endpoints/ep-2/voices',
        expect.objectContaining({ signal: expect.any(AbortSignal) }),
      )
    })
    await waitFor(() => {
      expect(screen.getByPlaceholderText('Enter voice name')).toBeInTheDocument()
    })
  })

  it('aborts in-flight voices request when endpoint changes', async () => {
    const abortedSignals: AbortSignal[] = []
    fetchMock.mockImplementation(
      (_url: string, init?: RequestInit) =>
        new Promise((_resolve, reject) => {
          const signal = init?.signal as AbortSignal | undefined
          if (signal) {
            abortedSignals.push(signal)
            signal.addEventListener('abort', () => {
              reject(new DOMException('aborted', 'AbortError'))
            })
          }
        }),
    )
    render(
      <AliasForm
        alias={mockAlias}
        endpoints={mockEndpoints}
        onSubmit={vi.fn()}
        onCancel={vi.fn()}
        isSaving={false}
      />,
    )
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        '/api/v1/endpoints/ep-1/voices',
        expect.objectContaining({ signal: expect.any(AbortSignal) }),
      )
    })
    expect(abortedSignals[0].aborted).toBe(false)
    act(() => {
      selectCallbacks['alias-endpoint']('ep-2')
    })
    await waitFor(() => {
      expect(abortedSignals[0].aborted).toBe(true)
    })
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        '/api/v1/endpoints/ep-2/voices',
        expect.objectContaining({ signal: expect.any(AbortSignal) }),
      )
    })
  })

  it('ignores fulfilled response from a fetch that was already aborted', async () => {
    // This mock does NOT reject on abort — it lets us simulate the race where
    // fetch had already produced a Response by the time abort fired, so .then
    // runs and must see signal.aborted === true.
    const resolvers: Array<(res: { ok: boolean; json: () => Promise<unknown> }) => void> = []
    const signals: AbortSignal[] = []
    fetchMock.mockImplementation((url: string, init?: RequestInit) => {
      const signal = init?.signal as AbortSignal | undefined
      if (signal) signals.push(signal)
      if (url.includes('ep-1')) {
        return new Promise((resolve) => {
          resolvers.push(resolve)
        })
      }
      return Promise.resolve({
        ok: true,
        json: () => Promise.resolve([makeEv('piper-en', 'Piper English')]),
      })
    })
    render(
      <AliasForm
        alias={mockAlias}
        endpoints={mockEndpoints}
        onSubmit={vi.fn()}
        onCancel={vi.fn()}
        isSaving={false}
      />,
    )
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith('/api/v1/endpoints/ep-1/voices', expect.any(Object))
    })
    // Switch endpoint -> aborts the ep-1 request and starts the ep-2 fetch
    act(() => {
      selectCallbacks['alias-endpoint']('ep-2')
    })
    await waitFor(() => {
      expect(signals[0].aborted).toBe(true)
    })
    await waitFor(() => {
      expect(screen.getByText('Piper English')).toBeInTheDocument()
    })
    // Now race-fulfil the ORIGINAL ep-1 request with stale data
    act(() => {
      resolvers[0]({
        ok: true,
        json: () => Promise.resolve([makeEv('stale', 'Stale Voice')]),
      })
    })
    // Allow microtasks to flush; the guard MUST prevent the stale write
    await new Promise((r) => setTimeout(r, 0))
    expect(screen.queryByText('Stale Voice')).not.toBeInTheDocument()
    expect(screen.getByText('Piper English')).toBeInTheDocument()
  })

  it('ignores non-ok response from a fetch that was already aborted', async () => {
    const resolvers: Array<(res: { ok: boolean; json: () => Promise<unknown> }) => void> = []
    const signals: AbortSignal[] = []
    fetchMock.mockImplementation((url: string, init?: RequestInit) => {
      const signal = init?.signal as AbortSignal | undefined
      if (signal) signals.push(signal)
      if (url.includes('ep-1')) {
        return new Promise((resolve) => {
          resolvers.push(resolve)
        })
      }
      return Promise.resolve({
        ok: true,
        json: () => Promise.resolve([makeEv('piper-en', 'Piper English')]),
      })
    })
    render(
      <AliasForm
        alias={mockAlias}
        endpoints={mockEndpoints}
        onSubmit={vi.fn()}
        onCancel={vi.fn()}
        isSaving={false}
      />,
    )
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith('/api/v1/endpoints/ep-1/voices', expect.any(Object))
    })
    act(() => {
      selectCallbacks['alias-endpoint']('ep-2')
    })
    await waitFor(() => {
      expect(screen.getByText('Piper English')).toBeInTheDocument()
    })
    // Race-fulfil with non-ok; the guard must prevent setVoices([]) from clobbering ep-2's voices
    act(() => {
      resolvers[0]({ ok: false, json: () => Promise.resolve(null) })
    })
    await new Promise((r) => setTimeout(r, 0))
    expect(screen.getByText('Piper English')).toBeInTheDocument()
  })

  it('clears voices when endpoint has no id', () => {
    render(
      <AliasForm
        endpoints={mockEndpoints}
        onSubmit={vi.fn()}
        onCancel={vi.fn()}
        isSaving={false}
      />,
    )
    // No endpoint selected, should show input
    expect(screen.getByPlaceholderText('Enter voice name')).toBeInTheDocument()
  })

  it('resets model and voice when endpoint changes', async () => {
    const onSubmit = vi.fn()
    render(
      <AliasForm
        alias={mockAlias}
        endpoints={mockEndpoints}
        onSubmit={onSubmit}
        onCancel={vi.fn()}
        isSaving={false}
      />,
    )
    await waitFor(() => {
      expect(screen.getByLabelText('Alias Name')).toHaveValue('Test Alias')
    })
    // Call the endpoint select's onValueChange directly via captured callback
    act(() => {
      selectCallbacks['alias-endpoint']('ep-2')
    })

    // Submit to verify model and voice were reset
    const form = screen.getByLabelText('Alias Name').closest('form') as HTMLFormElement
    fireEvent.submit(form)
    expect(onSubmit).toHaveBeenCalledWith(
      expect.objectContaining({
        endpoint_id: 'ep-2',
        model: '',
        voice: '',
      }),
    )
  })

  it('updates model when model select changes', async () => {
    const onSubmit = vi.fn()
    render(
      <AliasForm
        alias={mockAlias}
        endpoints={mockEndpoints}
        onSubmit={onSubmit}
        onCancel={vi.fn()}
        isSaving={false}
      />,
    )
    await waitFor(() => {
      expect(screen.getByLabelText('Alias Name')).toHaveValue('Test Alias')
    })
    // Call the model select's onValueChange directly via captured callback
    act(() => {
      selectCallbacks['alias-model']('tts-1-hd')
    })

    const form = screen.getByLabelText('Alias Name').closest('form') as HTMLFormElement
    fireEvent.submit(form)
    expect(onSubmit).toHaveBeenCalledWith(
      expect.objectContaining({
        model: 'tts-1-hd',
      }),
    )
  })

  it('shows enabled switch with correct state', () => {
    render(
      <AliasForm
        alias={{ ...mockAlias, enabled: false }}
        endpoints={mockEndpoints}
        onSubmit={vi.fn()}
        onCancel={vi.fn()}
        isSaving={false}
      />,
    )
    const toggle = screen.getByRole('switch', { name: 'Enabled' })
    expect(toggle).toBeInTheDocument()
    expect(toggle).toHaveAttribute('data-state', 'unchecked')
  })
})
