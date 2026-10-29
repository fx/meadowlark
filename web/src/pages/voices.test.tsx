import { fireEvent, render, screen, waitFor, within } from '@testing-library/preact'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { Router } from 'wouter'
import { memoryLocation } from 'wouter/memory-location'
import { clearCache } from '@/hooks/use-fetch'
import type { ResolvedVoice, VoiceAlias } from '@/lib/api'
import { jsonResponse, makeAlias, makeEndpoint, makeResolvedVoice } from '@/test-fixtures'
import { VoicesPage } from './voices'

let aliases: VoiceAlias[]
let voices: ResolvedVoice[]
let fetchMock: ReturnType<typeof vi.fn>
let failUrl: string | null

beforeEach(() => {
  clearCache()
  failUrl = null
  aliases = [
    makeAlias({ id: 'a-2', name: 'Zed', voice: 'onyx', enabled: false }),
    makeAlias({ id: 'a-1', name: 'Butler', endpoint_id: 'ep-gone' }),
  ]
  voices = [
    makeResolvedVoice(),
    makeResolvedVoice({
      name: 'clone:me (Local, kokoro)',
      endpoint_id: 'ep-2',
      endpoint: 'Local',
      model: 'kokoro',
      voice: 'clone:me',
    }),
    // Enabled aliases also appear here; the page takes aliases from /aliases.
    makeResolvedVoice({ name: 'Butler', is_alias: true }),
  ]
  fetchMock = vi.fn((url: string, init?: RequestInit) => {
    const method = init?.method ?? 'GET'
    if (url === failUrl) return jsonResponse({ error: { code: 'x', message: 'db down' } }, 500)
    if (url === '/api/v1/aliases') return jsonResponse(aliases)
    if (url === '/api/v1/voices') return jsonResponse(voices)
    if (url === '/api/v1/endpoints') return jsonResponse([makeEndpoint()])
    const id = url.replace('/api/v1/aliases/', '')
    if (method === 'PUT') {
      const body = JSON.parse(String(init?.body))
      aliases = aliases.map((a) => (a.id === id ? { ...a, ...body } : a))
      return jsonResponse(aliases.find((a) => a.id === id))
    }
    if (method === 'DELETE') {
      aliases = aliases.filter((a) => a.id !== id)
      return Promise.resolve({ ok: true, status: 204, json: () => Promise.resolve() } as Response)
    }
    return jsonResponse({})
  })
  vi.stubGlobal('fetch', fetchMock)
})

afterEach(() => {
  vi.unstubAllGlobals()
  clearCache()
})

function renderPage() {
  const location = memoryLocation({ path: '/voices', record: true })
  render(
    <Router hook={location.hook}>
      <VoicesPage />
    </Router>,
  )
  return location
}

const rowOf = (name: string) => screen.getByText(name).closest('tr') as HTMLElement

describe('VoicesPage', () => {
  it('shows loading until aliases and voices arrive', async () => {
    renderPage()
    expect(screen.getByText('Loading voices...')).toBeInTheDocument()
    await screen.findByText('Butler')
  })

  it('lists every alias, disabled ones included, then the canonical voices', async () => {
    renderPage()
    await screen.findByText('Butler')
    const rows = screen.getAllByRole('row').slice(1)
    expect(rows.map((r) => within(r).getAllByRole('cell')[0].textContent)).toEqual([
      'Butler',
      'Zed',
      'alloy (OpenAI, tts-1)',
      'clone:me (Local, kokoro)',
    ])
    expect(screen.getAllByText('canonical')).toHaveLength(2)
    expect(screen.getAllByText('alias')).toHaveLength(2)
    expect(screen.getByRole('columnheader', { name: 'Actions' })).toBeInTheDocument()
  })

  it('shows alias endpoint names, falling back to Unknown', async () => {
    renderPage()
    await screen.findByText('Butler')
    expect(within(rowOf('Butler')).getByText('Unknown')).toBeInTheDocument()
    expect(within(rowOf('Zed')).getByText('OpenAI')).toBeInTheDocument()
  })

  it('marks disabled aliases', async () => {
    renderPage()
    await screen.findByText('Butler')
    expect(within(rowOf('Zed')).getByText('disabled')).toBeInTheDocument()
    expect(rowOf('Zed')).toHaveClass('text-muted-foreground')
    expect(within(rowOf('Butler')).queryByText('disabled')).toBeNull()
  })

  it('opens an alias in the playground from its name or its button', async () => {
    const location = renderPage()
    await screen.findByText('Butler')
    expect(screen.getByRole('link', { name: 'Butler' })).toHaveAttribute('href', '/playground/a-1')
    fireEvent.click(screen.getByRole('button', { name: 'Open Zed in playground' }))
    expect(location.history).toContain('/playground/a-2')
  })

  it('opens a canonical voice in a preset playground', async () => {
    const location = renderPage()
    await screen.findByText('Butler')
    fireEvent.click(
      screen.getByRole('button', { name: 'Try clone:me (Local, kokoro) in playground' }),
    )
    expect(location.history).toContain('/playground?endpoint=ep-2&model=kokoro&voice=clone%3Ame')
  })

  it('starts a new alias in the playground', async () => {
    const location = renderPage()
    await screen.findByText('Butler')
    fireEvent.click(screen.getByRole('button', { name: 'New alias' }))
    expect(location.history).toContain('/playground')
  })

  it('toggles an alias and refreshes the lists', async () => {
    renderPage()
    await screen.findByText('Zed')
    fireEvent.click(screen.getByRole('switch', { name: 'Toggle Zed' }))
    await waitFor(() => expect(within(rowOf('Zed')).queryByText('disabled')).toBeNull())
    const put = fetchMock.mock.calls.find(([, init]) => init?.method === 'PUT')
    expect(put?.[0]).toBe('/api/v1/aliases/a-2')
    expect(JSON.parse(String(put?.[1].body))).toEqual({ enabled: true })
    expect(fetchMock.mock.calls.filter(([u]) => u === '/api/v1/voices')).toHaveLength(2)
  })

  it('refreshes even when a toggle fails', async () => {
    renderPage()
    await screen.findByText('Zed')
    fetchMock.mockImplementationOnce(() =>
      jsonResponse({ error: { code: 'x', message: 'nope' } }, 500),
    )
    fireEvent.click(screen.getByRole('switch', { name: 'Toggle Zed' }))
    await waitFor(() =>
      expect(fetchMock.mock.calls.filter(([u]) => u === '/api/v1/aliases')).toHaveLength(2),
    )
    expect(within(rowOf('Zed')).getByText('disabled')).toBeInTheDocument()
  })

  it('deletes an alias after confirmation', async () => {
    const user = userEvent.setup()
    renderPage()
    await screen.findByText('Zed')
    await user.click(screen.getByRole('button', { name: 'Delete Zed' }))
    const dialog = await screen.findByRole('alertdialog')
    expect(dialog).toHaveTextContent('Are you sure you want to delete "Zed"?')
    await user.click(within(dialog).getByRole('button', { name: 'Delete' }))
    await waitFor(() => expect(screen.queryByText('Zed')).toBeNull())
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/aliases/a-2', { method: 'DELETE' })
  })

  it('refreshes even when a delete fails', async () => {
    const user = userEvent.setup()
    renderPage()
    await screen.findByText('Zed')
    await user.click(screen.getByRole('button', { name: 'Delete Zed' }))
    fetchMock.mockImplementationOnce(() =>
      jsonResponse({ error: { code: 'x', message: 'nope' } }, 500),
    )
    await user.click(
      within(await screen.findByRole('alertdialog')).getByRole('button', { name: 'Delete' }),
    )
    await waitFor(() =>
      expect(fetchMock.mock.calls.filter(([u]) => u === '/api/v1/aliases')).toHaveLength(2),
    )
    expect(screen.getByText('Zed')).toBeInTheDocument()
  })

  it('searches aliases and canonical voices by name, ignoring case', async () => {
    const user = userEvent.setup()
    renderPage()
    await screen.findByText('Butler')
    await user.type(screen.getByPlaceholderText('Search voices...'), 'BUT')
    expect(screen.getByText('Butler')).toBeInTheDocument()
    expect(screen.queryByText('Zed')).toBeNull()
    expect(screen.queryByText('alloy (OpenAI, tts-1)')).toBeNull()

    await user.clear(screen.getByPlaceholderText('Search voices...'))
    await user.type(screen.getByPlaceholderText('Search voices...'), 'clone')
    expect(screen.getByText('clone:me (Local, kokoro)')).toBeInTheDocument()
    expect(screen.queryByText('Butler')).toBeNull()

    await user.type(screen.getByPlaceholderText('Search voices...'), 'zzz')
    expect(screen.getByText('No voices found')).toBeInTheDocument()
  })

  it('lists aliases without endpoint names when endpoints fail to load', async () => {
    failUrl = '/api/v1/endpoints'
    renderPage()
    await screen.findByText('Butler')
    expect(within(rowOf('Zed')).getByText('Unknown')).toBeInTheDocument()
  })

  it.each(['/api/v1/aliases', '/api/v1/voices'])('shows an error when %s fails', async (url) => {
    failUrl = url
    renderPage()
    expect(await screen.findByText('Error: db down')).toBeInTheDocument()
  })
})
