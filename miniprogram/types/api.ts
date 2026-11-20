export interface ApiEnvelope<T> {
  code: number
  message: string
  data: T
  traceId?: string
}

export interface PublicUser {
  uuid: string
  phoneMask?: string
  surname?: string
  role?: 'landlord' | 'tenant' | ''
  status: string
  profileCompleted: boolean
}

export interface AuthResult {
  token: string
  expiresAt: string
  profileCompleted: boolean
  user: PublicUser
}

export interface MapCluster {
  id: string
  districtCode: string
  districtName: string
  streetCode: string
  streetName: string
  lat: number
  lng: number
  count: number
  level: string
}

export interface MapItem {
  uuid: string
  districtCode: string
  districtName: string
  streetCode: string
  streetName: string
  lat: number
  lng: number
  areaSqm: number
  rentCents: number
  layout?: string
  title: string
}

export interface MapData {
  cityCode: string
  zoom: number
  clusters: MapCluster[]
  items: MapItem[]
}

export interface PropertyImage {
  objectKey: string
  width?: number
  height?: number
  sort: number
}

export interface BuildingSuggest {
  uuid: string
  name: string
  districtCode: string
  districtName: string
  streetCode: string
  streetName: string
  buildingType: 'industrial_park' | 'office_building'
  lat: number
  lng: number
}

export interface CreatePropertyInput {
  buildingUuid: string
  areaSqm: number
  rentCents: number
  layout?: string
  floor?: string
  orientation?: string
  title?: string
  description?: string
  imageKeys?: string[]
}

export interface CreatedProperty {
  uuid: string
  status: string
  pendingReason?: string
}

export interface ContactReveal {
  requestUuid: string
  phone?: string
  wechatId?: string
  contactAvailable: boolean
  notificationStatus: string
  message?: string
}

export interface PropertyDetail {
  uuid: string
  districtCode: string
  districtName: string
  streetCode: string
  streetName: string
  areaSqm: number
  rentCents: number
  layout?: string
  floor?: string
  orientation?: string
  title: string
  description?: string
  buildingType: string
  publishedAt?: string
  lastConfirmedAt?: string
  images: PropertyImage[]
}

export interface LoginResponse {
  token: string
  expiresAt: string
  profileCompleted: boolean
  user: PublicUser
}
