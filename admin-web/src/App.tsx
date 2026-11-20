import { useEffect, useState, type FormEvent } from 'react'
import {
  adminIntakeText,
  adminLogin,
  enableTOTP,
  fetchAdminProperties,
  fetchAdminStats,
  moderateProperty,
  setupTOTP,
  type AdminLoginResult,
  type AdminProperty,
  type AdminStats,
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

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    Promise.all([fetchAdminStats(session.token), fetchAdminProperties(session.token)])
      .then(([nextStats, nextProperties]) => {
        if (!cancelled) {
          setStats(nextStats)
          setProperties(nextProperties)
          setError('')
        }
      })
      .catch((err: Error) => {
        if (!cancelled) {
          setError(err.message || '数据加载失败')
        }
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
          <p className="muted">粘贴房源文字，填写房东手机号；系统自动解析楼宇/面积/租金并匹配房东。</p>
          <div className="intake-grid">
            <textarea
              value={intakeText}
              onChange={(e) => setIntakeText(e.target.value)}
              placeholder={'楼宇：科兴科学园\n面积：110\n租金：20000\n户型：3+1'}
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

        <section className="panel" id="audit">
          <h2>安全状态</h2>
          <ul className="checklist">
            <li>管理员 token 与租客 token 使用不同 secret + issuer，互不通用</li>
            <li>登录成功写入 audit_logs（admin_login）</li>
            <li>管理接口走 RequireAdmin 中间件</li>
            <li>TOTP 已支持绑定与登录校验</li>
          </ul>
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
