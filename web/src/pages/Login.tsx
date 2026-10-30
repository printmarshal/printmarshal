import { useEffect, useState } from 'react'
import { api } from '../api'
import type { User } from '../types'
import { Mark, Wordmark } from '../components/icons'
import { Btn } from '../components/ui'

/** One enabled sign-in method from GET /api/auth/providers. */
interface Provider { kind: 'oidc' | 'saml' | 'ldap' | 'radius'; name: string; method: 'redirect' | 'password'; start?: string }
interface Providers { local: boolean; oidc: { enabled: boolean; name?: string; start?: string }; providers?: Provider[] }

function initialError(): string {
  const q = new URLSearchParams(location.search)
  return q.get('sso_error') || q.get('oidc_error') || ''
}

export default function Login({ onLogin }: { onLogin: (u: User) => void }) {
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState(initialError)
  const [busy, setBusy] = useState(false)
  const [providers, setProviders] = useState<Provider[]>([])
  useEffect(() => {
    api.get<Providers>('/api/auth/providers').then((p) => {
      // Older servers only report OIDC; synthesise its row so the button keeps working.
      const list = p.providers ?? (p.oidc?.enabled ? [{ kind: 'oidc' as const, name: p.oidc.name || 'single sign-on', method: 'redirect' as const, start: p.oidc.start }] : [])
      setProviders(list)
    }).catch(() => {})
  }, [])
  const redirect = providers.filter((p) => p.method === 'redirect')
  const password_ = providers.filter((p) => p.method === 'password')
  const submit = async (e: React.FormEvent) => {
    e.preventDefault(); setBusy(true); setError('')
    try { const r = await api.login(username, password); onLogin(r.user) } catch (err) { setError((err as Error).message || 'login failed') } finally { setBusy(false) }
  }
  const formHint = password_.length
    ? `or use your ${password_.map((p) => p.name).join(' / ')} or local account`
    : 'or use a local account'
  return (
    <div className="login">
      <form onSubmit={submit} aria-label="Sign in">
        <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}><Mark size={30} /><b style={{ fontSize: 18, display: 'inline-flex', alignItems: 'baseline', gap: 8 }}>Sign in to <Wordmark size={18} /></b></div>
        <span className="cap mono">{location.host} · self-hosted</span>
        {redirect.length > 0 && (
          <>
            {redirect.map((p, i) => (
              <Btn key={p.kind} variant="pri" onClick={() => { location.href = p.start || `/api/auth/${p.kind}/start` }} style={{ marginTop: i === 0 ? 4 : 0 }}>Sign in with {p.name}</Btn>
            ))}
            <div className="cap" style={{ textAlign: 'center' }}>{formHint}</div>
          </>
        )}
        <label className="cap" htmlFor="un">Username</label>
        <input className="fin" id="un" value={username} onChange={(e) => setUsername(e.target.value)} autoComplete="username" autoFocus={redirect.length === 0} />
        <label className="cap" htmlFor="pw">Password</label>
        <input className="fin" id="pw" type="password" value={password} onChange={(e) => setPassword(e.target.value)} autoComplete="current-password" />
        {error && <div className="error small" role="alert">{error}</div>}
        <Btn type="submit" variant={redirect.length ? undefined : 'pri'} disabled={busy} style={{ marginTop: 4 }}>{busy ? 'Signing in…' : 'Sign in'}</Btn>
        {password_.length > 0 && redirect.length === 0 && (
          <div className="cap">Local accounts and {password_.map((p) => p.name).join(' / ')} accounts sign in here.</div>
        )}
        <div className="cap">First run: the admin password comes from PM_ADMIN_PASSWORD or the initial-admin-password file in the data directory.</div>
      </form>
    </div>
  )
}
