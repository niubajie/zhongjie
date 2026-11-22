import { useEffect, useState, type FormEvent } from 'react'
import {
  addProspectFollowUp,
  contactProspect,
  createLeadSource,
  deleteLeadSource,
  fetchCollectRuns,
  fetchKillSwitch,
  fetchLeadSources,
  fetchOptOuts,
  fetchProspectDetail,
  fetchProspectNotifications,
  fetchProspectReminders,
  fetchProspects,
  fetchProspectStats,
  fetchReviewQueue,
  markProspectNotificationRead,
  optOutProspect,
  reviewProspect,
  setKillSwitch,
  setLeadSourceEnabled,
  transitionProspect,
  updateLeadSource,
  type AdminNotification,
  type ContactChannel,
  type CreateLeadSourceInput,
  type FollowUpDirection,
  type KillSwitchState,
  type LeadSource,
  type LeadSourceKind,
  type LeadType,
  type OptOut,
  type ProspectContactChannel,
  type ProspectCollectRun,
  type ProspectIntentStatus,
  type ProspectLead,
  type ProspectLeadDetail,
  type ProspectReminder,
  type ProspectStats,
  type ProspectStatus,
  type ReminderLevel,
  type ReviewDecision,
  type TransitionStatus,
} from './api'

const KIND_LABEL: Record<LeadSourceKind, string> = {
  self_owned: '自有',
  public_api: '公开 API',
  third_party_scrape: '第三方采集',
}

const LEAD_TYPE_LABEL: Record<LeadType, string> = {
  tenant: '租客',
  landlord: '房东',
}

const STATUS_LABEL: Record<ProspectStatus, string> = {
  new: '待复核',
  reviewed: '已复核',
  contacted: '已联系',
  following_up: '跟进中',
  converted: '已转化',
  published: '已发布',
  cold: '冷淡',
  lost: '流失',
  opt_out: '已拒收',
  rejected: '已拒绝',
  ignored: '已忽略',
}

const INTENT_LABEL: Record<ProspectIntentStatus, string> = {
  seeking: '求租',
  rented: '已租',
  landlord_listing: '房东放盘',
  agent: '中介',
  irrelevant: '无关',
  unknown: '未评分',
}

const PROSPECT_CHANNEL_LABEL: Record<ProspectContactChannel, string> = {
  phone: '电话',
  wechat: '微信',
  in_app: '站内信',
  sms: '短信',
  unknown: '未知',
}

const CHANNEL_LABEL: Record<ContactChannel, string> = {
  in_app: '站内信',
  sms: '短信',
  wechat: '微信',
  phone: '电话',
}

const DIRECTION_LABEL: Record<FollowUpDirection, string> = {
  outbound: '外呼',
  inbound: '回收',
}

const DIRECTION_OPTIONS: FollowUpDirection[] = ['outbound', 'inbound']

const REMINDER_LEVEL_LABEL: Record<ReminderLevel, string> = {
  due_3d: '3 天待跟进',
  overdue_7d: '7 天超期',
}

const INTENT_ORDER: ProspectIntentStatus[] = [
  'seeking',
  'rented',
  'landlord_listing',
  'agent',
  'irrelevant',
  'unknown',
]

function formatRate(rate: number): string {
  return `${Math.round(rate * 1000) / 10}%`
}

interface BarItem {
  key: string
  label: string
  count: number
}

function renderBars(items: BarItem[], total: number) {
  if (items.length === 0) {
    return <p className="muted">暂无数据</p>
  }
  return (
    <div className="bar-list">
      {items.map((item) => {
        const pct = total > 0 ? Math.round((item.count / total) * 100) : 0
        return (
          <div className="bar-row" key={item.key}>
            <span className="bar-label">{item.label}</span>
            <span className="bar-track">
              <span className="bar-fill" style={{ width: `${pct}%` }} />
            </span>
            <span className="bar-value">{item.count}</span>
          </div>
        )
      })}
    </div>
  )
}

const LEAD_TYPES: LeadType[] = ['tenant', 'landlord']
const CONTACT_CHANNELS: ContactChannel[] = ['in_app', 'sms', 'wechat', 'phone']
const PROSPECT_STATUSES: ProspectStatus[] = [
  'new',
  'reviewed',
  'contacted',
  'following_up',
  'converted',
  'published',
  'cold',
  'lost',
  'opt_out',
  'rejected',
  'ignored',
]

// 与后端 CanTransition 对齐：reviewed / contacted 走专用接口，opt_out 走拒收接口。
const TRANSITIONS: Partial<Record<ProspectStatus, TransitionStatus[]>> = {
  new: ['rejected', 'ignored'],
  reviewed: ['rejected', 'ignored'],
  contacted: ['following_up', 'converted', 'published', 'cold', 'lost'],
  following_up: ['converted', 'published', 'cold', 'lost'],
}

function transitionsFor(status: ProspectStatus): TransitionStatus[] {
  return TRANSITIONS[status] ?? []
}

function formatBudget(cents?: number): string {
  if (!cents || cents <= 0) {
    return '—'
  }
  const yuan = Math.round(cents / 100)
  return yuan >= 10000 ? `${(yuan / 10000).toFixed(1)} 万` : `${yuan} 元`
}

interface SourceForm {
  uuid: string
  name: string
  kind: LeadSourceKind
  baseUrl: string
  rateLimitMs: string
  collectIntervalMinutes: string
  reviewNote: string
  robotsOk: boolean
  termsOk: boolean
}

function emptySourceForm(): SourceForm {
  return {
    uuid: '',
    name: '',
    kind: 'self_owned',
    baseUrl: '',
    rateLimitMs: '',
    collectIntervalMinutes: '60',
    reviewNote: '',
    robotsOk: true,
    termsOk: true,
  }
}

export default function Prospecting({ token }: { token: string }) {
  const [kill, setKill] = useState<KillSwitchState | null>(null)
  const [killBusy, setKillBusy] = useState(false)
  const [killError, setKillError] = useState('')

  const [sources, setSources] = useState<LeadSource[]>([])
  const [sourcesLoading, setSourcesLoading] = useState(true)
  const [sourcesError, setSourcesError] = useState('')
  const [sourceBusy, setSourceBusy] = useState(false)
  const [sourceMsg, setSourceMsg] = useState('')
  const [form, setForm] = useState<SourceForm>(emptySourceForm)

  const [collectRuns, setCollectRuns] = useState<ProspectCollectRun[]>([])
  const [collectRunsLoading, setCollectRunsLoading] = useState(true)
  const [collectRunsError, setCollectRunsError] = useState('')

  const [queueType, setQueueType] = useState<LeadType | ''>('')
  const [queue, setQueue] = useState<ProspectLead[]>([])
  const [queueLoading, setQueueLoading] = useState(true)
  const [queueError, setQueueError] = useState('')

  const [leadType, setLeadType] = useState<LeadType | ''>('')
  const [leadStatus, setLeadStatus] = useState<ProspectStatus | ''>('')
  const [leads, setLeads] = useState<ProspectLead[]>([])
  const [leadsLoading, setLeadsLoading] = useState(true)
  const [leadsError, setLeadsError] = useState('')

  const [detail, setDetail] = useState<ProspectLeadDetail | null>(null)
  const [detailLoading, setDetailLoading] = useState(false)
  const [detailError, setDetailError] = useState('')
  const [actionBusy, setActionBusy] = useState(false)
  const [actionMsg, setActionMsg] = useState('')
  const [contactChannel, setContactChannel] = useState<ContactChannel>('in_app')
  const [contactContent, setContactContent] = useState('')
  const [transitionTo, setTransitionTo] = useState<TransitionStatus>('following_up')
  const [optOutReason, setOptOutReason] = useState('')
  const [followUpChannel, setFollowUpChannel] = useState<ContactChannel>('phone')
  const [followUpDirection, setFollowUpDirection] = useState<FollowUpDirection>('outbound')
  const [followUpContent, setFollowUpContent] = useState('')

  const [reminders, setReminders] = useState<ProspectReminder[]>([])
  const [remindersLoading, setRemindersLoading] = useState(true)
  const [remindersError, setRemindersError] = useState('')

  const [notifications, setNotifications] = useState<AdminNotification[]>([])
  const [notificationsUnread, setNotificationsUnread] = useState(0)
  const [notificationsLoading, setNotificationsLoading] = useState(true)
  const [notificationsError, setNotificationsError] = useState('')
  const [notificationsBusy, setNotificationsBusy] = useState(false)

  const [stats, setStats] = useState<ProspectStats | null>(null)
  const [statsLoading, setStatsLoading] = useState(true)
  const [statsError, setStatsError] = useState('')

  const [optOutType, setOptOutType] = useState<LeadType | ''>('')
  const [optOuts, setOptOuts] = useState<OptOut[]>([])
  const [optOutsLoading, setOptOutsLoading] = useState(true)
  const [optOutsError, setOptOutsError] = useState('')

  useEffect(() => {
    let cancelled = false
    fetchKillSwitch(token)
      .then((state) => {
        if (!cancelled) {
          setKill(state)
          setKillError('')
        }
      })
      .catch((err: Error) => {
        if (!cancelled) {
          setKillError(err.message || '停机开关加载失败')
        }
      })
    return () => {
      cancelled = true
    }
  }, [token])

  useEffect(() => {
    let cancelled = false
    setSourcesLoading(true)
    fetchLeadSources(token)
      .then((items) => {
        if (!cancelled) {
          setSources(items)
          setSourcesError('')
        }
      })
      .catch((err: Error) => {
        if (!cancelled) {
          setSourcesError(err.message || '信息源加载失败')
        }
      })
      .finally(() => {
        if (!cancelled) {
          setSourcesLoading(false)
        }
      })
    return () => {
      cancelled = true
    }
  }, [token])

  useEffect(() => {
    let cancelled = false
    setCollectRunsLoading(true)
    fetchCollectRuns(token)
      .then((items) => {
        if (!cancelled) {
          setCollectRuns(items)
          setCollectRunsError('')
        }
      })
      .catch((err: Error) => {
        if (!cancelled) {
          setCollectRunsError(err.message || '采集运行记录加载失败')
        }
      })
      .finally(() => {
        if (!cancelled) {
          setCollectRunsLoading(false)
        }
      })
    return () => {
      cancelled = true
    }
  }, [token])

  useEffect(() => {
    let cancelled = false
    setQueueLoading(true)
    fetchReviewQueue(token, queueType)
      .then((items) => {
        if (!cancelled) {
          setQueue(items)
          setQueueError('')
        }
      })
      .catch((err: Error) => {
        if (!cancelled) {
          setQueueError(err.message || '待复核队列加载失败')
        }
      })
      .finally(() => {
        if (!cancelled) {
          setQueueLoading(false)
        }
      })
    return () => {
      cancelled = true
    }
  }, [token, queueType])

  useEffect(() => {
    let cancelled = false
    setLeadsLoading(true)
    fetchProspects(token, { leadType, status: leadStatus })
      .then((items) => {
        if (!cancelled) {
          setLeads(items)
          setLeadsError('')
        }
      })
      .catch((err: Error) => {
        if (!cancelled) {
          setLeadsError(err.message || '线索列表加载失败')
        }
      })
      .finally(() => {
        if (!cancelled) {
          setLeadsLoading(false)
        }
      })
    return () => {
      cancelled = true
    }
  }, [token, leadType, leadStatus])

  useEffect(() => {
    let cancelled = false
    setOptOutsLoading(true)
    fetchOptOuts(token, optOutType)
      .then((items) => {
        if (!cancelled) {
          setOptOuts(items)
          setOptOutsError('')
        }
      })
      .catch((err: Error) => {
        if (!cancelled) {
          setOptOutsError(err.message || '拒收名单加载失败')
        }
      })
      .finally(() => {
        if (!cancelled) {
          setOptOutsLoading(false)
        }
      })
    return () => {
      cancelled = true
    }
  }, [token, optOutType])

  useEffect(() => {
    let cancelled = false
    setRemindersLoading(true)
    fetchProspectReminders(token)
      .then((items) => {
        if (!cancelled) {
          setReminders(items)
          setRemindersError('')
        }
      })
      .catch((err: Error) => {
        if (!cancelled) {
          setRemindersError(err.message || '跟进提醒加载失败')
        }
      })
      .finally(() => {
        if (!cancelled) {
          setRemindersLoading(false)
        }
      })
    return () => {
      cancelled = true
    }
  }, [token])

  useEffect(() => {
    let cancelled = false
    setNotificationsLoading(true)
    fetchProspectNotifications(token)
      .then((data) => {
        if (!cancelled) {
          setNotifications(data.items)
          setNotificationsUnread(data.unread)
          setNotificationsError('')
        }
      })
      .catch((err: Error) => {
        if (!cancelled) {
          setNotificationsError(err.message || '站内通知加载失败')
        }
      })
      .finally(() => {
        if (!cancelled) {
          setNotificationsLoading(false)
        }
      })
    return () => {
      cancelled = true
    }
  }, [token])

  useEffect(() => {
    let cancelled = false
    setStatsLoading(true)
    fetchProspectStats(token)
      .then((data) => {
        if (!cancelled) {
          setStats(data)
          setStatsError('')
        }
      })
      .catch((err: Error) => {
        if (!cancelled) {
          setStatsError(err.message || '获客统计加载失败')
        }
      })
      .finally(() => {
        if (!cancelled) {
          setStatsLoading(false)
        }
      })
    return () => {
      cancelled = true
    }
  }, [token])

  function refreshSources() {
    return fetchLeadSources(token)
      .then((items) => {
        setSources(items)
        setSourcesError('')
      })
      .catch((err: Error) => setSourcesError(err.message || '信息源加载失败'))
  }

  function refreshCollectRuns() {
    setCollectRunsLoading(true)
    return fetchCollectRuns(token)
      .then((items) => {
        setCollectRuns(items)
        setCollectRunsError('')
      })
      .catch((err: Error) => setCollectRunsError(err.message || '采集运行记录加载失败'))
      .finally(() => setCollectRunsLoading(false))
  }

  function refreshQueue() {
    return fetchReviewQueue(token, queueType)
      .then((items) => {
        setQueue(items)
        setQueueError('')
      })
      .catch((err: Error) => setQueueError(err.message || '待复核队列加载失败'))
  }

  function refreshLeads() {
    return fetchProspects(token, { leadType, status: leadStatus })
      .then((items) => {
        setLeads(items)
        setLeadsError('')
      })
      .catch((err: Error) => setLeadsError(err.message || '线索列表加载失败'))
  }

  function refreshOptOuts() {
    return fetchOptOuts(token, optOutType)
      .then((items) => {
        setOptOuts(items)
        setOptOutsError('')
      })
      .catch((err: Error) => setOptOutsError(err.message || '拒收名单加载失败'))
  }

  function refreshReminders() {
    return fetchProspectReminders(token)
      .then((items) => {
        setReminders(items)
        setRemindersError('')
      })
      .catch((err: Error) => setRemindersError(err.message || '跟进提醒加载失败'))
  }

  function refreshNotifications() {
    return fetchProspectNotifications(token)
      .then((data) => {
        setNotifications(data.items)
        setNotificationsUnread(data.unread)
        setNotificationsError('')
      })
      .catch((err: Error) => setNotificationsError(err.message || '站内通知加载失败'))
  }

  function markNotificationRead(id: number) {
    setNotificationsBusy(true)
    markProspectNotificationRead(token, id)
      .then(() => refreshNotifications())
      .catch((err: Error) => setNotificationsError(err.message || '标记已读失败'))
      .finally(() => setNotificationsBusy(false))
  }

  function refreshStats() {
    return fetchProspectStats(token)
      .then((data) => {
        setStats(data)
        setStatsError('')
      })
      .catch((err: Error) => setStatsError(err.message || '获客统计加载失败'))
  }

  function refreshDetail(uuid: string) {
    return fetchProspectDetail(token, uuid)
      .then((item) => {
        setDetail(item)
        setDetailError('')
        const allowed = transitionsFor(item.status)
        if (allowed.length > 0) {
          setTransitionTo(allowed[0])
        }
      })
      .catch((err: Error) => setDetailError(err.message || '线索详情加载失败'))
  }

  function openDetail(uuid: string) {
    setDetail(null)
    setDetailError('')
    setActionMsg('')
    setContactContent('')
    setFollowUpContent('')
    setOptOutReason('')
    setDetailLoading(true)
    refreshDetail(uuid).finally(() => setDetailLoading(false))
  }

  function toggleKill() {
    if (!kill) {
      return
    }
    setKillBusy(true)
    setKillError('')
    setKillSwitch(token, !kill.halted)
      .then(setKill)
      .catch((err: Error) => setKillError(err.message || '切换停机开关失败'))
      .finally(() => setKillBusy(false))
  }

  function submitSource(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const name = form.name.trim()
    if (!name) {
      setSourceMsg('请填写信息源名称')
      return
    }
    const rate = Number(form.rateLimitMs)
    const base: CreateLeadSourceInput = {
      name,
      kind: form.kind,
      baseUrl: form.baseUrl.trim() || undefined,
      reviewNote: form.reviewNote.trim() || undefined,
    }
    if (Number.isFinite(rate) && rate > 0) {
      base.rateLimitMs = Math.trunc(rate)
    }
    const interval = Number(form.collectIntervalMinutes)
    if (Number.isFinite(interval) && interval > 0) {
      base.collectIntervalMinutes = Math.trunc(interval)
    }
    const editing = Boolean(form.uuid)
    setSourceBusy(true)
    setSourceMsg('')
    const task = form.uuid
      ? updateLeadSource(token, form.uuid, { ...base, robotsOk: form.robotsOk, termsOk: form.termsOk })
      : createLeadSource(token, base).then(() => undefined)
    task
      .then(() => {
        setSourceMsg(editing ? '信息源已更新' : '信息源已新增')
        setForm(emptySourceForm())
        return Promise.all([refreshSources(), refreshCollectRuns()])
      })
      .catch((err: Error) => setSourceMsg(err.message || '保存信息源失败'))
      .finally(() => setSourceBusy(false))
  }

  function editSource(source: LeadSource) {
    setSourceMsg('')
    setForm({
      uuid: source.uuid,
      name: source.name,
      kind: source.kind,
      baseUrl: source.baseUrl ?? '',
      rateLimitMs: source.rateLimitMs > 0 ? String(source.rateLimitMs) : '',
      collectIntervalMinutes: String(source.collectIntervalMinutes || 60),
      reviewNote: source.reviewNote ?? '',
      robotsOk: source.robotsOk,
      termsOk: source.termsOk,
    })
  }

  function removeSource(source: LeadSource) {
    if (!window.confirm(`确认删除信息源「${source.name}」？`)) {
      return
    }
    setSourceBusy(true)
    setSourceMsg('')
    deleteLeadSource(token, source.uuid)
      .then(() => {
        setSourceMsg('信息源已删除')
        if (form.uuid === source.uuid) {
          setForm(emptySourceForm())
        }
        return refreshSources()
      })
      .catch((err: Error) => setSourceMsg(err.message || '删除信息源失败'))
      .finally(() => setSourceBusy(false))
  }

  function toggleSource(source: LeadSource) {
    setSourceBusy(true)
    setSourceMsg('')
    setLeadSourceEnabled(token, source.uuid, !source.enabled)
      .then((enabled) => {
        setSourceMsg(`${source.name} 已${enabled ? '启用' : '停用'}`)
        return refreshSources()
      })
      .catch((err: Error) => setSourceMsg(err.message || '切换信息源失败'))
      .finally(() => setSourceBusy(false))
  }

  function decide(lead: ProspectLead, decision: ReviewDecision) {
    setActionBusy(true)
    setActionMsg('')
    reviewProspect(token, lead.uuid, decision)
      .then((updated) => {
        setActionMsg(`已${decision === 'approve' ? '通过' : '拒绝'}：${STATUS_LABEL[updated.status]}`)
        return Promise.all([refreshQueue(), refreshLeads(), refreshStats()])
      })
      .then(() => {
        if (detail && detail.uuid === lead.uuid) {
          return refreshDetail(lead.uuid)
        }
        return undefined
      })
      .catch((err: Error) => setActionMsg(err.message || '复核失败'))
      .finally(() => setActionBusy(false))
  }

  function sendContact(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!detail) {
      return
    }
    const current = detail
    const content = contactContent.trim()
    if (!content) {
      setActionMsg('请填写首条联系内容')
      return
    }
    setActionBusy(true)
    setActionMsg('')
    contactProspect(token, current.uuid, contactChannel, content)
      .then((followUp) => {
        setActionMsg(`首条联系已记录（${CHANNEL_LABEL[followUp.channel]} · ${followUp.status}）`)
        setContactContent('')
        return Promise.all([
          refreshDetail(current.uuid),
          refreshLeads(),
          refreshQueue(),
          refreshStats(),
        ])
      })
      .catch((err: Error) => setActionMsg(err.message || '发起联系失败'))
      .finally(() => setActionBusy(false))
  }

  function sendFollowUp(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!detail) {
      return
    }
    const current = detail
    const content = followUpContent.trim()
    if (!content) {
      setActionMsg('请填写跟进内容')
      return
    }
    setActionBusy(true)
    setActionMsg('')
    addProspectFollowUp(token, current.uuid, followUpChannel, followUpDirection, content)
      .then((followUp) => {
        setActionMsg(
          `跟进已记录（${CHANNEL_LABEL[followUp.channel]} · ${DIRECTION_LABEL[followUp.direction]}）`,
        )
        setFollowUpContent('')
        return Promise.all([
          refreshDetail(current.uuid),
          refreshLeads(),
          refreshQueue(),
          refreshReminders(),
          refreshStats(),
        ])
      })
      .catch((err: Error) => setActionMsg(err.message || '记录跟进失败'))
      .finally(() => setActionBusy(false))
  }

  function changeStatus() {
    if (!detail) {
      return
    }
    const current = detail
    setActionBusy(true)
    setActionMsg('')
    transitionProspect(token, current.uuid, transitionTo)
      .then((updated) => {
        setActionMsg(`状态已更新为 ${STATUS_LABEL[updated.status]}`)
        return Promise.all([
          refreshDetail(updated.uuid),
          refreshLeads(),
          refreshQueue(),
          refreshReminders(),
          refreshStats(),
        ])
      })
      .catch((err: Error) => setActionMsg(err.message || '流转状态失败'))
      .finally(() => setActionBusy(false))
  }

  function submitOptOut() {
    if (!detail) {
      return
    }
    const current = detail
    setActionBusy(true)
    setActionMsg('')
    optOutProspect(token, current.uuid, optOutReason.trim())
      .then((updated) => {
        setActionMsg('已加入拒收名单')
        setOptOutReason('')
        return Promise.all([
          refreshDetail(updated.uuid),
          refreshLeads(),
          refreshQueue(),
          refreshOptOuts(),
          refreshReminders(),
          refreshStats(),
        ])
      })
      .catch((err: Error) => setActionMsg(err.message || '拒收失败'))
      .finally(() => setActionBusy(false))
  }

  return (
    <section className="panel" id="prospecting">
      <div className="panel-head">
        <h2>自主获客</h2>
        <span className="muted">v1.5 · M10</span>
      </div>

      <div className="kill-row">
        <div>
          <strong>全局停机开关</strong>
          <p className="muted">
            开启后 Worker 停止采集、评分与联系任务。当前：
            {kill ? (kill.halted ? '已停机' : '运行中') : '加载中…'}
          </p>
          {killError && <p className="form-error">{killError}</p>}
        </div>
        <button
          className={kill?.halted ? 'btn primary' : 'btn secondary'}
          onClick={toggleKill}
          disabled={!kill || killBusy}
        >
          {killBusy ? '处理中…' : kill?.halted ? '恢复运行' : '立即停机'}
        </button>
      </div>

      <h3 className="sub-head">获客效果</h3>
      <p className="muted">状态与意向分布，以及联系 / 转化关键指标。</p>
      {statsError && <div className="alert">{statsError}</div>}
      {statsLoading && !stats && <p className="muted">加载中…</p>}
      {stats && (
        <>
          <div className="stats">
            <div className="stat">
              <span className="stat-label">线索总数</span>
              <strong className="stat-value">{stats.totalLeads}</strong>
            </div>
            <div className="stat">
              <span className="stat-label">待复核</span>
              <strong className="stat-value">{stats.reviewPending}</strong>
            </div>
            <div className="stat">
              <span className="stat-label">已联系</span>
              <strong className="stat-value">{stats.contactedLeads}</strong>
            </div>
            <div className="stat">
              <span className="stat-label">已转化</span>
              <strong className="stat-value">{stats.convertedLeads}</strong>
            </div>
            <div className="stat">
              <span className="stat-label">转化率</span>
              <strong className="stat-value">{formatRate(stats.conversionRate)}</strong>
            </div>
            <div className="stat">
              <span className="stat-label">超期线索</span>
              <strong className="stat-value">{stats.staleLeads}</strong>
            </div>
            <div className="stat">
              <span className="stat-label">未处理提醒</span>
              <strong className="stat-value">{stats.openReminders}</strong>
            </div>
          </div>
          <div className="stat-columns">
            <div>
              <h4>状态分布</h4>
              {renderBars(
                PROSPECT_STATUSES.map((status) => ({
                  key: status,
                  label: STATUS_LABEL[status],
                  count: stats.byStatus.find((item) => item.status === status)?.count ?? 0,
                })).filter((item) => item.count > 0),
                stats.totalLeads,
              )}
            </div>
            <div>
              <h4>意向分布</h4>
              {renderBars(
                INTENT_ORDER.map((intent) => ({
                  key: intent,
                  label: INTENT_LABEL[intent],
                  count: stats.byIntent.find((item) => item.intentStatus === intent)?.count ?? 0,
                })).filter((item) => item.count > 0),
                stats.totalLeads,
              )}
            </div>
          </div>
        </>
      )}

      <h3 className="sub-head">信息源</h3>
      <p className="muted">自有 / 公开 API / 第三方采集三类来源，单源可独立启停。</p>
      {sourcesError && <div className="alert">{sourcesError}</div>}
      <div className="table-wrap">
        <table>
          <thead>
            <tr>
              <th>名称</th>
              <th>类型</th>
              <th>状态</th>
              <th>robots</th>
              <th>条款</th>
              <th>限速</th>
              <th>采集间隔</th>
              <th>下次采集</th>
              <th>操作</th>
            </tr>
          </thead>
          <tbody>
            {sources.map((source) => (
              <tr key={source.uuid}>
                <td>{source.name}</td>
                <td>{KIND_LABEL[source.kind]}</td>
                <td>
                  <span className={`status ${source.enabled ? 'status-active' : 'status-offline'}`}>
                    {source.enabled ? '启用' : '停用'}
                  </span>
                </td>
                <td>{source.robotsOk ? '是' : '否'}</td>
                <td>{source.termsOk ? '是' : '否'}</td>
                <td>{source.rateLimitMs > 0 ? `${source.rateLimitMs} ms` : '—'}</td>
                <td>{source.collectIntervalMinutes} 分钟</td>
                <td>
                  {source.nextCollectAt || '待首次采集'}
                  {source.consecutiveFailures > 0 && (
                    <span className="muted">（连续失败 {source.consecutiveFailures} 次）</span>
                  )}
                </td>
                <td>
                  <button
                    className="table-action"
                    onClick={() => toggleSource(source)}
                    disabled={sourceBusy}
                  >
                    {source.enabled ? '停用' : '启用'}
                  </button>
                  <button
                    className="table-action"
                    onClick={() => editSource(source)}
                    disabled={sourceBusy}
                  >
                    编辑
                  </button>
                  <button
                    className="table-action"
                    onClick={() => removeSource(source)}
                    disabled={sourceBusy}
                  >
                    删除
                  </button>
                </td>
              </tr>
            ))}
            {sourcesLoading && (
              <tr>
                <td colSpan={9} className="empty">
                  加载中…
                </td>
              </tr>
            )}
            {!sourcesLoading && sources.length === 0 && (
              <tr>
                <td colSpan={9} className="empty">
                  暂无信息源
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>

      <form className="draft-card prospect-form" onSubmit={submitSource}>
        <strong>{form.uuid ? `编辑信息源 ${form.uuid.slice(0, 8)}` : '新增信息源'}</strong>
        <div className="draft-fields">
          <input
            value={form.name}
            onChange={(e) => setForm({ ...form, name: e.target.value })}
            placeholder="名称"
          />
          <select
            value={form.kind}
            onChange={(e) => setForm({ ...form, kind: e.target.value as LeadSourceKind })}
          >
            {(Object.keys(KIND_LABEL) as LeadSourceKind[]).map((kind) => (
              <option key={kind} value={kind}>
                {KIND_LABEL[kind]}
              </option>
            ))}
          </select>
          <input
            value={form.baseUrl}
            onChange={(e) => setForm({ ...form, baseUrl: e.target.value })}
            placeholder="Base URL（可选）"
          />
          <input
            value={form.rateLimitMs}
            onChange={(e) => setForm({ ...form, rateLimitMs: e.target.value })}
            placeholder="限速毫秒（可选）"
            inputMode="numeric"
          />
          <input
            value={form.collectIntervalMinutes}
            onChange={(e) => setForm({ ...form, collectIntervalMinutes: e.target.value })}
            placeholder="采集间隔分钟（5-1440，默认 60）"
            inputMode="numeric"
          />
          <input
            value={form.reviewNote}
            onChange={(e) => setForm({ ...form, reviewNote: e.target.value })}
            placeholder="合规备注（可选）"
          />
        </div>
        {form.uuid && (
          <div className="action-row">
            <label>
              <input
                type="checkbox"
                checked={form.robotsOk}
                onChange={(e) => setForm({ ...form, robotsOk: e.target.checked })}
              />{' '}
              robots 允许
            </label>
            <label>
              <input
                type="checkbox"
                checked={form.termsOk}
                onChange={(e) => setForm({ ...form, termsOk: e.target.checked })}
              />{' '}
              条款允许
            </label>
          </div>
        )}
        <div className="action-row">
          <button className="btn primary" type="submit" disabled={sourceBusy}>
            {sourceBusy ? '保存中…' : form.uuid ? '保存修改' : '新增信息源'}
          </button>
          {form.uuid && (
            <button
              className="btn secondary"
              type="button"
              onClick={() => setForm(emptySourceForm())}
              disabled={sourceBusy}
            >
              取消编辑
            </button>
          )}
        </div>
        {sourceMsg && <p className="muted">{sourceMsg}</p>}
      </form>

      <h3 className="sub-head">采集运行记录（最近 50 次）</h3>
      <div className="prospect-toolbar">
        <button
          className="btn secondary"
          onClick={() => refreshCollectRuns()}
          disabled={collectRunsLoading}
        >
          刷新
        </button>
        <span className="muted">
          {collectRunsLoading ? '加载中…' : `共 ${collectRuns.length} 条`}
        </span>
      </div>
      {collectRunsError && <div className="alert">{collectRunsError}</div>}
      <div className="table-wrap">
        <table>
          <thead>
            <tr>
              <th>开始时间</th>
              <th>信息源</th>
              <th>结果</th>
              <th>拉取</th>
              <th>入库</th>
              <th>重复</th>
              <th>拒收</th>
              <th>非法</th>
              <th>失败</th>
              <th>错误信息</th>
            </tr>
          </thead>
          <tbody>
            {collectRuns.map((run) => (
              <tr key={run.id}>
                <td>{run.startedAt}</td>
                <td>{run.sourceName}</td>
                <td>
                  <span className={`status ${run.status === 'success' ? 'status-active' : 'status-offline'}`}>
                    {run.status === 'success' ? '成功' : '失败'}
                  </span>
                </td>
                <td>{run.fetched}</td>
                <td>{run.ingested}</td>
                <td>{run.duplicate}</td>
                <td>{run.optedOut}</td>
                <td>{run.invalid}</td>
                <td>{run.failed}</td>
                <td>{run.errorMessage || '—'}</td>
              </tr>
            ))}
            {collectRunsLoading && (
              <tr>
                <td colSpan={10} className="empty">
                  加载中…
                </td>
              </tr>
            )}
            {!collectRunsLoading && collectRuns.length === 0 && (
              <tr>
                <td colSpan={10} className="empty">
                  暂无采集运行记录（worker 每轮调度后写入）
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>

      <h3 className="sub-head">待复核队列（评分 ≥ 70 且意向有效）</h3>
      <div className="prospect-toolbar">
        <select
          value={queueType}
          onChange={(e) => setQueueType(e.target.value as LeadType | '')}
        >
          <option value="">全部类型</option>
          {LEAD_TYPES.map((type) => (
            <option key={type} value={type}>
              {LEAD_TYPE_LABEL[type]}
            </option>
          ))}
        </select>
        <button className="btn secondary" onClick={() => refreshQueue()} disabled={queueLoading}>
          刷新
        </button>
        <span className="muted">{queueLoading ? '加载中…' : `共 ${queue.length} 条`}</span>
      </div>
      {queueError && <div className="alert">{queueError}</div>}
      <div className="table-wrap">
        <table>
          <thead>
            <tr>
              <th>来源</th>
              <th>类型</th>
              <th>意向</th>
              <th>分数</th>
              <th>预算</th>
              <th>渠道</th>
              <th>时间</th>
              <th>操作</th>
            </tr>
          </thead>
          <tbody>
            {queue.map((lead) => (
              <tr key={lead.uuid}>
                <td>{lead.sourceName}</td>
                <td>{LEAD_TYPE_LABEL[lead.leadType]}</td>
                <td>{INTENT_LABEL[lead.intentStatus]}</td>
                <td>{lead.intentScore ?? '—'}</td>
                <td>{formatBudget(lead.budgetCents)}</td>
                <td>{PROSPECT_CHANNEL_LABEL[lead.contactChannel]}</td>
                <td>{lead.createdAt}</td>
                <td>
                  <button
                    className="table-action"
                    onClick={() => openDetail(lead.uuid)}
                    disabled={actionBusy}
                  >
                    详情
                  </button>
                  <button
                    className="table-action"
                    onClick={() => decide(lead, 'approve')}
                    disabled={actionBusy}
                  >
                    通过
                  </button>
                  <button
                    className="table-action"
                    onClick={() => decide(lead, 'reject')}
                    disabled={actionBusy}
                  >
                    拒绝
                  </button>
                </td>
              </tr>
            ))}
            {queueLoading && (
              <tr>
                <td colSpan={8} className="empty">
                  加载中…
                </td>
              </tr>
            )}
            {!queueLoading && queue.length === 0 && (
              <tr>
                <td colSpan={8} className="empty">
                  暂无待复核线索
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>

      <h3 className="sub-head">跟进提醒（3 天待跟进 / 7 天超期）</h3>
      <p className="muted">由 Worker 定时扫描生成；记录新的跟进后自动解除。</p>
      <div className="prospect-toolbar">
        <button
          className="btn secondary"
          onClick={() => refreshReminders()}
          disabled={remindersLoading}
        >
          刷新
        </button>
        <span className="muted">{remindersLoading ? '加载中…' : `共 ${reminders.length} 条`}</span>
      </div>
      {remindersError && <div className="alert">{remindersError}</div>}
      <div className="table-wrap">
        <table>
          <thead>
            <tr>
              <th>线索</th>
              <th>类型</th>
              <th>当前状态</th>
              <th>提醒</th>
              <th>生成时间</th>
              <th>操作</th>
            </tr>
          </thead>
          <tbody>
            {reminders.map((item) => (
              <tr key={item.id}>
                <td>{item.leadUuid.slice(0, 8)}</td>
                <td>{LEAD_TYPE_LABEL[item.leadType]}</td>
                <td>
                  <span className={`status status-${item.leadStatus}`}>
                    {STATUS_LABEL[item.leadStatus]}
                  </span>
                </td>
                <td>
                  <span
                    className={`status ${
                      item.level === 'overdue_7d' ? 'status-stale' : 'status-pending'
                    }`}
                  >
                    {REMINDER_LEVEL_LABEL[item.level]}
                  </span>
                </td>
                <td>{item.createdAt}</td>
                <td>
                  <button
                    className="table-action"
                    onClick={() => openDetail(item.leadUuid)}
                    disabled={actionBusy}
                  >
                    详情
                  </button>
                </td>
              </tr>
            ))}
            {remindersLoading && (
              <tr>
                <td colSpan={6} className="empty">
                  加载中…
                </td>
              </tr>
            )}
            {!remindersLoading && reminders.length === 0 && (
              <tr>
                <td colSpan={6} className="empty">
                  暂无跟进提醒
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>

      <h3 className="sub-head">站内通知</h3>
      <p className="muted">
        跟进提醒会按管理员分发站内通知，同一提醒不重复通知；未读 {notificationsUnread} 条。
      </p>
      <div className="prospect-toolbar">
        <button
          className="btn secondary"
          onClick={() => refreshNotifications()}
          disabled={notificationsLoading}
        >
          刷新
        </button>
        <span className="muted">
          {notificationsLoading ? '加载中…' : `共 ${notifications.length} 条`}
        </span>
      </div>
      {notificationsError && <div className="alert">{notificationsError}</div>}
      <div className="table-wrap">
        <table>
          <thead>
            <tr>
              <th>标题</th>
              <th>内容</th>
              <th>线索</th>
              <th>级别</th>
              <th>状态</th>
              <th>时间</th>
              <th>操作</th>
            </tr>
          </thead>
          <tbody>
            {notifications.map((item) => (
              <tr key={item.id}>
                <td>{item.title}</td>
                <td>{item.body}</td>
                <td>{item.leadUuid ? item.leadUuid.slice(0, 8) : '—'}</td>
                <td>
                  <span
                    className={`status ${
                      item.level === 'overdue_7d' ? 'status-stale' : 'status-pending'
                    }`}
                  >
                    {REMINDER_LEVEL_LABEL[item.level]}
                  </span>
                </td>
                <td>
                  <span className={`status ${item.status === 'unread' ? 'status-pending' : ''}`}>
                    {item.status === 'unread' ? '未读' : '已读'}
                  </span>
                </td>
                <td>{item.createdAt}</td>
                <td>
                  {item.status === 'unread' ? (
                    <button
                      className="table-action"
                      onClick={() => markNotificationRead(item.id)}
                      disabled={notificationsBusy}
                    >
                      标记已读
                    </button>
                  ) : (
                    <span className="muted">—</span>
                  )}
                </td>
              </tr>
            ))}
            {notificationsLoading && (
              <tr>
                <td colSpan={7} className="empty">
                  加载中…
                </td>
              </tr>
            )}
            {!notificationsLoading && notifications.length === 0 && (
              <tr>
                <td colSpan={7} className="empty">
                  暂无站内通知
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>

      <h3 className="sub-head">线索列表</h3>
      <div className="prospect-toolbar">
        <select value={leadType} onChange={(e) => setLeadType(e.target.value as LeadType | '')}>
          <option value="">全部类型</option>
          {LEAD_TYPES.map((type) => (
            <option key={type} value={type}>
              {LEAD_TYPE_LABEL[type]}
            </option>
          ))}
        </select>
        <select
          value={leadStatus}
          onChange={(e) => setLeadStatus(e.target.value as ProspectStatus | '')}
        >
          <option value="">全部状态</option>
          {PROSPECT_STATUSES.map((status) => (
            <option key={status} value={status}>
              {STATUS_LABEL[status]}
            </option>
          ))}
        </select>
        <button className="btn secondary" onClick={() => refreshLeads()} disabled={leadsLoading}>
          刷新
        </button>
        <span className="muted">{leadsLoading ? '加载中…' : `共 ${leads.length} 条`}</span>
      </div>
      {leadsError && <div className="alert">{leadsError}</div>}
      <div className="table-wrap">
        <table>
          <thead>
            <tr>
              <th>来源</th>
              <th>类型</th>
              <th>意向</th>
              <th>分数</th>
              <th>状态</th>
              <th>预算</th>
              <th>时间</th>
              <th>操作</th>
            </tr>
          </thead>
          <tbody>
            {leads.map((lead) => (
              <tr key={lead.uuid}>
                <td>{lead.sourceName}</td>
                <td>{LEAD_TYPE_LABEL[lead.leadType]}</td>
                <td>{INTENT_LABEL[lead.intentStatus]}</td>
                <td>{lead.intentScore ?? '—'}</td>
                <td>
                  <span className={`status status-${lead.status}`}>{STATUS_LABEL[lead.status]}</span>
                  {lead.stale && <span className="status status-stale stale-tag">超期</span>}
                </td>
                <td>{formatBudget(lead.budgetCents)}</td>
                <td>{lead.createdAt}</td>
                <td>
                  <button
                    className="table-action"
                    onClick={() => openDetail(lead.uuid)}
                    disabled={actionBusy}
                  >
                    详情
                  </button>
                </td>
              </tr>
            ))}
            {leadsLoading && (
              <tr>
                <td colSpan={8} className="empty">
                  加载中…
                </td>
              </tr>
            )}
            {!leadsLoading && leads.length === 0 && (
              <tr>
                <td colSpan={8} className="empty">
                  暂无线索
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>

      {detailLoading && <p className="muted">线索详情加载中…</p>}
      {detailError && <div className="alert">{detailError}</div>}
      {detail && (
        <div className="draft-card prospect-detail">
          <div className="panel-head">
            <strong>
              线索 {detail.uuid.slice(0, 8)} · {detail.sourceName}
            </strong>
            <button className="table-action" onClick={() => setDetail(null)}>
              收起
            </button>
          </div>
          <p className="muted">
            {LEAD_TYPE_LABEL[detail.leadType]} · {STATUS_LABEL[detail.status]} · 意向{' '}
            {INTENT_LABEL[detail.intentStatus]}
            {typeof detail.intentScore === 'number' ? ` · ${detail.intentScore} 分` : ''}
            {detail.platform ? ` · ${detail.platform}` : ''}
            {detail.stale && <span className="status status-stale stale-tag">超期</span>}
          </p>
          {actionMsg && <p className="muted">{actionMsg}</p>}
          <div>
            <span className="muted">原文（每次读取均写入审计日志）</span>
            <pre className="raw-text">{detail.rawText || '（无原文）'}</pre>
          </div>
          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>渠道</th>
                  <th>方向</th>
                  <th>状态</th>
                  <th>内容</th>
                  <th>时间</th>
                </tr>
              </thead>
              <tbody>
                {detail.followUps.map((item) => (
                  <tr key={item.id}>
                    <td>{CHANNEL_LABEL[item.channel]}</td>
                    <td>{item.direction === 'outbound' ? '外呼' : '回收'}</td>
                    <td>{item.status}</td>
                    <td>{item.content || '—'}</td>
                    <td>{item.sentAt || item.createdAt}</td>
                  </tr>
                ))}
                {detail.followUps.length === 0 && (
                  <tr>
                    <td colSpan={5} className="empty">
                      暂无跟进记录
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>

          <div className="detail-actions">
            {detail.status === 'new' && (
              <div className="action-box">
                <h4>人工复核</h4>
                <p className="muted">通过后进入已复核，可发起首条联系。</p>
                <div className="action-row">
                  <button
                    className="btn primary"
                    onClick={() => decide(detail, 'approve')}
                    disabled={actionBusy}
                  >
                    通过
                  </button>
                  <button
                    className="btn secondary"
                    onClick={() => decide(detail, 'reject')}
                    disabled={actionBusy}
                  >
                    拒绝
                  </button>
                </div>
              </div>
            )}

            {detail.status === 'reviewed' && (
              <form className="action-box" onSubmit={sendContact}>
                <h4>首条联系（人工填写）</h4>
                <select
                  value={contactChannel}
                  onChange={(e) => setContactChannel(e.target.value as ContactChannel)}
                >
                  {CONTACT_CHANNELS.map((channel) => (
                    <option key={channel} value={channel}>
                      {CHANNEL_LABEL[channel]}
                    </option>
                  ))}
                </select>
                <textarea
                  value={contactContent}
                  onChange={(e) => setContactContent(e.target.value)}
                  placeholder="填写首条联系内容，服务端不会自动生成"
                />
                <div className="action-row">
                  <button className="btn primary" type="submit" disabled={actionBusy}>
                    发送首条联系
                  </button>
                </div>
                <p className="muted">仅已复核状态可发起；停机开关开启或命中拒收名单会被拦截。</p>
              </form>
            )}

            {(detail.status === 'contacted' || detail.status === 'following_up') && (
              <form className="action-box" onSubmit={sendFollowUp}>
                <h4>记录后续跟进</h4>
                <select
                  value={followUpChannel}
                  onChange={(e) => setFollowUpChannel(e.target.value as ContactChannel)}
                >
                  {CONTACT_CHANNELS.map((channel) => (
                    <option key={channel} value={channel}>
                      {CHANNEL_LABEL[channel]}
                    </option>
                  ))}
                </select>
                <select
                  value={followUpDirection}
                  onChange={(e) => setFollowUpDirection(e.target.value as FollowUpDirection)}
                >
                  {DIRECTION_OPTIONS.map((direction) => (
                    <option key={direction} value={direction}>
                      {DIRECTION_LABEL[direction]}
                    </option>
                  ))}
                </select>
                <textarea
                  value={followUpContent}
                  onChange={(e) => setFollowUpContent(e.target.value)}
                  placeholder="填写本次跟进内容，服务端加密存储"
                />
                <div className="action-row">
                  <button className="btn primary" type="submit" disabled={actionBusy}>
                    记录跟进
                  </button>
                </div>
                <p className="muted">记录后线索进入跟进中，并解除超期标记与未处理提醒。</p>
              </form>
            )}

            {transitionsFor(detail.status).length > 0 && (
              <div className="action-box">
                <h4>状态流转</h4>
                <select
                  value={transitionTo}
                  onChange={(e) => setTransitionTo(e.target.value as TransitionStatus)}
                >
                  {transitionsFor(detail.status).map((status) => (
                    <option key={status} value={status}>
                      {STATUS_LABEL[status]}
                    </option>
                  ))}
                </select>
                <div className="action-row">
                  <button className="btn primary" onClick={changeStatus} disabled={actionBusy}>
                    流转状态
                  </button>
                </div>
              </div>
            )}

            {transitionsFor(detail.status).includes('rejected') && (
              <div className="action-box">
                <h4>人工拒收</h4>
                <input
                  value={optOutReason}
                  onChange={(e) => setOptOutReason(e.target.value)}
                  placeholder="拒收原因（可选）"
                />
                <div className="action-row">
                  <button className="btn primary danger" onClick={submitOptOut} disabled={actionBusy}>
                    加入拒收名单
                  </button>
                </div>
              </div>
            )}
          </div>
        </div>
      )}

      <h3 className="sub-head">拒收名单</h3>
      <div className="prospect-toolbar">
        <select
          value={optOutType}
          onChange={(e) => setOptOutType(e.target.value as LeadType | '')}
        >
          <option value="">全部类型</option>
          {LEAD_TYPES.map((type) => (
            <option key={type} value={type}>
              {LEAD_TYPE_LABEL[type]}
            </option>
          ))}
        </select>
        <button className="btn secondary" onClick={() => refreshOptOuts()} disabled={optOutsLoading}>
          刷新
        </button>
        <span className="muted">{optOutsLoading ? '加载中…' : `共 ${optOuts.length} 条`}</span>
      </div>
      {optOutsError && <div className="alert">{optOutsError}</div>}
      <div className="table-wrap">
        <table>
          <thead>
            <tr>
              <th>类型</th>
              <th>原因</th>
              <th>时间</th>
            </tr>
          </thead>
          <tbody>
            {optOuts.map((item) => (
              <tr key={item.id}>
                <td>{LEAD_TYPE_LABEL[item.leadType]}</td>
                <td>{item.reason || '—'}</td>
                <td>{item.optedOutAt}</td>
              </tr>
            ))}
            {optOutsLoading && (
              <tr>
                <td colSpan={3} className="empty">
                  加载中…
                </td>
              </tr>
            )}
            {!optOutsLoading && optOuts.length === 0 && (
              <tr>
                <td colSpan={3} className="empty">
                  暂无拒收记录
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>
    </section>
  )
}
