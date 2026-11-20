import type { ApiEnvelope } from '../types/api'
import { clearSession, getToken } from './session'

export class ApiError extends Error {
  code: number
  statusCode: number

  constructor(message: string, code: number, statusCode: number) {
    super(message)
    this.name = 'ApiError'
    this.code = code
    this.statusCode = statusCode
  }
}

interface RequestOptions {
  method?: 'GET' | 'POST' | 'PUT' | 'DELETE'
  data?: object | string
  auth?: boolean
}

export function request<T>(path: string, options: RequestOptions = {}): Promise<T> {
  const app = getApp<IAppOption>()
  const token = getToken()
  const header: Record<string, string> = {
    'Content-Type': 'application/json',
  }
  if (options.auth !== false && token) {
    header.Authorization = `Bearer ${token}`
  }

  return new Promise<T>((resolve, reject) => {
    wx.request<ApiEnvelope<T>>({
      url: `${app.globalData.apiBase}${path}`,
      method: options.method ?? 'GET',
      data: options.data as WechatMiniprogram.IAnyObject | string | undefined,
      header,
      timeout: 10000,
      success(res) {
        const body = res.data
        if (res.statusCode === 401) {
          clearSession()
        }
        if (res.statusCode < 200 || res.statusCode >= 300 || body.code !== 0) {
          reject(new ApiError(body.message || '请求失败', body.code, res.statusCode))
          return
        }
        resolve(body.data)
      },
      fail(err) {
        reject(new ApiError(err.errMsg || '网络异常', -1, 0))
      },
    })
  })
}
