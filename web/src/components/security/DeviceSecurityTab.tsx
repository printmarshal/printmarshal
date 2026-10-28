// Security tab of the device page: compliance card (latest assessment per
// policy), certificates card, assess-now and the remediate / enroll flows.
import { useCallback, useEffect, useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, ago, fmtTime } from '../../api'
import type { Device, Job } from '../../types'
import { useEvents } from '../../ws'
import { Ic } from '../icons'
import { Banner, Btn, Card, Pill, Tag } from '../ui'
import type { Assessment, CAProfile, DeviceCertificates, DeviceSecurity, Policy } from '../../types-security'
import { ExpiryPill, ItemStatusPill, JobProgress, ScoreBar, SevMark, fmtVal, useJob } from './common'
import RemediateFlow from './RemediateFlow'
import EnrollDrawer from './EnrollDrawer'

export default function DeviceSecurityTab({ device, canOperate }: { device: Device; canOperate: boolean }) {
  const [sec, setSec] = useState<DeviceSecurity | null>(null)
  const [policies, setPolicies] = useState<Policy[]>([])
  const [profiles, setProfiles] = useState<CAProfile[]>([])
  const [policyId, setPolicyId] = useState('')
  const [error, setError] = useState('')
  const [jobId, setJobId] = useState<string | null>(null)
  const [remediate, setRemediate] = useState(false)
  const [enroll, setEnroll] = useState(false)
  const [msg, setMsg] = useState('')
  const job = useJob(jobId)

  const load = useCallback(() => {
    api.get<DeviceSecurity>(`/api/devices/${device.id}/security`).then(setSec).catch((e) => setError((e as Error).message))
  }, [device.id])
  useEffect(() => { load() }, [load])
  useEffect(() => {
    api.get<Policy[]>('/api/policies').then((ps) => { setPolicies(ps); setPolicyId((cur) => cur || ps[0]?.id || '') }).catch(() => {})
    api.get<CAProfile[]>('/api/ca-profiles').then(setProfiles).catch(() => {})
  }, [])
  useEvents((e) => {
    const d = e.data as { deviceId?: string } | null
    if ((e.type === 'security.assessed' || e.type === 'security.remediated' || e.type.startsWith('security.certificate.')) && d?.deviceId === device.id) load()
  }, [device.id, load])
  useEffect(() => { if (job.done) load() }, [job.done, load])

  const assessments = sec?.assessments || []
  const shown: Assessment | undefined = useMemo(() => assessments.find((a) => a.policyId === policyId) || assessments[0], [assessments, policyId])
  const policy = policies.find((p) => p.id === (shown?.policyId || policyId))
  const sevOf = (itemId: string) => policy?.items.find((i) => i.id === itemId)?.severity || 'medium'
  const failing = (shown?.items || []).filter((i) => i.status === 'fail')
  const unknown = (shown?.items || []).filter((i) => i.status === 'unknown')

  const assessNow = async () => {
    const pid = policyId || policies[0]?.id
    if (!pid) return
    setMsg('')
    try { const j = await api.post<Job>(`/api/policies/${pid}/assess`, { deviceIds: [device.id] }); setJobId(j.id) } catch (e) { setMsg((e as Error).message) }
  }
  const install = async (issuedId: string) => {
    try { const j = await api.post<Job>(`/api/devices/${device.id}/certificates/${issuedId}/install`, {}); setJobId(j.id); setMsg('Install job queued.') } catch (e) { setMsg((e as Error).message) }
  }

  if (error) return <Banner tone="crit">{error}</Banner>
  if (!sec) return <div className="cap" aria-busy="true">Loading security data…</div>
  const certs = certRows(sec.certificates)

  return (
    <>
      {msg && <Banner tone="info">{msg}</Banner>}
      {job.job && !job.done && <Card><div className="cb"><JobProgress job={job.job}>{job.job.type === 'security.assess' ? 'Assessing' : job.job.type === 'security.install' ? 'Installing certificate' : job.job.type} · {device.address}</JobProgress></div></Card>}
      {sec.alerts.length > 0 && (
        <Banner tone="crit" role="alert" right={<Link className="btn sm ghost" to="/alerts">Open alerts</Link>}>
          <b style={{ color: 'var(--crit)' }}>{sec.alerts.length} security alert{sec.alerts.length === 1 ? '' : 's'}</b> — {sec.alerts.map((a) => a.description).join('; ')}
        </Banner>
      )}
      <div className="cols">
        <Card style={{ flex: '1 1 400px' }} id="dsc"
          title={<>Compliance{policy ? ` · ${policy.name}` : ''}</>}
          sub={shown ? <>assessed {ago(shown.at)} · {fmtTime(shown.at)}{shown.jobId && <> · <Link to={`/security/assessments/${shown.jobId}`}>results</Link></>}</> : 'never assessed'}
          right={<div className="row">
            {assessments.length > 1 && <label className="chip"><select aria-label="Policy" value={shown?.policyId || ''} onChange={(e) => setPolicyId(e.target.value)}>{assessments.map((a) => <option key={a.policyId} value={a.policyId}>{policies.find((p) => p.id === a.policyId)?.name || a.policyId.slice(0, 8)}</option>)}</select></label>}
            {canOperate && assessments.length <= 1 && policies.length > 1 && <label className="chip"><select aria-label="Policy to assess" value={policyId} onChange={(e) => setPolicyId(e.target.value)}>{policies.map((p) => <option key={p.id} value={p.id}>{p.name}</option>)}</select></label>}
            {canOperate && <Btn sm onClick={assessNow} disabled={!!(job.job && !job.done) || policies.length === 0}><Ic.refresh />Assess now</Btn>}
            {canOperate && policy && failing.length > 0 && <Btn sm variant="wr" onClick={() => setRemediate(true)}><Ic.upload />Remediate…</Btn>}
          </div>}>
          {!shown ? (
            <div className="cb"><div className="state" style={{ minHeight: 120 }}><span className="ring" style={{ background: 'var(--accent-weak)', color: 'var(--accent)' }}><Ic.shield size={20} /></span><b>Not assessed yet</b><span className="cap">{policies.length ? `Run “${policies[0].name}” against this device to see its compliance.` : 'Create a policy under Security first.'}</span></div></div>
          ) : (
            <div className="cb" style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
              <div style={{ display: 'flex', alignItems: 'baseline', gap: 8, flexWrap: 'wrap' }}>
                <span className="num" style={{ fontSize: 24, fontWeight: 600 }}>{shown.passed} / {shown.passed + shown.failed}</span><span className="cap">items passing</span>
                <span style={{ marginLeft: 'auto' }}>{shown.compliant ? <Pill tone="ok">Compliant</Pill> : <Pill tone={shown.score >= 80 ? 'warn' : 'crit'}>Non-compliant</Pill>}</span>
                <span className="cap num">score {shown.score.toFixed(1)}{unknown.length ? ` · ${unknown.length} unknown` : ''}</span>
              </div>
              <ScoreBar a={shown} />
              <div style={{ borderTop: '1px solid var(--border)' }}>
                {failing.length === 0 && <div className="lrow" style={{ padding: '8px 0' }}><Pill tone="ok">Pass</Pill><span className="cap">Every evaluated item passes.</span></div>}
                {failing.map((it) => (
                  <div className="lrow" key={it.itemId} style={{ padding: '8px 0', alignItems: 'flex-start' }}>
                    <ItemStatusPill status={it.status} />
                    <div style={{ flex: 1, minWidth: 0 }}>
                      <span style={{ display: 'inline-flex', alignItems: 'center', gap: 8 }}>{it.name}<SevMark sev={sevOf(it.itemId)} /></span>
                      <div className="cap">{it.message || <>current <span className="mono">{fmtVal(it.current)}</span> · desired <span className="mono">{fmtVal(it.desired)}</span></>}</div>
                    </div>
                  </div>
                ))}
                {unknown.length > 0 && <div className="lrow" style={{ padding: '8px 0' }}><Pill tone="unk">Unknown</Pill><span className="cap">{unknown.map((u) => u.name).join(', ')} — not readable on this device; counted but not scored.</span></div>}
              </div>
            </div>
          )}
        </Card>
        <Card style={{ flex: '1 1 400px' }} id="dcert" title="Certificates" sub={`${certs.length} known · installed, observed by the TLS probe, and issued by PrintMarshal`}
          right={canOperate && <Btn sm variant="wr" onClick={() => setEnroll(true)}><Ic.upload />Enroll / renew…</Btn>}>
          {certs.length === 0 ? <div className="cb cap">No certificates observed yet. Running an assessment probes TLS on ports 443 and 631.</div> : (
            <div className="tw"><table className="tbl" style={{ minWidth: 520 }}>
              <thead><tr><th>Subject</th><th>Use</th><th>Source</th><th>Expires</th><th /></tr></thead>
              <tbody>{certs.map((c) => (
                <tr key={c.key}>
                  <td><span className="mono">{c.subject || '(no subject)'}</span>{c.selfSigned && <> <Tag>self-signed</Tag></>}<div className="cap" style={{ fontSize: 11 }}>issuer {c.issuer || '—'}{c.keyType ? ` · ${c.keyType}` : ''}</div></td>
                  <td className="cap">{c.use}</td>
                  <td><Tag>{c.source}</Tag></td>
                  <td><ExpiryPill notAfter={c.notAfter} /></td>
                  <td className="r">{c.issuedId && !c.installed && canOperate && <Btn sm variant="wr" onClick={() => install(c.issuedId!)} title="Retry installing this issued certificate through a driver"><Ic.upload />Install</Btn>}{c.issuedId && !c.installed && <div className="cap">issued · not installed</div>}</td>
                </tr>
              ))}</tbody>
            </table></div>
          )}
          {sec.certificates.tls?.some((t) => t.minVersion) && <div className="cf cap">TLS probe: {sec.certificates.tls.filter((t) => t.minVersion).map((t) => `port ${t.port} min ${t.minVersion}`).join(' · ')}</div>}
        </Card>
      </div>
      {remediate && policy && <RemediateFlow policy={policy} target={{ deviceIds: [device.id] }} devices={[device]} groups={[]} onClose={() => { setRemediate(false); load() }} />}
      {enroll && <EnrollDrawer device={device} profiles={profiles} onClose={() => setEnroll(false)} onDone={load} />}
    </>
  )
}

export interface CertRow { key: string; subject: string; issuer: string; use: string; source: string; notAfter: string; selfSigned: boolean; keyType?: string; issuedId?: string; installed?: boolean }
/** Flatten installed / observed / issued certificates into display rows (issued ones that were installed are shown once). */
export function certRows(c: DeviceCertificates | undefined): CertRow[] {
  if (!c) return []
  const rows: CertRow[] = []
  for (const x of c.installed || []) rows.push({ key: `i-${x.id}-${x.fingerprint}`, subject: x.subject, issuer: x.issuer, use: x.use, source: x.source || 'installed', notAfter: x.notAfter, selfSigned: x.selfSigned, keyType: x.keyType })
  for (const x of c.observed || []) if (!rows.some((r) => r.key.endsWith(x.fingerprint || '-'))) rows.push({ key: `o-${x.id}-${x.fingerprint}`, subject: x.subject, issuer: x.issuer, use: x.use || 'identity', source: 'observed', notAfter: x.notAfter, selfSigned: x.selfSigned, keyType: x.keyType })
  for (const x of c.issued || []) rows.push({ key: `s-${x.id}`, subject: x.subject, issuer: x.issuer, use: x.use, source: x.installed ? `issued · ${x.installedBy || 'installed'}` : 'issued', notAfter: x.notAfter, selfSigned: false, keyType: x.keyType, issuedId: x.id, installed: x.installed })
  return rows.sort((a, b) => new Date(a.notAfter).getTime() - new Date(b.notAfter).getTime())
}
