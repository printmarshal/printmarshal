import { useCallback, useEffect, useMemo, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { api, ago, fmtTime } from '../api'
import type { User } from '../types'
import { useEvents } from '../ws'
import { Ic } from '../components/icons'
import { Btn, Card, EmptyState, Pill, SearchBox, Tag } from '../components/ui'
import type { Profile, WriteRecord } from '../types-config'
import { areaLabel } from '../types-config'
import ProfileEditor from '../components/config/ProfileEditor'
import { useCatalog } from '../components/config/shared'

export default function Profiles({ user }: { user: User }) {
  const [profiles, setProfiles] = useState<Profile[] | null>(null)
  const [writes, setWrites] = useState<WriteRecord[]>([])
  const [filter, setFilter] = useState('')
  const [editing, setEditing] = useState<Profile | null | 'new'>(null)
  const [msg, setMsg] = useState('')
  const [error, setError] = useState('')
  const { catalog, byKey } = useCatalog()
  const navigate = useNavigate()
  const canOperate = user.role !== 'viewer'

  const load = useCallback(() => {
    api.get<{ profiles: Profile[] }>('/api/profiles').then((r) => setProfiles(r.profiles || [])).catch((e) => { setError(e.message); setProfiles([]) })
    api.get<{ writes: WriteRecord[] }>('/api/config/writes?limit=500').then((r) => setWrites(r.writes || [])).catch(() => {})
  }, [])
  useEffect(() => { load() }, [load])
  useEvents((e) => { if (e.type === 'config.applied') load() }, [load])

  /** Last real (non dry-run) write per profile. */
  const lastApplied = useMemo(() => {
    const m = new Map<string, WriteRecord>()
    for (const w of writes) if (w.profileId && !w.result.dryRun && (!m.has(w.profileId) || w.result.at > m.get(w.profileId)!.result.at)) m.set(w.profileId, w)
    return m
  }, [writes])
  const shown = (profiles || []).filter((p) => !filter || `${p.name} ${p.description}`.toLowerCase().includes(filter.toLowerCase()))
  const areasOf = (p: Profile) => Array.from(new Set(p.settings.map((s) => byKey.get(s.key)?.area || 'other')))
  const remove = async (p: Profile) => {
    if (!confirm(`Delete profile “${p.name}”? Devices keep their current settings.`)) return
    try { await api.del(`/api/profiles/${p.id}`); setMsg(`Deleted ${p.name}.`); load() } catch (e) { setMsg((e as Error).message) }
  }

  return (
    <>
      <div className="ph">
        <div><h1 className="h1">Configuration profiles</h1><div className="cap num">{profiles ? `${profiles.length} profile${profiles.length === 1 ? '' : 's'}` : '…'} · a profile is a set of desired settings · applying always dry-runs first</div></div>
        {canOperate ? <Btn sm variant="pri" onClick={() => setEditing('new')}><Ic.plus />New profile</Btn> : <Tag>Read-only · Viewer</Tag>}
      </div>
      <div className="row"><SearchBox sm filter value={filter} onChange={setFilter} placeholder="Filter profiles…" /></div>
      {error && <div className="banner crit" role="alert"><Ic.crit style={{ color: 'var(--crit)' }} /><div style={{ flex: 1 }}>{error}</div></div>}
      {msg && <div className="banner info" role="status"><Ic.info style={{ color: 'var(--info)' }} /><div style={{ flex: 1 }}>{msg}</div><Btn sm variant="ghost" ic onClick={() => setMsg('')}><Ic.x /></Btn></div>}
      <Card aria-label="Profiles">
        {profiles === null ? <div className="cb cap" aria-busy="true">Loading…</div>
          : shown.length === 0 ? <div className="cb"><EmptyState icon={<Ic.sliders size={20} />} title={filter ? 'No profiles match' : 'No profiles yet'} text={filter ? 'Try another name.' : 'A profile holds the settings you want a fleet to share — sleep timers, protocols, print defaults. Apply it to one device, check, then to a group.'} action={canOperate && !filter ? <Btn sm variant="pri" onClick={() => setEditing('new')}>New profile</Btn> : undefined} /></div>
            : (
              <div className="tw"><table className="tbl" style={{ minWidth: 720 }}>
                <thead><tr><th>Profile</th><th>Areas</th><th className="r">Settings</th><th>Last applied</th><th>Updated</th><th style={{ width: 1 }} /></tr></thead>
                <tbody>{shown.map((p) => {
                  const last = lastApplied.get(p.id)
                  return (
                    <tr key={p.id}>
                      <td style={{ whiteSpace: 'normal', minWidth: 220 }}><button className="link" style={{ fontWeight: 500, color: 'var(--text)' }} onClick={() => setEditing(p)}>{p.name}</button>{p.description && <div className="cap">{p.description}</div>}</td>
                      <td style={{ whiteSpace: 'normal' }}><span style={{ display: 'flex', flexWrap: 'wrap', gap: 4 }}>{areasOf(p).map((a) => <Tag key={a}>{areaLabel(a)}</Tag>)}</span></td>
                      <td className="num r">{p.settings.length}</td>
                      <td>{last ? <span title={fmtTime(last.result.at)}><span className="num">{ago(last.result.at)}</span> <span className="cap">by {last.actor}</span> {last.failed > 0 ? <Pill tone="crit" xs>{last.failed} failed</Pill> : <Pill tone="ok" xs noIcon>{last.written} written</Pill>}</span> : <span className="cap">never</span>}</td>
                      <td className="cap num">{ago(p.updatedAt)}</td>
                      <td><div className="row" style={{ flexWrap: 'nowrap', justifyContent: 'flex-end' }}>
                        <Link className="btn sm" to={`/profiles/${p.id}/drift`}><Ic.activity />Drift</Link>
                        {canOperate && <Btn sm onClick={() => setEditing(p)}><Ic.pencil />Edit</Btn>}
                        {canOperate && <Btn sm variant="wr" onClick={() => navigate(`/profiles/${p.id}/apply`)} title="Opens the apply flow: target, dry run, confirm"><Ic.upload />Apply…</Btn>}
                        {canOperate && <Btn sm variant="ghost" ic aria-label={`Delete ${p.name}`} onClick={() => remove(p)}><Ic.x /></Btn>}
                      </div></td>
                    </tr>
                  )
                })}</tbody>
              </table></div>
            )}
        <div className="cf cap" style={{ display: 'flex', alignItems: 'center', gap: 8 }}><Ic.info size={14} style={{ color: 'var(--info)' }} />Apply writes settings to devices after a dry run and a confirmation; the previous values are kept so a write can be reverted.</div>
      </Card>
      {editing && catalog && <ProfileEditor profile={editing === 'new' ? null : editing} catalog={catalog} onClose={() => setEditing(null)} onSaved={(p) => { setEditing(null); setMsg(`Saved ${p.name}.`); load() }} />}
    </>
  )
}
