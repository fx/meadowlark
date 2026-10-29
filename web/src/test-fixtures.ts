// Shared API fixtures for page and component tests.
import type { Endpoint, EndpointVoice, ResolvedVoice, VoiceAlias } from '@/lib/api'

export function makeEndpoint(overrides: Partial<Endpoint> = {}): Endpoint {
  return {
    id: 'ep-1',
    name: 'OpenAI',
    base_url: 'https://api.openai.com/v1',
    models: ['tts-1', 'tts-1-hd'],
    default_model: 'tts-1',
    default_voice: 'alloy',
    default_response_format: 'wav',
    streaming_enabled: false,
    stream_sample_rate: 0,
    enabled: true,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

export function makeAlias(overrides: Partial<VoiceAlias> = {}): VoiceAlias {
  return {
    id: 'alias-1',
    name: 'Narrator',
    endpoint_id: 'ep-1',
    model: 'tts-1-hd',
    voice: 'nova',
    languages: ['en'],
    enabled: true,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

export function makeEndpointVoice(voiceId: string, overrides: Partial<EndpointVoice> = {}) {
  return {
    endpoint_id: 'ep-1',
    voice_id: voiceId,
    name: '',
    enabled: true,
    created_at: '',
    updated_at: '',
    ...overrides,
  } satisfies EndpointVoice
}

export function makeResolvedVoice(overrides: Partial<ResolvedVoice> = {}): ResolvedVoice {
  return {
    name: 'alloy (OpenAI, tts-1)',
    endpoint_id: 'ep-1',
    endpoint: 'OpenAI',
    model: 'tts-1',
    voice: 'alloy',
    is_alias: false,
    ...overrides,
  }
}

/** A minimal fetch Response stand-in. */
export function jsonResponse(data: unknown, status = 200) {
  return Promise.resolve({
    ok: status >= 200 && status < 300,
    status,
    json: () => Promise.resolve(data),
  } as Response)
}
