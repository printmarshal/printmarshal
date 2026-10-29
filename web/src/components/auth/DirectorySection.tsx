import { useCallback, useEffect, useState } from 'react'
import { api } from '../../api'
import { Banner, Btn, Card, Tag } from '../ui'
import { Ic } from '../icons'

// Settings → Sign-in & directory: LDAP / Active Directory configured at
// runtime (issue #8). Fields the configuration file or environment set are
// shown read-only with their source; the service-account password is
// write-only and stored encrypted on the server.

interface DirectorySettings {
  enabled: boolean; url: string; startTls: boolean; caFile: string; insecureSkipVerify: boolean
  bindDN: string; userDNTemplate: string; baseDN: string; userFilter: string
  usernameAttr: string; subjectAttr: string; groupAttr: string
  roleMap: Record<string, string>; defaultRole: string; allowedGroups: string[]; timeout: string; displayName: string
}
interface DirectoryPage {
  enabled: boolean; fileConfigured: boolean; settings: DirectorySettings; sources: Record<string, string>; saved: DirectorySettings
  bindPassword: { set: boolean; source: string }; issuer: string; name: string
  defaults: { userFilter: string; timeout: string; groupAttr: string; defaultRole: string }
}
interface TestResult { steps: { name: string; ok: boolean; error?: string }[]; signIn?: { dn: string; username: string; subject: string; groups: string[]; role: string; allowed: boolean } }
interface Provider { kind: string; name: string; method: string }

const empty: DirectorySettings = { enabled: false, url: '', startTls: false, caFile: '', insecureSkipVerify: false, bindDN: '', userDNTemplate: '', baseDN: '', userFilter: '', usernameAttr: '', subjectAttr: '', groupAttr: '', roleMap: {}, defaultRole: '', allowedGroups: [], timeout: '', displayName: '' }

export default function DirectorySection() {
  const [page, setPage] = useState<DirectoryPage | null>(null)
  const [form, setForm] = useState<DirectorySettings>(empty)
  const [password, setPassword] = useState('')
  const [clearPw, setClearPw] = useState(false)
  const [providers, setProviders] = useState<Provider[]>([])
  const [msg, setMsg] = useState<{ ok: boolean; text: string } | null>(null)
  const [test, setTest] = useState<{ username: string; password: string }>({ username: '', password: '' })
  const [result, setResult] = useState<TestResult | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  const load = useCallback(() => {
    api.get<DirectoryPage>('/api/settings/directory').then((p) => { setPage(p); setForm({ ...p.settings, roleMap: { ...p.settings.roleMap }, allowedGroups: [...p.settings.allowedGroups] }) }).catch((e) => setError((e as Error).message))
    api.get<{ providers: Provider[] }>('/api/auth/providers').then((r) => setProviders(r.providers)).catch(() => {})
  }, [])
  useEffect(() => { load() }, [load])

  const locked = (field: string) => !!page && (page.sources[field] === 'file' || page.sources[field] === 'env')
  const Src = ({ field }: { field: string }) => {
    const s = page?.sources[field]
    if (s === 'file') return <Tag title="Set in the configuration file; change it there">file</Tag>
    if (s === 'env') return <Tag title="Set by an environment variable; change it there">env</Tag>
    return null
  }
  const set = <K extends keyof DirectorySettings>(k: K, v: DirectorySettings[K]) => setForm((f) => ({ ...f, [k]: v }))
  const body = () => ({ ...form, ...(clearPw ? { bindPassword: '' } : password ? { bindPassword: password } : {}) })

  const save = async (e: React.FormEvent) => {
    e.preventDefault(); setMsg(null); setBusy(true)
    try { const p = await api.put<DirectoryPage>('/api/settings/directory', body()); setPage(p); setForm({ ...p.settings }); setPassword(''); setClearPw(false); setMsg({ ok: true, text: 'Saved. In force for the next sign-in; no restart needed.' }) } catch (err) { setMsg({ ok: false, text: (err as Error).message }) } finally { setBusy(false) }
  }
  const runTest = async (withUser: boolean) => {
    setResult(null); setMsg(null); setBusy(true)
    try {
      const r = await api.post<TestResult>('/api/settings/directory/test', { settings: form, ...(password ? { bindPassword: password } : {}), ...(withUser ? test : {}) })
      setResult(r)
    } catch (err) { setMsg({ ok: false, text: (err as Error).message }) } finally { setBusy(false) }
  }

  const roleRows = Object.entries(form.roleMap)
  const setRole = (i: number, group: string, role: string) => {
    const entries = roleRows.map(([g, r], j) => (j === i ? [group, role] : [g, r]))
    set('roleMap', Object.fromEntries(entries.filter(([g]) => g !== undefined)))
  }
  const addRole = () => { if (!('' in form.roleMap)) set('roleMap', { ...form.roleMap, '': 'viewer' }) }
  const delRole = (g: string) => { const m = { ...form.roleMap }; delete m[g]; set('roleMap', m) }

  if (error) return <Banner tone="crit">{error}</Banner>
  if (!page) return <Card title="Sign-in & directory"><div className="cb cap">Loading…</div></Card>

  const F = ({ field, label, hint, children }: { field: string; label: string; hint?: string; children: React.ReactNode }) => (
    <div className="srow2"><span><label htmlFor={`dir-${field}`}>{label}</label>{hint && <div className="cap">{hint}</div>}</span><div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>{children}<Src field={field} /></div></div>
  )
  const Text = ({ field, k, placeholder, mono }: { field: string; k: keyof DirectorySettings; placeholder?: string; mono?: boolean }) => (
    <input className={`fin${mono ? ' mono' : ''}`} id={`dir-${field}`} style={{ flex: 1 }} value={String(form[k] ?? '')} placeholder={placeholder} disabled={locked(field)} onChange={(e) => set(k, e.target.value as never)} />
  )
  const Check = ({ field, k, label }: { field: string; k: keyof DirectorySettings; label: string }) => (
    <label className="chip"><input type="checkbox" id={`dir-${field}`} checked={!!form[k]} disabled={locked(field)} onChange={(e) => set(k, e.target.checked as never)} /> {label}</label>
  )

  return (
    <>
      <Card title="Sign-in methods" id="sm" sub="what the login page offers besides local accounts">
        <div className="cb" style={{ display: 'flex', flexWrap: 'wrap', gap: 8 }}>
          <Tag>local accounts</Tag>
          {providers.map((p) => <Tag key={p.kind} title={p.method === 'redirect' ? 'single sign-on redirect' : 'password checked by the provider'}>{p.kind.toUpperCase()} · {p.name}</Tag>)}
          {providers.length === 0 && <span className="cap">No external provider is active. OIDC, SAML and RADIUS are configured in the configuration file or environment (see the auth guide); the directory below can be set up here.</span>}
        </div>
      </Card>

      <form className="card" id="directory" onSubmit={save} autoComplete="off" aria-labelledby="dirh">
        <div className="ch"><h2 className="h2" id="dirh">LDAP / Active Directory</h2>
          <div className="row" style={{ gap: 8 }}>
            {page.fileConfigured && <Tag title="The configuration file or environment names the server; those values win and are read-only here">configured by file</Tag>}
            <Btn type="submit" sm variant="pri" disabled={busy}>Save</Btn>
          </div>
        </div>
        <div className="cap" style={{ padding: '10px 16px', borderBottom: '1px solid var(--border)' }}>
          Users type their directory credentials into the ordinary login form; PrintMarshal binds to the directory on their behalf. Accounts are bound by the directory's stable identifier, and the mapped role is re-applied on every sign-in. Fields marked <Tag>file</Tag> or <Tag>env</Tag> come from the configuration file or environment and win over anything saved here.
        </div>
        <div style={{ padding: '6px 0' }}>
          <F field="enabled" label="Directory sign-in" hint={page.fileConfigured ? 'always on while the file names a server' : 'off keeps the settings but stops sign-ins'}><Check field="enabled" k="enabled" label={form.enabled ? 'Enabled' : 'Disabled'} /></F>
          <F field="url" label="Server URL" hint="ldap://host:389 or ldaps://host:636"><Text field="url" k="url" placeholder="ldaps://dc1.corp.example.com:636" mono /></F>
          <F field="startTls" label="StartTLS" hint="upgrade an ldap:// connection before binding"><Check field="startTls" k="startTls" label={form.startTls ? 'On' : 'Off'} /></F>
          <F field="caFile" label="CA bundle (PEM)" hint="path on the server; system roots when empty"><Text field="caFile" k="caFile" placeholder="/etc/printmarshal/corp-root-ca.pem" mono /></F>
          <F field="insecureSkipVerify" label="Skip certificate verification" hint="lab use only"><Check field="insecureSkipVerify" k="insecureSkipVerify" label={form.insecureSkipVerify ? 'Skipped (insecure)' : 'Verified'} /></F>
          <F field="displayName" label="Name on the login page" hint="defaults to the server host"><Text field="displayName" k="displayName" placeholder="Corp AD" /></F>
          <F field="timeout" label="Timeout" hint={`per connection; default ${page.defaults.timeout}`}><Text field="timeout" k="timeout" placeholder={page.defaults.timeout} mono /></F>

          <div className="cap" style={{ padding: '12px 16px 4px', fontWeight: 600 }}>Finding the user</div>
          <F field="bindDN" label="Service account DN" hint="read-only account used to search; leave empty to bind directly as the user"><Text field="bindDN" k="bindDN" placeholder="CN=printmarshal,OU=Service Accounts,DC=corp,DC=example,DC=com" mono /></F>
          <div className="srow2"><span><label htmlFor="dir-pw">Service account password</label><div className="cap">{page.bindPassword.source === 'env' ? `from the environment variable named in the file` : page.bindPassword.set ? 'stored encrypted · never shown' : 'not set'}</div></span>
            <div style={{ display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap' }}>
              <input className="fin" id="dir-pw" type="password" style={{ flex: 1, minWidth: 180 }} value={password} placeholder={page.bindPassword.set ? '•••••••• (unchanged)' : 'enter to set'} autoComplete="new-password" disabled={page.bindPassword.source === 'env' || clearPw} onChange={(e) => setPassword(e.target.value)} />
              {page.bindPassword.set && page.bindPassword.source !== 'env' && <label className="chip"><input type="checkbox" checked={clearPw} onChange={(e) => { setClearPw(e.target.checked); if (e.target.checked) setPassword('') }} /> Clear</label>}
              {page.bindPassword.source === 'env' && <Tag>env</Tag>}
            </div>
          </div>
          <F field="userDNTemplate" label="User DN template" hint="direct bind without a service account; %s is the login name"><Text field="userDNTemplate" k="userDNTemplate" placeholder="uid=%s,ou=people,dc=example,dc=com or %s@corp.example.com" mono /></F>
          <F field="baseDN" label="Search base"><Text field="baseDN" k="baseDN" placeholder="OU=People,DC=corp,DC=example,DC=com" mono /></F>
          <F field="userFilter" label="User filter" hint="%s is the escaped login name"><Text field="userFilter" k="userFilter" placeholder={page.defaults.userFilter} mono /></F>
          <F field="usernameAttr" label="Username attribute" hint="default: the attribute that matched"><Text field="usernameAttr" k="usernameAttr" placeholder="sAMAccountName" mono /></F>
          <F field="subjectAttr" label="Stable identifier" hint="default objectGUID, then entryUUID, then the DN"><Text field="subjectAttr" k="subjectAttr" placeholder="objectGUID" mono /></F>
          <F field="groupAttr" label="Group attribute" hint={`default ${page.defaults.groupAttr}`}><Text field="groupAttr" k="groupAttr" placeholder={page.defaults.groupAttr} mono /></F>

          <div className="cap" style={{ padding: '12px 16px 4px', fontWeight: 600 }}>Roles and access</div>
          <F field="defaultRole" label="Default role" hint="when no group below matches">
            <select className="fin" id="dir-defaultRole" value={form.defaultRole} disabled={locked('defaultRole')} onChange={(e) => set('defaultRole', e.target.value)}><option value="">viewer (default)</option><option value="viewer">viewer</option><option value="operator">operator</option><option value="admin">admin</option></select>
          </F>
          <div className="srow2"><span>Role mapping<div className="cap">group DN or CN → role; the highest match wins</div></span>
            <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
              {roleRows.map(([g, r], i) => (
                <div key={i} className="row" style={{ gap: 6 }}>
                  <input className="fin mono" style={{ flex: 1 }} aria-label={`Group ${i + 1}`} value={g} placeholder="CN=PrintMarshal Admins,OU=Groups,DC=corp,DC=example,DC=com" disabled={locked('roleMap')} onChange={(e) => setRole(i, e.target.value, r)} />
                  <select className="fin" style={{ width: 120 }} aria-label={`Role for group ${i + 1}`} value={r} disabled={locked('roleMap')} onChange={(e) => setRole(i, g, e.target.value)}><option value="viewer">viewer</option><option value="operator">operator</option><option value="admin">admin</option></select>
                  {!locked('roleMap') && <Btn sm variant="ghost" aria-label={`Remove group ${i + 1}`} onClick={() => delRole(g)}><Ic.x /></Btn>}
                </div>
              ))}
              <div className="row" style={{ gap: 8 }}>{!locked('roleMap') && <Btn sm onClick={addRole}><Ic.plus />Add group</Btn>}<Src field="roleMap" /></div>
            </div>
          </div>
          <div className="srow2"><span><label htmlFor="dir-allowed">Allowed groups</label><div className="cap">one DN or CN per line; empty allows every directory user</div></span>
            <div style={{ display: 'flex', alignItems: 'flex-start', gap: 8 }}><textarea className="fin mono" id="dir-allowed" rows={3} style={{ flex: 1 }} value={form.allowedGroups.join('\n')} disabled={locked('allowedGroups')} onChange={(e) => set('allowedGroups', e.target.value.split('\n'))} /><Src field="allowedGroups" /></div>
          </div>
        </div>
        {msg && <div className={`cf ${msg.ok ? '' : 'error'} small`} role="status">{msg.text}</div>}
      </form>

      <Card title="Test" id="dirtest" sub="runs against the values above as they are now, saved or not">
        <div className="cb" style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
          <div className="row" style={{ gap: 8, flexWrap: 'wrap' }}>
            <Btn sm onClick={() => runTest(false)} disabled={busy || !form.url}>Test connection</Btn>
            <span className="cap">connects, upgrades to TLS when set, and binds the service account</span>
          </div>
          <div className="row" style={{ gap: 8, flexWrap: 'wrap' }}>
            <input className="fin" style={{ flex: '1 1 160px' }} placeholder="user name" aria-label="Test user name" value={test.username} onChange={(e) => setTest({ ...test, username: e.target.value })} autoComplete="off" />
            <input className="fin" style={{ flex: '1 1 160px' }} type="password" placeholder="that user's password" aria-label="Test user password" value={test.password} onChange={(e) => setTest({ ...test, password: e.target.value })} autoComplete="new-password" />
            <Btn sm onClick={() => runTest(true)} disabled={busy || !form.url || !test.username || !test.password}>Test sign-in</Btn>
            <span className="cap">the password is used for one bind and not stored</span>
          </div>
          {result && (
            <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
              {result.steps.map((s) => <div key={s.name} className="row" style={{ gap: 8, alignItems: 'flex-start' }}>{s.ok ? <Ic.ok size={16} style={{ color: 'var(--ok)', flex: 'none' }} /> : <Ic.crit size={16} style={{ color: 'var(--crit)', flex: 'none' }} />}<span><b>{s.name}</b>{s.error && <div className="cap" style={{ color: 'var(--crit)' }}>{s.error}</div>}</span></div>)}
              {result.signIn && (
                <div className="tw"><table className="tbl mx"><tbody>
                  <tr><td>Username</td><td className="mono">{result.signIn.username}</td></tr>
                  <tr><td>Entry</td><td className="mono" style={{ fontSize: 11 }}>{result.signIn.dn}</td></tr>
                  <tr><td>Stable identifier</td><td className="mono" style={{ fontSize: 11 }}>{result.signIn.subject}</td></tr>
                  <tr><td>Groups</td><td className="mono" style={{ fontSize: 11 }}>{result.signIn.groups.join(' · ') || '—'}</td></tr>
                  <tr><td>Role on sign-in</td><td><Tag>{result.signIn.role}</Tag> {result.signIn.allowed ? '' : <span style={{ color: 'var(--crit)' }}>not in an allowed group</span>}</td></tr>
                </tbody></table></div>
              )}
            </div>
          )}
        </div>
      </Card>
    </>
  )
}
