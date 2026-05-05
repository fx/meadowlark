import { render, screen, waitFor } from '@testing-library/preact'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { ProbeStatus } from '@/hooks/use-endpoint-probe'
import type { Endpoint, EndpointVoice } from '@/lib/api'

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

// Per-test endpoint-voices state and API mocks. The component calls
// api.endpoints.voices.{list,refresh,setEnabled} — each test can stub
// these via the helpers below.
const voicesState = {
  list: [] as EndpointVoice[],
  refresh: [] as EndpointVoice[],
  listShouldFail: false,
  setEnabledShouldFail: false,
  refreshError: undefined as string | undefined,
  setEnabledCalls: [] as Array<{ id: string; voiceId: string; enabled: boolean }>,
}

vi.mock('@/lib/api', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api')>('@/lib/api')
  return {
    ...actual,
    api: {
      ...actual.api,
      endpoints: {
        ...actual.api.endpoints,
        voices: {
          list: vi.fn(async (_id: string) => {
            if (voicesState.listShouldFail) throw new Error('list failed')
            return voicesState.list
          }),
          refresh: vi.fn(async (_id: string) => {
            if (voicesState.refreshError) throw new Error(voicesState.refreshError)
            return voicesState.refresh
          }),
          setEnabled: vi.fn(async (id: string, voiceId: string, enabled: boolean) => {
            voicesState.setEnabledCalls.push({ id, voiceId, enabled })
            if (voicesState.setEnabledShouldFail) throw new Error('toggle failed')
            return {
              endpoint_id: id,
              voice_id: voiceId,
              name: '',
              enabled,
              created_at: '',
              updated_at: '',
            }
          }),
        },
      },
    },
  }
})

import { EndpointForm } from './endpoint-form'

function makeVoice(voiceId: string, enabled = false, name = ''): EndpointVoice {
  return {
    endpoint_id: 'ep-1',
    voice_id: voiceId,
    name,
    enabled,
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
  }
}

beforeEach(() => {
  voicesState.list = []
  voicesState.refresh = []
  voicesState.listShouldFail = false
  voicesState.setEnabledShouldFail = false
  voicesState.refreshError = undefined
  voicesState.setEnabledCalls = []
})

const mockEndpoint: Endpoint = {
  id: 'ep-1',
  name: 'Test EP',
  base_url: 'https://api.example.com',
  api_key: 'sk-123',
  models: ['tts-1', 'tts-1-hd'],
  default_model: 'tts-1',
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

function resetProbe() {
  mockProbe.models = []
  mockProbe.voices = []
  mockProbe.status = 'idle'
  mockProbe.error = undefined
}

describe('EndpointForm', () => {
  it('renders empty form for create mode', () => {
    resetProbe()
    render(<EndpointForm onSubmit={vi.fn()} onCancel={vi.fn()} isSaving={false} />)
    expect(screen.getByLabelText('Name')).toHaveValue('')
    expect(screen.getByLabelText('Base URL')).toHaveValue('')
    expect(screen.getByText('Create')).toBeInTheDocument()
  })

  it('renders populated form for edit mode', () => {
    resetProbe()
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

  it('renders model rows for existing endpoint', () => {
    resetProbe()
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
    expect(screen.getByLabelText('Set tts-1 as default')).toBeChecked()
  })

  it('shows Saving... when isSaving is true', () => {
    resetProbe()
    render(<EndpointForm onSubmit={vi.fn()} onCancel={vi.fn()} isSaving={true} />)
    expect(screen.getByText('Saving...')).toBeInTheDocument()
  })

  it('calls onCancel when Cancel is clicked', async () => {
    resetProbe()
    const user = userEvent.setup()
    const onCancel = vi.fn()
    render(<EndpointForm onSubmit={vi.fn()} onCancel={onCancel} isSaving={false} />)
    await user.click(screen.getByText('Cancel'))
    expect(onCancel).toHaveBeenCalled()
  })

  it('toggles API key visibility', async () => {
    resetProbe()
    const user = userEvent.setup()
    render(<EndpointForm onSubmit={vi.fn()} onCancel={vi.fn()} isSaving={false} />)
    const keyInput = screen.getByLabelText('API Key')
    expect(keyInput).toHaveAttribute('type', 'password')
    await user.click(screen.getByRole('button', { name: 'Show API key' }))
    expect(keyInput).toHaveAttribute('type', 'text')
    await user.click(screen.getByRole('button', { name: 'Hide API key' }))
    expect(keyInput).toHaveAttribute('type', 'password')
  })

  it('empty form: probe surfaces models all toggled off, no default selected, submit disabled', () => {
    resetProbe()
    mockProbe.models = [{ id: 'tts-1' }, { id: 'tts-1-hd' }, { id: 'gpt-4o-mini-tts' }]
    mockProbe.status = 'success'
    render(<EndpointForm onSubmit={vi.fn()} onCancel={vi.fn()} isSaving={false} />)
    // All switches are off (data-state=unchecked)
    expect(screen.getByRole('switch', { name: 'Enable tts-1' })).toHaveAttribute(
      'data-state',
      'unchecked',
    )
    expect(screen.getByRole('switch', { name: 'Enable tts-1-hd' })).toHaveAttribute(
      'data-state',
      'unchecked',
    )
    // No default selected
    expect(screen.getByLabelText('Set tts-1 as default')).not.toBeChecked()
    // All radios are disabled (since all switches are off)
    expect(screen.getByLabelText('Set tts-1 as default')).toBeDisabled()
    // Submit disabled
    expect(screen.getByRole('button', { name: 'Create' })).toBeDisabled()
  })

  it('enabling first model auto-selects it as default', async () => {
    resetProbe()
    mockProbe.models = [{ id: 'tts-1' }, { id: 'tts-1-hd' }]
    mockProbe.status = 'success'
    const user = userEvent.setup()
    render(<EndpointForm onSubmit={vi.fn()} onCancel={vi.fn()} isSaving={false} />)
    expect(screen.getByLabelText('Set tts-1-hd as default')).not.toBeChecked()
    await user.click(screen.getByRole('switch', { name: 'Enable tts-1-hd' }))
    expect(screen.getByLabelText('Set tts-1-hd as default')).toBeChecked()
    expect(screen.getByLabelText('Set tts-1-hd as default')).not.toBeDisabled()
  })

  it('disabling current default moves default to next enabled in display order', async () => {
    resetProbe()
    mockProbe.models = [{ id: 'tts-1' }, { id: 'tts-1-hd' }]
    mockProbe.status = 'success'
    const user = userEvent.setup()
    render(<EndpointForm onSubmit={vi.fn()} onCancel={vi.fn()} isSaving={false} />)
    // Enable both, in order
    await user.click(screen.getByRole('switch', { name: 'Enable tts-1' }))
    await user.click(screen.getByRole('switch', { name: 'Enable tts-1-hd' }))
    // Default is tts-1 (auto-selected on first enable)
    expect(screen.getByLabelText('Set tts-1 as default')).toBeChecked()
    // Disable tts-1
    await user.click(screen.getByRole('switch', { name: 'Enable tts-1' }))
    // Default should shift to tts-1-hd
    expect(screen.getByLabelText('Set tts-1-hd as default')).toBeChecked()
    expect(screen.getByLabelText('Set tts-1 as default')).not.toBeChecked()
  })

  it('disabling a non-default model leaves the default unchanged', async () => {
    resetProbe()
    mockProbe.models = [{ id: 'tts-1' }, { id: 'tts-1-hd' }]
    mockProbe.status = 'success'
    const user = userEvent.setup()
    render(<EndpointForm onSubmit={vi.fn()} onCancel={vi.fn()} isSaving={false} />)
    await user.click(screen.getByRole('switch', { name: 'Enable tts-1' }))
    await user.click(screen.getByRole('switch', { name: 'Enable tts-1-hd' }))
    expect(screen.getByLabelText('Set tts-1 as default')).toBeChecked()
    // Disable the non-default model.
    await user.click(screen.getByRole('switch', { name: 'Enable tts-1-hd' }))
    // Default stays on tts-1.
    expect(screen.getByLabelText('Set tts-1 as default')).toBeChecked()
  })

  it('disabling the last enabled model clears the default', async () => {
    resetProbe()
    mockProbe.models = [{ id: 'tts-1' }]
    mockProbe.status = 'success'
    const user = userEvent.setup()
    render(<EndpointForm onSubmit={vi.fn()} onCancel={vi.fn()} isSaving={false} />)
    await user.click(screen.getByRole('switch', { name: 'Enable tts-1' }))
    expect(screen.getByLabelText('Set tts-1 as default')).toBeChecked()
    await user.click(screen.getByRole('switch', { name: 'Enable tts-1' }))
    expect(screen.getByLabelText('Set tts-1 as default')).not.toBeChecked()
    // Submit disabled (no enabled models)
    expect(screen.getByRole('button', { name: 'Create' })).toBeDisabled()
  })

  it('operator can switch the default to another enabled model via radio', async () => {
    resetProbe()
    mockProbe.models = [{ id: 'tts-1' }, { id: 'tts-1-hd' }]
    mockProbe.status = 'success'
    const user = userEvent.setup()
    const onSubmit = vi.fn()
    render(<EndpointForm onSubmit={onSubmit} onCancel={vi.fn()} isSaving={false} />)
    // Name and URL first; typing into URL clears selectedModels.
    await user.type(screen.getByLabelText('Name'), 'EP')
    await user.type(screen.getByLabelText('Base URL'), 'https://a.com')
    await user.click(screen.getByRole('switch', { name: 'Enable tts-1' }))
    await user.click(screen.getByRole('switch', { name: 'Enable tts-1-hd' }))
    await user.click(screen.getByLabelText('Set tts-1-hd as default'))
    expect(screen.getByLabelText('Set tts-1-hd as default')).toBeChecked()
    await user.click(screen.getByText('Create'))
    expect(onSubmit).toHaveBeenCalledWith(
      expect.objectContaining({
        models: ['tts-1', 'tts-1-hd'],
        default_model: 'tts-1-hd',
      }),
    )
  })

  it('Enabled switch is sibling of API key, not Default Speed', () => {
    resetProbe()
    render(<EndpointForm onSubmit={vi.fn()} onCancel={vi.fn()} isSaving={false} />)
    const enabledSwitch = screen.getByRole('switch', { name: 'Enabled' })
    const connection = enabledSwitch.closest('[data-section="connection"]')
    expect(connection).not.toBeNull()
    // API key input should also be inside the connection section
    expect(connection?.contains(screen.getByLabelText('API Key'))).toBe(true)
    // Default Speed input should NOT be inside the connection section
    expect(connection?.contains(screen.getByLabelText('Default Speed'))).toBe(false)
  })

  it('persisted-but-undiscovered models still appear so the operator can disable them', () => {
    // Probe returns nothing, but the endpoint already has persisted models.
    resetProbe()
    render(
      <EndpointForm
        endpoint={mockEndpoint}
        onSubmit={vi.fn()}
        onCancel={vi.fn()}
        isSaving={false}
      />,
    )
    expect(screen.getByRole('switch', { name: 'Enable tts-1' })).toHaveAttribute(
      'data-state',
      'checked',
    )
    expect(screen.getByRole('switch', { name: 'Enable tts-1-hd' })).toHaveAttribute(
      'data-state',
      'checked',
    )
  })

  it('submits form data for update', async () => {
    resetProbe()
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
        default_model: 'tts-1',
      }),
    )
  })

  it('populates speed and instructions from endpoint', () => {
    resetProbe()
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
    resetProbe()
    mockProbe.models = [{ id: 'tts-1' }]
    mockProbe.status = 'success'
    const user = userEvent.setup()
    const onSubmit = vi.fn()
    render(<EndpointForm onSubmit={onSubmit} onCancel={vi.fn()} isSaving={false} />)
    await user.type(screen.getByLabelText('Name'), 'Full EP')
    await user.type(screen.getByLabelText('Base URL'), 'https://a.com')
    await user.click(screen.getByRole('switch', { name: 'Enable tts-1' }))
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
    resetProbe()
    mockProbe.models = [{ id: 'tts-1' }]
    mockProbe.status = 'success'
    const user = userEvent.setup()
    const onSubmit = vi.fn()
    render(<EndpointForm onSubmit={onSubmit} onCancel={vi.fn()} isSaving={false} />)
    await user.type(screen.getByLabelText('Name'), 'NaN EP')
    await user.type(screen.getByLabelText('Base URL'), 'https://a.com')
    await user.click(screen.getByRole('switch', { name: 'Enable tts-1' }))
    await user.type(screen.getByLabelText('Default Speed'), 'abc')
    await user.click(screen.getByText('Create'))
    expect(onSubmit).toHaveBeenCalledWith(
      expect.objectContaining({
        default_speed: undefined,
      }),
    )
  })

  it('sends undefined for empty optional fields', async () => {
    resetProbe()
    mockProbe.models = [{ id: 'tts-1' }]
    mockProbe.status = 'success'
    const user = userEvent.setup()
    const onSubmit = vi.fn()
    render(<EndpointForm onSubmit={onSubmit} onCancel={vi.fn()} isSaving={false} />)
    await user.type(screen.getByLabelText('Name'), 'Minimal')
    await user.type(screen.getByLabelText('Base URL'), 'https://a.com')
    await user.click(screen.getByRole('switch', { name: 'Enable tts-1' }))
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
    resetProbe()
    mockProbe.error = 'connection refused'
    render(<EndpointForm onSubmit={vi.fn()} onCancel={vi.fn()} isSaving={false} />)
    expect(screen.getByText('connection refused')).toBeInTheDocument()
  })

  it('shows default voice radios on enabled voice rows', async () => {
    resetProbe()
    voicesState.list = [makeVoice('alloy', true, 'Alloy'), makeVoice('nova', true, 'Nova')]
    const epWithVoice: Endpoint = { ...mockEndpoint, default_voice: 'alloy' }
    render(
      <EndpointForm
        endpoint={epWithVoice}
        onSubmit={vi.fn()}
        onCancel={vi.fn()}
        isSaving={false}
      />,
    )
    await waitFor(() => {
      expect(screen.getByRole('radio', { name: 'Set alloy as default voice' })).toBeInTheDocument()
    })
    expect(screen.getByRole('radio', { name: 'Set alloy as default voice' })).toBeChecked()
    expect(screen.getByRole('radio', { name: 'Set nova as default voice' })).not.toBeChecked()
  })

  it('renders no default voice radios in create mode (no rows)', () => {
    resetProbe()
    render(<EndpointForm onSubmit={vi.fn()} onCancel={vi.fn()} isSaving={false} />)
    expect(screen.queryByRole('radio', { name: /default voice/ })).not.toBeInTheDocument()
  })

  it('enabling first voice auto-selects it as default', async () => {
    resetProbe()
    voicesState.list = [makeVoice('alloy', false, 'Alloy'), makeVoice('nova', false, 'Nova')]
    const user = userEvent.setup()
    const epNoVoice: Endpoint = { ...mockEndpoint, default_voice: '' }
    render(
      <EndpointForm endpoint={epNoVoice} onSubmit={vi.fn()} onCancel={vi.fn()} isSaving={false} />,
    )
    await waitFor(() => {
      expect(screen.getByRole('switch', { name: 'Enable voice nova' })).toBeInTheDocument()
    })
    expect(screen.getByRole('radio', { name: 'Set nova as default voice' })).not.toBeChecked()
    await user.click(screen.getByRole('switch', { name: 'Enable voice nova' }))
    expect(screen.getByRole('radio', { name: 'Set nova as default voice' })).toBeChecked()
  })

  it('disabling current default voice moves default to next enabled in display order', async () => {
    resetProbe()
    // alloy default, echo also enabled.
    voicesState.list = [makeVoice('alloy', true, 'Alloy'), makeVoice('echo', true, 'Echo')]
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
    await waitFor(() => {
      expect(screen.getByRole('switch', { name: 'Enable voice alloy' })).toBeInTheDocument()
    })
    expect(screen.getByRole('radio', { name: 'Set alloy as default voice' })).toBeChecked()
    // Disable alloy → default moves to echo.
    await user.click(screen.getByRole('switch', { name: 'Enable voice alloy' }))
    expect(screen.getByRole('radio', { name: 'Set echo as default voice' })).toBeChecked()
    await user.click(screen.getByRole('button', { name: 'Update' }))
    expect(onSubmit).toHaveBeenCalledWith(expect.objectContaining({ default_voice: 'echo' }))
  })

  it('disabling the last enabled voice clears default_voice', async () => {
    resetProbe()
    voicesState.list = [makeVoice('alloy', true, 'Alloy')]
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
    await waitFor(() => {
      expect(screen.getByRole('switch', { name: 'Enable voice alloy' })).toBeInTheDocument()
    })
    await user.click(screen.getByRole('switch', { name: 'Enable voice alloy' }))
    expect(screen.getByRole('radio', { name: 'Set alloy as default voice' })).not.toBeChecked()
    expect(screen.getByRole('radio', { name: 'Set alloy as default voice' })).toBeDisabled()
    await user.click(screen.getByRole('button', { name: 'Update' }))
    expect(onSubmit).toHaveBeenCalledWith(expect.objectContaining({ default_voice: '' }))
  })

  it('submits selected voice as default_voice via radio', async () => {
    resetProbe()
    voicesState.list = [
      makeVoice('alloy', true, 'Alloy'),
      makeVoice('echo', true, 'Echo'),
      makeVoice('nova', true, 'Nova'),
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
    await waitFor(() => {
      expect(screen.getByRole('radio', { name: 'Set echo as default voice' })).toBeInTheDocument()
    })
    await user.click(screen.getByRole('radio', { name: 'Set echo as default voice' }))
    expect(screen.getByRole('radio', { name: 'Set echo as default voice' })).toBeChecked()
    await user.click(screen.getByText('Update'))
    expect(onSubmit).toHaveBeenCalledWith(expect.objectContaining({ default_voice: 'echo' }))
  })

  it('submits empty default_voice when no voices are enabled', async () => {
    resetProbe()
    voicesState.list = [makeVoice('alloy', false, 'Alloy')]
    const user = userEvent.setup()
    const onSubmit = vi.fn()
    const epNoVoice: Endpoint = { ...mockEndpoint, default_voice: '' }
    render(
      <EndpointForm endpoint={epNoVoice} onSubmit={onSubmit} onCancel={vi.fn()} isSaving={false} />,
    )
    await waitFor(() => {
      expect(screen.getByRole('switch', { name: 'Enable voice alloy' })).toBeInTheDocument()
    })
    expect(screen.getByRole('radio', { name: 'Set alloy as default voice' })).toBeDisabled()
    await user.click(screen.getByText('Update'))
    expect(onSubmit).toHaveBeenCalledWith(expect.objectContaining({ default_voice: '' }))
  })

  it('renders default voice radio even when name is empty', async () => {
    resetProbe()
    voicesState.list = [makeVoice('alloy', true, '')]
    const epWithVoice: Endpoint = { ...mockEndpoint, default_voice: 'alloy' }
    render(
      <EndpointForm
        endpoint={epWithVoice}
        onSubmit={vi.fn()}
        onCancel={vi.fn()}
        isSaving={false}
      />,
    )
    await waitFor(() => {
      expect(screen.getByRole('radio', { name: 'Set alloy as default voice' })).toBeInTheDocument()
    })
    expect(screen.getByRole('radio', { name: 'Set alloy as default voice' })).toBeChecked()
  })

  it('shows model toggle list rows when probe returns models', () => {
    resetProbe()
    mockProbe.models = [{ id: 'tts-1' }, { id: 'tts-1-hd' }]
    mockProbe.status = 'success'
    render(<EndpointForm onSubmit={vi.fn()} onCancel={vi.fn()} isSaving={false} />)
    expect(screen.getByRole('switch', { name: 'Enable tts-1' })).toBeInTheDocument()
    expect(screen.getByRole('switch', { name: 'Enable tts-1-hd' })).toBeInTheDocument()
  })

  it('shows discovering placeholder while loading and no models surfaced', () => {
    resetProbe()
    mockProbe.status = 'loading'
    render(<EndpointForm onSubmit={vi.fn()} onCancel={vi.fn()} isSaving={false} />)
    expect(screen.getByText('Discovering models...')).toBeInTheDocument()
  })

  it('shows empty placeholder when no models discovered and not loading', () => {
    resetProbe()
    render(<EndpointForm onSubmit={vi.fn()} onCancel={vi.fn()} isSaving={false} />)
    expect(
      screen.getByText('No models discovered yet. Probe the base URL above.'),
    ).toBeInTheDocument()
  })

  it('renders refresh button next to URL field', () => {
    resetProbe()
    render(<EndpointForm onSubmit={vi.fn()} onCancel={vi.fn()} isSaving={false} />)
    expect(screen.getByRole('button', { name: 'Refresh endpoint' })).toBeInTheDocument()
  })

  it('refresh button calls probe.refresh on click', async () => {
    resetProbe()
    const user = userEvent.setup()
    mockProbe.refresh.mockClear()
    render(<EndpointForm onSubmit={vi.fn()} onCancel={vi.fn()} isSaving={false} />)
    await user.click(screen.getByRole('button', { name: 'Refresh endpoint' }))
    expect(mockProbe.refresh).toHaveBeenCalledTimes(1)
  })

  it('refresh button is disabled when status is loading', () => {
    resetProbe()
    mockProbe.status = 'loading'
    render(<EndpointForm onSubmit={vi.fn()} onCancel={vi.fn()} isSaving={false} />)
    expect(screen.getByRole('button', { name: 'Refresh endpoint' })).toBeDisabled()
  })

  it('refresh button is enabled when status is idle', () => {
    resetProbe()
    render(<EndpointForm onSubmit={vi.fn()} onCancel={vi.fn()} isSaving={false} />)
    expect(screen.getByRole('button', { name: 'Refresh endpoint' })).toBeEnabled()
  })

  it('refresh button is enabled when status is success', () => {
    resetProbe()
    mockProbe.status = 'success'
    render(<EndpointForm onSubmit={vi.fn()} onCancel={vi.fn()} isSaving={false} />)
    expect(screen.getByRole('button', { name: 'Refresh endpoint' })).toBeEnabled()
  })

  it('refresh button is enabled when status is error', () => {
    resetProbe()
    mockProbe.status = 'error'
    render(<EndpointForm onSubmit={vi.fn()} onCancel={vi.fn()} isSaving={false} />)
    expect(screen.getByRole('button', { name: 'Refresh endpoint' })).toBeEnabled()
  })

  it('clears selected models, default_model, and default_voice when URL changes', async () => {
    resetProbe()
    const user = userEvent.setup()
    render(
      <EndpointForm
        endpoint={mockEndpoint}
        onSubmit={vi.fn()}
        onCancel={vi.fn()}
        isSaving={false}
      />,
    )
    expect(screen.getByRole('switch', { name: 'Enable tts-1' })).toHaveAttribute(
      'data-state',
      'checked',
    )
    const urlInput = screen.getByLabelText('Base URL')
    await user.clear(urlInput)
    await user.type(urlInput, 'https://other.api.com')
    // Persisted rows are gone since selectedModels was cleared and probe returned nothing.
    expect(screen.queryByRole('switch', { name: 'Enable tts-1' })).not.toBeInTheDocument()
  })

  it('discovered models render in toggle list after probe surfaces them', async () => {
    resetProbe()
    const user = userEvent.setup()
    render(<EndpointForm onSubmit={vi.fn()} onCancel={vi.fn()} isSaving={false} />)
    await user.type(screen.getByLabelText('Base URL'), 'https://api.com')
    mockProbe.status = 'success'
    mockProbe.models = [{ id: 'discovered-model' }]
    mockProbe.voices = []
    await user.type(screen.getByLabelText('Base URL'), '/')
    expect(screen.getByRole('switch', { name: 'Enable discovered-model' })).toBeInTheDocument()
  })

  it('shows warning icon when URL is emptied after editing', async () => {
    resetProbe()
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
    resetProbe()
    const user = userEvent.setup()
    render(<EndpointForm onSubmit={vi.fn()} onCancel={vi.fn()} isSaving={false} />)
    await user.type(screen.getByLabelText('Base URL'), 'ftp://bad.url')
    expect(screen.getByTestId('icon-warning')).toBeInTheDocument()
    expect(screen.getByText('URL must start with http:// or https://')).toBeInTheDocument()
  })

  it('does not show warning for untouched empty URL in create mode', () => {
    resetProbe()
    render(<EndpointForm onSubmit={vi.fn()} onCancel={vi.fn()} isSaving={false} />)
    expect(screen.queryByTestId('icon-warning')).not.toBeInTheDocument()
  })

  it('streaming toggle renders and submits correct value', async () => {
    resetProbe()
    mockProbe.models = [{ id: 'tts-1' }]
    mockProbe.status = 'success'
    const user = userEvent.setup()
    const onSubmit = vi.fn()
    render(<EndpointForm onSubmit={onSubmit} onCancel={vi.fn()} isSaving={false} />)
    await user.type(screen.getByLabelText('Name'), 'Stream EP')
    await user.type(screen.getByLabelText('Base URL'), 'https://a.com')
    await user.click(screen.getByRole('switch', { name: 'Enable tts-1' }))
    await user.click(screen.getByRole('switch', { name: 'Streaming' }))
    await user.click(screen.getByText('Create'))
    expect(onSubmit).toHaveBeenCalledWith(
      expect.objectContaining({
        streaming_enabled: true,
        stream_sample_rate: 24000,
      }),
    )
  })

  it('sample rate input shown/hidden based on streaming toggle', async () => {
    resetProbe()
    const user = userEvent.setup()
    render(<EndpointForm onSubmit={vi.fn()} onCancel={vi.fn()} isSaving={false} />)
    expect(screen.queryByLabelText('Sample Rate')).not.toBeInTheDocument()
    await user.click(screen.getByRole('switch', { name: 'Streaming' }))
    expect(screen.getByLabelText('Sample Rate')).toBeInTheDocument()
    expect(screen.getByLabelText('Sample Rate')).toHaveValue(24000)
    await user.click(screen.getByRole('switch', { name: 'Streaming' }))
    expect(screen.queryByLabelText('Sample Rate')).not.toBeInTheDocument()
  })

  it('sample rate validation (min/max/step)', async () => {
    resetProbe()
    const user = userEvent.setup()
    render(<EndpointForm onSubmit={vi.fn()} onCancel={vi.fn()} isSaving={false} />)
    await user.click(screen.getByRole('switch', { name: 'Streaming' }))
    const sampleRateInput = screen.getByLabelText('Sample Rate')
    expect(sampleRateInput).toHaveAttribute('min', '8000')
    expect(sampleRateInput).toHaveAttribute('max', '48000')
    expect(sampleRateInput).toHaveAttribute('step', '1')
  })

  it('clamps sample rate on blur', async () => {
    resetProbe()
    const user = userEvent.setup()
    render(<EndpointForm onSubmit={vi.fn()} onCancel={vi.fn()} isSaving={false} />)
    await user.click(screen.getByRole('switch', { name: 'Streaming' }))
    const sampleRateInput = screen.getByLabelText('Sample Rate')
    await user.clear(sampleRateInput)
    await user.type(sampleRateInput, '100')
    await user.tab()
    expect(sampleRateInput).toHaveValue(8000)
    await user.clear(sampleRateInput)
    await user.type(sampleRateInput, '99999')
    await user.tab()
    expect(sampleRateInput).toHaveValue(48000)
    await user.clear(sampleRateInput)
    await user.type(sampleRateInput, '22050.7')
    await user.tab()
    expect(sampleRateInput).toHaveValue(22051)
    await user.clear(sampleRateInput)
    await user.tab()
    expect(sampleRateInput).toHaveValue(null)
  })

  it('submits custom sample rate when streaming is enabled', async () => {
    resetProbe()
    mockProbe.models = [{ id: 'tts-1' }]
    mockProbe.status = 'success'
    const user = userEvent.setup()
    const onSubmit = vi.fn()
    render(<EndpointForm onSubmit={onSubmit} onCancel={vi.fn()} isSaving={false} />)
    await user.type(screen.getByLabelText('Name'), 'Stream EP')
    await user.type(screen.getByLabelText('Base URL'), 'https://a.com')
    await user.click(screen.getByRole('switch', { name: 'Enable tts-1' }))
    await user.click(screen.getByRole('switch', { name: 'Streaming' }))
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
  })

  it('edit mode populates streaming fields from existing endpoint', () => {
    resetProbe()
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
    expect(screen.getByRole('switch', { name: 'Streaming' })).toHaveAttribute(
      'data-state',
      'checked',
    )
    expect(screen.getByLabelText('Sample Rate')).toHaveValue(16000)
  })

  it('treats zero sample rate as unset and defaults to 24000', () => {
    resetProbe()
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
    resetProbe()
    mockProbe.models = [{ id: 'tts-1' }]
    mockProbe.status = 'success'
    const user = userEvent.setup()
    const onSubmit = vi.fn()
    render(<EndpointForm onSubmit={onSubmit} onCancel={vi.fn()} isSaving={false} />)
    await user.type(screen.getByLabelText('Name'), 'Fallback EP')
    await user.type(screen.getByLabelText('Base URL'), 'https://a.com')
    await user.click(screen.getByRole('switch', { name: 'Enable tts-1' }))
    await user.click(screen.getByRole('switch', { name: 'Streaming' }))
    await user.clear(screen.getByLabelText('Sample Rate'))
    await user.click(screen.getByText('Create'))
    expect(onSubmit).toHaveBeenCalledWith(
      expect.objectContaining({
        streaming_enabled: true,
        stream_sample_rate: 24000,
      }),
    )
  })

  // --- Voices toggle list (change 0005) ---

  it('voices section: shows hint for unsaved endpoints', () => {
    resetProbe()
    render(<EndpointForm onSubmit={vi.fn()} onCancel={vi.fn()} isSaving={false} />)
    expect(
      screen.getByText('Voices become enable-able after saving the endpoint.'),
    ).toBeInTheDocument()
  })

  it('voices section: shows empty state when endpoint exists but has no voices', async () => {
    resetProbe()
    voicesState.list = []
    render(
      <EndpointForm
        endpoint={mockEndpoint}
        onSubmit={vi.fn()}
        onCancel={vi.fn()}
        isSaving={false}
      />,
    )
    await waitFor(() => {
      expect(screen.getByText('No voices discovered yet — click Refresh')).toBeInTheDocument()
    })
  })

  it('voices section: refresh populates rows', async () => {
    resetProbe()
    voicesState.list = []
    voicesState.refresh = [makeVoice('alloy', false, 'Alloy'), makeVoice('echo', false, 'Echo')]
    const user = userEvent.setup()
    render(
      <EndpointForm
        endpoint={mockEndpoint}
        onSubmit={vi.fn()}
        onCancel={vi.fn()}
        isSaving={false}
      />,
    )
    await waitFor(() => {
      expect(
        screen.getByRole('button', { name: 'Refresh voices from endpoint' }),
      ).toBeInTheDocument()
    })
    await user.click(screen.getByRole('button', { name: 'Refresh voices from endpoint' }))
    await waitFor(() => {
      expect(screen.getByRole('switch', { name: 'Enable voice alloy' })).toBeInTheDocument()
    })
    expect(screen.getByRole('switch', { name: 'Enable voice echo' })).toBeInTheDocument()
  })

  it('voices section: refresh shows error message on failure', async () => {
    resetProbe()
    voicesState.refreshError = 'upstream offline'
    const user = userEvent.setup()
    render(
      <EndpointForm
        endpoint={mockEndpoint}
        onSubmit={vi.fn()}
        onCancel={vi.fn()}
        isSaving={false}
      />,
    )
    await waitFor(() => {
      expect(
        screen.getByRole('button', { name: 'Refresh voices from endpoint' }),
      ).toBeInTheDocument()
    })
    await user.click(screen.getByRole('button', { name: 'Refresh voices from endpoint' }))
    await waitFor(() => {
      expect(screen.getByText('upstream offline')).toBeInTheDocument()
    })
  })

  it('voices section: toggling one voice does not affect siblings', async () => {
    resetProbe()
    voicesState.list = [makeVoice('alloy', false), makeVoice('echo', true)]
    const user = userEvent.setup()
    render(
      <EndpointForm
        endpoint={mockEndpoint}
        onSubmit={vi.fn()}
        onCancel={vi.fn()}
        isSaving={false}
      />,
    )
    await waitFor(() => {
      expect(screen.getByRole('switch', { name: 'Enable voice alloy' })).toBeInTheDocument()
    })
    await user.click(screen.getByRole('switch', { name: 'Enable voice alloy' }))
    expect(screen.getByRole('switch', { name: 'Enable voice alloy' })).toHaveAttribute(
      'data-state',
      'checked',
    )
    // Sibling 'echo' remains checked.
    expect(screen.getByRole('switch', { name: 'Enable voice echo' })).toHaveAttribute(
      'data-state',
      'checked',
    )
  })

  it('voices section: optimistic toggle on flips Switch immediately', async () => {
    resetProbe()
    voicesState.list = [makeVoice('alloy', false, 'Alloy')]
    const user = userEvent.setup()
    render(
      <EndpointForm
        endpoint={mockEndpoint}
        onSubmit={vi.fn()}
        onCancel={vi.fn()}
        isSaving={false}
      />,
    )
    await waitFor(() => {
      expect(screen.getByRole('switch', { name: 'Enable voice alloy' })).toBeInTheDocument()
    })
    const sw = screen.getByRole('switch', { name: 'Enable voice alloy' })
    expect(sw).toHaveAttribute('data-state', 'unchecked')
    await user.click(sw)
    expect(sw).toHaveAttribute('data-state', 'checked')
    await waitFor(() => {
      expect(voicesState.setEnabledCalls).toContainEqual({
        id: 'ep-1',
        voiceId: 'alloy',
        enabled: true,
      })
    })
  })

  it('voices section: optimistic toggle off flips Switch immediately', async () => {
    resetProbe()
    voicesState.list = [makeVoice('alloy', true, 'Alloy')]
    const user = userEvent.setup()
    render(
      <EndpointForm
        endpoint={mockEndpoint}
        onSubmit={vi.fn()}
        onCancel={vi.fn()}
        isSaving={false}
      />,
    )
    await waitFor(() => {
      const sw = screen.getByRole('switch', { name: 'Enable voice alloy' })
      expect(sw).toHaveAttribute('data-state', 'checked')
    })
    await user.click(screen.getByRole('switch', { name: 'Enable voice alloy' }))
    expect(screen.getByRole('switch', { name: 'Enable voice alloy' })).toHaveAttribute(
      'data-state',
      'unchecked',
    )
  })

  it('voices section: rolls back on API error and surfaces message; sibling rows untouched', async () => {
    resetProbe()
    // Two voices: alloy starts disabled (the one we toggle and that fails),
    // echo starts enabled (a sibling that must NOT be reverted by the rollback).
    voicesState.list = [makeVoice('alloy', false, 'Alloy'), makeVoice('echo', true, 'Echo')]
    voicesState.setEnabledShouldFail = true
    const user = userEvent.setup()
    render(
      <EndpointForm
        endpoint={mockEndpoint}
        onSubmit={vi.fn()}
        onCancel={vi.fn()}
        isSaving={false}
      />,
    )
    await waitFor(() => {
      expect(screen.getByRole('switch', { name: 'Enable voice alloy' })).toBeInTheDocument()
    })
    await user.click(screen.getByRole('switch', { name: 'Enable voice alloy' }))
    await waitFor(() => {
      expect(screen.getByText('toggle failed')).toBeInTheDocument()
    })
    expect(screen.getByRole('switch', { name: 'Enable voice alloy' })).toHaveAttribute(
      'data-state',
      'unchecked',
    )
    // Sibling echo MUST remain checked (per-voice rollback, not array-wide).
    expect(screen.getByRole('switch', { name: 'Enable voice echo' })).toHaveAttribute(
      'data-state',
      'checked',
    )
  })

  it('voices section: list failure is non-fatal — empty state still renders', async () => {
    resetProbe()
    voicesState.listShouldFail = true
    render(
      <EndpointForm
        endpoint={mockEndpoint}
        onSubmit={vi.fn()}
        onCancel={vi.fn()}
        isSaving={false}
      />,
    )
    await waitFor(() => {
      expect(screen.getByText('No voices discovered yet — click Refresh')).toBeInTheDocument()
    })
  })

  it('voices section: disabling a non-default voice leaves default_voice intact', async () => {
    resetProbe()
    // alloy is enabled and is the persisted default; echo is enabled but not default.
    voicesState.list = [makeVoice('alloy', true, 'Alloy'), makeVoice('echo', true, 'Echo')]
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
    await waitFor(() => {
      expect(screen.getByRole('switch', { name: 'Enable voice echo' })).toBeInTheDocument()
    })
    // Toggle echo (non-default) off — should NOT clear default_voice.
    await user.click(screen.getByRole('switch', { name: 'Enable voice echo' }))
    await user.click(screen.getByRole('button', { name: 'Update' }))
    expect(onSubmit).toHaveBeenCalledWith(expect.objectContaining({ default_voice: 'alloy' }))
  })

  it('voices section: disabling the current default voice rollback restores switch state', async () => {
    resetProbe()
    voicesState.list = [makeVoice('alloy', true, 'Alloy')]
    voicesState.setEnabledShouldFail = true
    const user = userEvent.setup()
    render(
      <EndpointForm
        endpoint={mockEndpoint}
        onSubmit={vi.fn()}
        onCancel={vi.fn()}
        isSaving={false}
      />,
    )
    await waitFor(() => {
      expect(screen.getByRole('switch', { name: 'Enable voice alloy' })).toBeInTheDocument()
    })
    await user.click(screen.getByRole('switch', { name: 'Enable voice alloy' }))
    // After failed setEnabled, rollback restores the switch state.
    await waitFor(() => {
      expect(screen.getByText('toggle failed')).toBeInTheDocument()
    })
    expect(screen.getByRole('switch', { name: 'Enable voice alloy' })).toHaveAttribute(
      'data-state',
      'checked',
    )
  })

  it('voices section: row shows name when distinct from voice id', async () => {
    resetProbe()
    voicesState.list = [makeVoice('alloy', false, 'Alloy')]
    render(
      <EndpointForm
        endpoint={mockEndpoint}
        onSubmit={vi.fn()}
        onCancel={vi.fn()}
        isSaving={false}
      />,
    )
    await waitFor(() => {
      expect(screen.getByText('alloy')).toBeInTheDocument()
    })
    expect(screen.getByText('Alloy')).toBeInTheDocument()
  })

  // --- Probe-driven voice refresh + Models Refresh button ---

  it('voices section: URL edit followed by probe success triggers voices.refresh for saved endpoint', async () => {
    resetProbe()
    voicesState.list = [makeVoice('alloy', false, 'Alloy')]
    voicesState.refresh = [makeVoice('alloy', false, 'Alloy'), makeVoice('nova', false, 'Nova')]
    const user = userEvent.setup()
    render(
      <EndpointForm
        endpoint={mockEndpoint}
        onSubmit={vi.fn()}
        onCancel={vi.fn()}
        isSaving={false}
      />,
    )
    await waitFor(() => {
      expect(screen.getByRole('switch', { name: 'Enable voice alloy' })).toBeInTheDocument()
    })
    expect(screen.queryByRole('switch', { name: 'Enable voice nova' })).not.toBeInTheDocument()
    mockProbe.status = 'success'
    await user.type(screen.getByLabelText('Base URL'), '/')
    await waitFor(() => {
      expect(screen.getByRole('switch', { name: 'Enable voice nova' })).toBeInTheDocument()
    })
  })

  it('voices section: probe success on initial mount does NOT redundantly call voices.refresh', async () => {
    resetProbe()
    voicesState.list = [makeVoice('alloy', false, 'Alloy')]
    voicesState.refresh = [makeVoice('alloy', false, 'Alloy'), makeVoice('nova', false, 'Nova')]
    mockProbe.status = 'success'
    render(
      <EndpointForm
        endpoint={mockEndpoint}
        onSubmit={vi.fn()}
        onCancel={vi.fn()}
        isSaving={false}
      />,
    )
    await waitFor(() => {
      expect(screen.getByRole('switch', { name: 'Enable voice alloy' })).toBeInTheDocument()
    })
    expect(screen.queryByRole('switch', { name: 'Enable voice nova' })).not.toBeInTheDocument()
  })

  it('voices section: Models Refresh button click invokes probe.refresh', async () => {
    resetProbe()
    voicesState.list = [makeVoice('alloy', false, 'Alloy')]
    mockProbe.status = 'success'
    mockProbe.refresh.mockClear()
    const user = userEvent.setup()
    render(
      <EndpointForm
        endpoint={mockEndpoint}
        onSubmit={vi.fn()}
        onCancel={vi.fn()}
        isSaving={false}
      />,
    )
    await waitFor(() => {
      expect(screen.getByRole('switch', { name: 'Enable voice alloy' })).toBeInTheDocument()
    })
    await user.click(screen.getByRole('button', { name: 'Refresh' }))
    expect(mockProbe.refresh).toHaveBeenCalledTimes(1)
  })

  it('voices section: URL-edit-driven refresh failure surfaces error', async () => {
    resetProbe()
    voicesState.list = [makeVoice('alloy', false, 'Alloy')]
    voicesState.refreshError = 'upstream offline'
    const user = userEvent.setup()
    render(
      <EndpointForm
        endpoint={mockEndpoint}
        onSubmit={vi.fn()}
        onCancel={vi.fn()}
        isSaving={false}
      />,
    )
    await waitFor(() => {
      expect(screen.getByRole('switch', { name: 'Enable voice alloy' })).toBeInTheDocument()
    })
    mockProbe.status = 'success'
    await user.type(screen.getByLabelText('Base URL'), '/')
    await waitFor(() => {
      expect(screen.getByText('upstream offline')).toBeInTheDocument()
    })
  })

  it('voices section: unsaved endpoint surfaces probe.voices as disabled preview rows', async () => {
    resetProbe()
    mockProbe.voices = [{ id: 'alloy', name: 'Alloy' }]
    mockProbe.status = 'success'
    render(<EndpointForm onSubmit={vi.fn()} onCancel={vi.fn()} isSaving={false} />)
    expect(
      screen.getByText('Voices become enable-able after saving the endpoint.'),
    ).toBeInTheDocument()
    const sw = screen.getByRole('switch', { name: 'Enable voice alloy' })
    expect(sw).toBeInTheDocument()
    expect(sw).toBeDisabled()
  })

  it('models section: Refresh button calls probe.refresh', async () => {
    resetProbe()
    const user = userEvent.setup()
    mockProbe.refresh.mockClear()
    render(<EndpointForm onSubmit={vi.fn()} onCancel={vi.fn()} isSaving={false} />)
    await user.type(screen.getByLabelText('Base URL'), 'https://api.example.com')
    const modelsSection = screen.getByText('Models').closest('section') as HTMLElement
    const btn = modelsSection.querySelector('button')
    expect(btn).not.toBeNull()
    expect(btn).toHaveTextContent('Refresh')
    mockProbe.refresh.mockClear()
    await user.click(btn as HTMLButtonElement)
    expect(mockProbe.refresh).toHaveBeenCalled()
  })

  it('does not submit stream_sample_rate when streaming is disabled', async () => {
    resetProbe()
    mockProbe.models = [{ id: 'tts-1' }]
    mockProbe.status = 'success'
    const user = userEvent.setup()
    const onSubmit = vi.fn()
    render(<EndpointForm onSubmit={onSubmit} onCancel={vi.fn()} isSaving={false} />)
    await user.type(screen.getByLabelText('Name'), 'No Stream')
    await user.type(screen.getByLabelText('Base URL'), 'https://a.com')
    await user.click(screen.getByRole('switch', { name: 'Enable tts-1' }))
    await user.click(screen.getByText('Create'))
    expect(onSubmit).toHaveBeenCalledWith(
      expect.objectContaining({
        streaming_enabled: false,
        stream_sample_rate: undefined,
      }),
    )
  })
})
