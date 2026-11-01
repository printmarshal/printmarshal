import { useCallback, useEffect, useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { api } from '../api'
import type { Device, Group, GroupRule, User } from '../types'
import { deviceName } from '../types'
import { Ic } from '../components/icons'
import { Btn, Card, DevicePill, EmptyState, SearchBox, Seg, Tag } from '../components/ui'

type Field = keyof GroupRule
const fields: { k: Field; label: string; op: string; placeholder: string; options?: string[] }[] = [
  { k: 'modelContains', label: 'Model', op: 'contains', placeholder: 'e.g. LaserJet' },
  { k: 'makeContains', label: 'Vendor', op: 'contains', placeholder: 'e.g. HP' },
  { k: 'subnet', label: 'Subnet', op: 'is in', placeholder: '10.0.0.0/24' },
  { k: 'locationContains', label: 'Location', op: 'contains', placeholder: 'e.g. Library' },
  { k: 'deviceType', label: 'Device type', op: 'is', placeholder: '', options: ['printer', 'mfp'] },
  { k: 'state', label: 'Lifecycle', op: 'is', placeholder: '', options: ['discovered', 'onboarded', 'active', 'needs_attention', 'decommissioned'] },
  { k: 'hasCapability', label: 'Capability', op: 'includes', placeholder: 'read_supplies' },
]

function matches(r: GroupRule | undefined, d: Device): boolean {
  if (!r) return false
  const has = (hay: string | undefined, needle?: string) => !needle || (hay || '').toLowerCase().includes(needle.toLowerCase())
  if (!has(d.make, r.makeContains) || !has(d.model, r.modelContains) || !has(d.location, r.locationContains)) return false
  if (r.deviceType && d.deviceType !== r.deviceType) return false
  if (r.state && d.state !== r.state) return false
  if (r.hasCapability && !(r.hasCapability in d.capabilities)) return false
  if (r.subnet) {
    const [net, bitsS] = r.subnet.split('/'); const bits = Number(bitsS)
    const ip = (s: string) => s.split('.').reduce((a, o) => ((a << 8) + (Number(o) || 0)) >>> 0, 0)
    if (!net || isNaN(bits)) return false
    const mask = bits === 0 ? 0 : (~0 << (32 - bits)) >>> 0
    if ((ip(d.address) & mask) >>> 0 !== (ip(net) & mask) >>> 0) return false
  }
  return true
}

export default function Groups({ user }: { user: User }) {
  const [groups, setGroups] = useState<Group[]>([])
  const [devices, setDevices] = useState<Device[]>([])
  const [selId, setSelId] = useState<string | null>(null)
  const [filter, setFilter] = useState('')
  const [draft, setDraft] = useState<Group | null>(null)
  const [msg, setMsg] = useState('')
  const canOperate = user.role !== 'viewer'
  const load = useCallback(() => {
    api.get<Group[]>('/api/groups').then((gs) => { setGroups(gs); setSelId((cur) => cur || gs[0]?.id || null) }).catch(() => {})
    api.get<{ devices: Device[] }>('/api/devices').then((r) => setDevices(r.devices)).catch(() => {})
  }, [])
  useEffect(() => { load() }, [load])
  const selected = groups.find((g) => g.id === selId) || null
  useEffect(() => { setDraft(selected ? { ...selected, rule: selected.rule ? { ...selected.rule } : undefined, memberIds: [...selected.memberIds] } : null); setMsg('') }, [selected])

  const members = useMemo(() => {
    if (!draft) return []
    return draft.kind === 'rule' ? devices.filter((d) => matches(draft.rule, d)) : devices.filter((d) => draft.memberIds.includes(d.id))
  }, [draft, devices])
  const savedMembers = useMemo(() => selected ? (selected.kind === 'rule' ? devices.filter((d) => matches(selected.rule, d)).map((d) => d.id) : selected.memberIds) : [], [selected, devices])
  const joins = members.filter((d) => !savedMembers.includes(d.id)).length
  const leaves = savedMembers.filter((id) => !members.some((d) => d.id === id)).length
  const dirty = draft && selected && (JSON.stringify({ n: draft.name, d: draft.description, k: draft.kind, r: draft.rule, m: [...draft.memberIds].sort() }) !== JSON.stringify({ n: selected.name, d: selected.description, k: selected.kind, r: selected.rule, m: [...selected.memberIds].sort() }))

  const newGroup = () => { const g: Group = { id: '', name: 'New group', description: '', kind: 'rule', rule: {}, memberIds: [] }; setGroups((gs) => [g, ...gs.filter((x) => x.id !== '')]); setSelId(''); setDraft(g) }
  const save = async () => {
    if (!draft) return
    setMsg('')
    try {
      if (!draft.id) {
        const g = await api.post<Group>('/api/groups', { name: draft.name, description: draft.description, kind: draft.kind, rule: draft.kind === 'rule' ? draft.rule : undefined, memberIds: draft.kind === 'static' ? draft.memberIds : [] })
        setSelId(g.id)
      } else {
        await api.patch(`/api/groups/${draft.id}`, { name: draft.name, description: draft.description, kind: draft.kind, rule: draft.kind === 'rule' ? draft.rule || {} : undefined })
        if (draft.kind === 'static') await api.put(`/api/groups/${draft.id}/members`, { deviceIds: draft.memberIds })
      }
      load(); setMsg('Saved.')
    } catch (e) { setMsg((e as Error).message) }
  }
  const remove = async () => { if (!selected || !confirm(`Delete group “${selected.name}”?`)) return; await api.del(`/api/groups/${selected.id}`); setSelId(null); load() }
  const ruleEntries = draft?.rule ? (Object.entries(draft.rule) as [Field, string][]).filter(([, v]) => v !== undefined && v !== '') : []
  const setRule = (k: Field, v: string) => setDraft((d) => d && ({ ...d, rule: { ...(d.rule || {}), [k]: v } }))
  const removeRule = (k: Field) => setDraft((d) => { if (!d) return d; const r = { ...(d.rule || {}) }; delete r[k]; return { ...d, rule: r } })
  const addable = fields.filter((f) => !ruleEntries.some(([k]) => k === f.k))
  const shown = groups.filter((g) => !filter || g.name.toLowerCase().includes(filter.toLowerCase()))
  const countFor = (g: Group) => g.kind === 'rule' ? devices.filter((d) => matches(g.rule, d)).length : g.memberIds.length
  const ruleText = (g: Group) => g.kind === 'static' ? 'Static' : `Rule-based · ${Object.entries(g.rule || {}).filter(([, v]) => v).map(([k, v]) => `${fields.find((f) => f.k === k)?.label || k} ${fields.find((f) => f.k === k)?.op || ''} ${v}`).join(', ') || 'matches everything'}`

  return (
    <>
      <div className="ph">
        <div><h1 className="h1">Groups</h1><div className="cap num">{groups.filter((g) => g.id).length} groups · {groups.filter((g) => g.id && g.kind === 'rule').length} rule-based, {groups.filter((g) => g.id && g.kind === 'static').length} static · rule groups re-evaluate on every poll</div></div>
        {canOperate && <Btn sm variant="pri" onClick={newGroup}><Ic.plus />New group</Btn>}
      </div>
      <div className="cols">
        <Card aria-label="Group list" style={{ flex: '1 1 300px', maxWidth: 380 }}>
          <div style={{ padding: '10px 12px', borderBottom: '1px solid var(--border)' }}><SearchBox sm value={filter} onChange={setFilter} placeholder="Filter groups…" /></div>
          {shown.length === 0 && <div className="cb"><EmptyState icon={<Ic.layers size={20} />} title="No groups yet" text="Groups are the target for polls, profiles, policies and campaigns." action={canOperate ? <Btn sm variant="pri" onClick={newGroup}>New group</Btn> : undefined} /></div>}
          {shown.map((g) => (
            <button key={g.id || 'new'} className={`grow${g.id === selId ? ' on' : ''}`} onClick={() => setSelId(g.id)} aria-current={g.id === selId ? 'true' : undefined}>
              {g.kind === 'rule' ? <Ic.filter style={{ color: g.id === selId ? 'var(--accent)' : 'var(--text-3)' }} /> : <Ic.sidebar style={{ color: 'var(--text-3)' }} />}
              <span style={{ flex: 1, minWidth: 0 }}><b style={{ fontWeight: g.id === selId ? 600 : 500 }}>{g.name}</b><span className="cap" style={{ display: 'block', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{ruleText(g)}</span></span>
              <span className={`num${g.id === selId ? '' : ' cap'}`} style={{ fontWeight: 600 }}>{countFor(g)}</span>
            </button>
          ))}
          <div className="cf cap" style={{ display: 'flex', gap: 14 }}><span style={{ display: 'inline-flex', alignItems: 'center', gap: 5 }}><Ic.filter size={13} />Rule-based</span><span style={{ display: 'inline-flex', alignItems: 'center', gap: 5 }}><Ic.sidebar size={13} />Static</span></div>
        </Card>
        <div className="stack" style={{ flex: '3 1 560px' }}>
          {!draft ? <Card><div className="cb cap">Select a group to see and edit it.</div></Card> : (
            <>
              <Card title={draft.name || 'Untitled group'} id="ge" sub={draft.kind === 'rule' ? 'Rule-based' : 'Static · hand-picked members'} right={canOperate ? <div className="row">{selected?.id && <Btn sm variant="dng" onClick={remove}>Delete</Btn>}<Btn sm variant="ghost" onClick={() => setDraft(selected ? { ...selected } : null)} disabled={!dirty}>Discard</Btn><Btn sm variant="pri" onClick={save} disabled={!dirty && !!draft.id}>Save group</Btn></div> : <Tag>Read-only · Viewer</Tag>}>
                <div className="cb" style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
                  <div style={{ display: 'grid', gridTemplateColumns: 'minmax(200px, 1fr) minmax(200px, 2fr)', gap: 10 }}>
                    <label className="cap">Name<input className="fin" value={draft.name} disabled={!canOperate} onChange={(e) => setDraft({ ...draft, name: e.target.value })} style={{ marginTop: 4 }} /></label>
                    <label className="cap">Description<input className="fin" value={draft.description} disabled={!canOperate} onChange={(e) => setDraft({ ...draft, description: e.target.value })} style={{ marginTop: 4 }} /></label>
                  </div>
                  <div className="row"><span>Membership</span><Seg label="Kind" value={draft.kind} onChange={(k) => canOperate && setDraft({ ...draft, kind: k, rule: k === 'rule' ? draft.rule || {} : draft.rule })} options={[{ v: 'rule', label: 'rule-based · all conditions' }, { v: 'static', label: 'static · hand-picked' }]} /></div>
                  {draft.kind === 'rule' && (<>
                    {ruleEntries.map(([k, v], i) => { const f = fields.find((x) => x.k === k)!; const hits = devices.filter((d) => matches({ [k]: v } as GroupRule, d)).length; return (
                      <div key={k}>{i > 0 && <div className="join">AND</div>}
                        <div className="cond">
                          <span className="fin" style={{ width: 150, display: 'inline-flex', alignItems: 'center' }}>{f.label}</span><span className="fin" style={{ width: 110, display: 'inline-flex', alignItems: 'center', color: 'var(--text-2)' }}>{f.op}</span>
                          {f.options ? <select className="fin mono" style={{ flex: '1 1 180px' }} value={v} disabled={!canOperate} onChange={(e) => setRule(k, e.target.value)}>{f.options.map((o) => <option key={o} value={o}>{o}</option>)}</select> : <input className="fin mono" style={{ flex: '1 1 180px' }} value={v} placeholder={f.placeholder} disabled={!canOperate} onChange={(e) => setRule(k, e.target.value)} />}
                          <span className="cap num" style={{ width: 92, textAlign: 'right' }}>{hits} devices</span>
                          {canOperate && <Btn sm variant="ghost" ic aria-label="Remove condition" onClick={() => removeRule(k)}><Ic.x /></Btn>}
                        </div></div>) })}
                    {canOperate && addable.length > 0 && <div className="row"><label className="chip"><Ic.plus />Add condition<select aria-label="Add condition" value="" onChange={(e) => { const f = fields.find((x) => x.k === e.target.value); if (f) setRule(f.k, f.options ? f.options[0] : '') }}><option value="">field…</option>{addable.map((f) => <option key={f.k} value={f.k}>{f.label}</option>)}</select></label><span className="cap">Fields: model, vendor, subnet, location, device type, lifecycle, capability</span></div>}
                  </>)}
                  {draft.kind === 'static' && canOperate && <div className="cap">Tick devices in the member list below to add or remove them.</div>}
                  {msg && <div className="cap">{msg}</div>}
                </div>
              </Card>
              <Card title="Live member preview" id="mp" right={<Link to={selected?.id ? `/devices?group=${selected.id}` : '/devices'} className="cap" style={{ color: 'var(--accent)' }}>Open as filter in Devices →</Link>}>
                <div className="ch" style={{ borderBottom: 0, paddingBottom: 0 }}><div style={{ display: 'flex', alignItems: 'baseline', gap: 10 }}><span className="num" style={{ fontSize: 20, fontWeight: 600, color: 'var(--accent)' }}>{members.length}</span><span className="cap">devices match · {dirty ? `${joins ? `+${joins} join` : ''}${joins && leaves ? ', ' : ''}${leaves ? `${leaves} leave` : ''}${!joins && !leaves ? 'no change' : ''} on save` : selected?.id ? 'saved' : 'not saved yet'}</span></div></div>
                <div className="tw"><table className="tbl" style={{ minWidth: 640 }}>
                  <thead><tr>{draft.kind === 'static' && canOperate && <th style={{ width: 36 }} />}<th>Device</th><th>Model</th><th>IP</th><th>Location</th><th>Status</th><th>Change on save</th></tr></thead>
                  <tbody>{(draft.kind === 'static' && canOperate ? devices : members).map((d) => { const inSaved = savedMembers.includes(d.id); const inDraft = members.some((m) => m.id === d.id); return (
                    <tr key={d.id} className={inDraft ? '' : 'dim'}>
                      {draft.kind === 'static' && canOperate && <td><input className="cbx" type="checkbox" aria-label={`Member ${deviceName(d)}`} checked={draft.memberIds.includes(d.id)} onChange={(e) => setDraft({ ...draft, memberIds: e.target.checked ? [...draft.memberIds, d.id] : draft.memberIds.filter((x) => x !== d.id) })} /></td>}
                      <td><Link to={`/devices/${d.id}`} style={{ fontWeight: 500 }}>{deviceName(d)}</Link></td><td className="mono">{d.model}</td><td className="mono">{d.address}</td><td className="cap">{d.location || '—'}</td><td><DevicePill d={d} xs /></td>
                      <td className="cap">{inDraft && !inSaved ? <span className="pill ok xs"><Ic.plus />joins</span> : !inDraft && inSaved ? <span className="pill off xs"><Ic.x />leaves</span> : inDraft ? 'already a member' : ''}</td>
                    </tr>) })}
                    {members.length === 0 && draft.kind === 'rule' && <tr><td colSpan={7} className="cap">No devices match these conditions.</td></tr>}
                  </tbody>
                </table></div>
              </Card>
            </>
          )}
        </div>
      </div>
    </>
  )
}
