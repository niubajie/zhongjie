import { absoluteMediaURL, createContactRequest, getProperty } from '../../utils/api'
import type { PropertyDetail } from '../../types/api'

interface PropertyPageData {
  uuid: string
  detail: PropertyDetail | null
  rentYuan: string
  loading: boolean
  error: string
  imageCount: number
  images: string[]
  videoUrl: string
  videoCover: string
  contactLoading: boolean
}

Page<PropertyPageData, WechatMiniprogram.Page.CustomOption>({
  data: {
    uuid: '',
    detail: null,
    rentYuan: '',
    loading: true,
    error: '',
    imageCount: 0,
    images: [],
    videoUrl: '',
    videoCover: '',
    contactLoading: false,
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
        const images = detail.images.map((item) => absoluteMediaURL(item.url)).filter(Boolean)
        const video = detail.videos?.find((item) => item.url)
        this.setData({
          detail,
          rentYuan: formatRent(detail.rentCents),
          imageCount: images.length,
          images,
          videoUrl: video ? absoluteMediaURL(video.url) : '',
          videoCover: video ? absoluteMediaURL(video.coverUrl) : '',
          loading: false,
        })
      })
      .catch((err: Error) => {
        this.setData({ loading: false, error: err.message || '房源加载失败' })
      })
  },

  onImageError(e: { currentTarget: { dataset: { src?: string } } }) {
    const src = e.currentTarget.dataset.src || ''
    const images = this.data.images.filter((item) => item !== src)
    this.setData({ images, imageCount: images.length })
  },

  onPreviewImage(e: { currentTarget: { dataset: { src?: string } } }) {
    const src = e.currentTarget.dataset.src || ''
    if (!src || !this.data.images.length) {
      return
    }
    wx.previewImage({ current: src, urls: this.data.images })
  },

  onContact() {
    const detail = this.data.detail
    if (!detail) {
      return
    }
    wx.showModal({
      title: '确定需要联系房东吗？',
      content: '确认后会发送你的联系信息给房东，并展示房东联系方式。',
      cancelText: '放弃',
      confirmText: '确认',
      success: (res) => {
        if (res.confirm) {
          this.submitContact(detail.uuid)
        }
      },
    })
  },

  submitContact(uuid: string) {
    this.setData({ contactLoading: true })
    createContactRequest(uuid, true)
      .then((result) => {
        this.setData({ contactLoading: false })
        if (!result.contactAvailable) {
          wx.showModal({
            title: '已提交联系请求',
            content: result.message || '房东暂未授权展示联系方式。',
            showCancel: false,
          })
          return
        }
        const lines: string[] = []
        if (result.phone) {
          lines.push(`电话：${result.phone}`)
        }
        if (result.wechatId) {
          lines.push(`微信：${result.wechatId}`)
        }
        wx.showModal({
          title: '房东联系方式',
          content: lines.join('\n'),
          showCancel: Boolean(result.phone),
          cancelText: '关闭',
          confirmText: result.phone ? '拨打' : '知道了',
          success: (res) => {
            if (res.confirm && result.phone) {
              wx.makePhoneCall({ phoneNumber: result.phone })
            }
          },
        })
      })
      .catch((err: Error) => {
        this.setData({ contactLoading: false })
        wx.showToast({ title: err.message || '提交失败', icon: 'none' })
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
