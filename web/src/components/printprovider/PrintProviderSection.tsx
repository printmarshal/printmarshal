import { useCallback, useEffect, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, fmtTime } from '../../api'
import type { Group, Job, JobResult, User } from '../../types'
import { Ic } from '../icons'
import { Banner, Btn, Card, JobPill, Pill, Tag } from '../ui'

// Optional print-provider add-on (spec §8.7): a CUPS / IPP Everywhere bridge.
// Off by default; it only configures CUPS on the PrintMarshal host and never
// writes to a printer. Types are local because the module lives outside the core.

type Mode = 'cups' | 'ipp-everywhere-list'
interface PPSettings { enabled: boolean; mode: Mode; queuePrefix: string; defaultOptions: string; autoCreateForGroupId: string; driverless: boolean }
interface Availability { backend: 'cups' | 'dry'; os: string; linux: boolean; cupsFound: boolean; tools?: Record<string, string>; reason?: string }
interface Queue { name: string; uri: string; description?: string; location?: string; managed: boolean; deviceId?: string }
type Action = 'create' | 'update' | 'remove' | 'unchanged' | 'skip' | 'list'
interface PlanEntry { deviceId?: string; deviceName?: string; address?: string; queueName: string; uri?: string; action: Action; reason?: string; current?: Queue; commands?: string[] }
interface Plan { enabled: boolean; mode: Mode; backend: string; inventoryRead: boolean; entries: PlanEntry[]; summary: Record<string, number>; generatedAt: string }
interface State { settings: PPSettings; availability: Availability; queues: Queue[]; inventoryError?: string }

const actionTone: Record<Action, 'ok' | 'warn' | 'crit' | 'off' | 'unk' | 'info' | 'new'> = { create: 'new', update: 'warn', remove: 'crit', unchanged: 'ok', skip: 'off', list: 'info' }
const actionLabel: Record<Action, string> = { create: 'create', update: 'update', remove: 'remove', unchanged: 'unchanged', skip: 'skipped', list: 'publish URI' }

function AvailabilityPill({ a }: { a: Availability }) {
  if (a.backend === 'cups') return <Pill tone="ok">CUPS found{a.tools?.lpadmin ? ` · ${a.tools.lpadmin}` : ''}</Pill>
  if (!a.linux) return <Pill tone="off">Not Linux ({a.os}) · preview only</Pill>
  return <Pill tone="warn">CUPS not found</Pill>
}

export default function PrintProviderSection({ user }: { user: User }) {
  const isAdmin = user.role === 'admin'
  const canOperate = user.role !== 'viewer'
  const [st, setSt] = useState<State | null>(null)
  const [form, setForm] = useState<PPSettings | null>(null)
  const [groups, setGroups] = useState<Group[]>([])
  const [plan, setPlan] = useState<Plan | null>(null)
  const [planErr, setPlanErr] = useState('')
  const [msg, setMsg] = useState<{ ok: boolean; text: string } | null>(null)
  const [job, setJob] = useState<Job | null>(null)
  const [results, setResults] = useState<JobResult[]>([])
  const [showCmds, setShowCmds] = useState(false)
  const [busy, setBusy] = useState(false)
  const timer = useRef<number | null>(null)

  const load = useCallback(async () => {
    try {
      const s = await api.get<State>('/api/print-provider')
      setSt(s); setForm(s.settings)
    } catch (err) { setMsg({ ok: false, text: (err as Error).message }) }
  }, [])
  const loadPlan = useCallback(async () => {
    setPlanErr('')
    try { setPlan(await api.get<Plan>('/api/print-provider/plan')) } catch (err) { setPlan(null); setPlanErr((err as Error).message) }
  }, [])
  useEffect(() => {
    load(); loadPlan()
    api.get<Group[]>('/api/groups').then(setGroups).catch(() => {})
    return () => { if (timer.current) window.clearTimeout(timer.current) }
  }, [load, loadPlan])

  // Follow a submitted job until it finishes, then refresh the plan and inventory.
  useEffect(() => {
    if (!job || (job.status !== 'queued' && job.status !== 'running')) return
    timer.current = window.setTimeout(async () => {
      try {
        const r = await api.get<{ job: Job; results: JobResult[] }>(`/api/jobs/${job.id}`)
        setJob(r.job); setResults(r.results || [])
        if (r.job.status !== 'queued' && r.job.status !== 'running') { load(); loadPlan() }
      } catch { /* keep polling */ }
    }, 1200)
    return () => { if (timer.current) window.clearTimeout(timer.current) }
  }, [job, load, loadPlan])

  const save = async (e: React.FormEvent) => {
    e.preventDefault(); if (!form) return; setMsg(null)
    try {
      const saved = await api.put<PPSettings>('/api/print-provider', form)
      setForm(saved); setMsg({ ok: true, text: 'Saved.' }); load(); loadPlan()
    } catch (err) { setMsg({ ok: false, text: (err as Error).message }) }
  }
  const sync = async (dryRun: boolean) => {
    if (!dryRun && !confirm('Sync queues now? This runs lpadmin on the PrintMarshal host and creates, re-points or removes the queues shown in the plan. Printers are not touched.')) return
    setBusy(true); setMsg(null); setResults([])
    try { const r = await api.post<{ job: Job }>('/api/print-provider/sync', { dryRun }); setJob(r.job) } catch (err) { setMsg({ ok: false, text: (err as Error).message }) } finally { setBusy(false) }
  }
  const remove = async (q: Queue) => {
    if (!q.deviceId || !confirm(`Remove queue ${q.name} from this host?`)) return
    setMsg(null)
    try { await api.del(`/api/print-provider/queues/${q.deviceId}`); setMsg({ ok: true, text: `Removed ${q.name}.` }); load(); loadPlan() } catch (err) { setMsg({ ok: false, text: (err as Error).message }) }
  }

  if (!st || !form) return <Card title="Print provider" id="pp"><div className="cb cap">{msg && !msg.ok ? msg.text : 'Loading…'}</div></Card>
  const a = st.availability
  const live = st.settings.enabled && st.settings.mode === 'cups'
  const canSync = canOperate && live
  const changes = plan ? (plan.summary.create || 0) + (plan.summary.update || 0) + (plan.summary.remove || 0) : 0
  const running = job && (job.status === 'queued' || job.status === 'running')

  return (
    <>
      <Banner tone="info" role="note">
        <strong>Optional add-on, off by default.</strong> The print provider lets this PrintMarshal host also act as a print server: it creates CUPS queues (IPP Everywhere) for discovered printers. It only configures CUPS on <em>this host</em> and never changes a printer. It is a convenience, not fleet management. See <code className="mono">docs/print-provider.md</code>.
      </Banner>

      <form className="card" onSubmit={save} aria-labelledby="pp">
        <div className="ch">
          <div><h2 className="h2" id="pp">Print provider</h2><div className="cap">CUPS / IPP Everywhere bridge · Linux host with CUPS required for real queues</div></div>
          <div className="row"><AvailabilityPill a={a} />{st.settings.enabled ? <Pill tone={live ? 'ok' : 'info'}>{live ? 'enabled' : 'list mode'}</Pill> : <Pill tone="off">disabled</Pill>}</div>
        </div>
        {a.backend !== 'cups' && <div className="cb cap" style={{ borderBottom: '1px solid var(--border)' }}><Ic.info size={13} style={{ verticalAlign: '-2px', marginRight: 6 }} />{a.reason || 'Real queues need a Linux host with CUPS.'} Previews still work: the dry backend records the commands it would run.</div>}
        <SRow label="Enable" hint="nothing runs on this host until this is on">
          <label className="cap" style={{ display: 'inline-flex', alignItems: 'center', gap: 8 }}><input className="cbx" type="checkbox" checked={form.enabled} disabled={!isAdmin} onChange={(e) => setForm({ ...form, enabled: e.target.checked })} />Create and serve print queues for discovered printers</label>
        </SRow>
        <SRow label="Mode" hint="“List” only publishes the IPP Everywhere URIs for clients to add themselves">
          <select className="fin" style={{ width: 260 }} value={form.mode} disabled={!isAdmin} aria-label="Mode" onChange={(e) => setForm({ ...form, mode: e.target.value as Mode })}>
            <option value="cups">CUPS queues on this host</option>
            <option value="ipp-everywhere-list">IPP Everywhere list (no local changes)</option>
          </select>
        </SRow>
        <SRow label="Queue prefix" hint="marks the queues this module owns; only prefixed queues are ever removed"><input className="fin mono" style={{ width: 160 }} value={form.queuePrefix} disabled={!isAdmin} aria-label="Queue prefix" onChange={(e) => setForm({ ...form, queuePrefix: e.target.value })} /></SRow>
        <SRow label="Driverless" hint="IPP Everywhere (lpadmin -m everywhere); off creates raw queues and clients supply the driver">
          <label className="cap" style={{ display: 'inline-flex', alignItems: 'center', gap: 8 }}><input className="cbx" type="checkbox" checked={form.driverless} disabled={!isAdmin} onChange={(e) => setForm({ ...form, driverless: e.target.checked })} />Use the CUPS “everywhere” model</label>
        </SRow>
        <SRow label="Default options" hint="lpadmin -o key=value pairs, space separated"><input className="fin mono" style={{ width: '100%', maxWidth: 420 }} placeholder="media=iso_a4_210x297mm sides=two-sided-long-edge" value={form.defaultOptions} disabled={!isAdmin} aria-label="Default options" onChange={(e) => setForm({ ...form, defaultOptions: e.target.value })} /></SRow>
        <SRow label="Only for group" hint="restrict queue creation to one group; empty = every IPP-capable device">
          <select className="fin" style={{ width: 260 }} value={form.autoCreateForGroupId} disabled={!isAdmin} aria-label="Group" onChange={(e) => setForm({ ...form, autoCreateForGroupId: e.target.value })}>
            <option value="">All devices</option>
            {groups.map((g) => <option key={g.id} value={g.id}>{g.name}</option>)}
          </select>
        </SRow>
        {isAdmin && <div className="cf" style={{ display: 'flex', justifyContent: 'flex-end', gap: 8, alignItems: 'center' }}>{msg && <span className={msg.ok ? 'cap' : 'error small'} style={{ marginRight: 'auto' }}>{msg.text}</span>}<Btn type="submit" variant="pri">Save</Btn></div>}
        {!isAdmin && msg && <div className="cf"><span className={msg.ok ? 'cap' : 'error small'}>{msg.text}</span></div>}
      </form>

      <Card title="Plan" id="ppplan" sub={plan ? <>{plan.inventoryRead ? 'diff against the queues on this host' : st.settings.enabled ? 'list mode: URIs only, nothing is created' : 'module disabled: existing queues were not inspected, so “create” may already exist'} · {fmtTime(plan.generatedAt)}</> : 'device → queue name → action'}
        right={<div className="row">
          <Btn sm variant="ghost" onClick={loadPlan}><Ic.refresh size={14} />Refresh</Btn>
          {canOperate && <Btn sm variant="ghost" disabled={!canSync || busy || !!running} title={canSync ? 'Runs the sync job without touching this host' : 'Enable the module in CUPS mode first'} onClick={() => sync(true)}>Preview (dry run)</Btn>}
          {canOperate && <Btn sm variant="wr" disabled={!canSync || busy || !!running} title="Changes this host: runs lpadmin / cupsenable / cupsaccept" onClick={() => sync(false)}><Ic.warn size={14} />Sync queues · changes this host</Btn>}
        </div>}>
        {planErr && <div className="cb error small">{planErr}</div>}
        {plan && (
          <>
            <div className="cb row" style={{ gap: 6 }}>
              {(['create', 'update', 'remove', 'unchanged', 'list', 'skip'] as Action[]).filter((k) => plan.summary[k]).map((k) => <Pill key={k} tone={actionTone[k]} xs>{plan.summary[k]} {actionLabel[k]}</Pill>)}
              {plan.entries.length === 0 && <span className="cap">No devices to plan for.</span>}
              {changes > 0 && live && <span className="cap" style={{ marginLeft: 'auto' }}>{changes} change{changes === 1 ? '' : 's'} pending</span>}
              {plan.entries.some((e) => e.commands?.length) && <label className="cap" style={{ display: 'inline-flex', alignItems: 'center', gap: 6, marginLeft: changes > 0 && live ? 0 : 'auto' }}><input className="cbx" type="checkbox" checked={showCmds} onChange={(e) => setShowCmds(e.target.checked)} />Show commands</label>}
            </div>
            {plan.entries.length > 0 && (
              <div className="tw"><table className="tbl compact" style={{ minWidth: 720 }}>
                <thead><tr><th>Device</th><th>Queue name</th><th>URI</th><th>Action</th><th>Note</th></tr></thead>
                <tbody>
                  {plan.entries.map((e, i) => (
                    <tr key={e.deviceId || e.queueName || i} className={e.action === 'skip' ? 'dim' : undefined}>
                      <td>{e.deviceId ? <Link to={`/devices/${e.deviceId}`}>{e.deviceName}</Link> : <span className="cap">— (orphan queue)</span>}{e.address && <div className="cap mono">{e.address}</div>}</td>
                      <td className="mono">{e.queueName || '—'}{e.current && e.current.name !== e.queueName && <div className="cap">was {e.current.name}</div>}</td>
                      <td className="mono cap">{e.uri || '—'}{e.action === 'update' && e.current && e.current.uri !== e.uri && <div><span className="old">{e.current.uri}</span></div>}</td>
                      <td><Pill tone={actionTone[e.action]} xs>{actionLabel[e.action]}</Pill></td>
                      <td className="cap wrap" style={{ whiteSpace: 'normal', maxWidth: 360 }}>{e.reason}{showCmds && e.commands?.length ? <pre className="mono" style={{ margin: '4px 0 0', whiteSpace: 'pre-wrap', fontSize: 11 }}>{e.commands.join('\n')}</pre> : null}</td>
                    </tr>
                  ))}
                </tbody>
              </table></div>
            )}
          </>
        )}
        {job && (
          <div className="cf">
            <div className="row"><span className="cap">Sync job</span><JobPill status={job.status} /><Link to={`/jobs/${job.id}`} className="cap">{job.id.slice(0, 8)}</Link>{job.params?.dryRun ? <Tag>dry run</Tag> : <Tag>changed host</Tag>}<span className="cap">{job.done}/{job.total}{job.failed ? ` · ${job.failed} failed` : ''}</span>{job.error && <span className="error small">{job.error}</span>}</div>
            {results.length > 0 && (
              <ul className="cap" style={{ margin: '8px 0 0', paddingLeft: 18 }}>
                {results.map((r) => <li key={r.id}><span className={r.status === 'error' ? 'error' : undefined}>{r.message}</span>{r.address ? <span className="mono"> · {r.address}</span> : null}</li>)}
              </ul>
            )}
          </div>
        )}
      </Card>

      <Card title="Current queues" id="ppq" sub={live ? 'as reported by lpstat on this host; only prefixed queues are managed' : 'shown once the module is enabled in CUPS mode'}>
        {st.inventoryError && <div className="cb error small">{st.inventoryError}</div>}
        {st.queues.length === 0 ? <div className="cb cap">{live ? 'No queues on this host.' : 'Nothing to show while the module is off.'}</div> : (
          <div className="tw"><table className="tbl compact" style={{ minWidth: 640 }}>
            <thead><tr><th>Queue</th><th>URI</th><th>Description</th><th>Location</th><th></th><th></th></tr></thead>
            <tbody>
              {st.queues.map((q) => (
                <tr key={q.name} className={q.managed ? undefined : 'dim'}>
                  <td className="mono">{q.name}</td>
                  <td className="mono cap">{q.uri}</td>
                  <td>{q.description || '—'}</td>
                  <td className="cap">{q.location || '—'}</td>
                  <td>{q.managed ? <Tag>managed</Tag> : <Tag>not ours</Tag>}</td>
                  <td className="r">{canOperate && q.managed && q.deviceId && <Btn sm variant="ghost" onClick={() => remove(q)}>Remove</Btn>}</td>
                </tr>
              ))}
            </tbody>
          </table></div>
        )}
      </Card>
    </>
  )
}

function SRow({ label, hint, children }: { label: React.ReactNode; hint?: string; children: React.ReactNode }) {
  return <div className="srow2"><span>{label}{hint && <div className="cap">{hint}</div>}</span><div>{children}</div></div>
}
