import { useMemo, useState } from 'react'
import { api } from '../../api'
import { Ic } from '../icons'
import { Btn, Tag } from '../ui'
import type { Catalog, CatalogDef, Profile, SettingChange } from '../../types-config'
import { areaLabel } from '../../types-config'
import { SettingInput, isSecret, valueComplete } from './shared'
import SettingPicker, { settingDragType } from './SettingPicker'

function defaultFor(def: CatalogDef): unknown {
  switch (def.type) { case 'bool': return true; case 'int': return ''; case 'list': return []; case 'enum': return ''; default: return '' }
}

/**
 * Drawer to create or edit a profile. Settings come from the catalogue grouped
 * by area; secret-typed keys are hidden because the API refuses them.
 */
export default function ProfileEditor({ profile, catalog, onClose, onSaved }: { profile: Profile | null; catalog: Catalog; onClose: () => void; onSaved: (p: Profile) => void }) {
  const [name, setName] = useState(profile?.name || '')
  const [description, setDescription] = useState(profile?.description || '')
  const [settings, setSettings] = useState<SettingChange[]>(profile?.settings.map((s) => ({ ...s })) || [])
  const [error, setError] = useState('')
  const [saving, setSaving] = useState(false)
  const [picking, setPicking] = useState(!profile)
  const [dragging, setDragging] = useState(false)
  const byKey = useMemo(() => new Map(catalog.settings.map((d) => [d.key, d])), [catalog])
  const chosen = new Set(settings.map((s) => s.key))
  const addable = catalog.settings.filter((d) => !isSecret(d) && !chosen.has(d.key))
  const grouped = catalog.areas.map((area) => ({ area, items: settings.filter((s) => byKey.get(s.key)?.area === area) })).filter((g) => g.items.length > 0)
  const unknown = settings.filter((s) => !byKey.has(s.key))
  const incomplete = settings.filter((s) => { const d = byKey.get(s.key); return d && !valueComplete(d, s.value) }).length
  const canSave = name.trim() !== '' && settings.length > 0 && incomplete === 0 && !saving

  const add = (key: string) => { const d = byKey.get(key); if (d) setSettings((s) => [...s, { key, value: defaultFor(d) }]) }
  const set = (key: string, value: unknown) => setSettings((s) => s.map((x) => (x.key === key ? { ...x, value } : x)))
  const remove = (key: string) => setSettings((s) => s.filter((x) => x.key !== key))
  const save = async () => {
    setSaving(true); setError('')
    const body = { name: name.trim(), description, settings }
    try {
      const p = profile ? await api.patch<Profile>(`/api/profiles/${profile.id}`, body) : await api.post<Profile>('/api/profiles', body)
      onSaved(p)
    } catch (e) { setError((e as Error).message) } finally { setSaving(false) }
  }

  return (
    <div className="drawer-backdrop" onClick={onClose}>
      <div className="drawer" role="dialog" aria-modal="true" aria-label={profile ? 'Edit profile' : 'New profile'} onClick={(e) => e.stopPropagation()}>
        <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 8 }}>
          <div><h2 className="h1" style={{ fontSize: 17 }}>{profile ? 'Edit profile' : 'New profile'}</h2><div className="cap">Desired settings, vendor-neutral. Applying is always a dry run first.</div></div>
          <Btn sm variant="ghost" ic aria-label="Close" onClick={onClose}><Ic.x /></Btn>
        </div>
        <div style={{ display: 'grid', gridTemplateColumns: '1fr', gap: 10 }}>
          <label className="cap">Name<input className="fin" value={name} onChange={(e) => setName(e.target.value)} style={{ marginTop: 4 }} placeholder="e.g. Energy — weekday" autoFocus /></label>
          <label className="cap">Description<input className="fin" value={description} onChange={(e) => setDescription(e.target.value)} style={{ marginTop: 4 }} placeholder="what this profile standardises" /></label>
        </div>
        <div className="row between">
          <span style={{ fontWeight: 500 }}>Settings <span className="cap num">· {settings.length}</span></span>
          <Btn sm aria-expanded={picking} onClick={() => setPicking((p) => !p)}>{picking ? <><Ic.x />Close list</> : <><Ic.plus />Add settings</>}</Btn>
        </div>
        {picking && <SettingPicker settings={addable} areas={catalog.areas} onAdd={add} onDragging={setDragging} />}
        <div className={`sp-drop${dragging ? ' on' : ''}`} onDragOver={(e) => { if (e.dataTransfer.types.includes(settingDragType)) { e.preventDefault(); e.dataTransfer.dropEffect = 'copy' } }} onDrop={(e) => { const key = e.dataTransfer.getData(settingDragType); if (key) { e.preventDefault(); if (!chosen.has(key)) add(key) } setDragging(false) }}>
        {settings.length === 0 && <div className="cap" style={{ padding: '12px 0', borderTop: '1px solid var(--border)' }}>No settings yet. Add the settings this profile should enforce; anything not listed is left as the printer has it.</div>}
        <div className="card" style={{ display: settings.length ? 'block' : 'none' }}>
          <div style={{ display: 'grid', gridTemplateColumns: '150px minmax(0, 1fr) 28px', gap: '8px 12px', alignItems: 'center', padding: '8px 14px', borderBottom: '1px solid var(--border)' }}>
            <span className="lbl">Setting</span><span className="lbl">Desired in profile</span><span />
          </div>
          {grouped.map((g) => (
            <div key={g.area}>
              <div className="cap" style={{ padding: '8px 14px 2px', fontWeight: 600, textTransform: 'uppercase', letterSpacing: '.06em', fontSize: 11 }}>{areaLabel(g.area)}</div>
              {g.items.map((s) => {
                const def = byKey.get(s.key)!
                const ok = valueComplete(def, s.value)
                return (
                  <div key={s.key} style={{ display: 'grid', gridTemplateColumns: '150px minmax(0, 1fr) 28px', gap: '8px 12px', alignItems: 'center', padding: '6px 14px' }}>
                    <label className="cap" htmlFor={`ps-${s.key}`} title={s.key} style={{ color: 'var(--text-2)' }}>{def.name}<div className="mono" style={{ fontSize: 10, color: 'var(--text-3)' }}>{s.key}</div></label>
                    <div><SettingInput def={def} value={s.value} onChange={(v) => set(s.key, v)} id={`ps-${s.key}`} />{!ok && <div style={{ fontSize: 11, color: 'var(--crit)', marginTop: 3 }}>{def.type === 'int' ? 'Whole number required' : 'A value is required'}</div>}{def.help && <div className="cap" style={{ fontSize: 11, marginTop: 3 }}>{def.help}</div>}</div>
                    <Btn sm variant="ghost" ic aria-label={`Remove ${def.name}`} onClick={() => remove(s.key)}><Ic.x /></Btn>
                  </div>
                )
              })}
            </div>
          ))}
          {unknown.length > 0 && <div className="cf cap">Vendor keys: {unknown.map((s) => <Tag key={s.key} mono>{s.key}</Tag>)}<Btn sm variant="ghost" onClick={() => setSettings((s) => s.filter((x) => byKey.has(x.key)))}>Remove</Btn></div>}
        </div>
        </div>
        <div className="cap" style={{ display: 'flex', gap: 6, alignItems: 'center' }}><Ic.lock size={13} />Secret settings (communities, passwords) cannot be stored in a profile and are not offered here.</div>
        {error && <div className="banner crit" role="alert"><Ic.crit style={{ color: 'var(--crit)' }} /><div style={{ flex: 1 }}>{error}</div></div>}
        <div style={{ marginTop: 'auto', display: 'flex', justifyContent: 'flex-end', gap: 8, paddingTop: 12, borderTop: '1px solid var(--border)' }}>
          <Btn onClick={onClose}>Cancel</Btn>
          <Btn variant="pri" onClick={save} disabled={!canSave}>{saving ? 'Saving…' : profile ? 'Save profile' : 'Create profile'}</Btn>
        </div>
      </div>
    </div>
  )
}
