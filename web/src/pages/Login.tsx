import { useEffect, useState } from 'react'
import { api } from '../api'
import type { User } from '../types'
import { Mark } from '../components/icons'
import { Btn } from '../components/ui'

interface Providers { local: boolean; oidc: { enabled: boolean; name?: string; start?: string } }

export default function Login({ onLogin }: { onLogin: (u: User) => void }) {
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState(() => new URLSearchParams(location.search).get('oidc_error') || '')
  const [busy, setBusy] = useState(false)
  const [oidc, setOidc] = useState<Providers['oidc'] | null>(null)
  useEffect(() => { api.get<Providers>('/api/auth/providers').then((p) => setOidc(p.oidc)).catch(() => {}) }, [])
  const submit = async (e: React.FormEvent) => {
    e.preventDefault(); setBusy(true); setError('')
    try { const r = await api.login(username, password); onLogin(r.user) } catch (err) { setError((err as Error).message || 'login failed') } finally { setBusy(false) }
  }
  return (
    <div className="login">
      <form onSubmit={submit} aria-label="Sign in">
        <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}><Mark size={30} /><b className="wm" style={{ fontSize: 18 }}>Sign in to Print<span style={{ color: 'var(--accent)' }}>Marshal</span></b></div>
        <span className="cap mono">{location.host} · self-hosted</span>
        {oidc?.enabled && (
          <>
            <Btn variant="pri" onClick={() => { location.href = oidc.start || '/api/auth/oidc/start' }} style={{ marginTop: 4 }}>Sign in with {oidc.name || 'single sign-on'}</Btn>
            <div className="cap" style={{ textAlign: 'center' }}>or use a local account</div>
          </>
        )}
        <label className="cap" htmlFor="un">Username</label>
        <input className="fin" id="un" value={username} onChange={(e) => setUsername(e.target.value)} autoComplete="username" autoFocus={!oidc?.enabled} />
        <label className="cap" htmlFor="pw">Password</label>
        <input className="fin" id="pw" type="password" value={password} onChange={(e) => setPassword(e.target.value)} autoComplete="current-password" />
        {error && <div className="error small" role="alert">{error}</div>}
        <Btn type="submit" variant={oidc?.enabled ? undefined : 'pri'} disabled={busy} style={{ marginTop: 4 }}>{busy ? 'Signing in…' : 'Sign in'}</Btn>
        <div className="cap">First run: the admin password comes from PM_ADMIN_PASSWORD or the server log.</div>
      </form>
    </div>
  )
}
