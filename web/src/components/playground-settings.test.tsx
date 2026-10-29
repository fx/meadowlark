import { act, fireEvent, render, screen, waitFor } from '@testing-library/preact'
import userEvent from '@testing-library/user-event'
import { useState } from 'preact/hooks'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { jsonResponse, makeEndpoint, makeEndpointVoice } from '@/test-fixtures'
import { resetSelectMock, selectCallbacks } from '@/test-select-mock'

vi.mock('@/components/ui/select', () => import('@/test-select-mock'))

import { endpointDefaults, type PlaygroundForm, PlaygroundSettings } from './playground-settings'

const endpoints = [
  makeEndpoint(),
  makeEndpoint({
    id: 'ep-2',
    name: 'Local',
    models: ['kokoro', 'piper'],
    default_model: '',
    default_voice: '',
    enabled: false,
  }),
]

const baseForm: PlaygroundForm = {
  endpointId: 'ep-1',
  model: 'tts-1',
  voice: 'alloy',
  speed: '',
  instructions: '',
  name: '',
  languages: 'en',
}

const fetchMock = vi.fn()

beforeEach(() => {
  fetchMock.mockImplementation((url: string) => {
    if (url === '/api/v1/endpoints/ep-1/voices') {
      return jsonResponse([
        makeEndpointVoice('alloy', { name: 'Alloy' }),
        makeEndpointVoice('nova'),
        makeEndpointVoice('echo', { name: 'Echo', enabled: false }),
      ])
    }
    return jsonResponse([])
  })
  vi.stubGlobal('fetch', fetchMock)
})

afterEach(() => {
  vi.unstubAllGlobals()
  fetchMock.mockReset()
  resetSelectMock()
})

/** Renders the panel with real state so edits flow back in. */
function Harness({
  initial = baseForm,
  aliasMode,
  onChange,
}: {
  initial?: PlaygroundForm
  aliasMode?: boolean
  onChange?: (f: PlaygroundForm) => void
}) {
  const [form, setForm] = useState(initial)
  return (
    <PlaygroundSettings
      endpoints={endpoints}
      value={form}
      aliasMode={aliasMode}
      onChange={(f) => {
        setForm(f)
        onChange?.(f)
      }}
    />
  )
}

describe('endpointDefaults', () => {
  it('prefers the default model, then the first model, and the default voice', () => {
    expect(endpointDefaults(endpoints[0])).toEqual({ model: 'tts-1', voice: 'alloy' })
    expect(endpointDefaults(endpoints[1])).toEqual({ model: 'kokoro', voice: '' })
    expect(endpointDefaults(makeEndpoint({ models: [], default_model: '' }))).toEqual({
      model: '',
      voice: 'alloy',
    })
    expect(endpointDefaults(undefined)).toEqual({ model: '', voice: '' })
  })
})

describe('PlaygroundSettings', () => {
  it('lists endpoints, marking disabled ones', () => {
    render(<Harness />)
    expect(screen.getByText('OpenAI')).toBeInTheDocument()
    expect(screen.getByText('Local (disabled)')).toBeInTheDocument()
    expect(screen.getByTestId('select-trigger-playground-endpoint')).toHaveAttribute(
      'data-value',
      'ep-1',
    )
  })

  it("lists the selected endpoint's models", () => {
    render(<Harness />)
    expect(screen.getByText('tts-1-hd')).toBeInTheDocument()
    expect(screen.queryByText('kokoro')).toBeNull()
    expect(screen.getByTestId('select-trigger-playground-model')).toHaveAttribute(
      'data-value',
      'tts-1',
    )
  })

  it('disables model and voice without an endpoint', () => {
    render(<Harness initial={{ ...baseForm, endpointId: '', model: '', voice: '' }} />)
    expect(screen.getByTestId('select-trigger-playground-model')).toHaveAttribute(
      'data-disabled',
      'true',
    )
    expect(screen.getByLabelText('Voice')).toBeDisabled()
    expect(fetchMock).not.toHaveBeenCalled()
  })

  it('resets model and voice to the new endpoint defaults when the endpoint changes', () => {
    const onChange = vi.fn()
    render(<Harness onChange={onChange} />)
    act(() => selectCallbacks['playground-endpoint']('ep-2'))
    expect(onChange).toHaveBeenLastCalledWith({
      ...baseForm,
      endpointId: 'ep-2',
      model: 'kokoro',
      voice: '',
    })
    expect(screen.getByText('piper')).toBeInTheDocument()
  })

  it('changes the model', () => {
    const onChange = vi.fn()
    render(<Harness onChange={onChange} />)
    act(() => selectCallbacks['playground-model']('tts-1-hd'))
    expect(onChange).toHaveBeenLastCalledWith({ ...baseForm, model: 'tts-1-hd' })
  })

  it("suggests the endpoint's enabled voices", async () => {
    const user = userEvent.setup()
    render(<Harness />)
    await waitFor(() =>
      expect(fetchMock).toHaveBeenCalledWith('/api/v1/endpoints/ep-1/voices', expect.anything()),
    )
    await user.click(await screen.findByRole('button', { name: 'Toggle options' }))
    expect(screen.getByRole('button', { name: 'Alloy' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'nova' })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Echo' })).toBeNull()
  })

  it('accepts a free-text voice', () => {
    const onChange = vi.fn()
    render(<Harness onChange={onChange} />)
    fireEvent.input(screen.getByLabelText('Voice'), { target: { value: 'clone:me' } })
    expect(onChange).toHaveBeenLastCalledWith({ ...baseForm, voice: 'clone:me' })
  })

  it('aborts the voice lookup when the endpoint changes, ignoring its late answer', async () => {
    let resolveFirst: (v: unknown) => void = () => {}
    fetchMock.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          resolveFirst = resolve
        }),
    )
    render(<Harness />)
    const firstSignal = fetchMock.mock.calls[0][1].signal as AbortSignal
    act(() => selectCallbacks['playground-endpoint']('ep-2'))
    expect(firstSignal.aborted).toBe(true)
    await act(async () => {
      resolveFirst({
        ok: true,
        status: 200,
        json: () => Promise.resolve([makeEndpointVoice('stale')]),
      })
      await new Promise((r) => setTimeout(r, 0))
    })
    expect(screen.queryByRole('button', { name: 'Toggle options' })).toBeNull()
    expect(fetchMock).toHaveBeenLastCalledWith('/api/v1/endpoints/ep-2/voices', expect.anything())
  })

  it('treats a failed voice lookup as no suggestions', async () => {
    fetchMock.mockImplementationOnce(() => Promise.reject(new Error('offline')))
    render(<Harness />)
    await waitFor(() =>
      expect(screen.getByLabelText('Voice')).toHaveAttribute('placeholder', 'Voice id'),
    )
    expect(screen.queryByRole('button', { name: 'Toggle options' })).toBeNull()
  })

  it('edits speed and instructions', () => {
    const onChange = vi.fn()
    render(<Harness onChange={onChange} />)
    fireEvent.input(screen.getByLabelText('Speed'), { target: { value: '1.25' } })
    expect(onChange).toHaveBeenLastCalledWith({ ...baseForm, speed: '1.25' })
    fireEvent.input(screen.getByLabelText('Instructions'), { target: { value: 'calm' } })
    expect(onChange).toHaveBeenLastCalledWith({ ...baseForm, speed: '1.25', instructions: 'calm' })
    expect(screen.getByLabelText('Speed')).toHaveAttribute('placeholder', 'Endpoint default')
  })

  it('hides the alias fields outside alias mode', () => {
    render(<Harness />)
    expect(screen.queryByLabelText('Name')).toBeNull()
  })

  it('edits the alias name and languages in alias mode', () => {
    const onChange = vi.fn()
    render(<Harness aliasMode onChange={onChange} />)
    fireEvent.input(screen.getByLabelText('Name'), { target: { value: 'Butler' } })
    expect(onChange).toHaveBeenLastCalledWith({ ...baseForm, name: 'Butler' })
    fireEvent.input(screen.getByLabelText('Languages (comma-separated)'), {
      target: { value: 'en, de' },
    })
    expect(onChange).toHaveBeenLastCalledWith({ ...baseForm, name: 'Butler', languages: 'en, de' })
  })
})
