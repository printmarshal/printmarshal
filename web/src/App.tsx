import { useCallback, useEffect, useState } from 'react'
import { Navigate, NavLink, Route, Routes, useNavigate } from 'react-router-dom'
import { api } from './api'
import type { User } from './types'
import { disconnectEvents, useEvents } from './ws'
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
import Credentials from './pages/Credentials'
import Settings from './pages/Settings'

export default function App() {
  const [user, setUser] = useState<User | null | undefined>(undefined)
  const navigate = useNavigate()

  const refresh = useCallback(() => {
    api.me().then(setUser).catch(() => setUser(null))
  }, [])

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
        <Route path="/" element={<Dashboard />} />
        <Route path="/devices" element={<Devices />} />
        <Route path="/devices/:id" element={<DeviceDetail user={user} />} />
        <Route path="/consumables" element={<Consumables user={user} />} />
        <Route path="/consumables/:id" element={<ConsumableDetail user={user} />} />
        <Route path="/discovery" element={<Discovery user={user} />} />
        <Route path="/jobs" element={<Jobs />} />
        <Route path="/jobs/:id" element={<JobDetail />} />
        <Route path="/alerts" element={<Alerts user={user} />} />
        <Route path="/groups" element={<Groups user={user} />} />
        <Route path="/credentials" element={<Credentials user={user} />} />
        <Route path="/settings" element={<Settings user={user} />} />
        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    </Shell>
  )
}

function Shell({ user, onLogout, children }: { user: User; onLogout: () => void; children: React.ReactNode }) {
  const [live, setLive] = useState(false)
  const [toast, setToast] = useState<string | null>(null)
  useEvents((e) => {
    if (e.type === 'stream.connected') setLive(true)
    else if (e.type === 'stream.disconnected') setLive(false)
    else setLive(true)
    if (e.type === 'alert.raised') {
      const a = e.data as { description: string; severity: string }
      setToast(`${a.severity.toUpperCase()}: ${a.description}`)
      setTimeout(() => setToast(null), 6000)
    }
  })
  const canOperate = user.role !== 'viewer'
  return (
    <div className="shell">
      <aside className="sidebar">
        <div className="brand">Print<span>Marshal</span></div>
        <nav>
          <NavLink to="/" end>Dashboard</NavLink>
          <NavLink to="/devices">Devices</NavLink>
          <NavLink to="/consumables">Consumables</NavLink>
          <NavLink to="/alerts">Alerts</NavLink>
          <NavLink to="/groups">Groups</NavLink>
          <NavLink to="/discovery">Discovery</NavLink>
          <NavLink to="/jobs">Jobs</NavLink>
          {canOperate && <NavLink to="/credentials">Credentials</NavLink>}
          <NavLink to="/settings">Settings</NavLink>
        </nav>
        <div className="sidebar-footer">
          <div className="muted small">{user.username} · {user.role}</div>
          <div className="muted small">{live ? '● live' : '○ connecting'}</div>
          <button className="link" onClick={onLogout}>Sign out</button>
        </div>
      </aside>
      <main className="content">{children}</main>
      {toast && <div className="toast">{toast}</div>}
    </div>
  )
}
