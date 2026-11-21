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

export interface AdminImportJob {
  uuid: string
  fileName: string
  fileType: string
  status: string
  totalRows: number
  successRows: number
  errorRows: number
  createdAt: string
}

export interface AdminBuilding {
  uuid: string
  name: string
  districtName: string
  streetName: string
  buildingType: string
  reviewStatus: string
  lat: number
  lng: number
}

export interface AdminContact {
  uuid: string
  tenantSurname: string
  tenantPhoneMask: string
  propertyTitle: string
  status: string
  createdAt: string
}

export interface AdminContactDetail extends AdminContact {
  note?: string
}

export interface AdminAuditLog {
  id: number
  actorType: string
  action: string
  targetType: string
  targetId?: string
  createdAt: string
}

export interface AdminIntakeDraft {
  uuid: string
  inputType: string
  status: string
  matchStatus: string
  communityName?: string
  landlordId?: string
  propertyId?: string
  confidence: number
  errorMessage?: string
  areaSqm: number
  rentYuan: number
  title?: string
  createdAt: string
}

export interface AdminFreshness {
  id: number
  propertyUuid: string
  title: string
  status: string
  createdAt: string
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
  if (init.body && !(init.body instanceof FormData)) {
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

export interface IntakeResult {
  jobId: string
  propertyId?: string
  status: string
  landlordId?: string
  communityName?: string
  buildingName?: string
  message?: string
}

export interface IntakeExtracted {
  communityName?: string
  areaSqm?: number
  rentYuan?: number
  layout?: string
  title?: string
}

export interface AdminIntakeMedia {
  jobId: string
  status: string
  matchStatus: string
  mediaUrls: string[]
  extracted: IntakeExtracted
  maskedPhone?: string
  landlordId?: string
  confidence: number
  message?: string
  ocrText?: string
}

export async function adminIntakeMedia(token: string, file: File): Promise<AdminIntakeMedia> {
  const body = new FormData()
  body.append('file', file)
  return request<AdminIntakeMedia>('/v1/admin/rentals/intake/media', { method: 'POST', body }, token)
}

export async function publishIntake(
  token: string,
  jobId: string,
  input: {
    communityUuid?: string
    communityName?: string
    areaSqm: number
    rentCents: number
    title?: string
    layout?: string
    landlordId?: string
  },
): Promise<IntakeResult> {
  return request<IntakeResult>(
    `/v1/admin/rentals/intake/${encodeURIComponent(jobId)}/publish`,
    { method: 'POST', body: JSON.stringify(input) },
    token,
  )
}

export async function matchIntakeLandlord(
  token: string,
  jobId: string,
  landlordId: string,
  reason: string,
): Promise<boolean> {
  const data = await request<{ matched: boolean }>(
    `/v1/admin/rentals/intake/${encodeURIComponent(jobId)}/match`,
    { method: 'POST', body: JSON.stringify({ landlordId, reason }) },
    token,
  )
  return data.matched
}

export interface CommunitySuggestion {
  uuid: string
  name: string
  districtCode: string
  districtName: string
  streetCode: string
  streetName: string
  communityType: string
  lat: number
  lng: number
}

export async function suggestCommunities(
  keyword: string,
  cityCode = '440300',
): Promise<CommunitySuggestion[]> {
  const data = await request<{ items: CommunitySuggestion[] }>(
    `/v1/communities/suggest?cityCode=${encodeURIComponent(cityCode)}&keyword=${encodeURIComponent(keyword)}`,
  )
  return data.items
}

export async function createCommunity(
  token: string,
  input: {
    cityCode: string
    districtCode: string
    districtName: string
    streetCode: string
    streetName: string
    name: string
    communityType: 'residential' | 'apartment'
    lat: number
    lng: number
  },
): Promise<{ uuid: string; name: string }> {
  return request<{ uuid: string; name: string }>(
    '/v1/admin/communities',
    { method: 'POST', body: JSON.stringify(input) },
    token,
  )
}

export async function adminIntakeText(
  token: string,
  text: string,
  landlordPhone: string,
): Promise<IntakeResult> {
  return request<IntakeResult>(
    '/v1/admin/rentals/intake/text',
    { method: 'POST', body: JSON.stringify({ text, landlordPhone }) },
    token,
  )
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

export async function fetchAdminImports(token: string, limit = 50): Promise<AdminImportJob[]> {
  const data = await request<{ items: AdminImportJob[] }>(`/v1/admin/imports?limit=${limit}`, {}, token)
  return data.items
}

export async function fetchAdminBuildings(token: string, limit = 200): Promise<AdminBuilding[]> {
  const data = await request<{ items: AdminBuilding[] }>(`/v1/admin/buildings?limit=${limit}`, {}, token)
  return data.items
}

export async function fetchAdminContacts(token: string, limit = 50): Promise<AdminContact[]> {
  const data = await request<{ items: AdminContact[] }>(`/v1/admin/contacts?limit=${limit}`, {}, token)
  return data.items
}

export async function fetchAdminContactDetail(token: string, uuid: string): Promise<AdminContactDetail> {
  return request<AdminContactDetail>(`/v1/admin/contacts/${encodeURIComponent(uuid)}`, {}, token)
}

export async function fetchAdminAuditLogs(token: string, limit = 100): Promise<AdminAuditLog[]> {
  const data = await request<{ items: AdminAuditLog[] }>(`/v1/admin/audit-logs?limit=${limit}`, {}, token)
  return data.items
}

export async function fetchAdminIntakeDrafts(token: string, limit = 50): Promise<AdminIntakeDraft[]> {
  const data = await request<{ items: AdminIntakeDraft[] }>(`/v1/admin/rentals/intake?limit=${limit}`, {}, token)
  return data.items
}

export async function fetchAdminIntakeDetail(token: string, jobId: string): Promise<AdminIntakeMedia> {
  return request<AdminIntakeMedia>(
    `/v1/admin/rentals/intake/${encodeURIComponent(jobId)}`,
    {},
    token,
  )
}

export async function retryAdminIntake(token: string, jobId: string): Promise<AdminIntakeMedia> {
  return request<AdminIntakeMedia>(
    `/v1/admin/rentals/intake/${encodeURIComponent(jobId)}/retry`,
    { method: 'POST', body: '{}' },
    token,
  )
}

export async function fetchAdminFreshness(token: string, limit = 50): Promise<AdminFreshness[]> {
  const data = await request<{ items: AdminFreshness[] }>(`/v1/admin/freshness?limit=${limit}`, {}, token)
  return data.items
}

export async function fetchAdminProperties(token: string, limit = 20): Promise<AdminProperty[]> {
  const data = await request<{ items: AdminProperty[] }>(
    `/v1/admin/properties?limit=${limit}`,
    {},
    token,
  )
  return data.items
}
