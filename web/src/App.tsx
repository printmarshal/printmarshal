import { useCallback, useEffect, useState } from 'react'
import { Navigate, NavLink, Route, Routes, useLocation, useNavigate } from 'react-router-dom'
import { api } from './api'
import type { Alert, Dashboard as DashboardData, User } from './types'
import { disconnectEvents, useEvents } from './ws'
import { Ic, Mark, Wordmark } from './components/icons'
import { Av, Btn } from './components/ui'
import Login from './pages/Login'
import Dashboard from './pages/Dashboard'
import Devices from './pages/Devices'
import DeviceDetail from './pages/DeviceDetail'
import Consumables from './pages/Consumables'
import ConsumableDetail from './pages/ConsumableDetail'
import Discovery from './pages/Discovery'
import Jobs from './pages/Jobs'
import JobDetail from './pages/JobDetail'
import Alerts from './pages/Alerts'
import Groups from './pages/Groups'
import Catalog from './pages/Catalog'
import Credentials from './pages/Credentials'
import Settings from './pages/Settings'
import './theme'

export default function App() {
  const [user, setUser] = useState<User | null | undefined>(undefined)
  const navigate = useNavigate()
  const refresh = useCallback(() => { api.me().then(setUser).catch(() => setUser(null)) }, [])
  useEffect(() => { refresh() }, [refresh])
  useEffect(() => {
    const onUnauthorized = () => { setUser(null); disconnectEvents() }
    window.addEventListener('pm:unauthorized', onUnauthorized)
    return () => window.removeEventListener('pm:unauthorized', onUnauthorized)
  }, [])

  if (user === undefined) return <div className="center muted">Loading…</div>
  if (!user) return <Login onLogin={(u) => { setUser(u); navigate('/') }} />

  return (
    <Shell user={user} onLogout={async () => { await api.logout(); disconnectEvents(); setUser(null) }}>
      <Routes>
        <Route path="/" element={<Dashboard user={user} />} />
        <Route path="/devices" element={<Devices user={user} />} />
        <Route path="/devices/:id" element={<DeviceDetail user={user} />} />
        <Route path="/consumables" element={<Consumables user={user} />} />
        <Route path="/consumables/:id" element={<ConsumableDetail user={user} />} />
        <Route path="/discovery" element={<Discovery user={user} />} />
        <Route path="/jobs" element={<Jobs />} />
        <Route path="/jobs/:id" element={<JobDetail />} />
        <Route path="/alerts" element={<Alerts user={user} />} />
        <Route path="/groups" element={<Groups user={user} />} />
        <Route path="/catalog" element={<Catalog user={user} />} />
        <Route path="/credentials" element={<Credentials user={user} />} />
        <Route path="/settings/*" element={<Settings user={user} />} />
        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    </Shell>
  )
}

interface Counts { devices: number; critical: number; warning: number; lowStock: number; jobs: number }

function Shell({ user, onLogout, children }: { user: User; onLogout: () => void; children: React.ReactNode }) {
  const [live, setLive] = useState(false)
  const [toast, setToast] = useState<Alert | null>(null)
  const [counts, setCounts] = useState<Counts>({ devices: 0, critical: 0, warning: 0, lowStock: 0, jobs: 0 })
  const [collapsed, setCollapsed] = useState(() => { try { return localStorage.getItem('pm.sidebar') === 'collapsed' } catch { return false } })
  const [jump, setJump] = useState('')
  const navigate = useNavigate()
  const location = useLocation()

  const loadCounts = useCallback(() => {
    api.get<DashboardData>('/api/dashboard').then((d) => setCounts({
      devices: d.summary.devices, critical: d.summary.criticalAlerts, warning: d.summary.activeAlerts - d.summary.criticalAlerts,
      lowStock: d.summary.lowStock || 0, jobs: d.summary.runningJobs,
    })).catch(() => {})
  }, [])
  useEffect(() => { loadCounts() }, [loadCounts, location.pathname])
  useEvents((e) => {
    if (e.type === 'stream.connected') setLive(true)
    else if (e.type === 'stream.disconnected') setLive(false)
    else setLive(true)
    if (e.type === 'alert.raised') { setToast(e.data as Alert); setTimeout(() => setToast(null), 7000) }
    if (e.type.startsWith('alert.') || e.type === 'job.updated' || e.type === 'device.discovered' || e.type.startsWith('consumable.')) loadCounts()
  }, [loadCounts])

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === '/' && !(e.target instanceof HTMLInputElement || e.target instanceof HTMLTextAreaElement || e.target instanceof HTMLSelectElement)) {
        e.preventDefault(); document.getElementById('jump')?.focus()
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [])

  const toggleSidebar = () => { const v = !collapsed; setCollapsed(v); try { localStorage.setItem('pm.sidebar', v ? 'collapsed' : 'open') } catch { /* ignore */ } }
  const canOperate = user.role !== 'viewer'
  const roleLabel = user.role.charAt(0).toUpperCase() + user.role.slice(1)

  return (
    <div className="shell">
      <nav className={`side${collapsed ? ' collapsed' : ''}`} aria-label="Primary">
        <div className="brand"><Mark size={26} /><Wordmark size={17} /></div>
        <NavLink className="nav" to="/" end title="Dashboard"><Ic.dashboard /><span>Dashboard</span></NavLink>
        <NavLink className="nav" to="/devices" title="Devices"><Ic.printer /><span>Devices</span><span className="ct num">{counts.devices.toLocaleString()}</span></NavLink>
        <NavLink className="nav" to="/consumables" title="Consumables"><Ic.consumables /><span>Consumables</span>{counts.lowStock > 0 && <span className="ct w">{counts.lowStock} low</span>}</NavLink>
        <NavLink className="nav" to="/alerts" title="Alerts"><Ic.bell /><span>Alerts</span>{counts.critical > 0 ? <span className="ct c">{counts.critical}</span> : counts.warning > 0 ? <span className="ct w">{counts.warning}</span> : null}</NavLink>
        <NavLink className="nav" to="/groups" title="Groups"><Ic.layers /><span>Groups</span></NavLink>
        <NavLink className="nav" to="/catalog" title="Catalogue"><Ic.chart /><span>Catalogue</span></NavLink>
        <div className="navsep">Manage</div>
        <a className="nav soon" aria-disabled="true" title="Configuration — phase 3, in progress"><Ic.sliders /><span>Configuration</span><span className="ct">soon</span></a>
        <a className="nav soon" aria-disabled="true" title="Security — phase 5, in progress"><Ic.shield /><span>Security</span><span className="ct">soon</span></a>
        <a className="nav soon" aria-disabled="true" title="Firmware — phase 6, in progress"><Ic.chip /><span>Firmware</span><span className="ct">soon</span></a>
        <NavLink className="nav" to="/jobs" title="Jobs"><Ic.activity /><span>Jobs</span>{counts.jobs > 0 && <span className="ct num">{counts.jobs}</span>}</NavLink>
        <NavLink className="nav" to="/discovery" title="Discovery"><Ic.radar /><span>Discovery</span></NavLink>
        {canOperate && <NavLink className="nav" to="/credentials" title="Credentials"><Ic.lock /><span>Credentials</span></NavLink>}
        <div style={{ flex: '1 1 24px' }} />
        <NavLink className="nav" to="/settings" title="Settings"><Ic.settings /><span>Settings</span></NavLink>
        <button className="nav" onClick={toggleSidebar} aria-label={collapsed ? 'Expand sidebar' : 'Collapse sidebar'} style={{ color: 'var(--text-3)', fontSize: 12 }}>
          {collapsed ? <Ic.chevronRight /> : <Ic.chevronsLeft />}<span>Collapse</span>
        </button>
      </nav>
      <div className="main">
        <header className="top">
          <form className="search" onSubmit={(e) => { e.preventDefault(); navigate(`/devices?q=${encodeURIComponent(jump)}`); setJump('') }}>
            <Ic.search />
            <input id="jump" type="search" aria-label="Jump to device" placeholder="Jump to device — name, IP, serial, model" value={jump} onChange={(e) => setJump(e.target.value)} />
            <span className="kbd">/</span>
          </form>
          <div style={{ marginLeft: 'auto', display: 'flex', alignItems: 'center', gap: 12 }}>
            <span style={{ display: 'inline-flex', alignItems: 'center', gap: 6, fontSize: 12, color: 'var(--text-2)' }}><span className="dot" style={{ background: live ? 'var(--ok)' : 'var(--off)' }} />{live ? 'Live' : 'Reconnecting'}</span>
            <NavLink to="/alerts" className="btn ic ghost" aria-label={`Alerts, ${counts.critical + counts.warning} active`} style={{ position: 'relative' }}>
              <Ic.bell />{counts.critical > 0 && <span className="dot" style={{ position: 'absolute', top: 6, right: 6, background: 'var(--crit)' }} />}
            </NavLink>
            <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
              <Av name={user.username} />
              <div style={{ lineHeight: 1.2 }}><div style={{ fontWeight: 500 }}>{user.username}</div><div className="cap">{roleLabel}</div></div>
              <Btn sm variant="ghost" onClick={onLogout} title="Sign out">Sign out</Btn>
            </div>
          </div>
        </header>
        <main className="content">{children}</main>
      </div>
      {toast && (
        <div className="toast" role="status">
          <Ic.crit style={{ color: toast.severity === 'critical' ? 'var(--crit)' : 'var(--warn)' }} />
          <div><b style={{ color: toast.severity === 'critical' ? 'var(--crit)' : 'var(--warn)' }}>{toast.severity === 'critical' ? 'Critical alert' : 'Warning'}</b><div className="cap">{toast.description}</div></div>
          <Btn sm variant="ghost" onClick={() => { setToast(null); navigate('/alerts') }}>View</Btn>
        </div>
      )}
    </div>
  )
}
