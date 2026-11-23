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

// ---- 自主获客（v1.5，批次 7 管理站接入）----

export type LeadSourceKind = 'self_owned' | 'public_api' | 'third_party_scrape'
export type LeadType = 'tenant' | 'landlord'
export type ProspectContactChannel = 'phone' | 'wechat' | 'in_app' | 'sms' | 'unknown'
export type ProspectIntentStatus =
  | 'seeking'
  | 'rented'
  | 'landlord_listing'
  | 'agent'
  | 'irrelevant'
  | 'unknown'
export type ProspectStatus =
  | 'new'
  | 'reviewed'
  | 'contacted'
  | 'following_up'
  | 'converted'
  | 'published'
  | 'cold'
  | 'lost'
  | 'opt_out'
  | 'rejected'
  | 'ignored'
export type TransitionStatus =
  | 'following_up'
  | 'converted'
  | 'published'
  | 'cold'
  | 'lost'
  | 'rejected'
  | 'ignored'
export type ContactChannel = 'in_app' | 'sms' | 'wechat' | 'phone'
export type FollowUpDirection = 'outbound' | 'inbound'
export type ReminderLevel = 'due_3d' | 'overdue_7d'
export type NotificationLevel = ReminderLevel | 'error'
export type ReviewDecision = 'approve' | 'reject'

export interface LeadSource {
  uuid: string
  name: string
  kind: LeadSourceKind
  baseUrl?: string
  robotsOk: boolean
  termsOk: boolean
  rateLimitMs: number
  collectIntervalMinutes: number
  lastCollectedAt?: string
  nextCollectAt?: string
  consecutiveFailures: number
  lastError?: string
  lastErrorAt?: string
  enabled: boolean
  reviewNote?: string
  createdAt: string
  updatedAt: string
}

export interface CreateLeadSourceInput {
  name: string
  kind: LeadSourceKind
  baseUrl?: string
  rateLimitMs?: number
  collectIntervalMinutes?: number
  reviewNote?: string
}

export interface UpdateLeadSourceInput extends CreateLeadSourceInput {
  robotsOk: boolean
  termsOk: boolean
}

export interface ProspectCollectRun {
  id: number
  sourceUuid: string
  sourceName: string
  status: 'success' | 'failed'
  fetched: number
  ingested: number
  duplicate: number
  optedOut: number
  invalid: number
  failed: number
  errorMessage?: string
  startedAt: string
  finishedAt: string
}

export interface ProspectLead {
  uuid: string
  sourceName: string
  leadType: LeadType
  platform: string
  contactChannel: ProspectContactChannel
  cityCode?: string
  districtCode?: string
  budgetCents?: number
  areaSqm?: number
  layout?: string
  intentScore?: number
  intentStatus: ProspectIntentStatus
  status: ProspectStatus
  matchedPropertyId?: string
  stale?: boolean
  staleAt?: string
  createdAt: string
  updatedAt: string
}

export interface FollowUp {
  id: number
  channel: ContactChannel
  direction: FollowUpDirection
  status: 'pending' | 'sent' | 'failed' | 'received'
  content?: string
  sentAt?: string
  createdAt: string
}

export interface ProspectReminder {
  id: number
  leadUuid: string
  leadType: LeadType
  leadStatus: ProspectStatus
  level: ReminderLevel
  createdAt: string
}

export interface ProspectLeadDetail extends ProspectLead {
  rawText: string
  followUps: FollowUp[]
}

export interface OptOut {
  id: number
  leadType: LeadType
  reason?: string
  optedOutAt: string
}

export interface KillSwitchState {
  halted: boolean
}

export async function fetchKillSwitch(token: string): Promise<KillSwitchState> {
  return request<KillSwitchState>('/v1/admin/prospects/kill-switch', {}, token)
}

export async function setKillSwitch(token: string, halted: boolean): Promise<KillSwitchState> {
  return request<KillSwitchState>(
    '/v1/admin/prospects/kill-switch',
    { method: 'PUT', body: JSON.stringify({ halted }) },
    token,
  )
}

export async function fetchLeadSources(token: string): Promise<LeadSource[]> {
  const data = await request<{ items: LeadSource[] }>('/v1/admin/lead-sources', {}, token)
  return data.items
}

export async function createLeadSource(
  token: string,
  input: CreateLeadSourceInput,
): Promise<LeadSource> {
  return request<LeadSource>(
    '/v1/admin/lead-sources',
    { method: 'POST', body: JSON.stringify(input) },
    token,
  )
}

export async function updateLeadSource(
  token: string,
  uuid: string,
  input: UpdateLeadSourceInput,
): Promise<void> {
  await request<{ updated: boolean }>(
    `/v1/admin/lead-sources/${encodeURIComponent(uuid)}`,
    { method: 'PATCH', body: JSON.stringify(input) },
    token,
  )
}

export async function deleteLeadSource(token: string, uuid: string): Promise<void> {
  await request<{ deleted: boolean }>(
    `/v1/admin/lead-sources/${encodeURIComponent(uuid)}`,
    { method: 'DELETE' },
    token,
  )
}

export async function setLeadSourceEnabled(
  token: string,
  uuid: string,
  enabled: boolean,
): Promise<boolean> {
  const data = await request<{ enabled: boolean }>(
    `/v1/admin/lead-sources/${encodeURIComponent(uuid)}/enabled`,
    { method: 'PATCH', body: JSON.stringify({ enabled }) },
    token,
  )
  return data.enabled
}

export async function fetchProspects(
  token: string,
  filters: { leadType?: LeadType | ''; status?: ProspectStatus | ''; limit?: number } = {},
): Promise<ProspectLead[]> {
  const params = new URLSearchParams()
  if (filters.leadType) params.set('leadType', filters.leadType)
  if (filters.status) params.set('status', filters.status)
  params.set('limit', String(filters.limit ?? 50))
  const data = await request<{ items: ProspectLead[] }>(
    `/v1/admin/prospects?${params.toString()}`,
    {},
    token,
  )
  return data.items
}

export async function fetchReviewQueue(
  token: string,
  leadType: LeadType | '' = '',
  limit = 50,
): Promise<ProspectLead[]> {
  const params = new URLSearchParams()
  if (leadType) params.set('leadType', leadType)
  params.set('limit', String(limit))
  const data = await request<{ items: ProspectLead[] }>(
    `/v1/admin/prospects/review-queue?${params.toString()}`,
    {},
    token,
  )
  return data.items
}

export async function fetchCollectRuns(
  token: string,
  sourceUuid = '',
  limit = 50,
): Promise<ProspectCollectRun[]> {
  const params = new URLSearchParams()
  if (sourceUuid) params.set('sourceUuid', sourceUuid)
  params.set('limit', String(limit))
  const data = await request<{ items: ProspectCollectRun[] }>(
    `/v1/admin/prospects/collect-runs?${params.toString()}`,
    {},
    token,
  )
  return data.items
}

export async function fetchProspectDetail(token: string, uuid: string): Promise<ProspectLeadDetail> {
  return request<ProspectLeadDetail>(`/v1/admin/prospects/${encodeURIComponent(uuid)}`, {}, token)
}

export async function reviewProspect(
  token: string,
  uuid: string,
  decision: ReviewDecision,
): Promise<ProspectLead> {
  return request<ProspectLead>(
    `/v1/admin/prospects/${encodeURIComponent(uuid)}/review`,
    { method: 'POST', body: JSON.stringify({ decision }) },
    token,
  )
}

export async function contactProspect(
  token: string,
  uuid: string,
  channel: ContactChannel,
  content: string,
): Promise<FollowUp> {
  return request<FollowUp>(
    `/v1/admin/prospects/${encodeURIComponent(uuid)}/contact`,
    { method: 'POST', body: JSON.stringify({ channel, content }) },
    token,
  )
}

export interface StatusCount {
  status: ProspectStatus
  count: number
}

export interface IntentCount {
  intentStatus: ProspectIntentStatus
  count: number
}

export interface ProspectStats {
  totalLeads: number
  reviewPending: number
  contactedLeads: number
  convertedLeads: number
  conversionRate: number
  staleLeads: number
  openReminders: number
  byStatus: StatusCount[]
  byIntent: IntentCount[]
}

export async function fetchProspectStats(token: string): Promise<ProspectStats> {
  return request<ProspectStats>('/v1/admin/prospects/stats', {}, token)
}

export interface ProspectSourceStat {
  sourceUuid: string
  sourceName: string
  kind: LeadSourceKind
  enabled: boolean
  runs: number
  fetched: number
  ingested: number
  duplicate: number
  optedOut: number
  invalid: number
  failed: number
  leadsTotal: number
  pending: number
  reviewed: number
  followingUp: number
  converted: number
  published: number
  rejected: number
  leadsOptedOut: number
  qualified: number
  avgScore: number
  followUps: number
  conversionRate: number
}

export interface SourceStatsFilter {
  from?: string
  to?: string
}

function sourceStatsQuery(filter: SourceStatsFilter): string {
  const params = new URLSearchParams()
  if (filter.from) {
    params.set('from', filter.from)
  }
  if (filter.to) {
    params.set('to', filter.to)
  }
  const query = params.toString()
  return query ? `?${query}` : ''
}

export async function fetchProspectSourceStats(
  token: string,
  filter: SourceStatsFilter = {},
): Promise<ProspectSourceStat[]> {
  const data = await request<{ items: ProspectSourceStat[] }>(
    `/v1/admin/prospects/source-stats${sourceStatsQuery(filter)}`,
    {},
    token,
  )
  return data.items
}

// 导出 CSV 走二进制响应，不能用 request() 解析 JSON。
export async function exportProspectSourceStatsCsv(
  token: string,
  filter: SourceStatsFilter = {},
): Promise<{ blob: Blob; filename: string }> {
  const response = await fetch(
    `${apiBase}/v1/admin/prospects/source-stats/export${sourceStatsQuery(filter)}`,
    { headers: { Authorization: `Bearer ${token}` } },
  )
  if (!response.ok) {
    let message = `HTTP ${response.status}`
    try {
      const body = (await response.json()) as Envelope<unknown>
      if (body.message) {
        message = body.message
      }
    } catch {
      // 非 JSON 错误响应，保留默认信息
    }
    throw new Error(message)
  }
  const disposition = response.headers.get('Content-Disposition') ?? ''
  const match = disposition.match(/filename="([^"]+)"/)
  return {
    blob: await response.blob(),
    filename: match?.[1] ?? 'source-stats.csv',
  }
}

export type ProspectBatchActionType = 'approve' | 'reject' | 'transition' | 'opt_out'

export interface ProspectBatchItemResult {
  uuid: string
  ok: boolean
  message?: string
}

export interface ProspectBatchActionResult {
  total: number
  succeeded: number
  failed: number
  results: ProspectBatchItemResult[]
}

export async function batchProspectLeads(
  token: string,
  action: ProspectBatchActionType,
  uuids: string[],
  options: { status?: string; reason?: string } = {},
): Promise<ProspectBatchActionResult> {
  return request<ProspectBatchActionResult>(
    '/v1/admin/prospects/batch',
    { method: 'POST', body: JSON.stringify({ action, uuids, ...options }) },
    token,
  )
}

export async function fetchProspectReminders(
  token: string,
  limit = 50,
): Promise<ProspectReminder[]> {
  const params = new URLSearchParams()
  params.set('limit', String(limit))
  const data = await request<{ items: ProspectReminder[] }>(
    `/v1/admin/prospects/reminders?${params.toString()}`,
    {},
    token,
  )
  return data.items
}

export async function addProspectFollowUp(
  token: string,
  uuid: string,
  channel: ContactChannel,
  direction: FollowUpDirection,
  content: string,
): Promise<FollowUp> {
  return request<FollowUp>(
    `/v1/admin/prospects/${encodeURIComponent(uuid)}/follow-ups`,
    { method: 'POST', body: JSON.stringify({ channel, direction, content }) },
    token,
  )
}

export async function transitionProspect(
  token: string,
  uuid: string,
  status: TransitionStatus,
): Promise<ProspectLead> {
  return request<ProspectLead>(
    `/v1/admin/prospects/${encodeURIComponent(uuid)}/status`,
    { method: 'POST', body: JSON.stringify({ status }) },
    token,
  )
}

export async function optOutProspect(
  token: string,
  uuid: string,
  reason: string,
): Promise<ProspectLead> {
  return request<ProspectLead>(
    `/v1/admin/prospects/${encodeURIComponent(uuid)}/opt-out`,
    { method: 'POST', body: JSON.stringify({ reason }) },
    token,
  )
}

export interface AdminNotification {
  id: number
  eventType: 'prospect_followup_due' | 'prospect_followup_overdue' | 'prospect_collect_failed'
  level: NotificationLevel
  title: string
  body: string
  leadUuid?: string
  status: 'unread' | 'read'
  createdAt: string
}

export interface AdminNotificationList {
  items: AdminNotification[]
  unread: number
}

export async function fetchProspectNotifications(
  token: string,
  options: { unread?: boolean; limit?: number } = {},
): Promise<AdminNotificationList> {
  const params = new URLSearchParams()
  if (options.unread) params.set('unread', 'true')
  params.set('limit', String(options.limit ?? 50))
  return request<AdminNotificationList>(
    `/v1/admin/prospects/notifications?${params.toString()}`,
    {},
    token,
  )
}

export async function markProspectNotificationRead(token: string, id: number): Promise<void> {
  await request<{ read: boolean }>(
    `/v1/admin/prospects/notifications/${id}/read`,
    { method: 'POST', body: '{}' },
    token,
  )
}

export async function fetchOptOuts(
  token: string,
  leadType: LeadType | '' = '',
  limit = 50,
): Promise<OptOut[]> {
  const params = new URLSearchParams()
  if (leadType) params.set('leadType', leadType)
  params.set('limit', String(limit))
  const data = await request<{ items: OptOut[] }>(
    `/v1/admin/prospects/opt-outs?${params.toString()}`,
    {},
    token,
  )
  return data.items
}

export interface ProspectAuditLog {
  id: number
  actorType: 'admin' | 'user'
  actorId?: number
  actorName?: string
  action: string
  targetType: string
  targetId?: string
  createdAt: string
}

export interface ProspectAuditLogPage {
  items: ProspectAuditLog[]
  total: number
  limit: number
  offset: number
}

export interface ProspectAuditLogFilters {
  action?: string
  targetType?: string
  targetId?: string
  actorId?: string
  from?: string
  to?: string
  limit?: number
  offset?: number
}

export async function fetchProspectAuditLogs(
  token: string,
  filters: ProspectAuditLogFilters = {},
): Promise<ProspectAuditLogPage> {
  const params = new URLSearchParams()
  if (filters.action) params.set('action', filters.action)
  if (filters.targetType) params.set('targetType', filters.targetType)
  if (filters.targetId) params.set('targetId', filters.targetId)
  if (filters.actorId) params.set('actorId', filters.actorId)
  if (filters.from) params.set('from', filters.from)
  if (filters.to) params.set('to', filters.to)
  params.set('limit', String(filters.limit ?? 50))
  params.set('offset', String(filters.offset ?? 0))
  return request<ProspectAuditLogPage>(
    `/v1/admin/prospects/audit-logs?${params.toString()}`,
    {},
    token,
  )
}
