import { useEffect, useState } from 'react'
import { Link, NavLink, Route, Routes } from 'react-router-dom'
import { fetchHealth, type Health } from './api'

type HealthState =
  | { kind: 'loading' }
  | { kind: 'ok'; body: Health }
  | { kind: 'error'; message: string }

function Dashboard() {
  const [health, setHealth] = useState<HealthState>({ kind: 'loading' })

  useEffect(() => {
    let cancelled = false
    fetchHealth()
      .then((body) => {
        if (!cancelled) {
          setHealth({ kind: 'ok', body })
        }
      })
      .catch((error: Error) => {
        if (!cancelled) {
          setHealth({ kind: 'error', message: error.message })
        }
      })
    return () => {
      cancelled = true
    }
  }, [])

  return (
    <main className="main">
      <header className="page-head">
        <div>
          <p className="eyebrow">M1 · 工程底座</p>
          <h1>运营控制台</h1>
          <p className="lede">
            当前版本已打通开发态登录、资料、房源地图与详情。管理站鉴权、录入与审核在后续里程碑接入。
          </p>
        </div>
        <div className={`health health-${health.kind}`}>
          <span className="health-dot" />
          {health.kind === 'loading' && '检查 API…'}
          {health.kind === 'ok' && `API ${health.body.status}`}
          {health.kind === 'error' && `API 离线：${health.message}`}
        </div>
      </header>

      <section className="stats" aria-label="核心指标">
        <article className="stat">
          <span className="stat-label">在租房源</span>
          <strong className="stat-value">5</strong>
          <span className="stat-note">开发种子数据</span>
        </article>
        <article className="stat">
          <span className="stat-label">覆盖楼宇</span>
          <strong className="stat-value">4</strong>
          <span className="stat-note">南山 / 福田</span>
        </article>
        <article className="stat">
          <span className="stat-label">联系请求</span>
          <strong className="stat-value">0</strong>
          <span className="stat-note">M3 接入</span>
        </article>
        <article className="stat">
          <span className="stat-label">待审核</span>
          <strong className="stat-value">0</strong>
          <span className="stat-note">管理审核流待接入</span>
        </article>
      </section>

      <section className="grid">
        <article className="panel">
          <h2>M1 已交付</h2>
          <ul className="checklist">
            <li>Go + Gin + pgx 分层骨架</li>
            <li>PostgreSQL 18 迁移与种子数据</li>
            <li>开发态微信登录 / JWT</li>
            <li>手机号加密、姓氏与角色资料</li>
            <li>地图聚合 / 房源详情 API</li>
            <li>微信原生地图首屏 + 登录页</li>
          </ul>
        </article>
        <article className="panel">
          <h2>下一阶段</h2>
          <ul className="checklist muted-list">
            <li>M2：管理站登录、房源发布、图片与楼宇词典</li>
            <li>M3：联系确认、服务通知、明文联系方式</li>
            <li>M4：小亿语音与逐套解说</li>
            <li>M5：文档 / 截图导入</li>
          </ul>
        </article>
      </section>

      <section className="panel notice">
        <h2>安全提示</h2>
        <p>
          当前管理站是<strong>只读预览骨架</strong>，没有接入管理端鉴权，不能直接部署到公网。M2 会先补
          TOTP、RBAC、HttpOnly 会话与审计，再开放任何写操作。
        </p>
      </section>
    </main>
  )
}

function LoginInfo() {
  return (
    <main className="main">
      <header className="page-head">
        <div>
          <p className="eyebrow">M1</p>
          <h1>管理站登录</h1>
          <p className="lede">管理端鉴权尚未接入。本页只说明状态，不提供假登录。</p>
        </div>
      </header>
      <section className="panel">
        <h2>为什么不能直接登录？</h2>
        <p>
          M1 只完成了租客端与公开房源 API。管理站写操作需要独立的 JWT audience、TOTP、RBAC
          和审计；在这些完成前开放登录会形成安全缺口。
        </p>
        <Link className="btn" to="/">
          返回控制台
        </Link>
      </section>
    </main>
  )
}

function Shell() {
  return (
    <div className="shell">
      <aside className="sidebar">
        <div className="brand">
          <div className="brand-mark">亿</div>
          <div>
            <strong>房屋中介</strong>
            <span>管理站 · v0.1</span>
          </div>
        </div>
        <nav className="nav">
          <NavLink to="/" end>
            控制台
          </NavLink>
          <NavLink to="/login">登录</NavLink>
        </nav>
        <div className="sidebar-foot">
          <span className="pill">M1</span>
          <span>开发环境</span>
        </div>
      </aside>
      <div className="workspace">
        <div className="preview-banner">
          预览骨架 · 管理鉴权未接入 · 请勿公网部署
        </div>
        <Routes>
          <Route path="/" element={<Dashboard />} />
          <Route path="/login" element={<LoginInfo />} />
        </Routes>
      </div>
    </div>
  )
}

export default Shell
