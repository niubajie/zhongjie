import { getToken } from './utils/session'

App<IAppOption>({
  globalData: {
    apiBase: 'http://127.0.0.1:8080',
    token: '',
  },
  onLaunch() {
    this.globalData.token = getToken()
  },
})
