import { useState } from 'react'
import { api } from '../api'
import type { User } from '../types'

export default function Login({ onLogin }: { onLogin: (u: User) => void }) {
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const submit = async (e: React.FormEvent) => {
    e.preventDefault()
    setBusy(true); setError('')
    try {
      const r = await api.login(username, password)
      onLogin(r.user)
    } catch (err) {
      setError((err as Error).message || 'login failed')
    } finally { setBusy(false) }
  }
  return (
    <div className="login panel">
      <div className="brand">Print<span>Marshal</span></div>
      <form onSubmit={submit}>
        <label className="field">Username<input autoFocus value={username} onChange={(e) => setUsername(e.target.value)} autoComplete="username" /></label>
        <label className="field">Password<input type="password" value={password} onChange={(e) => setPassword(e.target.value)} autoComplete="current-password" /></label>
        {error && <div className="error">{error}</div>}
        <button className="primary" disabled={busy}>Sign in</button>
        <div className="muted small">First run: the admin password comes from PM_ADMIN_PASSWORD or the server log.</div>
      </form>
    </div>
  )
}
