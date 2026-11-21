import type {
  AssistantRecommendations,
  AssistantReply,
  AssistantSession,
  AuthResult,
  BuildingSuggest,
  CommunitySuggest,
  ContactReveal,
  ContactSettings,
  CreatedProperty,
  CreatePropertyInput,
  MapData,
  Narration,
  PropertyDetail,
  PublicUser,
  Tour,
} from '../types/api'
import { request } from './request'
import { getToken } from './session'

// Media URLs may be relative (/media/...) in local mode; make them playable in the mini program.
export function absoluteMediaURL(url?: string): string {
  if (!url) {
    return ''
  }
  if (url.startsWith('http://') || url.startsWith('https://')) {
    return url
  }
  const base = getApp<IAppOption>().globalData.apiBase
  return `${base}${url.startsWith('/') ? '' : '/'}${url}`
}

export function login(code: string, devOpenid?: string): Promise<AuthResult> {
  return request<AuthResult>('/v1/auth/wechat/login', {
    method: 'POST',
    auth: false,
    data: { code, devOpenid },
  })
}

export function bindPhone(code: string, devPhone?: string): Promise<PublicUser> {
  return request<PublicUser>('/v1/auth/wechat/phone', {
    method: 'POST',
    data: { code, devPhone },
  })
}

export function getMe(): Promise<PublicUser> {
  return request<PublicUser>('/v1/me')
}

export function completeProfile(surname: string, role: 'landlord' | 'tenant'): Promise<PublicUser> {
  return request<PublicUser>('/v1/profile', {
    method: 'POST',
    data: { surname, role },
  })
}

export interface MapQuery {
  cityCode: string
  swLat: number
  swLng: number
  neLat: number
  neLng: number
  zoom: number
}

export function getMap(query: MapQuery): Promise<MapData> {
  const qs = [
    `cityCode=${encodeURIComponent(query.cityCode)}`,
    `swLat=${query.swLat}`,
    `swLng=${query.swLng}`,
    `neLat=${query.neLat}`,
    `neLng=${query.neLng}`,
    `zoom=${query.zoom}`,
  ].join('&')
  return request<MapData>(`/v1/properties/map?${qs}`, { auth: false })
}

export function getProperty(uuid: string): Promise<PropertyDetail> {
  return request<PropertyDetail>(`/v1/properties/${encodeURIComponent(uuid)}`, { auth: false })
}

export function suggestBuildings(cityCode: string, keyword: string): Promise<BuildingSuggest[]> {
  const qs = [
    `cityCode=${encodeURIComponent(cityCode)}`,
    `keyword=${encodeURIComponent(keyword)}`,
  ].join('&')
  return request<{ items: BuildingSuggest[] }>(`/v1/buildings/suggest?${qs}`, { auth: false }).then(
    (data) => data.items,
  )
}

export interface VoiceStatus {
  engine: string
  asr: boolean
  tts: boolean
  llm: boolean
  hint: string
}

export interface VoiceTurnResult {
  transcript: string
  reply: string
  ready: boolean
  action: string
  audioUrls: string[]
}

export function getVoiceStatus(): Promise<VoiceStatus> {
  return request<VoiceStatus>('/v1/assistant/voice/status', { auth: false })
}

export function voiceTurn(sessionId: string, filePath: string): Promise<VoiceTurnResult> {
  const app = getApp<IAppOption>()
  const token = getToken()
  return new Promise<VoiceTurnResult>((resolve, reject) => {
    wx.uploadFile({
      url: `${app.globalData.apiBase}/v1/assistant/voice/turn?sessionId=${encodeURIComponent(sessionId)}`,
      filePath,
      name: 'audio',
      header: token ? { Authorization: `Bearer ${token}` } : {},
      success(res) {
        try {
          const body = JSON.parse(res.data) as {
            code: number
            message: string
            data: VoiceTurnResult
          }
          if (res.statusCode < 200 || res.statusCode >= 300 || body.code !== 0) {
            reject(new Error(body.message || '语音请求失败'))
            return
          }
          resolve(body.data)
        } catch {
          reject(new Error('语音响应解析失败'))
        }
      },
      fail(err) {
        reject(new Error(err.errMsg || '语音上传失败'))
      },
    })
  })
}

export function suggestCommunities(cityCode: string, keyword: string): Promise<CommunitySuggest[]> {
  const qs = [
    `cityCode=${encodeURIComponent(cityCode)}`,
    `keyword=${encodeURIComponent(keyword)}`,
  ].join('&')
  return request<{ items: CommunitySuggest[] }>(`/v1/communities/suggest?${qs}`, { auth: false }).then(
    (data) => data.items,
  )
}

export function createProperty(input: CreatePropertyInput): Promise<CreatedProperty> {
  return request<CreatedProperty>('/v1/landlord/properties', {
    method: 'POST',
    data: input,
  })
}

export interface MyProperty {
  uuid: string
  title: string
  communityName?: string
  status: string
  pendingReason?: string
  areaSqm: number
  rentCents: number
  lastConfirmedAt?: string
  createdAt: string
  needsConfirm: boolean
}

export function listMyProperties(): Promise<MyProperty[]> {
  return request<{ items: MyProperty[] }>('/v1/landlord/properties').then((data) => data.items)
}

export function confirmPropertyFreshness(uuid: string, stillAvailable: boolean): Promise<void> {
  return request<{ status: string }>(
    `/v1/landlord/properties/${encodeURIComponent(uuid)}/freshness`,
    { method: 'POST', data: { status: stillAvailable ? 'active' : 'rented' } },
  ).then(() => undefined)
}

export function createAssistantSession(): Promise<AssistantSession> {
  return request<AssistantSession>('/v1/assistant/sessions', {
    method: 'POST',
    auth: false,
    data: {},
  })
}

export function assistantMessage(sessionId: string, text: string): Promise<AssistantReply> {
  return request<AssistantReply>('/v1/assistant/messages', {
    method: 'POST',
    auth: false,
    data: { sessionId, text },
  })
}

export function getAssistantRecommendations(sessionId: string): Promise<AssistantRecommendations> {
  return request<AssistantRecommendations>(
    `/v1/assistant/recommendations?sessionId=${encodeURIComponent(sessionId)}`,
    { auth: false },
  )
}

export function getNarration(propertyUuid: string): Promise<Narration> {
  return request<Narration>(`/v1/properties/${encodeURIComponent(propertyUuid)}/narration`, {
    auth: false,
  })
}

export function getTour(propertyUuid: string): Promise<Tour> {
  return request<Tour>(`/v1/properties/${encodeURIComponent(propertyUuid)}/tour`, {
    auth: false,
  })
}

export function getContactSettings(): Promise<ContactSettings> {
  return request<ContactSettings>('/v1/landlord/contact-settings')
}

export function updateContactSettings(input: ContactSettings): Promise<ContactSettings> {
  return request<ContactSettings>('/v1/landlord/contact-settings', {
    method: 'PUT',
    data: input,
  })
}

export function createContactRequest(
  propertyUuid: string,
  consent: boolean,
  note = '',
): Promise<ContactReveal> {
  return request<ContactReveal>('/v1/contact-requests', {
    method: 'POST',
    data: { propertyUuid, consent, note },
  })
}
