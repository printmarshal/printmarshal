// Certificate enrollment for one device: dry run (CSR only, no CA contact) →
// review subject / SANs / install driver → enroll for real → result. The private
// key never leaves the server; the API returns only the certificate.
import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { api } from '../../api'
import type { Device, Job } from '../../types'
import { deviceName } from '../../types'
import { Ic } from '../icons'
import { Banner, Btn, Dialog, Pill, Tag } from '../ui'
import type { CAProfile, EnrollData, EnrollRequest } from '../../types-security'
import { JobProgress, useJob } from './common'

type Step = 'form' | 'dry' | 'confirm' | 'run'

export default function EnrollDrawer({ device, profiles, onClose, onDone }: { device: Device; profiles: CAProfile[]; onClose: () => void; onDone?: () => void }) {
  const [step, setStep] = useState<Step>('form')
  const [form, setForm] = useState<{ caProfileId: string; use: 'identity' | 'ca'; commonName: string; sans: string }>({ caProfileId: profiles[0]?.id || '', use: 'identity', commonName: device.hostname || device.address, sans: [device.hostname, device.address].filter(Boolean).join(', ') })
  const [dryId, setDryId] = useState<string | null>(null)
  const [runId, setRunId] = useState<string | null>(null)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const [ask, setAsk] = useState(false)
  const dry = useJob(dryId)
  const run = useJob(runId)
  const profile = profiles.find((p) => p.id === form.caProfileId)
  useEffect(() => { if (dry.done && step === 'dry') setStep('confirm') }, [dry.done, step])
  useEffect(() => { if (run.done && onDone) onDone() }, [run.done, onDone])

  const body = (dryRun: boolean): EnrollRequest => ({ caProfileId: form.caProfileId, use: form.use, commonName: form.commonName.trim() || undefined, sans: form.sans.split(',').map((s) => s.trim()).filter(Boolean), dryRun })
  const submit = async (dryRun: boolean) => {
    setBusy(true); setError('')
    try {
      const j = await api.post<Job>(`/api/devices/${device.id}/certificates/enroll`, body(dryRun))
      if (dryRun) { setDryId(j.id); setStep('dry') } else { setRunId(j.id); setAsk(false); setStep('run') }
    } catch (e) { setError((e as Error).message) } finally { setBusy(false) }
  }
  const dryData = dry.results[0]?.data as unknown as EnrollData | undefined
  const runData = run.results[0]?.data as unknown as EnrollData | undefined

  return (
    <div className="drawer-backdrop" onClick={onClose}>
      <div className="drawer" role="dialog" aria-modal="true" aria-label="Enroll certificate" onClick={(e) => e.stopPropagation()}>
        <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 8 }}>
          <div><div className="lbl">Enroll / renew certificate</div><h2 style={{ fontSize: 16, fontWeight: 600, marginTop: 2 }}>{deviceName(device)}</h2><div className="cap mono">{device.address}{device.hostname ? ` · ${device.hostname}` : ''}</div></div>
          <Btn ic variant="ghost" aria-label="Close" onClick={onClose}><Ic.x /></Btn>
        </div>
        {error && <Banner tone="crit">{error}</Banner>}
        {profiles.length === 0 && <Banner tone="warn"><b style={{ color: 'var(--warn)' }}>No CA profile.</b> An admin must add a SCEP, EST or ACME profile under Security → Certificates before enrolling.</Banner>}

        {step === 'form' && (
          <>
            <div className="fr" style={{ padding: '10px 0' }}><label htmlFor="en-ca">CA profile</label><div>
              <select className="fin" id="en-ca" value={form.caProfileId} onChange={(e) => setForm({ ...form, caProfileId: e.target.value })}>{profiles.map((p) => <option key={p.id} value={p.id}>{p.name} · {p.protocol.toUpperCase()}</option>)}</select>
              {profile && <div className="hint mono" style={{ fontSize: 11 }}>{profile.url} · {profile.keyType || 'rsa2048'}{profile.credentialId ? ' · credential referenced' : ''}</div>}
            </div></div>
            <div className="fr" style={{ padding: '10px 0' }}><label htmlFor="en-use">Use</label><div><select className="fin" id="en-use" value={form.use} onChange={(e) => setForm({ ...form, use: e.target.value as 'identity' | 'ca' })}><option value="identity">identity (TLS server certificate)</option><option value="ca">ca (trust anchor)</option></select></div></div>
            <div className="fr" style={{ padding: '10px 0' }}><label htmlFor="en-cn">Common name</label><div><input className="fin mono" id="en-cn" value={form.commonName} onChange={(e) => setForm({ ...form, commonName: e.target.value })} /><div className="hint">Defaults to the device hostname, else its address.</div></div></div>
            <div className="fr" style={{ padding: '10px 0' }}><label htmlFor="en-sans">SANs</label><div><input className="fin mono" id="en-sans" value={form.sans} onChange={(e) => setForm({ ...form, sans: e.target.value })} placeholder="host.example.lan, 10.0.0.12" /><div className="hint">Comma-separated DNS names and IPs. ACME accepts DNS names only (dns-01).</div></div></div>
            <Banner tone="info"><b style={{ color: 'var(--info)' }}>Dry run first.</b> The server generates the key and CSR, reports the subject, SANs and which driver would install it, and contacts no CA.</Banner>
            <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 8, marginTop: 'auto', paddingTop: 8, borderTop: '1px solid var(--border)' }}>
              <Btn onClick={onClose}>Cancel</Btn>
              <Btn variant="pri" disabled={!form.caProfileId || busy} onClick={() => submit(true)}>{busy ? 'Queuing…' : 'Run dry run'}</Btn>
            </div>
          </>
        )}

        {step === 'dry' && (dry.job ? <JobProgress job={dry.job}>Dry run · CSR generation</JobProgress> : <div className="cap" aria-busy="true">Queuing dry run…</div>)}

        {step === 'confirm' && (
          <>
            {dry.results[0]?.status === 'error'
              ? <Banner tone="crit"><b style={{ color: 'var(--crit)' }}>Dry run failed</b> — {dry.results[0].message}</Banner>
              : <Banner tone="info"><b style={{ color: 'var(--info)' }}>Dry run complete</b> — {dry.results[0]?.message}</Banner>}
            {dryData && <Preview d={dryData} />}
            {dryData && !dryData.canInstall && <Banner tone="warn"><b style={{ color: 'var(--warn)' }}>No driver can install on this device.</b> SNMP and IPP have no certificate operation. The certificate will still be issued and kept retrievable so you can install it by hand through the device web page{device.adminUrl ? <> (<a href={device.adminUrl} target="_blank" rel="noreferrer">open</a>)</> : ''}, or install it later once a vendor plugin serves the device.</Banner>}
            <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 8, marginTop: 'auto', paddingTop: 8, borderTop: '1px solid var(--border)' }}>
              <Btn onClick={() => setStep('form')}>Back</Btn>
              {dry.results[0]?.status !== 'error' && <Btn variant="wr" disabled={busy} onClick={() => setAsk(true)}><Ic.upload />Enroll for real…</Btn>}
            </div>
          </>
        )}

        {ask && !runId && (
          <Dialog title={`Enroll ${form.use} certificate on ${deviceName(device)}?`} sub={`${profile?.name} (${profile?.protocol.toUpperCase()}) will be contacted; ${dryData?.canInstall ? `installed via ${dryData.installDriver}` : 'issued only — manual install'}`} onClose={() => setAsk(false)}
            icon={<span style={{ width: 32, height: 32, borderRadius: 8, background: 'var(--warn-bg)', color: 'var(--warn)', display: 'grid', placeItems: 'center', flex: 'none' }}><Ic.upload /></span>}
            footer={<><Btn onClick={() => setAsk(false)}>Back</Btn><Btn variant="dngf" disabled={busy} onClick={() => submit(false)}>{busy ? 'Starting…' : 'Enroll now'}</Btn></>}>
            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '6px 16px' }}>
              <span className="cap">Subject</span><b className="mono" style={{ fontWeight: 500 }}>{dryData?.commonName || form.commonName}</b>
              <span className="cap">SANs</span><b className="mono" style={{ fontWeight: 500, overflowWrap: 'anywhere' }}>{dryData?.sans?.join(', ') || '—'}</b>
              <span className="cap">Key</span><b style={{ fontWeight: 500 }}>{dryData?.keyType || profile?.keyType || 'rsa2048'}</b>
              <span className="cap">Private key</span><b style={{ fontWeight: 500 }}>generated server-side, stored encrypted, never returned</b>
            </div>
          </Dialog>
        )}
        {step === 'run' && runId && (
          <>
            {run.job ? <JobProgress job={run.job}>Enrollment · {profile?.protocol.toUpperCase()}</JobProgress> : <div className="cap" aria-busy="true">Queuing enrollment…</div>}
            {run.done && run.results[0] && (
              <Banner tone={run.results[0].status === 'ok' ? (runData?.installed ? 'ok' : 'warn') : 'crit'}>
                <b>{run.results[0].status === 'ok' ? (runData?.installed ? 'Issued and installed' : 'Issued') : 'Enrollment failed'}</b> — {run.results[0].message}
              </Banner>
            )}
            {runData && <Preview d={runData} />}
            {runData?.dnsRecord && <div className="card"><div className="cb"><div className="lbl" style={{ marginBottom: 6 }}>DNS record to create (dns-01)</div><div className="mono" style={{ overflowWrap: 'anywhere' }}>{runData.dnsRecord.name} {runData.dnsRecord.type} "{runData.dnsRecord.value}"</div><div className="cap" style={{ marginTop: 4 }}>Create it, then run the enrollment again with a `dnsWait` on the CA profile so the job polls for it.</div></div></div>}
            <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 8, marginTop: 'auto', paddingTop: 8, borderTop: '1px solid var(--border)' }}>
              {run.job && <Link className="btn sm ghost" to={`/jobs/${run.job.id}`}>Job</Link>}
              <Btn variant="pri" onClick={onClose}>Done</Btn>
            </div>
          </>
        )}
      </div>
    </div>
  )
}

function Preview({ d }: { d: EnrollData }) {
  return (
    <div className="card">
      <div style={{ padding: '10px 14px', display: 'grid', gridTemplateColumns: '130px minmax(0, 1fr)', gap: '6px 12px', alignItems: 'center' }}>
        <span className="cap">Protocol</span><span><Tag up>{d.protocol.toUpperCase()}</Tag> <span className="cap">{d.use}</span></span>
        <span className="cap">Subject</span><span className="mono">{d.subject || d.commonName}</span>
        <span className="cap">SANs</span><span className="mono" style={{ overflowWrap: 'anywhere' }}>{d.sans?.length ? d.sans.join(', ') : '—'}</span>
        <span className="cap">Key type</span><span className="mono">{d.keyType}</span>
        <span className="cap">Install</span><span>{d.canInstall ? <Pill tone="ok" xs>via {d.installDriver}</Pill> : <Pill tone="warn" xs>no driver · manual install</Pill>}{d.installed && <> <Pill tone="ok" xs>installed</Pill></>}</span>
        {d.fingerprint && <><span className="cap">Fingerprint</span><span className="mono" style={{ fontSize: 11, overflowWrap: 'anywhere' }}>{d.fingerprint}</span></>}
        {d.notAfter && <><span className="cap">Expires</span><span className="num">{new Date(d.notAfter).toLocaleString()}</span></>}
        {d.installError && <><span className="cap">Install error</span><span style={{ color: 'var(--crit)' }}>{d.installError}</span></>}
        {d.dryRun && <><span className="cap">Mode</span><span><Pill tone="info" xs noIcon>dry run · no CA contacted</Pill></span></>}
      </div>
    </div>
  )
}
