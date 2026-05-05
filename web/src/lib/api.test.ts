import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ApiRequestError, api } from './api'

const mockFetch = vi.fn()

beforeEach(() => {
  vi.stubGlobal('fetch', mockFetch)
})

afterEach(() => {
  vi.restoreAllMocks()
})

function jsonResponse(data: unknown, status = 200) {
  return Promise.resolve({
    ok: status >= 200 && status < 300,
    status,
    json: () => Promise.resolve(data),
  })
}

function noContentResponse() {
  return Promise.resolve({
    ok: true,
    status: 204,
    json: () => Promise.resolve(undefined),
  })
}

function errorResponse(status: number, code: string, message: string) {
  return Promise.resolve({
    ok: false,
    status,
    json: () => Promise.resolve({ error: { code, message } }),
  })
}

describe('api.endpoints', () => {
  it('list calls GET /api/v1/endpoints', async () => {
    const endpoints = [{ id: 'ep-1', name: 'OpenAI' }]
    mockFetch.mockReturnValueOnce(jsonResponse(endpoints))
    const result = await api.endpoints.list()
    expect(result).toEqual(endpoints)
    expect(mockFetch).toHaveBeenCalledWith('/api/v1/endpoints', undefined)
  })

  it('get calls GET /api/v1/endpoints/:id', async () => {
    const endpoint = { id: 'ep-1', name: 'OpenAI' }
    mockFetch.mockReturnValueOnce(jsonResponse(endpoint))
    const result = await api.endpoints.get('ep-1')
    expect(result).toEqual(endpoint)
    expect(mockFetch).toHaveBeenCalledWith('/api/v1/endpoints/ep-1', undefined)
  })

  it('create calls POST /api/v1/endpoints with body', async () => {
    const data = { name: 'Test', base_url: 'http://localhost', models: ['tts-1'] }
    const created = { id: 'ep-new', ...data }
    mockFetch.mockReturnValueOnce(jsonResponse(created, 201))
    const result = await api.endpoints.create(data)
    expect(result).toEqual(created)
    expect(mockFetch).toHaveBeenCalledWith('/api/v1/endpoints', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(data),
    })
  })

  it('update calls PUT /api/v1/endpoints/:id with body', async () => {
    const data = { name: 'Updated' }
    const updated = { id: 'ep-1', name: 'Updated' }
    mockFetch.mockReturnValueOnce(jsonResponse(updated))
    const result = await api.endpoints.update('ep-1', data)
    expect(result).toEqual(updated)
    expect(mockFetch).toHaveBeenCalledWith('/api/v1/endpoints/ep-1', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(data),
    })
  })

  it('delete calls DELETE /api/v1/endpoints/:id', async () => {
    mockFetch.mockReturnValueOnce(noContentResponse())
    const result = await api.endpoints.delete('ep-1')
    expect(result).toBeUndefined()
    expect(mockFetch).toHaveBeenCalledWith('/api/v1/endpoints/ep-1', {
      method: 'DELETE',
    })
  })

  it('probe calls POST /api/v1/endpoints/probe with url and api_key', async () => {
    const probeResult = {
      models: [{ id: 'tts-1' }],
      voices: [{ id: 'alloy', name: 'Alloy' }],
    }
    mockFetch.mockReturnValueOnce(jsonResponse(probeResult))
    const result = await api.endpoints.probe('https://api.example.com/v1', 'sk-test')
    expect(result).toEqual(probeResult)
    expect(mockFetch).toHaveBeenCalledWith('/api/v1/endpoints/probe', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ url: 'https://api.example.com/v1', api_key: 'sk-test' }),
    })
  })

  it('voices.list calls GET /api/v1/endpoints/:id/voices', async () => {
    const voices = [
      {
        endpoint_id: 'ep-1',
        voice_id: 'alloy',
        name: 'Alloy',
        enabled: true,
        created_at: '',
        updated_at: '',
      },
    ]
    mockFetch.mockReturnValueOnce(jsonResponse(voices))
    const result = await api.endpoints.voices.list('ep-1')
    expect(result).toEqual(voices)
    expect(mockFetch).toHaveBeenCalledWith('/api/v1/endpoints/ep-1/voices', undefined)
  })

  it('voices.refresh calls POST /api/v1/endpoints/:id/voices/refresh', async () => {
    const voices = [
      {
        endpoint_id: 'ep-1',
        voice_id: 'alloy',
        name: 'Alloy',
        enabled: false,
        created_at: '',
        updated_at: '',
      },
    ]
    mockFetch.mockReturnValueOnce(jsonResponse(voices))
    const result = await api.endpoints.voices.refresh('ep-1')
    expect(result).toEqual(voices)
    expect(mockFetch).toHaveBeenCalledWith('/api/v1/endpoints/ep-1/voices/refresh', {
      method: 'POST',
      headers: undefined,
      body: undefined,
    })
  })

  it('voices.setEnabled calls PATCH /api/v1/endpoints/:id/voices/:voiceId with body', async () => {
    const voice = {
      endpoint_id: 'ep-1',
      voice_id: 'alloy',
      name: '',
      enabled: true,
      created_at: '',
      updated_at: '',
    }
    mockFetch.mockReturnValueOnce(jsonResponse(voice))
    const result = await api.endpoints.voices.setEnabled('ep-1', 'alloy', true)
    expect(result).toEqual(voice)
    expect(mockFetch).toHaveBeenCalledWith('/api/v1/endpoints/ep-1/voices/alloy', {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ enabled: true }),
    })
  })

  it('voices.setEnabled URL-encodes the voice_id', async () => {
    const voice = {
      endpoint_id: 'ep-1',
      voice_id: 'clone:abc',
      name: '',
      enabled: true,
      created_at: '',
      updated_at: '',
    }
    mockFetch.mockReturnValueOnce(jsonResponse(voice))
    await api.endpoints.voices.setEnabled('ep-1', 'clone:abc', true)
    expect(mockFetch).toHaveBeenCalledWith(
      '/api/v1/endpoints/ep-1/voices/clone%3Aabc',
      expect.objectContaining({ method: 'PATCH' }),
    )
  })
})

describe('api.aliases', () => {
  it('list calls GET /api/v1/aliases', async () => {
    const aliases = [{ id: 'va-1', name: 'my-voice' }]
    mockFetch.mockReturnValueOnce(jsonResponse(aliases))
    const result = await api.aliases.list()
    expect(result).toEqual(aliases)
    expect(mockFetch).toHaveBeenCalledWith('/api/v1/aliases', undefined)
  })

  it('get calls GET /api/v1/aliases/:id', async () => {
    const alias = { id: 'va-1', name: 'my-voice' }
    mockFetch.mockReturnValueOnce(jsonResponse(alias))
    const result = await api.aliases.get('va-1')
    expect(result).toEqual(alias)
    expect(mockFetch).toHaveBeenCalledWith('/api/v1/aliases/va-1', undefined)
  })

  it('create calls POST /api/v1/aliases with body', async () => {
    const data = { name: 'new-alias', endpoint_id: 'ep-1', model: 'tts-1', voice: 'alloy' }
    const created = { id: 'va-new', ...data }
    mockFetch.mockReturnValueOnce(jsonResponse(created, 201))
    const result = await api.aliases.create(data)
    expect(result).toEqual(created)
    expect(mockFetch).toHaveBeenCalledWith('/api/v1/aliases', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(data),
    })
  })

  it('update calls PUT /api/v1/aliases/:id with body', async () => {
    const data = { name: 'updated-alias' }
    const updated = { id: 'va-1', name: 'updated-alias' }
    mockFetch.mockReturnValueOnce(jsonResponse(updated))
    const result = await api.aliases.update('va-1', data)
    expect(result).toEqual(updated)
    expect(mockFetch).toHaveBeenCalledWith('/api/v1/aliases/va-1', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(data),
    })
  })

  it('delete calls DELETE /api/v1/aliases/:id', async () => {
    mockFetch.mockReturnValueOnce(noContentResponse())
    const result = await api.aliases.delete('va-1')
    expect(result).toBeUndefined()
    expect(mockFetch).toHaveBeenCalledWith('/api/v1/aliases/va-1', {
      method: 'DELETE',
    })
  })

  it('test calls POST /api/v1/aliases/:id/test', async () => {
    const testResult = { ok: true }
    mockFetch.mockReturnValueOnce(jsonResponse(testResult))
    const result = await api.aliases.test('va-1')
    expect(result).toEqual(testResult)
    expect(mockFetch).toHaveBeenCalledWith('/api/v1/aliases/va-1/test', {
      method: 'POST',
      headers: undefined,
      body: undefined,
    })
  })
})

describe('api.system', () => {
  it('status calls GET /api/v1/status', async () => {
    const status = {
      version: '1.0.0',
      uptime_seconds: 3600,
      wyoming_port: 10400,
      http_port: 8080,
      db_driver: 'sqlite',
      voice_count: 5,
      endpoint_count: 2,
      alias_count: 3,
    }
    mockFetch.mockReturnValueOnce(jsonResponse(status))
    const result = await api.system.status()
    expect(result).toEqual(status)
    expect(mockFetch).toHaveBeenCalledWith('/api/v1/status', undefined)
  })

  it('voices calls GET /api/v1/voices', async () => {
    const voices = [
      { name: 'alloy', endpoint: 'OpenAI', model: 'tts-1', voice: 'alloy', is_alias: false },
    ]
    mockFetch.mockReturnValueOnce(jsonResponse(voices))
    const result = await api.system.voices()
    expect(result).toEqual(voices)
    expect(mockFetch).toHaveBeenCalledWith('/api/v1/voices', undefined)
  })
})

describe('error handling', () => {
  it('throws ApiRequestError on HTTP error', async () => {
    mockFetch.mockReturnValueOnce(errorResponse(404, 'not_found', 'endpoint not found'))
    await expect(api.endpoints.get('bad-id')).rejects.toThrow(ApiRequestError)
    try {
      mockFetch.mockReturnValueOnce(errorResponse(404, 'not_found', 'endpoint not found'))
      await api.endpoints.get('bad-id')
    } catch (err) {
      expect(err).toBeInstanceOf(ApiRequestError)
      const apiErr = err as ApiRequestError
      expect(apiErr.status).toBe(404)
      expect(apiErr.code).toBe('not_found')
      expect(apiErr.message).toBe('endpoint not found')
    }
  })

  it('ApiRequestError has correct name', () => {
    const err = new ApiRequestError(500, 'internal_error', 'something broke')
    expect(err.name).toBe('ApiRequestError')
    expect(err.status).toBe(500)
    expect(err.code).toBe('internal_error')
    expect(err.message).toBe('something broke')
  })
})
