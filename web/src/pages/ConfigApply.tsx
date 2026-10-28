import { useCallback, useEffect, useMemo, useState } from 'react'
import { Link, useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { api, fmtTime } from '../api'
import type { Device, Group, Job, User } from '../types'
import { deviceName } from '../types'
import { Ic } from '../components/icons'
import { Banner, Btn, Card, DevicePill, Dialog, JobBar, SearchBox, Seg } from '../components/ui'
import type { Profile } from '../types-config'
import { analyse, DeviceRows, SettingRows } from '../components/config/ApplyResults'
import RevertPanel, { type RevertTarget } from '../components/config/RevertPanel'
import { StatTile, Steps, ViewOnly, settingName, useCatalog, useJob } from '../components/config/shared'

type Kind = 'device' | 'group'
const STEPS = ['Target', 'Dry-run preview', 'Confirm', 'Results']

/**
 * Apply flow: target → dry run (no writes) → confirm (blast radius) → real apply → results.
 * Reachable as /profiles/:id/apply, optionally with ?device=<id> or ?group=<id> preselected.
 */
export default function ConfigApply({ user }: { user: User }) {
  const { id = '' } = useParams()
  const [params] = useSearchParams()
  const navigate = useNavigate()
  const canOperate = user.role !== 'viewer'
  const { byKey } = useCatalog()
  const [profile, setProfile] = useState<Profile | null>(null)
  const [devices, setDevices] = useState<Device[]>([])
  const [groups, setGroups] = useState<Group[]>([])
  const [error, setError] = useState('')
  const [kind, setKind] = useState<Kind>(params.get('group') ? 'group' : 'device')
  const [sel, setSel] = useState<string[]>(() => params.get('device') ? [params.get('device')!] : [])
  const [groupId, setGroupId] = useState(params.get('group') || '')
  const [groupDevices, setGroupDevices] = useState<Device[] | null>(null)
  const [q, setQ] = useState('')
  const [step, setStep] = useState(1)
  const [dryJobId, setDryJobId] = useState<string | null>(null)
  const [applyJobId, setApplyJobId] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)
  const [confirmText, setConfirmText] = useState('')
  const [revert, setRevert] = useState<RevertTarget[] | null>(null)
  const dry = useJob(dryJobId)
  const real = useJob(applyJobId)

  useEffect(() => {
    api.get<{ profile: Profile }>(`/api/profiles/${id}`).then((r) => setProfile(r.profile)).catch((e) => setError(e.message))
    api.get<{ devices: Device[] }>('/api/devices').then((r) => setDevices(r.devices)).catch(() => {})
    api.get<Group[]>('/api/groups').then(setGroups).catch(() => {})
  }, [id])
  useEffect(() => {
    if (!groupId) { setGroupDevices(null); return }
    setGroupDevices(null)
    api.get<Device[]>(`/api/groups/${groupId}/devices`).then(setGroupDevices).catch(() => setGroupDevices([]))
  }, [groupId])

  const deviceMap = useMemo(() => new Map(devices.map((d) => [d.id, d])), [devices])
  const group = groups.find((g) => g.id === groupId) || null
  const targetDevices = kind === 'group' ? groupDevices || [] : sel.map((x) => deviceMap.get(x)).filter((d): d is Device => !!d)
  const targetLabel = kind === 'group' ? (group ? `group ${group.name}` : 'a group') : targetDevices.length === 1 ? deviceName(targetDevices[0]) : `${targetDevices.length} devices`
  const target = kind === 'group' ? { groupId } : { deviceIds: sel }
  const ready = kind === 'group' ? !!groupId && (groupDevices?.length || 0) > 0 : sel.length > 0

  const runDryRun = useCallback(async (deviceIds?: string[]) => {
    setSubmitting(true); setError('')
    try {
      const r = await api.post<{ job: Job }>(`/api/profiles/${id}/apply`, { target: deviceIds ? { deviceIds } : target, dryRun: true })
      setDryJobId(r.job.id); setStep(2)
    } catch (e) { setError((e as Error).message) } finally { setSubmitting(false) }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [id, kind, groupId, sel])

  const dryA = useMemo(() => (dry.done && profile ? analyse(dry.results, deviceMap, profile.settings) : null), [dry.done, dry.results, deviceMap, profile])
  const realA = useMemo(() => (real.done && profile ? analyse(real.results, deviceMap, profile.settings) : null), [real.done, real.results, deviceMap, profile])
  const writeIds = dryA ? dryA.changing.map((r) => r.deviceId) : []
  const needsTyped = writeIds.length > 10
  const confirmWord = kind === 'group' && group ? group.name : profile?.name || ''
  const confirmed = !needsTyped || confirmText.trim().toLowerCase() === confirmWord.trim().toLowerCase()

  const apply = async () => {
    if (!dryA) return
    setSubmitting(true); setError('')
    try {
      const r = await api.post<{ job: Job }>(`/api/profiles/${id}/apply`, { target: { deviceIds: writeIds }, dryRun: false })
      setApplyJobId(r.job.id); setStep(4); setConfirmText('')
    } catch (e) { setError((e as Error).message) } finally { setSubmitting(false) }
  }
  const retryFailed = () => {
    if (!realA) return
    const ids = realA.rows.filter((r) => r.kind === 'failed' || r.kind === 'unreachable').map((r) => r.deviceId).filter(Boolean)
    setKind('device'); setSel(ids); setApplyJobId(null); setRevert(null)
    runDryRun(ids)
  }

  if (error && !profile) return <Banner tone="crit">{error}</Banner>
  if (!profile) return <div className="cap" aria-busy="true">Loading…</div>
  if (!canOperate) return (<>
    <nav aria-label="Breadcrumb" className="cap"><Link to="/profiles">Configuration</Link> / <Link to="/profiles">Profiles</Link> / {profile.name} / Apply</nav>
    <Card><div className="cb"><ViewOnly /></div></Card>
  </>)

  const filtered = devices.filter((d) => !q || `${deviceName(d)} ${d.address} ${d.model || ''} ${d.location} ${d.serial || ''}`.toLowerCase().includes(q.toLowerCase()))
  const toggle = (did: string) => setSel((s) => (s.includes(did) ? s.filter((x) => x !== did) : [...s, did]))

  return (
    <>
      <div>
        <nav aria-label="Breadcrumb" className="cap"><Link to="/profiles">Configuration</Link> / <Link to="/profiles">Profiles</Link> / {profile.name} / Apply</nav>
        <h1 className="h1" style={{ marginTop: 6 }}>Apply “{profile.name}” to {step === 1 && !ready ? '…' : targetLabel}</h1>
      </div>
      <Steps current={step} steps={STEPS.map((s, i) => (i === 0 && step > 1 ? `Target · ${targetLabel}${kind === 'group' ? ` (${targetDevices.length})` : ''}` : s))} />
      {error && <Banner tone="crit" role="alert">{error}</Banner>}

      {step === 1 && (
        <div className="cols">
          <Card title="Target" id="tg" sub="Apply to one device first, check the result, then apply to the group." style={{ flex: '3 1 560px' }}>
            <div className="cb" style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
              <Seg label="Target kind" value={kind} onChange={setKind} options={[{ v: 'device', label: <><Ic.printer />Devices</> }, { v: 'group', label: <><Ic.layers />Group</> }]} />
              {kind === 'group' ? (
                <>
                  <label className="chip" style={{ alignSelf: 'flex-start' }}><Ic.layers />Group<select aria-label="Group" value={groupId} onChange={(e) => setGroupId(e.target.value)}><option value="">choose…</option>{groups.map((g) => <option key={g.id} value={g.id}>{g.name}</option>)}</select></label>
                  {groupId && (groupDevices === null ? <div className="cap" aria-busy="true">Resolving members…</div> : groupDevices.length === 0 ? <div className="cap">This group has no devices.</div> : (
                    <div className="tw" style={{ maxHeight: 320, overflow: 'auto', border: '1px solid var(--border)', borderRadius: 8 }}><table className="tbl compact"><thead><tr><th>Device</th><th>Model</th><th>IP</th><th>Location</th><th>Status</th></tr></thead><tbody>
                      {groupDevices.map((d) => <tr key={d.id}><td style={{ fontWeight: 500 }}>{deviceName(d)}</td><td className="mono">{d.model}</td><td className="mono">{d.address}</td><td className="cap">{d.location || '—'}</td><td><DevicePill d={d} xs /></td></tr>)}
                    </tbody></table></div>
                  ))}
                </>
              ) : (
                <>
                  <div className="row"><SearchBox sm filter value={q} onChange={setQ} placeholder="Find device — name, IP, serial, model…" autoFocus={sel.length === 0} /><span className="cap num">{sel.length} selected</span>{sel.length > 0 && <Btn sm variant="ghost" onClick={() => setSel([])}>Clear</Btn>}</div>
                  <div className="tw" style={{ maxHeight: 380, overflow: 'auto', border: '1px solid var(--border)', borderRadius: 8 }}>
                    <table className="tbl compact" style={{ minWidth: 640 }}>
                      <thead><tr><th style={{ width: 36 }} /><th>Device</th><th>Model</th><th>IP</th><th>Location</th><th>Status</th></tr></thead>
                      <tbody>
                        {filtered.map((d) => <tr key={d.id} className={`clickable${sel.includes(d.id) ? ' sel' : ''}`} onClick={() => toggle(d.id)}>
                          <td onClick={(e) => e.stopPropagation()}><input className="cbx" type="checkbox" aria-label={`Select ${deviceName(d)}`} checked={sel.includes(d.id)} onChange={() => toggle(d.id)} /></td>
                          <td style={{ fontWeight: 500 }}>{deviceName(d)}</td><td className="mono">{d.model}</td><td className="mono">{d.address}</td><td className="cap">{d.location || '—'}</td><td><DevicePill d={d} xs /></td>
                        </tr>)}
                        {filtered.length === 0 && <tr><td colSpan={6} className="cap">No devices match.</td></tr>}
                      </tbody>
                    </table>
                  </div>
                </>
              )}
            </div>
            <div className="cf" style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'center', gap: 8 }}>
              <span className="cap" style={{ flex: 1 }}>The dry run reads each device and shows what would change. Nothing is written until you confirm.</span>
              <Btn variant="pri" disabled={!ready || submitting} onClick={() => runDryRun()}>{submitting ? 'Starting…' : 'Run dry run'}<Ic.arrowRight /></Btn>
            </div>
          </Card>
          <Card title="Profile" id="pf" sub={profile.description || `${profile.settings.length} settings`} style={{ flex: '1 1 300px' }} right={<Link className="btn sm" to="/profiles">Edit</Link>}>
            <div style={{ padding: '4px 0' }}>{profile.settings.map((s) => <div key={s.key} className="frow" style={{ gridTemplateColumns: 'minmax(0,1fr) auto' }}><span className="cap" title={s.key}>{settingName(s.key, byKey)}</span><span className="mono new-v" style={{ justifySelf: 'end' }}>{String(Array.isArray(s.value) ? s.value.join(', ') : typeof s.value === 'boolean' ? (s.value ? 'On' : 'Off') : s.value)}</span></div>)}</div>
          </Card>
        </div>
      )}

      {(step === 2 || step === 3) && (
        <>
          {!dry.done ? (
            <Card><div className="cb" style={{ display: 'flex', flexDirection: 'column', gap: 8 }} aria-busy="true">
              <div style={{ display: 'flex', justifyContent: 'space-between' }}><span>Dry run — reading {targetDevices.length || dry.job?.total || ''} device{targetDevices.length === 1 ? '' : 's'}…</span><span className="num" style={{ fontWeight: 600 }}>{dry.job?.done ?? 0} <span className="cap" style={{ fontWeight: 400 }}>/ {dry.job?.total ?? targetDevices.length}</span></span></div>
              <JobBar done={dry.job?.done ?? 0} failed={dry.job?.failed ?? 0} total={dry.job?.total ?? Math.max(1, targetDevices.length)} running />
              <span className="cap">No write I/O happens in a dry run. {dry.job ? <Link to={`/jobs/${dry.job.id}`}>Job {dry.job.id.slice(0, 8)}</Link> : null}</span>
            </div></Card>
          ) : dryA && (
            <>
              <Banner tone="info" right={<Btn sm onClick={() => runDryRun()} disabled={submitting}><Ic.refresh />Re-run preview</Btn>}>
                <b style={{ color: 'var(--info)' }}>Dry run — nothing has been written yet.</b> Values were read live from each device at {fmtTime(dry.job?.finishedAt)}. Operator-set fields (name, location, asset tag, notes) are never part of a profile.
              </Banner>
              <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(170px, 1fr))', gap: 12 }}>
                <StatTile label="Devices that would change" value={dryA.changing.length} color="var(--warn)" detail={`of ${dryA.rows.length} in target`} />
                <StatTile label="Already match profile" value={dryA.matching.length} color="var(--ok)" detail="no writes" />
                <StatTile label="Unreachable · will skip" value={dryA.skippedRows.length} color="var(--off)" detail={dryA.skippedRows[0] ? dryA.skippedRows.map((r) => r.label).slice(0, 2).join(', ') : 'none'} />
                <StatTile label="Setting writes" value={dryA.writes} detail={`across ${dryA.touched.length} setting${dryA.touched.length === 1 ? '' : 's'}`} />
                <StatTile label="Undo" value={<span style={{ fontSize: 16, padding: '4px 0', display: 'inline-block' }}>Previous values saved</span>} detail="revert from the results" />
              </div>
              <Card title="Changes by setting" id="bys" sub={`profile has ${profile.settings.length} setting${profile.settings.length === 1 ? '' : 's'} · ${profile.settings.length - dryA.touched.length} with no writes`}>
                <SettingRows settings={dryA.settings} byKey={byKey} dry />
              </Card>
              <Card title="Changes by device" id="byd">
                <DeviceRows rows={dryA.rows} byKey={byKey} dry />
              </Card>
              <div style={{ position: 'sticky', bottom: 0, display: 'flex', flexWrap: 'wrap', alignItems: 'center', gap: 12, padding: '12px 16px', background: 'var(--surface)', border: '1px solid var(--border)', borderRadius: 8 }}>
                <span style={{ display: 'inline-flex', alignItems: 'center', gap: 8, color: dryA.writes ? 'var(--warn)' : 'var(--text-3)', fontWeight: 500 }}><Ic.upload />{dryA.writes ? `Next step writes ${dryA.writes} setting${dryA.writes === 1 ? '' : 's'} to ${writeIds.length} device${writeIds.length === 1 ? '' : 's'}` : 'Nothing to write'}</span>
                <div style={{ marginLeft: 'auto', display: 'flex', flexWrap: 'wrap', gap: 8 }}>
                  <Btn onClick={() => { setStep(1); setDryJobId(null) }}>Back</Btn>
                  <Btn variant="pri" disabled={!dryA.writes} onClick={() => setStep(3)}>Continue to confirm<Ic.arrowRight /></Btn>
                </div>
              </div>
            </>
          )}
          {step === 3 && dryA && (
            <Dialog title={`Write ${dryA.writes} setting${dryA.writes === 1 ? '' : 's'} to ${writeIds.length} device${writeIds.length === 1 ? '' : 's'}?`} sub={`Profile “${profile.name}” → ${targetLabel}`} onClose={() => setStep(2)}
              icon={<span style={{ width: 32, height: 32, borderRadius: 8, background: 'var(--warn-bg)', color: 'var(--warn)', display: 'grid', placeItems: 'center', flex: 'none' }}><Ic.upload /></span>}
              footer={<><Btn onClick={() => setStep(2)}>Back to preview</Btn><Btn variant="dngf" disabled={!confirmed || submitting} onClick={apply}>{submitting ? 'Writing…' : `Write to ${writeIds.length} device${writeIds.length === 1 ? '' : 's'}`}</Btn></>}>
              <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '6px 16px', padding: '4px 0 10px' }}>
                <span className="cap">Devices written</span><b className="num" style={{ fontWeight: 500 }}>{writeIds.length}</b>
                <span className="cap">Skipped (unreachable / unsupported)</span><b className="num" style={{ fontWeight: 500 }}>{dryA.skippedRows.length}</b>
                <span className="cap">Already matching</span><b className="num" style={{ fontWeight: 500 }}>{dryA.matching.length}</b>
                <span className="cap">Settings touched</span><b style={{ fontWeight: 500, whiteSpace: 'normal' }}>{dryA.touched.map((k) => settingName(k, byKey)).join(', ')}</b>
                <span className="cap">Undo</span><b style={{ fontWeight: 500 }}>Previous values saved — revert from results</b>
              </div>
              {needsTyped && (
                <label className="cap" style={{ display: 'block' }}>Type <b className="mono" style={{ color: 'var(--text)' }}>{confirmWord.toUpperCase()}</b> to confirm (required above 10 devices)
                  <input className="fin mono" style={{ marginTop: 4 }} value={confirmText} onChange={(e) => setConfirmText(e.target.value)} autoFocus aria-label="Confirmation" />
                </label>
              )}
            </Dialog>
          )}
        </>
      )}

      {step === 4 && (
        <>
          {!real.done ? (
            <Card><div className="cb" style={{ display: 'flex', flexDirection: 'column', gap: 8 }} aria-busy="true">
              <div style={{ display: 'flex', justifyContent: 'space-between' }}><span>Writing to {writeIds.length} device{writeIds.length === 1 ? '' : 's'}…</span><span className="num" style={{ fontWeight: 600 }}>{real.job?.done ?? 0} <span className="cap" style={{ fontWeight: 400 }}>/ {real.job?.total ?? writeIds.length}</span></span></div>
              <JobBar done={real.job?.done ?? 0} failed={real.job?.failed ?? 0} total={real.job?.total ?? Math.max(1, writeIds.length)} running />
              <span className="cap">{real.job ? <Link to={`/jobs/${real.job.id}`}>Job {real.job.id.slice(0, 8)}</Link> : 'Starting…'}</span>
            </div></Card>
          ) : realA && (
            <>
              {(() => { const ok = realA.rows.filter((r) => r.kind === 'change' || r.kind === 'match').length; const bad = realA.rows.length - ok; return (
                <Banner tone={bad ? 'warn' : 'ok'} right={real.job && <Link className="btn sm ghost" to={`/jobs/${real.job.id}`}>Job detail</Link>}>
                  <b style={{ color: bad ? 'var(--warn)' : 'var(--ok)' }}>Applied</b> — {ok} of {realA.rows.length} device{realA.rows.length === 1 ? '' : 's'} verified · {bad} failed · {realA.writes} setting{realA.writes === 1 ? '' : 's'} written. Values were re-read after each write.
                </Banner>) })()}
              <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(170px, 1fr))', gap: 12 }}>
                <StatTile label="Devices written" value={realA.rows.filter((r) => r.writes > 0).length} color="var(--ok)" detail={`of ${realA.rows.length} targeted`} />
                <StatTile label="Failed" value={realA.rows.filter((r) => r.kind === 'failed' || r.kind === 'unreachable').length} color={realA.rows.some((r) => r.kind === 'failed' || r.kind === 'unreachable') ? 'var(--crit)' : undefined} detail="per-setting reasons below" />
                <StatTile label="Settings written" value={realA.writes} detail={`across ${realA.touched.length} setting${realA.touched.length === 1 ? '' : 's'}`} />
                <StatTile label="Finished" value={<span style={{ fontSize: 16, padding: '4px 0', display: 'inline-block' }}>{fmtTime(real.job?.finishedAt)}</span>} detail={`by ${real.job?.createdBy || user.username}`} />
              </div>
              <div className="row">
                {realA.rows.some((r) => r.kind === 'failed' || r.kind === 'unreachable') && <Btn variant="wr" onClick={retryFailed} title="Dry run again on the failed devices only"><Ic.refresh />Retry failed (dry run first)</Btn>}
                {realA.rows.some((r) => r.writes > 0 && r.writeId) && <Btn variant="wr" onClick={() => setRevert(realA.rows.filter((r) => r.writes > 0 && r.writeId).map((r) => ({ writeId: r.writeId!, deviceId: r.deviceId, label: r.label })))}><Ic.chevronLeft />Revert (dry run first)</Btn>}
                <Btn variant="ghost" style={{ marginLeft: 'auto' }} onClick={() => navigate('/profiles')}>Done</Btn>
                <Link className="btn ghost" to={`/profiles/${id}/drift${kind === 'group' ? `?group=${groupId}` : ''}`}>Check drift</Link>
              </div>
              {revert && <RevertPanel targets={revert} byKey={byKey} onClose={() => setRevert(null)} />}
              <Card title="Results by setting" id="rs"><SettingRows settings={realA.settings} byKey={byKey} dry={false} /></Card>
              <Card title="Results by device" id="rd" sub="failures first"><DeviceRows rows={realA.rows} byKey={byKey} dry={false} initialOpen={5} /></Card>
            </>
          )}
        </>
      )}
    </>
  )
}
