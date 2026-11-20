import { createContactRequest, getMap, getProperty, type MapQuery } from '../../utils/api'
import type { MapCluster, MapItem, PropertyDetail } from '../../types/api'

interface MarkerTarget {
  id: number
  kind: 'cluster' | 'item'
  uuid?: string
  latitude: number
  longitude: number
}

interface MapMarker {
  id: number
  latitude: number
  longitude: number
  iconPath: string
  width: number
  height: number
  callout: {
    content: string
    color: string
    bgColor: string
    borderRadius: number
    padding: number
    fontSize: number
    display: 'ALWAYS'
    textAlign: 'center'
  }
}

interface RegionResult {
  southwest: { latitude: number; longitude: number }
  northeast: { latitude: number; longitude: number }
}

interface MapPageData {
  cityCode: string
  cityName: string
  latitude: number
  longitude: number
  scale: number
  markers: MapMarker[]
  markerTargets: MarkerTarget[]
  loading: boolean
  error: string
  totalCount: number
  detail: PropertyDetail | null
  showDetail: boolean
  rentYuan: string
  imageCount: number
  contactLoading: boolean
  showRobot: boolean
  robotText: string
  hasToken: boolean
}

const SHENZHEN = {
  cityCode: '440300',
  cityName: '深圳',
  latitude: 22.5431,
  longitude: 114.0579,
}

Page<MapPageData, WechatMiniprogram.Page.CustomOption>({
  data: {
    cityCode: SHENZHEN.cityCode,
    cityName: SHENZHEN.cityName,
    latitude: SHENZHEN.latitude,
    longitude: SHENZHEN.longitude,
    scale: 13,
    markers: [],
    markerTargets: [],
    loading: true,
    error: '',
    totalCount: 0,
    detail: null,
    showDetail: false,
    rentYuan: '',
    imageCount: 0,
    contactLoading: false,
    showRobot: true,
    robotText: '我是小亿，想找什么样的房子？',
    hasToken: false,
  },

  onLoad() {
    this.setData({ hasToken: Boolean(wx.getStorageSync('housing_token')) })
    this.locate()
  },

  onShow() {
    this.setData({ hasToken: Boolean(wx.getStorageSync('housing_token')) })
  },

  onUnload() {
    if (this.regionTimer) {
      clearTimeout(this.regionTimer)
    }
  },

  regionTimer: 0 as number,
  mapContext: null as WechatMiniprogram.MapContext | null,
  lastQuery: null as MapQuery | null,

  locate() {
    wx.getFuzzyLocation({
      type: 'gcj02',
      success: (res) => {
        this.setData({ latitude: res.latitude, longitude: res.longitude })
        this.loadAround(res.latitude, res.longitude, this.data.scale)
      },
      fail: () => {
        this.loadAround(this.data.latitude, this.data.longitude, this.data.scale)
      },
    })
  },

  loadAround(latitude: number, longitude: number, zoom: number) {
    const span = zoom >= 15 ? 0.018 : 0.08
    const query: MapQuery = {
      cityCode: this.data.cityCode,
      swLat: latitude - span,
      swLng: longitude - span,
      neLat: latitude + span,
      neLng: longitude + span,
      zoom,
    }
    this.lastQuery = query
    this.setData({ loading: true, error: '' })
    getMap(query)
      .then((data) => {
        const markers: MapMarker[] = []
        const targets: MarkerTarget[] = []
        let total = 0
        let id = 1
        for (const cluster of data.clusters) {
          total += cluster.count
          markers.push(this.clusterMarker(id, cluster))
          targets.push({
            id,
            kind: 'cluster',
            latitude: cluster.lat,
            longitude: cluster.lng,
          })
          id += 1
        }
        for (const item of data.items) {
          total += 1
          markers.push(this.itemMarker(id, item))
          targets.push({
            id,
            kind: 'item',
            uuid: item.uuid,
            latitude: item.lat,
            longitude: item.lng,
          })
          id += 1
        }
        this.setData({ markers, markerTargets: targets, totalCount: total, loading: false })
      })
      .catch((err: Error) => {
        this.setData({ loading: false, error: err.message || '房源加载失败' })
      })
  },

  clusterMarker(id: number, cluster: MapCluster): MapMarker {
    return {
      id,
      latitude: cluster.lat,
      longitude: cluster.lng,
      iconPath: '../../assets/pin.png',
      width: 30,
      height: 38,
      callout: {
        content: `${cluster.count}套`,
        color: '#ffffff',
        bgColor: '#2563eb',
        borderRadius: 14,
        padding: 6,
        fontSize: 12,
        display: 'ALWAYS',
        textAlign: 'center',
      },
    }
  },

  itemMarker(id: number, item: MapItem): MapMarker {
    return {
      id,
      latitude: item.lat,
      longitude: item.lng,
      iconPath: '../../assets/pin.png',
      width: 30,
      height: 38,
      callout: {
        content: `${formatRent(item.rentCents)}`,
        color: '#0f172a',
        bgColor: '#ffffff',
        borderRadius: 12,
        padding: 6,
        fontSize: 12,
        display: 'ALWAYS',
        textAlign: 'center',
      },
    }
  },

  onRegionChange(e: { type: string }) {
    if (e.type !== 'end') {
      return
    }
    if (this.regionTimer) {
      clearTimeout(this.regionTimer)
    }
    this.regionTimer = setTimeout(() => {
      if (!this.mapContext) {
        this.mapContext = wx.createMapContext('housingMap', this)
      }
      this.mapContext.getRegion({
        success: (res: RegionResult) => {
          const zoom = this.data.scale
          const query: MapQuery = {
            cityCode: this.data.cityCode,
            swLat: res.southwest.latitude,
            swLng: res.southwest.longitude,
            neLat: res.northeast.latitude,
            neLng: res.northeast.longitude,
            zoom,
          }
          this.lastQuery = query
          this.loadQuery(query)
        },
      })
    }, 300)
  },

  loadQuery(query: MapQuery) {
    this.setData({ loading: true, error: '' })
    getMap(query)
      .then((data) => {
        const markers: MapMarker[] = []
        const targets: MarkerTarget[] = []
        let total = 0
        let id = 1
        for (const cluster of data.clusters) {
          total += cluster.count
          markers.push(this.clusterMarker(id, cluster))
          targets.push({ id, kind: 'cluster', latitude: cluster.lat, longitude: cluster.lng })
          id += 1
        }
        for (const item of data.items) {
          total += 1
          markers.push(this.itemMarker(id, item))
          targets.push({ id, kind: 'item', uuid: item.uuid, latitude: item.lat, longitude: item.lng })
          id += 1
        }
        this.setData({ markers, markerTargets: targets, totalCount: total, loading: false })
      })
      .catch((err: Error) => {
        this.setData({ loading: false, error: err.message || '房源加载失败' })
      })
  },

  onMarkerTap(e: { detail: { markerId: number } }) {
    const target = this.data.markerTargets.find((item) => item.id === e.detail.markerId)
    if (!target) {
      return
    }
    if (target.kind === 'cluster') {
      this.setData({ scale: 16, latitude: target.latitude, longitude: target.longitude })
      this.loadAround(target.latitude, target.longitude, 16)
      return
    }
    if (target.uuid) {
      this.openDetail(target.uuid)
    }
  },

  openDetail(uuid: string) {
    wx.showLoading({ title: '加载房源' })
    getProperty(uuid)
      .then((detail) => {
        this.setData({
          detail,
          showDetail: true,
          rentYuan: formatRent(detail.rentCents),
          imageCount: detail.images.length,
        })
      })
      .catch((err: Error) => {
        wx.showToast({ title: err.message || '加载失败', icon: 'none' })
      })
      .finally(() => wx.hideLoading())
  },

  closeDetail() {
    this.setData({ showDetail: false })
  },

  onContact() {
    const detail = this.data.detail
    if (!detail) {
      return
    }
    if (!this.data.hasToken) {
      wx.navigateTo({ url: '/pages/auth/index?next=contact' })
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
        this.submitContact(detail.uuid)
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

  onRobotStart() {
    this.setData({ showRobot: false })
    wx.navigateTo({ url: '/pages/assistant/index' })
  },

  onRobotClose() {
    this.setData({ showRobot: false })
  },

  onAuthTap() {
    wx.navigateTo({ url: '/pages/auth/index' })
  },

  onPublishTap() {
    if (!this.data.hasToken) {
      wx.navigateTo({ url: '/pages/auth/index' })
      return
    }
    wx.navigateTo({ url: '/pages/publish/index' })
  },

  onRetry() {
    if (this.lastQuery) {
      this.loadQuery(this.lastQuery)
    } else {
      this.loadAround(this.data.latitude, this.data.longitude, this.data.scale)
    }
  },
})

function formatRent(cents: number): string {
  const yuan = Math.round(cents / 100)
  if (yuan >= 10000) {
    return `${(yuan / 10000).toFixed(1)}万/月`
  }
  return `${yuan}元/月`
}
