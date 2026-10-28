// Results of one security.assess (or security.remediate) job: live progress,
// compliance meter, per-item pass rates and the per-device breakdown.
import { useCallback, useEffect, useMemo, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { api, ago, fmtTime } from '../api'
import type { Device, Group, Job, User } from '../types'
import { deviceName } from '../types'
import { useEvents } from '../ws'
import { Ic } from '../components/icons'
import { Banner, Btn, Card, EmptyState, Pill, Seg, Tag } from '../components/ui'
import type { Assessment, ComplianceResponse, Policy, SecurityTarget } from '../types-security'
import { ComplianceRing, FailingChips, ItemStatusPill, JobProgress, MgmtPathTag, PassRateBar, ScoreBar, SevMark, fmtVal, operatorLabel, targetLabel, useJob } from '../components/security/common'
import RemediateFlow, { ResultsTable } from '../components/security/RemediateFlow'

export default function SecurityAssessment({ user }: { user: User }) {
  const { jobId } = useParams()
  const navigate = useNavigate()
  const { job, results, error } = useJob(jobId)
  const [policy, setPolicy] = useState<Policy | null>(null)
  const [devices, setDevices] = useState<Device[]>([])
  const [groups, setGroups] = useState<Group[]>([])
  const [assessments, setAssessments] = useState<Assessment[]>([])
  const [failOnly, setFailOnly] = useState(false)
  const [open, setOpen] = useState<Record<string, boolean>>({})
  const [remediate, setRemediate] = useState<SecurityTarget | null>(null)
  const [msg, setMsg] = useState('')
  const canOperate = user.role !== 'viewer'
  const policyId = (job?.params?.policyId || job?.target?.policyId) as string | undefined
  const target = useMemo<SecurityTarget>(() => ({ deviceIds: job?.target?.deviceIds as string[] | undefined, groupId: job?.target?.groupId as string | undefined }), [job?.target])

  useEffect(() => {
    api.get<{ devices: Device[] }>('/api/devices?includeDecommissioned=true').then((r) => setDevices(r.devices)).catch(() => {})
    api.get<Group[]>('/api/groups').then(setGroups).catch(() => {})
  }, [])
  useEffect(() => { if (policyId) api.get<Policy>(`/api/policies/${policyId}`).then(setPolicy).catch((e) => setMsg((e as Error).message)) }, [policyId])
  const loadAssessments = useCallback(() => {
    if (!policyId) return
    api.get<ComplianceResponse>(`/api/compliance?policy=${encodeURIComponent(policyId)}`).then((r) => setAssessments(r.results)).catch(() => {})
  }, [policyId])
  useEffect(() => { loadAssessments() }, [loadAssessments, job?.done, job?.status])
  useEvents((e) => { if (e.type === 'security.assessed') loadAssessments() }, [loadAssessments])

  const byId = useMemo(() => Object.fromEntries(devices.map((d) => [d.id, d])), [devices])
  const jobDeviceIds = useMemo(() => new Set(results.map((r) => r.deviceId).filter(Boolean) as string[]), [results])
  // Latest assessment per device for the job's devices (the job's own result when it is the latest).
  const rows = useMemo(() => assessments.filter((a) => jobDeviceIds.has(a.deviceId)).sort((a, b) => a.score - b.score || b.failed - a.failed), [assessments, jobDeviceIds])
  const fromThisJob = rows.filter((a) => a.jobId === jobId).length
  const sevOf = (itemId: string) => policy?.items.find((i) => i.id === itemId)?.severity || 'medium'
  const perItem = useMemo(() => (policy?.items || []).map((it) => {
    let pass = 0, fail = 0, unknown = 0
    for (const a of rows) { const r = a.items.find((x) => x.itemId === it.id); if (!r) continue; if (r.status === 'pass') pass++; else if (r.status === 'fail') fail++; else unknown++ }
    return { it, pass, fail, unknown }
  }), [policy, rows])
  const compliant = rows.filter((a) => a.compliant).length
  const errors = results.filter((r) => r.status === 'error')
  const pct = rows.length ? (compliant / rows.length) * 100 : null
  const failingDeviceIds = rows.filter((a) => !a.compliant).map((a) => a.deviceId)

  const reassess = async () => {
    if (!policyId) return
    try { const j = await api.post<Job>(`/api/policies/${policyId}/assess`, target); navigate(`/security/assessments/${j.id}`) } catch (e) { setMsg((e as Error).message) }
  }

  if (error) return <Banner tone="crit">{error}</Banner>
  if (!job) return <div className="cap" aria-busy="true">Loading job…</div>
  const isRemediation = job.type === 'security.remediate'
  const title = `${policy?.name || 'Policy'} · ${isRemediation ? (job.params?.dryRun === false ? 'remediation results' : 'remediation dry run') : 'assessment results'}`

  return (
    <>
      <nav aria-label="Breadcrumb" className="cap"><Link to="/security">Security</Link> / <Link to={`/security?tab=compliance${policyId ? `&policy=${policyId}` : ''}`}>Compliance</Link> / <span style={{ color: 'var(--text-2)' }}>{title}</span></nav>
      <div className="ph">
        <div>
          <h1 className="h1">{title}</h1>
          <div className="cap num">{policy ? `${policy.items.length} items` : ''} · target {targetLabel(target, groups, devices)} · {job.finishedAt ? `completed ${ago(job.finishedAt)} (${fmtTime(job.finishedAt)})` : job.status} · {job.done}/{job.total} devices{errors.length ? ` · ${errors.length} unreachable / errored` : ''} · job <span className="mono">{job.id.slice(0, 8)}</span></div>
        </div>
        <div className="row">
          <Link className="btn sm ghost" to={`/jobs/${job.id}`}>Job log</Link>
          {canOperate && !isRemediation && <Btn sm onClick={reassess}><Ic.refresh />Re-assess</Btn>}
          {canOperate && policy && failingDeviceIds.length > 0 && <Btn sm variant="wr" onClick={() => setRemediate({ deviceIds: failingDeviceIds })}><Ic.upload />Remediate failing…</Btn>}
        </div>
      </div>
      {msg && <Banner tone="crit">{msg}</Banner>}
      {(job.status === 'queued' || job.status === 'running') && <Card><div className="cb"><JobProgress job={job}>{isRemediation ? 'Remediating' : 'Assessing'} · results fill in live</JobProgress></div></Card>}

      {isRemediation ? (
        <>
          {job.params?.dryRun !== false && <Banner tone="info"><b style={{ color: 'var(--info)' }}>Dry run</b> — nothing was written. Items marked “would write” are what a real run would change.</Banner>}
          <Card title="Per-device remediation" id="rr" sub="each item: written, would write, unchanged, skipped or failed · secrets are never carried here">
            <div className="cb"><ResultsTable results={results} byDevice={(id) => (id ? byId[id] : undefined)} mgmtPathOf={(id) => policy?.items.find((i) => i.id === id)?.managementPath} /></div>
          </Card>
        </>
      ) : (
        <>
          <section className="card" style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'stretch' }} aria-label="Assessment summary">
            <div style={{ display: 'flex', alignItems: 'center', gap: 16, padding: '16px 20px', flex: '1 1 280px' }}>
              <ComplianceRing pct={pct} caption="compliant" label={pct === null ? 'No results yet' : `${pct.toFixed(1)}% of assessed devices compliant`} />
              <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
                <div className="cap">Average score</div>
                <div className="num" style={{ fontSize: 22, fontWeight: 600 }}>{rows.length ? (rows.reduce((s, a) => s + a.score, 0) / rows.length).toFixed(1) : '—'}</div>
                <div className="cap">{fromThisJob < rows.length ? `${rows.length - fromThisJob} device${rows.length - fromThisJob === 1 ? ' has' : 's have'} a newer assessment` : 'severity-weighted · unknown items excluded'}</div>
              </div>
            </div>
            <div className="stat" style={{ flex: '1 1 150px' }}><span className="cap">Compliant devices</span><span className="v" style={{ color: 'var(--ok)' }}>{compliant}</span><span className="cap">all evaluated items pass</span></div>
            <div className="stat" style={{ flex: '1 1 150px' }}><span className="cap">Non-compliant</span><span className="v" style={{ color: rows.length - compliant ? 'var(--crit)' : undefined }}>{rows.length - compliant}</span><span className="cap">{rows.filter((a) => a.items.some((i) => i.status === 'fail' && ['critical', 'high'].includes(sevOf(i.itemId)))).length} with critical / high fails</span></div>
            <div className="stat" style={{ flex: '1 1 150px' }}><span className="cap">Failing items</span><span className="v">{perItem.filter((p) => p.fail > 0).length} <span style={{ fontSize: 14, color: 'var(--text-3)', fontWeight: 500 }}>/ {policy?.items.length ?? 0}</span></span><span className="cap">{perItem.reduce((n, p) => n + p.fail, 0)} item-failures total</span></div>
            <div className="stat" style={{ flex: '1 1 150px' }}><span className="cap">Not assessed</span><span className="v" style={{ color: 'var(--off)' }}>{errors.length}</span><span className="cap">unreachable or errored</span></div>
          </section>
          {errors.length > 0 && <Banner tone="warn"><b style={{ color: 'var(--warn)' }}>{errors.length} device{errors.length === 1 ? '' : 's'} could not be assessed:</b> {errors.map((e) => `${byId[e.deviceId || '']?.displayName || e.address || e.deviceId?.slice(0, 8)} — ${e.message}`).join('; ')}</Banner>}

          <Card title="By policy item" id="bi" right={<Seg sm label="Show items" value={failOnly ? 'fail' : 'all'} onChange={(v) => setFailOnly(v === 'fail')} options={[{ v: 'all', label: `All ${perItem.length}` }, { v: 'fail', label: 'Failing only' }]} />}>
            <div className="tw"><table className="tbl" style={{ minWidth: 860 }}>
              <thead><tr><th>Policy item</th><th>Severity</th><th style={{ width: '30%' }}>Pass rate</th><th className="r">Failing</th><th>Remediation</th></tr></thead>
              <tbody>{perItem.filter((p) => !failOnly || p.fail > 0).map(({ it, pass, fail, unknown }) => (
                <tr key={it.id}>
                  <td style={{ fontWeight: 500 }}><span style={{ display: 'inline-flex', alignItems: 'center', gap: 6 }}>{it.name}<MgmtPathTag path={it.managementPath} xs /></span><div className="cap mono" style={{ fontWeight: 400, fontSize: 11 }}>{it.key} {operatorLabel[it.operator]} {fmtVal(it.value)}</div></td>
                  <td><SevMark sev={it.severity} /></td>
                  <td><PassRateBar pass={pass} fail={fail} unknown={unknown} /></td>
                  <td className="num r" style={{ fontWeight: 600 }}>{fail > 0 ? <span style={{ color: 'var(--crit)' }}>{fail}</span> : <span style={{ color: 'var(--ok)', display: 'inline-flex', alignItems: 'center', gap: 4 }}><Ic.check size={14} />0</span>}{unknown > 0 && <div className="cap" style={{ fontWeight: 400 }}>{unknown} unknown</div>}</td>
                  <td className="cap">{it.remediate ? `Auto · writes ${it.remediation?.key || it.key}` : it.description || 'Assess-only'}</td>
                </tr>
              ))}</tbody>
            </table></div>
          </Card>

          <Card title="Per-device results" id="pd" sub={`${rows.length} devices · worst first · expand a row for every item`}>
            {rows.length === 0 ? <div className="cb"><EmptyState icon={<Ic.shield size={20} />} title="No results yet" text="Per-device results appear as the job completes each device." tone="muted" /></div> : (
              <div className="tw"><table className="tbl" style={{ minWidth: 900 }}>
                <thead><tr><th style={{ width: 28 }} /><th>Device</th><th>Status</th><th style={{ width: 200 }}>Compliance</th><th>Failing items</th><th>Assessed</th></tr></thead>
                <tbody>{rows.map((a) => {
                  const d = byId[a.deviceId]
                  const isOpen = !!open[a.deviceId]
                  const failing = a.items.filter((i) => i.status === 'fail').map((i) => ({ itemId: i.itemId, name: i.name, severity: sevOf(i.itemId) }))
                  return [
                    <tr key={a.id} className="clickable" onClick={() => setOpen((o) => ({ ...o, [a.deviceId]: !o[a.deviceId] }))} aria-expanded={isOpen}>
                      <td>{isOpen ? <Ic.chevronDown size={14} /> : <Ic.chevronRight size={14} />}</td>
                      <td><Link to={`/devices/${a.deviceId}`} style={{ fontWeight: 500 }} onClick={(e) => e.stopPropagation()}>{d ? deviceName(d) : a.deviceId.slice(0, 8)}</Link><div className="cap mono" style={{ fontSize: 11 }}>{d ? `${d.model || ''} · ${d.address}` : ''}</div></td>
                      <td>{a.compliant ? <Pill tone="ok">Compliant</Pill> : <Pill tone={a.score >= 80 ? 'warn' : 'crit'}>Non-compliant</Pill>}{a.jobId !== jobId && <div className="cap">newer run</div>}</td>
                      <td><ScoreBar a={a} /></td>
                      <td><FailingChips items={failing} /></td>
                      <td className="cap num" title={fmtTime(a.at)}>{ago(a.at)}</td>
                    </tr>,
                    isOpen && (
                      <tr key={`${a.id}-items`}><td /><td colSpan={5} style={{ padding: 0, whiteSpace: 'normal' }}>
                        <table className="tbl compact" style={{ background: 'var(--surface-2)' }}>
                          <thead><tr><th>Item</th><th>Result</th><th>Severity</th><th>Current</th><th>Desired</th><th>Note</th></tr></thead>
                          <tbody>{[...a.items].sort((x, y) => (x.status === 'fail' ? 0 : x.status === 'unknown' ? 1 : 2) - (y.status === 'fail' ? 0 : y.status === 'unknown' ? 1 : 2)).map((it) => (
                            <tr key={it.itemId} className={it.status === 'pass' ? 'dim' : ''}>
                              <td style={{ fontWeight: it.status === 'fail' ? 500 : 400 }}>{it.name}<div className="cap mono" style={{ fontSize: 11 }}>{it.key}</div></td>
                              <td><ItemStatusPill status={it.status} xs /></td>
                              <td><SevMark sev={sevOf(it.itemId)} /></td>
                              <td className="mono" style={{ color: it.status === 'fail' ? 'var(--crit)' : undefined }}>{fmtVal(it.current)}</td>
                              <td className="mono">{policy ? `${operatorLabel[policy.items.find((p) => p.id === it.itemId)?.operator || 'eq']} ` : ''}{fmtVal(it.desired)}</td>
                              <td className="wrap cap">{it.message || ''}</td>
                            </tr>
                          ))}</tbody>
                        </table>
                      </td></tr>
                    ),
                  ]
                })}</tbody>
              </table></div>
            )}
            <div className="cf cap" style={{ display: 'flex', gap: 14, flexWrap: 'wrap' }}><span><Tag>unknown</Tag> = setting not readable on this device; counted but never scored</span><span>Secrets are reported as <span className="mono">&lt;set&gt;</span> / <span className="mono">&lt;unset&gt;</span> only</span></div>
          </Card>
        </>
      )}
      {remediate && policy && <RemediateFlow policy={policy} target={remediate} devices={devices} groups={groups} onClose={() => { setRemediate(null); loadAssessments() }} />}
    </>
  )
}
