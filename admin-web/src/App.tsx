import { useEffect, useState, type FormEvent } from 'react'
import {
  adminLogin,
  fetchAdminProperties,
  fetchAdminStats,
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
                  </tr>
                ))}
                {properties.length === 0 && !loading && (
                  <tr>
                    <td colSpan={5} className="empty">
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
            <li>TOTP 字段已预留；绑定/强制校验在 M2.1 接入</li>
          </ul>
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
