import { getToken } from '../../utils/session'

interface ImportResult {
  jobId: string
  totalRows: number
  successRows: number
  errorRows: number
}

interface ImportPageData {
  fileName: string
  filePath: string
  fileSize: string
  loading: boolean
  result: ImportResult | null
  error: string
}

Page<ImportPageData, WechatMiniprogram.Page.CustomOption>({
  data: {
    fileName: '',
    filePath: '',
    fileSize: '',
    loading: false,
    result: null,
    error: '',
  },

  onChoose() {
    wx.chooseMessageFile({
      count: 1,
      type: 'file',
      extension: ['docx', 'xlsx'],
      success: (res) => {
        const file = res.tempFiles[0]
        if (!file) {
          return
        }
        this.setData({
          fileName: file.name,
          filePath: file.path,
          fileSize: `${Math.round(file.size / 1024)} KB`,
          result: null,
          error: '',
        })
      },
      fail: () => this.setData({ error: '选择文件失败' }),
    })
  },

  onUpload() {
    if (!this.data.filePath) {
      this.setData({ error: '请先选择 docx 或 xlsx 文件' })
      return
    }
    const token = getToken()
    if (!token) {
      wx.navigateTo({ url: '/pages/auth/index' })
      return
    }
    const app = getApp<IAppOption>()
    this.setData({ loading: true, error: '', result: null })
    wx.uploadFile({
      url: `${app.globalData.apiBase}/v1/landlord/properties/imports`,
      filePath: this.data.filePath,
      name: 'file',
      header: { Authorization: `Bearer ${token}` },
      success: (res: { statusCode: number; data: string }) => {
        try {
          const body = JSON.parse(res.data) as {
            code: number
            message: string
            data?: ImportResult
          }
          if (res.statusCode >= 200 && res.statusCode < 300 && body.code === 0 && body.data) {
            this.setData({ result: body.data, loading: false })
            return
          }
          this.setData({ loading: false, error: body.message || '导入失败' })
        } catch {
          this.setData({ loading: false, error: '导入响应解析失败' })
        }
      },
      fail: (err: { errMsg: string }) => {
        this.setData({ loading: false, error: err.errMsg || '上传失败' })
      },
    })
  },
})
