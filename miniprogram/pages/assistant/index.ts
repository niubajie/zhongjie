import {
  absoluteMediaURL,
  assistantMessage,
  createAssistantSession,
  createContactRequest,
  getAssistantRecommendations,
  getNarration,
  getProperty,
  getTour,
  getVoiceStatus,
  voiceTurn,
} from '../../utils/api'
import { getToken } from '../../utils/session'
import type { AssistantItem, NarrationSegment, Tour } from '../../types/api'

interface ChatMessage {
  role: 'user' | 'robot'
  text: string
}

interface VoiceServerEvent {
  type: string
  text?: string
  seq?: number
  url?: string
  hint?: string
  status?: string
  action?: string
  ready?: boolean
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
  gallery: string[]
  videoUrl: string
  videoCover: string
  showGallery: boolean
  showMap: boolean
  showVideo: boolean
  mapLat: number
  mapLng: number
  tour: Tour | null
  playing: boolean
  ready: boolean
  voiceState: 'idle' | 'recording' | 'thinking' | 'playing'
  voiceHint: string
  voiceReady: boolean
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
    gallery: [],
    videoUrl: '',
    videoCover: '',
    showGallery: false,
    showMap: false,
    showVideo: false,
    mapLat: 22.5431,
    mapLng: 114.0579,
    tour: null,
    playing: true,
    ready: false,
    voiceState: 'idle',
    voiceHint: '点麦克风说话，小亿会边听边答',
    voiceReady: false,
  },

  autoTimer: 0 as number,
  recorder: null as WechatMiniprogram.RecorderManager | null,
  audioPlayer: null as WechatMiniprogram.InnerAudioContext | null,
  audioQueue: [] as string[],
  audioIndex: 0,
  audioBusy: false,
  imageURLs: {} as Record<string, string>,
  socket: null as WechatMiniprogram.SocketTask | null,
  socketReady: false,
  useSocket: false,
  socketRecording: false,
  turnActive: false,
  voiceConfigured: false,
  stopGuard: 0 as number,
  pageAlive: true,

  onLoad() {
    this.createSession()
    this.initVoice()
  },

  onUnload() {
    this.pageAlive = false
    this.stopTimer()
    this.stopAudio()
    this.clearStopGuard()
    this.closeVoiceSocket()
    if (this.recorder && this.data.voiceState === 'recording') {
      this.recorder.stop()
    }
  },

  initVoice() {
    const recorder = wx.getRecorderManager()
    recorder.onStop((res) => {
      if (!this.pageAlive) {
        return
      }
      if (this.socketRecording) {
        this.socketRecording = false
        if (this.data.voiceState === 'recording') {
          this.setData({ voiceState: 'idle', voiceHint: '点麦克风继续聊' })
        }
        return
      }
      if (res.tempFilePath) {
        this.uploadVoice(res.tempFilePath)
      } else {
        this.setData({ voiceState: 'idle' })
      }
    })
    recorder.onError(() => {
      this.socketRecording = false
      this.setData({ voiceState: 'idle', voiceHint: '录音失败，请检查麦克风权限' })
    })
    recorder.onFrameRecorded((res) => {
      if (!this.socketRecording || !this.socketReady || !this.socket) {
        return
      }
      this.socket.send({ data: res.frameBuffer })
    })
    this.recorder = recorder

    const audio = wx.createInnerAudioContext()
    audio.onEnded(() => this.playNextAudio())
    audio.onError(() => this.stopAudio())
    this.audioPlayer = audio

    getVoiceStatus()
      .then((status) => {
        this.voiceConfigured = status.asr && status.tts
        this.setData({
          voiceReady: this.voiceConfigured,
          voiceHint: status.hint,
        })
        if (this.voiceConfigured && this.data.sessionId) {
          this.connectVoiceSocket(this.data.sessionId)
        }
      })
      .catch(() => this.setData({ voiceHint: '语音状态未知，可继续用文字' }))
  },

  connectVoiceSocket(sessionId: string) {
    if (!sessionId) {
      return
    }
    this.closeVoiceSocket()
    const task = wx.connectSocket({
      url: voiceSocketURL(getApp<IAppOption>().globalData.apiBase, sessionId),
      timeout: 8000,
    })
    this.socket = task
    task.onOpen(() => {
      if (this.socket !== task) {
        return
      }
      this.socketReady = true
      this.useSocket = true
      this.setData({ voiceHint: '点麦克风说话，小亿会边听边答' })
    })
    task.onMessage((res) => {
      if (typeof res.data !== 'string') {
        return
      }
      try {
        this.handleVoiceEvent(JSON.parse(res.data) as VoiceServerEvent)
      } catch {
        // Ignore malformed frames; the socket stays usable.
      }
    })
    task.onError(() => {
      if (this.socket === task) {
        this.socketReady = false
        this.useSocket = false
      }
    })
    task.onClose(() => {
      if (this.socket === task) {
        this.socket = null
        this.socketReady = false
        this.useSocket = false
        if (this.data.voiceState === 'thinking') {
          this.setData({ voiceState: 'idle', voiceHint: '语音连接已断开，可改用文字' })
        }
      }
    })
  },

  closeVoiceSocket() {
    const socket = this.socket
    this.socket = null
    this.socketReady = false
    this.useSocket = false
    if (socket) {
      socket.close({})
    }
  },

  sendSocket(payload: { type: string }) {
    if (this.socket && this.socketReady) {
      this.socket.send({ data: JSON.stringify(payload) })
    }
  },

  handleVoiceEvent(event: VoiceServerEvent) {
    switch (event.type) {
      case 'error':
        this.clearStopGuard()
        this.turnActive = false
        if (event.status === 'not_configured') {
          this.voiceConfigured = false
          this.useSocket = false
          this.setData({
            voiceState: 'idle',
            voiceReady: false,
            voiceHint: event.hint || '语音未配置，可改用文字',
          })
          return
        }
        this.setData({ voiceState: 'idle', voiceHint: event.hint || '语音服务暂时不可用' })
        return
      case 'vad':
        if (event.status === 'speech') {
          // Barge-in: the user started talking, stop the current sentence.
          this.stopAudio()
        } else if (event.status === 'silence') {
          this.setData({ voiceState: 'thinking', voiceHint: '小亿正在理解…' })
          if (this.socketRecording) {
            this.recorder?.stop()
          }
        }
        return
      case 'processing':
        this.clearStopGuard()
        this.setData({ voiceState: 'thinking', voiceHint: '小亿正在理解…' })
        return
      case 'asr':
        if (event.text) {
          this.clearStopGuard()
          this.setData({ messages: [...this.data.messages, { role: 'user', text: event.text }] })
        }
        return
      case 'reply':
        this.clearStopGuard()
        this.turnActive = true
        this.audioQueue = []
        this.audioIndex = 0
        this.setData({
          messages: [...this.data.messages, { role: 'robot', text: event.text || '' }],
          ready: Boolean(event.ready),
          voiceState: 'thinking',
          voiceHint: '小亿正在回应…',
        })
        if (event.action === 'recommend') {
          this.loadRecommendations()
        }
        return
      case 'audio':
        if (event.url) {
          this.queueAudio(event.url)
        }
        return
      case 'turn_end':
        this.clearStopGuard()
        this.turnActive = false
        if (!this.audioBusy) {
          this.setData({ voiceState: 'idle', voiceHint: '点麦克风继续聊' })
        }
        return
      case 'interrupted':
        this.stopAudio()
        this.clearStopGuard()
        this.turnActive = false
        if (!this.socketRecording) {
          this.setData({ voiceState: 'idle', voiceHint: '已打断，可以继续说话' })
        }
        return
      default:
        return
    }
  },

  onVoiceTap() {
    const state = this.data.voiceState
    if (state === 'recording') {
      if (this.socketRecording) {
        this.flushSocketSilence()
        this.setData({ voiceState: 'thinking', voiceHint: '小亿正在理解…' })
        this.startStopGuard()
      }
      this.recorder?.stop()
      return
    }
    if (state === 'thinking') {
      return
    }
    if (!this.data.voiceReady) {
      wx.showToast({ title: this.data.voiceHint || '语音未配置', icon: 'none' })
      return
    }
    if (state === 'playing') {
      // Barge-in: stop the current reply immediately, then listen.
      this.stopAudio()
      this.sendSocket({ type: 'interrupt' })
    }
    this.turnActive = false
    if (this.useSocket && this.socketReady) {
      this.startSocketRecording()
      return
    }
    this.startFileRecording()
  },

  startSocketRecording() {
    this.socketRecording = true
    this.audioQueue = []
    this.audioIndex = 0
    this.recorder?.start({
      duration: 40000,
      sampleRate: 16000,
      numberOfChannels: 1,
      format: 'PCM',
      frameSize: 5,
    })
    this.setData({ voiceState: 'recording', voiceHint: '正在听…停下后小亿会自动回应' })
  },

  startFileRecording() {
    this.socketRecording = false
    this.recorder?.start({
      duration: 60000,
      sampleRate: 16000,
      numberOfChannels: 1,
      encodeBitRate: 48000,
      format: 'mp3',
    })
    this.setData({ voiceState: 'recording', voiceHint: '正在听…再次点击结束' })
  },

  flushSocketSilence() {
    if (this.socket && this.socketReady) {
      // Trailing silence lets the server VAD close the turn when the user taps stop.
      this.socket.send({ data: new ArrayBuffer(25600) })
    }
  },

  startStopGuard() {
    this.clearStopGuard()
    this.stopGuard = setTimeout(() => {
      this.stopGuard = 0
      if (!this.turnActive && this.data.voiceState === 'thinking') {
        this.setData({ voiceState: 'idle', voiceHint: '没听清，点麦克风再说一次' })
      }
    }, 3000)
  },

  clearStopGuard() {
    if (this.stopGuard) {
      clearTimeout(this.stopGuard)
      this.stopGuard = 0
    }
  },

  uploadVoice(filePath: string) {
    if (!this.data.sessionId) {
      this.setData({ voiceState: 'idle' })
      return
    }
    this.setData({ voiceState: 'thinking', voiceHint: '小亿正在理解…' })
    voiceTurn(this.data.sessionId, filePath)
      .then((result) => {
        const messages = [...this.data.messages]
        if (result.transcript) {
          messages.push({ role: 'user', text: result.transcript })
        }
        messages.push({ role: 'robot', text: result.reply })
        this.setData({
          messages,
          ready: result.ready,
          voiceState: 'idle',
          voiceHint: '点麦克风继续聊',
        })
        if (result.action === 'recommend') {
          this.loadRecommendations()
        }
        this.playAudios(result.audioUrls)
      })
      .catch((err: Error) => {
        this.setData({ voiceState: 'idle', voiceHint: err.message || '语音请求失败' })
      })
  },

  playAudios(urls: string[]) {
    if (!urls.length || !this.audioPlayer) {
      return
    }
    this.audioQueue = urls
    this.audioIndex = 0
    this.playNextAudio()
  },

  queueAudio(url: string) {
    this.audioQueue.push(url)
    if (!this.audioBusy) {
      this.playNextAudio()
    }
  },

  playNextAudio() {
    const audio = this.audioPlayer
    if (!audio || this.audioIndex >= this.audioQueue.length) {
      this.audioQueue = []
      this.audioIndex = 0
      this.audioBusy = false
      if (this.data.voiceState === 'playing' && !this.turnActive) {
        this.setData({ voiceState: 'idle', voiceHint: '点麦克风继续聊' })
      }
      return
    }
    this.audioBusy = true
    audio.src = this.audioQueue[this.audioIndex]
    this.audioIndex += 1
    this.setData({ voiceState: 'playing' })
    audio.play()
  },

  stopAudio() {
    if (this.audioPlayer) {
      this.audioPlayer.stop()
    }
    this.audioQueue = []
    this.audioIndex = 0
    this.audioBusy = false
    if (this.data.voiceState === 'playing') {
      this.setData({ voiceState: 'idle' })
    }
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
        if (this.voiceConfigured) {
          this.connectVoiceSocket(session.sessionId)
        }
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
    this.setData({
      current: index,
      currentItem: item,
      currentImage: '',
      gallery: [],
      videoUrl: '',
      videoCover: '',
      showGallery: false,
      showMap: false,
      showVideo: false,
    })
    this.loadNarration(item.uuid)
  },

  loadNarration(uuid: string) {
    this.loadMedia(uuid)
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

  loadMedia(uuid: string) {
    getProperty(uuid)
      .then((detail) => {
        const urls: Record<string, string> = {}
        const gallery: string[] = []
        detail.images.forEach((item) => {
          const url = absoluteMediaURL(item.url)
          if (!url) {
            return
          }
          urls[item.objectKey] = url
          gallery.push(url)
        })
        const video = detail.videos?.find((item) => item.url)
        this.imageURLs = urls
        this.setData({
          gallery,
          imageCount: gallery.length,
          videoUrl: video ? absoluteMediaURL(video.url) : '',
          videoCover: video ? absoluteMediaURL(video.coverUrl) : '',
        })
      })
      .catch(() => {
        this.imageURLs = {}
        this.setData({ gallery: [], imageCount: 0, videoUrl: '', videoCover: '' })
      })
  },

  onGalleryImageError() {
    this.setData({ currentImage: '' })
  },

  onPreviewImage() {
    const urls = this.data.gallery
    if (!urls.length) {
      return
    }
    wx.previewImage({ current: this.data.currentImage || urls[0], urls })
  },

  applySegment(index: number) {
    const segment = this.data.narration[index]
    if (!segment) {
      this.stopTimer()
      return
    }
    this.setData({ segmentIndex: index, currentText: segment.text })
    if (segment.action === 'show_image') {
      const matched = segment.imageId ? this.imageURLs[segment.imageId] : ''
      this.setData({ currentImage: matched || this.data.gallery[0] || '', showGallery: true })
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

function voiceSocketURL(apiBase: string, sessionId: string): string {
  const base = apiBase.replace(/\/+$/, '')
  const path = `/v1/assistant/voice?sessionId=${encodeURIComponent(sessionId)}`
  if (base.startsWith('https://')) {
    return `wss://${base.slice('https://'.length)}${path}`
  }
  if (base.startsWith('http://')) {
    return `ws://${base.slice('http://'.length)}${path}`
  }
  return `ws://${base}${path}`
}
