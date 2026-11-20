import { getContactSettings, updateContactSettings } from '../../utils/api'
import { getToken } from '../../utils/session'

interface SettingsPageData {
  phoneMask: string
  wechatId: string
  sharePhone: boolean
  shareWechat: boolean
  loading: boolean
  message: string
}

Page<SettingsPageData, WechatMiniprogram.Page.CustomOption>({
  data: {
    phoneMask: '',
    wechatId: '',
    sharePhone: true,
    shareWechat: false,
    loading: true,
    message: '',
  },

  onLoad() {
    if (!getToken()) {
      wx.redirectTo({ url: '/pages/auth/index' })
      return
    }
    getContactSettings()
      .then((settings) => {
        this.setData({
          phoneMask: settings.phoneMask,
          wechatId: settings.wechatId || '',
          sharePhone: settings.sharePhone,
          shareWechat: settings.shareWechat,
          loading: false,
        })
      })
      .catch((err: Error) => this.setData({ loading: false, message: err.message || '加载失败' }))
  },

  onWechatInput(e: { detail: { value: string } }) {
    this.setData({ wechatId: e.detail.value })
  },

  onSharePhoneChange(e: { detail: { value: boolean } }) {
    this.setData({ sharePhone: e.detail.value })
  },

  onShareWechatChange(e: { detail: { value: boolean } }) {
    this.setData({ shareWechat: e.detail.value })
  },

  onSave() {
    if (this.data.shareWechat && !this.data.wechatId.trim()) {
      this.setData({ message: '展示微信号时必须填写微信号' })
      return
    }
    this.setData({ loading: true, message: '' })
    updateContactSettings({
      phoneMask: this.data.phoneMask,
      wechatId: this.data.wechatId.trim(),
      sharePhone: this.data.sharePhone,
      shareWechat: this.data.shareWechat,
    })
      .then((settings) => {
        this.setData({
          phoneMask: settings.phoneMask,
          wechatId: settings.wechatId || '',
          sharePhone: settings.sharePhone,
          shareWechat: settings.shareWechat,
          loading: false,
          message: '已保存',
        })
      })
      .catch((err: Error) => this.setData({ loading: false, message: err.message || '保存失败' }))
  },
})
