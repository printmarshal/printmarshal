import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Link, useLocation, useNavigate, useParams } from 'react-router-dom'
import { api, ago, fmtTime } from '../api'
import type { Device, Group, User } from '../types'
import type { CampaignDevice, CampaignDeviceRow, CampaignDeviceStatus, CampaignSummary, CampaignView, FirmwarePackage } from '../types-firmware'
import { canaryLabel, checkCompatibility, fmtBytes, isActive, isFinished, shortSha, windowLabel } from '../types-firmware'
import { useEvents } from '../ws'
import { Ic } from '../components/icons'
import { Banner, Btn, Card, ChipSelect, EmptyState, Kpi, Pill, SearchBox, Seg, Tag } from '../components/ui'
import { fwApi } from '../components/firmware/api'
import { CampaignBar, CampaignPill, ConfirmDialog, DeviceStatusPill, DryRunBanner, Legend, RecoveryNotes, summarize } from '../components/firmware/shared'
import CampaignWizard, { type WizardPreset } from '../components/firmware/CampaignWizard'

type Gate = 'start' | 'confirm' | 'pause' | 'resume' | 'cancel' | 'delete'
type Filter = 'all' | 'inprogress' | 'failed' | 'succeeded' | 'pending' | 'skipped'

export default function FirmwareCampaign({ user }: { user: User }) {
  const { id = '' } = useParams()
  const navigate = useNavigate()
  const location = useLocation()
  const nav = (location.state || {}) as { dryRun?: boolean; created?: boolean }
  const [c, setC] = useState<CampaignView | null>(null)
  const [rows, setRows] = useState<CampaignDeviceRow[]>([])
  const [pkg, setPkg] = useState<FirmwarePackage | null>(null)
  const [group, setGroup] = useState<Group | null>(null)
  const [targets, setTargets] = useState<Device[] | null>(null)
  const [error, setError] = useState('')
  const [gate, setGate] = useState<Gate | null>(null)
  const [clone, setClone] = useState<WizardPreset | null>(null)
  const [startDry, setStartDry] = useState(nav.dryRun ?? true)
  const [busy, setBusy] = useState(false)
  const [gateErr, setGateErr] = useState('')
  const [note, setNote] = useState<{ tone: 'ok' | 'info' | 'warn' | 'crit'; text: string } | null>(null)
  const [filter, setFilter] = useState<Filter>('all')
  const [wave, setWave] = useState('')
  const [q, setQ] = useState('')
  const [now, setNow] = useState(Date.now())
  const [showCreated, setShowCreated] = useState(!!nav.created)
  const canOperate = user.role !== 'viewer'

  const load = useCallback(() => {
    fwApi.campaign(id).then((r) => { setC(r.campaign); setRows(r.devices); setError('') }).catch((e) => setError((e as Error).message))
  }, [id])
  useEffect(() => { load() }, [load])
  useEffect(() => { if (c?.packageId) fwApi.package(c.packageId).then(setPkg).catch(() => setPkg(null)) }, [c?.packageId])
  useEffect(() => { if (c?.groupId) api.get<Group>(`/api/groups/${c.groupId}`).then(setGroup).catch(() => setGroup(null)); else setGroup(null) }, [c?.groupId])
  // For drafts the server has not planned yet: resolve the targets so the Start gate can state the blast radius.
  useEffect(() => {
    if (!c || c.status !== 'draft') { setTargets(null); return }
    const path = c.groupId ? `/api/devices?group=${encodeURIComponent(c.groupId)}` : '/api/devices'
    api.get<{ devices: Device[] }>(path).then((r) => setTargets(r.devices.filter((d) => d.state !== 'decommissioned' && (c.groupId || (c.deviceIds || []).includes(d.id))))).catch(() => setTargets([]))
  }, [c?.status, c?.groupId, c?.deviceIds]) // eslint-disable-line react-hooks/exhaustive-deps

  // Live updates: merge device rows immediately, then reload the view (summary, status) shortly after.
  const timer = useRef<number | null>(null)
  const scheduleReload = useCallback(() => { if (timer.current) window.clearTimeout(timer.current); timer.current = window.setTimeout(load, 250) }, [load])
  useEvents((e) => {
    if (e.type === 'firmware.campaign.updated' && (e.data as { id?: string })?.id === id) scheduleReload()
    else if (e.type === 'firmware.device.updated' && (e.data as CampaignDevice)?.campaignId === id) {
      const d = e.data as CampaignDevice
      setRows((rs) => rs.some((r) => r.deviceId === d.deviceId) ? rs.map((r) => r.deviceId === d.deviceId ? { ...r, ...d } : r) : rs)
      scheduleReload()
    } else if (e.type === 'job.updated' && c?.jobId && (e.data as { id?: string })?.id === c.jobId) scheduleReload()
  }, [id, c?.jobId, scheduleReload])
  useEffect(() => { if (!c || !isActive(c)) return; const t = window.setInterval(() => setNow(Date.now()), 5000); return () => window.clearInterval(t) }, [c?.status]) // eslint-disable-line react-hooks/exhaustive-deps

  const summary: CampaignSummary = c?.summary || summarize(rows)
  const w1 = useMemo(() => summarize(rows.filter((r) => r.wave === 1)), [rows])
  const w2 = useMemo(() => summarize(rows.filter((r) => r.wave === 2)), [rows])
  const verdicts = useMemo(() => { const m = new Map<string, ReturnType<typeof checkCompatibility>>(); if (pkg && targets) for (const d of targets) m.set(d.id, checkCompatibility(pkg, d, !!c?.allowDowngrade)); return m }, [pkg, targets, c?.allowDowngrade])
  const eligiblePreview = targets ? targets.filter((d) => { const v = verdicts.get(d.id); return !!v && v.compatible && !v.current }).length : null
  const attempted = summary.succeeded + summary.failed
  const failRate = attempted > 0 ? Math.round((summary.failed / attempted) * 1000) / 10 : 0
  const inflight = summary.updating + summary.verifying

  const run = async (what: Gate) => {
    if (!c) return
    setBusy(true); setGateErr('')
    try {
      let v: CampaignView | undefined
      switch (what) {
        case 'start': v = await fwApi.start(c.id, startDry); setNote({ tone: 'ok', text: startDry ? 'Dry run started — nothing is delivered.' : 'Campaign started. Wave 1 (canary) is being planned and dispatched.' }); break
        case 'confirm': v = await fwApi.confirm(c.id); setNote({ tone: 'ok', text: 'Wave 2 released.' }); break
        case 'pause': v = await fwApi.pause(c.id); setNote({ tone: 'info', text: 'Paused. Devices already updating will finish; nothing new is dispatched.' }); break
        case 'resume': v = await fwApi.resume(c.id); setNote({ tone: 'ok', text: 'Resumed.' }); break
        case 'cancel': v = await fwApi.cancel(c.id); setNote({ tone: 'warn', text: 'Cancelled. Pending devices are marked skipped; devices mid-update finish and are verified.' }); break
        case 'delete': await fwApi.deleteCampaign(c.id); navigate('/firmware?tab=campaigns', { replace: true }); return
      }
      if (v) setC(v)
      setGate(null); setShowCreated(false); load()
    } catch (e) { setGateErr((e as Error).message) } finally { setBusy(false) }
  }

  if (error && !c) return <><nav aria-label="Breadcrumb" className="cap"><Link to="/firmware?tab=campaigns">Firmware</Link> / Campaigns</nav><div className="state"><span className="ring" style={{ background: 'var(--crit-bg)', color: 'var(--crit)' }}><Ic.warn size={20} /></span><b>Couldn’t load this campaign</b><span className="cap">{error}</span><Btn sm onClick={load}>Retry</Btn></div></>
  if (!c) return <div className="cap" aria-busy="true">Loading…</div>

  const targetLabel = c.groupId ? `Group “${group?.name || c.groupId}”${group ? ` · ${group.kind === 'rule' ? 'rule-based' : 'static'}` : ''}` : `${c.deviceIds?.length || 0} device${c.deviceIds?.length === 1 ? '' : 's'}`
  const pkgLabel = pkg ? `${pkg.vendor} ${pkg.version}` : c.packageVersion || c.packageId
  const started = c.status !== 'draft'
  const finished = isFinished(c)
  const pendingCount = summary.pending
  const targetedCount = started ? summary.total : (targets?.length ?? 0)
  const filtered = rows.filter((r) => {
    if (filter === 'inprogress' && !(r.status === 'updating' || r.status === 'verifying')) return false
    if (filter !== 'all' && filter !== 'inprogress' && r.status !== filter) return false
    if (wave && String(r.wave) !== wave) return false
    if (q && !`${r.name} ${r.address} ${r.model} ${r.message || ''} ${r.fromVersion || ''}`.toLowerCase().includes(q.toLowerCase())) return false
    return true
  })
  const count = (f: Filter) => f === 'all' ? rows.length : f === 'inprogress' ? inflight : summary[f as CampaignDeviceStatus]
  const elapsed = (r: CampaignDeviceRow) => {
    if (!r.startedAt) return '—'
    const end = r.finishedAt ? new Date(r.finishedAt).getTime() : now
    const s = Math.max(0, Math.round((end - new Date(r.startedAt).getTime()) / 1000))
    return s < 60 ? `${s} s` : s < 3600 ? `${Math.floor(s / 60)} m ${s % 60} s` : `${Math.floor(s / 3600)} h ${Math.floor((s % 3600) / 60)} m`
  }

  const toolbar = canOperate ? (
    <div className="row" role="toolbar" aria-label="Campaign gates">
      {c.status === 'draft' && <>
        <Btn variant="dng" sm onClick={() => { setGate('delete'); setGateErr('') }}>Delete draft</Btn>
        <Btn onClick={() => { setStartDry(true); setGate('start'); setGateErr('') }}><Ic.play />Start dry run…</Btn>
        <Btn variant="wr" onClick={() => { setStartDry(false); setGate('start'); setGateErr('') }}><Ic.upload />Start campaign…</Btn>
      </>}
      {c.status === 'paused' && c.pauseReason === 'canary' && <Btn variant={c.dryRun ? 'pri' : 'wr'} onClick={() => { setGate('confirm'); setGateErr('') }}><Ic.shieldCheck />Confirm wave 2…</Btn>}
      {c.status === 'paused' && c.pauseReason !== 'canary' && <Btn variant={c.dryRun ? 'pri' : 'wr'} onClick={() => { setGate('resume'); setGateErr('') }}><Ic.play />Resume campaign…</Btn>}
      {(c.status === 'running' || c.status === 'scheduled') && <Btn onClick={() => { setGate('pause'); setGateErr('') }}><Ic.pause />Pause campaign…</Btn>}
      {isActive(c) && <Btn variant="dng" onClick={() => { setGate('cancel'); setGateErr('') }}><Ic.x />Cancel remaining…</Btn>}
      {finished && <Btn variant="dng" sm onClick={() => { setGate('delete'); setGateErr('') }}>Delete campaign</Btn>}
      {finished && <Btn onClick={() => setClone({ name: c.dryRun ? c.name.replace(/\s*\(dry run\)$/i, '') : `${c.name} (again)`, description: c.description, packageId: c.packageId, groupId: c.groupId, deviceIds: c.deviceIds, canaryCount: c.canaryCount, concurrency: c.concurrency, failureThresholdPercent: c.failureThresholdPercent, window: c.window, scheduledAt: c.scheduledAt, verifyTimeoutSecs: c.verifyTimeoutSecs, allowDowngrade: c.allowDowngrade, dryRun: !c.dryRun })}><Ic.plus />{c.dryRun ? 'Run for real…' : 'Clone as new campaign…'}</Btn>}
    </div>
  ) : <Tag>Read-only · Viewer</Tag>

  return (
    <>
      <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'center', justifyContent: 'space-between', gap: 8 }}>
        <nav aria-label="Breadcrumb" className="cap"><Link to="/firmware">Firmware</Link> / <Link to="/firmware?tab=campaigns">Campaigns</Link> / <span style={{ color: 'var(--text-2)' }}>{c.name}</span></nav>
        <Btn sm variant="ghost" onClick={() => navigate('/firmware?tab=campaigns')}><Ic.chevronLeft />All campaigns</Btn>
      </div>
      <div className="ph">
        <div style={{ display: 'flex', flexDirection: 'column', gap: 6, minWidth: 0 }}>
          <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'center', gap: 10 }}><h1 className="h1">{c.name}</h1><CampaignPill c={c} />{c.dryRun && started && <Tag up>Dry run</Tag>}{c.allowDowngrade && <span className="pill warn xs" title="Older packages may be delivered">downgrade allowed</span>}</div>
          <div className="cap" style={{ display: 'flex', flexWrap: 'wrap', gap: '4px 14px', color: 'var(--text-2)' }}>
            <span>Package <span className="mono">{pkgLabel}</span>{pkg && pkg.models.length > 0 && <> for <span className="mono">{pkg.models.join(', ')}</span></>}</span>
            <span><Ic.arrowRight size={12} /> {targetLabel}</span>
            {started ? <span className="num">Started {fmtTime(c.startedAt)} by {c.startedBy || c.createdBy}{c.finishedAt ? ` · finished ${ago(c.finishedAt)}` : ''}</span> : <span className="num">Draft by {c.createdBy} · {ago(c.createdAt)}</span>}
            {started && <span className="num">{summary.total} device{summary.total === 1 ? '' : 's'} planned</span>}
          </div>
          {c.description && <div className="cap">{c.description}</div>}
        </div>
        {toolbar}
      </div>

      {showCreated && c.status === 'draft' && <Banner tone="accent" right={<Btn sm variant="ghost" ic onClick={() => setShowCreated(false)} aria-label="Dismiss"><Ic.x /></Btn>}><b>Draft saved.</b> Nothing has been sent. {canOperate ? 'Start a dry run first to validate planning and pre-flight, then start the campaign for real from this page.' : ''}</Banner>}
      {note && <Banner tone={note.tone} right={<Btn sm variant="ghost" ic onClick={() => setNote(null)} aria-label="Dismiss"><Ic.x /></Btn>}>{note.text}</Banner>}
      {c.dryRun && started && <DryRunBanner started />}
      {c.dryRun && finished && canOperate && <Banner tone="accent" right={<Btn sm variant="pri" onClick={() => setClone({ name: c.name, description: c.description, packageId: c.packageId, groupId: c.groupId, deviceIds: c.deviceIds, canaryCount: c.canaryCount, concurrency: c.concurrency, failureThresholdPercent: c.failureThresholdPercent, window: c.window, scheduledAt: c.scheduledAt, verifyTimeoutSecs: c.verifyTimeoutSecs, allowDowngrade: c.allowDowngrade, dryRun: false })}><Ic.plus />Run for real…</Btn>}>A dry run uses up its campaign. To deliver, create the real campaign with the same settings — it is saved as a draft and goes through the Start gate again.</Banner>}
      {c.error && <Banner tone="crit" role="alert"><b style={{ color: 'var(--crit)' }}>Campaign error</b> — {c.error}</Banner>}
      {c.status === 'paused' && c.pauseReason === 'canary' && <Banner tone="info" right={canOperate ? <Btn sm variant={c.dryRun ? 'pri' : 'wr'} onClick={() => setGate('confirm')}>Confirm wave 2…</Btn> : undefined}><b style={{ color: 'var(--info)' }}>Canary wave finished</b> — {w1.succeeded} succeeded, {w1.failed} failed, {w1.skipped} skipped. Wave 2 ({w2.total} device{w2.total === 1 ? '' : 's'}) waits for your explicit confirmation; resume cannot bypass this gate.</Banner>}
      {c.status === 'paused' && c.pauseReason === 'threshold' && <Banner tone="crit" role="alert"><b style={{ color: 'var(--crit)' }}>Auto-paused: failure threshold exceeded</b> — {summary.failed} of {attempted} attempted devices failed ({failRate} % &gt; {c.failureThresholdPercent} %). Review the failed devices and the recovery notes before resuming or cancel the remaining {pendingCount}.</Banner>}
      {c.status === 'paused' && c.pauseReason === 'interrupted' && <Banner tone="warn" role="alert"><b style={{ color: 'var(--warn)' }}>Interrupted by a server restart</b> — devices that were mid-update are marked failed with “verify the device manually”. Nothing restarts on its own: check those devices, then resume or cancel.</Banner>}
      {c.status === 'paused' && c.pauseReason === 'operator' && <Banner tone="warn"><b style={{ color: 'var(--warn)' }}>Paused by an operator</b> — {pendingCount} pending device{pendingCount === 1 ? '' : 's'} held{inflight > 0 ? `; ${inflight} still finishing` : ''}.</Banner>}
      {c.status === 'scheduled' && <Banner tone="info"><b style={{ color: 'var(--info)' }}>Waiting</b> — {c.scheduledAt && new Date(c.scheduledAt).getTime() > now ? `starts no earlier than ${fmtTime(c.scheduledAt)}` : 'outside the maintenance window'}{c.window ? ` · window ${windowLabel(c.window)}` : ''}. Re-checked every minute.</Banner>}
      {c.status === 'running' && !c.dryRun && (
        <div role="note" className="banner warn" style={{ alignItems: 'flex-start' }}>
          <Ic.warn style={{ color: 'var(--warn)', marginTop: 1 }} />
          <div style={{ flex: '1 1 400px' }}><b style={{ color: 'var(--warn)' }}>Devices reboot while installing and cannot print.</b> Pausing or cancelling stops <i>new</i> deliveries only — the {inflight} device{inflight === 1 ? '' : 's'} already writing will finish and be verified.{c.failureThresholdPercent > 0 ? <> Auto-pause triggers when the failure rate passes {c.failureThresholdPercent} % (now <b className="num">{failRate} %</b>).</> : ' No failure threshold is set.'}</div>
        </div>
      )}

      <section className="card" style={{ display: 'flex', flexWrap: 'wrap' }} aria-label="Rollout waves">
        <Stage n={1} title="Canary" s={w1} current={c.wave === 1 && isActive(c)} c={c} planned={!started} note={started ? (c.canaryCount === 0 ? 'No canary — every device is in wave 1' : `${w1.total} device${w1.total === 1 ? '' : 's'} · pauses for confirmation when done`) : canaryLabel(c.canaryCount, eligiblePreview ?? undefined)} />
        {(c.canaryCount !== 0 || !started) && <Stage n={2} title="Remainder" s={w2} current={c.wave === 2 && isActive(c)} c={c} planned={!started} note={started ? `${w2.total} device${w2.total === 1 ? '' : 's'} · ${c.confirmed ? 'released by operator' : 'needs explicit confirmation'}` : 'released only after you confirm the canary outcome'} />}
        <div style={{ ...stageStyle, flex: '1 1 200px', borderRight: 0 }}>
          <div style={{ display: 'flex', justifyContent: 'space-between', gap: 8 }}><b>Overall</b><span className="num cap" style={{ fontWeight: 600 }}>{summary.succeeded + summary.failed + summary.skipped} / {summary.total}</span></div>
          <CampaignBar s={summary} />
          <Legend s={summary} compact />
        </div>
      </section>

      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(130px, 1fr))', gap: 12 }}>
        <Kpi label="Targeted" value={started ? summary.total : targets === null ? '—' : targetedCount} detail={started ? 'planned at start' : eligiblePreview !== null ? `${eligiblePreview} look compatible` : 'resolving…'} />
        <Kpi label="Succeeded" value={summary.succeeded} color={summary.succeeded > 0 ? 'var(--ok)' : undefined} detail="verified by polling" />
        <Kpi label="Failed" value={summary.failed} color={summary.failed > 0 ? 'var(--crit)' : undefined} detail={attempted > 0 ? `${failRate} % of attempted` : '—'} />
        <Kpi label="In progress" value={inflight} color={inflight > 0 ? 'var(--accent)' : undefined} detail={`${summary.updating} updating · ${summary.verifying} verifying`} />
        <Kpi label="Pending" value={summary.pending} detail={c.status === 'paused' ? 'held' : `${c.concurrency} at a time`} />
        <Kpi label="Skipped" value={summary.skipped} detail="incompatible · current · cancelled" />
      </div>

      <div className="cols">
        <Card style={{ flex: '3 1 620px' }} title="Per-device progress" id="pd" sub={started ? 'live · pending → updating → verifying → succeeded | failed · skipped' : 'devices are planned when the campaign starts'}
          right={<Seg sm label="Filter devices" value={filter} onChange={setFilter} options={[
            { v: 'all', label: <>All <span className="num cap">{count('all')}</span></> },
            { v: 'inprogress', label: <><span className="dot" style={{ background: 'var(--accent)' }} />In progress <span className="num">{count('inprogress')}</span></> },
            { v: 'failed', label: <><span className="dot" style={{ background: 'var(--crit)' }} />Failed <span className="num">{count('failed')}</span></> },
            { v: 'succeeded', label: <><span className="dot" style={{ background: 'var(--ok)' }} />Succeeded <span className="num">{count('succeeded')}</span></> },
            { v: 'pending', label: <>Pending <span className="num">{count('pending')}</span></> },
            { v: 'skipped', label: <>Skipped <span className="num">{count('skipped')}</span></> },
          ]} />}>
          <div className="row" style={{ padding: '8px 12px', borderBottom: '1px solid var(--border)' }}>
            <SearchBox sm filter value={q} onChange={setQ} placeholder="Filter by device, address, model, message…" />
            <ChipSelect label="Wave" value={wave} onChange={setWave} options={[{ v: '1', label: '1 · canary' }, { v: '2', label: '2 · remainder' }]} />
          </div>
          {!started ? (
            <div className="cb">{targets === null ? <div className="cap" aria-busy="true">Resolving targets…</div> : targets.length === 0 ? <EmptyState icon={<Ic.printer size={20} />} title="No targets" text="The group is empty or the devices no longer exist." /> : (
              <div className="tw"><table className="tbl" style={{ minWidth: 640 }}>
                <thead><tr><th>Device</th><th>Model</th><th>Installed</th><th>Preview</th></tr></thead>
                <tbody>{targets.map((d) => { const v = verdicts.get(d.id); return (
                  <tr key={d.id}><td><Link to={`/devices/${d.id}`} style={{ fontWeight: 500 }}>{d.displayName || d.sysName || d.hostname || d.address}</Link><div className="cap mono" style={{ fontSize: 11 }}>{d.address}</div></td><td className="mono">{d.model || '—'}</td><td className="mono">{d.firmware || <span className="cap">unknown</span>}</td>
                    <td className="wrap">{!pkg ? <span className="cap">—</span> : v?.compatible && !v.current ? <Pill tone="ok" xs>{v.downgrade ? 'Downgrade' : 'Will update'}</Pill> : <span style={{ display: 'inline-flex', gap: 6, alignItems: 'center' }}><span className="pill off xs">Skip</span><span className="cap">{v?.reason}</span></span>}</td></tr>
                ) })}</tbody>
              </table></div>
            )}<div className="cap" style={{ marginTop: 8 }}>Preview only — the server re-checks vendor, model, minimum version and the installed version at start and before every delivery.</div></div>
          ) : rows.length === 0 ? <div className="cb cap" aria-busy="true">Planning…</div>
            : filtered.length === 0 ? <div className="cb cap">No devices match this filter.</div>
            : (
              <div className="tw" style={{ maxHeight: 640, overflow: 'auto' }}><table className="tbl" style={{ minWidth: 900 }}>
                <thead><tr><th>Device</th><th>Wave</th><th>Status</th><th>Version</th><th>Message</th><th>Started</th><th className="r">Elapsed</th></tr></thead>
                <tbody>
                  {filtered.map((r) => (
                    <tr key={r.deviceId} className={r.status === 'skipped' ? 'dim' : ''}>
                      <td><Link to={`/devices/${r.deviceId}`} style={{ fontWeight: 500 }}>{r.name}</Link><div className="cap" style={{ fontSize: 11 }}>{r.model}{r.model && r.address ? ' · ' : ''}<span className="mono">{r.address}</span></div></td>
                      <td className="num">{r.wave || '—'}</td>
                      <td><DeviceStatusPill status={r.status} /></td>
                      <td className="mono">{r.fromVersion || '?'}{r.toVersion ? <> <Ic.arrowRight size={12} /> {r.toVersion}</> : r.status === 'succeeded' ? <> <Ic.arrowRight size={12} /> {c.packageVersion || pkg?.version}</> : null}</td>
                      <td className="wrap cap" style={{ color: r.status === 'failed' ? 'var(--crit)' : 'var(--text-2)', minWidth: 220 }}>{r.message || '—'}</td>
                      <td className="cap num" title={fmtTime(r.startedAt)}>{r.startedAt ? ago(r.startedAt) : '—'}</td>
                      <td className="cap num r">{elapsed(r)}</td>
                    </tr>
                  ))}
                </tbody>
              </table></div>
            )}
        </Card>
        <div className="stack" style={{ flex: '1 1 320px' }}>
          <Card title="Campaign settings" id="cs" sub={started ? 'locked once started' : 'draft · editable until started'}>
            <div style={{ display: 'grid', gridTemplateColumns: '110px minmax(0, 1fr)', gap: '8px 12px', padding: '12px 16px', fontSize: 13 }}>
              <span className="cap">Package</span><span><span className="mono">{pkgLabel}</span>{pkg && <div className="cap mono" style={{ fontSize: 11 }} title={pkg.sha256}>sha256 {shortSha(pkg.sha256)} · {fmtBytes(pkg.size)} · {pkg.fileName}</div>}{pkg?.minVersion && <div className="cap">requires ≥ <span className="mono">{pkg.minVersion}</span></div>}</span>
              <span className="cap">Target</span><span>{c.groupId ? <Link to={`/devices?group=${c.groupId}`}>{targetLabel}</Link> : targetLabel}</span>
              <span className="cap">Canary</span><span>{canaryLabel(c.canaryCount, started ? undefined : eligiblePreview ?? undefined)}{started && w1.total > 0 && c.canaryCount !== 0 ? ` · ${w1.total} planned` : ''}</span>
              <span className="cap">Concurrency</span><span className="num">{c.concurrency} device{c.concurrency === 1 ? '' : 's'} at once</span>
              <span className="cap">Auto-pause</span><span>{c.failureThresholdPercent > 0 ? `failure rate > ${c.failureThresholdPercent} %` : 'never'}</span>
              <span className="cap">Window</span><span>{windowLabel(c.window)}</span>
              <span className="cap">Schedule</span><span className="num">{c.scheduledAt ? fmtTime(c.scheduledAt) : 'when started'}</span>
              <span className="cap">Verify timeout</span><span className="num">{c.verifyTimeoutSecs ? `${Math.round(c.verifyTimeoutSecs / 60)} min` : '15 min (default)'}</span>
              <span className="cap">Downgrade</span><span>{c.allowDowngrade ? <span style={{ color: 'var(--warn)' }}>allowed</span> : 'not allowed'}</span>
              <span className="cap">Mode</span><span>{started ? (c.dryRun ? 'Dry run — nothing delivered' : 'Real delivery') : 'decided at the Start gate'}</span>
              <span className="cap">Created</span><span className="num">{c.createdBy} · {fmtTime(c.createdAt)}</span>
              {started && <><span className="cap">Started</span><span className="num">{c.startedBy || '—'} · {fmtTime(c.startedAt)}</span></>}
              {c.finishedAt && <><span className="cap">Finished</span><span className="num">{fmtTime(c.finishedAt)}</span></>}
              {c.jobId && <><span className="cap">Job</span><span><Link to={`/jobs/${c.jobId}`} className="mono">{c.jobId.slice(0, 8)}</Link></span></>}
            </div>
          </Card>
          <Card title="If an update goes wrong" id="rn" sub="rollback & recovery guidance for this package's vendor">
            <RecoveryNotes text={c.recoveryNotes} />
          </Card>
        </div>
      </div>

      {clone && <CampaignWizard preset={clone} onClose={() => setClone(null)} onCreated={(n, o) => { setClone(null); navigate(`/firmware/campaigns/${n.id}`, { state: { dryRun: o.dryRun, created: true } }) }} />}
      {gate === 'start' && (
        <ConfirmDialog title={startDry ? `Start a dry run of “${c.name}”?` : `Start campaign “${c.name}”?`} sub={<>{pkgLabel} → {targetLabel}</>} tone={startDry ? 'info' : 'warn'} dryRun={startDry}
          typed={c.name.toUpperCase()} count={eligiblePreview ?? targetedCount} busy={busy} error={gateErr} onConfirm={() => run('start')} onClose={() => setGate(null)}
          confirmLabel={startDry ? 'Start dry run' : `Deliver to ${eligiblePreview ?? targetedCount} device${(eligiblePreview ?? targetedCount) === 1 ? '' : 's'}`} confirmVariant={startDry ? 'pri' : 'dngf'} confirmIcon={startDry ? <Ic.play /> : <Ic.upload />}
          rows={[
            { label: 'Devices targeted', value: <>{targetedCount}{eligiblePreview !== null && <span className="cap"> · {eligiblePreview} look compatible, {targetedCount - eligiblePreview} will be skipped</span>}</> },
            { label: 'Canary wave', value: canaryLabel(c.canaryCount, eligiblePreview ?? undefined) },
            { label: 'Concurrency', value: `${c.concurrency} at a time` },
            { label: 'Auto-pause', value: c.failureThresholdPercent > 0 ? `failure rate > ${c.failureThresholdPercent} %` : 'never' },
            { label: 'Window', value: windowLabel(c.window) },
            { label: 'Schedule', value: c.scheduledAt ? fmtTime(c.scheduledAt) : 'now' },
            { label: 'Reboot / print outage', value: startDry ? <span style={{ color: 'var(--ok)' }}>None — nothing is delivered</span> : <span style={{ color: 'var(--warn)' }}>Each device reboots during install</span> },
            ...(c.allowDowngrade ? [{ label: 'Downgrade', value: <span style={{ color: 'var(--warn)' }}>allowed</span> }] : []),
          ]}>
          <label className={`meth${startDry ? ' on' : ''}`} style={{ marginTop: 10 }}><input className="cbx" type="checkbox" checked={startDry} onChange={(e) => setStartDry(e.target.checked)} style={{ marginTop: 2 }} /><span><b style={{ fontWeight: 600 }}>Dry run</b><span className="cap" style={{ display: 'block' }}>Plan, pre-flight and validate drivers without sending an image. No audit entry, no device change.</span></span></label>
          {!startDry && <div className="cap" style={{ marginTop: 8 }}>Wave 1 starts now; wave 2 waits for a second confirmation after the canary. Every real delivery is audited.</div>}
        </ConfirmDialog>
      )}
      {gate === 'confirm' && (
        <ConfirmDialog title={`Release wave 2 of “${c.name}”?`} sub={<>Canary outcome · {pkgLabel}</>} tone={c.dryRun ? 'info' : 'warn'} dryRun={c.dryRun} typed={c.name.toUpperCase()} count={w2.pending}
          busy={busy} error={gateErr} onConfirm={() => run('confirm')} onClose={() => setGate(null)}
          confirmLabel={c.dryRun ? `Release wave 2 (dry run)` : `Deliver to ${w2.pending} device${w2.pending === 1 ? '' : 's'}`} confirmVariant={c.dryRun ? 'pri' : 'dngf'} confirmIcon={<Ic.upload />}
          rows={[
            { label: 'Canary succeeded', value: <span style={{ color: w1.succeeded > 0 ? 'var(--ok)' : undefined }}>{w1.succeeded} of {w1.total}</span> },
            { label: 'Canary failed', value: <span style={{ color: w1.failed > 0 ? 'var(--crit)' : undefined }}>{w1.failed}</span> },
            { label: 'Canary skipped', value: w1.skipped },
            { label: 'Wave 2', value: `${w2.pending} pending device${w2.pending === 1 ? '' : 's'} · ${c.concurrency} at a time` },
            { label: 'Auto-pause', value: c.failureThresholdPercent > 0 ? `failure rate > ${c.failureThresholdPercent} %` : 'never' },
          ]}>
          {w1.failed > 0 && <div style={{ marginTop: 8, border: '1px solid var(--crit)', borderRadius: 6, background: 'var(--crit-bg)', padding: '8px 10px', fontSize: 12 }}><b style={{ color: 'var(--crit)' }}>Canary failures</b><ul style={{ margin: '4px 0 0', paddingLeft: 18 }}>{rows.filter((r) => r.wave === 1 && r.status === 'failed').slice(0, 5).map((r) => <li key={r.deviceId}>{r.name} — {r.message || 'failed'}</li>)}</ul>{w1.failed > 5 && <div className="cap">…and {w1.failed - 5} more</div>}</div>}
          <div className="cap" style={{ marginTop: 8 }}>{c.dryRun ? 'Still a dry run: wave 2 is validated, nothing is delivered.' : 'This is the second gate. Review the canary devices before releasing the fleet.'}</div>
        </ConfirmDialog>
      )}
      {gate === 'pause' && (
        <ConfirmDialog title={`Pause “${c.name}”?`} tone="info" confirmLabel="Pause campaign" confirmVariant="wr" confirmIcon={<Ic.pause />} busy={busy} error={gateErr} onConfirm={() => run('pause')} onClose={() => setGate(null)}
          rows={[{ label: 'Held', value: `${pendingCount} pending device${pendingCount === 1 ? '' : 's'}` }, { label: 'Will finish', value: `${inflight} already updating or verifying` }]}>
          <div className="cap" style={{ marginTop: 8 }}>A write in progress is never interrupted. Resume later from this page.</div>
        </ConfirmDialog>
      )}
      {gate === 'resume' && (
        <ConfirmDialog title={`Resume “${c.name}”?`} sub={c.pauseReason === 'threshold' ? `Paused because ${failRate} % of attempted devices failed` : c.pauseReason === 'interrupted' ? 'Paused by a server restart' : 'Paused by an operator'} tone={c.dryRun ? 'info' : 'warn'} dryRun={c.dryRun}
          typed={c.name.toUpperCase()} count={pendingCount} confirmLabel={c.dryRun ? 'Resume dry run' : `Resume · ${pendingCount} pending`} confirmVariant={c.dryRun ? 'pri' : 'wr'} confirmIcon={<Ic.play />} busy={busy} error={gateErr} onConfirm={() => run('resume')} onClose={() => setGate(null)}
          rows={[{ label: 'Pending', value: `${pendingCount} device${pendingCount === 1 ? '' : 's'} will be dispatched, ${c.concurrency} at a time` }, { label: 'Failed so far', value: <span style={{ color: summary.failed > 0 ? 'var(--crit)' : undefined }}>{summary.failed}</span> }, { label: 'Auto-pause', value: c.failureThresholdPercent > 0 ? `failure rate > ${c.failureThresholdPercent} %` : 'never' }]}>
          {c.pauseReason === 'threshold' && <div className="cap" style={{ marginTop: 8, color: 'var(--warn)' }}>The threshold is still exceeded; the campaign pauses again after the next batch unless the failure rate drops.</div>}
          {c.pauseReason === 'canary' && <div className="cap" style={{ marginTop: 8 }}>Resume cannot bypass the canary gate — use Confirm wave 2.</div>}
        </ConfirmDialog>
      )}
      {gate === 'cancel' && (
        <ConfirmDialog title={`Cancel the remaining devices of “${c.name}”?`} tone="crit" confirmLabel={`Cancel remaining ${pendingCount}`} confirmVariant="dngf" confirmIcon={<Ic.x />} busy={busy} error={gateErr} onConfirm={() => run('cancel')} onClose={() => setGate(null)}
          rows={[{ label: 'Marked skipped', value: `${pendingCount} pending device${pendingCount === 1 ? '' : 's'}` }, { label: 'Will still finish', value: `${inflight} updating or verifying` }, { label: 'Already done', value: `${summary.succeeded} succeeded · ${summary.failed} failed` }]}>
          <div className="cap" style={{ marginTop: 8 }}>Cancelling is final for this campaign; create a new one to retry the skipped devices.</div>
        </ConfirmDialog>
      )}
      {gate === 'delete' && (
        <ConfirmDialog title={`Delete campaign “${c.name}”?`} tone="crit" confirmLabel="Delete campaign" confirmVariant="dngf" busy={busy} error={gateErr} onConfirm={() => run('delete')} onClose={() => setGate(null)}
          rows={[{ label: 'Status', value: c.status }, { label: 'Devices', value: started ? `${summary.total} rows of history are removed` : 'none touched' }]}>
          <div className="cap" style={{ marginTop: 8 }}>Audit entries and job results stay. Devices are not affected.</div>
        </ConfirmDialog>
      )}
    </>
  )
}

const stageStyle: React.CSSProperties = { flex: '1 1 220px', padding: '14px 16px', display: 'flex', flexDirection: 'column', gap: 8, borderRight: '1px solid var(--border)', minWidth: 0 }

function Stage({ n, title, s, current, c, planned, note }: { n: number; title: string; s: CampaignSummary; current: boolean; c: CampaignView; planned: boolean; note: string }) {
  const inflight = s.updating + s.verifying
  const done = s.succeeded + s.failed + s.skipped
  let pill: React.ReactNode
  if (planned) pill = <span className="pill off xs"><Ic.clock />Not planned</span>
  else if (s.total === 0) pill = <span className="pill off xs">Empty</span>
  else if (inflight > 0 || (current && c.status === 'running')) pill = <span className="pill info xs"><Ic.refresh />Running</span>
  else if (done === s.total) pill = s.failed > 0 && s.succeeded === 0 ? <Pill tone="crit" xs>Failed</Pill> : s.failed > 0 ? <Pill tone="warn" xs>{s.failed} failed</Pill> : <Pill tone="ok" xs>Passed</Pill>
  else if (n === 2 && c.status === 'paused' && c.pauseReason === 'canary') pill = <span className="pill info xs"><Ic.shieldCheck />Awaiting confirmation</span>
  else if (c.status === 'paused') pill = <span className="pill warn xs"><Ic.pause />Held</span>
  else if (c.status === 'cancelled') pill = <span className="pill off xs"><Ic.x />Cancelled</span>
  else pill = <span className="pill off xs"><Ic.clock />Waiting</span>
  return (
    <div style={{ ...stageStyle, background: current ? 'var(--accent-weak)' : undefined }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', gap: 8, alignItems: 'center' }}><b style={current ? { color: 'var(--accent)' } : undefined}>{n} · {title}</b>{pill}</div>
      <CampaignBar s={s} label={`Wave ${n}: ${s.succeeded} succeeded, ${s.failed} failed, ${inflight} in progress, ${s.pending} pending, ${s.skipped} skipped`} />
      <div className="cap num">{planned ? '—' : `${done} / ${s.total} done · ${s.succeeded} ok · ${s.failed} failed${s.skipped ? ` · ${s.skipped} skipped` : ''}`}</div>
      <div className="cap">{note}</div>
    </div>
  )
}
