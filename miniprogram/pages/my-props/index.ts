import { confirmPropertyFreshness, listMyProperties, type MyProperty } from '../../utils/api'
import { getToken } from '../../utils/session'

interface MyPropCard extends MyProperty {
  statusText: string
  rentYuan: string
}

interface MyPropsPageData {
  items: MyPropCard[]
  loading: boolean
  message: string
  filter: 'all' | 'need'
}

const STATUS_TEXT: Record<string, string> = {
  active: '在租',
  pending: '待核验',
  rented: '已租出',
  offline: '已下线',
  stale: '待确认',
  draft: '草稿',
}

Page<MyPropsPageData, WechatMiniprogram.Page.CustomOption>({
  data: {
    items: [],
    loading: true,
    message: '',
    filter: 'all',
  },

  onShow() {
    this.load()
  },

  onPullDownRefresh() {
    this.load().then(() => wx.stopPullDownRefresh())
  },

  load() {
    if (!getToken()) {
      wx.redirectTo({ url: '/pages/auth/index' })
      return Promise.resolve()
    }
    this.setData({ loading: true, message: '' })
    return listMyProperties()
      .then((items) => {
        this.setData({
          items: items.map((item) => ({
            ...item,
            statusText: STATUS_TEXT[item.status] || item.status,
            rentYuan: String(Math.round(item.rentCents / 100)),
            lastConfirmedAt: item.lastConfirmedAt ? item.lastConfirmedAt.slice(0, 10) : '',
          })),
          loading: false,
          message: items.length ? '' : '还没有发布房源',
        })
      })
      .catch((err: Error) =>
        this.setData({ loading: false, message: err.message || '加载失败' }),
      )
  },

  onFilter(e: { currentTarget: { dataset: { filter: string } } }) {
    const filter = e.currentTarget.dataset.filter === 'need' ? 'need' : 'all'
    this.setData({ filter })
  },

  onConfirm(e: { currentTarget: { dataset: { uuid: string; available: string } } }) {
    const { uuid, available } = e.currentTarget.dataset
    const stillAvailable = available === '1'
    wx.showLoading({ title: '提交中' })
    confirmPropertyFreshness(uuid, stillAvailable)
      .then(() => {
        wx.hideLoading()
        wx.showToast({ title: stillAvailable ? '已确认在租' : '已标记租出', icon: 'success' })
        return this.load()
      })
      .catch((err: Error) => {
        wx.hideLoading()
        wx.showToast({ title: err.message || '更新失败', icon: 'none' })
      })
  },

  onOpen(e: { currentTarget: { dataset: { uuid: string } } }) {
    wx.navigateTo({ url: `/pages/property/index?uuid=${e.currentTarget.dataset.uuid}` })
  },
})
