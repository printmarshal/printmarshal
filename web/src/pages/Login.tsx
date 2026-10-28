import { useState } from 'react'
import { api } from '../api'
import type { User } from '../types'
import { Mark } from '../components/icons'
import { Btn } from '../components/ui'

export default function Login({ onLogin }: { onLogin: (u: User) => void }) {
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const submit = async (e: React.FormEvent) => {
    e.preventDefault(); setBusy(true); setError('')
    try { const r = await api.login(username, password); onLogin(r.user) } catch (err) { setError((err as Error).message || 'login failed') } finally { setBusy(false) }
  }
  return (
    <div className="login">
      <form onSubmit={submit} aria-label="Sign in">
        <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}><Mark size={30} /><b className="wm" style={{ fontSize: 18 }}>Sign in to Print<span style={{ color: 'var(--accent)' }}>Marshal</span></b></div>
        <span className="cap mono">{location.host} · self-hosted</span>
        <label className="cap" htmlFor="un">Username</label>
        <input className="fin" id="un" value={username} onChange={(e) => setUsername(e.target.value)} autoComplete="username" autoFocus />
        <label className="cap" htmlFor="pw">Password</label>
        <input className="fin" id="pw" type="password" value={password} onChange={(e) => setPassword(e.target.value)} autoComplete="current-password" />
        {error && <div className="error small">{error}</div>}
        <Btn type="submit" variant="pri" disabled={busy} style={{ marginTop: 4 }}>{busy ? 'Signing in…' : 'Sign in'}</Btn>
        <div className="cap">First run: the admin password comes from PM_ADMIN_PASSWORD or the server log.</div>
      </form>
    </div>
  )
}
