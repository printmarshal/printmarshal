import { useCallback, useEffect, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { api, ago, fmtTime } from '../api'
import type { Credential, Job, JobResult, User } from '../types'
import { useEvents } from '../ws'
import { Ic } from '../components/icons'
import { Btn, Card, JobBar, JobPill, Pill, Tag } from '../components/ui'
import { ImportDialog } from '../components/ImportDialog'

const importStatus: Record<string, { tone: 'ok' | 'info' | 'off' | 'crit'; label: string }> = {
  created: { tone: 'ok', label: 'Created' },
  updated: { tone: 'info', label: 'Updated' },
  skipped: { tone: 'off', label: 'Skipped' },
  failed: { tone: 'crit', label: 'Failed' },
}

export default function Discovery({ user }: { user: User }) {
  const [runs, setRuns] = useState<Job[]>([])
  const [selected, setSelected] = useState<string | null>(null)
  const [results, setResults] = useState<JobResult[]>([])
  const [targets, setTargets] = useState<string[]>([])
  const [input, setInput] = useState('')
  const [zeroconf, setZeroconf] = useState(true)
  const [active, setActive] = useState(false)
  const [allowLarge, setAllowLarge] = useState(false)
  const [creds, setCreds] = useState<Credential[]>([])
  const [settings, setSettings] = useState<{ discovery?: { maxAddresses?: number; targets?: string[] }; poll?: { concurrency?: number } } | null>(null)
  const [msg, setMsg] = useState('')
  const [importing, setImporting] = useState(false)
  const navigate = useNavigate()
  const canOperate = user.role !== 'viewer'

  const load = useCallback(() => {
    api.get<Job[]>('/api/discovery/runs').then((rs) => { setRuns(rs); setSelected((cur) => cur || rs[0]?.id || null) }).catch(() => {})
  }, [])
  useEffect(() => { load(); api.get('/api/settings').then((s) => setSettings(s as typeof settings)).catch(() => {}); if (canOperate) api.get<Credential[]>('/api/credentials').then(setCreds).catch(() => {}) }, [load, canOperate])
  useEffect(() => { if (!selected) { setResults([]); return } api.get<{ job: Job; results: JobResult[] }>(`/api/jobs/${selected}`).then((r) => setResults(r.results)).catch(() => {}) }, [selected, runs])
  useEvents((e) => { if (e.type === 'job.updated' && ((e.data as Job).type === 'discovery' || (e.data as Job).type === 'import')) load() }, [load])

  const addTarget = (raw: string) => { const list = raw.split(/[\s,]+/).map((t) => t.trim()).filter(Boolean); if (list.length) setTargets((t) => Array.from(new Set([...t, ...list]))); setInput('') }
  const start = async (e: React.FormEvent) => {
    e.preventDefault(); setMsg('')
    const list = active ? [...targets, ...(input.trim() ? [input.trim()] : [])] : []
    if (!zeroconf && list.length === 0) { setMsg('Enable zero-config or add at least one range.'); return }
    try { const j = await api.post<Job>('/api/discovery/runs', { targets: list, zeroconf, allowLarge }); setInput(''); setSelected(j.id); load() } catch (err) { setMsg((err as Error).message) }
  }
  const run = runs.find((r) => r.id === selected) || null
  const found = results.filter((r) => r.status === 'ok')
  const created = found.filter((r) => r.data?.created === true)
  const skipped = results.filter((r) => r.status === 'skipped')
  const errs = results.filter((r) => r.status === 'error')
  const isImport = run?.type === 'import'
  const imp = { created: results.filter((r) => r.status === 'created').length, updated: results.filter((r) => r.status === 'updated').length, skipped: results.filter((r) => r.status === 'skipped').length, failed: results.filter((r) => r.status === 'failed').length }
  const rowOf = (r: JobResult) => (typeof r.data?.row === 'number' ? (r.data.row as number) : null)

  return (
    <>
      <div className="ph">
        <div><h1 className="h1">Discovery</h1><div className="cap">{run ? `Run ${run.id.slice(0, 8)} · ${run.status} · started ${ago(run.createdAt)} by ${run.createdBy}` : 'No runs yet'} · read-only — nothing is written to devices</div></div>
        <div className="row">{canOperate && <Btn sm onClick={() => setImporting(true)}><Ic.upload />Import CSV…</Btn>}<Link className="btn sm" to="/jobs"><Ic.clock />Past runs</Link></div>
      </div>
      <div className="cols">
        <Card title="Run setup" id="cfg" sub={run?.status === 'running' ? 'a run is in progress' : undefined} style={{ flex: '1 1 360px', maxWidth: 460 }}>
          <form className="cb" onSubmit={start} style={{ display: 'flex', flexDirection: 'column', gap: 14 }}>
            <div className="cap" style={{ fontWeight: 500, color: 'var(--text-2)' }}>Methods</div>
            <label className={`meth${zeroconf ? ' on' : ''}`}><input className="cbx" type="checkbox" checked={zeroconf} onChange={(e) => setZeroconf(e.target.checked)} style={{ marginTop: 2 }} /><span><b style={{ fontWeight: 600 }}>Zero-config listen</b><span className="cap" style={{ display: 'block' }}>mDNS / DNS-SD and WS-Discovery announcements · passive; needs the same L2 network as the printers</span></span></label>
            <label className={`meth${active ? ' on' : ''}`}><input className="cbx" type="checkbox" checked={active} onChange={(e) => setActive(e.target.checked)} style={{ marginTop: 2 }} /><span><b style={{ fontWeight: 600 }}>Active scan</b><span className="cap" style={{ display: 'block' }}>Probe SNMP 161 on each address, then identify over SNMP and IPP 631</span></span></label>
            {active && (
              <div>
                <div className="cap" style={{ fontWeight: 500, color: 'var(--text-2)', marginBottom: 6 }}>Ranges · IP, range or CIDR</div>
                <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6 }}>
                  {targets.map((t) => <span key={t} className="rchip mono">{t}<Btn variant="ghost" sm ic style={{ width: 22, height: 22 }} aria-label={`Remove ${t}`} onClick={() => setTargets(targets.filter((x) => x !== t))}><Ic.x size={12} /></Btn></span>)}
                  {targets.length === 0 && settings?.discovery?.targets?.length ? <Btn sm variant="ghost" onClick={() => setTargets(settings.discovery!.targets!)}>Use configured: {settings.discovery.targets.join(', ')}</Btn> : null}
                </div>
                <input className="fin mono" aria-label="Add range" placeholder="Add 10.0.0.0/24, 10.0.1.10-40 or a single IP" value={input} onChange={(e) => setInput(e.target.value)} onKeyDown={(e) => { if (e.key === 'Enter') { e.preventDefault(); addTarget(input) } }} onBlur={() => input && addTarget(input)} style={{ marginTop: 6 }} />
                <label className="cap" style={{ display: 'flex', alignItems: 'center', gap: 6, marginTop: 8 }}><input className="cbx" type="checkbox" checked={allowLarge} onChange={(e) => setAllowLarge(e.target.checked)} />Allow large scans (lift the {settings?.discovery?.maxAddresses ?? 4096}-address cap)</label>
              </div>
            )}
            {canOperate && (
              <div>
                <div className="cap" style={{ fontWeight: 500, color: 'var(--text-2)', marginBottom: 6 }}>Credentials to try, in order</div>
                <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
                  {creds.filter((c) => c.type.startsWith('snmp')).length === 0 && <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}><span className="mono cap">1</span><Ic.lock style={{ color: 'var(--text-3)' }} /><span style={{ flex: 1 }}>Default community <span className="cap">· “public”</span></span><Tag>v2c</Tag></div>}
                  {creds.filter((c) => c.type.startsWith('snmp')).map((c, i) => <div key={c.id} style={{ display: 'flex', alignItems: 'center', gap: 8 }}><span className="mono cap">{i + 1}</span><Ic.lock style={{ color: 'var(--text-3)' }} /><span style={{ flex: 1 }}>{c.name} <span className="cap">· {c.scope || 'global'}</span></span>{c.type === 'snmpv3' ? <Tag>v3</Tag> : <Pill tone="warn" xs noIcon>v2c</Pill>}</div>)}
                  <Link to="/credentials" className="cap">Manage credentials →</Link>
                </div>
              </div>
            )}
            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 10 }}><label className="cap">Concurrency<input className="fin num" value={settings?.poll?.concurrency ?? ''} readOnly style={{ marginTop: 4 }} /></label><label className="cap">Scan cap<input className="fin num" value={settings?.discovery?.maxAddresses ?? ''} readOnly style={{ marginTop: 4 }} /></label></div>
            {msg && <div className="error small">{msg}</div>}
            {canOperate ? <Btn type="submit" variant="pri" disabled={run?.status === 'running'}><Ic.radar />{run?.status === 'running' ? 'Run in progress…' : 'Start discovery'}</Btn> : <Tag>Read-only · Viewer</Tag>}
            <div className="cap">Only scan networks you are authorised to manage. Discovery never changes a device.</div>
          </form>
        </Card>
        <div className="stack" style={{ flex: '3 1 560px' }}>
          <Card title="Progress" id="pr" right={run ? <span className="cap num">{run.finishedAt ? `finished ${ago(run.finishedAt)} · ${fmtTime(run.finishedAt)}` : `elapsed ${ago(run.startedAt || run.createdAt)}`}</span> : undefined}>
            {!run ? <div className="cb cap">Start a run to see progress here.</div> : (
              <>
                <div className="cb" style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
                  <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}><span style={{ display: 'inline-flex', alignItems: 'center', gap: 10 }}><JobPill status={run.status} /><span>{isImport ? 'Rows processed' : 'Candidates identified'}</span></span><span className="num" style={{ fontWeight: 600 }}>{run.done} <span className="cap" style={{ fontWeight: 400 }}>/ {run.total}</span></span></div>
                  <JobBar done={run.done} failed={run.failed} total={run.total} running={run.status === 'running'} />
                </div>
                {isImport ? (
                <div style={{ display: 'flex', flexWrap: 'wrap', borderTop: '1px solid var(--border)' }}>
                  <div className="stat"><span className="cap">Rows</span><span className="v">{run.total}</span></div>
                  <div className="stat"><span className="cap">Created</span><span className="v" style={{ color: 'var(--accent)' }}>{imp.created}</span></div>
                  <div className="stat"><span className="cap">Updated</span><span className="v">{imp.updated}</span></div>
                  <div className="stat"><span className="cap">Skipped</span><span className="v" style={{ color: 'var(--off)' }}>{imp.skipped}</span></div>
                  <div className="stat"><span className="cap">Failed</span><span className="v" style={{ color: imp.failed ? 'var(--crit)' : undefined }}>{imp.failed}</span></div>
                </div>
                ) : (
                <div style={{ display: 'flex', flexWrap: 'wrap', borderTop: '1px solid var(--border)' }}>
                  <div className="stat"><span className="cap">Responding addresses</span><span className="v">{run.total}</span></div>
                  <div className="stat"><span className="cap">Printers identified</span><span className="v">{found.length}</span></div>
                  <div className="stat"><span className="cap">New to PrintMarshal</span><span className="v" style={{ color: 'var(--accent)' }}>{created.length}</span></div>
                  <div className="stat"><span className="cap">Already managed</span><span className="v">{found.length - created.length}</span></div>
                  <div className="stat"><span className="cap">Not printers</span><span className="v" style={{ color: 'var(--off)' }}>{skipped.length}</span></div>
                </div>
                )}
                <div className="cb" style={{ borderTop: '1px solid var(--border)' }}><div className="log" role="log" aria-label={isImport ? 'Import log' : 'Discovery log'}>{results.length === 0 ? 'waiting for the first result…' : results.slice().reverse().map((r) => isImport
                  ? <div key={r.id}>{new Date(r.at).toLocaleTimeString()} row {rowOf(r) ?? '—'} · {r.status} · {r.message}</div>
                  : <div key={r.id}>{new Date(r.at).toLocaleTimeString()} {r.address || '—'} · {(r.data?.methods as string[] | undefined)?.join('+') || ''} · {r.message}</div>)}</div></div>
              </>
            )}
          </Card>
          <Card title="Results" id="rs" right={<span className="cap num">{isImport ? `${imp.created} created · ${imp.updated} updated · ${imp.skipped} skipped${imp.failed ? ` · ${imp.failed} failed` : ''}` : `${found.length} printers${skipped.length ? ` + ${skipped.length} not printers` : ''}${errs.length ? ` · ${errs.length} errors` : ''}`}</span>}>
            {results.length === 0 ? <div className="cb cap">Nothing yet.</div> : isImport ? (
              <div className="tw"><table className="tbl" style={{ minWidth: 560 }}>
                <thead><tr><th className="r">Row</th><th>Outcome</th><th>Message</th></tr></thead>
                <tbody>{results.slice().sort((a, b) => (rowOf(a) ?? 0) - (rowOf(b) ?? 0)).map((r) => {
                  const st = importStatus[r.status] || { tone: 'off' as const, label: r.status }
                  return (
                    <tr key={r.id} className={r.deviceId ? 'clickable' : r.status === 'skipped' ? 'dim' : undefined} onClick={() => r.deviceId && navigate(`/devices/${r.deviceId}`)}>
                      <td className="num r">{rowOf(r) ?? '—'}</td>
                      <td><Pill tone={st.tone} xs>{st.label}</Pill></td>
                      <td className="cap wrap">{r.message || '—'}{r.deviceId ? <> · <Link to={`/devices/${r.deviceId}`} onClick={(e) => e.stopPropagation()}>open device →</Link></> : null}</td>
                    </tr>
                  )
                })}</tbody>
              </table></div>
            ) : (
              <div className="tw"><table className="tbl" style={{ minWidth: 720 }}>
                <thead><tr><th>State</th><th>Address</th><th>Identified as</th><th>Found via</th><th>Detail</th></tr></thead>
                <tbody>{results.map((r) => (
                  <tr key={r.id} className={r.status === 'ok' ? 'clickable' : 'dim'} onClick={() => r.deviceId && navigate(`/devices/${r.deviceId}`)}>
                    <td>{r.status === 'ok' ? (r.data?.created ? <Pill tone="new">New</Pill> : <Pill tone="ok">Managed</Pill>) : r.status === 'skipped' ? <Pill tone="off">Not a printer</Pill> : <Pill tone="crit">Error</Pill>}</td>
                    <td className="mono">{r.address}</td>
                    <td className="mono">{(r.data?.model as string) || '—'}</td>
                    <td>{((r.data?.methods as string[]) || []).map((m) => <Tag key={m}>{m}</Tag>)}</td>
                    <td className="cap wrap">{r.status === 'ok' ? (r.deviceId ? <Link to={`/devices/${r.deviceId}`}>open device →</Link> : r.message) : (r.message || '').slice(0, 140)}</td>
                  </tr>
                ))}</tbody>
              </table></div>
            )}
          </Card>
          {runs.length > 1 && (
            <Card title="Previous runs" id="prev">
              <div className="tw"><table className="tbl"><thead><tr><th>Started</th><th>By</th><th>Methods</th><th>Status</th><th className="r">Found</th></tr></thead><tbody>
                {runs.slice(0, 10).map((j) => <tr key={j.id} className={`clickable${j.id === selected ? ' sel' : ''}`} onClick={() => setSelected(j.id)}><td className="num cap">{fmtTime(j.createdAt)}</td><td>{j.createdBy}</td><td>{j.type === 'import' ? <Tag>Import</Tag> : null}{j.target?.zeroconf ? <Tag>zeroconf</Tag> : null} {(j.target?.targets as string[] | undefined)?.length ? <Tag mono>{(j.target.targets as string[]).join(', ')}</Tag> : null}</td><td><JobPill status={j.status} xs /></td><td className="num r">{j.type === 'import' ? '—' : j.done - j.failed}</td></tr>)}
              </tbody></table></div>
            </Card>
          )}
        </div>
      </div>
      {importing && <ImportDialog user={user} onClose={() => setImporting(false)} onApplied={(j) => { setImporting(false); setSelected(j.id); load() }} />}
    </>
  )
}
