import { useEffect, useMemo, useState } from 'react'
import { api } from '../../api'
import type { Device, Group } from '../../types'
import { deviceName, makeModel } from '../../types'
import type { CampaignRequest, CampaignView, FirmwarePackage, MaintenanceWindow } from '../../types-firmware'
import { canaryLabel, checkCompatibility, packageLabel, weekDays } from '../../types-firmware'
import { Ic } from '../icons'
import { Banner, Btn, SearchBox, Seg, Tag } from '../ui'
import { fwApi } from './api'

export interface WizardPreset {
  packageId?: string; deviceIds?: string[]; groupId?: string; name?: string; description?: string
  canaryCount?: number; concurrency?: number; failureThresholdPercent?: number; window?: MaintenanceWindow | null
  scheduledAt?: string; verifyTimeoutSecs?: number; allowDowngrade?: boolean
  /** Default for the "start as a dry run" toggle (a cloned dry run defaults to the real thing). */
  dryRun?: boolean
}

/**
 * New-campaign drawer. Creates the campaign as a **draft** — nothing starts here;
 * the Start gate lives on the monitor. The dry-run toggle is handed to the monitor
 * as the default for that gate.
 */
export default function CampaignWizard({ preset, onClose, onCreated }: { preset?: WizardPreset; onClose: () => void; onCreated: (c: CampaignView, opts: { dryRun: boolean }) => void }) {
  const [packages, setPackages] = useState<FirmwarePackage[] | null>(null)
  const [groups, setGroups] = useState<Group[]>([])
  const [devices, setDevices] = useState<Device[]>([])
  const [groupMembers, setGroupMembers] = useState<Device[] | null>(null)
  const [name, setName] = useState(preset?.name || '')
  const [description, setDescription] = useState(preset?.description || '')
  const [packageId, setPackageId] = useState(preset?.packageId || '')
  const [targetKind, setTargetKind] = useState<'group' | 'devices'>(preset?.groupId ? 'group' : preset?.deviceIds?.length ? 'devices' : 'group')
  const [groupId, setGroupId] = useState(preset?.groupId || '')
  const [deviceIds, setDeviceIds] = useState<string[]>(preset?.deviceIds || [])
  const [deviceFilter, setDeviceFilter] = useState('')
  const [showAll, setShowAll] = useState(false)
  const pw = preset?.window || null
  const [canary, setCanary] = useState(preset?.canaryCount !== undefined && preset.canaryCount >= 0 ? String(preset.canaryCount) : '')
  const [concurrency, setConcurrency] = useState(String(preset?.concurrency || 2))
  const [threshold, setThreshold] = useState(String(preset?.failureThresholdPercent ?? 20))
  const [useWindow, setUseWindow] = useState(!!pw && (!!pw.days?.length || !!pw.start))
  const [days, setDays] = useState<string[]>(pw?.days || [])
  const [start, setStart] = useState(pw?.start || '22:00')
  const [end, setEnd] = useState(pw?.end || '06:00')
  const [tz, setTz] = useState(() => { if (pw?.tz) return pw.tz; try { return Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC' } catch { return 'UTC' } })
  const [when, setWhen] = useState<'now' | 'at'>(preset?.scheduledAt && new Date(preset.scheduledAt).getTime() > Date.now() ? 'at' : 'now')
  const [at, setAt] = useState(() => { const t = preset?.scheduledAt ? new Date(preset.scheduledAt) : null; if (!t || t.getTime() <= Date.now()) return ''; const p = (n: number) => String(n).padStart(2, '0'); return `${t.getFullYear()}-${p(t.getMonth() + 1)}-${p(t.getDate())}T${p(t.getHours())}:${p(t.getMinutes())}` })
  const [verifyMin, setVerifyMin] = useState(String(preset?.verifyTimeoutSecs ? Math.max(1, Math.round(preset.verifyTimeoutSecs / 60)) : 15))
  const [dryRun, setDryRun] = useState(preset?.dryRun ?? true)
  const [allowDowngrade, setAllowDowngrade] = useState(!!preset?.allowDowngrade)
  const [msg, setMsg] = useState('')
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    fwApi.packages().then(setPackages).catch(() => setPackages([]))
    api.get<Group[]>('/api/groups').then(setGroups).catch(() => {})
    api.get<{ devices: Device[] }>('/api/devices').then((r) => setDevices(r.devices.filter((d) => d.state !== 'decommissioned'))).catch(() => {})
  }, [])
  useEffect(() => {
    if (targetKind !== 'group' || !groupId) { setGroupMembers(null); return }
    api.get<{ devices: Device[] }>(`/api/devices?group=${encodeURIComponent(groupId)}`).then((r) => setGroupMembers(r.devices.filter((d) => d.state !== 'decommissioned'))).catch(() => setGroupMembers([]))
  }, [targetKind, groupId])
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => { if (e.key === 'Escape') onClose() }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [onClose])

  const pkg = packages?.find((p) => p.id === packageId) || null
  useEffect(() => { if (pkg && !name) setName(`${pkg.models[0] || pkg.vendor || 'Firmware'} → ${pkg.version}`) }, [pkg]) // eslint-disable-line react-hooks/exhaustive-deps

  /** Compatibility verdict per device for the chosen package (client preview; the server decides at start). */
  const verdicts = useMemo(() => {
    const m = new Map<string, ReturnType<typeof checkCompatibility>>()
    if (pkg) for (const d of devices) m.set(d.id, checkCompatibility(pkg, d, allowDowngrade))
    return m
  }, [pkg, devices, allowDowngrade])
  const deliverable = (d: Device) => { const v = verdicts.get(d.id); return !!v && v.compatible && !v.current }
  const targets: Device[] = targetKind === 'group' ? (groupMembers || []) : devices.filter((d) => deviceIds.includes(d.id))
  const eligible = targets.filter(deliverable)
  const compatibleDevices = devices.filter((d) => deliverable(d))
  const pickerList = (showAll ? devices : compatibleDevices).filter((d) => !deviceFilter || `${deviceName(d)} ${d.address} ${d.model || ''} ${d.firmware || ''}`.toLowerCase().includes(deviceFilter.toLowerCase()))
  const toggleDevice = (id: string) => setDeviceIds((ids) => ids.includes(id) ? ids.filter((x) => x !== id) : [...ids, id])
  const toggleDay = (d: string) => setDays((ds) => ds.includes(d) ? ds.filter((x) => x !== d) : weekDays.filter((w) => w === d || ds.includes(w)))

  const canaryN = canary.trim() === '' ? undefined : Math.max(0, Math.floor(Number(canary)))
  const problems: string[] = []
  if (!name.trim()) problems.push('a name')
  if (!pkg) problems.push('a package')
  if (targetKind === 'group' ? !groupId : deviceIds.length === 0) problems.push('a target')
  if (when === 'at' && !at) problems.push('a start time')
  if (useWindow && (!start || !end)) problems.push('window start and end')

  const submit = async (e: React.FormEvent) => {
    e.preventDefault(); setMsg('')
    if (problems.length) { setMsg(`Still needed: ${problems.join(', ')}.`); return }
    const body: CampaignRequest = {
      name: name.trim(), description: description.trim(), packageId,
      concurrency: Math.max(1, Math.floor(Number(concurrency) || 2)),
      failureThresholdPercent: Math.min(100, Math.max(0, Math.floor(Number(threshold) || 0))),
      allowDowngrade,
      verifyTimeoutSecs: Math.max(0, Math.floor(Number(verifyMin) || 0)) * 60,
    }
    if (canaryN !== undefined && !isNaN(canaryN)) body.canaryCount = canaryN
    if (targetKind === 'group') body.groupId = groupId; else body.deviceIds = deviceIds
    if (useWindow) { const w: MaintenanceWindow = { days, start, end, tz: tz.trim() }; body.window = w }
    if (when === 'at' && at) body.scheduledAt = new Date(at).toISOString()
    setBusy(true)
    try { const c = await fwApi.createCampaign(body); onCreated(c, { dryRun }) } catch (err) { setMsg((err as Error).message) } finally { setBusy(false) }
  }

  const field = (label: string, input: React.ReactNode, hint?: React.ReactNode) => (
    <label className="cap" style={{ display: 'block' }}>{label}{input}{hint && <div style={{ fontSize: 12, color: 'var(--text-3)', marginTop: 4 }}>{hint}</div>}</label>
  )

  return (
    <div className="drawer-backdrop" onClick={onClose}>
      <form className="drawer" style={{ width: 'min(640px, 100%)' }} onClick={(e) => e.stopPropagation()} onSubmit={submit} aria-label="New firmware campaign">
        <div className="row between"><h2 className="h2" style={{ fontSize: 16 }}>New firmware campaign</h2><Btn sm variant="ghost" ic onClick={onClose} aria-label="Close"><Ic.x /></Btn></div>
        <Banner tone="accent" role="note">Saving creates a <b>draft</b>. Nothing reaches a device until you pass the Start gate on the campaign page, and wave 2 waits for a second confirmation after the canary wave.</Banner>

        <div style={{ display: 'grid', gridTemplateColumns: 'minmax(0, 2fr) minmax(0, 3fr)', gap: 10 }}>
          {field('Name', <input className="fin" value={name} onChange={(e) => setName(e.target.value)} required autoFocus style={{ marginTop: 4 }} />)}
          {field('Description', <input className="fin" value={description} onChange={(e) => setDescription(e.target.value)} placeholder="optional" style={{ marginTop: 4 }} />)}
        </div>

        <section style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
          <div className="cap" style={{ fontWeight: 500, color: 'var(--text-2)' }}>Package</div>
          {packages === null ? <div className="cap" aria-busy="true">Loading packages…</div> : packages.length === 0 ? <div className="cap">No packages registered yet — an admin uploads them on the Firmware → Packages tab.</div> : (
            <select className="fin" value={packageId} onChange={(e) => setPackageId(e.target.value)} aria-label="Package" required>
              <option value="">Choose a package…</option>
              {packages.map((p) => <option key={p.id} value={p.id}>{packageLabel(p)}</option>)}
            </select>
          )}
          {pkg && (
            <div style={{ border: '1px solid var(--border)', borderRadius: 8, padding: '8px 12px', display: 'flex', flexDirection: 'column', gap: 6, background: 'var(--surface-2)' }}>
              <div className="row" style={{ gap: 6 }}><span className="cap">Compatible models</span>{pkg.models.length ? pkg.models.map((m) => <Tag key={m} mono>{m}</Tag>) : <Tag>any model</Tag>}<span className="cap">· vendor <b className="mono">{pkg.vendor || 'any'}</b></span></div>
              <div className="cap">Version <b className="mono" style={{ color: 'var(--text)' }}>{pkg.version}</b>{pkg.minVersion && <> · requires ≥ <span className="mono">{pkg.minVersion}</span></>} · <span className="num">{compatibleDevices.length}</span> device{compatibleDevices.length === 1 ? '' : 's'} in the fleet would take this update{pkg.notes ? ` · ${pkg.notes}` : ''}</div>
            </div>
          )}
        </section>

        <section style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
          <div className="row between"><div className="cap" style={{ fontWeight: 500, color: 'var(--text-2)' }}>Target</div><Seg sm label="Target kind" value={targetKind} onChange={setTargetKind} options={[{ v: 'group', label: 'Group' }, { v: 'devices', label: 'Pick devices' }]} /></div>
          {targetKind === 'group' ? (
            <>
              <select className="fin" value={groupId} onChange={(e) => setGroupId(e.target.value)} aria-label="Group">
                <option value="">Choose a group…</option>
                {groups.map((g) => <option key={g.id} value={g.id}>{g.name} · {g.kind === 'rule' ? 'rule-based' : 'static'}</option>)}
              </select>
              {groupId && groupMembers && <div className="cap"><span className="num">{groupMembers.length}</span> member{groupMembers.length === 1 ? '' : 's'}{pkg && <> · <b className="num" style={{ color: 'var(--text)' }}>{eligible.length}</b> would be updated, {groupMembers.length - eligible.length} skipped (incompatible or already current)</>} · rule groups re-evaluate at start</div>}
            </>
          ) : (
            <div style={{ border: '1px solid var(--border)', borderRadius: 8, overflow: 'hidden' }}>
              <div className="row" style={{ padding: 8, borderBottom: '1px solid var(--border)', background: 'var(--surface-2)' }}>
                <SearchBox sm filter value={deviceFilter} onChange={setDeviceFilter} placeholder="Filter devices…" />
                <label className="cap" style={{ display: 'inline-flex', alignItems: 'center', gap: 6 }}><input className="cbx" type="checkbox" checked={showAll} onChange={(e) => setShowAll(e.target.checked)} />Show incompatible too</label>
                <span className="cap num" style={{ marginLeft: 'auto' }}>{deviceIds.length} selected</span>
              </div>
              <div style={{ maxHeight: 260, overflow: 'auto' }}>
                {!pkg && <div className="cb cap">Choose a package first — the list shows the devices it fits.</div>}
                {pkg && pickerList.length === 0 && <div className="cb cap">{showAll ? 'No devices match.' : 'No compatible device needs this version. Tick “Show incompatible too” to see why.'}</div>}
                {pkg && pickerList.map((d) => { const v = verdicts.get(d.id); const ok = deliverable(d); return (
                  <label key={d.id} className="lrow" style={{ padding: '6px 10px', gap: 10, cursor: 'pointer', opacity: ok ? 1 : .7 }}>
                    <input className="cbx" type="checkbox" checked={deviceIds.includes(d.id)} onChange={() => toggleDevice(d.id)} aria-label={`Target ${deviceName(d)}`} />
                    <span style={{ flex: 1, minWidth: 0 }}><b style={{ fontWeight: 500 }}>{deviceName(d)}</b><span className="cap" style={{ display: 'block', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{makeModel(d)} · <span className="mono">{d.address}</span> · fw <span className="mono">{d.firmware || 'unknown'}</span></span></span>
                    {ok ? <span className="pill ok xs">{v?.downgrade ? 'Downgrade' : 'Compatible'}</span> : <span className="pill off xs" title={v?.reason}>{v?.current ? 'Current' : 'Skip'}</span>}
                  </label>
                ) })}
              </div>
            </div>
          )}
        </section>

        <section style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(150px, 1fr))', gap: 10 }}>
          {field('Canary wave', <input className="fin num" type="number" min={0} value={canary} onChange={(e) => setCanary(e.target.value)} placeholder="default" style={{ marginTop: 4 }} />, canaryLabel(canaryN, eligible.length || undefined) + ' · 0 = no canary')}
          {field('Concurrency', <input className="fin num" type="number" min={1} value={concurrency} onChange={(e) => setConcurrency(e.target.value)} style={{ marginTop: 4 }} />, 'devices updating at once')}
          {field('Failure threshold %', <input className="fin num" type="number" min={0} max={100} value={threshold} onChange={(e) => setThreshold(e.target.value)} style={{ marginTop: 4 }} />, 'pause when exceeded · 0 = never')}
          {field('Verify timeout (min)', <input className="fin num" type="number" min={1} value={verifyMin} onChange={(e) => setVerifyMin(e.target.value)} style={{ marginTop: 4 }} />, 'wait for the new version to be reported')}
        </section>

        <section style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
          <label className="cap" style={{ display: 'inline-flex', alignItems: 'center', gap: 6, fontWeight: 500, color: 'var(--text-2)' }}><input className="cbx" type="checkbox" checked={useWindow} onChange={(e) => setUseWindow(e.target.checked)} />Maintenance window</label>
          {useWindow && (
            <div style={{ display: 'flex', flexDirection: 'column', gap: 8, border: '1px solid var(--border)', borderRadius: 8, padding: 10 }}>
              <div className="row" role="group" aria-label="Days">{weekDays.map((d) => <button key={d} type="button" className={`chip pillish${days.includes(d) ? ' set' : ''}`} aria-pressed={days.includes(d)} onClick={() => toggleDay(d)}>{d.charAt(0).toUpperCase() + d.slice(1)}</button>)}<span className="cap">{days.length === 0 ? 'every day' : ''}</span></div>
              <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr 2fr', gap: 10 }}>
                {field('Start', <input className="fin num" type="time" value={start} onChange={(e) => setStart(e.target.value)} style={{ marginTop: 4 }} />)}
                {field('End', <input className="fin num" type="time" value={end} onChange={(e) => setEnd(e.target.value)} style={{ marginTop: 4 }} />, 'end before start spans midnight')}
                {field('Time zone', <input className="fin mono" value={tz} onChange={(e) => setTz(e.target.value)} placeholder="Europe/Berlin" style={{ marginTop: 4 }} />)}
              </div>
            </div>
          )}
        </section>

        <section style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
          <div className="row between"><div className="cap" style={{ fontWeight: 500, color: 'var(--text-2)' }}>Schedule</div><Seg sm label="Schedule" value={when} onChange={setWhen} options={[{ v: 'now', label: 'When started' }, { v: 'at', label: 'At a time' }]} /></div>
          {when === 'at' && field('Start no earlier than', <input className="fin num" type="datetime-local" value={at} onChange={(e) => setAt(e.target.value)} style={{ marginTop: 4 }} />, 'local time · the campaign still needs the Start gate and shows “scheduled” until then')}
        </section>

        <section style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
          <label className={`meth${dryRun ? ' on' : ''}`}><input className="cbx" type="checkbox" checked={dryRun} onChange={(e) => setDryRun(e.target.checked)} style={{ marginTop: 2 }} /><span><b style={{ fontWeight: 600 }}>Start as a dry run</b><span className="cap" style={{ display: 'block' }}>Preselects “dry run” on the Start gate: planning, pre-flight and driver validation run, nothing is delivered. A dry run uses up the campaign; “Run for real…” on its page then creates the real one with the same settings.</span></span></label>
          <label className={`meth${allowDowngrade ? ' on' : ''}`} style={allowDowngrade ? { borderColor: 'var(--warn-line)', background: 'var(--warn-bg)' } : undefined}><input className="cbx" type="checkbox" checked={allowDowngrade} onChange={(e) => setAllowDowngrade(e.target.checked)} style={{ marginTop: 2 }} /><span><b style={{ fontWeight: 600 }}>Allow downgrade</b><span className="cap" style={{ display: 'block' }}>Deliver even when the device runs a newer version (rollback). Off by default.</span></span></label>
          {allowDowngrade && <Banner tone="warn"><b style={{ color: 'var(--warn)' }}>Downgrades can brick devices.</b> Not every vendor permits them; check the vendor's guidance before starting this campaign for real.</Banner>}
        </section>

        {msg && <div className="error" style={{ fontSize: 12 }}>{msg}</div>}
        <div className="row" style={{ justifyContent: 'flex-end', marginTop: 'auto', paddingTop: 8, borderTop: '1px solid var(--border)' }}>
          <span className="cap" style={{ marginRight: 'auto' }}>{pkg && targets.length > 0 ? <><b className="num" style={{ color: 'var(--text)' }}>{eligible.length}</b> of {targets.length} targeted device{targets.length === 1 ? '' : 's'} would be updated</> : problems.length ? `Needs ${problems.join(', ')}` : ''}</span>
          <Btn variant="ghost" onClick={onClose} disabled={busy}>Cancel</Btn>
          <Btn type="submit" variant="pri" disabled={busy || problems.length > 0}>{busy ? 'Saving…' : 'Save draft & open'}</Btn>
        </div>
      </form>
    </div>
  )
}
