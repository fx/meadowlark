import { render, screen } from '@testing-library/preact'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import type { ProbeStatus } from '@/hooks/use-endpoint-probe'
import type { Endpoint } from '@/lib/api'

// Mock useEndpointProbe to avoid real fetch calls and allow per-test overrides
const mockProbe = {
  models: [] as { id: string }[],
  voices: [] as { id: string; name: string }[],
  status: 'idle' as ProbeStatus,
  error: undefined as string | undefined,
  refresh: vi.fn(),
}

vi.mock('@/hooks/use-endpoint-probe', () => ({
  useEndpointProbe: () => mockProbe,
}))

import { EndpointForm } from './endpoint-form'

const mockEndpoint: Endpoint = {
  id: 'ep-1',
  name: 'Test EP',
  base_url: 'https://api.example.com',
  api_key: 'sk-123',
  models: ['tts-1', 'tts-1-hd'],
  default_voice: 'alloy',
  default_speed: 1.5,
  default_instructions: 'Speak clearly',
  default_response_format: 'wav',
  streaming_enabled: false,
  stream_sample_rate: 24000,
  enabled: true,
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-01T00:00:00Z',
}

/** Open the models combobox dropdown via the toggle button. */
async function openModelsDropdown(user: ReturnType<typeof userEvent.setup>) {
  await user.click(screen.getByRole('button', { name: 'Toggle options' }))
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

  it('renders model badges for existing endpoint', () => {
    render(
      <EndpointForm
        endpoint={mockEndpoint}
        onSubmit={vi.fn()}
        onCancel={vi.fn()}
        isSaving={false}
      />,
    )
    expect(screen.getByText('tts-1')).toBeInTheDocument()
    expect(screen.getByText('tts-1-hd')).toBeInTheDocument()
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

  it('submits form data for create with models added via combobox', async () => {
    mockProbe.models = [{ id: 'tts-1' }, { id: 'tts-2' }]
    const user = userEvent.setup()
    const onSubmit = vi.fn()
    render(<EndpointForm onSubmit={onSubmit} onCancel={vi.fn()} isSaving={false} />)
    await user.type(screen.getByLabelText('Name'), 'New EP')
    await user.type(screen.getByLabelText('Base URL'), 'https://api.test.com')
    // Open combobox and select a model from dropdown
    await openModelsDropdown(user)
    await user.click(screen.getByText('tts-1'))
    // Open again to add second model
    await openModelsDropdown(user)
    await user.click(screen.getByText('tts-2'))
    await user.click(screen.getByText('Create'))
    expect(onSubmit).toHaveBeenCalledWith(
      expect.objectContaining({
        name: 'New EP',
        base_url: 'https://api.test.com',
        models: ['tts-1', 'tts-2'],
        enabled: true,
      }),
    )
    mockProbe.models = []
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
    mockProbe.models = [{ id: 'tts-1' }]
    const user = userEvent.setup()
    const onSubmit = vi.fn()
    render(<EndpointForm onSubmit={onSubmit} onCancel={vi.fn()} isSaving={false} />)
    await user.type(screen.getByLabelText('Name'), 'Full EP')
    await user.type(screen.getByLabelText('Base URL'), 'https://a.com')
    // Add model via combobox dropdown
    await openModelsDropdown(user)
    await user.click(screen.getByText('tts-1'))
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
    mockProbe.models = []
  })

  it('treats NaN speed as undefined', async () => {
    mockProbe.models = [{ id: 'tts-1' }]
    const user = userEvent.setup()
    const onSubmit = vi.fn()
    render(<EndpointForm onSubmit={onSubmit} onCancel={vi.fn()} isSaving={false} />)
    await user.type(screen.getByLabelText('Name'), 'NaN EP')
    await user.type(screen.getByLabelText('Base URL'), 'https://a.com')
    await openModelsDropdown(user)
    await user.click(screen.getByText('tts-1'))
    await user.type(screen.getByLabelText('Default Speed'), 'abc')
    await user.click(screen.getByText('Create'))
    expect(onSubmit).toHaveBeenCalledWith(
      expect.objectContaining({
        default_speed: undefined,
      }),
    )
    mockProbe.models = []
  })

  it('sends undefined for empty optional fields', async () => {
    mockProbe.models = [{ id: 'tts-1' }]
    const user = userEvent.setup()
    const onSubmit = vi.fn()
    render(<EndpointForm onSubmit={onSubmit} onCancel={vi.fn()} isSaving={false} />)
    await user.type(screen.getByLabelText('Name'), 'Minimal')
    await user.type(screen.getByLabelText('Base URL'), 'https://a.com')
    await openModelsDropdown(user)
    await user.click(screen.getByText('tts-1'))
    await user.click(screen.getByText('Create'))
    expect(onSubmit).toHaveBeenCalledWith(
      expect.objectContaining({
        api_key: undefined,
        default_speed: undefined,
        default_instructions: undefined,
      }),
    )
    mockProbe.models = []
  })

  it('removes model badge when X is clicked', async () => {
    const user = userEvent.setup()
    render(
      <EndpointForm
        endpoint={mockEndpoint}
        onSubmit={vi.fn()}
        onCancel={vi.fn()}
        isSaving={false}
      />,
    )
    expect(screen.getByText('tts-1')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Remove tts-1' }))
    expect(screen.queryByText('tts-1')).not.toBeInTheDocument()
    // tts-1-hd should still be there
    expect(screen.getByText('tts-1-hd')).toBeInTheDocument()
  })

  it('does not add duplicate model', async () => {
    mockProbe.models = [{ id: 'tts-1' }]
    const user = userEvent.setup()
    render(
      <EndpointForm
        endpoint={mockEndpoint}
        onSubmit={vi.fn()}
        onCancel={vi.fn()}
        isSaving={false}
      />,
    )
    // tts-1 is already in selectedModels so it won't appear in dropdown options
    // (filtered out by modelOptions). Type it manually to try adding.
    await user.type(screen.getByLabelText('Models'), 'tts-1')
    // Still only one badge with tts-1 text
    const badges = screen.getAllByText('tts-1')
    expect(badges).toHaveLength(1)
    mockProbe.models = []
  })

  it('shows probe error when present', () => {
    mockProbe.error = 'connection refused'
    render(<EndpointForm onSubmit={vi.fn()} onCancel={vi.fn()} isSaving={false} />)
    expect(screen.getByText('connection refused')).toBeInTheDocument()
    mockProbe.error = undefined
  })

  it('shows default voice select when probe returns voices', () => {
    mockProbe.status = 'success'
    mockProbe.models = [{ id: 'tts-1' }]
    mockProbe.voices = [
      { id: 'alloy', name: 'Alloy' },
      { id: 'nova', name: 'Nova' },
    ]
    render(<EndpointForm onSubmit={vi.fn()} onCancel={vi.fn()} isSaving={false} />)
    expect(screen.getByText('Default Voice')).toBeInTheDocument()
    mockProbe.models = []
    mockProbe.voices = []
    mockProbe.status = 'idle'
  })

  it('does not show default voice section when no voices available', () => {
    mockProbe.voices = []
    render(<EndpointForm onSubmit={vi.fn()} onCancel={vi.fn()} isSaving={false} />)
    expect(screen.queryByText('Default Voice')).not.toBeInTheDocument()
    mockProbe.voices = []
  })

  it('shows voice id in default voice select when voice name is empty', () => {
    mockProbe.voices = [{ id: 'alloy', name: '' }]
    // Render with an endpoint that already has default_voice set to 'alloy'.
    // This makes the Select display the selected item text (v.name || v.id).
    const epWithVoice: Endpoint = {
      ...mockEndpoint,
      default_voice: 'alloy',
    }
    render(
      <EndpointForm
        endpoint={epWithVoice}
        onSubmit={vi.fn()}
        onCancel={vi.fn()}
        isSaving={false}
      />,
    )
    expect(screen.getByText('Default Voice')).toBeInTheDocument()
    // The voice id should be displayed as fallback when name is empty.
    // The Select trigger and the hidden SelectItem both render the text,
    // so use getAllByText to allow multiple matches.
    expect(screen.getAllByText('alloy').length).toBeGreaterThan(0)
    mockProbe.voices = []
  })

  it('submits selected voice as default_voice', async () => {
    mockProbe.status = 'success'
    mockProbe.voices = [
      { id: 'alloy', name: 'Alloy' },
      { id: 'nova', name: 'Nova' },
    ]
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
    // Open the select and choose "Nova"
    await user.click(screen.getByRole('combobox', { name: 'Default Voice' }))
    await user.click(screen.getByRole('option', { name: 'Nova' }))
    await user.click(screen.getByText('Update'))
    expect(onSubmit).toHaveBeenCalledWith(
      expect.objectContaining({
        default_voice: 'nova',
      }),
    )
    mockProbe.voices = []
    mockProbe.status = 'idle'
  })

  it('submits empty default_voice when None is selected', async () => {
    mockProbe.status = 'success'
    mockProbe.voices = [
      { id: 'alloy', name: 'Alloy' },
      { id: 'nova', name: 'Nova' },
    ]
    const user = userEvent.setup()
    const onSubmit = vi.fn()
    const epWithVoice: Endpoint = {
      ...mockEndpoint,
      default_voice: 'alloy',
    }
    render(
      <EndpointForm
        endpoint={epWithVoice}
        onSubmit={onSubmit}
        onCancel={vi.fn()}
        isSaving={false}
      />,
    )
    // Open the select and choose "None" to clear the default voice
    await user.click(screen.getByRole('combobox', { name: 'Default Voice' }))
    await user.click(screen.getByRole('option', { name: 'None' }))
    await user.click(screen.getByText('Update'))
    expect(onSubmit).toHaveBeenCalledWith(
      expect.objectContaining({
        default_voice: '',
      }),
    )
    mockProbe.voices = []
    mockProbe.status = 'idle'
  })

  it('shows loading placeholder in combobox when loading', () => {
    mockProbe.status = 'loading'
    render(<EndpointForm onSubmit={vi.fn()} onCancel={vi.fn()} isSaving={false} />)
    expect(screen.getByPlaceholderText('Loading...')).toBeInTheDocument()
    mockProbe.status = 'idle'
  })

  it('shows normal placeholder when not loading', () => {
    mockProbe.status = 'idle'
    render(<EndpointForm onSubmit={vi.fn()} onCancel={vi.fn()} isSaving={false} />)
    expect(screen.getByPlaceholderText('Search or type a model name')).toBeInTheDocument()
  })

  it('shows "Add another model" placeholder when models are selected', () => {
    render(
      <EndpointForm
        endpoint={mockEndpoint}
        onSubmit={vi.fn()}
        onCancel={vi.fn()}
        isSaving={false}
      />,
    )
    expect(screen.getByPlaceholderText('Add another model...')).toBeInTheDocument()
  })

  it('shows combobox dropdown when probe returns models', async () => {
    mockProbe.models = [{ id: 'tts-1' }, { id: 'tts-1-hd' }]
    const user = userEvent.setup()
    render(<EndpointForm onSubmit={vi.fn()} onCancel={vi.fn()} isSaving={false} />)
    await openModelsDropdown(user)
    expect(screen.getByText('tts-1')).toBeInTheDocument()
    expect(screen.getByText('tts-1-hd')).toBeInTheDocument()
    mockProbe.models = []
  })

  it('adds model from combobox selection', async () => {
    mockProbe.models = [{ id: 'tts-1' }, { id: 'tts-1-hd' }]
    const user = userEvent.setup()
    const onSubmit = vi.fn()
    render(<EndpointForm onSubmit={onSubmit} onCancel={vi.fn()} isSaving={false} />)
    await user.type(screen.getByLabelText('Name'), 'EP')
    await user.type(screen.getByLabelText('Base URL'), 'https://a.com')
    await openModelsDropdown(user)
    await user.click(screen.getByText('tts-1'))
    // Badge should appear
    expect(screen.getByText('tts-1')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Remove tts-1' })).toBeInTheDocument()
    mockProbe.models = []
  })

  it('requires models field for create when no models selected', () => {
    render(<EndpointForm onSubmit={vi.fn()} onCancel={vi.fn()} isSaving={false} />)
    const modelsInput = screen.getByLabelText('Models')
    expect(modelsInput).toBeRequired()
  })

  it('does not require models field for edit', () => {
    render(
      <EndpointForm
        endpoint={mockEndpoint}
        onSubmit={vi.fn()}
        onCancel={vi.fn()}
        isSaving={false}
      />,
    )
    const modelsInput = screen.getByLabelText('Models')
    expect(modelsInput).not.toBeRequired()
  })

  it('allows free-text model entry via combobox', async () => {
    const user = userEvent.setup()
    const onSubmit = vi.fn()
    render(<EndpointForm onSubmit={onSubmit} onCancel={vi.fn()} isSaving={false} />)
    await user.type(screen.getByLabelText('Name'), 'FT EP')
    await user.type(screen.getByLabelText('Base URL'), 'https://a.com')
    // Type a free-text model name (no dropdown options available)
    await user.type(screen.getByLabelText('Models'), 'custom-model')
    await user.click(screen.getByText('Create'))
    // The free-text value should be passed as models input value
    expect(onSubmit).toHaveBeenCalled()
  })

  it('ignores whitespace-only model from probe', async () => {
    // Probe returns a whitespace-only model id; selecting it should be rejected
    mockProbe.models = [{ id: ' ' }]
    const user = userEvent.setup()
    const onSubmit = vi.fn()
    render(<EndpointForm onSubmit={onSubmit} onCancel={vi.fn()} isSaving={false} />)
    await user.type(screen.getByLabelText('Name'), 'WS EP')
    await user.type(screen.getByLabelText('Base URL'), 'https://a.com')
    await openModelsDropdown(user)
    // Select the whitespace model from dropdown
    const wsOption = screen.getByRole('button', { name: '' })
    await user.click(wsOption)
    // No badge should appear for whitespace-only model
    expect(screen.queryByRole('button', { name: /^Remove / })).not.toBeInTheDocument()
    mockProbe.models = []
  })

  it('renders refresh button next to URL field', () => {
    render(<EndpointForm onSubmit={vi.fn()} onCancel={vi.fn()} isSaving={false} />)
    expect(screen.getByRole('button', { name: 'Refresh endpoint' })).toBeInTheDocument()
  })

  it('refresh button calls probe.refresh on click', async () => {
    const user = userEvent.setup()
    mockProbe.refresh.mockClear()
    render(<EndpointForm onSubmit={vi.fn()} onCancel={vi.fn()} isSaving={false} />)
    await user.click(screen.getByRole('button', { name: 'Refresh endpoint' }))
    expect(mockProbe.refresh).toHaveBeenCalledTimes(1)
  })

  it('refresh button is disabled when status is loading', () => {
    mockProbe.status = 'loading'
    render(<EndpointForm onSubmit={vi.fn()} onCancel={vi.fn()} isSaving={false} />)
    expect(screen.getByRole('button', { name: 'Refresh endpoint' })).toBeDisabled()
    mockProbe.status = 'idle'
  })

  it('refresh button is enabled when status is idle', () => {
    mockProbe.status = 'idle'
    render(<EndpointForm onSubmit={vi.fn()} onCancel={vi.fn()} isSaving={false} />)
    expect(screen.getByRole('button', { name: 'Refresh endpoint' })).toBeEnabled()
  })

  it('refresh button is enabled when status is success', () => {
    mockProbe.status = 'success'
    render(<EndpointForm onSubmit={vi.fn()} onCancel={vi.fn()} isSaving={false} />)
    expect(screen.getByRole('button', { name: 'Refresh endpoint' })).toBeEnabled()
    mockProbe.status = 'idle'
  })

  it('refresh button is enabled when status is error', () => {
    mockProbe.status = 'error'
    render(<EndpointForm onSubmit={vi.fn()} onCancel={vi.fn()} isSaving={false} />)
    expect(screen.getByRole('button', { name: 'Refresh endpoint' })).toBeEnabled()
    mockProbe.status = 'idle'
  })

  it('clears selected models when URL changes', async () => {
    const user = userEvent.setup()
    render(
      <EndpointForm
        endpoint={mockEndpoint}
        onSubmit={vi.fn()}
        onCancel={vi.fn()}
        isSaving={false}
      />,
    )
    // Models should be present initially
    expect(screen.getByText('tts-1')).toBeInTheDocument()
    expect(screen.getByText('tts-1-hd')).toBeInTheDocument()
    // Change the URL
    const urlInput = screen.getByLabelText('Base URL')
    await user.clear(urlInput)
    await user.type(urlInput, 'https://other.api.com')
    // Model badges should be cleared
    expect(screen.queryByText('tts-1')).not.toBeInTheDocument()
    expect(screen.queryByText('tts-1-hd')).not.toBeInTheDocument()
  })

  it('clears selected models when URL is emptied', async () => {
    const user = userEvent.setup()
    render(
      <EndpointForm
        endpoint={mockEndpoint}
        onSubmit={vi.fn()}
        onCancel={vi.fn()}
        isSaving={false}
      />,
    )
    expect(screen.getByText('tts-1')).toBeInTheDocument()
    const urlInput = screen.getByLabelText('Base URL')
    await user.clear(urlInput)
    expect(screen.queryByText('tts-1')).not.toBeInTheDocument()
    expect(screen.queryByText('tts-1-hd')).not.toBeInTheDocument()
  })

  it('auto-populates models when probe succeeds after URL change', async () => {
    const user = userEvent.setup()
    render(<EndpointForm onSubmit={vi.fn()} onCancel={vi.fn()} isSaving={false} />)
    // Type URL to mark dirty (useEffect sees status=idle, no auto-populate)
    await user.type(screen.getByLabelText('Base URL'), 'https://api.com')
    expect(screen.queryByText('discovered-model')).not.toBeInTheDocument()
    // Simulate probe completing: change mock then trigger re-render
    mockProbe.status = 'success'
    mockProbe.models = [{ id: 'discovered-model' }]
    mockProbe.voices = [
      { id: 'alloy', name: 'Alloy' },
      { id: 'nova', name: 'Nova' },
    ]
    // Type one more char to trigger re-render; useEffect sees status changed to 'success'
    await user.type(screen.getByLabelText('Base URL'), '/')
    expect(screen.getByText('discovered-model')).toBeInTheDocument()
    // Default voice should be auto-populated with first voice
    expect(screen.getByText('Default Voice')).toBeInTheDocument()
    mockProbe.models = []
    mockProbe.voices = []
    mockProbe.status = 'idle'
  })

  it('shows warning icon when URL is emptied after editing', async () => {
    const user = userEvent.setup()
    render(
      <EndpointForm
        endpoint={mockEndpoint}
        onSubmit={vi.fn()}
        onCancel={vi.fn()}
        isSaving={false}
      />,
    )
    const urlInput = screen.getByLabelText('Base URL')
    await user.clear(urlInput)
    expect(screen.getByTestId('icon-warning')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Refresh endpoint' })).toBeDisabled()
  })

  it('shows warning icon and message for invalid URL scheme', async () => {
    const user = userEvent.setup()
    render(<EndpointForm onSubmit={vi.fn()} onCancel={vi.fn()} isSaving={false} />)
    await user.type(screen.getByLabelText('Base URL'), 'ftp://bad.url')
    expect(screen.getByTestId('icon-warning')).toBeInTheDocument()
    expect(screen.getByText('URL must start with http:// or https://')).toBeInTheDocument()
  })

  it('does not show warning for untouched empty URL in create mode', () => {
    render(<EndpointForm onSubmit={vi.fn()} onCancel={vi.fn()} isSaving={false} />)
    expect(screen.queryByTestId('icon-warning')).not.toBeInTheDocument()
  })

  it('streaming toggle renders and submits correct value', async () => {
    mockProbe.models = [{ id: 'tts-1' }]
    const user = userEvent.setup()
    const onSubmit = vi.fn()
    render(<EndpointForm onSubmit={onSubmit} onCancel={vi.fn()} isSaving={false} />)
    await user.type(screen.getByLabelText('Name'), 'Stream EP')
    await user.type(screen.getByLabelText('Base URL'), 'https://a.com')
    await openModelsDropdown(user)
    await user.click(screen.getByText('tts-1'))
    // Enable streaming
    await user.click(screen.getByRole('switch', { name: 'Streaming' }))
    await user.click(screen.getByText('Create'))
    expect(onSubmit).toHaveBeenCalledWith(
      expect.objectContaining({
        streaming_enabled: true,
        stream_sample_rate: 24000,
      }),
    )
    mockProbe.models = []
  })

  it('sample rate input shown/hidden based on streaming toggle', async () => {
    const user = userEvent.setup()
    render(<EndpointForm onSubmit={vi.fn()} onCancel={vi.fn()} isSaving={false} />)
    // Sample rate should not be visible by default
    expect(screen.queryByLabelText('Sample Rate')).not.toBeInTheDocument()
    // Enable streaming
    await user.click(screen.getByRole('switch', { name: 'Streaming' }))
    // Sample rate should now be visible
    expect(screen.getByLabelText('Sample Rate')).toBeInTheDocument()
    expect(screen.getByLabelText('Sample Rate')).toHaveValue(24000)
    // Disable streaming
    await user.click(screen.getByRole('switch', { name: 'Streaming' }))
    // Sample rate should be hidden again
    expect(screen.queryByLabelText('Sample Rate')).not.toBeInTheDocument()
  })

  it('sample rate validation (min/max/step)', async () => {
    const user = userEvent.setup()
    render(<EndpointForm onSubmit={vi.fn()} onCancel={vi.fn()} isSaving={false} />)
    // Enable streaming to show sample rate
    await user.click(screen.getByRole('switch', { name: 'Streaming' }))
    const sampleRateInput = screen.getByLabelText('Sample Rate')
    expect(sampleRateInput).toHaveAttribute('min', '8000')
    expect(sampleRateInput).toHaveAttribute('max', '48000')
    expect(sampleRateInput).toHaveAttribute('step', '1')
  })

  it('clamps sample rate on blur', async () => {
    const user = userEvent.setup()
    render(<EndpointForm onSubmit={vi.fn()} onCancel={vi.fn()} isSaving={false} />)
    await user.click(screen.getByRole('switch', { name: 'Streaming' }))
    const sampleRateInput = screen.getByLabelText('Sample Rate')
    // Type a value below min
    await user.clear(sampleRateInput)
    await user.type(sampleRateInput, '100')
    await user.tab() // trigger blur
    expect(sampleRateInput).toHaveValue(8000)
    // Type a value above max
    await user.clear(sampleRateInput)
    await user.type(sampleRateInput, '99999')
    await user.tab()
    expect(sampleRateInput).toHaveValue(48000)
    // Type a fractional value
    await user.clear(sampleRateInput)
    await user.type(sampleRateInput, '22050.7')
    await user.tab()
    expect(sampleRateInput).toHaveValue(22051)
    // Empty value on blur should not change
    await user.clear(sampleRateInput)
    await user.tab()
    expect(sampleRateInput).toHaveValue(null)
  })

  it('submits custom sample rate when streaming is enabled', async () => {
    mockProbe.models = [{ id: 'tts-1' }]
    const user = userEvent.setup()
    const onSubmit = vi.fn()
    render(<EndpointForm onSubmit={onSubmit} onCancel={vi.fn()} isSaving={false} />)
    await user.type(screen.getByLabelText('Name'), 'Stream EP')
    await user.type(screen.getByLabelText('Base URL'), 'https://a.com')
    await openModelsDropdown(user)
    await user.click(screen.getByText('tts-1'))
    // Enable streaming
    await user.click(screen.getByRole('switch', { name: 'Streaming' }))
    // Change sample rate
    const sampleRateInput = screen.getByLabelText('Sample Rate')
    await user.clear(sampleRateInput)
    await user.type(sampleRateInput, '44100')
    await user.click(screen.getByText('Create'))
    expect(onSubmit).toHaveBeenCalledWith(
      expect.objectContaining({
        streaming_enabled: true,
        stream_sample_rate: 44100,
      }),
    )
    mockProbe.models = []
  })

  it('edit mode populates streaming fields from existing endpoint', () => {
    const streamingEndpoint: Endpoint = {
      ...mockEndpoint,
      streaming_enabled: true,
      stream_sample_rate: 16000,
    }
    render(
      <EndpointForm
        endpoint={streamingEndpoint}
        onSubmit={vi.fn()}
        onCancel={vi.fn()}
        isSaving={false}
      />,
    )
    // Streaming toggle should be checked
    expect(screen.getByRole('switch', { name: 'Streaming' })).toHaveAttribute(
      'data-state',
      'checked',
    )
    // Sample rate should be visible and populated
    expect(screen.getByLabelText('Sample Rate')).toHaveValue(16000)
  })

  it('treats zero sample rate as unset and defaults to 24000', () => {
    const zeroRateEndpoint: Endpoint = {
      ...mockEndpoint,
      streaming_enabled: true,
      stream_sample_rate: 0,
    }
    render(
      <EndpointForm
        endpoint={zeroRateEndpoint}
        onSubmit={vi.fn()}
        onCancel={vi.fn()}
        isSaving={false}
      />,
    )
    expect(screen.getByLabelText('Sample Rate')).toHaveValue(24000)
  })

  it('defaults sample rate to 24000 when input is cleared', async () => {
    mockProbe.models = [{ id: 'tts-1' }]
    const user = userEvent.setup()
    const onSubmit = vi.fn()
    render(<EndpointForm onSubmit={onSubmit} onCancel={vi.fn()} isSaving={false} />)
    await user.type(screen.getByLabelText('Name'), 'Fallback EP')
    await user.type(screen.getByLabelText('Base URL'), 'https://a.com')
    await openModelsDropdown(user)
    await user.click(screen.getByText('tts-1'))
    await user.click(screen.getByRole('switch', { name: 'Streaming' }))
    // Clear the sample rate to trigger the || 24000 fallback
    await user.clear(screen.getByLabelText('Sample Rate'))
    await user.click(screen.getByText('Create'))
    expect(onSubmit).toHaveBeenCalledWith(
      expect.objectContaining({
        streaming_enabled: true,
        stream_sample_rate: 24000,
      }),
    )
    mockProbe.models = []
  })

  it('does not submit stream_sample_rate when streaming is disabled', async () => {
    mockProbe.models = [{ id: 'tts-1' }]
    const user = userEvent.setup()
    const onSubmit = vi.fn()
    render(<EndpointForm onSubmit={onSubmit} onCancel={vi.fn()} isSaving={false} />)
    await user.type(screen.getByLabelText('Name'), 'No Stream')
    await user.type(screen.getByLabelText('Base URL'), 'https://a.com')
    await openModelsDropdown(user)
    await user.click(screen.getByText('tts-1'))
    await user.click(screen.getByText('Create'))
    expect(onSubmit).toHaveBeenCalledWith(
      expect.objectContaining({
        streaming_enabled: false,
        stream_sample_rate: undefined,
      }),
    )
    mockProbe.models = []
  })
})
