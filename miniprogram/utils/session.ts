const TOKEN_KEY = 'housing_token'

export function getToken(): string {
  const value = wx.getStorageSync<string>(TOKEN_KEY)
  return typeof value === 'string' ? value : ''
}

export function setToken(token: string): void {
  wx.setStorageSync(TOKEN_KEY, token)
  const app = getApp<IAppOption>()
  app.globalData.token = token
}

export function clearSession(): void {
  wx.removeStorageSync(TOKEN_KEY)
  const app = getApp<IAppOption>()
  app.globalData.token = ''
}
