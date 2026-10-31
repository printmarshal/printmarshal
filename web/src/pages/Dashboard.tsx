import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, ago } from '../api'
import type { AuditEntry, Dashboard as DashboardData, InstalledSupply, LayoutEntry, User } from '../types'
import { useEvents } from '../ws'
import { Ic } from '../components/icons'
import { Banner, Btn, fmtInt, remaining } from '../components/ui'
import { CATALOGUE, roleAllows, widgetById } from '../dashboard/catalogue'
import { effectiveEntries, move, sameLayout, setFlag, toLayout } from '../dashboard/layout'
import { WIDGETS } from '../dashboard/widgets'
import { WidgetFrame } from '../dashboard/WidgetFrame'
import { EditBar } from '../dashboard/EditBar'

export default function Dashboard({ user }: { user: User }) {
  const [data, setData] = useState<DashboardData | null>(null)
  const [low, setLow] = useState<InstalledSupply[]>([])
  const [audit, setAudit] = useState<AuditEntry[]>([])
  const [error, setError] = useState('')
  const [errorDismissed, setErrorDismissed] = useState(false)
  const [retrying, setRetrying] = useState(false)
  const [dismissed, setDismissed] = useState(false)
  const [tick, setTick] = useState(0)
  const [editing, setEditing] = useState(false)
  const [draft, setDraft] = useState<LayoutEntry[] | null>(null)
  const [busy, setBusy] = useState(false)
  const [notice, setNotice] = useState('')
  const latest = useRef(0) // the newest dashboard request sent
  const applied = useRef(0) // the newest dashboard response shown
  const load = useCallback(() => {
    // Live events can overlap requests. Only a newer response replaces the data, and only the
    // newest request can report a failure, so one that a later success has overtaken stays silent.
    // The error stays set while a request is in flight, so a refresh never flashes it off and on.
    const id = ++latest.current
    const done = api.get<DashboardData>('/api/dashboard').then((d) => {
      if (id > applied.current) { applied.current = id; setData(d); setError(''); setErrorDismissed(false) }
    }).catch((e) => { if (id === latest.current) setError(e.message) })
    api.get<InstalledSupply[]>('/api/consumables/installed?forecast=true').then((r) => setLow(r.filter((s) => remaining(s.supply) !== null && (remaining(s.supply) as number) <= 20).sort((a, b) => (remaining(a.supply) ?? 0) - (remaining(b.supply) ?? 0)).slice(0, 6))).catch(() => {})
    if (user.role !== 'viewer') api.get<AuditEntry[]>('/api/audit?limit=6').then(setAudit).catch(() => {})
    return done
  }, [user.role])
  // Asked for by hand: a failure shows again even if the last warning was dismissed.
  const refresh = () => { setRetrying(true); setErrorDismissed(false); load().finally(() => setRetrying(false)) }
  useEffect(() => { load() }, [load])
  useEffect(() => { const t = setInterval(() => setTick((n) => n + 1), 30000); return () => clearInterval(t) }, [])
  useEffect(() => { if (tick) load() }, [tick, load])
  // A dropped stream says nothing new about the data, and fetching then would fail during a server restart.
  useEvents((e) => { if (e.type !== 'stream.disconnected') load() }, [load])

  const saved = useMemo(() => effectiveEntries(data?.layout.effective), [data])
  const entries = editing && draft ? draft : saved
  const dirty = !!draft && !sameLayout(draft, saved)

  if (!data) {
    if (!error) return <DashboardSkeleton />
    return <Banner tone="crit" role="alert" right={<Btn sm disabled={retrying} onClick={refresh}><Ic.refresh />Retry</Btn>}>Could not load the dashboard: {error}</Banner>
  }
  const s = data.summary
  const newCrit = data.alerts.filter((a) => a.severity === 'critical' && !a.ackedAt)
  const lastPoll = data.jobs.find((j) => j.type === 'poll' && j.finishedAt)
  const visible = entries.filter((e) => { const w = widgetById.get(e.id); return !!w && roleAllows(user.role, w) })
  const hidden = visible.filter((e) => e.hidden)
  const shown = visible.filter((e) => !e.hidden)

  const run = async (label: string, fn: () => Promise<unknown>) => {
    setBusy(true)
    try { await fn(); setNotice(label); setEditing(false); setDraft(null); load() } catch (e) { setNotice((e as Error).message) } finally { setBusy(false) }
  }
  const startEdit = () => { setDraft(saved); setEditing(true); setNotice('') }
  // Functional updates: successive edits compose even before React re-renders.
  const update = (fn: (cur: LayoutEntry[]) => LayoutEntry[]) => setDraft((cur) => fn(cur ?? saved))

  return (
    <>
      {newCrit.length > 0 && !dismissed && (
        <Banner tone="crit" role="alert" right={<><Link className="btn sm" to="/alerts">View alerts</Link><Btn sm variant="ghost" ic aria-label="Dismiss banner" onClick={() => setDismissed(true)}><Ic.x /></Btn></>}>
          <b style={{ color: 'var(--crit)' }}>{newCrit.length} unacknowledged critical alert{newCrit.length === 1 ? '' : 's'}</b> — {newCrit.slice(0, 2).map((a) => a.description).join(' · ')}{newCrit.length > 2 ? ' · …' : ''}
        </Banner>
      )}
      <div className="ph">
        <div><h1 className="h1">Fleet overview</h1><div className="cap num">{fmtInt(s.devices)} devices{lastPoll ? ` · last full poll ${ago(lastPoll.finishedAt)}` : ''}</div></div>
        <div className="row">
          <span className="cap num" style={{ display: 'inline-flex', alignItems: 'center', gap: 6 }}><Ic.clock />Auto-refresh every 30 s</span>
          <Btn sm onClick={refresh}><Ic.refresh />Refresh</Btn>
          {!editing && <Btn sm onClick={startEdit}><Ic.sliders />Customise</Btn>}
        </div>
      </div>
      {error && !errorDismissed && (
        <Banner tone="warn" role="alert" right={<><Btn sm disabled={retrying} onClick={refresh}><Ic.refresh />Retry</Btn><Btn sm variant="ghost" ic aria-label="Dismiss warning" onClick={() => setErrorDismissed(true)}><Ic.x /></Btn></>}>
          Could not refresh the dashboard: {error}. Showing the last loaded data.
        </Banner>
      )}
      {notice && <Banner tone="info" right={<Btn sm variant="ghost" ic aria-label="Dismiss" onClick={() => setNotice('')}><Ic.x /></Btn>}>{notice}</Banner>}
      {editing && draft && (
        <EditBar
          dirty={dirty} busy={busy} isAdmin={user.role === 'admin'} hasPersonal={!!data.layout.personal} hasDefault={!!data.layout.default}
          onDone={() => run('Dashboard saved for your sign-in.', () => api.put('/api/dashboard/layout', toLayout(draft)))}
          onCancel={() => { setEditing(false); setDraft(null) }}
          onReset={() => run('Your dashboard now follows the default.', () => api.del('/api/dashboard/layout'))}
          onSaveDefault={() => run('Saved as the default dashboard for everyone.', () => api.put('/api/dashboard/layout/default', toLayout(draft)))}
          onClearDefault={() => run('The default dashboard is back to the built-in order.', () => api.del('/api/dashboard/layout/default'))}
        />
      )}
      <div className={`dash-grid${editing ? ' editing' : ''}`}>
        {shown.map((e, i) => {
          const def = widgetById.get(e.id)!
          const render = WIDGETS[e.id]
          if (!render) return null
          return (
            <WidgetFrame key={e.id} def={def} editing={editing} collapsed={!!e.collapsed} first={i === 0} last={i === shown.length - 1}
              onMove={(dir) => update((cur) => move(cur, e.id, dir))}
              onHide={() => update((cur) => setFlag(cur, e.id, 'hidden', true))}
              onCollapse={(v) => update((cur) => setFlag(cur, e.id, 'collapsed', v))}>
              {render({ data, low, audit, user })}
            </WidgetFrame>
          )
        })}
      </div>
      {editing && (
        <div className="hidden-tray" role="region" aria-label="Hidden widgets">
          <span className="cap">{hidden.length === 0 ? 'No hidden widgets. Use Hide on a widget to put it here.' : 'Hidden:'}</span>
          {hidden.map((e) => <Btn sm key={e.id} onClick={() => update((cur) => setFlag(cur, e.id, 'hidden', false))}><Ic.plus />{widgetById.get(e.id)?.title}</Btn>)}
        </div>
      )}
    </>
  )
}

export { CATALOGUE }

function DashboardSkeleton() {
  return (
    <div className="grid-auto" aria-busy="true">
      {Array.from({ length: 6 }).map((_, i) => <div className="kpi skel" key={i} style={{ animation: 'none' }}><span className="skel" style={{ width: '60%', height: 10 }} /><span className="skel" style={{ width: '40%', height: 22 }} /><span className="skel" style={{ width: '70%', height: 10 }} /></div>)}
    </div>
  )
}
