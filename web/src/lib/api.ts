// API types matching Go models in internal/model/model.go
// and API response types in internal/api/

export interface Endpoint {
  id: string
  name: string
  base_url: string
  api_key?: string
  models: string[]
  default_speed?: number
  default_instructions?: string
  default_response_format: string
  enabled: boolean
  created_at: string
  updated_at: string
}

export interface CreateEndpoint {
  name: string
  base_url: string
  api_key?: string
  models: string[]
  default_speed?: number
  default_instructions?: string
  default_response_format?: string
  enabled?: boolean
}

export interface UpdateEndpoint {
  name?: string
  base_url?: string
  api_key?: string
  models?: string[]
  default_speed?: number
  default_instructions?: string
  default_response_format?: string
  enabled?: boolean
}

export interface VoiceAlias {
  id: string
  name: string
  endpoint_id: string
  model: string
  voice: string
  speed?: number
  instructions?: string
  languages: string[]
  enabled: boolean
  created_at: string
  updated_at: string
}

export interface CreateVoiceAlias {
  name: string
  endpoint_id: string
  model: string
  voice: string
  speed?: number
  instructions?: string
  languages?: string[]
  enabled?: boolean
}

export interface UpdateVoiceAlias {
  name?: string
  endpoint_id?: string
  model?: string
  voice?: string
  speed?: number
  instructions?: string
  languages?: string[]
  enabled?: boolean
}

export interface ServerStatus {
  version: string
  uptime_seconds: number
  wyoming_port: number
  http_port: number
  db_driver: string
  voice_count: number
  endpoint_count: number
  alias_count: number
}

export interface ResolvedVoice {
  name: string
  endpoint: string
  model: string
  voice: string
  is_alias: boolean
}

export interface TestResult {
  ok: boolean
  error?: string
  latency_ms?: number
}

export interface ProbeModel {
  id: string
}

export interface ProbeVoice {
  id: string
  name: string
}

export interface ProbeResult {
  models: ProbeModel[]
  voices: ProbeVoice[]
}

export interface ApiError {
  error: {
    code: string
    message: string
  }
}

export class ApiRequestError extends Error {
  status: number
  code: string

  constructor(status: number, code: string, message: string) {
    super(message)
    this.name = 'ApiRequestError'
    this.status = status
    this.code = code
  }
}

async function request<T>(url: string, options?: RequestInit): Promise<T> {
  const res = await fetch(url, options)
  if (res.status === 204) {
    return undefined as T
  }
  const body = await res.json()
  if (!res.ok) {
    const err = body as ApiError
    throw new ApiRequestError(res.status, err.error.code, err.error.message)
  }
  return body as T
}

function get<T>(url: string): Promise<T> {
  return request<T>(url)
}

function post<T>(url: string, data?: unknown): Promise<T> {
  return request<T>(url, {
    method: 'POST',
    headers: data ? { 'Content-Type': 'application/json' } : undefined,
    body: data ? JSON.stringify(data) : undefined,
  })
}

function put<T>(url: string, data: unknown): Promise<T> {
  return request<T>(url, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data),
  })
}

function del<T = void>(url: string): Promise<T> {
  return request<T>(url, { method: 'DELETE' })
}

export const api = {
  endpoints: {
    list: () => get<Endpoint[]>('/api/v1/endpoints'),
    get: (id: string) => get<Endpoint>(`/api/v1/endpoints/${id}`),
    create: (data: CreateEndpoint) => post<Endpoint>('/api/v1/endpoints', data),
    update: (id: string, data: UpdateEndpoint) => put<Endpoint>(`/api/v1/endpoints/${id}`, data),
    delete: (id: string) => del(`/api/v1/endpoints/${id}`),
    probe: (url: string, apiKey: string) =>
      post<ProbeResult>('/api/v1/endpoints/probe', { url, api_key: apiKey }),
  },
  aliases: {
    list: () => get<VoiceAlias[]>('/api/v1/aliases'),
    get: (id: string) => get<VoiceAlias>(`/api/v1/aliases/${id}`),
    create: (data: CreateVoiceAlias) => post<VoiceAlias>('/api/v1/aliases', data),
    update: (id: string, data: UpdateVoiceAlias) => put<VoiceAlias>(`/api/v1/aliases/${id}`, data),
    delete: (id: string) => del(`/api/v1/aliases/${id}`),
    test: (id: string) => post<TestResult>(`/api/v1/aliases/${id}/test`),
  },
  system: {
    status: () => get<ServerStatus>('/api/v1/status'),
    voices: () => get<ResolvedVoice[]>('/api/v1/voices'),
  },
}
