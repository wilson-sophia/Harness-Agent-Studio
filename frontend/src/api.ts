import type { PipelineForm, PipelineResponse, ProjectAnalysisResponse, SessionInfo, HarnessResource, RunRecord, DemoConfig } from './types'

const apiBaseUrl = import.meta.env.VITE_API_BASE_URL ?? 'http://127.0.0.1:8080'
let csrfToken = ''

export class ApiError extends Error {
  code: string
  status: number
  constructor(message: string, code = '', status = 0) { super(message); this.code = code; this.status = status }
}

async function request<T>(path: string, method = 'GET', body?: unknown, retryCSRF = true): Promise<T> {
  const response = await fetch(apiBaseUrl + path, {
    method,
    credentials: 'include',
    headers: {
      ...(body === undefined ? {} : { 'Content-Type': 'application/json' }),
      ...(method === 'GET' ? {} : { 'X-CSRF-Token': csrfToken }),
    },
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  if (!response.ok) {
    const error = await response.json().catch(() => ({})) as { error?: string; code?: string }
    if (retryCSRF && method !== 'GET' && error.error === 'invalid CSRF token') {
      await getSession()
      return request<T>(path, method, body, false)
    }
    throw new ApiError(error.error ?? `Request failed with ${response.status}`, error.code, response.status)
  }
  if (response.status === 204) return undefined as T
  return response.json() as Promise<T>
}

export async function getSession(): Promise<SessionInfo> {
  const session = await request<SessionInfo>('/api/auth/me')
  csrfToken = session.csrfToken ?? ''
  return session
}
export async function authenticate(email: string, password: string, remember: boolean, register: boolean): Promise<SessionInfo> {
  const session = await request<SessionInfo>(register ? '/api/auth/register' : '/api/auth/login', 'POST', { email, password, remember })
  csrfToken = session.csrfToken ?? ''
  return session
}
export async function confirmPassword(password: string) {
  return request<{ freshUntil: string }>('/api/auth/reauth', 'POST', { password })
}
export async function logout() {
  await request<void>('/api/auth/logout', 'POST', {})
  csrfToken = ''
}
export function analyzeRepository(form: PipelineForm) {
  return request<ProjectAnalysisResponse>('/api/projects/analyze', 'POST', {
    repoUrl: form.repoUrl, branch: form.branch, mode: form.mode,
  })
}
export function generatePipeline(form: PipelineForm) {
  return request<PipelineResponse>('/api/pipelines/generate', 'POST', form)
}
export function getConnection() {
  return request<{ connected: boolean; accountId: string; encryptionConfigured: boolean }>('/api/harness/connection')
}
export function saveConnection(accountId: string, apiToken: string) {
  return request<{ connected: boolean; accountId: string }>('/api/harness/connection', 'POST', { accountId, apiToken })
}
export function deleteConnection() {
  return request<void>('/api/harness/connection', 'DELETE')
}
export function getResources(kind: 'orgs' | 'projects' | 'connectors', org = '', project = '') {
  const query = new URLSearchParams({ kind, org, project })
  return request<unknown>(`/api/harness/resources?${query}`)
}
export function createPipeline(form: PipelineForm) {
  return request<RunRecord>('/api/pipelines/create', 'POST', form)
}
export function getRuns() {
  return request<RunRecord[]>('/api/runs')
}
export function triggerRun(id: string) {
  return request<{ run: RunRecord; status: string }>(`/api/runs/${encodeURIComponent(id)}/trigger`, 'POST', {})
}
export function getRunStatus(id: string) {
  return request<{ run: RunRecord; status: string; details?: unknown }>(`/api/runs/${encodeURIComponent(id)}`)
}
export function getDemoConfig() {
  return request<DemoConfig>('/api/demo/config')
}
export function startDemo() {
  return request<{ run: RunRecord; status: string }>('/api/demo/run', 'POST', {})
}
export function resourceOptions(value: unknown): HarnessResource[] {
  if (Array.isArray(value)) return value.map(normalizeResource).filter((item): item is HarnessResource => item !== null)
  if (value && typeof value === 'object') {
    const object = value as Record<string, unknown>
    for (const key of ['data', 'content', 'organizations', 'projects', 'connectors', 'items']) {
      if (key in object) {
        const found = resourceOptions(object[key])
        if (found.length) return found
      }
    }
  }
  return []
}
function normalizeResource(value: unknown): HarnessResource | null {
  if (!value || typeof value !== 'object') return null
  const wrapper = value as Record<string, unknown>
  const inner = (wrapper.organization ?? wrapper.project ?? wrapper.connector ?? wrapper) as Record<string, unknown>
  const identifier = inner.identifier
  if (typeof identifier !== 'string') return null
  return { identifier, name: typeof inner.name === 'string' ? inner.name : identifier, type: typeof inner.type === 'string' ? inner.type : '' }
}
