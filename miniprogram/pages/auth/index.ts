import { bindPhone, completeProfile, getMe, login } from '../../utils/api'
import { clearSession, getToken, setToken } from '../../utils/session'

interface AuthPageData {
  step: 'login' | 'phone' | 'profile'
  loading: boolean
  message: string
  user: WechatMiniprogram.IAnyObject | null
  phoneMask: string
  surname: string
  role: 'landlord' | 'tenant'
  isDev: boolean
}

Page<AuthPageData, WechatMiniprogram.Page.CustomOption>({
  data: {
    step: 'login',
    loading: false,
    message: '',
    user: null,
    phoneMask: '',
    surname: '',
    role: 'tenant',
    isDev: false,
  },

  onLoad() {
    const account = wx.getAccountInfoSync()
    const isDev = account.miniProgram.envVersion !== 'release'
    this.setData({ isDev })
    if (!getToken()) {
      return
    }
    getMe()
      .then((user) => {
        this.setData({
          user,
          phoneMask: user.phoneMask || '',
          surname: user.surname || '',
          role: user.role === 'landlord' ? 'landlord' : 'tenant',
          step: user.phoneMask ? 'profile' : 'phone',
        })
        if (user.profileCompleted) {
          wx.navigateBack()
        }
      })
      .catch(() => clearSession())
  },

  onLogin() {
    this.setData({ loading: true, message: '' })
    wx.login({
      success: (res) => {
        if (!res.code) {
          this.setData({ loading: false, message: '微信登录失败，请重试' })
          return
        }
        const devOpenid = this.data.isDev ? 'dev-tenant-1' : undefined
        login(res.code, devOpenid)
          .then((result) => {
            setToken(result.token)
            if (result.profileCompleted) {
              this.setData({ loading: false })
              wx.navigateBack()
              return
            }
            this.setData({
              user: result.user as unknown as WechatMiniprogram.IAnyObject,
              phoneMask: result.user.phoneMask || '',
              step: result.user.phoneMask ? 'profile' : 'phone',
              loading: false,
            })
          })
          .catch((err: Error) => {
            this.setData({ loading: false, message: err.message || '登录失败' })
          })
      },
      fail: () => this.setData({ loading: false, message: '微信登录失败，请重试' }),
    })
  },

  onGetPhone(e: { detail: { code?: string; errMsg: string } }) {
    if (e.detail.errMsg !== 'getPhoneNumber:ok' || !e.detail.code) {
      this.setData({ message: '需要授权手机号才能继续' })
      return
    }
    this.bind(e.detail.code, undefined)
  },

  onDevPhone() {
    this.bind('', '13800138000')
  },

  bind(code: string, devPhone?: string) {
    this.setData({ loading: true, message: '' })
    bindPhone(code, devPhone)
      .then((user) => {
        this.setData({
          user: user as unknown as WechatMiniprogram.IAnyObject,
          phoneMask: user.phoneMask || '',
          step: 'profile',
          loading: false,
        })
      })
      .catch((err: Error) => {
        this.setData({ loading: false, message: err.message || '手机号绑定失败' })
      })
  },

  onSurnameInput(e: { detail: { value: string } }) {
    this.setData({ surname: e.detail.value })
  },

  onRoleChange(e: { detail: { value: string } }) {
    this.setData({ role: e.detail.value === 'landlord' ? 'landlord' : 'tenant' })
  },

  onSubmitProfile() {
    const surname = this.data.surname.trim()
    if (!surname) {
      this.setData({ message: '请填写姓氏' })
      return
    }
    this.setData({ loading: true, message: '' })
    completeProfile(surname, this.data.role)
      .then(() => {
        this.setData({ loading: false })
        wx.showToast({ title: '资料已保存', icon: 'success' })
        setTimeout(() => wx.navigateBack(), 500)
      })
      .catch((err: Error) => {
        this.setData({ loading: false, message: err.message || '保存失败' })
      })
  },
})
