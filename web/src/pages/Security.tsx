// Security (spec §8.5): compliance, policies, certificates, credentials & rotation.
import { useCallback, useEffect, useMemo, useState } from 'react'
import { Link, useNavigate, useSearchParams } from 'react-router-dom'
import { api, ago, fmtTime } from '../api'
import type { Credential, Device, Group, Job, JobResult, User } from '../types'
import { deviceName } from '../types'
import { useEvents } from '../ws'
import { Ic } from '../components/icons'
import { Banner, Btn, Card, ChipSelect, Dialog, EmptyState, Kpi, Pill, Seg, Tabs, Tag } from '../components/ui'
import type { CAProfile, ComplianceResponse, CompliancePoint, DeviceCertificates, Policy, RotateKind, RotationData, SecurityCatalog, SecurityTarget } from '../types-security'
import { ComplianceRing, ConfirmWriteDialog, ExpiryPill, ExpiryTimeline, FailingChips, HistoryChart, JobProgress, MgmtPathTag, PassRateBar, ScoreBar, SecretRow, SevMark, TargetPicker, expiryTone, operatorLabel, fmtVal, targetLabel, useJob } from '../components/security/common'
import PolicyEditor from '../components/security/PolicyEditor'
import RemediateFlow from '../components/security/RemediateFlow'
import EnrollDrawer from '../components/security/EnrollDrawer'
import { certRows, type CertRow } from '../components/security/DeviceSecurityTab'

type Tab = 'compliance' | 'policies' | 'certificates' | 'credentials'

export default function Security({ user }: { user: User }) {
  const [params, setParams] = useSearchParams()
  const tab = (params.get('tab') as Tab) || 'compliance'
  const setTab = (t: Tab) => { const p = new URLSearchParams(params); p.set('tab', t); setParams(p, { replace: true }) }
  const [devices, setDevices] = useState<Device[]>([])
  const [groups, setGroups] = useState<Group[]>([])
  const [policies, setPolicies] = useState<Policy[]>([])
  const [catalog, setCatalog] = useState<SecurityCatalog | null>(null)
  const [error, setError] = useState('')
  const canOperate = user.role !== 'viewer'
  const loadPolicies = useCallback(() => api.get<Policy[]>('/api/policies').then(setPolicies).catch((e) => setError((e as Error).message)), [])
  useEffect(() => {
    api.get<{ devices: Device[] }>('/api/devices').then((r) => setDevices(r.devices)).catch(() => {})
    api.get<Group[]>('/api/groups').then(setGroups).catch(() => {})
    api.get<SecurityCatalog>('/api/security/catalog').then(setCatalog).catch(() => {})
    loadPolicies()
  }, [loadPolicies])
  useEvents((e) => { if (e.type.startsWith('device.')) api.get<{ devices: Device[] }>('/api/devices').then((r) => setDevices(r.devices)).catch(() => {}) }, [])

  return (
    <>
      <div className="ph">
        <div><h1 className="h1">Security</h1><div className="cap num">{policies.length} polic{policies.length === 1 ? 'y' : 'ies'} · {devices.length} devices · every write runs as a dry run first</div></div>
        {!canOperate && <Tag>Read-only · Viewer</Tag>}
      </div>
      {error && <Banner tone="crit">{error}</Banner>}
      <Tabs label="Security sections" value={tab} onChange={setTab} tabs={[
        { v: 'compliance', label: 'Compliance' },
        { v: 'policies', label: <>Policies <span className="cnt" style={{ background: 'var(--surface-2)', color: 'var(--text-2)' }}>{policies.length}</span></> },
        { v: 'certificates', label: 'Certificates' },
        { v: 'credentials', label: 'Credentials & rotation' },
      ]} />
      {tab === 'compliance' && <ComplianceTab devices={devices} groups={groups} policies={policies} canOperate={canOperate} />}
      {tab === 'policies' && <PoliciesTab policies={policies} catalog={catalog} canOperate={canOperate} onChanged={loadPolicies} />}
      {tab === 'certificates' && <CertificatesTab devices={devices} user={user} />}
      {tab === 'credentials' && <CredentialsTab devices={devices} groups={groups} canOperate={canOperate} />}
    </>
  )
}

// --- Compliance --------------------------------------------------------------------------

function ComplianceTab({ devices, groups, policies, canOperate }: { devices: Device[]; groups: Group[]; policies: Policy[]; canOperate: boolean }) {
  const [params, setParams] = useSearchParams()
  const navigate = useNavigate()
  const policyId = params.get('policy') || policies.find((p) => p.name === 'Baseline')?.id || policies[0]?.id || ''
  const groupId = params.get('group') || ''
  const set = (k: string, v: string) => { const p = new URLSearchParams(params); if (v) p.set(k, v); else p.delete(k); setParams(p, { replace: true }) }
  const [data, setData] = useState<ComplianceResponse | null>(null)
  const [history, setHistory] = useState<CompliancePoint[]>([])
  const [failOnly, setFailOnly] = useState(false)
  const [assess, setAssess] = useState<SecurityTarget | null>(null)
  const [remediate, setRemediate] = useState<SecurityTarget | null>(null)
  const [pick, setPick] = useState<'assess' | 'remediate' | null>(null)
  const [msg, setMsg] = useState('')
  const policy = policies.find((p) => p.id === policyId)
  const byId = useMemo(() => Object.fromEntries(devices.map((d) => [d.id, d])), [devices])

  const load = useCallback(() => {
    if (!policyId) return
    api.get<ComplianceResponse>(`/api/compliance?policy=${encodeURIComponent(policyId)}${groupId ? `&group=${encodeURIComponent(groupId)}` : ''}`).then(setData).catch((e) => setMsg((e as Error).message))
    api.get<{ points: CompliancePoint[] }>(`/api/compliance/history?policy=${encodeURIComponent(policyId)}&from=${new Date(Date.now() - 30 * 864e5).toISOString()}`).then((r) => setHistory(r.points || [])).catch(() => {})
  }, [policyId, groupId])
  useEffect(() => { load() }, [load])
  useEvents((e) => { if (e.type === 'security.assessed' || e.type === 'security.remediated') load() }, [load])

  const results = data?.results || []
  const summary = data?.summary
  const perItem = useMemo(() => (policy?.items || []).map((it) => {
    let pass = 0, fail = 0, unknown = 0
    for (const a of results) { const r = a.items.find((x) => x.itemId === it.id); if (!r) continue; if (r.status === 'pass') pass++; else if (r.status === 'fail') fail++; else unknown++ }
    return { it, pass, fail, unknown }
  }), [policy, results])
  const failingItems = perItem.filter((p) => p.fail > 0).length
  const sorted = useMemo(() => [...results].sort((a, b) => a.score - b.score || b.failed - a.failed), [results])
  const assessedIds = new Set(results.map((r) => r.deviceId))
  const scopeGroup = groups.find((g) => g.id === groupId)
  const notAssessed: number | null = scopeGroup?.kind === 'rule' ? null : devices.filter((d) => d.state !== 'decommissioned' && !assessedIds.has(d.id) && (!scopeGroup || scopeGroup.memberIds.includes(d.id))).length
  const compliantPct = summary && summary.devices ? (summary.compliant / summary.devices) * 100 : null
  const sevOf = (itemId: string) => policy?.items.find((i) => i.id === itemId)?.severity || 'medium'

  const defaultTarget = (): SecurityTarget => groupId ? { groupId } : { deviceIds: devices.filter((d) => d.state !== 'decommissioned').map((d) => d.id) }
  const startAssess = async (t: SecurityTarget) => {
    try { const j = await api.post<Job>(`/api/policies/${policyId}/assess`, t); setAssess(null); navigate(`/security/assessments/${j.id}`) } catch (e) { setMsg((e as Error).message) }
  }

  if (policies.length === 0) return <Card><div className="cb"><EmptyState icon={<Ic.shield size={20} />} title="No policy yet" text="The built-in Baseline is created on first run; create one under Policies to assess the fleet." /></div></Card>

  return (
    <>
      <div className="row">
        <label className="chip set"><Ic.shield size={13} />Policy<select aria-label="Policy" value={policyId} onChange={(e) => set('policy', e.target.value)}>{policies.map((p) => <option key={p.id} value={p.id}>{p.name}</option>)}</select></label>
        <ChipSelect label="Group" value={groupId} onChange={(v) => set('group', v)} options={groups.map((g) => ({ v: g.id, label: g.name }))} />
        <span className="cap">{policy ? `${policy.items.length} items · ${policy.items.filter((i) => i.remediate).length} remediable` : ''}</span>
        {canOperate && (
          <div className="row" style={{ marginLeft: 'auto' }}>
            <Btn sm onClick={() => { setAssess(defaultTarget()); setPick('assess') }}><Ic.refresh />Assess…</Btn>
            <Btn sm variant="wr" disabled={!summary || summary.nonCompliant === 0} onClick={() => { setRemediate({ deviceIds: results.filter((r) => !r.compliant).map((r) => r.deviceId) }); setPick('remediate') }}><Ic.upload />Remediate failing…</Btn>
          </div>
        )}
      </div>
      {msg && <Banner tone="crit" right={<Btn sm variant="ghost" ic onClick={() => setMsg('')}><Ic.x /></Btn>}>{msg}</Banner>}

      <section className="card" style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'stretch' }} aria-label="Fleet compliance summary">
        <div style={{ display: 'flex', alignItems: 'center', gap: 16, padding: '16px 20px', flex: '1 1 280px' }}>
          <ComplianceRing pct={compliantPct} caption="compliant" label={compliantPct === null ? 'No devices assessed yet' : `${compliantPct.toFixed(1)}% of assessed devices compliant`} />
          <div style={{ display: 'flex', flexDirection: 'column', gap: 6, minWidth: 0 }}>
            <div className="cap">Average score</div>
            <div className="num" style={{ fontSize: 22, fontWeight: 600 }}>{summary && summary.devices ? summary.avgScore.toFixed(1) : '—'}</div>
            <div className="cap">{results.length ? `latest assessment ${ago(sorted.reduce((m, a) => (a.at > m ? a.at : m), sorted[0].at))}` : 'no assessments for this policy yet'}</div>
          </div>
        </div>
        <div className="stat" style={{ flex: '1 1 150px' }}><span className="cap">Compliant devices</span><span className="v" style={{ color: 'var(--ok)' }}>{summary?.compliant ?? '—'}</span><span className="cap">all evaluated items pass</span></div>
        <div className="stat" style={{ flex: '1 1 150px' }}><span className="cap">Non-compliant</span><span className="v" style={{ color: summary?.nonCompliant ? 'var(--crit)' : undefined }}>{summary?.nonCompliant ?? '—'}</span><span className="cap">{results.filter((r) => r.items.some((i) => i.status === 'fail' && ['critical', 'high'].includes(sevOf(i.itemId)))).length} with critical / high fails</span></div>
        <div className="stat" style={{ flex: '1 1 150px' }}><span className="cap">Failing items</span><span className="v">{failingItems} <span style={{ fontSize: 14, color: 'var(--text-3)', fontWeight: 500 }}>/ {policy?.items.length ?? 0}</span></span><span className="cap">{perItem.reduce((n, p) => n + p.fail, 0)} item-failures total</span></div>
        <div className="stat" style={{ flex: '1 1 150px' }}><span className="cap">Not assessed</span><span className="v" style={{ color: 'var(--off)' }}>{summary ? summary.notAssessed : (notAssessed ?? '—')}</span><span className="cap">no result for this policy</span></div>
        <div className="stat" style={{ flex: '1 1 150px' }}><span className="cap">Unknown items</span><span className="v" style={{ color: 'var(--text-3)' }}>{summary?.unknown ?? '—'}</span><span className="cap">not readable; never counted as pass</span></div>
      </section>

      <Banner tone="warn" role="note">
        <b style={{ color: 'var(--warn)' }}>Remediation can change how devices are reached.</b> Disabling SNMPv1/v2c breaks tools that still use community strings, and enforcing HTTPS-only closes plain-HTTP admin pages. Every remediation runs as dry run → confirm → per-item results, and the engine never removes a device's last management path.
      </Banner>

      <div className="cols">
        <Card title="By policy item" id="bi" style={{ flex: '2 1 560px' }} right={<Seg sm label="Show items" value={failOnly ? 'fail' : 'all'} onChange={(v) => setFailOnly(v === 'fail')} options={[{ v: 'all', label: `All ${perItem.length}` }, { v: 'fail', label: 'Failing only' }]} />}>
          <div className="tw"><table className="tbl" style={{ minWidth: 720 }}>
            <thead><tr><th>Policy item</th><th>Severity</th><th style={{ width: '30%' }}>Pass rate</th><th className="r">Failing</th><th>Remediation</th></tr></thead>
            <tbody>
              {perItem.filter((p) => !failOnly || p.fail > 0).map(({ it, pass, fail, unknown }) => (
                <tr key={it.id}>
                  <td style={{ fontWeight: 500 }}><span style={{ display: 'inline-flex', alignItems: 'center', gap: 6 }}>{it.name}<MgmtPathTag path={it.managementPath} xs /></span><div className="cap mono" style={{ fontWeight: 400, fontSize: 11 }}>{it.key} {operatorLabel[it.operator]} {fmtVal(it.value)}</div></td>
                  <td><SevMark sev={it.severity} /></td>
                  <td><PassRateBar pass={pass} fail={fail} unknown={unknown} /></td>
                  <td className="num r" style={{ fontWeight: 600 }}>{fail > 0 ? <span style={{ color: 'var(--crit)' }}>{fail}</span> : <span style={{ color: 'var(--ok)', display: 'inline-flex', alignItems: 'center', gap: 4 }}><Ic.check size={14} />0</span>}{unknown > 0 && <div className="cap" style={{ fontWeight: 400 }}>{unknown} unknown</div>}</td>
                  <td className="cap">{it.remediate ? `Auto · writes ${it.remediation?.key || it.key}` : 'Assess-only'}</td>
                </tr>
              ))}
              {perItem.length === 0 && <tr><td colSpan={5} className="cap">This policy has no items.</td></tr>}
            </tbody>
          </table></div>
        </Card>
        <Card title="Trend · last 30 days" id="tr" sub="per-day average score and share of compliant devices" style={{ flex: '1 1 340px' }}>
          <div className="cb" style={{ display: 'flex', flexDirection: 'column', gap: 10 }}><HistoryChart points={history} /></div>
        </Card>
      </div>

      <Card title="Devices" id="bd" sub={`${results.length} assessed · worst first`}>
        {results.length === 0 ? <div className="cb"><EmptyState icon={<Ic.shield size={20} />} title="Nothing assessed yet" text={`Run “${policy?.name}” against a group or a set of devices.`} action={canOperate ? <Btn sm variant="pri" onClick={() => { setAssess(defaultTarget()); setPick('assess') }}>Assess…</Btn> : undefined} /></div> : (
          <div className="tw"><table className="tbl" style={{ minWidth: 900 }}>
            <thead><tr><th>Device</th><th>Status</th><th style={{ width: 200 }}>Compliance</th><th>Failing items</th><th>Assessed</th>{canOperate && <th className="r">Actions</th>}</tr></thead>
            <tbody>{sorted.map((a) => {
              const d = byId[a.deviceId]
              const failing = a.items.filter((i) => i.status === 'fail').map((i) => ({ itemId: i.itemId, name: i.name, severity: sevOf(i.itemId) }))
              return (
                <tr key={a.id}>
                  <td><Link to={`/devices/${a.deviceId}`} style={{ fontWeight: 500 }}>{d ? deviceName(d) : a.deviceId.slice(0, 8)}</Link><div className="cap mono" style={{ fontSize: 11 }}>{d ? `${d.model || ''} · ${d.address}` : ''}</div></td>
                  <td>{a.compliant ? <Pill tone="ok">Compliant</Pill> : <Pill tone={a.score >= 80 ? 'warn' : 'crit'}>Non-compliant</Pill>}</td>
                  <td><ScoreBar a={a} /></td>
                  <td><FailingChips items={failing} /></td>
                  <td className="cap num" title={fmtTime(a.at)}>{ago(a.at)}{a.jobId && <> · <Link to={`/security/assessments/${a.jobId}`}>results</Link></>}</td>
                  {canOperate && <td className="r" style={{ whiteSpace: 'nowrap' }}><Btn sm onClick={() => startAssess({ deviceIds: [a.deviceId] })}>Assess</Btn> {!a.compliant && <Btn sm variant="wr" onClick={() => { setRemediate({ deviceIds: [a.deviceId] }); setPick(null) }}>Remediate…</Btn>}</td>}
                </tr>
              )
            })}</tbody>
          </table></div>
        )}
      </Card>

      {pick === 'assess' && assess && (
        <Dialog title={`Assess with “${policy?.name}”`} sub="Read-only: gathers each device's security document (drivers, TLS and port probes) and scores it." onClose={() => setPick(null)}
          icon={<span style={{ width: 32, height: 32, borderRadius: 8, background: 'var(--accent-weak)', color: 'var(--accent)', display: 'grid', placeItems: 'center', flex: 'none' }}><Ic.shieldCheck /></span>}
          footer={<><Btn onClick={() => setPick(null)}>Cancel</Btn><Btn variant="pri" disabled={!assess.groupId && !(assess.deviceIds?.length)} onClick={() => startAssess(assess)}>Start assessment</Btn></>}>
          <TargetPicker value={assess} onChange={setAssess} devices={devices} groups={groups} />
        </Dialog>
      )}
      {pick === 'remediate' && remediate && (
        <Dialog title={`Remediate “${policy?.name}”`} sub="Choose the target; a dry run comes first and nothing is written until you confirm." onClose={() => setPick(null)}
          icon={<span style={{ width: 32, height: 32, borderRadius: 8, background: 'var(--warn-bg)', color: 'var(--warn)', display: 'grid', placeItems: 'center', flex: 'none' }}><Ic.upload /></span>}
          footer={<><Btn onClick={() => setPick(null)}>Cancel</Btn><Btn variant="wr" disabled={!remediate.groupId && !(remediate.deviceIds?.length)} onClick={() => setPick(null)}><Ic.upload />Run dry run</Btn></>}>
          <TargetPicker value={remediate} onChange={setRemediate} devices={devices} groups={groups} hint="Pre-selected: devices that are non-compliant in the current view." />
        </Dialog>
      )}
      {pick === null && remediate && policy && <RemediateFlow policy={policy} target={remediate} devices={devices} groups={groups} onClose={() => { setRemediate(null); load() }} />}
    </>
  )
}

// --- Policies ------------------------------------------------------------------------------

function PoliciesTab({ policies, catalog, canOperate, onChanged }: { policies: Policy[]; catalog: SecurityCatalog | null; canOperate: boolean; onChanged: () => void }) {
  const [editing, setEditing] = useState<Policy | null | 'new'>(null)
  const sevCount = (p: Policy) => ['critical', 'high', 'medium', 'low'].map((s) => ({ s, n: p.items.filter((i) => i.severity === s).length })).filter((x) => x.n > 0)
  return (
    <>
      <div className="row" style={{ justifyContent: 'space-between' }}>
        <span className="cap">A policy is a named list of desired settings. Items marked assess-only are never written; management-path items are skipped when they would cut the last channel.</span>
        {canOperate && <Btn sm variant="pri" disabled={!catalog} onClick={() => setEditing('new')}><Ic.plus />New policy</Btn>}
      </div>
      <Card aria-label="Policies">
        {policies.length === 0 ? <div className="cb"><EmptyState icon={<Ic.shield size={20} />} title="No policies" text="Create one from the security catalogue." action={canOperate ? <Btn sm variant="pri" onClick={() => setEditing('new')}>New policy</Btn> : undefined} /></div> : (
          <div className="tw"><table className="tbl" style={{ minWidth: 760 }}>
            <thead><tr><th>Policy</th><th>Items</th><th>Severity mix</th><th>Remediable</th><th>Management paths</th><th>Updated</th><th /></tr></thead>
            <tbody>{policies.map((p) => (
              <tr key={p.id} className="clickable" onClick={() => setEditing(p)}>
                <td><b style={{ fontWeight: 500 }}>{p.name}</b>{p.createdBy === 'system' && <> <Tag>built-in</Tag></>}<div className="cap" style={{ whiteSpace: 'normal', maxWidth: 420 }}>{p.description}</div></td>
                <td className="num">{p.items.length}</td>
                <td><span style={{ display: 'inline-flex', gap: 10 }}>{sevCount(p).map(({ s, n }) => <span key={s} style={{ display: 'inline-flex', alignItems: 'center', gap: 4 }}><SevMark sev={s} noLabel /><span className="num cap">{n}</span></span>)}</span></td>
                <td className="num">{p.items.filter((i) => i.remediate).length} <span className="cap">/ {p.items.length}</span></td>
                <td>{Array.from(new Set(p.items.map((i) => i.managementPath).filter(Boolean))).map((m) => <Tag key={m}>{m!.toUpperCase()}</Tag>)}</td>
                <td className="cap num">{ago(p.updatedAt)}<div>by {p.createdBy}</div></td>
                <td className="r"><Btn sm variant="ghost" onClick={(e) => { e.stopPropagation(); setEditing(p) }}>{canOperate ? <><Ic.pencil />Edit</> : 'View'}</Btn></td>
              </tr>
            ))}</tbody>
          </table></div>
        )}
      </Card>
      {editing && catalog && <PolicyEditor policy={editing === 'new' ? null : editing} catalog={catalog} canEdit={canOperate} onClose={() => setEditing(null)} onSaved={() => { setEditing(null); onChanged() }} onDeleted={() => { setEditing(null); onChanged() }} />}
    </>
  )
}

// --- Certificates ----------------------------------------------------------------------------

interface FleetCert extends CertRow { device: Device }

function CertificatesTab({ devices, user }: { devices: Device[]; user: User }) {
  const [certs, setCerts] = useState<Record<string, DeviceCertificates>>({})
  const [loading, setLoading] = useState(false)
  const [filter, setFilter] = useState<'all' | 'expiring' | 'expired' | 'self'>('all')
  const [profiles, setProfiles] = useState<CAProfile[]>([])
  const [enroll, setEnroll] = useState<Device | null>(null)
  const [msg, setMsg] = useState('')
  const canOperate = user.role !== 'viewer'
  const active = useMemo(() => devices.filter((d) => d.state !== 'decommissioned'), [devices])
  const loadProfiles = useCallback(() => api.get<CAProfile[]>('/api/ca-profiles').then(setProfiles).catch(() => {}), [])
  const load = useCallback(async () => {
    setLoading(true)
    const out: Record<string, DeviceCertificates> = {}
    for (let i = 0; i < active.length; i += 8) {
      await Promise.all(active.slice(i, i + 8).map((d) => api.get<DeviceCertificates>(`/api/devices/${d.id}/certificates`).then((c) => { out[d.id] = c }).catch(() => {})))
    }
    setCerts(out); setLoading(false)
  }, [active])
  useEffect(() => { load(); loadProfiles() }, [load, loadProfiles])
  useEvents((e) => { if (e.type.startsWith('security.certificate.') || e.type === 'security.assessed') load() }, [load])

  const rows: FleetCert[] = useMemo(() => active.flatMap((d) => certRows(certs[d.id]).map((r) => ({ ...r, device: d }))).sort((a, b) => new Date(a.notAfter).getTime() - new Date(b.notAfter).getTime()), [active, certs])
  const counts = { expired: rows.filter((r) => expiryTone(r.notAfter) === 'expired').length, d7: rows.filter((r) => expiryTone(r.notAfter) === 'critical').length, d30: rows.filter((r) => expiryTone(r.notAfter) === 'warning').length, self: rows.filter((r) => r.selfSigned).length }
  const shown = rows.filter((r) => filter === 'all' || (filter === 'expired' ? expiryTone(r.notAfter) === 'expired' : filter === 'expiring' ? ['critical', 'warning'].includes(expiryTone(r.notAfter)) : r.selfSigned))
  const withoutCert = active.filter((d) => certs[d.id] && certRows(certs[d.id]).length === 0).length
  const install = async (r: FleetCert) => { try { await api.post(`/api/devices/${r.device.id}/certificates/${r.issuedId}/install`, {}); setMsg(`Install job queued for ${deviceName(r.device)}.`) } catch (e) { setMsg((e as Error).message) } }

  return (
    <>
      {msg && <Banner tone="info" right={<Btn sm variant="ghost" ic onClick={() => setMsg('')}><Ic.x /></Btn>}>{msg}</Banner>}
      <div className="grid-auto">
        <Kpi label="Certificates known" value={rows.length} detail={`${active.length} devices · ${withoutCert} with none observed`} />
        <Kpi label="Expired" value={counts.expired} color={counts.expired ? 'var(--crit)' : undefined} detail="critical alert raised" />
        <Kpi label="≤ 30 days" value={counts.d7 + counts.d30} color={counts.d7 + counts.d30 ? 'var(--warn)' : undefined} detail={`${counts.d7} within 7 days`} />
        <Kpi label="Self-signed" value={counts.self} detail="enroll from a CA profile to replace" />
      </div>
      <div className="cols">
        <Card title="Inventory" id="ci" sub={loading ? 'loading per-device certificates…' : `${rows.length} certificates · soonest expiry first`} style={{ flex: '3 1 620px' }}
          right={<Seg sm label="Filter certificates" value={filter} onChange={setFilter} options={[{ v: 'all', label: 'All' }, { v: 'expiring', label: <>≤ 30 d <span className="num cap">{counts.d7 + counts.d30}</span></> }, { v: 'expired', label: <>Expired <span className="num cap">{counts.expired}</span></> }, { v: 'self', label: <>Self-signed <span className="num cap">{counts.self}</span></> }]} />}>
          <div className="cb" style={{ borderBottom: '1px solid var(--border)' }}>
            <div className="lbl" style={{ marginBottom: 8 }}>Certificate expiry · next 90 days</div>
            <ExpiryTimeline dates={rows.map((r) => r.notAfter)} />
          </div>
          {shown.length === 0 ? <div className="cb"><EmptyState icon={<Ic.lock size={20} />} title={rows.length === 0 ? 'No certificates observed' : 'No certificates match'} text={rows.length === 0 ? 'Assessing a device probes TLS on 443 / 631 and records the leaf certificate.' : 'Try another filter.'} tone={rows.length === 0 ? 'muted' : 'accent'} /></div> : (
            <div className="tw"><table className="tbl" style={{ minWidth: 860 }}>
              <thead><tr><th>Device</th><th>Subject</th><th>Use</th><th>Source</th><th>Expires</th>{canOperate && <th className="r">Actions</th>}</tr></thead>
              <tbody>{shown.map((r) => (
                <tr key={`${r.device.id}-${r.key}`} className={expiryTone(r.notAfter) === 'expired' ? '' : undefined}>
                  <td><Link to={`/devices/${r.device.id}`} style={{ fontWeight: 500 }}>{deviceName(r.device)}</Link><div className="cap mono" style={{ fontSize: 11 }}>{r.device.address}</div></td>
                  <td><span className="mono">{r.subject || '(no subject)'}</span>{r.selfSigned && <> <Tag>self-signed</Tag></>}<div className="cap" style={{ fontSize: 11 }}>issuer {r.issuer || '—'}{r.keyType ? ` · ${r.keyType}` : ''}</div></td>
                  <td className="cap">{r.use}</td>
                  <td><Tag>{r.source}</Tag></td>
                  <td><ExpiryPill notAfter={r.notAfter} /><div className="cap num" style={{ fontSize: 11 }}>{fmtTime(r.notAfter)}</div></td>
                  {canOperate && <td className="r" style={{ whiteSpace: 'nowrap' }}>{r.issuedId && !r.installed && <Btn sm variant="wr" onClick={() => install(r)} title="Retry installing the issued certificate"><Ic.upload />Install</Btn>} <Btn sm variant="wr" onClick={() => setEnroll(r.device)}><Ic.upload />Enroll…</Btn></td>}
                </tr>
              ))}</tbody>
            </table></div>
          )}
          {canOperate && active.length > 0 && (
            <div className="cf row"><span className="cap">Enroll on a device without a certificate:</span><label className="chip"><Ic.printer size={13} />Device<select aria-label="Enroll on device" value="" onChange={(e) => { const d = active.find((x) => x.id === e.target.value); if (d) setEnroll(d) }}><option value="">choose…</option>{active.map((d) => <option key={d.id} value={d.id}>{deviceName(d)} · {d.address}</option>)}</select></label></div>
          )}
        </Card>
        <CAProfilesCard profiles={profiles} isAdmin={user.role === 'admin'} onChanged={loadProfiles} />
      </div>
      {enroll && <EnrollDrawer device={enroll} profiles={profiles} onClose={() => setEnroll(null)} onDone={load} />}
    </>
  )
}

function CAProfilesCard({ profiles, isAdmin, onChanged }: { profiles: CAProfile[]; isAdmin: boolean; onChanged: () => void }) {
  const [creds, setCreds] = useState<Credential[]>([])
  const [adding, setAdding] = useState(false)
  const [form, setForm] = useState({ name: '', protocol: 'scep' as CAProfile['protocol'], url: '', credentialId: '', caFingerprint: '', validityDays: '', keyType: 'rsa2048', email: '', dnsWait: '', label: '', caIdent: '' })
  const [msg, setMsg] = useState('')
  useEffect(() => { if (isAdmin) api.get<Credential[]>('/api/credentials').then(setCreds).catch(() => {}) }, [isAdmin])
  const create = async (e: React.FormEvent) => {
    e.preventDefault(); setMsg('')
    const options: Record<string, string> = {}
    if (form.protocol === 'acme') { if (form.email) options.email = form.email; if (form.dnsWait) options.dnsWait = form.dnsWait; options.dnsProvider = 'manual' }
    if (form.protocol === 'est' && form.label) options.label = form.label
    if (form.protocol === 'scep' && form.caIdent) options.caIdent = form.caIdent
    try {
      await api.post('/api/ca-profiles', { name: form.name, protocol: form.protocol, url: form.url, credentialId: form.credentialId || undefined, caFingerprint: form.caFingerprint || undefined, validityDays: form.validityDays ? Number(form.validityDays) : undefined, keyType: form.keyType, options })
      setAdding(false); setForm({ ...form, name: '', url: '', credentialId: '', caFingerprint: '' }); onChanged()
    } catch (err) { setMsg((err as Error).message) }
  }
  const remove = async (p: CAProfile) => { if (!confirm(`Delete CA profile “${p.name}”?`)) return; try { await api.del(`/api/ca-profiles/${p.id}`); onChanged() } catch (e) { setMsg((e as Error).message) } }
  const credHint = form.protocol === 'scep' ? 'challenge password (admin_password credential)' : form.protocol === 'est' ? 'HTTP Basic user / password (admin_password credential)' : 'ACME account key as PEM (certificate credential); none = fresh account per enrollment'
  return (
    <Card title="CA profiles" id="cap" sub="SCEP · EST · ACME (dns-01) · credentials referenced by id, never shown" style={{ flex: '2 1 380px' }} right={isAdmin && !adding && <Btn sm variant="pri" onClick={() => setAdding(true)}><Ic.plus />Add profile</Btn>}>
      {msg && <div className="cb"><Banner tone="crit">{msg}</Banner></div>}
      {profiles.length === 0 && !adding && <div className="cb cap">{isAdmin ? 'No CA profile yet — add one to enroll certificates.' : 'No CA profile configured. An admin can add one.'}</div>}
      {profiles.map((p) => (
        <div key={p.id} className="lrow" style={{ alignItems: 'flex-start' }}>
          <Tag up>{p.protocol.toUpperCase()}</Tag>
          <div style={{ flex: 1, minWidth: 0 }}>
            <b style={{ fontWeight: 500 }}>{p.name}</b> <span className="cap">{p.keyType || 'rsa2048'}{p.validityDays ? ` · ${p.validityDays} d` : ''}</span>
            <div className="cap mono" style={{ fontSize: 11, overflowWrap: 'anywhere' }}>{p.url}</div>
            <div className="cap" style={{ display: 'flex', flexWrap: 'wrap', gap: 8, marginTop: 4 }}>
              {p.credentialId ? <span style={{ display: 'inline-flex', alignItems: 'center', gap: 4 }}><Ic.lock size={12} />credential <span className="mono">{p.credentialId.slice(0, 8)}</span> · never displayed</span> : <span>no credential</span>}
              {p.caFingerprint && <span title={p.caFingerprint}>pinned CA {p.caFingerprint.slice(0, 12)}…</span>}
              {p.options && Object.entries(p.options).map(([k, v]) => <span key={k}>{k}={v}</span>)}
            </div>
          </div>
          {isAdmin && <Btn sm variant="ghost" ic aria-label={`Delete ${p.name}`} onClick={() => remove(p)}><Ic.x /></Btn>}
        </div>
      ))}
      {adding && (
        <form onSubmit={create} style={{ borderTop: '1px solid var(--border)' }}>
          <div className="fr"><label htmlFor="ca-name">Name</label><input className="fin" id="ca-name" required value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} /></div>
          <div className="fr"><label htmlFor="ca-proto">Protocol</label><div><Seg sm label="Protocol" value={form.protocol} onChange={(v) => setForm({ ...form, protocol: v })} options={[{ v: 'scep', label: 'SCEP' }, { v: 'est', label: 'EST' }, { v: 'acme', label: 'ACME' }]} /><div className="hint">{form.protocol === 'scep' ? 'RFC 8894 · RSA keys only' : form.protocol === 'est' ? 'RFC 7030 · /cacerts + /simpleenroll' : 'RFC 8555 · dns-01 only; printers cannot answer http-01'}</div></div></div>
          <div className="fr"><label htmlFor="ca-url">URL</label><input className="fin mono" id="ca-url" required placeholder="https://ca.example.lan/scep" value={form.url} onChange={(e) => setForm({ ...form, url: e.target.value })} /></div>
          <div className="fr"><label htmlFor="ca-cred">Credential</label><div><select className="fin" id="ca-cred" value={form.credentialId} onChange={(e) => setForm({ ...form, credentialId: e.target.value })}><option value="">none</option>{creds.map((c) => <option key={c.id} value={c.id}>{c.name} · {c.type.replace('_', ' ')}</option>)}</select><div className="hint">{credHint} · <Link to="/credentials">manage credentials</Link></div></div></div>
          <div className="fr"><label htmlFor="ca-fp">CA fingerprint</label><div><input className="fin mono" id="ca-fp" placeholder="sha256 hex of the server's TLS leaf or chain cert" value={form.caFingerprint} onChange={(e) => setForm({ ...form, caFingerprint: e.target.value })} /><div className="hint">Pins a private CA instead of system roots.</div></div></div>
          <div className="fr"><label htmlFor="ca-key">Key type</label><div className="row"><select className="fin" id="ca-key" style={{ width: 160 }} value={form.keyType} onChange={(e) => setForm({ ...form, keyType: e.target.value })}><option value="rsa2048">rsa2048</option><option value="rsa3072">rsa3072</option><option value="ecdsa-p256">ecdsa-p256</option></select><input className="fin" type="number" min={1} placeholder="validity days" style={{ width: 140 }} value={form.validityDays} onChange={(e) => setForm({ ...form, validityDays: e.target.value })} /></div></div>
          {form.protocol === 'acme' && <div className="fr"><label>ACME options</label><div className="row"><input className="fin" placeholder="account email" style={{ flex: '1 1 160px' }} value={form.email} onChange={(e) => setForm({ ...form, email: e.target.value })} /><input className="fin mono" placeholder="dnsWait e.g. 5m" style={{ width: 140 }} value={form.dnsWait} onChange={(e) => setForm({ ...form, dnsWait: e.target.value })} /></div></div>}
          {form.protocol === 'est' && <div className="fr"><label htmlFor="ca-label">EST label</label><input className="fin mono" id="ca-label" placeholder="optional path label" value={form.label} onChange={(e) => setForm({ ...form, label: e.target.value })} /></div>}
          {form.protocol === 'scep' && <div className="fr"><label htmlFor="ca-ident">CA identifier</label><input className="fin mono" id="ca-ident" placeholder="optional GetCACert message" value={form.caIdent} onChange={(e) => setForm({ ...form, caIdent: e.target.value })} /></div>}
          <div className="cf" style={{ display: 'flex', justifyContent: 'flex-end', gap: 8 }}><Btn onClick={() => setAdding(false)}>Cancel</Btn><Btn type="submit" variant="pri">Create profile</Btn></div>
        </form>
      )}
    </Card>
  )
}

// --- Credentials & rotation ------------------------------------------------------------------

function CredentialsTab({ devices, groups, canOperate }: { devices: Device[]; groups: Group[]; canOperate: boolean }) {
  const [kind, setKind] = useState<RotateKind>('snmpCommunity')
  const [target, setTarget] = useState<SecurityTarget>({ deviceIds: [] })
  const [dryId, setDryId] = useState<string | null>(null)
  const [runId, setRunId] = useState<string | null>(null)
  const [confirm, setConfirm] = useState(false)
  const [busy, setBusy] = useState(false)
  const [msg, setMsg] = useState('')
  const [creds, setCreds] = useState<Credential[]>([])
  const dry = useJob(dryId)
  const run = useJob(runId)
  useEffect(() => { if (canOperate) api.get<Credential[]>('/api/credentials').then(setCreds).catch(() => {}) }, [canOperate, run.done])
  const byId = useMemo(() => Object.fromEntries(devices.map((d) => [d.id, d])), [devices])
  const submit = async (dryRun: boolean) => {
    setBusy(true); setMsg('')
    try {
      const j = await api.post<Job>('/api/security/rotate', { kind, target, dryRun })
      if (dryRun) { setDryId(j.id); setRunId(null) } else { setRunId(j.id); setConfirm(false) }
    } catch (e) { setMsg((e as Error).message) } finally { setBusy(false) }
  }
  const would = dry.results.filter((r) => (r.data as unknown as RotationData | undefined)?.status === 'would_rotate')
  const kindLabel = kind === 'snmpCommunity' ? 'SNMP read community' : 'administrator password'
  const managed = creds.filter((c) => c.type === 'snmp_community' || c.type === 'admin_password')
  const valid = !!target.groupId || !!target.deviceIds?.length

  return (
    <>
      <Banner tone="info"><b style={{ color: 'var(--info)' }}>Secrets are never displayed.</b> Rotation writes a new random 24-character secret to each device; only when the device accepts it is a device-scoped credential stored, outranking the previous one. Results, events, audit and logs carry the credential id, never the value.</Banner>
      {msg && <Banner tone="crit">{msg}</Banner>}
      <div className="cols">
        <Card title="Rotate credentials" id="rot" sub="dry run first · per-device results" style={{ flex: '3 1 560px' }}>
          {!canOperate ? <div className="cb"><EmptyState icon={<Ic.lock size={20} />} title="View only" text="Rotating credentials needs the operator role." tone="muted" /></div> : (
            <>
              <div className="cb" style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
                <div className="row"><span style={{ fontWeight: 500 }}>Secret</span><Seg label="Secret kind" value={kind} onChange={(k) => { setKind(k); setDryId(null); setRunId(null) }} options={[{ v: 'snmpCommunity', label: 'SNMP community' }, { v: 'adminPassword', label: 'Admin password' }]} /><span className="cap">writes <span className="mono">{kind === 'snmpCommunity' ? 'protocols.snmp.readCommunity' : 'security.adminPassword'}</span></span></div>
                <TargetPicker value={target} onChange={(t) => { setTarget(t); setDryId(null); setRunId(null) }} devices={devices} groups={groups} />
                <div className="row" style={{ justifyContent: 'flex-end' }}>
                  <Btn variant="pri" disabled={!valid || busy || (!!dry.job && !dry.done)} onClick={() => submit(true)}><Ic.refresh />Dry run</Btn>
                  <Btn variant="wr" disabled={!dry.done || would.length === 0 || busy || !!runId} onClick={() => setConfirm(true)}><Ic.upload />Rotate for real…</Btn>
                </div>
              </div>
              {dry.job && (
                <div style={{ borderTop: '1px solid var(--border)' }}>
                  <div className="cb"><JobProgress job={dry.job}>Dry run · {targetLabel(target, groups, devices)}</JobProgress></div>
                  {dry.done && <div style={{ padding: '0 16px 12px' }}><Banner tone="info"><b style={{ color: 'var(--info)' }}>Dry run</b> — {would.length} device{would.length === 1 ? '' : 's'} would accept a new {kindLabel}; nothing was written or stored.</Banner></div>}
                  <RotationTable results={dry.results} byId={byId} />
                </div>
              )}
              {run.job && (
                <div style={{ borderTop: '1px solid var(--border)' }}>
                  <div className="cb"><JobProgress job={run.job}>Rotation · {targetLabel(target, groups, devices)}</JobProgress></div>
                  {run.done && <div style={{ padding: '0 16px 12px' }}><Banner tone={run.job.status === 'succeeded' ? 'ok' : run.job.status === 'partial' ? 'warn' : 'crit'}><b>{run.job.status === 'succeeded' ? 'Rotated' : run.job.status === 'partial' ? 'Partially rotated' : 'Rotation failed'}</b> — {run.results.filter((r) => (r.data as unknown as RotationData | undefined)?.status === 'rotated').length} new credential{run.results.filter((r) => (r.data as unknown as RotationData | undefined)?.status === 'rotated').length === 1 ? '' : 's'} stored. <Link to={`/jobs/${run.job.id}`}>Job</Link></Banner></div>}
                  <RotationTable results={run.results} byId={byId} />
                </div>
              )}
            </>
          )}
        </Card>
        <Card title="Managed secrets" id="ms" sub="stored credentials of the rotatable kinds" style={{ flex: '2 1 340px' }}>
          <div className="cb" style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
            {!canOperate ? <span className="cap">Listing credentials needs the operator role.</span> : managed.length === 0 ? <span className="cap">No SNMP community or admin password stored yet. Rotation stores one per device that accepts the new secret.</span> : managed.slice(0, 12).map((c) => (
              <SecretRow key={c.id} label={c.name} detail={`${c.type === 'snmp_community' ? 'community' : 'password'} · ${c.scope ? c.scope.replace('device:', 'device ').slice(0, 24) : 'global'} · ${ago(c.createdAt)}`} replaceTo="/credentials" />
            ))}
            {managed.length > 12 && <span className="cap">+{managed.length - 12} more under <Link to="/credentials">Credentials</Link></span>}
          </div>
        </Card>
      </div>
      {confirm && (
        <ConfirmWriteDialog title={`Rotate the ${kindLabel} on ${would.length} device${would.length === 1 ? '' : 's'}?`} sub={<>Target {targetLabel(target, groups, devices)} · dry-run job {dry.job?.id.slice(0, 8)}</>}
          deviceCount={would.length} confirmWord={target.groupId ? (groups.find((g) => g.id === target.groupId)?.name || 'ROTATE') : 'ROTATE'} confirmLabel={`Rotate on ${would.length} device${would.length === 1 ? '' : 's'}`}
          rows={[['Devices written', would.length], ['Skipped in dry run', dry.results.length - would.length], ['Secret', 'new random 24 characters per device · never displayed'], ['Stored as', 'device-scoped credential, higher priority than the current one'], ['If a device rejects it', 'nothing is stored; the old credential stays']]}
          onBack={() => setConfirm(false)} onConfirm={() => submit(false)} busy={busy} backLabel="Back" icon={<Ic.key />} />
      )}
    </>
  )
}

function RotationTable({ results, byId }: { results: JobResult[]; byId: Record<string, Device> }) {
  if (results.length === 0) return <div className="cb cap">No per-device results yet.</div>
  const tone = (s?: string) => s === 'rotated' ? 'ok' : s === 'would_rotate' ? 'info' : s === 'failed' ? 'crit' : 'off'
  return (
    <div className="tw"><table className="tbl" style={{ minWidth: 640 }}>
      <thead><tr><th>Device</th><th>Outcome</th><th>Driver</th><th>Credential</th><th>Note</th></tr></thead>
      <tbody>{results.map((r) => { const d = r.data as unknown as RotationData | undefined; const dev = byId[r.deviceId || '']; return (
        <tr key={r.id} className={d?.status === 'skipped' ? 'dim' : ''}>
          <td style={{ fontWeight: 500 }}>{dev ? <Link to={`/devices/${dev.id}`}>{deviceName(dev)}</Link> : r.address || '—'}</td>
          <td><Pill tone={tone(d?.status)} noIcon={d?.status === 'would_rotate'}>{d?.status === 'would_rotate' ? <><Ic.upload />Would rotate</> : d?.status ? d.status.charAt(0).toUpperCase() + d.status.slice(1) : r.status}</Pill></td>
          <td className="cap">{d?.driver ? <Tag>{d.driver}</Tag> : '—'}</td>
          <td className="cap">{d?.credentialId ? <span style={{ display: 'inline-flex', alignItems: 'center', gap: 4 }}><Ic.lock size={12} />stored · <span className="mono">{d.credentialId.slice(0, 8)}</span> · never displayed</span> : '—'}</td>
          <td className="wrap cap">{r.message}</td>
        </tr>) })}</tbody>
    </table></div>
  )
}

