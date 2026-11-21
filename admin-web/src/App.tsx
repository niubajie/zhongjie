import { useEffect, useState, type FormEvent } from 'react'
import {
  adminIntakeMedia,
  adminIntakeText,
  adminLogin,
  createCommunity,
  matchIntakeLandlord,
  publishIntake,
  enableTOTP,
  fetchAdminAuditLogs,
  fetchAdminBuildings,
  fetchAdminContactDetail,
  fetchAdminContacts,
  fetchAdminFreshness,
  fetchAdminImports,
  fetchAdminIntakeDetail,
  fetchAdminIntakeDrafts,
  fetchAdminProperties,
  fetchAdminStats,
  moderateProperty,
  retryAdminIntake,
  setupTOTP,
  suggestCommunities,
  type AdminAuditLog,
  type AdminBuilding,
  type AdminContact,
  type AdminContactDetail,
  type AdminFreshness,
  type AdminImportJob,
  type AdminIntakeDraft,
  type AdminIntakeMedia,
  type AdminLoginResult,
  type AdminProperty,
  type AdminStats,
  type CommunitySuggestion,
} from './api'

type Session = Pick<AdminLoginResult, 'token' | 'admin'>

function Login({ onLogin }: { onLogin: (session: Session) => void }) {
  const [username, setUsername] = useState('admin')
  const [password, setPassword] = useState('')
  const [totpCode, setTotpCode] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setError('')
    setLoading(true)
    adminLogin(username.trim(), password, totpCode.trim())
      .then((result) => onLogin({ token: result.token, admin: result.admin }))
      .catch((err: Error) => setError(err.message || '登录失败'))
      .finally(() => setLoading(false))
  }

  return (
    <div className="login-page">
      <div className="login-card">
        <div className="login-brand">
          <div className="brand-mark">亿</div>
          <div>
            <h1>管理站登录</h1>
            <p>房屋中介 · 运营与审核</p>
          </div>
        </div>

        <form onSubmit={submit} className="login-form">
          <label>
            用户名
            <input value={username} onChange={(e) => setUsername(e.target.value)} autoComplete="username" />
          </label>
          <label>
            密码
            <input
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              autoComplete="current-password"
              placeholder="开发种子：Admin123!"
            />
          </label>
          <label>
            动态验证码（如已开启 TOTP）
            <input
              value={totpCode}
              onChange={(e) => setTotpCode(e.target.value)}
              inputMode="numeric"
              placeholder="可留空"
            />
          </label>
          {error && <p className="form-error">{error}</p>}
          <button className="btn primary" type="submit" disabled={loading}>
            {loading ? '登录中…' : '登录'}
          </button>
        </form>

        <p className="login-note">
          管理端 token 只放内存，刷新页面需要重新登录；不使用 localStorage。
        </p>
      </div>
    </div>
  )
}

function Dashboard({ session, onLogout }: { session: Session; onLogout: () => void }) {
  const [stats, setStats] = useState<AdminStats | null>(null)
  const [properties, setProperties] = useState<AdminProperty[]>([])
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(true)
  const [totpSecret, setTotpSecret] = useState('')
  const [totpURL, setTotpURL] = useState('')
  const [totpCode, setTotpCode] = useState('')
  const [totpMessage, setTotpMessage] = useState('')
  const [intakeText, setIntakeText] = useState('')
  const [intakePhone, setIntakePhone] = useState('')
  const [intakeMessage, setIntakeMessage] = useState('')
  const [intakeLoading, setIntakeLoading] = useState(false)
  const [imports, setImports] = useState<AdminImportJob[]>([])
  const [buildings, setBuildings] = useState<AdminBuilding[]>([])
  const [contacts, setContacts] = useState<AdminContact[]>([])
  const [freshness, setFreshness] = useState<AdminFreshness[]>([])
  const [drafts, setDrafts] = useState<AdminIntakeDraft[]>([])
  const [auditLogs, setAuditLogs] = useState<AdminAuditLog[]>([])
  const [contactDetail, setContactDetail] = useState<AdminContactDetail | null>(null)
  const [communityOptions, setCommunityOptions] = useState<CommunitySuggestion[]>([])
  const [communityMessage, setCommunityMessage] = useState('')
  const [newCommunity, setNewCommunity] = useState({
    name: '',
    districtCode: '440305',
    districtName: '南山区',
    streetCode: '440305006',
    streetName: '粤海街道',
    communityType: 'residential' as 'residential' | 'apartment',
    lat: '',
    lng: '',
  })
  const [mediaFile, setMediaFile] = useState<File | null>(null)
  const [mediaResult, setMediaResult] = useState<AdminIntakeMedia | null>(null)
  const [mediaLoading, setMediaLoading] = useState(false)
  const [mediaMessage, setMediaMessage] = useState('')
  const [draft, setDraft] = useState({
    landlordId: '',
    communityUuid: '',
    communityName: '',
    areaSqm: '',
    rentCents: '',
    title: '',
    layout: '',
    matchReason: '房东授权补充',
  })

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    Promise.allSettled([
      fetchAdminStats(session.token),
      fetchAdminProperties(session.token),
      fetchAdminImports(session.token),
      fetchAdminBuildings(session.token),
      fetchAdminContacts(session.token),
      fetchAdminFreshness(session.token),
      fetchAdminIntakeDrafts(session.token),
      fetchAdminAuditLogs(session.token),
    ])
      .then((results) => {
        if (cancelled) {
          return
        }
        const [statsR, propertiesR, importsR, buildingsR, contactsR, freshnessR, draftsR, auditR] =
          results
        if (statsR.status === 'fulfilled') setStats(statsR.value)
        if (propertiesR.status === 'fulfilled') setProperties(propertiesR.value)
        if (importsR.status === 'fulfilled') setImports(importsR.value)
        if (buildingsR.status === 'fulfilled') setBuildings(buildingsR.value)
        if (contactsR.status === 'fulfilled') setContacts(contactsR.value)
        if (freshnessR.status === 'fulfilled') setFreshness(freshnessR.value)
        if (draftsR.status === 'fulfilled') setDrafts(draftsR.value)
        if (auditR.status === 'fulfilled') setAuditLogs(auditR.value)
        const failed = results.filter((result) => result.status === 'rejected')
        setError(failed.length > 0 ? `${failed.length} 个区块加载失败，其余数据仍可查看` : '')
      })
      .finally(() => {
        if (!cancelled) {
          setLoading(false)
        }
      })
    return () => {
      cancelled = true
    }
  }, [session.token])

  function toggleProperty(property: AdminProperty) {
    const action = property.status === 'active' ? 'offline' : 'approve'
    moderateProperty(session.token, property.uuid, action)
      .then((status) => {
        setProperties((prev) =>
          prev.map((item) => (item.uuid === property.uuid ? { ...item, status } : item)),
        )
      })
      .catch((err: Error) => setError(err.message || '操作失败'))
  }

  function submitIntake() {
    setIntakeLoading(true)
    setIntakeMessage('')
    adminIntakeText(session.token, intakeText, intakePhone.trim())
      .then((result) => {
        setIntakeMessage(
          `已创建 ${result.buildingName || '房源'}，状态 ${result.status}，房源 ${result.propertyId || ''}`,
        )
        setIntakeText('')
        return fetchAdminProperties(session.token)
      })
      .then((next) => setProperties(next))
      .catch((err: Error) => setIntakeMessage(err.message || '录入失败'))
      .finally(() => setIntakeLoading(false))
  }

  function applyMediaResult(result: AdminIntakeMedia) {
    setMediaResult(result)
    setDraft({
      landlordId: result.landlordId ?? '',
      communityUuid: '',
      communityName: result.extracted.communityName ?? '',
      areaSqm: result.extracted.areaSqm ? String(result.extracted.areaSqm) : '',
      rentCents: result.extracted.rentYuan ? String(result.extracted.rentYuan) : '',
      title: result.extracted.title ?? '',
      layout: result.extracted.layout ?? '',
      matchReason: '房东授权补充',
    })
  }

  function submitMedia() {
    if (!mediaFile) {
      setMediaMessage('请先选择图片、视频或文档')
      return
    }
    setMediaLoading(true)
    setMediaMessage('')
    adminIntakeMedia(session.token, mediaFile)
      .then((result) => {
        applyMediaResult(result)
        setMediaMessage(result.message || `识别完成，状态 ${result.status}`)
        return refreshDrafts()
      })
      .catch((err: Error) => setMediaMessage(err.message || '上传失败'))
      .finally(() => setMediaLoading(false))
  }

  function refreshDrafts() {
    return fetchAdminIntakeDrafts(session.token).then((next) => setDrafts(next))
  }

  function openDraft(item: AdminIntakeDraft) {
    setMediaLoading(true)
    setMediaMessage('')
    fetchAdminIntakeDetail(session.token, item.uuid)
      .then((result) => {
        applyMediaResult(result)
        setMediaMessage(`已载入草稿 ${item.uuid}`)
      })
      .catch((err: Error) => setMediaMessage(err.message || '草稿加载失败'))
      .finally(() => setMediaLoading(false))
  }

  function retryDraft(item: AdminIntakeDraft) {
    setMediaLoading(true)
    setMediaMessage('')
    retryAdminIntake(session.token, item.uuid)
      .then((result) => {
        applyMediaResult(result)
        setMediaMessage(result.message || `已重试，状态 ${result.status}`)
        return refreshDrafts()
      })
      .catch((err: Error) => setMediaMessage(err.message || '重试失败'))
      .finally(() => setMediaLoading(false))
  }

  function onCommunityNameChange(value: string) {
    setDraft({ ...draft, communityName: value, communityUuid: '' })
    setCommunityMessage('')
    const keyword = value.trim()
    if (keyword.length < 2) {
      setCommunityOptions([])
      return
    }
    suggestCommunities(keyword)
      .then(setCommunityOptions)
      .catch(() => setCommunityOptions([]))
  }

  function pickCommunity(item: CommunitySuggestion) {
    setDraft({ ...draft, communityUuid: item.uuid, communityName: item.name })
    setCommunityOptions([])
    setCommunityMessage(`已选择 ${item.name}（${item.districtName} · ${item.streetName}）`)
  }

  function submitNewCommunity() {
    const name = newCommunity.name.trim()
    const lat = Number(newCommunity.lat)
    const lng = Number(newCommunity.lng)
    if (!name) {
      setCommunityMessage('请填写小区名称')
      return
    }
    if (!Number.isFinite(lat) || !Number.isFinite(lng) || lat === 0 || lng === 0) {
      setCommunityMessage('请填写有效经纬度，例如 22.5178 / 113.9356')
      return
    }
    setCommunityMessage('')
    createCommunity(session.token, {
      cityCode: '440300',
      districtCode: newCommunity.districtCode.trim(),
      districtName: newCommunity.districtName.trim(),
      streetCode: newCommunity.streetCode.trim(),
      streetName: newCommunity.streetName.trim(),
      name,
      communityType: newCommunity.communityType,
      lat,
      lng,
    })
      .then((created) => {
        setDraft({ ...draft, communityUuid: created.uuid, communityName: created.name })
        setNewCommunity({ ...newCommunity, name: '', lat: '', lng: '' })
        setCommunityOptions([])
        setCommunityMessage(`已创建小区 ${created.name}，可直接发布`)
      })
      .catch((err: Error) => setCommunityMessage(err.message || '创建小区失败'))
  }

  function showContactDetail(contact: AdminContact) {
    setContactDetail(null)
    fetchAdminContactDetail(session.token, contact.uuid)
      .then(setContactDetail)
      .catch((err: Error) => setError(err.message || '联系记录加载失败'))
  }

  function bindLandlord() {
    if (!mediaResult) return
    if (!draft.landlordId.trim()) {
      setMediaMessage('请填写房东 UUID')
      return
    }
    matchIntakeLandlord(session.token, mediaResult.jobId, draft.landlordId.trim(), draft.matchReason)
      .then(() => setMediaMessage('房东已绑定'))
      .catch((err: Error) => setMediaMessage(err.message || '绑定失败'))
  }

  function publishDraft() {
    if (!mediaResult) return
    const area = Number(draft.areaSqm)
    const rentYuan = Number(draft.rentCents)
    if (!Number.isFinite(area) || area <= 0 || !Number.isFinite(rentYuan) || rentYuan <= 0) {
      setMediaMessage('请填写有效面积和月租')
      return
    }
    if (!draft.landlordId.trim()) {
      setMediaMessage('请先绑定房东')
      return
    }
    setMediaLoading(true)
    publishIntake(session.token, mediaResult.jobId, {
      communityUuid: draft.communityUuid.trim() || undefined,
      communityName: draft.communityName.trim() || undefined,
      areaSqm: area,
      rentCents: Math.round(rentYuan * 100),
      title: draft.title.trim(),
      layout: draft.layout.trim(),
      landlordId: draft.landlordId.trim(),
    })
      .then((result) => {
        setMediaMessage(`已发布，房源 ${result.propertyId || ''}，状态 ${result.status}`)
        setMediaResult(null)
        setMediaFile(null)
        return fetchAdminProperties(session.token)
      })
      .then((next) => {
        setProperties(next)
        return refreshDrafts()
      })
      .catch((err: Error) => setMediaMessage(err.message || '发布失败'))
      .finally(() => setMediaLoading(false))
  }

  function beginTOTP() {
    setTotpMessage('')
    setupTOTP(session.token)
      .then((result) => {
        setTotpSecret(result.secret)
        setTotpURL(result.otpauthUrl)
      })
      .catch((err: Error) => setTotpMessage(err.message || '生成失败'))
  }

  function confirmTOTP() {
    enableTOTP(session.token, totpCode.trim())
      .then(() => {
        setTotpSecret('')
        setTotpURL('')
        setTotpCode('')
        setTotpMessage('TOTP 已启用，下次登录需要动态验证码。')
      })
      .catch((err: Error) => setTotpMessage(err.message || '启用失败'))
  }

  return (
    <div className="shell">
      <aside className="sidebar">
        <div className="brand">
          <div className="brand-mark">亿</div>
          <div>
            <strong>房屋中介</strong>
            <span>管理站 · M2</span>
          </div>
        </div>
        <nav className="nav">
          <a className="active" href="#dashboard">
            控制台
          </a>
          <a href="#properties">房源</a>
          <a href="#audit">审计</a>
        </nav>
        <div className="sidebar-foot">
          <span className="pill">{session.admin.role}</span>
          <span>{session.admin.username}</span>
          <button className="logout" onClick={onLogout}>
            退出
          </button>
        </div>
      </aside>

      <main className="main">
        <header className="page-head">
          <div>
            <p className="eyebrow">M2 · 管理站鉴权</p>
            <h1>运营控制台</h1>
            <p className="lede">独立管理员 JWT、RBAC 角色、登录审计，数据只读展示。</p>
          </div>
          <span className="badge">独立 issuer · housing-admin</span>
        </header>

        {error && <div className="alert">{error}</div>}

        <section className="stats">
          <Stat label="在租房源" value={stats?.activeProperties ?? '—'} note="active" />
          <Stat label="待审核" value={stats?.pendingProperties ?? '—'} note="pending" />
          <Stat label="用户" value={stats?.users ?? '—'} note="租客 + 房东" />
          <Stat label="房东" value={stats?.landlords ?? '—'} note="landlord" />
          <Stat label="联系请求" value={stats?.contactRequests ?? '—'} note="已落库" />
        </section>

        <section className="panel" id="intake">
          <h2>管理站租房录入</h2>
          <p className="muted">粘贴房源文字或上传图片/视频/文档；系统自动解析小区、面积、租金并匹配房东。</p>
          <div className="intake-grid">
            <textarea
              value={intakeText}
              onChange={(e) => setIntakeText(e.target.value)}
              placeholder={'小区：海岸城花园\n面积：89\n租金：7800\n户型：3室2厅\n电话：13800000001'}
            />
            <div className="intake-side">
              <input
                value={intakePhone}
                onChange={(e) => setIntakePhone(e.target.value)}
                placeholder="房东手机号"
                inputMode="tel"
              />
              <button className="btn primary" onClick={submitIntake} disabled={intakeLoading}>
                {intakeLoading ? '录入中…' : '解析并创建'}
              </button>
              {intakeMessage && <p className="muted">{intakeMessage}</p>}
            </div>
          </div>

          <h3 className="sub-head">图片 / 视频 / 文档录入</h3>
          <div className="intake-grid">
            <div className="intake-side">
              <input
                type="file"
                accept="image/jpeg,image/png,image/webp,video/mp4,video/quicktime,.docx,.txt,.md"
                onChange={(e) => setMediaFile(e.target.files?.[0] ?? null)}
              />
              <p className="muted">支持 jpg/png/webp、mp4/mov，以及 docx/txt/md（PDF 与旧版 .doc 暂不支持）。</p>
              <button className="btn primary" onClick={submitMedia} disabled={mediaLoading}>
                {mediaLoading ? '识别中…' : '上传并识别'}
              </button>
              {mediaMessage && <p className="muted">{mediaMessage}</p>}
            </div>
            {mediaResult && (
              <div className="draft-card">
                <p className="muted">
                  状态 {mediaResult.status} · 匹配 {mediaResult.matchStatus}
                  {mediaResult.maskedPhone ? ` · 电话 ${mediaResult.maskedPhone}` : ''}
                  {mediaResult.confidence > 0 ? ` · 置信度 ${Math.round(mediaResult.confidence * 100)}%` : ''}
                </p>
                {mediaResult.mediaUrls.map((url) => {
                  const lower = url.toLowerCase()
                  if (lower.endsWith('.mp4') || lower.endsWith('.mov') || lower.endsWith('.m4v')) {
                    return <video key={url} src={url} controls style={{ width: '100%', maxHeight: 220 }} />
                  }
                  if (/\.(jpg|jpeg|png|webp)$/.test(lower)) {
                    return (
                      <img
                        key={url}
                        src={url}
                        alt="录入素材"
                        style={{ width: '100%', maxHeight: 220, objectFit: 'contain' }}
                      />
                    )
                  }
                  return (
                    <p key={url}>
                      <a href={url} target="_blank" rel="noreferrer">
                        查看原始文档
                      </a>
                    </p>
                  )
                })}
                {mediaResult.ocrText && (
                  <details>
                    <summary>识别原文</summary>
                    <pre style={{ whiteSpace: 'pre-wrap', maxHeight: 200, overflow: 'auto' }}>
                      {mediaResult.ocrText}
                    </pre>
                  </details>
                )}
                <div className="draft-fields">
                  <input
                    value={draft.landlordId}
                    onChange={(e) => setDraft({ ...draft, landlordId: e.target.value })}
                    placeholder="房东 UUID（自动匹配后可确认）"
                  />
                  <div className="community-field">
                    <input
                      value={draft.communityName}
                      onChange={(e) => onCommunityNameChange(e.target.value)}
                      placeholder="小区名称（输入 2 个字自动联想）"
                    />
                    {communityOptions.length > 0 && (
                      <div className="community-suggest">
                        {communityOptions.map((item) => (
                          <button key={item.uuid} type="button" onClick={() => pickCommunity(item)}>
                            <span>{item.name}</span>
                            <span className="muted">
                              {item.districtName} · {item.streetName}
                            </span>
                          </button>
                        ))}
                      </div>
                    )}
                  </div>
                  <input
                    value={draft.communityUuid}
                    onChange={(e) => setDraft({ ...draft, communityUuid: e.target.value })}
                    placeholder="小区 UUID（选择后自动填充）"
                  />
                  <input
                    value={draft.areaSqm}
                    onChange={(e) => setDraft({ ...draft, areaSqm: e.target.value })}
                    placeholder="面积（㎡）"
                    inputMode="decimal"
                  />
                  <input
                    value={draft.rentCents}
                    onChange={(e) => setDraft({ ...draft, rentCents: e.target.value })}
                    placeholder="月租（元）"
                    inputMode="decimal"
                  />
                  <input
                    value={draft.layout}
                    onChange={(e) => setDraft({ ...draft, layout: e.target.value })}
                    placeholder="户型"
                  />
                  <input
                    value={draft.title}
                    onChange={(e) => setDraft({ ...draft, title: e.target.value })}
                    placeholder="标题（可留空自动生成）"
                  />
                </div>
                <details className="community-new">
                  <summary>小区不存在？新建小区</summary>
                  <div className="draft-fields">
                    <input
                      value={newCommunity.name}
                      onChange={(e) => setNewCommunity({ ...newCommunity, name: e.target.value })}
                      placeholder="小区名称"
                    />
                    <input
                      value={newCommunity.districtName}
                      onChange={(e) => setNewCommunity({ ...newCommunity, districtName: e.target.value })}
                      placeholder="区名，如 南山区"
                    />
                    <input
                      value={newCommunity.districtCode}
                      onChange={(e) => setNewCommunity({ ...newCommunity, districtCode: e.target.value })}
                      placeholder="区代码，如 440305"
                    />
                    <input
                      value={newCommunity.streetName}
                      onChange={(e) => setNewCommunity({ ...newCommunity, streetName: e.target.value })}
                      placeholder="街道名，如 粤海街道"
                    />
                    <input
                      value={newCommunity.streetCode}
                      onChange={(e) => setNewCommunity({ ...newCommunity, streetCode: e.target.value })}
                      placeholder="街道代码，如 440305006"
                    />
                    <select
                      value={newCommunity.communityType}
                      onChange={(e) =>
                        setNewCommunity({
                          ...newCommunity,
                          communityType: e.target.value === 'apartment' ? 'apartment' : 'residential',
                        })
                      }
                    >
                      <option value="residential">住宅小区</option>
                      <option value="apartment">公寓</option>
                    </select>
                    <input
                      value={newCommunity.lat}
                      onChange={(e) => setNewCommunity({ ...newCommunity, lat: e.target.value })}
                      placeholder="纬度，如 22.5178"
                      inputMode="decimal"
                    />
                    <input
                      value={newCommunity.lng}
                      onChange={(e) => setNewCommunity({ ...newCommunity, lng: e.target.value })}
                      placeholder="经度，如 113.9356"
                      inputMode="decimal"
                    />
                  </div>
                  <button className="btn" onClick={submitNewCommunity}>
                    创建并选中
                  </button>
                </details>
                {communityMessage && <p className="muted">{communityMessage}</p>}
                <div className="draft-actions">
                  <button className="btn" onClick={bindLandlord} disabled={mediaLoading}>
                    绑定房东
                  </button>
                  <button className="btn primary" onClick={publishDraft} disabled={mediaLoading}>
                    确认发布
                  </button>
                </div>
              </div>
            )}
          </div>
        </section>

        <section className="panel" id="intake-drafts">
          <div className="panel-head">
            <h2>录入草稿</h2>
            <span className="muted">{drafts.length} 条</span>
          </div>
          <p className="muted">
            图片/视频识别产生的草稿在这里查看；识别失败或未配置模型的草稿，配置凭证后可重试。
          </p>
          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>类型</th>
                  <th>小区</th>
                  <th>面积</th>
                  <th>月租</th>
                  <th>状态</th>
                  <th>房东</th>
                  <th>时间</th>
                  <th>操作</th>
                </tr>
              </thead>
              <tbody>
                {drafts.map((item) => (
                  <tr key={item.uuid}>
                    <td>
                      {item.inputType === 'video'
                        ? '视频'
                        : item.inputType === 'image'
                          ? '图片'
                          : item.inputType === 'doc'
                            ? '文档'
                            : '文字'}
                    </td>
                    <td>{item.communityName || '—'}</td>
                    <td>{item.areaSqm ? `${item.areaSqm}㎡` : '—'}</td>
                    <td>{item.rentYuan ? `${item.rentYuan} 元/月` : '—'}</td>
                    <td>
                      <span className={`status status-${item.status}`}>{item.status}</span>
                      {item.errorMessage ? <span className="muted"> · {item.errorMessage}</span> : null}
                    </td>
                    <td>{item.landlordId ? item.landlordId.slice(0, 8) : '未匹配'}</td>
                    <td>{item.createdAt}</td>
                    <td>
                      <button className="table-action" onClick={() => openDraft(item)} disabled={mediaLoading}>
                        查看
                      </button>
                      {(item.status === 'failed' || item.status === 'not_configured') && (
                        <button className="table-action" onClick={() => retryDraft(item)} disabled={mediaLoading}>
                          重试
                        </button>
                      )}
                    </td>
                  </tr>
                ))}
                {drafts.length === 0 && (
                  <tr>
                    <td colSpan={8} className="empty">
                      暂无草稿
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>
        </section>

        <section className="panel" id="properties">
          <div className="panel-head">
            <h2>最近房源</h2>
            <span className="muted">{loading ? '加载中…' : `共 ${properties.length} 条`}</span>
          </div>
          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>标题</th>
                  <th>区域</th>
                  <th>面积</th>
                  <th>月租</th>
                  <th>状态</th>
                  <th>操作</th>
                </tr>
              </thead>
              <tbody>
                {properties.map((property) => (
                  <tr key={property.uuid}>
                    <td>{property.title}</td>
                    <td>
                      {property.districtName} · {property.streetName}
                    </td>
                    <td>{property.areaSqm}㎡</td>
                    <td>{formatRent(property.rentCents)}</td>
                    <td>
                      <span className={`status status-${property.status}`}>{property.status}</span>
                    </td>
                    <td>
                      <button className="table-action" onClick={() => toggleProperty(property)}>
                        {property.status === 'active' ? '下架' : '上架'}
                      </button>
                    </td>
                  </tr>
                ))}
                {properties.length === 0 && !loading && (
                  <tr>
                    <td colSpan={6} className="empty">
                      暂无房源
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>
        </section>

        <section className="panel" id="imports">
          <h2>导入任务</h2>
          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>文件</th>
                  <th>类型</th>
                  <th>状态</th>
                  <th>成功 / 失败</th>
                  <th>时间</th>
                </tr>
              </thead>
              <tbody>
                {imports.map((job) => (
                  <tr key={job.uuid}>
                    <td>{job.fileName}</td>
                    <td>{job.fileType}</td>
                    <td>{job.status}</td>
                    <td>
                      {job.successRows} / {job.errorRows}
                    </td>
                    <td>{job.createdAt}</td>
                  </tr>
                ))}
                {imports.length === 0 && (
                  <tr>
                    <td colSpan={5} className="empty">
                      暂无导入任务
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>
        </section>

        <section className="panel" id="buildings">
          <h2>楼宇词典</h2>
          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>楼宇</th>
                  <th>区域</th>
                  <th>类型</th>
                  <th>审核</th>
                </tr>
              </thead>
              <tbody>
                {buildings.map((building) => (
                  <tr key={building.uuid}>
                    <td>{building.name}</td>
                    <td>
                      {building.districtName} · {building.streetName}
                    </td>
                    <td>{building.buildingType}</td>
                    <td>{building.reviewStatus}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </section>

        <section className="panel" id="contacts">
          <h2>联系记录</h2>
          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>租客</th>
                  <th>手机号</th>
                  <th>房源</th>
                  <th>状态</th>
                  <th>时间</th>
                  <th>操作</th>
                </tr>
              </thead>
              <tbody>
                {contacts.map((contact) => (
                  <tr key={contact.uuid}>
                    <td>{contact.tenantSurname || '—'}</td>
                    <td>{contact.tenantPhoneMask}</td>
                    <td>{contact.propertyTitle}</td>
                    <td>{contact.status}</td>
                    <td>{contact.createdAt}</td>
                    <td>
                      <button className="table-action" onClick={() => showContactDetail(contact)}>
                        详情
                      </button>
                    </td>
                  </tr>
                ))}
                {contacts.length === 0 && (
                  <tr>
                    <td colSpan={6} className="empty">
                      暂无联系记录
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>
          {contactDetail && (
            <div className="draft-card">
              <p className="muted">联系记录 {contactDetail.uuid}</p>
              <p>
                租客 {contactDetail.tenantSurname || '—'} · {contactDetail.tenantPhoneMask} · 状态{' '}
                {contactDetail.status}
              </p>
              <p>房源：{contactDetail.propertyTitle}</p>
              <p>时间：{contactDetail.createdAt}</p>
              {contactDetail.note && <p>备注：{contactDetail.note}</p>}
            </div>
          )}
        </section>

        <section className="panel" id="freshness">
          <h2>30 天巡检</h2>
          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>房源</th>
                  <th>状态</th>
                  <th>生成时间</th>
                </tr>
              </thead>
              <tbody>
                {freshness.map((check) => (
                  <tr key={check.id}>
                    <td>{check.title}</td>
                    <td>{check.status}</td>
                    <td>{check.createdAt}</td>
                  </tr>
                ))}
                {freshness.length === 0 && (
                  <tr>
                    <td colSpan={3} className="empty">
                      暂无巡检任务
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>
        </section>

        <section className="panel" id="audit">
          <h2>安全状态</h2>
          <ul className="checklist">
            <li>管理员 token 与租客 token 使用不同 secret + issuer，互不通用</li>
            <li>登录成功写入 audit_logs（admin_login）</li>
            <li>管理接口走 RequireAdmin 中间件</li>
            <li>TOTP 已支持绑定与登录校验</li>
          </ul>
          <h3 className="sub-head">审计日志</h3>
          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>动作</th>
                  <th>操作者</th>
                  <th>对象</th>
                  <th>时间</th>
                </tr>
              </thead>
              <tbody>
                {auditLogs.map((log) => (
                  <tr key={log.id}>
                    <td>{log.action}</td>
                    <td>{log.actorType}</td>
                    <td>
                      {log.targetType}
                      {log.targetId ? ` · ${log.targetId}` : ''}
                    </td>
                    <td>{log.createdAt}</td>
                  </tr>
                ))}
                {auditLogs.length === 0 && (
                  <tr>
                    <td colSpan={4} className="empty">
                      暂无审计日志
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>
          <div className="totp-box">
            {!totpSecret && (
              <button className="btn secondary" onClick={beginTOTP}>
                设置动态验证码
              </button>
            )}
            {totpSecret && (
              <>
                <p className="muted">
                  把下面密钥加入验证器，或打开 otpauth URL；然后输入 6 位验证码。
                </p>
                <code className="secret">{totpSecret}</code>
                <code className="secret small">{totpURL}</code>
                <div className="totp-row">
                  <input
                    value={totpCode}
                    onChange={(e) => setTotpCode(e.target.value)}
                    placeholder="6 位验证码"
                    inputMode="numeric"
                  />
                  <button className="btn primary" onClick={confirmTOTP}>
                    启用
                  </button>
                </div>
              </>
            )}
            {totpMessage && <p className="muted">{totpMessage}</p>}
          </div>
        </section>
      </main>
    </div>
  )
}

function Stat({ label, value, note }: { label: string; value: number | string; note: string }) {
  return (
    <article className="stat">
      <span className="stat-label">{label}</span>
      <strong className="stat-value">{value}</strong>
      <span className="stat-note">{note}</span>
    </article>
  )
}

function formatRent(cents: number): string {
  const yuan = Math.round(cents / 100)
  return yuan >= 10000 ? `${(yuan / 10000).toFixed(1)} 万/月` : `${yuan} 元/月`
}

function App() {
  const [session, setSession] = useState<Session | null>(null)
  if (!session) {
    return <Login onLogin={setSession} />
  }
  return <Dashboard session={session} onLogout={() => setSession(null)} />
}

export default App
