import { createProperty, getMe, suggestBuildings } from '../../utils/api'
import { getToken } from '../../utils/session'
import type { BuildingSuggest } from '../../types/api'

interface PublishPageData {
  role: string
  canPublish: boolean
  keyword: string
  suggestions: BuildingSuggest[]
  selected: BuildingSuggest | null
  area: string
  rentYuan: string
  layout: string
  floor: string
  orientation: string
  title: string
  description: string
  loading: boolean
  searching: boolean
  message: string
}

Page<PublishPageData, WechatMiniprogram.Page.CustomOption>({
  data: {
    role: '',
    canPublish: false,
    keyword: '',
    suggestions: [],
    selected: null,
    area: '',
    rentYuan: '',
    layout: '',
    floor: '',
    orientation: '',
    title: '',
    description: '',
    loading: false,
    searching: false,
    message: '',
  },

  searchTimer: 0 as number,

  onLoad() {
    if (!getToken()) {
      wx.redirectTo({ url: '/pages/auth/index' })
      return
    }
    getMe()
      .then((user) => {
        if (user.role !== 'landlord') {
          this.setData({
            role: user.role || '',
            canPublish: false,
            message: '只有房东可以发布房源，请在资料页切换为房东身份。',
          })
          return
        }
        this.setData({ role: 'landlord', canPublish: true })
        this.search('')
      })
      .catch(() => {
        wx.redirectTo({ url: '/pages/auth/index' })
      })
  },

  onKeywordInput(e: { detail: { value: string } }) {
    const keyword = e.detail.value
    this.setData({ keyword, selected: null })
    if (this.searchTimer) {
      clearTimeout(this.searchTimer)
    }
    this.searchTimer = setTimeout(() => this.search(keyword), 300)
  },

  search(keyword: string) {
    this.setData({ searching: true })
    suggestBuildings('440300', keyword)
      .then((items) => this.setData({ suggestions: items, searching: false }))
      .catch((err: Error) => this.setData({ searching: false, message: err.message || '楼宇加载失败' }))
  },

  onSelectBuilding(e: { currentTarget: { dataset: { uuid: string } } }) {
    const uuid = e.currentTarget.dataset.uuid
    const selected = this.data.suggestions.find((item) => item.uuid === uuid) || null
    this.setData({ selected })
  },

  onAreaInput(e: { detail: { value: string } }) {
    this.setData({ area: e.detail.value })
  },

  onRentInput(e: { detail: { value: string } }) {
    this.setData({ rentYuan: e.detail.value })
  },

  onLayoutInput(e: { detail: { value: string } }) {
    this.setData({ layout: e.detail.value })
  },

  onFloorInput(e: { detail: { value: string } }) {
    this.setData({ floor: e.detail.value })
  },

  onOrientationInput(e: { detail: { value: string } }) {
    this.setData({ orientation: e.detail.value })
  },

  onTitleInput(e: { detail: { value: string } }) {
    this.setData({ title: e.detail.value })
  },

  onDescriptionInput(e: { detail: { value: string } }) {
    this.setData({ description: e.detail.value })
  },

  onSettings() {
    wx.navigateTo({ url: '/pages/settings/index' })
  },

  onSubmit() {
    if (!this.data.canPublish) {
      return
    }
    if (!this.data.selected) {
      this.setData({ message: '请先选择工业园区或写字楼' })
      return
    }
    const area = Number(this.data.area)
    const rentYuan = Number(this.data.rentYuan)
    if (!Number.isFinite(area) || area < 5 || area > 1000) {
      this.setData({ message: '面积需在 5–1000㎡ 之间' })
      return
    }
    if (!Number.isFinite(rentYuan) || rentYuan < 100 || rentYuan > 1_000_000) {
      this.setData({ message: '月租需在 100–1,000,000 元之间' })
      return
    }

    this.setData({ loading: true, message: '' })
    createProperty({
      buildingUuid: this.data.selected.uuid,
      areaSqm: area,
      rentCents: Math.round(rentYuan * 100),
      layout: this.data.layout,
      floor: this.data.floor,
      orientation: this.data.orientation,
      title: this.data.title,
      description: this.data.description,
    })
      .then((created) => {
        this.setData({ loading: false })
        const text = created.status === 'active' ? '发布成功' : '已提交，等待审核'
        wx.showToast({ title: text, icon: 'success' })
        setTimeout(() => wx.navigateBack(), 600)
      })
      .catch((err: Error) => {
        this.setData({ loading: false, message: err.message || '发布失败' })
      })
  },
})
