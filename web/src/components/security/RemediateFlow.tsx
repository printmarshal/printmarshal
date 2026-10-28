// Remediation flow (spec §11): dry run → preview per device/item → confirm with
// blast radius → real run → results. Nothing is written until the operator
// confirms, and management-path skips are explained, never hidden.
import { useEffect, useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { api } from '../../api'
import type { Device, Group, Job, JobResult } from '../../types'
import { deviceName } from '../../types'
import { Ic } from '../icons'
import { Banner, Btn, Tag } from '../ui'
import type { Policy, RemediationData, RemediationItem, SecurityTarget } from '../../types-security'
import { ConfirmWriteDialog, JobProgress, MgmtPathTag, RemediationPill, fmtVal, isMgmtPathSkip, targetLabel, useJob } from './common'

type Step = 'dry' | 'preview' | 'confirm' | 'run'

export default function RemediateFlow({ policy, target, devices, groups, onClose, onDone }: { policy: Policy; target: SecurityTarget; devices: Device[]; groups: Group[]; onClose: () => void; onDone?: (job: Job) => void }) {
  const [step, setStep] = useState<Step>('dry')
  const [dryId, setDryId] = useState<string | null>(null)
  const [runId, setRunId] = useState<string | null>(null)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const dry = useJob(dryId)
  const run = useJob(runId)

  useEffect(() => {
    let cancelled = false
    api.post<Job>(`/api/policies/${policy.id}/remediate`, { ...target, dryRun: true }).then((j) => { if (!cancelled) setDryId(j.id) }).catch((e) => setError((e as Error).message))
    return () => { cancelled = true }
  }, [policy.id, target])
  useEffect(() => { if (dry.done && step === 'dry') setStep('preview') }, [dry.done, step])
  useEffect(() => { if (run.done && run.job && onDone) onDone(run.job) }, [run.done, run.job, onDone])

  const byDevice = (id?: string) => devices.find((d) => d.id === id)
  const plan = useMemo(() => summarise(dry.results), [dry.results])
  const itemsByName = useMemo(() => Object.fromEntries(policy.items.map((i) => [i.id, i])), [policy.items])

  const confirmWord = target.groupId ? (groups.find((g) => g.id === target.groupId)?.name || 'REMEDIATE') : 'REMEDIATE'
  const start = async () => {
    setBusy(true); setError('')
    try { const j = await api.post<Job>(`/api/policies/${policy.id}/remediate`, { ...target, dryRun: false }); setRunId(j.id); setStep('run') } catch (e) { setError((e as Error).message) } finally { setBusy(false) }
  }

  return (
    <div className="drawer-backdrop" onClick={onClose}>
      <div className="drawer" role="dialog" aria-modal="true" aria-label="Remediate policy" style={{ width: 'min(860px, 100%)' }} onClick={(e) => e.stopPropagation()}>
        <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 8 }}>
          <div>
            <div className="lbl">Remediate · {step === 'run' ? 'writing' : 'dry run'}</div>
            <h2 style={{ fontSize: 16, fontWeight: 600, marginTop: 2 }}>{policy.name} → {targetLabel(target, groups, devices)}</h2>
            <div className="cap">{policy.items.filter((i) => i.remediate).length} of {policy.items.length} items allow remediation · every change is re-assessed and audited</div>
          </div>
          <Btn ic variant="ghost" aria-label="Close" onClick={onClose}><Ic.x /></Btn>
        </div>
        <Steps step={step} />
        {error && <Banner tone="crit">{error}</Banner>}

        {step === 'dry' && (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
            <Banner tone="info"><b style={{ color: 'var(--info)' }}>Dry run</b> — nothing is written. Each device is re-assessed and the engine plans one change per failing item.</Banner>
            {dry.job ? <JobProgress job={dry.job}>Dry run · {dry.job.id.slice(0, 8)}</JobProgress> : <div className="cap" aria-busy="true">Queuing dry run…</div>}
          </div>
        )}

        {(step === 'preview' || step === 'confirm') && (
          <>
            {plan.wouldWrite === 0
              ? <Banner tone={plan.failed > 0 ? 'warn' : 'ok'}><b>Nothing to write.</b> {plan.failed > 0 ? `${plan.failed} item${plan.failed === 1 ? '' : 's'} could not be evaluated.` : 'Every remediable item already passes, or no driver can change it.'}</Banner>
              : <Banner tone="info"><b style={{ color: 'var(--info)' }}>Preview</b> — {plan.wouldWrite} change{plan.wouldWrite === 1 ? '' : 's'} on {plan.devicesChanged} device{plan.devicesChanged === 1 ? '' : 's'} would be written. Nothing has changed yet.</Banner>}
            {plan.mgmtSkips > 0 && (
              <Banner tone="warn">
                <b style={{ color: 'var(--warn)' }}>{plan.mgmtSkips} item{plan.mgmtSkips === 1 ? '' : 's'} skipped by the management-path rule.</b> Disabling the only protocol PrintMarshal manages a device through would lock it out, so that change stays unapplied. Add another management path (for example an SNMPv3 credential or a vendor plugin) and run again.
              </Banner>
            )}
            {plan.noDriverSkips > 0 && <Banner tone="info"><b style={{ color: 'var(--info)' }}>{plan.noDriverSkips} item{plan.noDriverSkips === 1 ? '' : 's'} have no writer.</b> No driver on these devices accepts the setting (standards-only SNMP/IPP cannot change security settings); a vendor plugin is needed.</Banner>}
            <div style={{ display: 'flex', flexWrap: 'wrap', gap: 14 }} className="cap">
              <span><b className="num" style={{ color: 'var(--text)' }}>{plan.wouldWrite}</b> would write</span>
              <span><b className="num" style={{ color: 'var(--text)' }}>{plan.unchanged}</b> unchanged</span>
              <span><b className="num" style={{ color: 'var(--text)' }}>{plan.skipped}</b> skipped</span>
              <span><b className="num" style={{ color: plan.failed ? 'var(--crit)' : 'var(--text)' }}>{plan.failed}</b> failed</span>
              <span><b className="num" style={{ color: 'var(--text)' }}>{plan.errors}</b> device error{plan.errors === 1 ? '' : 's'}</span>
            </div>
            <ResultsTable results={dry.results} byDevice={byDevice} mgmtPathOf={(id) => itemsByName[id]?.managementPath} />
            <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 8, marginTop: 'auto', paddingTop: 8, borderTop: '1px solid var(--border)' }}>
              {dry.job && <Link className="btn sm ghost" to={`/jobs/${dry.job.id}`}>Dry-run job</Link>}
              <Btn onClick={onClose}>Close</Btn>
              <Btn variant="pri" disabled={plan.wouldWrite === 0} onClick={() => setStep('confirm')}>Continue to confirm</Btn>
            </div>
            {step === 'confirm' && (
              <ConfirmWriteDialog
                title={`Write ${plan.wouldWrite} change${plan.wouldWrite === 1 ? '' : 's'} to ${plan.devicesChanged} device${plan.devicesChanged === 1 ? '' : 's'}?`}
                sub={<>Policy “{policy.name}” → {targetLabel(target, groups, devices)}</>}
                deviceCount={plan.devicesChanged} confirmWord={confirmWord} confirmLabel={`Write to ${plan.devicesChanged} device${plan.devicesChanged === 1 ? '' : 's'}`}
                rows={[
                  ['Devices written', plan.devicesChanged],
                  ['Skipped (management path)', plan.mgmtSkips],
                  ['Skipped (no driver / assess-only)', plan.noDriverSkips + plan.otherSkips],
                  ['Items touched', plan.itemNames.join(', ') || '—'],
                  ['Management paths affected', plan.paths.length ? <span style={{ color: 'var(--warn)' }}>{plan.paths.map((p) => p.toUpperCase()).join(', ')} — at least one path is kept</span> : <span style={{ color: 'var(--ok)' }}>None</span>],
                  ['After writing', 'each device is re-assessed; before/after values are audited'],
                ]}
                onBack={() => setStep('preview')} onConfirm={start} busy={busy} />
            )}
          </>
        )}

        {step === 'run' && (
          <>
            {run.job ? <JobProgress job={run.job}>Remediation · {run.job.id.slice(0, 8)}</JobProgress> : <div className="cap" aria-busy="true">Queuing remediation…</div>}
            {run.done && run.job && (
              <Banner tone={run.job.status === 'succeeded' ? 'ok' : run.job.status === 'partial' ? 'warn' : 'crit'}>
                <b>{run.job.status === 'succeeded' ? 'Applied' : run.job.status === 'partial' ? 'Partially applied' : 'Failed'}</b> — {summarise(run.results).written} written, {summarise(run.results).skipped} skipped, {summarise(run.results).failed} failed across {run.results.length} device{run.results.length === 1 ? '' : 's'}.
              </Banner>
            )}
            <ResultsTable results={run.results} byDevice={byDevice} mgmtPathOf={(id) => itemsByName[id]?.managementPath} />
            <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 8, marginTop: 'auto', paddingTop: 8, borderTop: '1px solid var(--border)' }}>
              {run.job && <Link className="btn sm ghost" to={`/security/assessments/${run.job.id}`}>Open results</Link>}
              <Btn variant="pri" onClick={onClose}>Done</Btn>
            </div>
          </>
        )}
      </div>
    </div>
  )
}

function Steps({ step }: { step: Step }) {
  const steps: { k: Step; label: string }[] = [{ k: 'dry', label: 'Dry run' }, { k: 'preview', label: 'Preview' }, { k: 'confirm', label: 'Confirm' }, { k: 'run', label: 'Write & verify' }]
  const idx = steps.findIndex((s) => s.k === step)
  return (
    <ol style={{ display: 'flex', flexWrap: 'wrap', gap: 6, listStyle: 'none', padding: 0, margin: 0 }} aria-label="Remediation steps">
      {steps.map((s, i) => <li key={s.k} className={`tag${i === idx ? ' up' : ''}`} aria-current={i === idx ? 'step' : undefined} style={i < idx ? { color: 'var(--ok)' } : undefined}>{i < idx ? <Ic.check size={11} /> : <span className="num">{i + 1}</span>} {s.label}</li>)}
    </ol>
  )
}

function summarise(results: JobResult[]) {
  const out = { wouldWrite: 0, written: 0, unchanged: 0, skipped: 0, failed: 0, mgmtSkips: 0, noDriverSkips: 0, otherSkips: 0, errors: 0, devicesChanged: 0, itemNames: [] as string[], paths: [] as string[] }
  const names = new Set<string>(), paths = new Set<string>()
  for (const r of results) {
    if (r.status === 'error' && !r.data) { out.errors++; continue }
    const d = r.data as unknown as RemediationData | undefined
    if (!d?.items) continue
    let changed = false
    for (const it of d.items) {
      switch (it.status) {
        case 'would_write': out.wouldWrite++; changed = true; names.add(it.name); break
        case 'written': out.written++; changed = true; names.add(it.name); break
        case 'unchanged': out.unchanged++; break
        case 'failed': out.failed++; break
        default:
          out.skipped++
          if (isMgmtPathSkip(it.message)) { out.mgmtSkips++; const m = /: (\w+) is the only/.exec(it.message || ''); if (m) paths.add(m[1].toLowerCase()) }
          else if (/no driver|no writer|unsupported/i.test(it.message || '')) out.noDriverSkips++
          else out.otherSkips++
      }
    }
    if (changed) out.devicesChanged++
  }
  out.itemNames = Array.from(names)
  out.paths = Array.from(paths)
  return out
}

/** Per-device / per-item remediation rows. Secrets never appear: the API redacts them before they reach us. */
export function ResultsTable({ results, byDevice, mgmtPathOf }: { results: JobResult[]; byDevice: (id?: string) => Device | undefined; mgmtPathOf?: (itemId: string) => string | undefined }) {
  if (results.length === 0) return <div className="cap">No per-device results yet.</div>
  return (
    <div className="tw" style={{ border: '1px solid var(--border)', borderRadius: 8 }}>
      <table className="tbl" style={{ minWidth: 720 }}>
        <thead><tr><th>Device</th><th>Item</th><th>Outcome</th><th>Change</th><th>Driver</th><th>Note</th></tr></thead>
        <tbody>
          {results.map((r) => {
            const dev = byDevice(r.deviceId)
            const name = dev ? deviceName(dev) : r.address || r.deviceId?.slice(0, 8) || '—'
            const d = r.data as unknown as RemediationData | undefined
            const items: RemediationItem[] = d?.items || []
            if (items.length === 0) return (
              <tr key={r.id}><td style={{ fontWeight: 500 }}>{r.deviceId ? <Link to={`/devices/${r.deviceId}`}>{name}</Link> : name}</td><td className="cap">—</td><td><RemediationPill status={r.status === 'error' ? 'failed' : 'skipped'} /></td><td /><td /><td className="wrap cap">{r.message}</td></tr>
            )
            return items.map((it, i) => (
              <tr key={`${r.id}-${it.itemId}`} className={it.status === 'skipped' || it.status === 'unchanged' ? 'dim' : ''}>
                <td style={{ fontWeight: 500 }}>{i === 0 ? (r.deviceId ? <Link to={`/devices/${r.deviceId}`}>{name}</Link> : name) : ''}</td>
                <td><span style={{ display: 'inline-flex', alignItems: 'center', gap: 6 }}>{it.name}{mgmtPathOf?.(it.itemId) && <MgmtPathTag path={mgmtPathOf(it.itemId)} xs />}</span><div className="cap mono" style={{ fontSize: 11 }}>{it.key}</div></td>
                <td><RemediationPill status={it.status} /></td>
                <td>{it.status === 'would_write' || it.status === 'written' ? <span style={{ display: 'inline-flex', gap: 6, alignItems: 'center' }}><span className="old mono">{fmtVal(it.before)}</span><Ic.arrowRight size={14} style={{ color: 'var(--text-3)' }} /><span className="new-v mono">{fmtVal(it.after)}</span></span> : it.status === 'unchanged' ? <span className="cap">already compliant</span> : <span className="cap">—</span>}</td>
                <td className="cap">{it.driver ? <Tag>{it.driver}</Tag> : '—'}</td>
                <td className="wrap cap" style={{ color: isMgmtPathSkip(it.message) ? 'var(--warn)' : undefined }}>{it.message || ''}</td>
              </tr>
            ))
          })}
        </tbody>
      </table>
    </div>
  )
}

