import { render, screen } from '@testing-library/preact'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import type { Endpoint } from '@/lib/api'
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

  it('sends undefined for empty optional fields', async () => {
    const user = userEvent.setup()
    const onSubmit = vi.fn()
    render(<EndpointForm onSubmit={onSubmit} onCancel={vi.fn()} isSaving={false} />)
    await user.type(screen.getByLabelText('Name'), 'Minimal')
    await user.type(screen.getByLabelText('Base URL'), 'https://a.com')
    await user.click(screen.getByText('Create'))
    expect(onSubmit).toHaveBeenCalledWith(
      expect.objectContaining({
        api_key: undefined,
        default_speed: undefined,
        default_instructions: undefined,
      }),
    )
  })
})
