export interface Health {
  status: string
}

export interface AdminUser {
  uuid: string
  username: string
  role: string
}

export interface AdminLoginResult {
  token: string
  expiresAt: string
  admin: AdminUser
}

export interface AdminStats {
  activeProperties: number
  pendingProperties: number
  users: number
  landlords: number
  contactRequests: number
}

export interface AdminProperty {
  uuid: string
  title: string
  status: string
  districtName: string
  streetName: string
  areaSqm: number
  rentCents: number
  createdAt: string
}

interface Envelope<T> {
  code: number
  message: string
  data: T
  traceId?: string
}

const apiBase = (import.meta.env.VITE_API_BASE as string | undefined) ?? 'http://127.0.0.1:8080'

async function request<T>(path: string, init: RequestInit = {}, token?: string): Promise<T> {
  const headers = new Headers(init.headers)
  headers.set('Accept', 'application/json')
  if (init.body) {
    headers.set('Content-Type', 'application/json')
  }
  if (token) {
    headers.set('Authorization', `Bearer ${token}`)
  }
  const response = await fetch(`${apiBase}${path}`, { ...init, headers })
  const body = (await response.json()) as Envelope<T>
  if (!response.ok || body.code !== 0) {
    throw new Error(body.message || `HTTP ${response.status}`)
  }
  return body.data
}

export async function fetchHealth(): Promise<Health> {
  return request<Health>('/healthz')
}

export async function adminLogin(
  username: string,
  password: string,
  totpCode: string,
): Promise<AdminLoginResult> {
  return request<AdminLoginResult>('/v1/admin/auth/login', {
    method: 'POST',
    body: JSON.stringify({ username, password, totpCode }),
  })
}

export async function fetchAdminStats(token: string): Promise<AdminStats> {
  return request<AdminStats>('/v1/admin/stats', {}, token)
}

export async function moderateProperty(
  token: string,
  uuid: string,
  action: 'approve' | 'offline',
): Promise<string> {
  const data = await request<{ status: string }>(
    `/v1/admin/properties/${encodeURIComponent(uuid)}/moderate`,
    { method: 'POST', body: JSON.stringify({ action }) },
    token,
  )
  return data.status
}

export async function setupTOTP(token: string): Promise<{ secret: string; otpauthUrl: string }> {
  return request<{ secret: string; otpauthUrl: string }>(
    '/v1/admin/auth/totp/setup',
    { method: 'POST', body: '{}' },
    token,
  )
}

export async function enableTOTP(token: string, code: string): Promise<boolean> {
  const data = await request<{ enabled: boolean }>(
    '/v1/admin/auth/totp/enable',
    { method: 'POST', body: JSON.stringify({ code }) },
    token,
  )
  return data.enabled
}

export async function fetchAdminProperties(token: string, limit = 20): Promise<AdminProperty[]> {
  const data = await request<{ items: AdminProperty[] }>(
    `/v1/admin/properties?limit=${limit}`,
    {},
    token,
  )
  return data.items
}
