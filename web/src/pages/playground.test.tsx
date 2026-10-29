import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/preact'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { Route, Router } from 'wouter'
import { memoryLocation } from 'wouter/memory-location'
import { clearCache } from '@/hooks/use-fetch'
import type { VoiceAlias } from '@/lib/api'
import { addTake, getTakesState, resetTakes } from '@/lib/takes'
import { buildWav, makeNewTake, sineWav } from '@/test-audio'
import { jsonResponse, makeAlias, makeEndpoint } from '@/test-fixtures'
import { resetSelectMock, selectCallbacks } from '@/test-select-mock'

vi.mock('@/components/ui/select', () => import('@/test-select-mock'))
const invalidateCache = vi.hoisted(() => vi.fn())
vi.mock('@/hooks/use-fetch', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/hooks/use-fetch')>()
  return {
    ...actual,
    invalidateCache: invalidateCache.mockImplementation(actual.invalidateCache),
  }
})

import { PlaygroundPage } from './playground'

const endpoints = [
  makeEndpoint({ id: 'ep-0', name: 'Old', enabled: false, models: ['m0'], default_model: '' }),
  makeEndpoint(),
  makeEndpoint({
    id: 'ep-2',
    name: 'Local',
    models: ['kokoro', 'piper'],
    default_model: 'kokoro',
    default_voice: 'af_sky',
  }),
]

type Handler = (url: string, init?: RequestInit) => Promise<Response> | undefined
let overrides: Handler[]
let fetchMock: ReturnType<typeof vi.fn>
let aliases: Record<string, VoiceAlias>

function wavResponse(buf: ArrayBuffer = sineWav()) {
  return Promise.resolve({
    ok: true,
    status: 200,
    arrayBuffer: () => Promise.resolve(buf),
  } as Response)
}

function errorResponse(status: number, code: string, message: string) {
  return jsonResponse({ error: { code, message } }, status)
}

function defaultFetch(url: string, init?: RequestInit): Promise<Response> {
  const method = init?.method ?? 'GET'
  if (url === '/api/v1/endpoints') return jsonResponse(endpoints)
  if (/^\/api\/v1\/endpoints\/[^/]+\/voices$/.test(url)) return jsonResponse([])
  if (url === '/api/v1/synthesize') return wavResponse()
  const alias = /^\/api\/v1\/aliases\/([^/]+)$/.exec(url)
  if (alias && method === 'GET') {
    const found = aliases[alias[1]]
    return found ? jsonResponse(found) : errorResponse(404, 'not_found', 'alias not found')
  }
  if (alias && method === 'PUT') {
    const body = JSON.parse(String(init?.body))
    return jsonResponse({ ...aliases[alias[1]], ...body })
  }
  if (url === '/api/v1/aliases' && method === 'POST') {
    const created = makeAlias({ ...JSON.parse(String(init?.body)), id: 'alias-new' })
    aliases[created.id] = created
    return jsonResponse(created, 201)
  }
  return jsonResponse({})
}

function lastBody(url: string, method = 'POST') {
  const call = fetchMock.mock.calls.filter(
    ([u, init]) => u === url && (init?.method ?? 'GET') === method,
  )
  return JSON.parse(String(call[call.length - 1][1].body))
}

beforeEach(() => {
  clearCache()
  resetTakes()
  overrides = []
  aliases = {
    'alias-1': makeAlias({ speed: 1.2, instructions: 'warm', languages: ['en', 'fr'] }),
    'alias-2': makeAlias({ id: 'alias-2', name: 'Butler', enabled: false, voice: 'onyx' }),
  }
  fetchMock = vi.fn((input: string, init?: RequestInit) => {
    for (const h of overrides) {
      const res = h(input, init)
      if (res) return res
    }
    return defaultFetch(input, init)
  })
  vi.stubGlobal('fetch', fetchMock)
})

afterEach(() => {
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
  invalidateCache.mockClear()
  resetSelectMock()
  clearCache()
})

function renderAt(path: string) {
  const location = memoryLocation({ path, record: true })
  const utils = render(
    <Router hook={location.hook}>
      <Route path="/playground/:aliasId?" component={PlaygroundPage} />
    </Router>,
  )
  return { ...utils, location }
}

const trigger = (id: string) => screen.getByTestId(`select-trigger-${id}`)
const generateButton = () => screen.getByRole('button', { name: 'Generate' })
const textArea = () => screen.getByLabelText('Text') as HTMLTextAreaElement

async function ready() {
  await screen.findByRole('button', { name: 'Generate' })
}

async function generate() {
  fireEvent.click(generateButton())
  await screen.findByRole('region', { name: /^Take #/ })
}

describe('PlaygroundPage — blank mode', () => {
  it('shows loading, then defaults to the first enabled endpoint', async () => {
    renderAt('/playground')
    expect(screen.getByText('Loading playground...')).toBeInTheDocument()
    await ready()
    expect(screen.getByRole('heading', { name: 'Playground' })).toBeInTheDocument()
    expect(trigger('playground-endpoint')).toHaveAttribute('data-value', 'ep-1')
    expect(trigger('playground-model')).toHaveAttribute('data-value', 'tts-1')
    expect(screen.getByLabelText('Voice')).toHaveValue('alloy')
    expect(textArea()).toHaveValue('Hello, this is a test.')
    expect(screen.getByTestId('char-count')).toHaveTextContent('22 / 4096')
    expect(screen.getByText('Generate a take to hear it here.')).toBeInTheDocument()
    expect(screen.queryByLabelText('Name')).toBeNull()
  })

  it('falls back to the first endpoint when none is enabled', async () => {
    overrides.push((url) =>
      url === '/api/v1/endpoints' ? jsonResponse([endpoints[0]]) : undefined,
    )
    renderAt('/playground')
    await ready()
    expect(trigger('playground-endpoint')).toHaveAttribute('data-value', 'ep-0')
    expect(trigger('playground-model')).toHaveAttribute('data-value', 'm0')
  })

  it('presets endpoint, model, and voice from the query', async () => {
    renderAt('/playground?endpoint=ep-2&model=piper&voice=clone%3Ame')
    await ready()
    expect(trigger('playground-endpoint')).toHaveAttribute('data-value', 'ep-2')
    expect(trigger('playground-model')).toHaveAttribute('data-value', 'piper')
    expect(screen.getByLabelText('Voice')).toHaveValue('clone:me')
  })

  it("uses the endpoint's defaults for an unknown model or a missing voice", async () => {
    renderAt('/playground?endpoint=ep-2&model=nope')
    await ready()
    expect(trigger('playground-model')).toHaveAttribute('data-value', 'kokoro')
    expect(screen.getByLabelText('Voice')).toHaveValue('af_sky')
  })

  it('ignores an unknown endpoint in the query', async () => {
    renderAt('/playground?endpoint=missing&model=piper&voice=x')
    await ready()
    expect(trigger('playground-endpoint')).toHaveAttribute('data-value', 'ep-1')
    expect(trigger('playground-model')).toHaveAttribute('data-value', 'tts-1')
    expect(screen.getByLabelText('Voice')).toHaveValue('alloy')
  })

  it('asks for an endpoint when there is none', async () => {
    overrides.push((url) => (url === '/api/v1/endpoints' ? jsonResponse([]) : undefined))
    renderAt('/playground')
    expect(await screen.findByText(/No endpoints configured/)).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Add one' })).toHaveAttribute('href', '/endpoints')
  })

  it('asks for an endpoint when the endpoint list is null', async () => {
    overrides.push((url) => (url === '/api/v1/endpoints' ? jsonResponse(null) : undefined))
    renderAt('/playground')
    expect(await screen.findByText(/No endpoints configured/)).toBeInTheDocument()
  })

  it('shows an endpoint load error', async () => {
    overrides.push((url) =>
      url === '/api/v1/endpoints' ? errorResponse(500, 'internal_error', 'db down') : undefined,
    )
    renderAt('/playground')
    expect(await screen.findByText('Error: db down')).toBeInTheDocument()
  })
})

describe('PlaygroundPage — generating', () => {
  it('synthesizes, adds a take, shows its details, and auto-plays it', async () => {
    vi.spyOn(performance, 'now').mockReturnValueOnce(1000).mockReturnValueOnce(2240)
    renderAt('/playground')
    await ready()
    await generate()

    expect(lastBody('/api/v1/synthesize')).toEqual({
      endpoint_id: 'ep-1',
      model: 'tts-1',
      voice: 'alloy',
      text: 'Hello, this is a test.',
    })
    const take = getTakesState().takes[0]
    expect(take.params).toEqual({
      endpointId: 'ep-1',
      endpointName: 'OpenAI',
      model: 'tts-1',
      voice: 'alloy',
      speed: undefined,
      instructions: undefined,
    })
    expect(take.url).toMatch(/^blob:mock\//)
    expect(take.peaks.peak).toHaveLength(200)
    expect(take.miniPeaks.peak).toHaveLength(64)
    expect(
      screen.getByText(/24 kHz · 16-bit · mono · 0\.10 s · \d+ KB · 1\.24 s latency/),
    ).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: 'Takes (1)' })).toBeInTheDocument()
    await waitFor(() => expect(HTMLMediaElement.prototype.play).toHaveBeenCalled())
  })

  it('sends speed and instructions only when set', async () => {
    renderAt('/playground')
    await ready()
    fireEvent.input(screen.getByLabelText('Speed'), { target: { value: '1.5' } })
    fireEvent.input(screen.getByLabelText('Instructions'), { target: { value: '  ' } })
    await generate()
    expect(lastBody('/api/v1/synthesize')).toMatchObject({ speed: 1.5 })
    expect(lastBody('/api/v1/synthesize')).not.toHaveProperty('instructions')

    fireEvent.input(screen.getByLabelText('Instructions'), { target: { value: 'whisper' } })
    fireEvent.click(generateButton())
    await waitFor(() => expect(getTakesState().takes).toHaveLength(2))
    expect(lastBody('/api/v1/synthesize')).toMatchObject({ speed: 1.5, instructions: 'whisper' })
    expect(getTakesState().takes[0].params).toMatchObject({ speed: 1.5, instructions: 'whisper' })
  })

  it('shows the API error', async () => {
    overrides.push((url) =>
      url === '/api/v1/synthesize'
        ? errorResponse(502, 'upstream_error', 'synthesis failed: unknown voice')
        : undefined,
    )
    renderAt('/playground')
    await ready()
    fireEvent.click(generateButton())
    expect(await screen.findByRole('alert')).toHaveTextContent('synthesis failed: unknown voice')
    expect(getTakesState().takes).toHaveLength(0)
  })

  it('reports a non-Error rejection as text', async () => {
    overrides.push((url) =>
      url === '/api/v1/synthesize' ? Promise.reject('socket closed') : undefined,
    )
    renderAt('/playground')
    await ready()
    fireEvent.click(generateButton())
    expect(await screen.findByRole('alert')).toHaveTextContent('socket closed')
  })

  it('reports audio it cannot decode', async () => {
    overrides.push((url) =>
      url === '/api/v1/synthesize' ? wavResponse(buildWav({ audioFormat: 3 })) : undefined,
    )
    renderAt('/playground')
    await ready()
    fireEvent.click(generateButton())
    expect(await screen.findByRole('alert')).toHaveTextContent('Could not decode audio')
    expect(getTakesState().takes).toHaveLength(0)
    expect(generateButton()).toBeEnabled()
  })

  it('cancels an in-flight request without an error', async () => {
    overrides.push((url, init) =>
      url === '/api/v1/synthesize'
        ? new Promise((_, reject) => {
            init?.signal?.addEventListener('abort', () =>
              reject(new DOMException('aborted', 'AbortError')),
            )
          })
        : undefined,
    )
    renderAt('/playground')
    await ready()
    fireEvent.click(generateButton())
    const cancel = await screen.findByRole('button', { name: 'Cancel' })
    expect(screen.getByText('Generating…')).toBeInTheDocument()
    fireEvent.click(cancel)
    await ready()
    expect(screen.queryByRole('alert')).toBeNull()
    expect(getTakesState().takes).toHaveLength(0)
  })

  it('generates on Ctrl+Enter and Cmd+Enter, not on Enter alone', async () => {
    renderAt('/playground')
    await ready()
    fireEvent.keyDown(textArea(), { key: 'Enter' })
    expect(fetchMock).not.toHaveBeenCalledWith('/api/v1/synthesize', expect.anything())
    fireEvent.keyDown(textArea(), { key: 'Enter', ctrlKey: true })
    await waitFor(() => expect(getTakesState().takes).toHaveLength(1))
    fireEvent.keyDown(textArea(), { key: 'Enter', metaKey: true })
    await waitFor(() => expect(getTakesState().takes).toHaveLength(2))
  })

  it('blocks text over the limit', async () => {
    renderAt('/playground')
    await ready()
    fireEvent.input(textArea(), { target: { value: '😀'.repeat(4097) } })
    expect(screen.getByTestId('char-count')).toHaveTextContent('4097 / 4096')
    expect(screen.getByTestId('char-count')).toHaveClass('text-destructive')
    expect(textArea()).toHaveAttribute('aria-invalid', 'true')
    expect(generateButton()).toBeDisabled()
    fireEvent.keyDown(textArea(), { key: 'Enter', ctrlKey: true })
    expect(fetchMock).not.toHaveBeenCalledWith('/api/v1/synthesize', expect.anything())

    fireEvent.input(textArea(), { target: { value: '😀'.repeat(4096) } })
    expect(screen.getByTestId('char-count')).toHaveTextContent('4096 / 4096')
    expect(generateButton()).toBeEnabled()
  })

  it('needs text and a voice', async () => {
    renderAt('/playground')
    await ready()
    fireEvent.input(textArea(), { target: { value: '   ' } })
    expect(generateButton()).toBeDisabled()
    fireEvent.input(textArea(), { target: { value: 'Hi' } })
    fireEvent.input(screen.getByLabelText('Voice'), { target: { value: ' ' } })
    expect(generateButton()).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Save as alias' })).toBeDisabled()
  })
})

describe('PlaygroundPage — takes', () => {
  it('plays a selected take in the main player', async () => {
    const older = addTake(makeNewTake({ text: 'older take' }))
    addTake(makeNewTake({ text: 'newer take' }))
    renderAt('/playground')
    await ready()
    expect(screen.getByRole('region', { name: 'Take #2' })).toBeInTheDocument()
    expect(HTMLMediaElement.prototype.play).not.toHaveBeenCalled()

    fireEvent.click(screen.getByRole('button', { name: 'Play take #1' }))
    expect(screen.getByRole('region', { name: 'Take #1' })).toBeInTheDocument()
    expect(getTakesState().selectedId).toBe(older.id)
    expect(HTMLMediaElement.prototype.play).toHaveBeenCalled()
  })

  it('replays the selected take when it is clicked again', async () => {
    addTake(makeNewTake({ text: 'only take' }))
    renderAt('/playground')
    await ready()
    fireEvent.click(screen.getByRole('button', { name: 'Play take #1' }))
    expect(HTMLMediaElement.prototype.play).toHaveBeenCalledOnce()
    fireEvent.click(screen.getByRole('button', { name: 'Play take #1' }))
    expect(HTMLMediaElement.prototype.play).toHaveBeenCalledTimes(2)
  })

  it('restores a take’s settings and text into the panel', async () => {
    addTake(
      makeNewTake({
        params: {
          endpointId: 'ep-2',
          endpointName: 'Local',
          model: 'piper',
          voice: 'clone:x',
          speed: 0.9,
          instructions: 'slow',
        },
        text: 'restore me',
      }),
    )
    addTake(makeNewTake())
    renderAt('/playground')
    await ready()

    fireEvent.click(screen.getByRole('button', { name: 'Restore settings from take #1' }))
    expect(trigger('playground-endpoint')).toHaveAttribute('data-value', 'ep-2')
    expect(trigger('playground-model')).toHaveAttribute('data-value', 'piper')
    expect(screen.getByLabelText('Voice')).toHaveValue('clone:x')
    expect(screen.getByLabelText('Speed')).toHaveValue(0.9)
    expect(screen.getByLabelText('Instructions')).toHaveValue('slow')
    expect(textArea()).toHaveValue('restore me')

    fireEvent.click(screen.getByRole('button', { name: 'Restore settings from take #2' }))
    expect(screen.getByLabelText('Speed')).toHaveValue(null)
    expect(screen.getByLabelText('Instructions')).toHaveValue('')
  })

  it('removes and clears takes', async () => {
    addTake(makeNewTake())
    addTake(makeNewTake())
    renderAt('/playground')
    await ready()
    fireEvent.click(screen.getByRole('button', { name: 'Remove take #2' }))
    expect(screen.getByRole('heading', { name: 'Takes (1)' })).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Clear' }))
    expect(screen.queryByRole('heading', { name: /Takes/ })).toBeNull()
    expect(screen.getByText('Generate a take to hear it here.')).toBeInTheDocument()
  })
})

describe('PlaygroundPage — saving from blank mode', () => {
  it('saves the panel as a new alias and opens it', async () => {
    const { location } = renderAt('/playground')
    await ready()
    fireEvent.input(screen.getByLabelText('Instructions'), { target: { value: 'cheerful' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save as alias' }))

    const dialog = await screen.findByRole('dialog')
    expect(within(dialog).getByRole('heading', { name: 'Save as alias' })).toBeInTheDocument()
    expect(within(dialog).getByLabelText('Languages (comma-separated)')).toHaveValue('en')
    expect(within(dialog).getByRole('button', { name: 'Save' })).toBeDisabled()

    fireEvent.input(within(dialog).getByLabelText('Name'), { target: { value: '  Butler ' } })
    fireEvent.input(within(dialog).getByLabelText('Languages (comma-separated)'), {
      target: { value: 'en, de,' },
    })
    fireEvent.click(within(dialog).getByRole('button', { name: 'Save' }))

    await waitFor(() => expect(location.history).toContain('/playground/alias-new'))
    expect(lastBody('/api/v1/aliases')).toEqual({
      name: 'Butler',
      endpoint_id: 'ep-1',
      model: 'tts-1',
      voice: 'alloy',
      instructions: 'cheerful',
      languages: ['en', 'de'],
    })
    expect(invalidateCache).toHaveBeenCalledWith('/api/v1/aliases')
    expect(invalidateCache).toHaveBeenCalledWith('/api/v1/voices')
    expect(await screen.findByText('Editing alias')).toBeInTheDocument()
    expect(screen.queryByRole('dialog')).toBeNull()
  })

  it('shows a save error inside the dialog and keeps it open', async () => {
    overrides.push((url, init) =>
      url === '/api/v1/aliases' && init?.method === 'POST'
        ? errorResponse(409, 'conflict', 'alias with this name already exists')
        : undefined,
    )
    const { location } = renderAt('/playground')
    await ready()
    fireEvent.click(screen.getByRole('button', { name: 'Save as alias' }))
    const dialog = await screen.findByRole('dialog')
    fireEvent.input(within(dialog).getByLabelText('Name'), { target: { value: 'Narrator' } })
    fireEvent.submit(within(dialog).getByLabelText('Name').closest('form') as HTMLFormElement)
    expect(
      await within(dialog).findByText('alias with this name already exists'),
    ).toBeInTheDocument()
    expect(location.history).toEqual(['/playground'])
  })

  it('closes the dialog on Cancel and on Escape', async () => {
    renderAt('/playground')
    await ready()
    fireEvent.click(screen.getByRole('button', { name: 'Save as alias' }))
    fireEvent.click(
      within(await screen.findByRole('dialog')).getByRole('button', { name: 'Cancel' }),
    )
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())

    fireEvent.click(screen.getByRole('button', { name: 'Save as alias' }))
    fireEvent.keyDown(await screen.findByRole('dialog'), { key: 'Escape' })
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
  })
})

describe('PlaygroundPage — alias mode', () => {
  it('loads the alias into the panel', async () => {
    renderAt('/playground/alias-1')
    await ready()
    expect(screen.getByText('Editing alias')).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: 'Narrator' })).toBeInTheDocument()
    expect(trigger('playground-endpoint')).toHaveAttribute('data-value', 'ep-1')
    expect(trigger('playground-model')).toHaveAttribute('data-value', 'tts-1-hd')
    expect(screen.getByLabelText('Voice')).toHaveValue('nova')
    expect(screen.getByLabelText('Speed')).toHaveValue(1.2)
    expect(screen.getByLabelText('Instructions')).toHaveValue('warm')
    expect(screen.getByLabelText('Name')).toHaveValue('Narrator')
    expect(screen.getByLabelText('Languages (comma-separated)')).toHaveValue('en, fr')
    expect(screen.queryByText('disabled')).toBeNull()
  })

  it('marks a disabled alias', async () => {
    renderAt('/playground/alias-2')
    await ready()
    expect(screen.getByRole('heading', { name: /Butler/ })).toHaveTextContent('disabled')
    expect(screen.getByLabelText('Speed')).toHaveValue(null)
    expect(screen.getByLabelText('Instructions')).toHaveValue('')
  })

  it('says when the alias does not exist', async () => {
    renderAt('/playground/missing')
    expect(await screen.findByText('Alias not found')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Open a blank playground' })).toHaveAttribute(
      'href',
      '/playground',
    )
  })

  it('shows other alias load errors', async () => {
    overrides.push((url) =>
      url === '/api/v1/aliases/alias-1' ? errorResponse(500, 'internal_error', 'boom') : undefined,
    )
    renderAt('/playground/alias-1')
    expect(await screen.findByText('Error: boom')).toBeInTheDocument()
  })

  it('shows an endpoint load error', async () => {
    overrides.push((url) =>
      url === '/api/v1/endpoints' ? errorResponse(500, 'internal_error', 'db down') : undefined,
    )
    renderAt('/playground/alias-1')
    expect(await screen.findByText('Error: db down')).toBeInTheDocument()
  })

  it('saves changes, enabling Save only when the panel differs', async () => {
    renderAt('/playground/alias-1')
    await ready()
    const save = screen.getByRole('button', { name: 'Save' })
    expect(save).toBeDisabled()

    fireEvent.input(screen.getByLabelText('Languages (comma-separated)'), {
      target: { value: 'en,fr' },
    })
    expect(save).toBeDisabled()

    fireEvent.input(screen.getByLabelText('Voice'), { target: { value: 'shimmer' } })
    expect(save).toBeEnabled()
    fireEvent.input(screen.getByLabelText('Name'), { target: { value: ' ' } })
    expect(save).toBeDisabled()
    fireEvent.input(screen.getByLabelText('Name'), { target: { value: 'Narrator 2' } })
    fireEvent.click(save)

    await waitFor(() =>
      expect(screen.getByRole('heading', { name: 'Narrator 2' })).toBeInTheDocument(),
    )
    expect(lastBody('/api/v1/aliases/alias-1', 'PUT')).toEqual({
      name: 'Narrator 2',
      endpoint_id: 'ep-1',
      model: 'tts-1-hd',
      voice: 'shimmer',
      speed: 1.2,
      instructions: 'warm',
      languages: ['en', 'fr'],
    })
    expect(invalidateCache).toHaveBeenCalledWith('/api/v1/voices')
    expect(screen.getByRole('button', { name: 'Save' })).toBeDisabled()
  })

  it('shows what the server stored after saving', async () => {
    // UpdateAlias cannot clear speed: the response still carries it.
    overrides.push((url, init) =>
      url === '/api/v1/aliases/alias-1' && init?.method === 'PUT'
        ? jsonResponse({ ...aliases['alias-1'], instructions: 'warm' })
        : undefined,
    )
    renderAt('/playground/alias-1')
    await ready()
    fireEvent.input(screen.getByLabelText('Speed'), { target: { value: '' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(screen.getByLabelText('Speed')).toHaveValue(1.2))
  })

  it('shows a save error', async () => {
    overrides.push((url, init) =>
      url === '/api/v1/aliases/alias-1' && init?.method === 'PUT'
        ? errorResponse(409, 'conflict', 'alias with this name already exists')
        : undefined,
    )
    renderAt('/playground/alias-1')
    await ready()
    fireEvent.input(screen.getByLabelText('Name'), { target: { value: 'Butler' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))
    expect(await screen.findByText('alias with this name already exists')).toBeInTheDocument()
  })

  it('saves a copy as a new alias', async () => {
    const { location } = renderAt('/playground/alias-1')
    await ready()
    fireEvent.click(screen.getByRole('button', { name: 'Save as new' }))
    const dialog = await screen.findByRole('dialog')
    expect(within(dialog).getByRole('heading', { name: 'Save as new alias' })).toBeInTheDocument()
    expect(within(dialog).getByLabelText('Name')).toHaveValue('Narrator copy')
    expect(within(dialog).getByLabelText('Languages (comma-separated)')).toHaveValue('en, fr')
    fireEvent.click(within(dialog).getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(location.history).toContain('/playground/alias-new'))
    expect(lastBody('/api/v1/aliases')).toMatchObject({
      name: 'Narrator copy',
      voice: 'nova',
      speed: 1.2,
      instructions: 'warm',
      languages: ['en', 'fr'],
    })
  })

  it('reloads when switching aliases, keeping the text', async () => {
    const { location } = renderAt('/playground/alias-1')
    await ready()
    fireEvent.input(textArea(), { target: { value: 'keep me' } })
    act(() => location.navigate('/playground/alias-2'))
    await waitFor(() => expect(screen.getByLabelText('Voice')).toHaveValue('onyx'))
    expect(textArea()).toHaveValue('keep me')
  })

  it('selects an endpoint for the alias from the panel', async () => {
    renderAt('/playground/alias-1')
    await ready()
    act(() => selectCallbacks['playground-endpoint']('ep-2'))
    expect(trigger('playground-model')).toHaveAttribute('data-value', 'kokoro')
    expect(screen.getByRole('button', { name: 'Save' })).toBeEnabled()
  })
})
