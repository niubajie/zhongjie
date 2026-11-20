import { getProperty } from '../../utils/api'
import type { PropertyDetail } from '../../types/api'

interface PropertyPageData {
  uuid: string
  detail: PropertyDetail | null
  rentYuan: string
  loading: boolean
  error: string
  imageCount: number
}

Page<PropertyPageData, WechatMiniprogram.Page.CustomOption>({
  data: {
    uuid: '',
    detail: null,
    rentYuan: '',
    loading: true,
    error: '',
    imageCount: 0,
  },

  onLoad(options: Record<string, string | undefined>) {
    const uuid = options.uuid || ''
    this.setData({ uuid })
    if (!uuid) {
      this.setData({ loading: false, error: '缺少房源编号' })
      return
    }
    getProperty(uuid)
      .then((detail) => {
        this.setData({
          detail,
          rentYuan: formatRent(detail.rentCents),
          imageCount: detail.images.length,
          loading: false,
        })
      })
      .catch((err: Error) => {
        this.setData({ loading: false, error: err.message || '房源加载失败' })
      })
  },

  onContact() {
    wx.showModal({
      title: '确定需要联系房东吗？',
      content: '确认后会发送你的联系信息给房东，并展示房东联系方式。',
      cancelText: '放弃',
      confirmText: '确认',
      success: (res) => {
        if (res.confirm) {
          wx.showToast({ title: '联系功能将在下一阶段接入', icon: 'none' })
        }
      },
    })
  },

  onBack() {
    wx.navigateBack()
  },
})

function formatRent(cents: number): string {
  const yuan = Math.round(cents / 100)
  if (yuan >= 10000) {
    return `${(yuan / 10000).toFixed(1)}万/月`
  }
  return `${yuan}元/月`
}
