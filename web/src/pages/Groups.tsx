import { useCallback, useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { api } from '../api'
import type { Device, Group, GroupRule, User } from '../types'
import { deviceName } from '../types'

const emptyRule: GroupRule = {}

export default function Groups({ user }: { user: User }) {
  const [groups, setGroups] = useState<Group[]>([])
  const [devices, setDevices] = useState<Device[]>([])
  const [selected, setSelected] = useState<Group | null>(null)
  const [members, setMembers] = useState<Device[]>([])
  const [form, setForm] = useState<{ name: string; description: string; kind: 'static' | 'rule'; rule: GroupRule }>({ name: '', description: '', kind: 'static', rule: emptyRule })
  const [msg, setMsg] = useState('')
  const canOperate = user.role !== 'viewer'

  const load = useCallback(() => {
    api.get<Group[]>('/api/groups').then(setGroups).catch(() => {})
    api.get<{ devices: Device[] }>('/api/devices').then((r) => setDevices(r.devices)).catch(() => {})
  }, [])
  useEffect(() => { load() }, [load])
  useEffect(() => {
    if (!selected) { setMembers([]); return }
    api.get<Device[]>(`/api/groups/${selected.id}/devices`).then(setMembers).catch(() => setMembers([]))
  }, [selected])

  const create = async (e: React.FormEvent) => {
    e.preventDefault(); setMsg('')
    try {
      const body: Partial<Group> = { name: form.name, description: form.description, kind: form.kind }
      if (form.kind === 'rule') body.rule = form.rule
      await api.post('/api/groups', body)
      setForm({ name: '', description: '', kind: 'static', rule: emptyRule }); load()
    } catch (err) { setMsg((err as Error).message) }
  }
  const toggleMember = async (g: Group, d: Device) => {
    const ids = g.memberIds.includes(d.id) ? g.memberIds.filter((x) => x !== d.id) : [...g.memberIds, d.id]
    const updated = await api.put<Group>(`/api/groups/${g.id}/members`, { deviceIds: ids })
    setSelected(updated); load()
  }
  const remove = async (g: Group) => {
    if (!confirm(`Delete group "${g.name}"?`)) return
    await api.del(`/api/groups/${g.id}`); setSelected(null); load()
  }

  return (
    <>
      <h1>Groups</h1>
      <div className="grid cols-2">
        <div>
          <div className="panel">
            <table>
              <thead><tr><th>Name</th><th>Kind</th><th>Members</th></tr></thead>
              <tbody>
                {groups.map((g) => (
                  <tr key={g.id} className="clickable" onClick={() => setSelected(g)}>
                    <td><strong>{g.name}</strong><div className="muted small">{g.description}</div></td>
                    <td><span className="badge outline">{g.kind}</span></td>
                    <td>{g.kind === 'static' ? g.memberIds.length : <span className="muted small">{ruleText(g.rule)}</span>}</td>
                  </tr>
                ))}
                {groups.length === 0 && <tr><td colSpan={3} className="muted">No groups yet.</td></tr>}
              </tbody>
            </table>
          </div>
          {canOperate && (
            <form className="panel" onSubmit={create}>
              <h2>New group</h2>
              <div className="grid cols-2">
                <label className="field">Name<input value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} required /></label>
                <label className="field">Kind<select value={form.kind} onChange={(e) => setForm({ ...form, kind: e.target.value as 'static' | 'rule' })}><option value="static">static (hand-picked)</option><option value="rule">rule-based</option></select></label>
              </div>
              <label className="field" style={{ marginTop: 8 }}>Description<input value={form.description} onChange={(e) => setForm({ ...form, description: e.target.value })} /></label>
              {form.kind === 'rule' && (
                <div className="grid cols-2" style={{ marginTop: 8 }}>
                  <label className="field">Make contains<input value={form.rule.makeContains || ''} onChange={(e) => setForm({ ...form, rule: { ...form.rule, makeContains: e.target.value } })} /></label>
                  <label className="field">Model contains<input value={form.rule.modelContains || ''} onChange={(e) => setForm({ ...form, rule: { ...form.rule, modelContains: e.target.value } })} /></label>
                  <label className="field">Location contains<input value={form.rule.locationContains || ''} onChange={(e) => setForm({ ...form, rule: { ...form.rule, locationContains: e.target.value } })} /></label>
                  <label className="field">Subnet (CIDR)<input value={form.rule.subnet || ''} onChange={(e) => setForm({ ...form, rule: { ...form.rule, subnet: e.target.value } })} placeholder="10.0.0.0/24" /></label>
                  <label className="field">Device type<select value={form.rule.deviceType || ''} onChange={(e) => setForm({ ...form, rule: { ...form.rule, deviceType: e.target.value } })}><option value="">any</option><option value="printer">printer</option><option value="mfp">mfp</option></select></label>
                  <label className="field">Has capability<input value={form.rule.hasCapability || ''} onChange={(e) => setForm({ ...form, rule: { ...form.rule, hasCapability: e.target.value } })} placeholder="read_supplies" /></label>
                </div>
              )}
              <div className="row" style={{ marginTop: 10 }}><button className="primary">Create</button><span className="error small">{msg}</span></div>
            </form>
          )}
        </div>
        <div className="panel">
          {!selected && <div className="muted">Select a group to see its devices.</div>}
          {selected && (
            <>
              <div className="row between"><h2>{selected.name} <span className="badge outline">{selected.kind}</span></h2>{canOperate && <button className="danger" onClick={() => remove(selected)}>Delete</button>}</div>
              {selected.kind === 'rule' && <div className="muted small" style={{ marginBottom: 8 }}>Rule: {ruleText(selected.rule)}</div>}
              <table><tbody>
                {(selected.kind === 'static' ? devices : members).map((d) => {
                  const member = selected.kind === 'static' ? selected.memberIds.includes(d.id) : true
                  return (
                    <tr key={d.id}>
                      {selected.kind === 'static' && <td style={{ width: 30 }}><input type="checkbox" checked={member} disabled={!canOperate} onChange={() => toggleMember(selected, d)} /></td>}
                      <td><Link to={`/devices/${d.id}`}>{deviceName(d)}</Link><div className="muted small">{d.model} · {d.address}</div></td>
                    </tr>
                  )
                })}
                {selected.kind === 'rule' && members.length === 0 && <tr><td className="muted">No devices match this rule.</td></tr>}
              </tbody></table>
            </>
          )}
        </div>
      </div>
    </>
  )
}

function ruleText(r?: GroupRule): string {
  if (!r) return ''
  return Object.entries(r).filter(([, v]) => v).map(([k, v]) => `${k}=${v}`).join(', ') || 'matches everything'
}
