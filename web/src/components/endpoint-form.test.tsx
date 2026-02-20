import { act, render, screen } from '@testing-library/preact'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import type { Endpoint } from '@/lib/api'

// Mock useEndpointProbe to avoid real fetch calls and allow per-test overrides
const mockProbe = {
  models: [] as { id: string }[],
  voices: [] as { id: string; name: string }[],
  loading: false,
  error: undefined as string | undefined,
}

vi.mock('@/hooks/use-endpoint-probe', () => ({
  useEndpointProbe: () => mockProbe,
}))

// Store onValueChange callback for model-add Select
let selectOnValueChange: ((value: string) => void) | undefined

vi.mock('@/components/ui/select', () => ({
  Select: ({
    children,
    onValueChange,
  }: {
    children: preact.ComponentChildren
    value?: string
    onValueChange?: (value: string) => void
  }) => {
    selectOnValueChange = onValueChange
    return <div data-testid="model-select">{children}</div>
  },
  SelectTrigger: ({
    children,
    'aria-label': ariaLabel,
  }: {
    children: preact.ComponentChildren
    'aria-label'?: string
    className?: string
  }) => (
    <button type="button" data-testid="model-select-trigger" aria-label={ariaLabel}>
      {children}
    </button>
  ),
  SelectValue: ({ placeholder }: { placeholder?: string }) => <span>{placeholder}</span>,
  SelectContent: ({ children }: { children: preact.ComponentChildren }) => <div>{children}</div>,
  SelectItem: ({ children, value }: { children: preact.ComponentChildren; value: string }) => (
    <option value={value}>{children}</option>
  ),
}))

import { EndpointForm } from './endpoint-form'

const mockEndpoint: Endpoint = {
  id: 'ep-1',
  name: 'Test EP',
  base_url: 'https://api.example.com',
  api_key: 'sk-123',
  models: ['tts-1', 'tts-1-hd'],
  default_speed: 1.5,
  default_instructions: 'Speak clearly',
  default_response_format: 'wav',
  enabled: true,
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-01T00:00:00Z',
}

describe('EndpointForm', () => {
  it('renders empty form for create mode', () => {
    render(<EndpointForm onSubmit={vi.fn()} onCancel={vi.fn()} isSaving={false} />)
    expect(screen.getByLabelText('Name')).toHaveValue('')
    expect(screen.getByLabelText('Base URL')).toHaveValue('')
    expect(screen.getByText('Create')).toBeInTheDocument()
  })

  it('renders populated form for edit mode', () => {
    render(
      <EndpointForm
        endpoint={mockEndpoint}
        onSubmit={vi.fn()}
        onCancel={vi.fn()}
        isSaving={false}
      />,
    )
    expect(screen.getByLabelText('Name')).toHaveValue('Test EP')
    expect(screen.getByLabelText('Base URL')).toHaveValue('https://api.example.com')
    expect(screen.getByText('Update')).toBeInTheDocument()
  })

  it('shows Saving... when isSaving is true', () => {
    render(<EndpointForm onSubmit={vi.fn()} onCancel={vi.fn()} isSaving={true} />)
    expect(screen.getByText('Saving...')).toBeInTheDocument()
  })

  it('calls onCancel when Cancel is clicked', async () => {
    const user = userEvent.setup()
    const onCancel = vi.fn()
    render(<EndpointForm onSubmit={vi.fn()} onCancel={onCancel} isSaving={false} />)
    await user.click(screen.getByText('Cancel'))
    expect(onCancel).toHaveBeenCalled()
  })

  it('toggles API key visibility', async () => {
    const user = userEvent.setup()
    render(<EndpointForm onSubmit={vi.fn()} onCancel={vi.fn()} isSaving={false} />)
    const keyInput = screen.getByLabelText('API Key')
    expect(keyInput).toHaveAttribute('type', 'password')
    await user.click(screen.getByRole('button', { name: 'Show API key' }))
    expect(keyInput).toHaveAttribute('type', 'text')
    await user.click(screen.getByRole('button', { name: 'Hide API key' }))
    expect(keyInput).toHaveAttribute('type', 'password')
  })

  it('submits form data for create', async () => {
    const user = userEvent.setup()
    const onSubmit = vi.fn()
    render(<EndpointForm onSubmit={onSubmit} onCancel={vi.fn()} isSaving={false} />)
    await user.type(screen.getByLabelText('Name'), 'New EP')
    await user.type(screen.getByLabelText('Base URL'), 'https://api.test.com')
    await user.type(screen.getByLabelText('Models (comma-separated)'), 'tts-1, tts-2')
    await user.click(screen.getByText('Create'))
    expect(onSubmit).toHaveBeenCalledWith(
      expect.objectContaining({
        name: 'New EP',
        base_url: 'https://api.test.com',
        models: ['tts-1', 'tts-2'],
        enabled: true,
      }),
    )
  })

  it('submits form data for update', async () => {
    const user = userEvent.setup()
    const onSubmit = vi.fn()
    render(
      <EndpointForm
        endpoint={mockEndpoint}
        onSubmit={onSubmit}
        onCancel={vi.fn()}
        isSaving={false}
      />,
    )
    await user.clear(screen.getByLabelText('Name'))
    await user.type(screen.getByLabelText('Name'), 'Updated EP')
    await user.click(screen.getByText('Update'))
    expect(onSubmit).toHaveBeenCalledWith(
      expect.objectContaining({
        name: 'Updated EP',
        base_url: 'https://api.example.com',
      }),
    )
  })

  it('populates speed and instructions from endpoint', () => {
    render(
      <EndpointForm
        endpoint={mockEndpoint}
        onSubmit={vi.fn()}
        onCancel={vi.fn()}
        isSaving={false}
      />,
    )
    expect(screen.getByLabelText('Default Speed')).toHaveValue(1.5)
    expect(screen.getByLabelText('Default Instructions')).toHaveValue('Speak clearly')
  })

  it('submits api key, speed, and instructions when filled', async () => {
    const user = userEvent.setup()
    const onSubmit = vi.fn()
    render(<EndpointForm onSubmit={onSubmit} onCancel={vi.fn()} isSaving={false} />)
    await user.type(screen.getByLabelText('Name'), 'Full EP')
    await user.type(screen.getByLabelText('Base URL'), 'https://a.com')
    await user.type(screen.getByLabelText('Models (comma-separated)'), 'tts-1')
    await user.type(screen.getByLabelText('API Key'), 'sk-test')
    await user.type(screen.getByLabelText('Default Speed'), '1.5')
    await user.type(screen.getByLabelText('Default Instructions'), 'Speak slowly')
    await user.click(screen.getByText('Create'))
    expect(onSubmit).toHaveBeenCalledWith(
      expect.objectContaining({
        api_key: 'sk-test',
        default_speed: 1.5,
        default_instructions: 'Speak slowly',
      }),
    )
  })

  it('treats NaN speed as undefined', async () => {
    const user = userEvent.setup()
    const onSubmit = vi.fn()
    render(<EndpointForm onSubmit={onSubmit} onCancel={vi.fn()} isSaving={false} />)
    await user.type(screen.getByLabelText('Name'), 'NaN EP')
    await user.type(screen.getByLabelText('Base URL'), 'https://a.com')
    await user.type(screen.getByLabelText('Models (comma-separated)'), 'tts-1')
    await user.type(screen.getByLabelText('Default Speed'), 'abc')
    await user.click(screen.getByText('Create'))
    expect(onSubmit).toHaveBeenCalledWith(
      expect.objectContaining({
        default_speed: undefined,
      }),
    )
  })

  it('sends undefined for empty optional fields', async () => {
    const user = userEvent.setup()
    const onSubmit = vi.fn()
    render(<EndpointForm onSubmit={onSubmit} onCancel={vi.fn()} isSaving={false} />)
    await user.type(screen.getByLabelText('Name'), 'Minimal')
    await user.type(screen.getByLabelText('Base URL'), 'https://a.com')
    await user.type(screen.getByLabelText('Models (comma-separated)'), 'tts-1')
    await user.click(screen.getByText('Create'))
    expect(onSubmit).toHaveBeenCalledWith(
      expect.objectContaining({
        api_key: undefined,
        default_speed: undefined,
        default_instructions: undefined,
      }),
    )
  })

  it('shows probe error when present', () => {
    mockProbe.error = 'connection refused'
    render(<EndpointForm onSubmit={vi.fn()} onCancel={vi.fn()} isSaving={false} />)
    expect(screen.getByText('Probe error: connection refused')).toBeInTheDocument()
    mockProbe.error = undefined
  })

  it('shows available voices when probe returns voices', () => {
    mockProbe.models = [{ id: 'tts-1' }]
    mockProbe.voices = [
      { id: 'alloy', name: 'Alloy' },
      { id: 'nova', name: 'Nova' },
    ]
    render(<EndpointForm onSubmit={vi.fn()} onCancel={vi.fn()} isSaving={false} />)
    expect(screen.getByText('Available voices: alloy, nova')).toBeInTheDocument()
    mockProbe.models = []
    mockProbe.voices = []
  })

  it('shows voice id as label when name is empty', () => {
    mockProbe.voices = [{ id: 'alloy', name: '' }]
    render(<EndpointForm onSubmit={vi.fn()} onCancel={vi.fn()} isSaving={false} />)
    expect(screen.getByText('Available voices: alloy')).toBeInTheDocument()
    mockProbe.voices = []
  })

  it('shows probing spinner near URL when loading', () => {
    mockProbe.loading = true
    render(<EndpointForm onSubmit={vi.fn()} onCancel={vi.fn()} isSaving={false} />)
    expect(screen.getByLabelText('Probing endpoint')).toBeInTheDocument()
    mockProbe.loading = false
  })

  it('hides probing spinner when not loading', () => {
    mockProbe.loading = false
    render(<EndpointForm onSubmit={vi.fn()} onCancel={vi.fn()} isSaving={false} />)
    expect(screen.queryByLabelText('Probing endpoint')).not.toBeInTheDocument()
  })

  it('hides model Select when no probe models available', () => {
    mockProbe.models = []
    render(<EndpointForm onSubmit={vi.fn()} onCancel={vi.fn()} isSaving={false} />)
    expect(screen.queryByTestId('model-select')).not.toBeInTheDocument()
  })

  it('shows model Select when probe returns models', () => {
    mockProbe.models = [{ id: 'tts-1' }, { id: 'tts-1-hd' }]
    render(<EndpointForm onSubmit={vi.fn()} onCancel={vi.fn()} isSaving={false} />)
    expect(screen.getByTestId('model-select')).toBeInTheDocument()
    expect(screen.getByText('Add...')).toBeInTheDocument()
    mockProbe.models = []
  })

  it('appends model from Select to empty input', () => {
    mockProbe.models = [{ id: 'tts-1' }, { id: 'tts-1-hd' }]
    render(<EndpointForm onSubmit={vi.fn()} onCancel={vi.fn()} isSaving={false} />)
    act(() => {
      selectOnValueChange?.('tts-1')
    })
    expect(screen.getByLabelText('Models (comma-separated)')).toHaveValue('tts-1')
    mockProbe.models = []
  })

  it('appends model with comma when input already has value', async () => {
    const user = userEvent.setup()
    mockProbe.models = [{ id: 'tts-1' }, { id: 'tts-1-hd' }]
    render(<EndpointForm onSubmit={vi.fn()} onCancel={vi.fn()} isSaving={false} />)
    await user.type(screen.getByLabelText('Models (comma-separated)'), 'custom-model')
    act(() => {
      selectOnValueChange?.('tts-1-hd')
    })
    expect(screen.getByLabelText('Models (comma-separated)')).toHaveValue('custom-model, tts-1-hd')
    mockProbe.models = []
  })

  it('does not duplicate model when already in input', async () => {
    const user = userEvent.setup()
    mockProbe.models = [{ id: 'tts-1' }]
    render(<EndpointForm onSubmit={vi.fn()} onCancel={vi.fn()} isSaving={false} />)
    await user.type(screen.getByLabelText('Models (comma-separated)'), 'tts-1')
    act(() => {
      selectOnValueChange?.('tts-1')
    })
    expect(screen.getByLabelText('Models (comma-separated)')).toHaveValue('tts-1')
    mockProbe.models = []
  })
})
