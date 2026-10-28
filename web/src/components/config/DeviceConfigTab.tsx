import { useCallback, useEffect, useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, ago, fmtTime } from '../../api'
import type { Device } from '../../types'
import { deviceName } from '../../types'
import { useEvents } from '../../ws'
import { Ic } from '../icons'
import { Banner, Btn, Card, ChipSelect, EmptyState, Pill, Tag } from '../ui'
import type { ConfigDiff, ConfigSetting, ConfigSnapshot, DeviceConfigResponse, Profile, WriteRecord } from '../../types-config'
import { areaLabel, valuesEqual } from '../../types-config'
import RevertPanel, { type RevertTarget } from './RevertPanel'
import { SourceTag, fmtValue, isSecret, useCatalog } from './shared'

const profileKey = (deviceId: string) => `pm.config.profile.${deviceId}`

/**
 * Configuration tab of a device: the latest snapshot grouped by area, drift
 * against a chosen profile, snapshot history and device-vs-device diff.
 * Mounted even while hidden so the drifted count reaches the tab label.
 */
export default function DeviceConfigTab({ device, canOperate, hidden, onDrift }: { device: Device; canOperate: boolean; hidden?: boolean; onDrift?: (n: number | null) => void }) {
  const { catalog, byKey } = useCatalog()
  const [cfg, setCfg] = useState<DeviceConfigResponse | null | undefined>(undefined)
  const [cfgError, setCfgError] = useState('')
  const [reading, setReading] = useState(false)
  const [profiles, setProfiles] = useState<Profile[]>([])
  const [profileId, setProfileId] = useState(() => { try { return localStorage.getItem(profileKey(device.id)) || '' } catch { return '' } })
  const [snapshots, setSnapshots] = useState<ConfigSnapshot[]>([])
  const [writes, setWrites] = useState<WriteRecord[]>([])
  const [devices, setDevices] = useState<Device[]>([])
  const [other, setOther] = useState('')
  const [diff, setDiff] = useState<ConfigDiff | null>(null)
  const [diffError, setDiffError] = useState('')
  const [revert, setRevert] = useState<RevertTarget[] | null>(null)

  const load = useCallback((refresh = false) => {
    if (refresh) setReading(true)
    setCfgError('')
    api.get<DeviceConfigResponse>(`/api/devices/${device.id}/config${refresh ? '?refresh=true' : ''}`).then(setCfg).catch((e) => { setCfgError((e as Error).message); setCfg((c) => c ?? null) }).finally(() => setReading(false))
    api.get<{ snapshots: ConfigSnapshot[] }>(`/api/devices/${device.id}/config/snapshots?limit=20`).then((r) => setSnapshots(r.snapshots || [])).catch(() => {})
    api.get<{ writes: WriteRecord[] }>(`/api/config/writes?device=${device.id}&limit=20`).then((r) => setWrites(r.writes || [])).catch(() => {})
  }, [device.id])
  useEffect(() => { load() }, [load])
  useEffect(() => {
    api.get<{ profiles: Profile[] }>('/api/profiles').then((r) => setProfiles(r.profiles || [])).catch(() => {})
    api.get<{ devices: Device[] }>('/api/devices').then((r) => setDevices(r.devices.filter((d) => d.id !== device.id))).catch(() => {})
  }, [device.id])
  useEvents((e) => { if (e.type === 'config.applied' && (e.data as { deviceId?: string })?.deviceId === device.id) load() }, [load])
  useEffect(() => {
    if (!other) { setDiff(null); return }
    setDiffError('')
    api.get<ConfigDiff>(`/api/config/diff?a=${device.id}&b=${other}`).then(setDiff).catch((e) => { setDiff(null); setDiffError((e as Error).message) })
  }, [device.id, other])

  const doc = cfg?.document
  const profile = profiles.find((p) => p.id === profileId) || null
  const byArea = useMemo(() => {
    const areas = catalog?.areas || []
    const m = new Map<string, ConfigSetting[]>()
    for (const s of doc?.settings || []) (m.get(s.area) || m.set(s.area, []).get(s.area)!).push(s)
    return [...areas.filter((a) => m.has(a)), ...Array.from(m.keys()).filter((a) => !areas.includes(a))].map((a) => ({ area: a, items: m.get(a)! }))
  }, [doc, catalog])
  const drift = useMemo(() => profile && doc ? profile.settings.map((s) => {
    const cur = doc.settings.find((x) => x.key === s.key)
    const state: 'drifted' | 'matches' | 'missing' = !cur ? 'missing' : valuesEqual(cur.value, s.value) ? 'matches' : 'drifted'
    return { key: s.key, desired: s.value, current: cur?.value, state, writable: cur?.writable ?? false }
  }) : null, [profile, doc])
  const drifted = drift ? drift.filter((d) => d.state === 'drifted').length : null
  useEffect(() => { onDrift?.(profile ? drifted : null) }, [drifted, profile, onDrift])
  const chooseProfile = (pid: string) => { setProfileId(pid); try { if (pid) localStorage.setItem(profileKey(device.id), pid); else localStorage.removeItem(profileKey(device.id)) } catch { /* ignore */ } }
  const lastWrite = profile ? writes.find((w) => !w.result.dryRun && w.profileId === profile.id) : undefined

  if (hidden) return null
  return (
    <div className="stack">
      {cfgError && <Banner tone={doc ? 'warn' : 'crit'}>{cfgError}{doc ? ' — showing the last stored snapshot.' : ''}</Banner>}
      <Card title={profile ? `Profile: ${profile.name}` : 'Profile drift'} id="drift"
        sub={profile ? `${lastWrite ? `Last applied ${ago(lastWrite.result.at)} by ${lastWrite.actor}` : 'Never applied from here'} · ${drifted ?? 0} of ${profile.settings.length} settings drifted` : 'Pick a profile to compare this device against it'}
        right={<div className="row"><ChipSelect label="Profile" value={profileId} onChange={chooseProfile} any="none" options={profiles.map((p) => ({ v: p.id, label: p.name }))} />{canOperate && profile && <Link className="btn sm wr" to={`/profiles/${profile.id}/apply?device=${device.id}`} title="Opens the apply flow with this device preselected; a dry run runs before anything is written"><Ic.upload />Re-apply drifted (dry run first)</Link>}</div>}>
        {!profile ? <div className="cb cap">{profiles.length === 0 ? <>No profiles yet. <Link to="/profiles">Create one</Link> to track drift.</> : 'No profile selected.'}</div>
          : !doc ? <div className="cb cap">{cfg === undefined ? 'Loading…' : 'No configuration has been read from this device yet.'}</div>
            : (
              <div className="tw"><table className="tbl" style={{ minWidth: 760 }}>
                <thead><tr><th>Area</th><th>Setting</th><th>Profile wants</th><th>Device has</th><th>State</th></tr></thead>
                <tbody>{drift!.map((d) => { const def = byKey.get(d.key); return (
                  <tr key={d.key}>
                    <td className="cap">{areaLabel(def?.area || 'other')}</td>
                    <td title={d.key}>{def?.name || d.key}</td>
                    <td className="mono">{fmtValue(d.desired, def)}</td>
                    <td className="mono" style={{ color: d.state === 'drifted' ? 'var(--warn)' : undefined }}>{d.state === 'missing' ? <span className="cap">not reported</span> : fmtValue(d.current, def)}</td>
                    <td>{d.state === 'drifted' ? <Pill tone="warn">Drifted</Pill> : d.state === 'matches' ? <Pill tone="ok">Matches</Pill> : <Pill tone="off">Not exposed</Pill>}{d.state === 'drifted' && !d.writable && <span className="cap" style={{ marginLeft: 6 }}>read-only here</span>}</td>
                  </tr>) })}</tbody>
              </table></div>
            )}
      </Card>

      <Card title="Configuration" id="cfg"
        sub={doc ? <>read {ago(cfg?.at || doc.readAt)} · {fmtTime(cfg?.at || doc.readAt)} · via {doc.sources.map((s) => <Tag key={s}>{s}</Tag>)}{cfg?.fresh ? ' · fresh' : ' · stored snapshot'}</> : 'latest stored snapshot'}
        right={<Btn sm onClick={() => load(true)} disabled={reading} title="Reads the device now; nothing is written"><Ic.refresh />{reading ? 'Reading…' : 'Read from device'}</Btn>}>
        {cfg === undefined ? <div className="cb cap" aria-busy="true">Loading…</div>
          : !doc || doc.settings.length === 0 ? <div className="cb"><EmptyState icon={<Ic.sliders size={20} />} title="No configuration read yet" text="Read the device to see its settings. Reads are safe; they never change anything." action={<Btn sm onClick={() => load(true)} disabled={reading}><Ic.refresh />Read from device</Btn>} /></div>
            : (
              <div className="tw"><table className="tbl" style={{ minWidth: 720 }}>
                <thead><tr><th>Setting</th><th>Value</th><th>Source</th><th>Write</th></tr></thead>
                <tbody>{byArea.map((g) => (<GroupRows key={g.area} area={g.area} items={g.items} secretKeys={new Set(g.items.filter((s) => isSecret(byKey.get(s.key)) || s.type === 'secret').map((s) => s.key))} />))}</tbody>
              </table></div>
            )}
        <div className="cf cap" style={{ display: 'flex', alignItems: 'center', gap: 8 }}><Ic.info size={14} style={{ color: 'var(--info)' }} />Source is the driver that reported the value. "writable" means a driver can set it; secrets are never displayed.</div>
      </Card>

      <div className="cols">
        <Card title="Snapshot history" id="snaps" sub={`${snapshots.length} stored read${snapshots.length === 1 ? '' : 's'} · newest 50 kept`} style={{ flex: '1 1 320px' }}>
          {snapshots.length === 0 ? <div className="cb cap">No snapshots yet.</div> : (
            <div>{snapshots.map((s) => <div className="lrow" key={s.id}><Ic.clock style={{ color: 'var(--text-3)' }} /><span style={{ flex: 1 }}>{fmtTime(s.at)}<div className="cap">{s.document?.settings.length ?? 0} settings · {s.document?.sources.join(', ')}</div></span><span className="cap num">{ago(s.at)}</span>{cfg?.snapshotId === s.id && <Tag up>current</Tag>}</div>)}</div>
          )}
        </Card>
        <Card title="Write history" id="wh" sub="applies to this device, newest first" style={{ flex: '1 1 320px' }}>
          {writes.length === 0 ? <div className="cb cap">No writes recorded.</div> : (
            <div>{writes.map((w) => <div className="lrow" key={w.id}>
              {w.result.dryRun ? <Pill tone="info" xs noIcon>dry run</Pill> : w.failed ? <Pill tone="crit" xs>{w.failed} failed</Pill> : <Pill tone="ok" xs>{w.written} written</Pill>}
              <span style={{ flex: 1, minWidth: 0 }}>{w.profileId ? profiles.find((p) => p.id === w.profileId)?.name || 'profile' : w.revertOf ? 'revert' : 'ad-hoc'} · {w.changes.length} setting{w.changes.length === 1 ? '' : 's'}<div className="cap">{fmtTime(w.result.at)} · {w.actor}{w.jobId && <> · <Link to={`/jobs/${w.jobId}`}>job</Link></>}</div></span>
              {canOperate && !w.result.dryRun && (w.result.rollback?.length || 0) > 0 && <Btn sm variant="wr" onClick={() => setRevert([{ writeId: w.id, deviceId: device.id, label: deviceName(device) }])}>Revert…</Btn>}
            </div>)}</div>
          )}
        </Card>
      </div>
      {revert && <RevertPanel targets={revert} byKey={byKey} onClose={() => setRevert(null)} onDone={() => load()} />}

      <Card title="Compare with another device" id="cmp" sub="latest stored snapshots of both; values that differ" right={<ChipSelect label="Compare with" value={other} onChange={setOther} any="choose…" options={devices.map((d) => ({ v: d.id, label: `${deviceName(d)} · ${d.address}` }))} />}>
        {diffError && <div className="cb" style={{ color: 'var(--crit)' }}>{diffError}</div>}
        {!other ? <div className="cb cap">Pick a device to see which settings differ.</div> : !diff ? <div className="cb cap" aria-busy="true">Comparing…</div> : diff.items.length === 0 ? <div className="cb cap">Identical on every shared key · {diff.same} same.</div> : (
          <div className="tw"><table className="tbl" style={{ minWidth: 640 }}>
            <thead><tr><th>Setting</th><th>{deviceName(device)}</th><th>{deviceName(devices.find((d) => d.id === other)!)}</th></tr></thead>
            <tbody>{diff.items.map((it) => { const def = byKey.get(it.key); return <tr key={it.key}><td title={it.key}><span className="cap" style={{ marginRight: 6 }}>{areaLabel(it.area || def?.area || 'other')}</span>{it.name || def?.name || it.key}</td><td className="mono">{it.onlyB ? <span className="cap">not reported</span> : fmtValue(it.a, def)}</td><td className="mono">{it.onlyA ? <span className="cap">not reported</span> : fmtValue(it.b, def)}</td></tr> })}</tbody>
          </table></div>
        )}
        {diff && diff.items.length > 0 && <div className="cf cap num">{diff.items.length} differ · {diff.same} same</div>}
      </Card>
    </div>
  )
}

function GroupRows({ area, items, secretKeys }: { area: string; items: ConfigSetting[]; secretKeys: Set<string> }) {
  return (
    <>
      <tr className="grp"><td colSpan={4}>{areaLabel(area)} <span className="cap" style={{ fontWeight: 400 }}>· {items.length}</span></td></tr>
      {items.map((s) => (
        <tr key={s.key}>
          <td title={s.key}>{s.name || s.key}<div className="mono cap" style={{ fontSize: 10 }}>{s.key}</div></td>
          <td className="mono" style={{ whiteSpace: 'normal' }}>{secretKeys.has(s.key) ? <span style={{ display: 'inline-flex', alignItems: 'center', gap: 6, color: 'var(--text-3)' }}><Ic.lock size={13} />Set · never displayed</span> : fmtValue(s.value)}</td>
          <td><SourceTag source={s.source} /></td>
          <td>{s.writable ? <Tag title="a driver can set this value"><Ic.pencil size={11} />writable</Tag> : <span className="cap" title={s.readOnlyReason}>read-only{s.readOnlyReason ? ` · ${s.readOnlyReason}` : ''}</span>}</td>
        </tr>
      ))}
    </>
  )
}
