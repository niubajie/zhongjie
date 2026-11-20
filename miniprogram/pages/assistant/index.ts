import {
  assistantMessage,
  createAssistantSession,
  createContactRequest,
  getAssistantRecommendations,
  getNarration,
  getTour,
} from '../../utils/api'
import { getToken } from '../../utils/session'
import type { AssistantItem, NarrationSegment, Tour } from '../../types/api'

interface ChatMessage {
  role: 'user' | 'robot'
  text: string
}

interface AssistantPageData {
  sessionId: string
  messages: ChatMessage[]
  input: string
  loading: boolean
  error: string
  items: AssistantItem[]
  current: number
  currentItem: AssistantItem | null
  narration: NarrationSegment[]
  segmentIndex: number
  currentText: string
  currentImage: string
  imageCount: number
  showGallery: boolean
  showMap: boolean
  showVideo: boolean
  mapLat: number
  mapLng: number
  tour: Tour | null
  playing: boolean
  ready: boolean
}

Page<AssistantPageData, WechatMiniprogram.Page.CustomOption>({
  data: {
    sessionId: '',
    messages: [],
    input: '',
    loading: false,
    error: '',
    items: [],
    current: 0,
    currentItem: null,
    narration: [],
    segmentIndex: 0,
    currentText: '',
    currentImage: '',
    imageCount: 0,
    showGallery: false,
    showMap: false,
    showVideo: false,
    mapLat: 22.5431,
    mapLng: 114.0579,
    tour: null,
    playing: true,
    ready: false,
  },

  autoTimer: 0 as number,

  onLoad() {
    this.createSession()
  },

  onUnload() {
    this.stopTimer()
  },

  createSession() {
    createAssistantSession()
      .then((session) => {
        this.setData({
          sessionId: session.sessionId,
          messages: [
            {
              role: 'robot',
              text: '我是小亿。告诉我区域、预算、面积和楼宇类型，我来帮你找。',
            },
          ],
        })
      })
      .catch((err: Error) => this.setData({ error: err.message || '小亿暂时不在线' }))
  },

  onInput(e: { detail: { value: string } }) {
    this.setData({ input: e.detail.value })
  },

  onSend() {
    const text = this.data.input.trim()
    if (!text || !this.data.sessionId || this.data.loading) {
      return
    }
    this.setData({
      input: '',
      loading: true,
      messages: [...this.data.messages, { role: 'user', text }],
    })
    assistantMessage(this.data.sessionId, text)
      .then((reply) => {
        this.setData({
          messages: [...this.data.messages, { role: 'robot', text: reply.reply }],
          ready: reply.ready,
          loading: false,
        })
        if (reply.action === 'recommend') {
          this.loadRecommendations()
        }
      })
      .catch((err: Error) => {
        this.setData({ loading: false, error: err.message || '发送失败' })
      })
  },

  onStartRecommend() {
    if (!this.data.sessionId) {
      return
    }
    this.setData({ loading: true })
    assistantMessage(this.data.sessionId, '没有了，开始找')
      .then((reply) => {
        this.setData({
          messages: [...this.data.messages, { role: 'robot', text: reply.reply }],
          loading: false,
          ready: true,
        })
        this.loadRecommendations()
      })
      .catch((err: Error) => this.setData({ loading: false, error: err.message || '匹配失败' }))
  },

  loadRecommendations() {
    getAssistantRecommendations(this.data.sessionId)
      .then((result) => {
        if (result.items.length === 0) {
          this.setData({
            items: [],
            currentItem: null,
            error: result.message || '暂时没有匹配的房源',
          })
          return
        }
        this.setData({
          items: result.items,
          current: 0,
          currentItem: result.items[0],
          error: '',
          messages: [
            ...this.data.messages,
            {
              role: 'robot',
              text: result.message || `按你的条件找到 ${result.items.length} 套，我从第一套开始讲。`,
            },
          ],
        })
        this.loadNarration(result.items[0].uuid)
      })
      .catch((err: Error) => this.setData({ error: err.message || '推荐失败' }))
  },

  onCardChange(e: { detail: { current: number } }) {
    const index = e.detail.current
    const item = this.data.items[index]
    if (!item) {
      return
    }
    this.stopTimer()
    this.setData({ current: index, currentItem: item, showGallery: false, showMap: false, showVideo: false })
    this.loadNarration(item.uuid)
  },

  loadNarration(uuid: string) {
    getNarration(uuid)
      .then((narration) => {
        this.setData({ narration: narration.segments, segmentIndex: 0 })
        this.applySegment(0)
        if (this.data.playing) {
          this.startTimer()
        }
      })
      .catch((err: Error) => this.setData({ error: err.message || '解说加载失败' }))
  },

  applySegment(index: number) {
    const segment = this.data.narration[index]
    if (!segment) {
      this.stopTimer()
      return
    }
    this.setData({ segmentIndex: index, currentText: segment.text })
    if (segment.action === 'show_image' && segment.imageId) {
      this.setData({ currentImage: segment.imageId, showGallery: true })
    }
    if (segment.action === 'open_map3d') {
      getTour(this.data.currentItem?.uuid || '')
        .then((tour) => {
          this.setData({
            tour,
            mapLat: tour.lat,
            mapLng: tour.lng,
            showMap: true,
          })
        })
        .catch(() => this.setData({ showMap: true }))
    }
    if (segment.action === 'play_video') {
      this.setData({ showVideo: true })
    }
  },

  onNextSegment() {
    const next = this.data.segmentIndex + 1
    if (next >= this.data.narration.length) {
      this.stopTimer()
      this.setData({ playing: false })
      return
    }
    this.applySegment(next)
  },

  onTogglePlay() {
    if (this.data.playing) {
      this.stopTimer()
      this.setData({ playing: false })
      return
    }
    this.setData({ playing: true })
    this.startTimer()
  },

  startTimer() {
    this.stopTimer()
    this.autoTimer = setInterval(() => {
      if (!this.data.playing) {
        return
      }
      this.onNextSegment()
    }, 4500)
  },

  stopTimer() {
    if (this.autoTimer) {
      clearInterval(this.autoTimer)
      this.autoTimer = 0
    }
  },

  noop() {
    // Stop tap-through on overlay cards.
  },

  onCloseGallery() {
    this.setData({ showGallery: false })
  },

  onCloseMap() {
    this.setData({ showMap: false })
  },

  onCloseVideo() {
    this.setData({ showVideo: false })
  },

  onContact() {
    const item = this.data.currentItem
    if (!item) {
      return
    }
    if (!getToken()) {
      wx.navigateTo({ url: '/pages/auth/index' })
      return
    }
    wx.showModal({
      title: '确定需要联系房东吗？',
      content: '确认后会发送你的联系信息给房东，并展示房东联系方式。',
      cancelText: '放弃',
      confirmText: '确认',
      success: (res) => {
        if (!res.confirm) {
          return
        }
        createContactRequest(item.uuid, true)
          .then((result) => {
            if (!result.contactAvailable) {
              wx.showModal({
                title: '已提交',
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
              confirmText: result.phone ? '拨打' : '知道了',
              success: (r) => {
                if (r.confirm && result.phone) {
                  wx.makePhoneCall({ phoneNumber: result.phone })
                }
              },
            })
          })
          .catch((err: Error) => wx.showToast({ title: err.message || '提交失败', icon: 'none' }))
      },
    })
  },

  onBack() {
    wx.navigateBack()
  },
})

function formatRent(cents: number): string {
  const yuan = Math.round(cents / 100)
  return yuan >= 10000 ? `${(yuan / 10000).toFixed(1)}万/月` : `${yuan}元/月`
}
